// Package link implements `tld link`, the sibling command of the ARC205 source
// grounding rule. It links a workspace element to an explicit source target
// (file, folder, or path#symbol) or to an external resource, then reports the
// updated grounding score. `--next` optionally suggests unlinked elements.
package link

import (
	"fmt"
	"net/url"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/cmd/update"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/repolink"
	"github.com/mertcikla/tld/v2/internal/sourcelink"
	"github.com/mertcikla/tld/v2/internal/term"
	archwarnings "github.com/mertcikla/tld/v2/internal/warnings"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
	"github.com/spf13/cobra"
)

type linkOptions struct {
	next     bool
	unlink   bool
	ignore   bool
	unignore bool
	external bool
	file     string
	symbol   string
	line     int
	nodeType string
	repo     string
	branch   string
	dryRun   bool
	dataDir  string
	format   string
	compact  bool
}

// NewLinkCmd builds the link command.
func NewLinkCmd(wdir, format *string, compact *bool) *cobra.Command {
	var opts linkOptions

	c := &cobra.Command{
		Use:   "link <element-ref> [target]",
		Short: "Link an element to source (file/symbol) or an external resource",
		Long: `Link a workspace element to a source file, folder, or symbol anchor, or to an
external resource.

A positional target is interpreted as a path#symbol anchor, a file path, a
folder (trailing slash), or a URL. Use --repo for a repository-level link and
--external to force an external (documented) link.

Use --next (without an element ref) to list up to 5 unlinked elements sorted by
view level as suggestions.

Use --ignore to ignore, and --unignore to restore it.

  tld link svc internal/api.go#function:Handle
  tld link svc internal/api.go --line 42
  tld link svc --file internal/api.go --symbol Handle
  tld link svc --repo acme/app
  tld link svc --external https://status.acme.com
  tld link --next                       # suggest up to 5 unlinked elements
  tld link svc --unlink                 # clear the link
  tld link svc --ignore                 # exempt an unlinked element from grounding
  tld link svc --unignore               # restore grounding for an ignored element`,
		Args: cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != nil {
				opts.format = *format
			}
			if compact != nil {
				opts.compact = *compact
			}
			return run(cmd, wdir, opts, args)
		},
	}

	c.Flags().BoolVar(&opts.next, "next", false, "show up to 5 unlinked elements, sorted by view level")
	c.Flags().BoolVar(&opts.unlink, "unlink", false, "clear the element's source link")
	c.Flags().BoolVar(&opts.ignore, "ignore", false, "exempt an unlinked element from the grounding score")
	c.Flags().BoolVar(&opts.unignore, "unignore", false, "restore grounding for an ignored element")
	c.Flags().BoolVar(&opts.external, "external", false, "link externally (URL or unindexed repository)")
	c.Flags().StringVar(&opts.file, "file", "", "file or folder path within a repository")
	c.Flags().StringVar(&opts.symbol, "symbol", "", "declaration name to anchor within --file")
	c.Flags().IntVar(&opts.line, "line", 0, "line number anchor")
	c.Flags().StringVar(&opts.nodeType, "node-type", "", "symbol node type for the anchor (default derived)")
	c.Flags().StringVar(&opts.repo, "repo", "", "repository remote URL or owner/name")
	c.Flags().StringVar(&opts.branch, "branch", "", "branch to record with the link")
	c.Flags().BoolVar(&opts.dryRun, "dry-run", false, "preview the change without writing files")
	c.Flags().StringVar(&opts.dataDir, "data-dir", "", "data directory for local target state")
	return c
}

