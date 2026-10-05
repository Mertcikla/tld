package server

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/pkg/app"
)

func TestWatchStatusShowsStoppedWhenNoRecord(t *testing.T) {
	workspaceID := uuid.New()
	ws, routes := newTestServer(t, workspaceID, nil)
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	ctx := app.WithTenantOrgID(context.Background(), workspaceID)
	repoID := graph.RepositoryID("/repo")
	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewWatchServiceClient(ts.Client(), ts.URL+"/api")
	status, err := client.GetWatchStatus(ctx, connect.NewRequest(&pb.GetWatchStatusRequest{RepositoryId: repoID}))
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
	status, err = client.GetWatchStatus(ctx, connect.NewRequest(&pb.GetWatchStatusRequest{RepositoryId: repoID}))
	if err != nil {
		t.Fatal(err)
	}
	got := status.Msg
	if !got.GetRunning() || got.GetState() != "scanning" || got.GetStage() != "tree-sitter" ||
		got.GetChangedFiles() != 2 || got.GetPendingFiles() != 1 || got.GetLastScanMs() != 42 ||
		got.GetPollIntervalMs() != 2000 || got.GetDebounceMs() != 350 || got.GetGitBranch() != "main" {
		t.Fatalf("status: %+v", got)
	}
	list, err := client.ListWatches(ctx, connect.NewRequest(&pb.ListWatchesRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Msg.GetWatchers()) != 1 || list.Msg.GetWatchers()[0].GetRepositoryId() != repoID {
		t.Fatalf("list: %+v", list.Msg)
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
	client := codeindexv1connect.NewWatchServiceClient(ts.Client(), ts.URL+"/api")
	status, err := client.GetWatchStatus(ctx, connect.NewRequest(&pb.GetWatchStatusRequest{RepositoryId: repoID}))
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
	client := codeindexv1connect.NewWatchServiceClient(ts.Client(), ts.URL+"/api")
	status, err := client.StopWatch(ctx, connect.NewRequest(&pb.StopWatchRequest{RepositoryId: repoID}))
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
