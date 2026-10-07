package git

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	indexcmd "github.com/mertcikla/tld/v2/cmd/index"
	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	"github.com/mertcikla/tld/v2/internal/codeindex/identity"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
	"github.com/mertcikla/tld/v2/internal/codeindex/remote"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/mermaid"
	localstore "github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
)

type compareOptions struct {
	mermaid        bool
	markdown       bool
	verbose        bool
	radius         uint32
	depth          uint32
	maxNodes       int
	maxBytes       int
	dataDir        string
	maxElements    int
	maxConnectors  int
	reportJSON     string
	prepareCommand string
}

// protoJSONOptions keeps zero-value fields explicit so downstream consumers can
// read distances and counts without knowing proto3 defaults.
var protoJSONOptions = protojson.MarshalOptions{Multiline: true, Indent: "  ", EmitUnpopulated: true}

func newCompareCmd() *cobra.Command {
	opts := compareOptions{}
	c := &cobra.Command{
		Use:   "compare [repository] <base> <head>",
		Short: "Compare two revisions of a repository",
		Long: `Compare two Git revisions and emit the repository impact scene.

The repository may be omitted to use the current checkout, or given as a
repository id, a local path, or a remote URL (github.com/owner/repo,
owner/repo, or a Git URL). Missing snapshots are indexed on demand.

The default output is the protojson encoding of the impact scene: exactly the
payload the canvas loads, and self-contained, so it renders offline without the
repository, its index, or its snapshots. Each placement carries a change overlay
with the file's line counts and structured symbol changes, and the scene names
what it compared. Pass --mermaid to emit the Mermaid change diagram, which is
drawn from the comparison's file-level dependency graph.

--depth controls how many dependency hops of unchanged context are included
(0 = direct changes only). An explicit --radius narrows the displayed scope
without shrinking the computed neighbourhood. When the output exceeds the node
or byte budget it is progressively narrowed by blast radius and a warning is
written to stderr.

Indexing progress is quiet by default so scripted runs only emit the payload
and budget warnings; pass --verbose to follow the base and head scans on
stderr.`,
		Args: cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 3 {
				target = args[0]
			}
			return runCompare(cmd, opts, target, args[len(args)-2], args[len(args)-1])
		},
	}
	c.Flags().BoolVar(&opts.mermaid, "mermaid", false, "emit the Mermaid change diagram instead of the scene")
	c.Flags().BoolVar(&opts.markdown, "markdown", false, "wrap the Mermaid diagram in a Markdown code fence")
	c.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "report indexing progress for both revisions on stderr")
	c.Flags().Uint32Var(&opts.radius, "radius", 0, "blast radius to display; defaults to --depth")
	c.Flags().Uint32Var(&opts.depth, "depth", impact.DefaultContextDepth, "dependency hops of unchanged context to include (0 = direct changes only)")
	c.Flags().IntVar(&opts.maxNodes, "max-nodes", impact.DefaultMaxNodes, "node budget; the blast radius is narrowed when exceeded (0 disables)")
	c.Flags().IntVar(&opts.maxBytes, "max-bytes", 2<<20, "output byte budget; the blast radius is narrowed when exceeded (0 disables)")
	c.Flags().StringVar(&opts.dataDir, "data-dir", "", "override the data directory")
	c.Flags().IntVar(&opts.maxElements, "max-elements", 0, "skip output when the requested diagram exceeds this element count (0 disables)")
	c.Flags().IntVar(&opts.maxConnectors, "max-connectors", 0, "skip output when the requested diagram exceeds this connector count (0 disables)")
	c.Flags().StringVar(&opts.reportJSON, "report-json", "", "write comparison status, counts, revisions, and index warnings to this JSON file")
	c.Flags().StringVar(&opts.prepareCommand, "prepare-command", "", "run this Bash command in each uncached revision before indexing; requires Bash")
	return c
}

