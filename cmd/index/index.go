package index

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	ci "github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	"github.com/mertcikla/tld/v2/internal/codeindex/gitstate"
	cgraph "github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/identity"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
	"github.com/mertcikla/tld/v2/internal/codeindex/ingest"
	"github.com/mertcikla/tld/v2/internal/codeindex/mapconfig"
	"github.com/mertcikla/tld/v2/internal/codeindex/maprun"
	"github.com/mertcikla/tld/v2/internal/codeindex/parity"
	"github.com/mertcikla/tld/v2/internal/codeindex/remote"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/codeindex/watch"
	"github.com/mertcikla/tld/v2/internal/localserver"
	localstore "github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/app"
	"github.com/spf13/cobra"
)

type options struct {
	path         string
	watch        bool
	detach       bool
	watchOwner   string
	jsonOut      bool
	mapGraph     bool
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
		Use:   "index [path|url]",
		Short: "Index a repository into the codeindex graph",
		Long: `Index extracts code facts and edges from a repository using the
in-tree codeindex engine and publishes an immutable snapshot.

The target is a local directory or a remote URL (github.com/owner/repo,
owner/repo, or a Git URL). Remote repositories are cloned into tld's data
directory and treated as managed checkouts.

For a one-time index, pass --map to group the dependency graph into
architectural components and materialize the map view. Map grouping and
connector budgets inherit global map.* configuration, with per-repository
overrides configurable on the repository settings page.
With --watch, --map refreshes the map after each scan.

With --watch, Git's current commit is the Base and the combined staged,
unstaged, and nonignored untracked files are the Head. Git changes trigger
incremental indexing after a debounce. Each new commit gets an immutable
snapshot from its committed contents. Watch always saves an affected-file
change overlay, available in Repositories > Live changes. Compare maps can
compare commits or saved snapshots, always showing the full computed
neighbourhood of unchanged elements by dependency hops.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.path = "."
			if len(args) == 1 {
				opts.path = args[0]
			}
			if opts.watch && opts.detach {
				return runDetached(cmd, opts)
			}
			return run(cmd, opts)
		},
	}
	c.Flags().BoolVar(&opts.watch, "watch", false, "watch Git changes and update the live change overlay")
	c.Flags().BoolVar(&opts.detach, "detach", false, "start the watcher in the background and return")
	c.Flags().StringVar(&opts.watchOwner, "watch-owner", "cli", "internal: who started the watcher (cli or server)")
	_ = c.Flags().MarkHidden("watch-owner")
	c.Flags().BoolVar(&opts.jsonOut, "json", false, "emit machine-readable JSON")
	c.Flags().BoolVar(&opts.mapGraph, "map", false, "group the dependency graph and materialize the map view (opt-in)")
	c.Flags().StringVar(&opts.dataDir, "data-dir", "", "override the data directory")
	c.Flags().DurationVar(&opts.pollInterval, "poll-interval", 2*time.Second, "Git change polling interval")
	c.Flags().DurationVar(&opts.debounce, "debounce", 500*time.Millisecond, "delay used to batch file changes")
	c.Flags().StringArrayVar(&opts.exclude, "exclude", nil, "repository-relative path to exclude (repeatable)")
	c.AddCommand(newStatusCmd(), newStopCmd())
	return c
}

type engine struct {
	store   *cstore.Store
	ws      *app.Store
	cfg     ci.Config
	global  *workspace.Config
	opts    options
	dataDir string
	out     io.Writer
	errOut  io.Writer
	// repoID is the stable logical repository identity resolved from the
	// checkout's remote, so the same repository is one repository across
	// checkouts and machines.
	repoID string
}

// detectRemoteTarget reports whether the index target is a remote repository
// reference. Explicit remote syntax always wins; owner/repo shorthand is only
// treated as remote when no local directory matches.
func detectRemoteTarget(raw string) (remote.Spec, bool, error) {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return remote.Spec{}, false, nil
	}
	explicit := strings.Contains(cleaned, "://") || strings.HasPrefix(cleaned, "git@") ||
		strings.HasPrefix(cleaned, "github.com/") || strings.HasSuffix(cleaned, ".git")
	if !explicit {
		if _, err := os.Stat(cleaned); err == nil {
			return remote.Spec{}, false, nil
		}
	}
	spec, err := remote.Parse(cleaned)
	if err != nil {
		if explicit {
			return remote.Spec{}, false, err
		}
		return remote.Spec{}, false, nil
	}
	return spec, true, nil
}

func run(cmd *cobra.Command, opts options) error {
	ctx := cmd.Context()
	opts.jsonOut = opts.jsonOut || cmdutil.WantsJSONFromCmd(cmd)
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

	var remoteSpec remote.Spec
	root := ""
	if spec, isRemote, err := detectRemoteTarget(opts.path); err != nil {
		return err
	} else if isRemote {
		remoteSpec = spec
		root = remote.ManagedDir(dataDir, spec, uuid.Nil)
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Cloning %s into %s\n", spec.WebURL, root)
		if err := remote.Clone(ctx, spec, root); err != nil {
			return err
		}
	} else {
		root, err = filepath.Abs(opts.path)
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
	}

	sq, err := localstore.OpenLocal(ctx, global, dataDir, assets.FS)
	if err != nil {
		return err
	}
	defer func() { _ = sq.Close() }()

	eng := &engine{
		store:   cstore.NewStore(sq.DB(), sq.BunDB(), sq.Dialect()),
		ws:      sq,
		cfg:     configbridge.FromGlobal(global),
		global:  global,
		opts:    opts,
		dataDir: dataDir,
		out:     cmd.OutOrStdout(),
		errOut:  cmd.ErrOrStderr(),
	}
	_ = eng.store.BackfillRemoteKeys(ctx)
	resolved, err := identity.Apply(ctx, eng.store, root, "", remoteSpec.WebURL, remoteSpec.WebURL != "")
	if err != nil {
		return err
	}
	eng.repoID = resolved.ID
	if opts.watch {
		ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stop()
		cmd.SetContext(ctx)
		return eng.watch(ctx, cmd, root)
	}
	ctx, release, err := eng.store.AcquireLease(ctx, eng.repoID)
	if err != nil {
		return err
	}
	defer release()
	snap, report, mapRes, _, err := eng.buildAndPublish(ctx, root, nil)
	if err != nil {
		return err
	}
	return eng.print(cmd, snap, report, mapRes)
}

var indexStageDisplay = map[string]string{
	"discover":      "Discover",
	"tree-sitter":   "Parse sources",
	"scip":          "Index symbols",
	"relationships": "Relationships",
	"infra":         "Infrastructure",
	"verify":        "Verify",
	"overlay":       stageChanges,
}

var indexStageOrder = []string{
	"Discover", "Parse sources", "Index symbols", "Relationships", "Infrastructure", "Verify",
	"Publish snapshot", "Save diff diagram", "Map graph",
}

// compareStageOrder is the index stage prefix a comparison runs before saving
// its change overlay. It omits publishing and graph mapping.
var compareStageOrder = []string{
	"Discover", "Parse sources", "Index symbols", "Relationships", "Infrastructure", "Verify",
	stageChanges,
}

const (
	stagePublish  = "Publish snapshot"
	stageChanges  = "Save diff diagram"
	stageMapGraph = "Map graph"
)

// NewCompareStageTracker returns the stage tracker used by `tld git compare`,
// matching the `tld index` progress display.
func NewCompareStageTracker(out io.Writer) *term.StageTracker {
	return term.NewStageTracker(out, compareStageOrder, term.StageTrackerOptions{Jokes: indexJokes})
}

// DisplayStage maps an indexer stage id to its user-facing label.
func DisplayStage(stage string) string { return displayStage(stage) }

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

// resolveRepoID returns the stable repository identity for a checkout, falling
// back to the path-derived id when the engine was not seeded by run (tests and
// direct callers).
func (e *engine) resolveRepoID(root string) string {
	if e.repoID != "" {
		return e.repoID
	}
	return cgraph.RepositoryID(root)
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
func (e *engine) buildAndPublish(ctx context.Context, root string, base *indexer.IncrementalBase) (*pb.Snapshot, parity.Report, *pb.MapResult, bool, error) {
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

	pipeline := indexer.Pipeline{Config: e.cfg, RepositoryID: e.repoID}
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
	mapSnapshot := func() *pb.MapResult {
		if !e.opts.mapGraph {
			return nil
		}
		res, err := e.mapGraph(ctx, snap, tracker)
		if err != nil {
			// The snapshot is already published; a map failure is a warning,
			// not a reason to discard a valid index.
			e.warnf("map failed for %s: %v", root, err)
			return nil
		}
		return res
	}
	if reuse {
		return snap, parity.Report{}, mapSnapshot(), true, nil
	}

	tracker.Begin(stagePublish)
	if err := e.store.Publish(ctx, root, snap, g); err != nil {
		tracker.Fail(stagePublish, err)
		return nil, parity.Report{}, nil, false, err
	}
	return snap, parity.Summarize(snap, g), mapSnapshot(), false, nil
}

// warnf writes a non-fatal warning to the command's stderr.
func (e *engine) warnf(format string, args ...any) {
	w := e.errOut
	if w == nil {
		w = os.Stderr
	}
	_, _ = fmt.Fprintf(w, "warning: "+format+"\n", args...)
}

// mapGraph runs the graph mapping pipeline for a snapshot using global map
// defaults and repository overrides and reports progress through the active stage tracker.
func (e *engine) mapGraph(ctx context.Context, snap *pb.Snapshot, tracker *term.StageTracker) (*pb.MapResult, error) {
	tracker.Begin(stageMapGraph)
	result, _, err := maprun.Run(ctx, maprun.Deps{
		Workspace: e.ws,
		Codeindex: e.store,
		Options:   mapconfig.FromGlobal(e.global),
	}, maprun.Request{
		RepositoryID: snap.RepositoryId,
		SnapshotID:   snap.Id,
	}, func(_ string, current, total int, detail string) {
		tracker.Report(stageMapGraph, int64(current), int64(total), detail)
	})
	if err != nil {
		tracker.Fail(stageMapGraph, err)
		return nil, err
	}
	return result, nil
}

// watch detects Git changes through fsnotify with a polling failsafe, coalesces
// bursts with a quiet window, and cooperatively stops when the shared control
// record requests it. Failed scans remain pending and are retried without
// waiting for another edit.
func (e *engine) watch(ctx context.Context, cmd *cobra.Command, root string) error {
	if _, err := gitstate.Run(ctx, root, "rev-parse", "--git-dir"); err != nil {
		return fmt.Errorf("watch requires a Git repository: %w", err)
	}
	// Watch ownership is per checkout (path), so every developer can watch
	// their own clone. Snapshots and maps are published under e.repoID, the
	// shared logical repository identity.
	watchKey := cgraph.RepositoryID(root)
	ownerID := uuid.NewString()
	ownerKind := e.opts.watchOwner
	if ownerKind == "" {
		ownerKind = "cli"
	}
	status := cstore.WatchState{
		RepositoryID:   watchKey,
		OwnerKind:      ownerKind,
		OwnerPID:       os.Getpid(),
		OwnerID:        ownerID,
		State:          "starting",
		RepoRoot:       root,
		StartedUnix:    time.Now().Unix(),
		PollIntervalMS: e.opts.pollInterval.Milliseconds(),
		DebounceMS:     e.opts.debounce.Milliseconds(),
	}
	if err := e.store.ClaimWatch(ctx, status); err != nil {
		if errors.Is(err, cstore.ErrWatchActive) {
			return fmt.Errorf("a watcher is already running for %s", root)
		}
		return err
	}
	_ = localserver.RegisterProcess(localserver.ProcessRecord{
		Kind: localserver.ProcessKindWatch, PID: os.Getpid(), DataDir: e.dataDir, RepoRoot: root,
	})
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_ = e.store.ReleaseWatch(cleanup, watchKey, ownerID)
		_ = localserver.RemoveProcess(os.Getpid())
	}()

	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	var mu sync.Mutex
	persist := func(mutate func(*cstore.WatchState)) {
		mu.Lock()
		if mutate != nil {
			mutate(&status)
		}
		status.RepositoryID = watchKey
		status.OwnerID = ownerID
		status.OwnerPID = os.Getpid()
		snapshot := status
		mu.Unlock()
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if err := e.store.WatchHeartbeat(writeCtx, snapshot); errors.Is(err, cstore.ErrWatchOwnershipLost) {
			// Another watcher took over this repository. Exit so two watchers
			// never index concurrently.
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "another watcher took over %s; exiting\n", root)
			cancelRun()
		}
	}

	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	defer stopHeartbeat()
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		stoppingSince := time.Time{}
		for {
			if requested, err := e.store.WatchStopRequested(heartbeatCtx, watchKey); err == nil && requested {
				if stoppingSince.IsZero() {
					stoppingSince = time.Now()
					persist(func(s *cstore.WatchState) { s.State = "stopping" })
				}
				// Escalate if the scan loop does not stop promptly, so a stop can
				// never leave the watcher stuck.
				if time.Since(stoppingSince) > cstore.WatchStopDeadline {
					cancelRun()
					return
				}
			} else {
				stoppingSince = time.Time{}
			}
			persist(nil)
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	defer func() {
		stopHeartbeat()
		<-heartbeatDone
	}()

	detector := watch.NewDetector(watch.Options{
		Root:    root,
		Exclude: e.opts.exclude,
		OnState: func(state string) {
			persist(func(s *cstore.WatchState) { s.State = state; s.Stage = "" })
		},
		Debounce:     e.opts.debounce,
		MaxWait:      2*e.opts.debounce + time.Second,
		PollInterval: e.opts.pollInterval,
	})
	defer func() { _ = detector.Close() }()
	capture := func(captureCtx context.Context) (string, error) {
		qs, err := gitstate.CaptureQuick(captureCtx, root)
		if err != nil {
			return "", err
		}
		return qs.Signature(), nil
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "watching %s for Git changes (debounce %s, poll %s)\n", root, e.opts.debounce, e.opts.pollInterval)
	lastSignature, lastRevision := "", ""
	var prev *pb.Snapshot
	for {
		if runCtx.Err() != nil {
			return nil
		}
		persist(func(s *cstore.WatchState) { s.State = "idle" })
		_, err := detector.Next(runCtx, lastSignature, capture)
		if runCtx.Err() != nil {
			return nil
		}
		if err != nil {
			persist(func(s *cstore.WatchState) { s.State = "error"; s.Error = err.Error() })
			select {
			case <-runCtx.Done():
				return nil
			case <-time.After(e.opts.pollInterval):
			}
			continue
		}
		qs, err := gitstate.CaptureQuick(runCtx, root)
		if err != nil {
			persist(func(s *cstore.WatchState) { s.State = "error"; s.Error = err.Error() })
			continue
		}
		persist(func(s *cstore.WatchState) {
			s.State = "scanning"
			s.Stage = "discover"
			s.ChangedFiles = len(qs.Paths)
			s.PendingFiles = len(qs.Paths)
			s.GitBranch, s.GitRevision = qs.Branch, qs.Revision
		})
		started := time.Now()
		snap, report, mapRes, err := e.scanWatched(runCtx, root, qs, lastRevision, func(stage string) {
			mu.Lock()
			unchanged := status.Stage == stage
			mu.Unlock()
			if unchanged {
				return
			}
			persist(func(s *cstore.WatchState) { s.Stage = stage; s.State = "scanning" })
		})
		if err != nil {
			persist(func(s *cstore.WatchState) { s.State = "error"; s.Error = err.Error() })
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "index error: %v\n", err)
			continue
		}
		if prev == nil || prev.Id != snap.Id {
			if prev != nil {
				e.printDiff(cmd, prev.Id, snap.Id)
			}
			if err = e.print(cmd, snap, report, mapRes); err != nil {
				return err
			}
		}
		prev = snap
		lastSignature, lastRevision = qs.Signature(), qs.Revision
		persist(func(s *cstore.WatchState) {
			s.State = "idle"
			s.Error = ""
			s.Stage = ""
			s.ChangedFiles = 0
			s.PendingFiles = 0
			s.LastScanUnix = time.Now().Unix()
			s.LastScanMS = time.Since(started).Milliseconds()
			s.SnapshotID = snap.Id
			s.ContentFingerprint = snap.ContentFingerprint
			s.GitBranch, s.GitRevision = qs.Branch, qs.Revision
		})
	}
}

func (e *engine) scanWatched(ctx context.Context, root string, state gitstate.QuickState, previousRevision string, onStage ...func(string)) (*pb.Snapshot, parity.Report, *pb.MapResult, error) {
	reportStage := func(stage string) {
		for _, report := range onStage {
			report(stage)
		}
	}
	repoID := e.resolveRepoID(root)
	reportStage("waiting-indexer")
	var release func()
	var err error
	for {
		var leased context.Context
		leased, release, err = e.store.AcquireLease(ctx, repoID)
		if !errors.Is(err, cstore.ErrBusy) {
			if err != nil {
				return nil, parity.Report{}, nil, err
			}
			ctx = leased
			break
		}
		select {
		case <-ctx.Done():
			return nil, parity.Report{}, nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	defer release()
	reportStage("discover")
	out := e.out
	if e.opts.jsonOut {
		out = io.Discard
	}
	tracker := term.NewStageTracker(out, indexStageOrder, term.StageTrackerOptions{Jokes: indexJokes})
	defer tracker.Finish()
	engine := ingest.Engine{Store: e.store, Config: e.cfg, Root: root, RepositoryID: repoID, Exclude: e.opts.exclude, Progress: func(p indexer.Progress) {
		reportStage(p.Stage)
		tracker.Report(displayStage(p.Stage), p.Current, p.Total, p.Detail)
	}}
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
		if _, err = engine.Prepare(ctx, &pb.Revision{GitRevision: revision, GitBranch: state.Branch}); err != nil {
			return nil, parity.Report{}, nil, err
		}
	}
	base, err := engine.Prepare(ctx, &pb.Revision{GitRevision: state.Revision, GitBranch: state.Branch})
	if err != nil {
		return nil, parity.Report{}, nil, err
	}
	snap, err := engine.Prepare(ctx, &pb.Revision{WorkingTree: true})
	if err != nil {
		return nil, parity.Report{}, nil, err
	}
	after, err := gitstate.CaptureQuick(ctx, root)
	if err != nil {
		return nil, parity.Report{}, nil, err
	}
	if after.Signature() != state.Signature() {
		return nil, parity.Report{}, nil, fmt.Errorf("git inputs changed during indexing; retrying")
	}
	if err = e.store.AdvanceLatest(ctx, repoID, snap.Id); err != nil {
		return nil, parity.Report{}, nil, err
	}
	g, err := e.store.LoadGraph(ctx, snap.Id)
	if err != nil {
		return nil, parity.Report{}, nil, err
	}
	reportStage("live-map")
	tracker.Begin(stageChanges)
	if _, err = impact.Save(ctx, e.ws, e.store, repoID, "live", base.Id, snap.Id, impact.DefaultContextDepth); err != nil {
		return nil, parity.Report{}, nil, err
	}
	var mapRes *pb.MapResult
	if e.opts.mapGraph {
		if res, mapErr := e.mapGraph(ctx, snap, tracker); mapErr != nil {
			e.warnf("map failed for %s: %v", root, mapErr)
		} else {
			mapRes = res
		}
	}
	return snap, parity.Summarize(snap, g), mapRes, nil
}

func (e *engine) print(cmd *cobra.Command, snap *pb.Snapshot, report parity.Report, mapRes *pb.MapResult) error {
	out := cmd.OutOrStdout()
	if e.opts.jsonOut {
		payload := struct {
			Snapshot *pb.Snapshot  `json:"snapshot"`
			Report   parity.Report `json:"report"`
			Map      *pb.MapResult `json:"map,omitempty"`
		}{Snapshot: snap, Report: report, Map: mapRes}
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
	_, _ = fmt.Fprintf(tw, "edges\t%d\n", report.Edges)
	if report.Warnings > 0 {
		_, _ = fmt.Fprintf(tw, "warnings\t%d\n", report.Warnings)
	}
	if mapRes != nil {
		_, _ = fmt.Fprintf(tw, "map\tview %d (%d components, %d groups, %d isolated, modularity %.2f)\n",
			mapRes.GetViewId(), mapRes.GetClusters(), mapRes.GetBins(), mapRes.GetUnclustered(), mapRes.GetWeightedTightness())
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
