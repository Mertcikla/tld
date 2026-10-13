package warnings

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mertcikla/tld/v2/internal/ignore"
	"github.com/mertcikla/tld/v2/internal/repolink"
	"github.com/mertcikla/tld/v2/internal/sourcelink"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// RepoCoverage is ARC206's per-repository view of how much of the indexed code
// the workspace's source-linked elements cover. Directories are the folders
// that directly contain an indexed file, so a flat repository has one
// directory (".") and A/B/C.go contributes "A/B".
type RepoCoverage struct {
	Name     string
	Percent  int
	Covered  int
	Total    int
	Unlinked []string
	// Excluded counts the repository's indexed files the workspace exclude
	// rules (from .tld.yaml) drop from the coverage denominator.
	Excluded int
}

// linkCoverageViolationLimit caps how many unlinked directories are listed in
// one violation, keeping the output readable on large repositories.
const linkCoverageViolationLimit = 8

// WithLinkTargets supplies the indexed repositories, with the files each one
// contains, that workspace elements can link to. ARC206 measures the share of
// those directories covered by user-authored source links. When unset, or when
// no repository carries files, the rule is inert.
func WithLinkTargets(repos []LinkedRepository) Option {
	return func(ctx *warningContext) {
		ctx.linkTargets = repos
	}
}

// WithIgnoreRules supplies the workspace exclusion rules (.tld.yaml exclude
// plus the always-on implicit excludes). Indexed files they match are removed
// from the ARC206 coverage denominator so deliberately out-of-scope code does
// not read as unlinked. A nil rules value still drops the implicit excludes.
func WithIgnoreRules(rules *ignore.Rules) Option {
	return func(ctx *warningContext) {
		ctx.ignoreRules = rules
	}
}

// LinkCoverage computes, for every indexed repository the workspace links into,
// the share of its file directories the source-linked elements cover. It is
// exported so callers can render the full report on demand, even when no
// warning threshold was crossed.
func LinkCoverage(ws *workspace.Workspace, opts ...Option) WarningScore {
	if ws == nil {
		return WarningScore{}
	}
	ctx := newWarningContext(ws, opts...)
	ctx.prepareData()
	return ctx.linkCoverageReport()
}

// linkCoverageThreshold is the coverage percentage ARC206 requires. The
// workspace configures it via validation.link_coverage_percent; unset or
// invalid values fall back to the default, and the result is clamped to 0-100.
func linkCoverageThreshold(ws *workspace.Workspace) int {
	percent := workspace.DefaultLinkCoveragePercent
	if ws != nil {
		if configured := ws.Config.Validation.LinkCoveragePercent; configured > 0 {
			percent = configured
		}
	}
	if percent > 100 {
		return 100
	}
	if percent < 0 {
		return 0
	}
	return percent
}

// LinkCoverageThreshold is the coverage percentage ARC206 requires for the
// workspace, falling back to workspace.DefaultLinkCoveragePercent when
// validation.link_coverage_percent is unset. Callers use it to report the
// threshold next to the coverage numbers.
func LinkCoverageThreshold(ws *workspace.Workspace) int {
	return linkCoverageThreshold(ws)
}

func (ctx *warningContext) linkCoverageReport() WarningScore {
	var report WarningScore
	if len(ctx.linkTargets) == 0 {
		report.Reasoning = []string{"No indexed repository is available; link coverage does not apply."}
		return report
	}

	links := ctx.linkDirectories()
	for _, repo := range ctx.linkTargets {
		if len(links[repo.ID]) == 0 {
			continue
		}
		inScope, excluded := inScopeIndexedFiles(repo.Paths, ctx.ignoreRules)
		if len(inScope) == 0 {
			continue
		}
		coverage := repositoryCoverage(repo.Name, inScope, links[repo.ID])
		coverage.Excluded = excluded
		report.Repos = append(report.Repos, coverage)
		report.Grounded += coverage.Covered
		report.Eligible += coverage.Total
		report.Excluded += excluded
	}
	sort.Slice(report.Repos, func(i, j int) bool { return report.Repos[i].Name < report.Repos[j].Name })
	report.Value = groundingValue(report.Grounded, report.Eligible)
	report.Reasoning = linkCoverageReasoning(report, ctx.linkCoverageMin)
	return report
}