func run(cmd *cobra.Command, wdir *string, opts linkOptions, args []string) error {
	if opts.next {
		if len(args) != 0 {
			return fail(cmd, opts, fmt.Errorf("--next does not take an element ref"))
		}
		if opts.unlink || opts.ignore || opts.unignore || opts.external {
			return fail(cmd, opts, fmt.Errorf("--next cannot be combined with --unlink, --ignore, --unignore, or --external"))
		}
	}
	if opts.ignore && opts.unignore {
		return fail(cmd, opts, fmt.Errorf("--ignore and --unignore are mutually exclusive"))
	}
	if opts.unlink && (opts.ignore || opts.unignore) {
		return fail(cmd, opts, fmt.Errorf("--unlink cannot be combined with --ignore or --unignore"))
	}
	if (opts.ignore || opts.unignore) && (len(args) > 1 || opts.external || opts.repo != "" || opts.file != "" || opts.symbol != "" || opts.line != 0) {
		return fail(cmd, opts, fmt.Errorf("--ignore and --unignore cannot be combined with a link target"))
	}
	markerMode := opts.unlink || opts.ignore || opts.unignore
	if markerMode && len(args) != 1 {
		return fail(cmd, opts, fmt.Errorf("--unlink, --ignore, and --unignore require exactly one element ref"))
	}
	if !opts.next && !markerMode && len(args) == 0 {
		return fail(cmd, opts, fmt.Errorf("an element ref is required (or use --next)"))
	}
	if len(args) > 2 {
		return fail(cmd, opts, fmt.Errorf("too many arguments"))
	}

	sess, err := cmdutil.OpenSession(cmd, *wdir, "", opts.dataDir)
	if err != nil {
		return fail(cmd, opts, err)
	}
	defer func() { _ = sess.Close() }()

	ws, err := sess.LoadWorkspace()
	if err != nil {
		return fail(cmd, opts, fmt.Errorf("load workspace: %w", err))
	}

	if opts.next {
		return runNext(cmd, ws, opts)
	}

	ref, err := cmdutil.ResolveElementArg(ws, args[0])
	if err != nil {
		return fail(cmd, opts, err)
	}
	element := ws.Elements[ref]
	if element == nil {
		return fail(cmd, opts, fmt.Errorf("element %q not found", ref))
	}

	if opts.unlink {
		if err := applyUnlink(cmd, sess, ws, ref, element, opts.dryRun); err != nil {
			return fail(cmd, opts, err)
		}
		if !opts.dryRun {
			ws, _ = sess.Reload()
		}
		return reportResult(cmd, ws, ref, nil, opts, "unlinked")
	}

	if opts.ignore || opts.unignore {
		return applyIgnore(cmd, sess, ws, ref, element, opts)
	}

	target := ""
	if len(args) == 2 {
		target = args[1]
	}
	res, err := resolve(target, opts)
	if err != nil {
		return fail(cmd, opts, err)
	}
	if err := validateResolved(res); err != nil {
		return fail(cmd, opts, err)
	}

	if opts.dryRun {
		term.Successf(cmd.OutOrStdout(), "dry-run: link %s → %s", ref, res.display)
		return nil
	}

	for _, field := range res.elementFields(element.Tags) {
		if strings.TrimSpace(field.value) == "" {
			continue
		}
		if err := update.ApplyElementFieldUpdate(cmd, sess, ws, ref, field.name, field.value); err != nil {
			return fail(cmd, opts, fmt.Errorf("write link: %w", err))
		}
	}
	updated, err := sess.Reload()
	if err != nil {
		return fail(cmd, opts, fmt.Errorf("reload workspace: %w", err))
	}
	return reportResult(cmd, updated, ref, &res, opts, "linked")
}

