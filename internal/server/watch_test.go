package server

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/pkg/app"
)

func TestWatchManagerResolveCLI(t *testing.T) {
	for _, tt := range []struct {
		name       string
		onPath     string
		executable string
		want       string
	}{
		{name: "installed CLI", onPath: "bin", executable: "tld.app/Contents/MacOS", want: "bin"},
		{name: "standalone CLI fallback", executable: "standalone", want: "standalone"},
		{name: "Finder launched desktop", executable: "tld.app/Contents/MacOS"},
		{name: "desktop on PATH", onPath: "tld.app/Contents/MacOS", executable: "tld.app/Contents/MacOS"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("PATH", "")
			if tt.onPath != "" {
				dir := filepath.Join(root, tt.onPath)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, cliBinaryName()), []byte("CLI"), 0o755); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", dir)
			}
			manager := newWatchManager("", nil)
			manager.executable = func() (string, error) {
				return filepath.Join(root, tt.executable, cliBinaryName()), nil
			}
			got, err := manager.resolveCLI()
			if tt.want == "" {
				if err == nil || manager.cliAvailable() {
					t.Fatalf("desktop executable reported as an available CLI: %q", got)
				}
				return
			}
			want := filepath.Join(root, tt.want, cliBinaryName())
			if err != nil || got != want {
				t.Fatalf("resolveCLI = %q, %v; want %q", got, err, want)
			}
		})
	}
}

func TestWatchManagerRejectsSymlinkToDesktop(t *testing.T) {
	root := t.TempDir()
	appDir := filepath.Join(root, "tld.app", "Contents", "MacOS")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(appDir, cliBinaryName())
	if err := os.WriteFile(app, []byte("desktop"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, cliBinaryName())
	if err := os.Symlink(app, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	t.Setenv("PATH", root)
	manager := newWatchManager("", nil)
	manager.executable = func() (string, error) { return app, nil }
	if got, err := manager.resolveCLI(); err == nil {
		t.Fatalf("resolved desktop symlink as CLI: %q", got)
	}
}

func TestWatchStatusShowsStoppedWhenNoRecord(t *testing.T) {
	workspaceID := uuid.New()
	ws, routes := newTestServer(t, workspaceID, nil)
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	ctx := app.WithTenantOrgID(context.Background(), workspaceID)
	repoID := graph.RepositoryID("/repo")
	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewRepositoryServiceClient(ts.Client(), ts.URL+"/api")
	status, err := client.GetWatchStatus(ctx, connect.NewRequest(&pb.ID{Id: repoID}))
	if err != nil {
		t.Fatal(err)
	}
	if status.Msg.GetRunning() || status.Msg.GetState() != "stopped" {
		t.Fatalf("status: %+v", status.Msg)
	}
	if err := idx.UpsertWatchState(ctx, cstore.WatchState{
		RepositoryID:   repoID,
		OwnerKind:      "cli",
		State:          "scanning",
		Stage:          "tree-sitter",
		RepoRoot:       "/repo",
		GitBranch:      "main",
		GitRevision:    "abcdef123456",
		ChangedFiles:   2,
		PendingFiles:   1,
		LastScanMS:     42,
		LastScanUnix:   100,
		PollIntervalMS: 2000,
		DebounceMS:     350,
	}); err != nil {
		t.Fatal(err)
	}
	status, err = client.GetWatchStatus(ctx, connect.NewRequest(&pb.ID{Id: repoID}))
	if err != nil {
		t.Fatal(err)
	}
	got := status.Msg
	if !got.GetRunning() || got.GetState() != "scanning" || got.GetStage() != "tree-sitter" ||
		got.GetChangedFiles() != 2 || got.GetPendingFiles() != 1 || got.GetLastScanMs() != 42 ||
		got.GetPollIntervalMs() != 2000 || got.GetDebounceMs() != 350 || got.GetGitBranch() != "main" {
		t.Fatalf("status: %+v", got)
	}
}

func TestWatchStatusReapsDeadOwnerWithFreshHeartbeat(t *testing.T) {
	workspaceID := uuid.New()
	ws, routes := newTestServer(t, workspaceID, nil)
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	ctx := app.WithTenantOrgID(context.Background(), workspaceID)
	repoID := graph.RepositoryID("/repo")
	// A fresh heartbeat whose owning process does not exist must not read as
	// running; status reads reap it so the UI never shows a phantom watcher.
	if err := idx.UpsertWatchState(ctx, cstore.WatchState{
		RepositoryID: repoID, OwnerKind: "cli", OwnerID: "dead", OwnerPID: 1 << 30,
		State: "scanning", Stage: "tree-sitter", RepoRoot: "/repo",
	}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewRepositoryServiceClient(ts.Client(), ts.URL+"/api")
	status, err := client.GetWatchStatus(ctx, connect.NewRequest(&pb.ID{Id: repoID}))
	if err != nil {
		t.Fatal(err)
	}
	if status.Msg.GetRunning() || status.Msg.GetState() != "stopped" {
		t.Fatalf("dead owner reported running: %+v", status.Msg)
	}
	state, ok, err := idx.WatchState(ctx, repoID)
	if err != nil || !ok {
		t.Fatalf("state: %v %v", ok, err)
	}
	if state.State != "stopped" || state.HeartbeatUnix != 0 {
		t.Fatalf("dead owner not reaped: %+v", state)
	}
}

func TestWatchStopStopsAndClearsRecord(t *testing.T) {
	workspaceID := uuid.New()
	ws, routes := newTestServer(t, workspaceID, nil)
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	ctx := app.WithTenantOrgID(context.Background(), workspaceID)
	repoID := graph.RepositoryID("/repo")
	// A watcher with no live pid (an impossible pid) is treated as stale.
	if err := idx.UpsertWatchState(ctx, cstore.WatchState{RepositoryID: repoID, OwnerKind: "cli", OwnerID: "owner", OwnerPID: 1 << 30, State: "watching", RepoRoot: "/repo"}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewRepositoryServiceClient(ts.Client(), ts.URL+"/api")
	status, err := client.StopWatch(ctx, connect.NewRequest(&pb.ID{Id: repoID}))
	if err != nil {
		t.Fatal(err)
	}
	if status.Msg.GetRunning() || status.Msg.GetStopRequested() {
		t.Fatalf("stop left the watcher running: %+v", status.Msg)
	}
	state, ok, err := idx.WatchState(ctx, repoID)
	if err != nil || !ok {
		t.Fatalf("state: %v %v", ok, err)
	}
	if state.Live(time.Now()) || state.StopRequested {
		t.Fatalf("record not cleared: %+v", state)
	}
}
