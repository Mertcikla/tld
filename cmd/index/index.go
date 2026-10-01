package index

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	assets "github.com/mertcikla/tld/v2"
	ci "github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	"github.com/mertcikla/tld/v2/internal/codeindex/embed"
	cgraph "github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
	"github.com/mertcikla/tld/v2/internal/codeindex/materialize"
	"github.com/mertcikla/tld/v2/internal/codeindex/parity"
	"github.com/mertcikla/tld/v2/internal/codeindex/project"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/codeindex/visibility"
	localstore "github.com/mertcikla/tld/v2/internal/store"
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
in-tree codeindex engine and publishes an immutable snapshot. It only builds the
code graph; pass --materialize to additionally project candidate elements and
connectors into a workspace view. With --watch the repository is re-indexed
incrementally as files change.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.path = "."
			if len(args) == 1 {
				opts.path = args[0]
			}
			return run(cmd, opts)
		},
	}
	c.Flags().BoolVar(&opts.watch, "watch", false, "re-index incrementally as files change")
	c.Flags().BoolVar(&opts.jsonOut, "json", false, "emit machine-readable JSON")
	c.Flags().BoolVar(&opts.embed, "embed", false, "compute embeddings for the snapshot when an endpoint is configured")
	c.Flags().BoolVar(&opts.materialize, "materialize", false, "also materialize candidates into a workspace view (opt-in)")
	c.Flags().StringVar(&opts.dataDir, "data-dir", "", "override the data directory")
	c.Flags().DurationVar(&opts.pollInterval, "poll-interval", 2*time.Second, "file change polling interval")
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
}

func run(cmd *cobra.Command, opts options) error {
	ctx := cmd.Context()
	root, err := filepath.Abs(opts.path)
	if err != nil {
		return err
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return fmt.Errorf("index: %s is not a directory", opts.path)
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
	}
	if opts.watch {
		return eng.watch(ctx, cmd, root)
	}
	snap, report, mres, _, err := eng.buildAndPublish(ctx, root, nil)
	if err != nil {
		return err
	}
	return eng.print(cmd, snap, report, mres)
}

// buildAndPublish indexes root and publishes a snapshot. base, when non-nil,
// enables incremental reuse of unchanged files. reused is true when nothing
// changed and no new snapshot was written.
func (e *engine) buildAndPublish(ctx context.Context, root string, base *indexer.IncrementalBase) (*pb.Snapshot, parity.Report, *materialize.Result, bool, error) {
	pipeline := indexer.Pipeline{Config: e.cfg}
	req := &pb.IndexRequest{Directory: root, Exclude: e.opts.exclude, Incremental: base != nil}

	var (
		snap  *pb.Snapshot
		g     *cgraph.Graph
		reuse bool
		err   error
	)
	if base != nil {
		snap, g, reuse, err = pipeline.BuildIncremental(ctx, req, nil, base)
	} else {
		snap, g, err = pipeline.Build(ctx, req, nil)
	}
	if err != nil {
		return nil, parity.Report{}, nil, false, err
	}
	if reuse {
		return snap, parity.Report{}, nil, true, nil
	}
	if err := e.store.Publish(ctx, root, snap, g); err != nil {
		return nil, parity.Report{}, nil, false, err
	}
	if e.opts.embed && e.cfg.Embedding.Endpoint != "" {
		client := embed.Client{Config: e.cfg, Store: e.store}
		if err := client.Embed(ctx, snap); err != nil {
			return nil, parity.Report{}, nil, false, fmt.Errorf("embed: %w", err)
		}
	}
	var mres *materialize.Result
	if e.opts.materialize && e.ws != nil {
		res, err := e.materializeSnapshot(ctx, snap, g, changedFiles(base, snap))
		if err != nil {
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

func (e *engine) loadBase(ctx context.Context, repoID string) *indexer.IncrementalBase {
	latest, err := e.store.Latest(ctx, repoID)
	if err != nil || latest == "" {
		return nil
	}
	snap, err := e.store.Snapshot(ctx, latest)
	if err != nil {
		return nil
	}
	g, err := e.store.LoadGraph(ctx, latest)
	if err != nil {
		return nil
	}
	sources, err := e.store.SnapshotSources(ctx, latest)
	if err != nil {
		return nil
	}
	return &indexer.IncrementalBase{Graph: g, Snapshot: snap, Sources: sources}
}

func (e *engine) watch(ctx context.Context, cmd *cobra.Command, root string) error {
	repoID := cgraph.RepositoryID(root)
	base := e.loadBase(ctx, repoID)
	snap, report, mres, reused, err := e.buildAndPublish(ctx, root, base)
	if err != nil {
		return err
	}
	if !reused {
		if err := e.print(cmd, snap, report, mres); err != nil {
			return err
		}
	}
	sig, err := treeSignature(root)
	if err != nil {
		return err
	}
	prev := snap

	ticker := time.NewTicker(e.opts.pollInterval)
	defer ticker.Stop()
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "watching %s for changes (poll %s)\n", root, e.opts.pollInterval)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		next, err := treeSignature(root)
		if err != nil {
			return err
		}
		if next == sig {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(e.opts.debounce):
		}
		sig = next
		base = e.loadBase(ctx, repoID)
		snap, report, mres, reused, err = e.buildAndPublish(ctx, root, base)
		if err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "index error: %v\n", err)
			continue
		}
		if reused {
			continue
		}
		if prev != nil {
			e.printDiff(cmd, prev.Id, snap.Id)
		}
		if err := e.print(cmd, snap, report, mres); err != nil {
			return err
		}
		prev = snap
	}
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
	_, _ = fmt.Fprintf(out, "  repository %s\n", snap.RepositoryId)
	if snap.GitRevision != "" {
		_, _ = fmt.Fprintf(out, "  revision   %s\n", shortHash(snap.GitRevision))
	}
	_, _ = fmt.Fprintf(out, "  sources    %d\n", report.Sources)
	_, _ = fmt.Fprintf(out, "  projects   %d\n", report.Projects)
	_, _ = fmt.Fprintf(out, "  facts      %d\n", report.Facts)
	_, _ = fmt.Fprintf(out, "  chunks     %d\n", report.Chunks)
	_, _ = fmt.Fprintf(out, "  edges      %d\n", report.Edges)
	if report.Warnings > 0 {
		_, _ = fmt.Fprintf(out, "  warnings   %d\n", report.Warnings)
	}
	if mres != nil {
		_, _ = fmt.Fprintf(out, "  view       %d (%d elements, %d connectors, %d pruned)\n", mres.ViewID, mres.Elements, mres.Connectors, mres.Pruned)
	}
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

func treeSignature(root string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "dist", "build", ".tld":
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		_, _ = fmt.Fprintf(h, "%s|%d|%d\n", path, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func shortHash(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
