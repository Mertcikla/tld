package git

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	mermaid  bool
	markdown bool
	verbose  bool
	radius   uint32
	depth    uint32
	maxNodes int
	maxBytes int
	dataDir  string
}

// protoJSONOptions keeps zero-value fields explicit so downstream consumers can
// read distances and counts without knowing proto3 defaults.
var protoJSONOptions = protojson.MarshalOptions{Multiline: true, Indent: "  ", EmitUnpopulated: true}

func newCompareCmd() *cobra.Command {
	opts := compareOptions{}
	c := &cobra.Command{
		Use:   "compare [repository] <base> <head>",
		Short: "Compare two revisions of a repository",
		Long: `Compare two Git revisions and emit the repository impact diagram.

The repository may be omitted to use the current checkout, or given as a
repository id, a local path, or a remote URL (github.com/owner/repo,
owner/repo, or a Git URL). Missing snapshots are indexed on demand.

The default output is the protojson encoding of the impact diagram. Pass
--mermaid to emit the same Mermaid change diagram the web UI renders.

--depth controls how many dependency hops of unchanged context are included
(0 = direct changes only). An explicit --radius narrows the displayed scope
without shrinking the computed neighbourhood. When the diagram exceeds the
node or byte budget the output is progressively narrowed by blast radius and
a warning is written to stderr.

Indexing progress is quiet by default so scripted runs only emit the diagram
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
	c.Flags().BoolVar(&opts.mermaid, "mermaid", false, "emit the Mermaid change diagram instead of protojson")
	c.Flags().BoolVar(&opts.markdown, "markdown", false, "wrap the Mermaid diagram in a Markdown code fence")
	c.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "report indexing progress for both revisions on stderr")
	c.Flags().Uint32Var(&opts.radius, "radius", 0, "blast radius to display; defaults to --depth")
	c.Flags().Uint32Var(&opts.depth, "depth", impact.DefaultContextDepth, "dependency hops of unchanged context to include (0 = direct changes only)")
	c.Flags().IntVar(&opts.maxNodes, "max-nodes", impact.DefaultMaxNodes, "node budget; the blast radius is narrowed when exceeded (0 disables)")
	c.Flags().IntVar(&opts.maxBytes, "max-bytes", 2<<20, "output byte budget; the blast radius is narrowed when exceeded (0 disables)")
	c.Flags().StringVar(&opts.dataDir, "data-dir", "", "override the data directory")
	return c
}

func runCompare(cmd *cobra.Command, opts compareOptions, target, base, head string) error {
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
	lastStage := indexcmd.DisplayStage("discover")
	revisions := map[string]string{impact.TargetBase: base, impact.TargetHead: head}
	shownTarget := ""
	display, depth := compareScope(opts, cmd.Flags().Changed("radius"))
	diagram, err := service.Compare(ctx, impact.CompareRequest{
		RepositoryID: repositoryID,
		Base:         &pb.Revision{GitRevision: base},
		Head:         &pb.Revision{GitRevision: head},
		ContextDepth: depth,
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
	result := scopeToBudget(diagram, requested, opts, outputSize(opts))
	if result.limited {
		notice(fmt.Sprintf(
			"warning: output limited to blast radius %d of %d (%d nodes); pass --radius %d or raise --max-nodes to include more",
			result.radius, requested, len(result.diagram.GetNodes()), requested))
	}
	if result.overBudget {
		notice(fmt.Sprintf("warning: output is %s even with direct changes only; --max-bytes %s cannot be met",
			humanBytes(result.size), humanBytes(opts.maxBytes)))
	}
	// The diagram shares the terminal with the progress display; close the
	// pinned stage line before anything reaches stdout.
	tracker.Finish()
	payload, _, err := renderDiagram(result.diagram, opts, result.radius)
	if err != nil {
		return err
	}
	if opts.mermaid || opts.markdown {
		_, err = fmt.Fprint(cmd.OutOrStdout(), payload)
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), payload)
	return err
}

// renderDiagram serializes a scoped diagram in the requested output format and
// reports its size in bytes. Sizes are taken from the format actually selected
// so --max-bytes describes the payload the caller receives rather than the
// protojson encoding the UI happens to use.
func renderDiagram(diagram *pb.ImpactDiagram, opts compareOptions, radius uint32) (string, int, error) {
	if opts.mermaid || opts.markdown {
		code := mermaid.ExportImpactDiagram(diagram, mermaid.ImpactExportOptions{IncludeMetadata: true, Radius: radius})
		if opts.markdown {
			code = mermaid.MermaidBlock(code)
		}
		return code, len(code), nil
	}
	payload, err := protoJSONOptions.Marshal(diagram)
	if err != nil {
		return "", 0, err
	}
	// The protojson payload is written with a trailing newline.
	return string(payload), len(payload) + 1, nil
}

// outputSize adapts renderDiagram into the sizer scopeToBudget narrows against.
func outputSize(opts compareOptions) func(*pb.ImpactDiagram, uint32) int {
	return func(diagram *pb.ImpactDiagram, radius uint32) int {
		_, size, err := renderDiagram(diagram, opts, radius)
		if err != nil {
			return 0
		}
		return size
	}
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
	size       int
	limited    bool // radius was narrowed below the requested scope
	overBudget bool // direct changes alone still exceed --max-bytes
}

// scopeToBudget narrows the diagram by blast radius until it fits the node and
// byte budgets. sizeOf reports the bytes the selected output format would emit,
// so a Mermaid run is not judged by the protojson encoding. If even the
// direct-change diagram exceeds the byte budget the caller is told the budget
// cannot be met.
func scopeToBudget(diagram *pb.ImpactDiagram, requested uint32, opts compareOptions, sizeOf func(*pb.ImpactDiagram, uint32) int) scopedDiagram {
	result := scopedDiagram{diagram: diagram}
	result.radius, result.limited = impact.FitRadius(diagram, requested, opts.maxNodes)
	result.diagram = impact.Scope(diagram, result.radius)
	result.size = sizeOf(result.diagram, result.radius)
	if opts.maxBytes <= 0 {
		return result
	}
	for result.radius > 0 && result.size > opts.maxBytes {
		result.radius--
		result.limited = true
		result.diagram = impact.Scope(diagram, result.radius)
		result.size = sizeOf(result.diagram, result.radius)
	}
	result.overBudget = result.size > opts.maxBytes
	return result
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
