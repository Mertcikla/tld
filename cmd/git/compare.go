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
	"github.com/mertcikla/codeindex/indexer"
	"github.com/mertcikla/codeindex/remote"
	assets "github.com/mertcikla/tld/v2"
	indexcmd "github.com/mertcikla/tld/v2/cmd/index"
	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/mermaid"
	"github.com/mertcikla/tld/v2/internal/sceneurl"
	localstore "github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/app"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
)

type compareOptions struct {
	auto            bool
	url             bool
	urlLocal        bool
	urlBase         string
	mermaid         bool
	markdown        bool
	verbose         bool
	plain           bool
	noTheme         bool
	themeFile       string
	theme           *mermaid.Theme
	maxNodes        int
	maxBytes        int
	blastRadius     int
	dataDir         string
	maxElements     int
	maxConnectors   int
	reportJSON      string
	prepareCommand  string
	view            string
	debugProvenance bool
}

// protoJSONOptions keeps zero-value fields explicit so downstream consumers can
// read distances and counts without knowing proto3 defaults.
var protoJSONOptions = protojson.MarshalOptions{Multiline: true, Indent: "  ", EmitDefaultValues: true}

func newCompareCmd() *cobra.Command {
	opts := compareOptions{}
	c := &cobra.Command{
		Use:   "compare [repository] <base> <head>",
		Short: "Compare two revisions of a repository",
		Example: `  tld git compare HEAD~1 HEAD --mermaid
  tld git compare main feature --markdown
  tld git compare main feature --auto --markdown
  tld git compare HEAD~1 HEAD --url`,
		Long: `Compare two Git revisions and emit the canonical impact scene.

The repository may be omitted to use the current checkout, or given as a
repository id, a local path, or a remote URL (github.com/owner/repo,
owner/repo, or a Git URL). Missing snapshots are indexed on demand.

The default output is the canonical impact scene: the user-authored diagram stays
canonical, each linked element is verified against the pinned snapshots, and
the generated map contributes collapsed context only. The payload is
self-contained, so it renders offline without the repository, its index, or
its snapshots. Pass --mermaid to render a bounded projection of that scene,
with nested views, themed change markers, and directed connectors. Authored
context follows connector hops; hierarchy alone does not expand Mermaid.

--view restricts the scene to one authored view (id or name substring).
Only directly changed sections are emitted by default; --blast-radius (0-2)
additionally includes the neighbours and edges of each change that many
dependency hops away. Context beyond that is never computed or displayed;
when the output exceeds the node or byte budget it is progressively narrowed
by blast radius and a warning is written to stderr. All selected workspace connectors retain their original direction and styling.

--auto adapts context for PR review, simplifies unchanged relationships, and
declines oversized changes rather than hiding changed files. An explicit
--blast-radius caps its search (otherwise up to 2 hops). With --markdown it
may emit two disconnected diagrams, list isolated changed files below them,
or use a file summary when relationships are unavailable. Auto applies only
to Mermaid/Markdown; JSON and canvas URLs are unchanged. See docs/git-compare.md
for limits.
When a budget is exceeded, auto retries at each smaller radius down to 0,
assembling each attempt from the same pinned snapshots, then tries a flat
Plain diagram at radius 0 before declining.

Indexing progress is quiet by default so scripted runs only emit the payload
and budget warnings; pass --verbose to follow the base and head scans on
stderr.`,
		Args: cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			runOpts := opts
			if runOpts.auto && (runOpts.mermaid || runOpts.markdown) && !cmd.Flags().Changed("blast-radius") {
				runOpts.blastRadius = impact.MaxBlastRadius
			}
			target := "."
			if len(args) == 3 {
				target = args[0]
			}
			return runCompare(cmd, runOpts, target, args[len(args)-2], args[len(args)-1])
		},
	}
	c.Flags().BoolVar(&opts.mermaid, "mermaid", false, "emit the Mermaid change diagram instead of the scene")
	c.Flags().BoolVar(&opts.markdown, "markdown", false, "wrap the Mermaid diagram in a Markdown code fence")
	c.Flags().BoolVar(&opts.auto, "auto", false, "adapt context and density for PR review; Markdown may contain two diagrams or a file summary")
	c.Flags().BoolVar(&opts.url, "url", false, "emit a self-contained comparison URL for the local web server (maximum 10000 characters)")
	c.Flags().BoolVar(&opts.urlLocal, "url-local", false, "emit a self-contained tld:// comparison URL (maximum 10000 characters)")
	c.Flags().StringVar(&opts.urlBase, "url-base", "", "application base URL for --url (e.g. https://tldiagram.com/app)")
	c.Flags().BoolVar(&opts.plain, "plain", false, "hide placements without change impact from the Mermaid diagram")
	c.Flags().StringVar(&opts.themeFile, "theme", "", "JSON theme override for --mermaid or --markdown (merged with the embedded default)")
	c.Flags().BoolVar(&opts.noTheme, "no-theme", false, "omit Mermaid class and link styling; retain textual change labels")
	c.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "report indexing progress for both revisions on stderr")
	c.Flags().IntVar(&opts.maxNodes, "max-nodes", impact.DefaultMaxNodes, "node budget; the blast radius is narrowed when exceeded (0 disables)")
	c.Flags().IntVar(&opts.maxBytes, "max-bytes", 2<<20, "output byte budget; the blast radius is narrowed when exceeded (0 disables)")
	c.Flags().IntVar(&opts.blastRadius, "blast-radius", impact.DefaultBlastRadius, "dependency hops of unchanged neighbours and edges to include around each change (0-2)")
	c.Flags().StringVar(&opts.dataDir, "data-dir", "", "override the data directory")
	c.Flags().IntVar(&opts.maxElements, "max-elements", 0, "skip output when the requested diagram exceeds this element count (0 disables)")
	c.Flags().IntVar(&opts.maxConnectors, "max-connectors", 0, "skip output when the requested diagram exceeds this connector count (0 disables)")
	c.Flags().StringVar(&opts.reportJSON, "report-json", "", "write comparison status, counts, revisions, and index warnings to this JSON file")
	c.Flags().StringVar(&opts.prepareCommand, "prepare-command", "", "run this Bash command in each uncached revision before indexing; requires Bash")
	c.Flags().StringVar(&opts.view, "view", "", "restrict the scene to one authored view (id or name substring)")
	c.Flags().BoolVar(&opts.debugProvenance, "debug-provenance", false, "include provenance in JSON output and show pipeline decisions and coverage on stderr")
	return c
}

