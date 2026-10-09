// Package link implements `tld link`, the sibling command of the ARC205 source
// grounding rule. It links a workspace element to a codeindex primitive (file,
// folder, or symbol) or to an external resource, then reports the updated
// grounding score and the next element that still needs a link.
package link

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/codeindex/mappingcheck"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/codeindex/suggest"
	"github.com/mertcikla/tld/v2/internal/completion"
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
	external bool
	file     string
	symbol   string
	line     int
	nodeType string
	repo     string
	branch   string
	view     string
	limit    int
	noVerify bool
	quiet    bool
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
		Long: `Link a workspace element to a codeindex primitive or an external resource.

A target is resolved in preference order: an explicit anchor (path#symbol), a
file or folder path, a bare symbol name, an indexed repository, then an external
URL or unindexed repository. Use --external to force an external link.

After linking, the updated ARC205 source grounding score and the next element
that still needs a link (ordered by view depth) are shown, so you can iterate
without re-running 'tld validate'.

  tld link svc internal/api.go#function:Handle
  tld link svc --symbol HandleCheckout --repo acme/app
  tld link svc                          # show candidate targets
  tld link --next                       # show the next element to link
  tld link svc --unlink                 # clear the link
  tld link svc --external https://status.acme.com`,
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

	c.Flags().BoolVar(&opts.next, "next", false, "show the next ungrounded element instead of linking")
	c.Flags().BoolVar(&opts.unlink, "unlink", false, "clear the element's source link")
	c.Flags().BoolVar(&opts.external, "external", false, "link externally (unindexed repo / URL / cloud resource)")
	c.Flags().StringVar(&opts.file, "file", "", "file or folder path within a repository")
	c.Flags().StringVar(&opts.symbol, "symbol", "", "declaration name to link to")
	c.Flags().IntVar(&opts.line, "line", 0, "line number anchor")
	c.Flags().StringVar(&opts.nodeType, "node-type", "", "symbol node type for the anchor (default derived)")
	c.Flags().StringVar(&opts.repo, "repo", "", "repository remote URL, owner/name, or codeindex id")
	c.Flags().StringVar(&opts.branch, "branch", "", "branch to record with the link")
	c.Flags().StringVar(&opts.view, "view", "", "limit --next to a view")
	c.Flags().IntVar(&opts.limit, "limit", 5, "maximum candidates shown")
	c.Flags().BoolVar(&opts.noVerify, "no-verify", false, "skip verification against the codeindex")
	c.Flags().BoolVar(&opts.quiet, "quiet", false, "only print the link result")
	c.Flags().BoolVar(&opts.dryRun, "dry-run", false, "preview the change without writing files")
	c.Flags().StringVar(&opts.dataDir, "data-dir", "", "data directory for the local codeindex database")

	_ = c.RegisterFlagCompletionFunc("view", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ElementRefs(wdir)
	})
	_ = c.RegisterFlagCompletionFunc("repo", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	})
	return c
}