// applyIgnore toggles the reserved grounding-ignore tag on an element. Ignoring
// exempts an unlinked element from the ARC205 score without marking it external.
func applyIgnore(cmd *cobra.Command, sess *cmdutil.Session, ws *workspace.Workspace, ref string, element *workspace.Element, opts linkOptions) error {
	action := "ignored"
	var tags []string
	if opts.ignore {
		tags = appendTag(element.Tags, archwarnings.GroundingIgnoreTag)
	} else {
		action = "unignored"
		tags = removeTag(element.Tags, archwarnings.GroundingIgnoreTag)
	}

	if opts.dryRun {
		term.Successf(cmd.OutOrStdout(), "dry-run: %s %s", action, ref)
		return nil
	}

	if err := update.ApplyElementFieldUpdate(cmd, sess, ws, ref, "tags", strings.Join(tags, ", ")); err != nil {
		return fail(cmd, opts, fmt.Errorf("write ignore: %w", err))
	}
	updated, err := sess.Reload()
	if err != nil {
		return fail(cmd, opts, fmt.Errorf("reload workspace: %w", err))
	}
	return reportResult(cmd, updated, ref, nil, opts, action)
}

func fail(cmd *cobra.Command, opts linkOptions, err error) error {
	if cmdutil.WantsJSON(opts.format) {
		_ = cmdutil.WriteCommandError(cmd.OutOrStdout(), opts.compact, "link", err)
	}
	return err
}

func reportResult(cmd *cobra.Command, ws *workspace.Workspace, ref string, res *resolvedLink, opts linkOptions, action string) error {
	report := archwarnings.Grounding(ws)

	if cmdutil.WantsJSON(opts.format) {
		extra := map[string]any{
			"action":   action,
			"score":    report.Value,
			"grounded": report.Grounded,
			"eligible": report.Eligible,
			"external": report.External,
			"ignored":  report.Ignored,
			"views":    report.Views,
		}
		if res != nil {
			extra["link"] = res.jsonMap()
		}
		return cmdutil.WriteJSON(cmd.OutOrStdout(), opts.compact, cmdutil.JSONOutput{
			Command: "link",
			Status:  "ok",
			Items:   []cmdutil.JSONItem{{Ref: ref, Action: action, Name: displayName(ws, ref)}},
			Extra:   extra,
		})
	}

	out := cmd.OutOrStdout()
	switch action {
	case "unlinked":
		term.Successf(out, "Unlinked %q", ref)
	case "ignored":
		term.Successf(out, "Ignored %q (exempt from grounding)", ref)
	case "unignored":
		term.Successf(out, "Unignored %q", ref)
	default:
		if res != nil {
			term.Successf(out, "Linked %q → %s", ref, res.display)
			for _, note := range res.notes {
				term.Warnf(out, "  %s", note)
			}
		}
	}
	return nil
}

// nextSuggestionLimit caps how many unlinked elements `--next` reports.
const nextSuggestionLimit = 5

// runNext reports up to nextSuggestionLimit unlinked elements, shallowest view
// level first, as optional suggestions. It never mutates anything.
func runNext(cmd *cobra.Command, ws *workspace.Workspace, opts linkOptions) error {
	_, details := archwarnings.GroundingDetails(ws)
	next := ungroundedElements(details, nextSuggestionLimit)

	if cmdutil.WantsJSON(opts.format) {
		return cmdutil.WriteJSON(cmd.OutOrStdout(), opts.compact, cmdutil.JSONOutput{
			Command: "link",
			Status:  "ok",
			Extra:   map[string]any{"next": next},
		})
	}

	out := cmd.OutOrStdout()
	if len(next) == 0 {
		term.Successf(out, "All linkable elements are grounded.")
		return nil
	}
	_, _ = fmt.Fprintf(out, "Next (%d) unlinked elements, run tld validate ARC205 for full list\n", len(next))
	for _, element := range next {
		_, _ = fmt.Fprintf(out, "  - %s\n", suggestionLine(element))
	}
	return nil
}

