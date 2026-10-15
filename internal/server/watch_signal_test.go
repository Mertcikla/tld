//go:build !windows

package server

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/pkg/app"
)

// Run as a subprocess so a graceful interrupt must reach NotifyContext, rather
// than merely observing that the supervisor eventually killed a process.
func TestWatchInterruptProcess(t *testing.T) {
	ready := os.Getenv("TLD_TEST_WATCH_READY")
	if ready == "" {
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := os.WriteFile(ready, []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	<-ctx.Done()
	if err := os.WriteFile(os.Getenv("TLD_TEST_WATCH_STOPPED"), []byte(ctx.Err().Error()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWatchManagerPropagatesInterrupt(t *testing.T) {
	for _, mode := range []string{"managed", "external", "shutdown"} {
		t.Run(mode, func(t *testing.T) {
			workspaceID := uuid.New()
			ws, _ := newTestServer(t, workspaceID, nil)
			idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
			ctx := app.WithTenantOrgID(context.Background(), workspaceID)
			root := t.TempDir()
			ready, stopped := filepath.Join(root, "ready"), filepath.Join(root, "stopped")
			childCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			cmd := exec.CommandContext(childCtx, os.Args[0], "-test.run=^TestWatchInterruptProcess$")
			cmd.Env = append(os.Environ(), "TLD_TEST_WATCH_READY="+ready, "TLD_TEST_WATCH_STOPPED="+stopped)
			cmd.SysProcAttr = getSysProcAttr()
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			child := &watchChild{repositoryID: "repo", repoRoot: root, pid: cmd.Process.Pid, cancel: cancel, done: make(chan struct{})}
			exit := make(chan error, 1)
			go func() {
				exit <- cmd.Wait()
				close(child.done)
			}()
			defer func() {
				cancel()
				select {
				case <-child.done:
				case <-time.After(5 * time.Second):
					t.Error("watch subprocess did not exit during cleanup")
				}
			}()
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, err := os.Stat(ready); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("subprocess did not install its context signal handler")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := idx.ClaimWatch(ctx, cstore.WatchState{RepositoryID: "repo", OwnerID: "owner", OwnerPID: child.pid, State: "scanning", RepoRoot: root}); err != nil {
				t.Fatal(err)
			}
			manager := newWatchManager(root, idx)
			if mode != "external" {
				manager.children["repo"] = child
			}
			if err := idx.RequestWatchStop(ctx, "repo"); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				if mode == "shutdown" {
					_ = manager.Close()
				} else {
					manager.stop(ctx, "repo")
				}
			}()
			defer func() {
				cancel()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("supervisor did not finish during cleanup")
				}
			}()
			select {
			case <-done:
			case <-time.After(4 * time.Second):
				t.Fatal("stop waited for the kill deadline instead of signalling cancellation")
			}
			if err := <-exit; err != nil {
				t.Fatalf("watcher was killed instead of exiting through its context: %v", err)
			}
			content, err := os.ReadFile(stopped)
			if err != nil || string(content) != context.Canceled.Error() {
				t.Fatalf("context cancellation did not run cleanup: %q, %v", content, err)
			}
		})
	}
}
