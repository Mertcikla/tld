package index

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	ci "github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	"github.com/mertcikla/tld/v2/internal/codeindex/embed"
	"github.com/mertcikla/tld/v2/internal/codeindex/gitstate"
	cgraph "github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
	"github.com/mertcikla/tld/v2/internal/codeindex/ingest"
	"github.com/mertcikla/tld/v2/internal/codeindex/materialize"
	"github.com/mertcikla/tld/v2/internal/codeindex/parity"
	"github.com/mertcikla/tld/v2/internal/codeindex/project"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/codeindex/visibility"
	localstore "github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

type options struct {
	path         string
	watch        bool
	jsonOut      bool
	embed        bool
	materialize  bool
	dataDir      string
	pollInterval time.Duration
	debounce     time.Duration
	exclude      []string
}

// NewIndexCmd builds the `tld index` command, which indexes a repository into
// the in-tree codeindex graph and optionally watches it for changes.
func NewIndexCmd() *cobra.Command {
	opts := options{}
	c := &cobra.Command{
		Use:   "index [path]",
		Short: "Index a repository into the codeindex graph",
		Long: `Index extracts code facts, edges, and chunks from a repository using the
in-tree codeindex engine and publishes an immutable snapshot.

Embeddings are computed by default and require a running embedding server
(start one with 'make embed-server'); pass --embed=false to publish a graph
without vectors. For a one-time index, pass --materialize to additionally
project candidate elements and connectors into a workspace view.

With --watch, Git's current commit is the Base and the combined staged,
unstaged, and nonignored untracked files are the Head. Git changes trigger
incremental indexing after a debounce. Each new commit gets an immutable
snapshot from its committed contents. Watch always saves an affected-file
change overlay, available in Repositories > Live changes; --materialize also
updates the full map. Compare maps can compare commits or saved snapshots.
The blast-radius slider adds existing unchanged elements by dependency hops.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.path = "."
			if len(args) == 1 {
				opts.path = args[0]
			}
			return run(cmd, opts)
		},
	}
	c.Flags().BoolVar(&opts.watch, "watch", false, "watch Git changes and update the live change overlay")
	c.Flags().BoolVar(&opts.jsonOut, "json", false, "emit machine-readable JSON")
	c.Flags().BoolVar(&opts.embed, "embed", true, "compute embeddings for the snapshot (requires a working embedding server)")
	c.Flags().BoolVar(&opts.materialize, "materialize", false, "also materialize candidates into a workspace view (opt-in)")
	c.Flags().StringVar(&opts.dataDir, "data-dir", "", "override the data directory")
	c.Flags().DurationVar(&opts.pollInterval, "poll-interval", 2*time.Second, "Git change polling interval")
	c.Flags().DurationVar(&opts.debounce, "debounce", 500*time.Millisecond, "delay used to batch file changes")
	c.Flags().StringArrayVar(&opts.exclude, "exclude", nil, "repository-relative path to exclude (repeatable)")
	return c
}

type engine struct {
	store    *cstore.Store
	ws       *localstore.SQLiteStore
	cfg      ci.Config
	opts     options
	repoName string
	repoRoot string
	out      io.Writer
}

func run(cmd *cobra.Command, opts options) error {
	ctx := cmd.Context()
	opts.jsonOut = opts.jsonOut || cmdutil.WantsJSONFromCmd(cmd)
	root, err := filepath.Abs(opts.path)
	if err != nil {
		return err
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return fmt.Errorf("index: %s is not a directory", opts.path)
	}

	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	if opts.pollInterval <= 0 || opts.debounce < 0 {
		return fmt.Errorf("poll interval must be positive and debounce nonnegative")
	}
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

	eng := &engine{
		store:    cstore.NewStore(sq.DB(), sq.BunDB(), sq.Dialect()),
		ws:       sq,
		cfg:      configbridge.FromGlobal(global),
		opts:     opts,
		repoName: filepath.Base(root),
		repoRoot: root,
		out:      cmd.OutOrStdout(),
	}
	if opts.embed {
		if err := (embed.Client{Config: eng.cfg}).Health(ctx); err != nil {
			return err
		}
	}
	if opts.watch {
		ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stop()
		cmd.SetContext(ctx)
		return eng.watch(ctx, cmd, root)
	}
	ctx, release, err := eng.store.AcquireLease(ctx, cgraph.RepositoryID(root))
	if err != nil {
		return err
	}
	defer release()
	snap, report, mres, _, err := eng.buildAndPublish(ctx, root, nil)
	if err != nil {
		return err
	}
	return eng.print(cmd, snap, report, mres)
}

var indexStageDisplay = map[string]string{
	"discover":      "Discover",
	"tree-sitter":   "Parse sources",
	"scip":          "Index symbols",
	"relationships": "Relationships",
	"infra":         "Infrastructure",
	"verify":        "Verify",
}

var indexStageOrder = []string{
	"Discover", "Parse sources", "Index symbols", "Relationships", "Infrastructure", "Verify",
	"Publish snapshot", "Embeddings", "Materialize view",
	"Save change overlay",
}

const (
	stagePublish     = "Publish snapshot"
	stageEmbeddings  = "Embeddings"
	stageMaterialize = "Materialize view"
	stageChanges     = "Save change overlay"
)

// indexJokes are rotated on the active stage line to keep long indexes
// entertaining and to gently roast whatever codebase is being indexed.
var indexJokes = []string{
	"This function is so long it has its own time zone.",
	"Found 0 tests and 47 \"temporary\" fixes. All permanent.",
	"Cyclomatic complexity called; it needs a nap.",
	"Someone named a variable data2_final_USE_THIS_one.",
	"Copy-paste is clearly a design pattern here.",
	"git blame suggests we all blame each other.",
	"This abstraction has some abstraction issues.",
	"Number of people who understand this module: 0.",
	"Analyzing your architecture... sorry, 'architecture' was generous. Analyzing your pile.",
	"Refactoring this would void the warranty.",
	"Dead code detected, still awaiting its funeral.",
	"The linter filed a restraining order.",
	"Warning: comments say \"trust me\"; tests say nothing.",
	"This file has seen things. Terrible things.",
	"It compiles, therefore it is correct. Probably.",
	"Checking for security issues... you hardcoded that in plain text? Bold. Truly bold.",
	"Your tests all pass! Impressive, considering none of them test anything.",
	"TODO: understand this code. TODO: never return.",
	"Almost done. Just waiting for the legacy module nobody dares to touch.",
	"Indexing variable names. temp, temp2, and temp_final_REAL are all accounted for.",
	"Oh good, another masterpiece of 'we'll refactor it later.'.",
	"Global state: because namespacing is hard.",
	"Detected a commit message that just says 'fix'. We have questions.",
}

func displayStage(stage string) string {
	if name, ok := indexStageDisplay[stage]; ok {
		return name
	}
	return stage
}

// buildAndPublish indexes root and publishes a snapshot. base, when non-nil,
// enables incremental reuse of unchanged files. reused is true when nothing
// changed and no new snapshot was written.
func (e *engine) buildAndPublish(ctx context.Context, root string, base *indexer.IncrementalBase) (*pb.Snapshot, parity.Report, *materialize.Result, bool, error) {
	out := e.out
	if out == nil || e.opts.jsonOut {
		out = io.Discard
	}
	tracker := term.NewStageTracker(out, indexStageOrder, term.StageTrackerOptions{
		Jokes: indexJokes,
	})
	defer tracker.Finish()

	lastStage := indexStageOrder[0]
	progress := func(update indexer.Progress) {
		stage := displayStage(update.Stage)
		if update.Total > 0 || update.Current > 0 || update.Detail != "" {
			tracker.Report(stage, update.Current, update.Total, update.Detail)
		} else {
			tracker.Begin(stage)
		}
		lastStage = stage
	}

	pipeline := indexer.Pipeline{Config: e.cfg}
	req := &pb.IndexRequest{Directory: root, Exclude: e.opts.exclude, Incremental: base != nil}

	var (
		snap  *pb.Snapshot
		g     *cgraph.Graph
		reuse bool
		err   error
	)
	if base != nil {
		snap, g, reuse, err = pipeline.BuildIncremental(ctx, req, progress, base)
	} else {
		snap, g, err = pipeline.Build(ctx, req, progress)
	}
	if err != nil {
		tracker.Fail(lastStage, err)
		return nil, parity.Report{}, nil, false, err
	}
	if reuse {
		return snap, parity.Report{}, nil, true, nil
	}

	tracker.Begin(stagePublish)
	if err := e.store.Publish(ctx, root, snap, g); err != nil {
		tracker.Fail(stagePublish, err)
		return nil, parity.Report{}, nil, false, err
	}

	if e.opts.embed && e.cfg.Embedding.Endpoint != "" {
		tracker.Begin(stageEmbeddings)
		client := embed.Client{Config: e.cfg, Store: e.store, Progress: func(current, total int, detail string) {
			tracker.Report(stageEmbeddings, int64(current), int64(total), detail)
		}}
		if err := client.Embed(ctx, snap); err != nil {
			wrapped := fmt.Errorf("embed: %w", err)
			tracker.Fail(stageEmbeddings, wrapped)
			return nil, parity.Report{}, nil, false, wrapped
		}
	}

	var mres *materialize.Result
	if e.opts.materialize && e.ws != nil {
		tracker.Begin(stageMaterialize)
		res, err := e.materializeSnapshot(ctx, snap, g, changedFiles(base, snap))
		if err != nil {
			tracker.Fail(stageMaterialize, err)
			return nil, parity.Report{}, nil, false, err
		}
		mres = &res
	}
	return snap, parity.Summarize(snap, g), mres, false, nil
}

// materializeSnapshot projects the snapshot and upserts every candidate into
// the workspace. All candidates are materialized so identity mappings stay
// stable; the visibility decision only controls each element's noise-gate
// bypass, so hidden candidates are still ranked and capped by the density
// engine rather than deleted.
func (e *engine) materializeSnapshot(ctx context.Context, snap *pb.Snapshot, g *cgraph.Graph, changed map[string]bool) (materialize.Result, error) {
	proj := project.Project(snap, g)
	decisions := visibility.Compute(visibility.Input{Projection: proj, ChangedFiles: changed}, visibility.DefaultConfig())
	return materialize.Apply(ctx, e.ws, e.store, proj, decisions, materialize.Options{
		RepositoryID:   snap.RepositoryId,
		RepositoryName: e.repoName,
		RepositoryRoot: e.repoRoot,
		SnapshotID:     snap.Id,
	})
}

// changedFiles lists sources whose hash differs from the incremental base. A nil
// base (cold start) yields an empty set: nothing is "changed", so visibility
// falls back to high-signal and density-ranked candidates.
func changedFiles(base *indexer.IncrementalBase, snap *pb.Snapshot) map[string]bool {
	if base == nil {
		return nil
	}
	out := map[string]bool{}
	for _, s := range snap.Sources {
		if h, ok := base.Sources[s.Path]; !ok || h != s.Hash {
			out[s.Path] = true
		}
	}
	return out
}

// watch polls Git state and dirty content, never directory mtimes. Failed
// captures remain pending and are retried without waiting for another edit.
func (e *engine) watch(ctx context.Context, cmd *cobra.Command, root string) error {
	if _, err := gitstate.Run(ctx, root, "rev-parse", "--git-dir"); err != nil {
		return fmt.Errorf("watch requires a Git repository: %w", err)
	}
	repoID := cgraph.RepositoryID(root)
	heartbeatCtx, stop := context.WithCancel(ctx)
	defer stop()
	var mu sync.Mutex
	branch, revision, message := "", "", ""
	updateStatus := func(state gitstate.State, err error) {
		mu.Lock()
		branch, revision, message = state.Branch, state.Revision, ""
		if err != nil {
			message = err.Error()
		}
		mu.Unlock()
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			mu.Lock()
			b, r, m := branch, revision, message
			mu.Unlock()
			_ = e.store.WatchHeartbeat(heartbeatCtx, repoID, b, r, m, true)
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	defer func() {
		stop()
		<-done
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_ = e.store.WatchHeartbeat(cleanup, repoID, branch, revision, message, false)
	}()
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "watching %s for Git changes (poll %s)\n", root, e.opts.pollInterval)
	lastSignature, lastRevision := "", ""
	var prev *pb.Snapshot
	for {
		if ctx.Err() != nil {
			return nil
		}
		state, err := gitstate.Capture(ctx, root)
		if err == nil && state.Signature != lastSignature {
			// Read again after the debounce, restarting whenever inputs change.
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(e.opts.debounce):
				}
				next, captureErr := gitstate.Capture(ctx, root)
				if captureErr != nil {
					err = captureErr
					break
				}
				if next.Signature == state.Signature {
					break
				}
				state = next
			}
			if err == nil {
				var snap *pb.Snapshot
				var report parity.Report
				var mres *materialize.Result
				snap, report, mres, err = e.scanWatched(ctx, root, state, lastRevision)
				if err == nil {
					if prev == nil || prev.Id != snap.Id {
						if prev != nil {
							e.printDiff(cmd, prev.Id, snap.Id)
						}
						if err = e.print(cmd, snap, report, mres); err != nil {
							return err
						}
					}
					prev = snap
					lastSignature, lastRevision = state.Signature, state.Revision
				}
			}
		}
		updateStatus(state, err)
		if err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "index error: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(e.opts.pollInterval):
		}
	}
}

func (e *engine) scanWatched(ctx context.Context, root string, state gitstate.State, previousRevision string) (*pb.Snapshot, parity.Report, *materialize.Result, error) {
	repoID := cgraph.RepositoryID(root)
	ctx, release, err := e.store.AcquireLease(ctx, repoID)
	if err != nil {
		return nil, parity.Report{}, nil, err
	}
	defer release()
	out := e.out
	if e.opts.jsonOut {
		out = io.Discard
	}
	tracker := term.NewStageTracker(out, indexStageOrder, term.StageTrackerOptions{Jokes: indexJokes})
	defer tracker.Finish()
	engine := ingest.Engine{Store: e.store, Config: e.cfg, Root: root, RepositoryID: repoID, Exclude: e.opts.exclude, Progress: func(p indexer.Progress) { tracker.Report(displayStage(p.Stage), p.Current, p.Total, p.Detail) }}
	if previousRevision == "" {
		if live, loadErr := e.store.Impact(ctx, repoID, "live"); loadErr == nil {
			previousRevision = live.Diff.FromGitRevision
		}
	}
	commits, err := gitstate.Commits(ctx, root, previousRevision, state.Revision)
	if err != nil {
		return nil, parity.Report{}, nil, err
	}
	for _, revision := range commits {
		if _, err = engine.Prepare(ctx, &pb.ComparisonTarget{GitRevision: revision, GitBranch: state.Branch}); err != nil {
			return nil, parity.Report{}, nil, err
		}
	}
	base, err := engine.Prepare(ctx, &pb.ComparisonTarget{GitRevision: state.Revision, GitBranch: state.Branch})
	if err != nil {
		return nil, parity.Report{}, nil, err
	}
	snap, err := engine.Prepare(ctx, &pb.ComparisonTarget{WorkingTree: true})
	if err != nil {
		return nil, parity.Report{}, nil, err
	}
	after, err := gitstate.Capture(ctx, root)
	if err != nil {
		return nil, parity.Report{}, nil, err
	}
	if after.Signature != state.Signature {
		return nil, parity.Report{}, nil, fmt.Errorf("git inputs changed during indexing; retrying")
	}
	if err = e.store.AdvanceLatest(ctx, repoID, snap.Id); err != nil {
		return nil, parity.Report{}, nil, err
	}
	g, err := e.store.LoadGraph(ctx, snap.Id)
	if err != nil {
		return nil, parity.Report{}, nil, err
	}
	if e.opts.embed {
		tracker.Begin(stageEmbeddings)
		client := embed.Client{Config: e.cfg, Store: e.store, Progress: func(current, total int, detail string) {
			tracker.Report(stageEmbeddings, int64(current), int64(total), detail)
		}}
		if err = client.Embed(ctx, snap); err != nil {
			return nil, parity.Report{}, nil, err
		}
	}
	var mres *materialize.Result
	if e.opts.materialize {
		tracker.Begin(stageMaterialize)
		incremental, err := engine.Base(ctx, base.Id)
		if err != nil {
			return nil, parity.Report{}, nil, err
		}
		result, err := e.materializeSnapshot(ctx, snap, g, changedFiles(incremental, snap))
		if err != nil {
			return nil, parity.Report{}, nil, err
		}
		mres = &result
	}
	radius := uint32(0)
	if recorded, loadErr := e.store.Impact(ctx, repoID, "live"); loadErr == nil {
		radius = recorded.Radius
	}
	tracker.Begin(stageChanges)
	if _, err = impact.Save(ctx, e.ws, e.store, repoID, "live", base.Id, snap.Id, radius); err != nil {
		return nil, parity.Report{}, nil, err
	}
	return snap, parity.Summarize(snap, g), mres, nil
}

func (e *engine) print(cmd *cobra.Command, snap *pb.Snapshot, report parity.Report, mres *materialize.Result) error {
	out := cmd.OutOrStdout()
	if e.opts.jsonOut {
		payload := struct {
			Snapshot     *pb.Snapshot        `json:"snapshot"`
			Report       parity.Report       `json:"report"`
			Materialized *materialize.Result `json:"materialized,omitempty"`
		}{Snapshot: snap, Report: report, Materialized: mres}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}
	_, _ = fmt.Fprintf(out, "snapshot %s\n", snap.Id)
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "repository\t%s\n", snap.RepositoryId)
	if snap.GitRevision != "" {
		_, _ = fmt.Fprintf(tw, "revision\t%s\n", shortHash(snap.GitRevision))
	}
	_, _ = fmt.Fprintf(tw, "sources\t%d\n", report.Sources)
	_, _ = fmt.Fprintf(tw, "projects\t%d\n", report.Projects)
	_, _ = fmt.Fprintf(tw, "facts\t%d\n", report.Facts)
	_, _ = fmt.Fprintf(tw, "chunks\t%d\n", report.Chunks)
	_, _ = fmt.Fprintf(tw, "edges\t%d\n", report.Edges)
	if report.Warnings > 0 {
		_, _ = fmt.Fprintf(tw, "warnings\t%d\n", report.Warnings)
	}
	if mres != nil {
		_, _ = fmt.Fprintf(tw, "view\t%d (%d elements, %d connectors, %d pruned)\n", mres.ViewID, mres.Elements, mres.Connectors, mres.Pruned)
	}
	_ = tw.Flush()
	return nil
}

func (e *engine) printDiff(cmd *cobra.Command, fromID, toID string) {
	diff, err := e.store.Diff(cmd.Context(), fromID, toID, false)
	if err != nil {
		return
	}
	added := len(diff.Facts.Added)
	removed := len(diff.Facts.Removed)
	modified := len(diff.Facts.Modified)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "changed: %d sources, +%d -%d ~%d facts\n",
		len(diff.Sources), added, removed, modified)
}

func shortHash(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