// ungroundedElements returns the first limit unlinked elements from details,
// which GroundingDetails already orders by view depth (shallowest first).
func ungroundedElements(details []archwarnings.GroundingElement, limit int) []archwarnings.GroundingElement {
	out := make([]archwarnings.GroundingElement, 0, limit)
	for _, detail := range details {
		if detail.Grounded {
			continue
		}
		out = append(out, detail)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func suggestionLine(element archwarnings.GroundingElement) string {
	view := ""
	if len(element.Views) > 0 {
		view = element.Views[0]
	}
	label := element.Name
	if strings.TrimSpace(label) == "" {
		label = element.Ref
	}
	return fmt.Sprintf("%q (ref %s, kind %s, depth %d, view %s)", label, element.Ref, element.Kind, element.Depth, view)
}

func displayName(ws *workspace.Workspace, ref string) string {
	if element := ws.Elements[ref]; element != nil {
		return element.Name
	}
	return ref
}

func validateResolved(res resolvedLink) error {
	if res.filePath == "" && res.repo == "" && res.url == "" {
		return fmt.Errorf("could not resolve a link target")
	}
	return nil
}

type resolvedLink struct {
	display  string
	filePath string
	symbol   string
	repo     string
	branch   string
	url      string
	external bool
	notes    []string
}

func (r resolvedLink) jsonMap() map[string]any {
	out := map[string]any{
		"display":   r.display,
		"file_path": r.filePath,
		"repo":      r.repo,
		"branch":    r.branch,
		"url":       r.url,
		"external":  r.external,
	}
	if r.symbol != "" {
		out["symbol"] = r.symbol
	}
	if len(r.notes) > 0 {
		out["notes"] = r.notes
	}
	return out
}

func isURL(target string) bool {
	lower := strings.ToLower(target)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// resolve maps a target and its selectors to a concrete link. Targets are
// explicit: a path#symbol anchor, a file/folder path, a URL, --repo, or
// --external.
func resolve(target string, opts linkOptions) (resolvedLink, error) {
	if opts.external {
		return externalTarget(target, opts)
	}
	if opts.file != "" || opts.symbol != "" {
		return resolveExplicit(opts)
	}
	if opts.repo != "" {
		return resolveRepoArg(opts.repo, opts)
	}

	target = strings.TrimSpace(target)
	if target == "" {
		return resolvedLink{}, fmt.Errorf("a target is required (path#symbol, path, --repo, or --external <url>)")
	}
	if isURL(target) {
		return externalURL(target)
	}
	parsed := sourcelink.Parse(target)
	switch parsed.Anchor.Kind {
	case sourcelink.AnchorSymbol:
		return resolveFileSymbol(parsed.BasePath, parsed.Anchor.Symbol, parsed.Anchor.NodeType, opts)
	case sourcelink.AnchorLine:
		return resolveFileLine(parsed.BasePath, parsed.Anchor.StartLine, opts)
	}
	if strings.HasSuffix(target, "/") {
		return resolveFolder(target, opts), nil
	}
	return resolveFilePath(target, opts), nil
}

func resolveExplicit(opts linkOptions) (resolvedLink, error) {
	file := strings.TrimSpace(opts.file)
	symbol := strings.TrimSpace(opts.symbol)
	if file == "" {
		return resolvedLink{}, fmt.Errorf("--symbol requires --file (a bare symbol cannot be resolved without a codeindex)")
	}
	if strings.HasSuffix(file, "/") {
		return resolveFolder(file, opts), nil
	}
	res := resolveFilePath(file, opts)
	if symbol != "" {
		res.symbol = symbol
		res.filePath = sourcelink.FormatSymbol(file, nodeTypeOr(opts.nodeType, ""), symbol)
		res.display = res.filePath
		res.notes = nil
	}
	return res, nil
}

func resolveFilePath(path string, opts linkOptions) resolvedLink {
	path = strings.TrimSpace(path)
	res := resolvedLink{filePath: path, display: path, branch: strings.TrimSpace(opts.branch)}
	if opts.line > 0 {
		res.filePath = sourcelink.FormatLine(path, opts.line)
		res.display = res.filePath
	}
	return res
}

func resolveFolder(folder string, opts linkOptions) resolvedLink {
	folder = ensureTrailingSlash(folder)
	return resolvedLink{
		filePath: folder,
		display:  folder,
		branch:   strings.TrimSpace(opts.branch),
		notes:    []string{"linked at folder granularity"},
	}
}

func resolveFileSymbol(base, symbol, nodeType string, opts linkOptions) (resolvedLink, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return resolvedLink{}, fmt.Errorf("a file path is required before the symbol anchor")
	}
	res := resolvedLink{
		symbol:   symbol,
		branch:   strings.TrimSpace(opts.branch),
		filePath: sourcelink.FormatSymbol(base, nodeTypeOr(opts.nodeType, nodeType), symbol),
	}
	res.display = res.filePath
	return res, nil
}

func resolveFileLine(base string, line int, opts linkOptions) (resolvedLink, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return resolvedLink{}, fmt.Errorf("a file path is required before the line anchor")
	}
	res := resolvedLink{
		branch:   strings.TrimSpace(opts.branch),
		filePath: sourcelink.FormatLine(base, line),
	}
	res.display = res.filePath
	return res, nil
}

func resolveRepoArg(repo string, opts linkOptions) (resolvedLink, error) {
	slug := strings.TrimSpace(repo)
	if normalized, ok := repolink.NormalizeRemote(repo); ok {
		slug = repoSlug(normalized)
	}
	if slug == "" {
		return resolvedLink{}, fmt.Errorf("invalid repository %q", repo)
	}
	return resolvedLink{
		repo:    slug,
		display: slug,
		branch:  strings.TrimSpace(opts.branch),
		notes:   []string{"repository-level link has no file_path; ARC205 will not count it as grounded"},
	}, nil
}

func externalTarget(target string, opts linkOptions) (resolvedLink, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		target = strings.TrimSpace(opts.repo)
	}
	if target == "" {
		return resolvedLink{}, fmt.Errorf("--external requires a URL or repository")
	}
	if isURL(target) {
		return externalURL(target)
	}
	return externalRepo(target)
}

