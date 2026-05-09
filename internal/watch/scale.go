package watch

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	tldgit "github.com/mertcikla/tld/internal/git"
	"github.com/mertcikla/tld/internal/ignore"
)

type scanPlan struct {
	Strategy            string
	Mode                string
	Reason              string
	TrackedFiles        int
	SelectedFiles       int
	SkippedTrackedFiles int
	Limited             bool
	Files               []string
	Warnings            []string
}

func planScan(repoRoot string, settings Settings, rules *ignore.Rules) (scanPlan, error) {
	settings = NormalizeSettings(settings)
	tracked, err := tldgit.ListTrackedFiles(repoRoot, settings.Scale.MaxTrackedFiles+1)
	if err != nil {
		return scanPlan{}, err
	}
	plan := scanPlan{
		Strategy:     settings.Scale.Strategy,
		Mode:         "full",
		TrackedFiles: tracked.Total,
	}
	switch settings.Scale.Strategy {
	case ScanStrategyFull:
		plan.Reason = "full scan requested"
		return plan, nil
	case ScanStrategyAbort:
		if tracked.Total > settings.Scale.MaxTrackedFiles || tracked.Capped {
			return plan, fmt.Errorf("repository has more than %d tracked files; watch.scale.strategy=abort", settings.Scale.MaxTrackedFiles)
		}
		plan.Reason = "repository below scale threshold"
		return plan, nil
	case ScanStrategyLimited:
		plan.Reason = "limited scan requested"
	default:
		if tracked.Total <= settings.Scale.MaxTrackedFiles && !tracked.Capped {
			plan.Reason = "repository below scale threshold"
			return plan, nil
		}
		plan.Reason = fmt.Sprintf("tracked files exceed %d", settings.Scale.MaxTrackedFiles)
	}
	plan.Mode = "limited"
	plan.Limited = true
	plan.Files = selectLimitedFiles(repoRoot, tracked.Files, settings, rules)
	plan.SelectedFiles = len(plan.Files)
	plan.SkippedTrackedFiles = tracked.Total - len(plan.Files)
	if plan.SkippedTrackedFiles < 0 {
		plan.SkippedTrackedFiles = 0
	}
	plan.Warnings = append(plan.Warnings, fmt.Sprintf("limited view: %s; selected %d high-signal files out of %d tracked files", plan.Reason, plan.SelectedFiles, plan.TrackedFiles))
	return plan, nil
}

func selectLimitedFiles(repoRoot string, tracked []string, settings Settings, rules *ignore.Rules) []string {
	settings = NormalizeSettings(settings)
	allowed := map[string]struct{}{}
	for _, language := range settings.Languages {
		allowed[language] = struct{}{}
	}
	if rules == nil {
		rules = &ignore.Rules{}
	}
	type candidate struct {
		path  string
		score int
	}
	var candidates []candidate
	seen := map[string]struct{}{}
	for _, rel := range tracked {
		rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
		if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || rules.ShouldIgnorePath(rel) {
			continue
		}
		score := highSignalScore(rel)
		abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
		language, parseable, ok := watchedFileLanguage(abs)
		if score == 0 || !ok {
			continue
		}
		if parseable && !languageAllowed(language, allowed) {
			continue
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		candidates = append(candidates, candidate{path: abs, score: score})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].path < candidates[j].path
	})
	limit := settings.Scale.MaxLimitedFiles
	if limit > len(candidates) {
		limit = len(candidates)
	}
	files := make([]string, 0, limit)
	for _, candidate := range candidates[:limit] {
		files = append(files, candidate.path)
	}
	sort.Strings(files)
	return files
}

func highSignalScore(rel string) int {
	base := strings.ToLower(filepath.Base(rel))
	relLower := strings.ToLower(filepath.ToSlash(rel))
	switch {
	case base == "dockerfile" || strings.HasPrefix(base, "dockerfile."):
		return 100
	case strings.HasPrefix(base, "docker-compose") && (strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml")):
		return 100
	case base == "kustomization.yaml" || base == "kustomization.yml" || base == "chart.yaml" || base == "values.yaml":
		return 95
	case strings.Contains(relLower, ".github/workflows/") || strings.Contains(relLower, "/.github/workflows/"):
		return 90
	case base == "codeowners":
		return 90
	case base == "package.json" || base == "package-lock.json" || base == "pnpm-lock.yaml" || base == "yarn.lock":
		return 85
	case base == "go.mod" || base == "go.sum" || base == "pom.xml" || base == "build.gradle" || base == "settings.gradle":
		return 85
	case base == "requirements.txt" || base == "requirements.in" || base == "pyproject.toml" || base == "poetry.lock" || base == "cargo.toml":
		return 85
	case strings.HasSuffix(base, ".tf") || strings.HasSuffix(base, ".proto"):
		return 80
	case strings.HasPrefix(base, "readme"):
		return 70
	case strings.HasSuffix(base, ".yaml") || strings.HasSuffix(base, ".yml"):
		if strings.Contains(relLower, "k8s") || strings.Contains(relLower, "kubernetes") || strings.Contains(relLower, "deploy") || strings.Contains(relLower, "manifest") {
			return 88
		}
		return 40
	case base == "main.go" || base == "server.go" || base == "app.ts" || base == "app.tsx" || base == "index.ts" || base == "index.tsx":
		return 60
	default:
		return 0
	}
}
