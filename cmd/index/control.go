package index

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"text/tabwriter"
	"time"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	cgraph "github.com/mertcikla/tld/v2/internal/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	localstore "github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

// openIndexStore resolves the data directory and opens the shared codeindex
// store used by the status and stop subcommands.
func openIndexStore(ctx context.Context, dataDirOverride string) (*localstore.SQLiteStore, *cstore.Store, string, error) {
	global, err := workspace.LoadGlobalConfig()
	if err != nil {
		return nil, nil, "", err
	}
	dataDir, err := workspace.ResolveDataDir(global, dataDirOverride)
	if err != nil {
		return nil, nil, "", err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, nil, "", err
	}
	sq, err := localstore.OpenLocal(ctx, global, dataDir, assets.FS)
	if err != nil {
		return nil, nil, "", err
	}
	return sq, cstore.NewStore(sq.DB(), sq.BunDB(), sq.Dialect()), dataDir, nil
}

func resolveRepoRoot(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func newStatusCmd() *cobra.Command {
	var (
		all     bool
		jsonOut bool
		dataDir string
	)
	c := &cobra.Command{
		Use:   "status [path]",
		Short: "Show index --watch status",
		Long: `Show the shared status of the codeindex watcher. With a path, shows that
repository's watcher. With no path or --all, lists every watcher.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut = jsonOut || cmdutil.WantsJSONFromCmd(cmd)
			ctx := cmd.Context()
			sq, idx, _, err := openIndexStore(ctx, dataDir)
			if err != nil {
				return err
			}
			defer func() { _ = sq.Close() }()
			out := cmd.OutOrStdout()
			if all || len(args) == 0 {
				_ = idx.ReapStaleWatchStates(ctx)
				states, err := idx.ListWatchStates(ctx)
				if err != nil {
					return err
				}
				if jsonOut {
					return json.NewEncoder(out).Encode(states)
				}
				if len(states) == 0 {
					_, _ = fmt.Fprintln(out, "no watchers")
					return nil
				}
				tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				_, _ = fmt.Fprintln(tw, "repository\tstate\towner\tpid\tbranch\trevision\tlast scan")
				for _, st := range states {
					_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
						st.RepoRoot, displayWatchState(st), st.OwnerKind, st.OwnerPID, st.GitBranch, shortHash(st.GitRevision), lastScan(st))
				}
				return tw.Flush()
			}
			root, err := resolveRepoRoot(args[0])
			if err != nil {
				return err
			}
			repoID := cgraph.RepositoryID(root)
			st, ok, err := idx.WatchState(ctx, repoID)
			if err != nil {
				return err
			}
			if ok && !st.Live(time.Now()) {
				_ = idx.ForceClearWatchState(ctx, repoID)
				st.State = "stopped"
				st.StopRequested = false
			}
			if !ok {
				if jsonOut {
					return json.NewEncoder(out).Encode(map[string]any{"repository_root": root, "running": false})
				}
				_, _ = fmt.Fprintf(out, "no watcher for %s\n", root)
				return nil
			}
			if jsonOut {
				return json.NewEncoder(out).Encode(st)
			}
			return printWatchStatus(out, st)
		},
	}
	c.Flags().BoolVar(&all, "all", false, "list every watcher")
	c.Flags().BoolVar(&jsonOut, "json", false, "emit machine-readable JSON")
	c.Flags().StringVar(&dataDir, "data-dir", "", "override the data directory")
	return c
}

func newStopCmd() *cobra.Command {
	var dataDir string
	c := &cobra.Command{
		Use:   "stop [path]",
		Short: "Stop the index --watch watcher",
		Long: `Request a cooperative stop of the repository's watcher. Works for watchers
started by the CLI and by the local server, since both read the same shared
control record.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}
			root, err := resolveRepoRoot(path)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			sq, idx, _, err := openIndexStore(ctx, dataDir)
			if err != nil {
				return err
			}
			defer func() { _ = sq.Close() }()
			repoID := cgraph.RepositoryID(root)
			st, ok, err := idx.WatchState(ctx, repoID)
			if err != nil {
				return err
			}
			if !ok || !st.Live(time.Now()) {
				_ = idx.ForceClearWatchState(ctx, repoID)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "no running watcher for %s\n", root)
				return nil
			}
			if err := idx.RequestWatchStop(ctx, repoID); err != nil {
				return err
			}
			stopped := func() bool {
				current, ok, err := idx.WatchState(ctx, repoID)
				return err == nil && (!ok || !current.Live(time.Now()))
			}
			deadline := time.Now().Add(cstore.WatchStopDeadline + 5*time.Second)
			for time.Now().Before(deadline) {
				time.Sleep(200 * time.Millisecond)
				if stopped() {
					_ = idx.ForceClearWatchState(ctx, repoID)
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "watcher stopped for %s\n", root)
					return nil
				}
			}
			// The watcher is not cooperating (e.g. an old binary that ignores the
			// stop flag). Terminate the recorded process and clear the record so
			// the repository is never left stuck.
			if st.OwnerPID > 0 {
				if proc, err := os.FindProcess(st.OwnerPID); err == nil {
					_ = proc.Kill()
				}
			}
			_ = idx.ForceClearWatchState(ctx, repoID)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "watcher for %s did not stop; terminated pid %d\n", root, st.OwnerPID)
			return nil
		},
	}
	c.Flags().StringVar(&dataDir, "data-dir", "", "override the data directory")
	return c
}