func externalURL(target string) (resolvedLink, error) {
	if !isURL(target) {
		return resolvedLink{}, fmt.Errorf("invalid external URL %q", target)
	}
	return resolvedLink{url: strings.TrimSpace(target), display: strings.TrimSpace(target), external: true, notes: []string{"linked externally (documented)"}}, nil
}

func externalRepo(target string) (resolvedLink, error) {
	slug := strings.TrimSpace(target)
	if normalized, ok := repolink.NormalizeRemote(target); ok {
		slug = repoSlug(normalized)
	}
	if slug == "" {
		return resolvedLink{}, fmt.Errorf("invalid external repository %q", target)
	}
	return resolvedLink{repo: slug, display: slug, external: true, notes: []string{"linked externally (documented)"}}, nil
}

// repoSlug renders a remote URL the way the UI stores it: owner/name for
// GitHub, otherwise host/path.
func repoSlug(remote string) string {
	normalized, ok := repolink.NormalizeRemote(remote)
	if !ok {
		return strings.TrimSpace(remote)
	}
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Host == "" {
		return normalized
	}
	slug := strings.Trim(strings.TrimSuffix(parsed.Path, "/"), "/")
	if parsed.Host == "github.com" || parsed.Host == "www.github.com" {
		return slug
	}
	return parsed.Host + "/" + slug
}

// elementFields lists the element field updates a resolved link maps to, in the
// order they are applied. Empty fields are skipped by the caller.
func (r resolvedLink) elementFields(currentTags []string) []struct{ name, value string } {
	fields := []struct{ name, value string }{
		{"file_path", r.filePath},
		{"repo", r.repo},
		{"branch", r.branch},
		{"url", r.url},
	}
	if r.external {
		fields = append(fields, struct{ name, value string }{
			"tags", strings.Join(appendTag(currentTags, "external"), ", "),
		})
	}
	return fields
}