// linkDirectories resolves every source-linked, user-authored element to the
// indexed repository it points into, keyed by repository id. Codeindex-owned,
// externally documented and ignored elements are excluded so the coverage
// reflects the diagram the user authored rather than a generated map.
func (ctx *warningContext) linkDirectories() map[string][]dirLink {
	out := make(map[string][]dirLink)
	if len(ctx.linkTargets) == 0 || ctx.ws == nil {
		return out
	}
	repos := make([]repolink.Repository, 0, len(ctx.linkTargets))
	for _, target := range ctx.linkTargets {
		repos = append(repos, repolink.Repository{ID: target.ID, Name: target.Name, Root: target.Root, RemoteURL: target.RemoteURL})
	}
	for _, element := range ctx.ws.Elements {
		if element == nil || ctx.isCodeindexElement(element) || hasExternalTag(element) || hasIgnoredTag(element) {
			continue
		}
		filePath := strings.TrimSpace(element.FilePath)
		if filePath == "" {
			continue
		}
		folder := strings.HasSuffix(filePath, "/")
		base := strings.TrimRight(strings.ReplaceAll(sourcelink.BasePath(filePath), "\\", "/"), "/")
		if base == "" {
			continue
		}
		repo, ok := repolink.Resolve(element.RepositoryID, element.Repo, base, repos)
		if !ok {
			// A workspace with a single indexed repository resolves unscoped
			// links to it, mirroring the editor's repository resolution.
			if len(repos) != 1 {
				continue
			}
			repo = repos[0]
		}
		relative, ok := repoRelativePath(base, repo.Root)
		if !ok {
			continue
		}
		out[repo.ID] = append(out[repo.ID], dirLink{dir: relative, folder: folder})
	}
	return out
}

// dirLink is a source-linked element pointer normalized to directory form: the
// cleaned repository-relative path plus whether it addresses a whole folder
// (a trailing slash), which covers every directory beneath it.
type dirLink struct {
	dir    string
	folder bool
}

// inScopeIndexedFiles drops the indexed files the workspace excludes, returning
// the files left to measure and how many were dropped. The exclusion rules are
// the workspace's .tld.yaml exclude list; the implicit ignores (vendor,
// node_modules, .venv, .git) always apply, also with nil rules.
func inScopeIndexedFiles(paths []string, rules *ignore.Rules) ([]string, int) {
	inScope := make([]string, 0, len(paths))
	excluded := 0
	for _, filePath := range paths {
		if rules.ShouldIgnorePath(filePath) {
			excluded++
			continue
		}
		inScope = append(inScope, filePath)
	}
	return inScope, excluded
}

// repositoryCoverage maps a repository's indexed files onto the directories
// that directly contain them and reports which of those directories the links
// cover. Repositories the workspace does not link into are never scored, so a
// repository with no links has Total == 0 rather than 0% coverage.
func repositoryCoverage(name string, paths []string, links []dirLink) RepoCoverage {
	nodes := indexedDirectories(paths)
	if len(nodes) == 0 {
		return RepoCoverage{Name: name}
	}
	covered := make(map[string]bool, len(nodes))
	for _, link := range links {
		applyDirLink(covered, nodes, link)
	}
	var uncovered []string
	for node := range nodes {
		if !covered[node] {
			uncovered = append(uncovered, node)
		}
	}
	sort.Strings(uncovered)
	return RepoCoverage{
		Name:     name,
		Percent:  len(covered) * 100 / len(nodes),
		Covered:  len(covered),
		Total:    len(nodes),
		Unlinked: topMostDirectories(uncovered),
	}
}

// indexedDirectories is the set of directories that directly contain an
// indexed file, e.g. ["A/B/C.go", "A/D.go"] -> {"A/B", "A/D"} and ["a.go"]
// -> {"."}.
func indexedDirectories(paths []string) map[string]struct{} {
	nodeSet := make(map[string]struct{}, len(paths))
	for _, filePath := range paths {
		dir := strings.Trim(filepath.ToSlash(filePath), "/")
		if dir == "" {
			continue
		}
		dir = path.Dir(dir)
		if dir == "" || dir == "/" {
			dir = "."
		}
		nodeSet[dir] = struct{}{}
	}
	return nodeSet
}