func printWatchStatus(out io.Writer, st cstore.WatchState) error {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "repository\t%s\n", st.RepoRoot)
	_, _ = fmt.Fprintf(tw, "state\t%s\n", displayWatchState(st))
	if st.Stage != "" {
		_, _ = fmt.Fprintf(tw, "stage\t%s\n", st.Stage)
	}
	_, _ = fmt.Fprintf(tw, "owner\t%s\n", st.OwnerKind)
	_, _ = fmt.Fprintf(tw, "pid\t%d\n", st.OwnerPID)
	_, _ = fmt.Fprintf(tw, "branch\t%s\n", st.GitBranch)
	if st.GitRevision != "" {
		_, _ = fmt.Fprintf(tw, "revision\t%s\n", shortHash(st.GitRevision))
	}
	if st.SnapshotID != "" {
		_, _ = fmt.Fprintf(tw, "snapshot\t%s\n", st.SnapshotID)
	}
	if st.ContentFingerprint != "" {
		_, _ = fmt.Fprintf(tw, "fingerprint\t%s\n", st.ContentFingerprint)
	}
	_, _ = fmt.Fprintf(tw, "changed files\t%d\n", st.ChangedFiles)
	_, _ = fmt.Fprintf(tw, "pending files\t%d\n", st.PendingFiles)
	_, _ = fmt.Fprintf(tw, "last scan\t%s\n", lastScan(st))
	if st.LastScanMS > 0 {
		_, _ = fmt.Fprintf(tw, "last scan ms\t%d\n", st.LastScanMS)
	}
	if st.HeartbeatUnix > 0 {
		_, _ = fmt.Fprintf(tw, "heartbeat\t%s\n", time.Unix(st.HeartbeatUnix, 0).Format(time.RFC3339))
	}
	if st.Error != "" {
		_, _ = fmt.Fprintf(tw, "error\t%s\n", st.Error)
	}
	return tw.Flush()
}

func displayWatchState(st cstore.WatchState) string {
	if !st.Live(time.Now()) {
		return "stopped"
	}
	if st.StopRequested {
		return "stopping"
	}
	if st.State == "" {
		return "running"
	}
	return st.State
}

func lastScan(st cstore.WatchState) string {
	if st.LastScanUnix == 0 {
		return "never"
	}
	return time.Unix(st.LastScanUnix, 0).Format(time.RFC3339)
}

// runDetached starts the watcher as a background process and returns. It is the
// fallback when no local server is available to host the watcher.
func runDetached(cmd *cobra.Command, opts options) error {
	root, err := resolveRepoRoot(opts.path)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	args := []string{"index", root, "--watch", "--watch-owner", "cli"}
	if opts.dataDir != "" {
		args = append(args, "--data-dir", opts.dataDir)
	}
	if !opts.embed {
		args = append(args, "--embed=false")
	}
	if opts.materialize {
		args = append(args, "--materialize")
	}
	if opts.pollInterval > 0 {
		args = append(args, "--poll-interval", opts.pollInterval.String())
	}
	if opts.debounce >= 0 {
		args = append(args, "--debounce", opts.debounce.String())
	}
	for _, pattern := range opts.exclude {
		args = append(args, "--exclude", pattern)
	}
	child := exec.Command(exe, args...)
	child.Dir = root
	child.Stdout = nil
	child.Stderr = nil
	child.SysProcAttr = getSysProcAttr()
	if err := child.Start(); err != nil {
		return fmt.Errorf("start background watcher: %w", err)
	}
	pid := child.Process.Pid

	// Verify the child actually claimed the repository. A child that loses the
	// claim race exits, and reporting success would be misleading.
	repoID := cgraph.RepositoryID(root)
	ctx := cmd.Context()
	sq, idx, _, err := openIndexStore(ctx, opts.dataDir)
	if err != nil {
		_ = child.Process.Kill()
		return err
	}
	defer func() { _ = sq.Close() }()
	deadline := time.Now().Add(5 * time.Second)
	claimed := false
	for time.Now().Before(deadline) && ctx.Err() == nil {
		if st, ok, err := idx.WatchState(ctx, repoID); err == nil && ok && st.Live(time.Now()) && st.OwnerPID == pid {
			claimed = true
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(100 * time.Millisecond):
		}
	}
	if !claimed {
		if reapErr := child.Process.Kill(); reapErr != nil {
			_ = child.Wait()
		}
		return fmt.Errorf("background watcher for %s did not start (another watcher may already be running)", root)
	}
	_ = child.Process.Release()
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "started background watcher for %s (pid %d)\n", root, pid)
	return nil
}