func linkElementInput(e *diagv1.Element) api.ElementInput {
	bypass := e.GetBypassNoiseGate()
	return api.ElementInput{
		Name:            e.GetName(),
		Description:     optionalString(e.Description),
		Kind:            optionalString(e.Kind),
		Technology:      optionalString(e.Technology),
		URL:             optionalString(e.Url),
		LogoURL:         optionalString(e.LogoUrl),
		TechLinks:       e.TechnologyLinks,
		Tags:            e.Tags,
		Repo:            optionalString(e.Repo),
		RepositoryID:    optionalString(e.RepositoryId),
		Branch:          optionalString(e.Branch),
		Language:        optionalString(e.Language),
		FilePath:        optionalString(e.FilePath),
		BypassNoiseGate: &bypass,
		HasView:         e.GetHasView(),
		ViewLabel:       optionalString(e.ViewLabel),
	}
}

func optionalString(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

func applyUnlink(cmd *cobra.Command, sess *cmdutil.Session, ws *workspace.Workspace, ref string, element *workspace.Element, dryRun bool) error {
	if dryRun {
		if sess.HasWorkspace() {
			return cmdutil.WithWorkspaceDryRun(sess.Wdir, func(cloneDir string) error {
				return unlinkFields(cloneDir, ref, element)
			})
		}
		return nil
	}
	ctx := sess.Context(cmd.Context())
	runner, err := sess.Runner()
	if err != nil {
		return err
	}
	elementID, err := exec.EnsureElementID(ctx, runner, ws, sess.Wdir, ref)
	if err != nil {
		return err
	}
	existing, err := runner.GetElement(ctx, elementID)
	if err != nil {
		return cmdutil.WithUnauthorizedHint("read element failed", err)
	}
	input := linkElementInput(existing)
	empty := ""
	// Empty strings (not nil) clear the columns; nil means "leave unchanged".
	input.FilePath = &empty
	input.RepositoryID = &empty
	input.Repo = &empty
	input.Branch = &empty
	input.Tags = removeTag(existing.GetTags(), "external")
	if input.Tags == nil {
		input.Tags = []string{}
	}
	updated, err := runner.UpdateElement(ctx, elementID, input)
	if err != nil {
		return cmdutil.WithUnauthorizedHint("unlink element failed", err)
	}
	// Refresh the YAML cache (write-through) when a workspace is in play.
	if sess.HasWorkspace() {
		if err := unlinkFields(sess.Wdir, ref, element); err != nil {
			return err
		}
		if err := exec.RecordElementMeta(ctx, sess.Wdir, ref, updated, 0, nil); err != nil {
			return fmt.Errorf("update cache metadata: %w", err)
		}

		if err := workspace.TouchCurrentElementMetadata(sess.Wdir, ref); err != nil {
			return fmt.Errorf("refresh cache metadata: %w", err)
		}
	}
	return nil
}

func unlinkFields(dir, ref string, element *workspace.Element) error {
	for _, field := range []string{"file_path", "repository_id", "repo", "branch"} {
		_ = workspace.RemoveElementField(dir, ref, field)
	}
	tags := removeTag(element.Tags, "external")
	if err := workspace.UpdateElementField(dir, ref, "tags", strings.Join(tags, ", ")); err != nil {
		return err
	}
	return nil
}

func nodeTypeOr(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if strings.TrimSpace(fallback) != "" {
		return strings.TrimSpace(fallback)
	}
	return "symbol"
}

func ensureTrailingSlash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasSuffix(value, "/") {
		return value
	}
	return value + "/"
}

func appendTag(tags []string, tag string) []string {
	for _, existing := range tags {
		if strings.EqualFold(strings.TrimSpace(existing), tag) {
			return tags
		}
	}
	return append(append([]string{}, tags...), tag)
}

func removeTag(tags []string, tag string) []string {
	out := make([]string, 0, len(tags))
	for _, existing := range tags {
		if strings.EqualFold(strings.TrimSpace(existing), tag) {
			continue
		}
		out = append(out, existing)
	}
	return out
}