func runCompare(cmd *cobra.Command, opts compareOptions, target, base, head string) error {
	if opts.maxElements < 0 || opts.maxConnectors < 0 {
		return fmt.Errorf("--max-elements and --max-connectors must be nonnegative")
	}
	ctx := cmd.Context()
	global, err := workspace.LoadGlobalConfig()
	if err != nil {
		return err
	}
	dataDir, err := workspace.ResolveDataDir(global, opts.dataDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	sq, err := localstore.OpenLocal(ctx, global, dataDir, assets.FS)
	if err != nil {
		return err
	}
	defer func() { _ = sq.Close() }()
	store := cstore.NewStore(sq.DB(), sq.BunDB(), sq.Dialect())
	errOut := cmd.ErrOrStderr()

	// Stage progress is opt-in. Without a tracker the Progress callback below
	// returns immediately, so a quiet run only formats the diagram itself.
	var tracker *term.StageTracker
	if opts.verbose {
		tracker = indexcmd.NewCompareStageTracker(errOut)
		defer tracker.Finish()
	}
	notice := func(text string) {
		if tracker != nil {
			tracker.Message(text)
			return
		}
		_, _ = fmt.Fprintln(errOut, text)
	}

	repositoryID, err := resolveRepository(ctx, store, dataDir, target, notice)
	if err != nil {
		return err
	}
	service := impact.Service{Workspace: sq, Index: store, Config: configbridge.FromGlobal(global)}
	service.Config.PrepareCommand = opts.prepareCommand
	var prepare func(context.Context, string) error
	if opts.prepareCommand != "" {
		prepare = func(ctx context.Context, directory string) error {
			command := exec.CommandContext(ctx, "bash", "-e", "-o", "pipefail", "-c", opts.prepareCommand)
			command.Dir = directory
			command.Stdout, command.Stderr = errOut, errOut
			command.WaitDelay = 2 * time.Second
			return command.Run()
		}
	}
	lastStage := indexcmd.DisplayStage("discover")
	revisions := map[string]string{impact.TargetBase: base, impact.TargetHead: head}
	shownTarget := ""
	display, depth := compareScope(opts, cmd.Flags().Changed("radius"))
	diagram, err := service.Compare(ctx, impact.CompareRequest{
		RepositoryID:    repositoryID,
		Base:            &pb.Revision{GitRevision: base},
		Head:            &pb.Revision{GitRevision: head},
		ContextDepth:    depth,
		PrepareCheckout: prepare,
		Progress: func(update indexer.Progress) {
			if tracker == nil {
				return
			}
			// Both revisions run the same pipeline, so announce the side before
			// its first stage instead of repeating identical stage lines.
			if update.Target != "" && update.Target != shownTarget {
				// Commit the previous side's trailing stage first so it does not
				// land under the next side's heading.
				tracker.Complete(lastStage)
				shownTarget = update.Target
				tracker.Message(compareTargetLabel(errOut, update.Target, revisions[update.Target]))
			}
			stage := indexcmd.DisplayStage(update.Stage)
			lastStage = stage
			if update.Total > 0 || update.Current > 0 || update.Detail != "" {
				tracker.Report(stage, update.Current, update.Total, update.Detail)
			} else {
				tracker.Begin(stage)
			}
		},
	})
	if err != nil {
		tracker.Fail(lastStage, err)
		return err
	}
	// Commit the trailing stage before reporting on the output so the notices
	// land below the work they describe.
	tracker.Complete(lastStage)
	requested := min(display, depth, diagram.GetMaxRadius())
	report := comparisonReport(impact.Scope(diagram, requested), opts)
	if opts.reportJSON != "" {
		for _, id := range []string{diagram.GetDiff().GetFromSnapshotId(), diagram.GetDiff().GetToSnapshotId()} {
			snapshot, loadErr := store.Snapshot(ctx, id)
			if loadErr != nil {
				return loadErr
			}
			report.Warnings = append(report.Warnings, snapshot.GetWarnings()...)
			if report.Base == "" {
				report.Base = snapshot.GetGitRevision()
			} else {
				report.Head = snapshot.GetGitRevision()
			}
		}
	}
	if report.Status == "skipped" {
		notice("warning: " + report.Reason)
		return writeCompareReport(opts.reportJSON, report)
	}
	render := compareRenderer{ctx: ctx, service: service, opts: opts}
	result, err := scopeToBudget(diagram, requested, opts, render.build)
	if err != nil {
		return err
	}
	if result.limited {
		notice(fmt.Sprintf(
			"warning: output limited to blast radius %d of %d (%d nodes); pass --radius %d or raise --max-nodes to include more",
			result.radius, requested, len(result.diagram.GetNodes()), requested))
	}
	if result.overBudget {
		notice(fmt.Sprintf("warning: output is %s even with direct changes only; --max-bytes %s cannot be met",
			humanBytes(result.size), humanBytes(opts.maxBytes)))
	}
	// The payload shares the terminal with the progress display; close the
	// pinned stage line before anything reaches stdout.
	tracker.Finish()
	if err := writeCompareReport(opts.reportJSON, report); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if opts.mermaid || opts.markdown {
		_, err = fmt.Fprint(out, result.text)
		return err
	}
	_, err = fmt.Fprintln(out, result.text)
	return err
}

// CompareReport counts the requested scope, before the shrinking output budgets.
type CompareReport struct {
	Status     string       `json:"status"`
	Elements   int          `json:"elements"`
	Connectors int          `json:"connectors"`
	Stats      CompareStats `json:"stats"`
	Base       string       `json:"base"`
	Head       string       `json:"head"`
	Warnings   []string     `json:"warnings"`
	Reason     string       `json:"reason,omitempty"`
}

// CompareStats summarizes the change itself: its scope, churn, and symbol
// deltas. It is derived from the snapshot diff, so it is independent of every
// output budget and stays available even when the diagram is skipped or empty.
// Paths feed downstream history lookups; they are capped so a large rename
// cannot blow the report or command line budget.
type CompareStats struct {
	Files           int      `json:"files"`
	Directories     int      `json:"directories"`
	Subsystems      int      `json:"subsystems"`
	LinesAdded      int      `json:"linesAdded"`
	LinesRemoved    int      `json:"linesRemoved"`
	SymbolsAdded    int      `json:"symbolsAdded"`
	SymbolsModified int      `json:"symbolsModified"`
	SymbolsRemoved  int      `json:"symbolsRemoved"`
	Paths           []string `json:"paths"`
	PathsTruncated  bool     `json:"pathsTruncated,omitempty"`
}

// maxReportPaths bounds the changed-path list carried in the report. The list
// exists so consumers can look up per-file history without re-deriving the
// change set; more paths than this is already an unusually large change.
const maxReportPaths = 500

func comparisonStats(diagram *pb.ImpactDiagram) CompareStats {
	stats := CompareStats{Paths: []string{}}
	diff := diagram.GetDiff()
	directories := map[string]struct{}{}
	subsystems := map[string]struct{}{}
	for _, change := range diff.GetSources() {
		file := change.GetPath()
		stats.Files++
		stats.LinesAdded += int(change.GetLinesAdded())
		stats.LinesRemoved += int(change.GetLinesRemoved())
		directories[path.Dir(file)] = struct{}{}
		if slash := strings.IndexByte(file, '/'); slash >= 0 {
			subsystems[file[:slash]] = struct{}{}
		} else {
			subsystems["."] = struct{}{}
		}
		if len(stats.Paths) < maxReportPaths {
			stats.Paths = append(stats.Paths, file)
		} else {
			stats.PathsTruncated = true
		}
	}
	stats.Directories = len(directories)
	stats.Subsystems = len(subsystems)
	stats.SymbolsAdded = countSymbols(diff.GetFacts().GetAdded())
	stats.SymbolsModified = countSymbols(diff.GetFacts().GetModified())
	stats.SymbolsRemoved = countSymbols(diff.GetFacts().GetRemoved())
	return stats
}

// countSymbols counts non-file facts. File facts mirror the changed path and
// would double the count that the change's symbol churn describes.
func countSymbols(facts []*pb.CodeFact) int {
	count := 0
	for _, fact := range facts {
		if fact.GetKind() == pb.FactKind_FACT_KIND_FILE {
			continue
		}
		count++
	}
	return count
}

func comparisonReport(diagram *pb.ImpactDiagram, opts compareOptions) CompareReport {
	report := CompareReport{Status: "ready", Elements: len(diagram.GetNodes()), Connectors: len(diagram.GetEdges()), Stats: comparisonStats(diagram), Warnings: []string{}}
	if report.Elements == 0 {
		report.Status = "empty"
	}
	if (opts.maxElements > 0 && report.Elements > opts.maxElements) || (opts.maxConnectors > 0 && report.Connectors > opts.maxConnectors) {
		report.Status = "skipped"
		report.Reason = fmt.Sprintf("diagram has %d elements and %d connectors; limits are %d elements and %d connectors (0 disables a limit)", report.Elements, report.Connectors, opts.maxElements, opts.maxConnectors)
	}
	return report
}

func writeCompareReport(path string, report CompareReport) error {
	if path == "" {
		return nil
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

// compareRenderer builds the payload for one blast radius. The default output is
// the portable impact scene — exactly what the canvas loads — so a reader needs
// no repository, index, or snapshot. Mermaid keeps the diagram as its source
// because it draws the file-level dependency graph, which a scene does not
// carry for files that already have workspace placements.
type compareRenderer struct {
	ctx     context.Context
	service impact.Service
	opts    compareOptions
}

// build renders a scoped diagram at radius, returning the text to write and its
// size in bytes so --max-bytes describes the payload the caller receives.
func (r compareRenderer) build(diagram *pb.ImpactDiagram, radius uint32) (string, int, error) {
	if r.opts.mermaid || r.opts.markdown {
		code := mermaid.ExportImpactDiagram(diagram, mermaid.ImpactExportOptions{IncludeMetadata: true, Radius: radius})
		if r.opts.markdown {
			code = mermaid.MermaidBlock(code)
		}
		return code, len(code), nil
	}
	scene, err := r.service.Scene(r.ctx, diagram)
	if err != nil {
		return "", 0, err
	}
	payload, err := protoJSONOptions.Marshal(scene)
	if err != nil {
		return "", 0, err
	}
	// The protojson payload is written with a trailing newline.
	return string(payload), len(payload) + 1, nil
}

// compareTargetLabel titles the stage run of one comparison side, naming the
// revision it scans so repeated stages are attributable.
func compareTargetLabel(out io.Writer, target, revision string) string {
	label := term.Colorize(out, term.ColorCyan, target)
	if revision == "" {
		return label
	}
	return label + " " + revision
}

// compareScope resolves the blast radius to display and the dependency depth to
// compute. The display defaults to --depth so passing --depth alone widens the
// output; an explicit --radius narrows the display without shrinking the
// computed neighbourhood.
func compareScope(opts compareOptions, radiusSet bool) (display, depth uint32) {
	display = opts.depth
	if radiusSet {
		display = opts.radius
	}
	depth = max(opts.depth, display)
	return display, depth
}

// scopedDiagram is the outcome of applying the CLI size budgets.
type scopedDiagram struct {
	diagram    *pb.ImpactDiagram
	radius     uint32
	text       string // the payload to write
	size       int
	limited    bool // radius was narrowed below the requested scope
	overBudget bool // direct changes alone still exceed --max-bytes
}

// scopeToBudget narrows the diagram by blast radius until it fits the node and
// byte budgets. build renders one candidate radius and reports the bytes it
// would write, so the budget describes the selected output format and not some
// internal encoding. If even the direct-change payload exceeds the byte budget
// the caller is told the budget cannot be met.
func scopeToBudget(diagram *pb.ImpactDiagram, requested uint32, opts compareOptions, build func(*pb.ImpactDiagram, uint32) (string, int, error)) (scopedDiagram, error) {
	radius, limited := impact.FitRadius(diagram, requested, opts.maxNodes)
	result := scopedDiagram{radius: radius, limited: limited}
	scoped := impact.Scope(diagram, radius)
	text, size, err := build(scoped, radius)
	if err != nil {
		return scopedDiagram{}, err
	}
	result.diagram, result.text, result.size = scoped, text, size
	if opts.maxBytes <= 0 {
		return result, nil
	}
	for radius > 0 && result.size > opts.maxBytes {
		radius--
		result.radius = radius
		result.limited = true
		result.diagram = impact.Scope(diagram, radius)
		if result.text, result.size, err = build(result.diagram, radius); err != nil {
			return scopedDiagram{}, err
		}
	}
	result.overBudget = result.size > opts.maxBytes
	return result, nil
}

func humanBytes(size int) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	value := float64(size)
	for _, suffix := range []string{"KiB", "MiB", "GiB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f TiB", value/unit)
}

// resolveRepository maps a repository id, local path, or remote URL to the
// stable repository id, cloning managed checkouts and registering identities
// the same way `tld index` does. Progress notices go through notice so they
// stay off stderr unless asked for.
func resolveRepository(ctx context.Context, store *cstore.Store, dataDir, target string, notice func(string)) (string, error) {
	cleaned := strings.TrimSpace(target)
	if cleaned == "" {
		return "", fmt.Errorf("repository is required")
	}
	if spec, isRemote := remoteTarget(cleaned); isRemote {
		root := remote.ManagedDir(dataDir, spec, uuid.Nil)
		notice(fmt.Sprintf("cloning %s into %s", spec.WebURL, root))
		if err := remote.Clone(ctx, spec, root); err != nil {
			return "", err
		}
		resolved, err := identity.Apply(ctx, store, root, "", spec.WebURL, true)
		if err != nil {
			return "", err
		}
		return resolved.ID, nil
	}
	if info, err := os.Stat(cleaned); err == nil && info.IsDir() {
		root, err := filepath.Abs(cleaned)
		if err != nil {
			return "", err
		}
		if root, err = filepath.EvalSymlinks(root); err != nil {
			return "", err
		}
		resolved, err := identity.Apply(ctx, store, root, "", "", false)
		if err != nil {
			return "", err
		}
		return resolved.ID, nil
	}
	repo, err := store.Repository(ctx, cleaned)
	if err != nil {
		return "", fmt.Errorf("repository %q is neither a local path nor a known repository id: %w", cleaned, err)
	}
	if strings.TrimSpace(repo.GetRoot()) == "" {
		return "", fmt.Errorf("repository %q has no checkout; run `tld index <path|url>` first", cleaned)
	}
	if info, err := os.Stat(repo.GetRoot()); err != nil || !info.IsDir() {
		return "", fmt.Errorf("repository checkout %q is unavailable; run `tld index <path|url>` again", repo.GetRoot())
	}
	return repo.GetId(), nil
}

// remoteTarget mirrors the index command's remote detection: explicit remote
// syntax always wins, while owner/repo shorthand only counts when no local
// directory matches.
func remoteTarget(raw string) (remote.Spec, bool) {
	cleaned := strings.TrimSpace(raw)
	explicit := strings.Contains(cleaned, "://") || strings.HasPrefix(cleaned, "git@") ||
		strings.HasPrefix(cleaned, "github.com/") || strings.HasSuffix(cleaned, ".git")
	if !explicit {
		if info, err := os.Stat(cleaned); err == nil && info.IsDir() {
			return remote.Spec{}, false
		}
	}
	spec, err := remote.Parse(cleaned)
	if err != nil {
		return remote.Spec{}, false
	}
	return spec, true
}
