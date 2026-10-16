package index

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mertcikla/codeindex/config"
	"github.com/mertcikla/codeindex/graph"
	assets "github.com/mertcikla/tld/v2"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	localstore "github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

func TestWatchStopCancelsRunContext(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		name := "idle"
		if blocked {
			name = "waiting for indexing lease"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("TLD_CONFIG_DIR", t.TempDir())
			root, dataDir := t.TempDir(), t.TempDir()
			testGit(t, root, "init", "-b", "main")
			testGit(t, root, "config", "user.name", "Test")
			testGit(t, root, "config", "user.email", "test@example.com")
			writeSource(t, root, "README.md", "watch stop fixture\n")
			testGit(t, root, "add", ".")
			testGit(t, root, "commit", "-m", "initial")
			ctx := context.Background()
			sq, err := localstore.OpenLocal(ctx, &workspace.Config{}, dataDir, assets.FS)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = sq.Close() }()
			idx := cstore.NewStore(sq.DB(), sq.BunDB(), sq.Dialect())
			repo, err := idx.ResolveRepository(ctx, root, "", false, uuid.Nil)
			if err != nil {
				t.Fatal(err)
			}
			if blocked {
				_, release, leaseErr := idx.AcquireLease(ctx, repo.ID)
				if leaseErr != nil {
					t.Fatal(leaseErr)
				}
				defer release()
			}
			var errors bytes.Buffer
			eng := &engine{
				store: idx, ws: sq, repoID: repo.ID, cfg: config.Default(), dataDir: dataDir,
				global: &workspace.Config{Serve: workspace.ServeConfig{Host: "127.0.0.1", Port: "1"}},
				out:    io.Discard, errOut: &errors,
				opts: options{pollInterval: time.Hour, debounce: 20 * time.Millisecond},
			}
			watchCtx, cancel := context.WithCancel(ctx)
			cmd := &cobra.Command{}
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			done := make(chan error, 1)
			go func() { done <- eng.watch(watchCtx, cmd, root) }()
			exited := false
			defer func() {
				cancel()
				if !exited {
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Error("watcher did not exit during test cleanup")
					}
				}
			}()
			key := graph.RepositoryID(root)
			deadline := time.Now().Add(10 * time.Second)
			for {
				st, ok, readErr := idx.WatchState(ctx, key)
				ready := ok && readErr == nil && (blocked && st.Stage == "waiting-indexer" || !blocked && st.State == "idle" && st.LastScanUnix > 0)
				if ready {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("watcher did not reach test state: %+v, %v", st, readErr)
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := idx.RequestWatchStop(ctx, key); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				exited = true
				if err != nil {
					t.Fatalf("stop returned an error: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("stop did not cancel the run context before the kill deadline")
			}
			if watchCtx.Err() != nil {
				t.Fatal("stop required cancellation of the parent context")
			}
			st, ok, err := idx.WatchState(ctx, key)
			if err != nil || !ok || st.State != "stopped" || st.StopRequested || st.HeartbeatUnix != 0 || st.Error != "" {
				t.Fatalf("stop did not clean up the control record: %+v, %v", st, err)
			}
			if strings.Contains(errors.String(), "index error") {
				t.Fatalf("cancellation reported as an indexing error: %s", errors.String())
			}
			if !blocked {
				_, release, err := idx.AcquireLease(ctx, repo.ID)
				if err != nil {
					t.Fatalf("indexing lease remained held after stop: %v", err)
				}
				release()
			}
		})
	}
}