// applyDirLink marks the directories a single link covers. A file link covers
// the directory that contains it (and the directory itself when the link
// points straight at one); a folder link covers its whole subtree.
func applyDirLink(covered map[string]bool, nodes map[string]struct{}, link dirLink) {
	if link.dir == "" {
		return
	}
	if link.folder {
		for node := range nodes {
			if link.dir == "." || node == link.dir || strings.HasPrefix(node, link.dir+"/") {
				covered[node] = true
			}
		}
		return
	}
	parent := path.Dir(link.dir)
	if parent == link.dir {
		parent = "."
	}
	if _, ok := nodes[parent]; ok {
		covered[parent] = true
	}
	if _, ok := nodes[link.dir]; ok {
		covered[link.dir] = true
	}
}

// topMostDirectories drops directories nested under another unlinked directory
// so the reported list names the smallest set of places to link next. The input
// must be sorted, which keeps every descendant contiguous after its ancestor.
func topMostDirectories(uncovered []string) []string {
	var out []string
	for _, dir := range uncovered {
		if len(out) > 0 {
			last := out[len(out)-1]
			if last == "." || strings.HasPrefix(dir, last+"/") {
				continue
			}
		}
		out = append(out, dir)
	}
	return out
}

// repoRelativePath converts a linked path to the repository-relative slash
// form the codeindex stores, resolving absolute paths that live inside the
// repository root. It reports false for relative paths that escape the
// repository and for absolute paths outside it.
func repoRelativePath(base, root string) (string, bool) {
	clean := filepath.ToSlash(filepath.Clean(base))
	if !filepath.IsAbs(base) {
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			return "", false
		}
		return clean, true
	}
	rootClean := filepath.ToSlash(filepath.Clean(root))
	if rootClean == "" || rootClean == "." {
		return clean, true
	}
	if clean == rootClean {
		return ".", true
	}
	if strings.HasPrefix(clean, rootClean+"/") {
		return strings.TrimPrefix(clean, rootClean+"/"), true
	}
	return "", false
}

func linkCoverageReasoning(report WarningScore, minimum int) []string {
	if report.Eligible == 0 {
		return []string{"No indexed repository is linked into; link coverage does not apply."}
	}
	lines := []string{
		fmt.Sprintf("Linked elements cover %d of %d indexed directories (%d/10).", report.Grounded, report.Eligible, report.Value),
	}
	if report.Excluded > 0 {
		lines = append(lines, fmt.Sprintf("%d indexed file(s) are excluded from coverage by the workspace exclude rules.", report.Excluded))
	}
	var weak []string
	for _, repo := range report.Repos {
		if repo.Total == 0 || repo.Percent >= minimum {
			continue
		}
		weak = append(weak, fmt.Sprintf("%s (%d%%; unlinked: %s)", repo.Name, repo.Percent, formatUnlinkedDirectories(repo.Unlinked)))
	}
	if len(weak) > 0 {
		lines = append(lines, "Repositories below the "+fmt.Sprintf("%d%%", minimum)+" coverage threshold: "+strings.Join(weak, ", ")+".")
	}
	return lines
}

func formatUnlinkedDirectories(dirs []string) string {
	if len(dirs) <= linkCoverageViolationLimit {
		return strings.Join(dirs, ", ")
	}
	remaining := len(dirs) - linkCoverageViolationLimit
	return fmt.Sprintf("%s (+%d more)", strings.Join(dirs[:linkCoverageViolationLimit], ", "), remaining)
}

// linkCoverageViolation renders one ARC206 violation for a repository below the
// coverage threshold.
func linkCoverageViolation(repo RepoCoverage) string {
	return fmt.Sprintf("Repository %q is %d%% linked (%d/%d directories); unlinked: %s",
		repo.Name, repo.Percent, repo.Covered, repo.Total, formatUnlinkedDirectories(repo.Unlinked))
}
