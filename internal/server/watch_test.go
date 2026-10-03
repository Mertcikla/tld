package server

import (
	"context"
	"net/http/httptest"
	"testing"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
)

func TestWatchStatusShowsStoppedWhenNoRecord(t *testing.T) {
	ctx := context.Background()
	ws, routes := newTestServer(t, uuid.New(), nil)
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
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

func TestWatchStopCreatesRecordAndRequestsStop(t *testing.T) {
	ctx := context.Background()
	ws, routes := newTestServer(t, uuid.New(), nil)
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	repoID := graph.RepositoryID("/repo")
	if err := idx.UpsertWatchState(ctx, cstore.WatchState{RepositoryID: repoID, OwnerKind: "cli", State: "watching", RepoRoot: "/repo"}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewWatchServiceClient(ts.Client(), ts.URL+"/api")
	if _, err := client.StopWatch(ctx, connect.NewRequest(&pb.StopWatchRequest{RepositoryId: repoID})); err != nil {
		t.Fatal(err)
	}
	requested, err := idx.WatchStopRequested(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if !requested {
		t.Fatal("stop was not recorded")
	}
}