func runCompare(cmd *cobra.Command, opts compareOptions, target, base, head string) error {
	if (opts.themeFile != "" || opts.noTheme) && !opts.mermaid && !opts.markdown {
		return fmt.Errorf("--theme and --no-theme require --mermaid or --markdown")
	}
	if opts.themeFile != "" && opts.noTheme {
		return fmt.Errorf("--theme and --no-theme cannot be combined")
	}
	if opts.themeFile != "" {
		raw, err := os.ReadFile(opts.themeFile)
		if err != nil {
			return fmt.Errorf("read Mermaid theme: %w", err)
		}
		opts.theme, err = mermaid.ParseTheme(raw)
		if err != nil {
			return err
		}
	}
	if (opts.url && opts.urlLocal) || ((opts.url || opts.urlLocal) && (opts.mermaid || opts.markdown)) {
		return fmt.Errorf("--url and --url-local cannot be combined with each other, --mermaid, or --markdown")
	}
	if opts.urlBase != "" && !opts.url {
		return fmt.Errorf("--url-base requires --url")
	}
	if opts.maxElements < 0 || opts.maxConnectors < 0 {
		return fmt.Errorf("--max-elements and --max-connectors must be nonnegative")
	}
	if opts.blastRadius < 0 || opts.blastRadius > impact.MaxBlastRadius {
		return fmt.Errorf("--blast-radius must be between 0 and %d", impact.MaxBlastRadius)
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
	if opts.url {
		opts.urlBase, err = compareURLBase(global, dataDir, opts.urlBase)
		if err != nil {
			return err
		}
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
	// Only the requested blast radius is computed: direct changes plus that
	// many dependency hops of unchanged neighbours. Budgets below can only
	// narrow further.
	diagram, err := service.Compare(ctx, impact.CompareRequest{
		RepositoryID:    repositoryID,
		Base:            &pb.Revision{GitRevision: base},
		Head:            &pb.Revision{GitRevision: head},
		BlastRadius:     uint32(opts.blastRadius),
		ViewFilter:      opts.view,
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
	if opts.auto && (opts.mermaid || opts.markdown) {
		tracker.Finish()
		return runAutoCompare(cmd, opts, diagram, compareRenderer{ctx: ctx, service: service, opts: opts})
	}
	requested := uint32(opts.blastRadius)
	scoped := opts.scope(diagram, requested)
	if opts.plain && !opts.mermaid && !opts.markdown && scoped.Scene != nil {
		scoped.Scene = impact.SelectScene(scoped.Scene, requested, true)
	}
	recordSelection(scoped.Scene, opts)
	report := comparisonReport(scoped, opts)
	// Summary, report and output all project the selected frozen scene.
	if len(scoped.GetScene().GetAuthoredViewIds()) == 0 && opts.verbose {
		notice("notice: no authored views intersect this change; scene uses mapped and raw coverage")
	}

	if report.Status == "skipped" {
		if err := writeProvenance(opts, scoped.GetScene(), errOut); err != nil {
			return err
		}
		notice("warning: " + report.Reason)
		return writeCompareReport(opts.reportJSON, report)
	}
	render := compareRenderer{ctx: ctx, service: service, opts: opts}
	result, err := scopeToBudget(scoped, requested, opts, render.build)
	if err != nil {
		return err
	}
	if result.limited {
		notice(fmt.Sprintf(
			"warning: output limited to blast radius %d of %d (%d nodes); raise --max-nodes to include more",
			result.radius, requested, len(result.diagram.GetNodes())))
	}
	if result.overBudget {
		notice(fmt.Sprintf("warning: output is %s even with direct changes only; --max-bytes %s cannot be met",
			humanBytes(result.size), humanBytes(opts.maxBytes)))
	}
	// The payload shares the terminal with the progress display; close the
	// pinned stage line before anything reaches stdout.
	tracker.Finish()
	if err := writeProvenance(opts, result.diagram.GetScene(), errOut); err != nil {
		return err
	}
	if opts.debugProvenance && opts.maxBytes > 0 && len(result.text) > opts.maxBytes && result.size <= opts.maxBytes {
		notice("notice: debug provenance exceeds the normal byte budget; view selection is unchanged")
	}
	report = comparisonReport(result.diagram, opts)
	return writeCompareOutput(cmd, opts, result, report)
}

func writeCompareOutput(cmd *cobra.Command, opts compareOptions, result scopedDiagram, report CompareReport) error {
	if opts.url || opts.urlLocal {
		var link string
		var encodeErr error
		if opts.urlLocal {
			link, encodeErr = sceneurl.Encode([]byte(result.text), true)
		} else {
			link, encodeErr = sceneurl.EncodeAt([]byte(result.text), opts.urlBase)
		}
		if encodeErr != nil {
			report.Status, report.Reason = "skipped", encodeErr.Error()
			if report.Auto != nil {
				report.Auto.Mode, report.Auto.Reason, report.Auto.Diagrams = "skipped", report.Reason, 0
			}
			if err := writeCompareReport(opts.reportJSON, report); err != nil {
				return err
			}
			return encodeErr
		}
		if err := writeCompareReport(opts.reportJSON, report); err != nil {
			return err
		}
		_, err := fmt.Fprintln(cmd.OutOrStdout(), link)
		return err
	}
	if err := writeCompareReport(opts.reportJSON, report); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if opts.mermaid || opts.markdown {
		_, err := fmt.Fprint(out, result.text)
		return err
	}
	_, err := fmt.Fprintln(out, result.text)
	return err
}

// CompareReport adds command status to canonical comparison fields. Hard limits
// are checked against the requested scene; successful reports describe the emitted selection.
type CompareReport struct {
	Auto             *autoReport     `json:"auto,omitempty"`
	Status           string          `json:"status"`
	Elements         int             `json:"elements"`
	Connectors       int             `json:"connectors"`
	Stats            CompareStats    `json:"stats"`
	Warnings         []string        `json:"warnings"`
	Reason           string          `json:"reason,omitempty"`
	RequestedRadius  int             `json:"requestedRadius"`
	EffectiveRadius  int             `json:"effectiveRadius"`
	ManifestComplete bool            `json:"manifestComplete"`
	Scene            *pb.ImpactScene `json:"-"`
}

// CompareStats summarizes the change itself: its scope, churn, and symbol
// deltas. It is derived from the canonical scene, preserving unknown counts
// through the completeness flag, even when the diagram is skipped or empty.
// Paths feed downstream history lookups; they are capped so a large rename
// cannot blow the report or command line budget.
type CompareStats struct {
	Files                  int      `json:"files"`
	Directories            int      `json:"directories"`
	Subsystems             int      `json:"subsystems"`
	LinesAdded             int      `json:"linesAdded"`
	LinesRemoved           int      `json:"linesRemoved"`
	LineCountsComplete     bool     `json:"lineCountsComplete"`
	BinaryFiles            int      `json:"binaryFiles"`
	IndexedSymbolsAdded    int      `json:"indexedSymbolsAdded"`
	IndexedSymbolsModified int      `json:"indexedSymbolsModified"`
	IndexedSymbolsRemoved  int      `json:"indexedSymbolsRemoved"`
	EdgesAdded             int      `json:"edgesAdded"`
	EdgesRemoved           int      `json:"edgesRemoved"`
	EdgesModified          int      `json:"edgesModified"`
	Paths                  []string `json:"paths"`
	PathsTruncated         bool     `json:"pathsTruncated,omitempty"`
}

// maxReportPaths bounds the changed-path list carried in the report. The list
// exists so consumers can look up per-file history without re-deriving the
// change set; more paths than this is already an unusually large change.
const maxReportPaths = 500

func comparisonStats(diagram *model.Diagram) CompareStats {
	stats := CompareStats{Paths: []string{}}
	scene := impact.PublicScene(diagram.Scene, false)
	directories := map[string]struct{}{}
	subsystems := map[string]struct{}{}
	for _, file := range scene.GetFiles() {
		name := file.Path
		directories[path.Dir(name)] = struct{}{}
		if slash := strings.IndexByte(name, '/'); slash >= 0 {
			subsystems[name[:slash]] = struct{}{}
		} else {
			subsystems["."] = struct{}{}
		}
		if len(stats.Paths) < maxReportPaths {
			stats.Paths = append(stats.Paths, name)
		} else {
			stats.PathsTruncated = true
		}
	}
	stats.Files = int(scene.GetSummary().GetFiles())
	stats.LinesAdded = int(scene.GetSummary().GetKnownLinesAdded())
	stats.LinesRemoved = int(scene.GetSummary().GetKnownLinesRemoved())
	stats.LineCountsComplete = scene.GetSummary().GetLineCountsComplete()
	stats.BinaryFiles = int(scene.GetSummary().GetBinaryFiles())
	stats.Directories, stats.Subsystems = len(directories), len(subsystems)
	for _, symbol := range scene.GetSymbols() {
		switch symbol.Change {
		case pb.ChangeKind_CHANGE_KIND_ADDED:
			stats.IndexedSymbolsAdded++
		case pb.ChangeKind_CHANGE_KIND_MODIFIED:
			stats.IndexedSymbolsModified++
		case pb.ChangeKind_CHANGE_KIND_REMOVED:
			stats.IndexedSymbolsRemoved++
		}
	}
	for _, view := range scene.GetViews() {
		for _, edge := range view.Connectors {
			for _, tag := range edge.Tags {
				switch tag {
				case "change:added":
					stats.EdgesAdded++
				case "change:removed":
					stats.EdgesRemoved++
				case "change:modified":
					stats.EdgesModified++
				}
			}
		}
	}
	return stats
}

func comparisonReport(diagram *model.Diagram, opts compareOptions) CompareReport {
	report := CompareReport{Status: "ready", Elements: len(diagram.GetNodes()), Connectors: len(diagram.GetEdges()), Stats: comparisonStats(diagram), Warnings: append([]string{}, diagram.GetWarnings()...), RequestedRadius: opts.blastRadius, EffectiveRadius: opts.blastRadius, ManifestComplete: diagram.GetManifestComplete(), Scene: impact.PublicScene(diagram.GetScene(), false)}
	if diagram.Scene != nil {
		report.EffectiveRadius = int(diagram.Scene.EffectiveRadius)
		report.Elements = impact.SceneNodeCount(diagram.Scene)
		report.Connectors = 0
		for _, view := range diagram.Scene.Views {
			report.Connectors += len(view.Connectors)
		}
	}
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
	// Reuse the canonical wire fields instead of encoding Go protobuf structs
	// with a second set of names, enum values and optional-count semantics.
	canonical, err := protoJSONOptions.Marshal(report.Scene)
	if err != nil {
		return err
	}
	var fields struct {
		Comparison json.RawMessage `json:"comparison"`
		Files      json.RawMessage `json:"files"`
		Summary    json.RawMessage `json:"summary"`
	}
	if err := json.Unmarshal(canonical, &fields); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(struct {
		CompareReport
		Comparison json.RawMessage `json:"comparison"`
		Files      json.RawMessage `json:"files"`
		Summary    json.RawMessage `json:"summary"`
	}{report, fields.Comparison, fields.Files, fields.Summary}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

// compareRenderer builds the payload for one blast radius. The output is
// self-contained; Mermaid applies its own presentation policy to a scene copy.
type compareRenderer struct {
	ctx     context.Context
	service impact.Service
	opts    compareOptions
}

// build renders a scoped diagram at radius, returning the text to write and its
// size in bytes so --max-bytes describes the payload the caller receives.
// The scene is pruned to the same radius, so unimpacted sections never reach
// the payload.
func (r compareRenderer) build(diagram *model.Diagram, radius uint32) (string, int, error) {
	scene, err := r.scene(diagram)
	if err != nil {
		return "", 0, err
	}
	if r.opts.mermaid || r.opts.markdown {
		scene = selectMermaidAssembly(scene, radius, r.opts.plain || scene.GetComparison().GetPlain())
		code := mermaid.ExportImpactScene(impact.PublicScene(scene, false), r.opts.sceneExportOptions())
		if r.opts.markdown {
			code = mermaid.MermaidBlock(code)
		}
		return code, len(code), nil
	}
	scene = impact.SelectScene(scene, radius, r.opts.plain)
	if err := impact.ValidateCoverage(scene); err != nil {
		return "", 0, err
	}
	recordSelection(scene, r.opts)
	public := impact.PublicScene(scene, r.opts.debugProvenance)
	payload, err := protoJSONOptions.Marshal(public)
	if err != nil {
		return "", 0, err
	}
	size := len(payload)
	if r.opts.debugProvenance {
		public.Provenance = nil
		normal, marshalErr := protoJSONOptions.Marshal(public)
		if marshalErr != nil {
			return "", 0, marshalErr
		}
		size = len(normal)
	}
	return string(payload), size, nil
}

func (opts compareOptions) sceneExportOptions() mermaid.SceneExportOptions {
	theme := opts.theme
	if theme == nil {
		theme = mermaid.DefaultTheme()
	}
	return mermaid.SceneExportOptions{IncludeMetadata: true, Theme: theme, NoTheme: opts.noTheme, Plain: opts.plain}
}

// recordSelection adds CLI-only budgets to the selected copy of the shared
// scene. The immutable cached assembly is never modified.
func recordSelection(scene *model.Assembly, opts compareOptions) {
	if scene == nil {
		return
	}
	if scene.Comparison == nil {
		scene.Comparison = &pb.ImpactComparison{}
	}
	if opts.maxNodes > 0 {
		scene.Comparison.NodeBudget = uint32(opts.maxNodes)
	}
	if opts.maxBytes > 0 {
		scene.Comparison.ByteBudget = uint32(opts.maxBytes)
	}
}

// scene returns the frozen scene for a diagram, rejecting obsolete artifacts that
// must be regenerated with the current pipeline. The returned scene may be the
// diagram's own; callers always copy through SelectScene before mutating.
func (r compareRenderer) scene(diagram *model.Diagram) (*model.Assembly, error) {
	scene := diagram.GetScene()
	if scene != nil && scene.GetSchemaVersion() == impact.PipelineVersion {
		return scene, nil
	}
	return r.service.Scene(r.ctx, diagram)
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

// scopedDiagram is the outcome of applying the CLI size budgets.
type scopedDiagram struct {
	diagram    *model.Diagram
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
func scopeToBudget(diagram *model.Diagram, requested uint32, opts compareOptions, build func(*model.Diagram, uint32) (string, int, error)) (scopedDiagram, error) {
	if opts.plain && !opts.mermaid && !opts.markdown && diagram.GetScene() != nil {
		diagram = impact.Scope(diagram, requested)
		diagram.Scene = impact.SelectScene(diagram.Scene, requested, true)
	}
	radius, limited := impact.FitRadius(diagram, requested, opts.maxNodes)
	if opts.mermaid || opts.markdown {
		radius = requested
		for radius > 0 && opts.maxNodes > 0 && impact.SceneNodeCount(opts.scope(diagram, radius).Scene) > opts.maxNodes {
			radius--
		}
		limited = radius < requested
	}
	result := scopedDiagram{radius: radius, limited: limited}
	scoped := opts.scope(diagram, radius)
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
		result.diagram = opts.scope(diagram, radius)
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
		resolved, err := store.ResolveRepository(ctx, root, spec.WebURL, true, app.TenantOrgIDFromCtx(ctx))
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
		resolved, err := store.ResolveRepository(ctx, root, "", false, app.TenantOrgIDFromCtx(ctx))
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

// scope selects presentation membership only for textual Mermaid exports.
func (opts compareOptions) scope(diagram *model.Diagram, radius uint32) *model.Diagram {
	out := impact.Scope(diagram, radius)
	if (opts.mermaid || opts.markdown) && out.Scene != nil {
		out.Scene = selectMermaidAssembly(diagram.Scene, radius, opts.plain)
	}
	return out
}

// selectMermaidAssembly applies the exporter policy to an owned assembly copy.
func selectMermaidAssembly(scene *model.Assembly, radius uint32, plain bool) *model.Assembly {
	return impact.SelectPresentation(scene, mermaid.SceneMembership(impact.PublicScene(scene, false), radius, plain), radius, plain)
}