func run(cmd *cobra.Command, wdir *string, opts linkOptions, args []string) error {
	ctx := cmd.Context()
	if opts.next && len(args) != 0 {
		return fail(cmd, opts, fmt.Errorf("--next does not take an element ref"))
	}
	if opts.unlink && len(args) != 1 {
		return fail(cmd, opts, fmt.Errorf("--unlink requires exactly one element ref"))
	}
	if !opts.next && len(args) == 0 {
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

	dataDir, err := workspace.ResolveDataDir(&ws.Config, opts.dataDir)
	if err != nil {
		dataDir = ""
	}

	var idx *cstore.Store
	closeStore := func() {}
	if opened, closer, ok := mappingcheck.OpenStore(ctx, dataDir); ok {
		idx, closeStore = opened, closer
	}
	defer closeStore()

	var sug *suggest.Suggester
	if idx != nil {
		sug = suggest.New(idx)
	}
	classify := codeindexClassifier(ctx, idx)
	groundingOpts := groundingOptions(classify)

	if opts.next {
		return runNext(cmd, ws, nil, sug, groundingOpts, opts)
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
		return reportResult(cmd, ws, ref, nil, sug, groundingOpts, opts, "unlinked")
	}

	// One positional argument and no target selectors: list candidates.
	if len(args) == 1 && !hasTarget(opts, "") {
		return runCandidates(cmd, ref, element, sug, opts)
	}

	target := ""
	if len(args) == 2 {
		target = args[1]
	}
	res, err := resolve(ctx, sug, element, target, opts)
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

	owner := ownerFor(ws, res.repositoryID)
	res.owner = owner
	writeCtx := sess.Context(ctx)
	runner, err := sess.Runner()
	if err != nil {
		return fail(cmd, opts, err)
	}
	elementID, err := exec.EnsureElementID(writeCtx, runner, ws, sess.Wdir, ref)
	if err != nil {
		return fail(cmd, opts, err)
	}
	updatedElement, err := applyLinkDB(writeCtx, runner, elementID, element.Tags, res)
	if err != nil {
		return fail(cmd, opts, fmt.Errorf("write link: %w", err))
	}
	// Refresh the YAML cache (write-through) when a workspace is in play.
	if sess.HasWorkspace() {
		if err := applyLink(sess.Wdir, ref, element.Tags, res); err != nil {
			return fail(cmd, opts, fmt.Errorf("write link: %w", err))
		}
		if err := exec.RecordElementMeta(writeCtx, sess.Wdir, ref, updatedElement, 0, nil); err != nil {
			return fail(cmd, opts, fmt.Errorf("update cache metadata: %w", err))
		}
	}
	updated, err := sess.Reload()
	if err != nil {
		return fail(cmd, opts, fmt.Errorf("reload workspace: %w", err))
	}
	return reportResult(cmd, updated, ref, &res, sug, groundingOpts, opts, "linked")
}

func hasTarget(opts linkOptions, positional string) bool {
	return positional != "" || opts.file != "" || opts.symbol != "" || opts.repo != "" ||
		opts.external || opts.line > 0
}

func groundingOptions(classify func(*workspace.Element) bool) []archwarnings.Option {
	if classify == nil {
		return nil
	}
	return []archwarnings.Option{archwarnings.WithCodeindexElementClassifier(classify)}
}

// codeindexClassifier builds the codeindex-owned predicate from an already open
// store, avoiding a second database handle.
func codeindexClassifier(ctx context.Context, idx *cstore.Store) func(*workspace.Element) bool {
	if idx == nil {
		return nil
	}
	sources, err := idx.MappedElementIndex(ctx)
	if err != nil || (len(sources.Sources) == 0 && len(sources.Names) == 0) {
		return nil
	}
	return func(element *workspace.Element) bool {
		if element == nil {
			return false
		}
		if _, ok := sources.Sources[cstore.ElementSourceKey(element.RepositoryID, element.FilePath)]; ok {
			return true
		}
		_, ok := sources.Names[cstore.ElementNameKey(element.Kind, element.Name)]
		return ok
	}
}

func fail(cmd *cobra.Command, opts linkOptions, err error) error {
	if cmdutil.WantsJSON(opts.format) {
		_ = cmdutil.WriteCommandError(cmd.OutOrStdout(), opts.compact, "link", err)
	}
	return err
}

func subjectFor(element *workspace.Element) suggest.Subject {
	return suggest.Subject{
		Ref:          element.Name,
		Name:         element.Name,
		Kind:         element.Kind,
		Technology:   element.Technology,
		RepositoryID: element.RepositoryID,
		Repo:         element.Repo,
		FilePath:     element.FilePath,
	}
}

func runCandidates(cmd *cobra.Command, ref string, element *workspace.Element, sug *suggest.Suggester, opts linkOptions) error {
	if sug == nil {
		return fail(cmd, opts, fmt.Errorf("no local codeindex available; run 'tld index' or use --external"))
	}
	candidates, err := sug.ForSubject(cmd.Context(), subjectFor(element), suggest.Options{Limit: opts.limit})
	if err != nil {
		return fail(cmd, opts, err)
	}
	if cmdutil.WantsJSON(opts.format) {
		return writeCandidatesJSON(cmd, opts, ref, candidates)
	}
	printCandidates(cmd.OutOrStdout(), ref, candidates, opts.dataDir)
	return nil
}

func runNext(cmd *cobra.Command, ws *workspace.Workspace, res *resolvedLink, sug *suggest.Suggester, gopts []archwarnings.Option, opts linkOptions) error {
	_, details := archwarnings.GroundingDetails(ws, gopts...)
	next := nextElement(details, opts.view)
	if next == nil {
		if cmdutil.WantsJSON(opts.format) {
			return cmdutil.WriteJSON(cmd.OutOrStdout(), opts.compact, cmdutil.JSONOutput{Command: "link", Status: "ok", Extra: map[string]any{"next": nil}})
		}
		term.Successf(cmd.OutOrStdout(), "All linkable elements are grounded.")
		return nil
	}
	return reportNext(cmd, ws, next, res, sug, opts)
}

func reportResult(cmd *cobra.Command, ws *workspace.Workspace, ref string, res *resolvedLink, sug *suggest.Suggester, gopts []archwarnings.Option, opts linkOptions, action string) error {
	report, details := archwarnings.GroundingDetails(ws, gopts...)
	next := nextElement(details, opts.view)

	if cmdutil.WantsJSON(opts.format) {
		extra := map[string]any{
			"action":   action,
			"score":    report.Value,
			"grounded": report.Grounded,
			"eligible": report.Eligible,
			"views":    report.Views,
		}
		if res != nil {
			extra["link"] = res.jsonMap()
		}
		if next != nil {
			extra["next"] = next
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
	default:
		if res != nil {
			term.Successf(out, "Linked %q → %s", ref, res.display)
			for _, note := range res.notes {
				term.Warnf(out, "  %s", note)
			}
		}
	}
	_, _ = fmt.Fprintf(out, "Grounding: workspace %d/10 (%d/%d)\n", report.Value, report.Grounded, report.Eligible)
	if opts.quiet {
		return nil
	}
	if next == nil {
		_, _ = fmt.Fprintf(out, "All linkable elements are grounded.\n")
		return nil
	}
	printNext(out, next)
	if sug != nil {
		element := ws.Elements[next.Ref]
		candidates, err := sug.ForSubject(cmd.Context(), subjectFor(element), suggest.Options{Limit: opts.limit})
		if err == nil {
			printCandidates(out, next.Ref, candidates, opts.dataDir)
		}
	}
	return nil
}

func reportNext(cmd *cobra.Command, ws *workspace.Workspace, next *archwarnings.GroundingElement, res *resolvedLink, sug *suggest.Suggester, opts linkOptions) error {
	var candidates []suggest.Candidate
	if sug != nil {
		if element := ws.Elements[next.Ref]; element != nil {
			candidates, _ = sug.ForSubject(cmd.Context(), subjectFor(element), suggest.Options{Limit: opts.limit})
		}
	}
	if cmdutil.WantsJSON(opts.format) {
		return cmdutil.WriteJSON(cmd.OutOrStdout(), opts.compact, cmdutil.JSONOutput{
			Command: "link",
			Status:  "ok",
			Extra:   map[string]any{"next": next, "candidates": candidates},
		})
	}
	printNext(cmd.OutOrStdout(), next)
	printCandidates(cmd.OutOrStdout(), next.Ref, candidates, opts.dataDir)
	return nil
}

func printNext(out io.Writer, next *archwarnings.GroundingElement) {
	view := ""
	if len(next.Views) > 0 {
		view = next.Views[0]
	}
	label := next.Name
	if strings.TrimSpace(label) == "" {
		label = next.Ref
	}
	_, _ = fmt.Fprintf(out, "\nNext: %q (ref %s, kind %s, depth %d, view %s)\n", label, next.Ref, next.Kind, next.Depth, view)
}

func printCandidates(out io.Writer, ref string, candidates []suggest.Candidate, dataDir string) {
	if len(candidates) == 0 {
		_, _ = fmt.Fprintf(out, "  no codeindex candidates; try: tld link %s --external <url>\n", ref)
		return
	}
	for _, candidate := range candidates {
		_, _ = fmt.Fprintf(out, "  %s\n", linkCommand(ref, candidate, dataDir))
	}
}

// linkCommand renders a runnable `tld link` invocation for a candidate.
func linkCommand(ref string, candidate suggest.Candidate, dataDir string) string {
	parts := []string{"tld", "link", ref}
	if candidate.Kind == suggest.KindRepo {
		id := candidate.RepositoryID
		if id == "" {
			id = candidate.RemoteURL
		}
		parts = append(parts, "--repo", shellQuote(id))
	} else {
		parts = append(parts, shellQuote(candidateTarget(candidate)))
	}
	if dataDir != "" {
		parts = append(parts, "--data-dir", shellQuote(dataDir))
	}
	return strings.Join(parts, " ")
}

func candidateTarget(candidate suggest.Candidate) string {
	switch candidate.Kind {
	case suggest.KindSymbol:
		return sourcelink.FormatSymbol(candidate.Path, nodeTypeOr(candidate.NodeType, ""), candidate.Symbol)
	default:
		return candidate.Path
	}
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.ContainsAny(value, " \t\"'\\$`") {
		return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
	}
	return value
}

func candidateLabel(candidate suggest.Candidate) string {
	switch candidate.Kind {
	case suggest.KindSymbol:
		if candidate.RepositoryName != "" {
			return fmt.Sprintf("%s %s#%s", candidate.RepositoryName, candidate.Path, candidate.Symbol)
		}
		return fmt.Sprintf("%s#%s", candidate.Path, candidate.Symbol)
	case suggest.KindRepo:
		return candidate.RepositoryName
	default:
		if candidate.RepositoryName != "" {
			return fmt.Sprintf("%s %s", candidate.RepositoryName, candidate.Path)
		}
		return candidate.Path
	}
}

func writeCandidatesJSON(cmd *cobra.Command, opts linkOptions, ref string, candidates []suggest.Candidate) error {
	return cmdutil.WriteJSON(cmd.OutOrStdout(), opts.compact, cmdutil.JSONOutput{
		Command: "link",
		Status:  "ok",
		Items:   []cmdutil.JSONItem{{Ref: ref, Action: "candidates"}},
		Extra:   map[string]any{"candidates": candidates},
	})
}

func nextElement(details []archwarnings.GroundingElement, view string) *archwarnings.GroundingElement {
	for i := range details {
		detail := details[i]
		if detail.Grounded {
			continue
		}
		if view != "" && !containsString(detail.Views, view) {
			continue
		}
		return &detail
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func displayName(ws *workspace.Workspace, ref string) string {
	if element := ws.Elements[ref]; element != nil {
		return element.Name
	}
	return ref
}

func ownerFor(ws *workspace.Workspace, repositoryID string) string {
	if ws == nil || ws.WorkspaceConfig == nil || repositoryID == "" {
		return ""
	}
	for key, repo := range ws.WorkspaceConfig.Repositories {
		if repo.ID == repositoryID {
			return key
		}
	}
	return ""
}

func validateResolved(res resolvedLink) error {
	if res.filePath == "" && res.repositoryID == "" && res.repo == "" && res.url == "" {
		return fmt.Errorf("could not resolve a link target")
	}
	return nil
}

type resolvedLink struct {
	display      string
	filePath     string
	symbol       string
	repositoryID string
	repo         string
	branch       string
	url          string
	owner        string
	external     bool
	verified     bool
	notes        []string
	candidate    *suggest.Candidate
}

func (r resolvedLink) jsonMap() map[string]any {
	out := map[string]any{
		"display":       r.display,
		"file_path":     r.filePath,
		"repository_id": r.repositoryID,
		"repo":          r.repo,
		"branch":        r.branch,
		"url":           r.url,
		"external":      r.external,
		"verified":      r.verified,
	}
	if r.symbol != "" {
		out["symbol"] = r.symbol
	}
	if len(r.notes) > 0 {
		out["notes"] = r.notes
	}
	return out
}

func repoTargetValue(candidate suggest.Candidate) string {
	if candidate.RemoteURL != "" {
		return repoSlug(candidate.RemoteURL)
	}
	if candidate.Root != "" {
		return candidate.Root
	}
	return candidate.RepositoryName
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

func isURL(target string) bool {
	lower := strings.ToLower(target)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func looksLikePath(target string) bool {
	if strings.Contains(target, "/") {
		return true
	}
	return filepath.Ext(target) != ""
}

func resolve(ctx context.Context, sug *suggest.Suggester, element *workspace.Element, target string, opts linkOptions) (resolvedLink, error) {
	if opts.external {
		return externalTarget(target, opts)
	}
	if opts.file != "" || opts.symbol != "" {
		return resolveExplicit(ctx, sug, opts)
	}
	if opts.repo != "" {
		if res, ok, err := resolveRepo(ctx, sug, opts.repo, opts); err != nil {
			return resolvedLink{}, err
		} else if ok {
			return res, nil
		}
		return externalRepo(opts.repo)
	}

	target = strings.TrimSpace(target)
	if target == "" {
		return resolvedLink{}, fmt.Errorf("a target is required")
	}
	parsed := sourcelink.Parse(target)
	if parsed.Anchor.Kind == sourcelink.AnchorSymbol {
		return resolveFileSymbol(ctx, sug, parsed.BasePath, parsed.Anchor.Symbol, parsed.Anchor.NodeType, opts)
	}
	if isURL(target) {
		return externalURL(target)
	}
	if strings.HasSuffix(target, "/") {
		return resolveFolder(ctx, sug, target, opts)
	}
	if looksLikePath(target) {
		if res, ok, err := resolveFilePath(ctx, sug, target, opts); err != nil {
			return resolvedLink{}, err
		} else if ok {
			return res, nil
		}
	}
	if res, ok, err := resolveSymbol(ctx, sug, target, opts); err != nil {
		return resolvedLink{}, err
	} else if ok {
		return res, nil
	}
	if repolink.RemoteKey(target) != "" {
		if res, ok, err := resolveRepo(ctx, sug, target, opts); err != nil {
			return resolvedLink{}, err
		} else if ok {
			return res, nil
		}
		return externalRepo(target)
	}
	if sug == nil {
		return resolvedLink{}, fmt.Errorf("no local codeindex available; run 'tld index' or use --external")
	}
	return resolvedLink{}, fmt.Errorf("no codeindex match for %q; use --external to link it externally", target)
}

func resolveExplicit(ctx context.Context, sug *suggest.Suggester, opts linkOptions) (resolvedLink, error) {
	file := strings.TrimSpace(opts.file)
	symbol := strings.TrimSpace(opts.symbol)
	if file == "" && symbol != "" {
		if res, ok, err := resolveSymbol(ctx, sug, symbol, opts); err != nil {
			return resolvedLink{}, err
		} else if ok {
			return res, nil
		}
		return resolvedLink{}, fmt.Errorf("no declaration named %q in the codeindex", symbol)
	}
	if sug != nil && !opts.noVerify {
		if candidates, err := sug.SearchFiles(ctx, file, suggest.Options{Limit: 1}); err == nil && len(candidates) > 0 {
			return buildFromFile(sug, candidates[0], symbol, opts), nil
		}
	}
	res := resolvedLink{display: file, filePath: file, symbol: symbol}
	if symbol != "" {
		res.filePath = sourcelink.FormatSymbol(file, nodeTypeOr(opts.nodeType, ""), symbol)
	}
	if opts.branch != "" {
		res.branch = opts.branch
	}
	res.notes = append(res.notes, "not found in the codeindex; recorded unverified")
	return res, nil
}

func resolveFilePath(ctx context.Context, sug *suggest.Suggester, path string, opts linkOptions) (resolvedLink, bool, error) {
	if sug == nil {
		return resolvedLink{}, false, nil
	}
	candidates, err := sug.SearchFiles(ctx, path, suggest.Options{Limit: 1})
	if err != nil {
		return resolvedLink{}, false, err
	}
	if len(candidates) == 0 {
		return resolvedLink{}, false, nil
	}
	return buildFromFile(sug, candidates[0], "", opts), true, nil
}

func resolveFolder(ctx context.Context, sug *suggest.Suggester, folder string, opts linkOptions) (resolvedLink, error) {
	if sug == nil {
		return resolvedLink{}, fmt.Errorf("no local codeindex available; run 'tld index' or use --external")
	}
	res := resolvedLink{filePath: ensureTrailingSlash(folder), display: ensureTrailingSlash(folder)}
	candidates, err := sug.SearchFiles(ctx, strings.TrimSuffix(folder, "/"), suggest.Options{Limit: 1})
	if err != nil {
		return resolvedLink{}, err
	}
	if len(candidates) > 0 {
		res.repositoryID = candidates[0].RepositoryID
		res.repo = repoTargetValue(candidates[0])
		res.branch = firstNonEmpty(opts.branch, branchFor(sug, candidates[0].RepositoryID))
		res.candidate = &candidates[0]
		res.notes = append(res.notes, "linked at folder granularity")
	}
	return res, nil
}

func resolveSymbol(ctx context.Context, sug *suggest.Suggester, symbol string, opts linkOptions) (resolvedLink, bool, error) {
	if sug == nil {
		return resolvedLink{}, false, nil
	}
	candidates, err := sug.SearchSymbols(ctx, symbol, suggest.Options{Limit: 1})
	if err != nil {
		return resolvedLink{}, false, err
	}
	if len(candidates) == 0 {
		return resolvedLink{}, false, nil
	}
	candidate := candidates[0]
	res := resolvedLink{
		repositoryID: candidate.RepositoryID,
		repo:         repoTargetValue(candidate),
		branch:       firstNonEmpty(opts.branch, branchFor(sug, candidate.RepositoryID)),
		symbol:       candidate.Symbol,
		verified:     !opts.noVerify,
		candidate:    &candidate,
	}
	res.filePath = sourcelink.FormatSymbol(candidate.Path, nodeTypeOr(opts.nodeType, candidate.NodeType), candidate.Symbol)
	res.display = candidateLabel(candidate)
	return res, true, nil
}

func resolveRepo(ctx context.Context, sug *suggest.Suggester, query string, opts linkOptions) (resolvedLink, bool, error) {
	if sug == nil {
		return resolvedLink{}, false, nil
	}
	candidates, err := sug.SearchRepositories(ctx, query, suggest.Options{Limit: 1})
	if err != nil {
		return resolvedLink{}, false, err
	}
	if len(candidates) == 0 {
		return resolvedLink{}, false, nil
	}
	candidate := candidates[0]
	res := resolvedLink{
		repositoryID: candidate.RepositoryID,
		repo:         repoTargetValue(candidate),
		branch:       firstNonEmpty(opts.branch, branchFor(sug, candidate.RepositoryID)),
		candidate:    &candidate,
		display:      candidateLabel(candidate),
	}
	res.notes = append(res.notes, "repository-level link has no file_path; ARC205 will not count it as grounded")
	return res, true, nil
}

func resolveFileSymbol(ctx context.Context, sug *suggest.Suggester, base, symbol, nodeType string, opts linkOptions) (resolvedLink, error) {
	if sug != nil {
		if candidates, err := sug.SearchFiles(ctx, base, suggest.Options{Limit: 1}); err == nil && len(candidates) > 0 {
			candidate := candidates[0]
			res := resolvedLink{
				repositoryID: candidate.RepositoryID,
				repo:         repoTargetValue(candidate),
				branch:       firstNonEmpty(opts.branch, branchFor(sug, candidate.RepositoryID)),
				symbol:       symbol,
				verified:     !opts.noVerify,
				candidate:    &candidate,
			}
			res.filePath = sourcelink.FormatSymbol(candidate.Path, nodeTypeOr(opts.nodeType, nodeType), symbol)
			res.display = candidateLabel(candidate) + "#" + symbol
			return res, nil
		}
	}
	res := resolvedLink{filePath: sourcelink.FormatSymbol(base, nodeTypeOr(opts.nodeType, nodeType), symbol), symbol: symbol}
	res.display = res.filePath
	res.notes = append(res.notes, "file not found in the codeindex; recorded unverified")
	return res, nil
}

func buildFromFile(sug *suggest.Suggester, candidate suggest.Candidate, symbol string, opts linkOptions) resolvedLink {
	res := resolvedLink{
		repositoryID: candidate.RepositoryID,
		repo:         repoTargetValue(candidate),
		branch:       firstNonEmpty(opts.branch, branchFor(sug, candidate.RepositoryID)),
		candidate:    &candidate,
	}
	switch {
	case symbol != "":
		res.symbol = symbol
		res.filePath = sourcelink.FormatSymbol(candidate.Path, nodeTypeOr(opts.nodeType, ""), symbol)
		res.display = candidateLabel(candidate) + "#" + symbol
	case opts.line > 0:
		res.filePath = sourcelink.FormatLine(candidate.Path, opts.line)
		res.display = candidateLabel(candidate) + "#L" + fmt.Sprint(opts.line)
	default:
		res.filePath = candidate.Path
		res.display = candidateLabel(candidate)
	}
	return res
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

func applyLink(dir, ref string, currentTags []string, res resolvedLink) error {
	fields := []struct{ name, value string }{
		{"file_path", res.filePath},
		{"repository_id", res.repositoryID},
		{"repo", res.repo},
		{"branch", res.branch},
		{"owner", res.owner},
		{"url", res.url},
	}
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			continue
		}
		if err := workspace.UpdateElementField(dir, ref, field.name, field.value); err != nil {
			return err
		}
	}
	if res.external {
		tags := appendTag(currentTags, "external")
		if err := workspace.UpdateElementField(dir, ref, "tags", strings.Join(tags, ", ")); err != nil {
			return err
		}
	}
	return nil
}

// applyLinkDB writes the resolved link to the target element directly and
// returns the updated element.
func applyLinkDB(ctx context.Context, runner exec.Runner, elementID int32, currentTags []string, res resolvedLink) (*diagv1.Element, error) {
	existing, err := runner.GetElement(ctx, elementID)
	if err != nil {
		return nil, cmdutil.WithUnauthorizedHint("read element failed", err)
	}
	input := linkElementInput(existing)
	if res.filePath != "" {
		value := res.filePath
		input.FilePath = &value
	}
	if res.repositoryID != "" {
		value := res.repositoryID
		input.RepositoryID = &value
	}
	if res.repo != "" {
		value := res.repo
		input.Repo = &value
	}
	if res.branch != "" {
		value := res.branch
		input.Branch = &value
	}
	if res.url != "" {
		value := res.url
		input.URL = &value
	}
	if res.external {
		input.Tags = appendTag(currentTags, "external")
	}
	updated, err := runner.UpdateElement(ctx, elementID, input)
	if err != nil {
		return nil, cmdutil.WithUnauthorizedHint("update element failed", err)
	}
	return updated, nil
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

func branchFor(sug *suggest.Suggester, repositoryID string) string {
	if sug == nil || repositoryID == "" {
		return ""
	}
	for _, repo := range sug.Repositories() {
		if repo.GetId() == repositoryID {
			return repo.GetGitBranch()
		}
	}
	return ""
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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
