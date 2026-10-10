package server

import (
	"context"
	"net/http/httptest"
	"testing"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/pkg/app"
)

func captureSnapshot(t *testing.T, client codeindexv1connect.CodeIndexServiceClient, ctx context.Context, request *pb.CaptureSnapshotRequest) (*pb.Snapshot, error) {
	t.Helper()
	stream, err := client.CaptureSnapshot(ctx, connect.NewRequest(request))
	if err != nil {
		return nil, err
	}
	var snapshot *pb.Snapshot
	for stream.Receive() {
		if snap := stream.Msg().GetSnapshot(); snap != nil {
			snapshot = snap
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	if snapshot == nil {
		t.Fatal("capture stream ended without a snapshot")
	}
	return snapshot, nil
}

func seedCapturedRepository(t *testing.T, workspaceID uuid.UUID, root string) (*cstore.Store, *httptest.Server) {
	t.Helper()
	appStore, routes := newTestServer(t, workspaceID, nil)
	idx := cstore.NewStore(appStore.DB(), appStore.BunDB(), appStore.Dialect())
	ctx := app.WithTenantOrgID(context.Background(), workspaceID)
	repoID := graph.RepositoryID(root)
	seed := &pb.Snapshot{Id: "seed", RepositoryId: repoID, IngestionStatus: "complete"}
	if err := idx.Publish(ctx, root, seed, graph.NewGraph(repoID, seed.Id)); err != nil {
		t.Fatalf("publish seed: %v", err)
	}
	return idx, httptest.NewServer(routes)
}

func TestCaptureSnapshotCommitPublishesSavedPoint(t *testing.T) {
	workspaceID := uuid.New()
	root, sha := gitFixture(t)
	_, ts := seedCapturedRepository(t, workspaceID, root)
	defer ts.Close()
	client := codeindexv1connect.NewCodeIndexServiceClient(ts.Client(), ts.URL+"/api")
	ctx := app.WithTenantOrgID(context.Background(), workspaceID)
	repoID := graph.RepositoryID(root)

	snapshot, err := captureSnapshot(t, client, ctx, &pb.CaptureSnapshotRequest{RepositoryId: repoID})
	if err != nil {
		t.Fatalf("CaptureSnapshot: %v", err)
	}
	if snapshot.GetProvenance() != "commit" || snapshot.GetGitRevision() != sha || snapshot.GetGitBranch() != "main" {
		t.Fatalf("snapshot provenance = %+v, want commit %s on main", snapshot, sha)
	}
	if snapshot.GetIngestionStatus() != "complete" {
		t.Fatalf("ingestion status = %q, want complete", snapshot.GetIngestionStatus())
	}
	if len(snapshot.GetSources()) != 1 || snapshot.GetSources()[0].GetPath() != "sample.go" {
		t.Fatalf("snapshot sources = %+v", snapshot.GetSources())
	}

	snaps, err := client.ListSnapshots(ctx, connect.NewRequest(&pb.ID{Id: repoID}))
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	found := false
	for _, snap := range snaps.Msg.GetSnapshots() {
		if snap.GetId() == snapshot.GetId() {
			found = true
		}
	}
	if !found {
		t.Fatalf("captured snapshot %s missing from ListSnapshots", snapshot.GetId())
	}
}

func TestCaptureSnapshotWorkingTreeIsSavedPoint(t *testing.T) {
	workspaceID := uuid.New()
	root, sha := gitFixture(t)
	writeFixtureSource(t, root, "untracked.go", "package sample\nfunc Local() {}\n")
	_, ts := seedCapturedRepository(t, workspaceID, root)
	defer ts.Close()
	client := codeindexv1connect.NewCodeIndexServiceClient(ts.Client(), ts.URL+"/api")
	ctx := app.WithTenantOrgID(context.Background(), workspaceID)
	repoID := graph.RepositoryID(root)

	first, err := captureSnapshot(t, client, ctx, &pb.CaptureSnapshotRequest{RepositoryId: repoID, WorkingTree: true})
	if err != nil {
		t.Fatalf("CaptureSnapshot: %v", err)
	}
	if first.GetProvenance() != "manual" || first.GetGitRevision() != sha {
		t.Fatalf("working tree snapshot = %+v, want manual provenance at %s", first, sha)
	}
	if len(first.GetSources()) != 2 {
		t.Fatalf("working tree sources = %+v, want the tracked and untracked files", first.GetSources())
	}

	snaps, err := client.ListSnapshots(ctx, connect.NewRequest(&pb.ID{Id: repoID}))
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	found := false
	for _, snap := range snaps.Msg.GetSnapshots() {
		if snap.GetId() == first.GetId() {
			found = true
		}
	}
	if !found {
		t.Fatalf("manual snapshot %s filtered out of ListSnapshots", first.GetId())
	}

	// An unchanged working tree reuses the saved point instead of duplicating it.
	reused, err := captureSnapshot(t, client, ctx, &pb.CaptureSnapshotRequest{RepositoryId: repoID, WorkingTree: true})
	if err != nil {
		t.Fatalf("second CaptureSnapshot: %v", err)
	}
	if reused.GetId() != first.GetId() {
		t.Fatalf("unchanged capture = %s, want reuse of %s", reused.GetId(), first.GetId())
	}

	writeFixtureSource(t, root, "untracked.go", "package sample\nfunc Changed() {}\n")
	changed, err := captureSnapshot(t, client, ctx, &pb.CaptureSnapshotRequest{RepositoryId: repoID, WorkingTree: true})
	if err != nil {
		t.Fatalf("changed CaptureSnapshot: %v", err)
	}
	if changed.GetId() == first.GetId() || changed.GetProvenance() != "manual" {
		t.Fatalf("changed capture = %+v, want a new manual snapshot", changed)
	}
}

func TestCaptureSnapshotRequiresCommitHistory(t *testing.T) {
	workspaceID := uuid.New()
	root := t.TempDir()
	writeFixtureSource(t, root, "a.go", "package a\n\nfunc A() {}\n")
	_, ts := seedCapturedRepository(t, workspaceID, root)
	defer ts.Close()
	client := codeindexv1connect.NewCodeIndexServiceClient(ts.Client(), ts.URL+"/api")
	ctx := app.WithTenantOrgID(context.Background(), workspaceID)
	repoID := graph.RepositoryID(root)

	if _, err := captureSnapshot(t, client, ctx, &pb.CaptureSnapshotRequest{RepositoryId: repoID}); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("commit capture without history = %v, want failed precondition", err)
	}
	// The working tree remains capturable as a manual saved point.
	snapshot, err := captureSnapshot(t, client, ctx, &pb.CaptureSnapshotRequest{RepositoryId: repoID, WorkingTree: true})
	if err != nil {
		t.Fatalf("working tree CaptureSnapshot: %v", err)
	}
	if snapshot.GetProvenance() != "manual" {
		t.Fatalf("provenance = %q, want manual", snapshot.GetProvenance())
	}
}

func TestCaptureSnapshotRejectsUnknownRepository(t *testing.T) {
	workspaceID := uuid.New()
	root, _ := gitFixture(t)
	_, ts := seedCapturedRepository(t, workspaceID, root)
	defer ts.Close()
	client := codeindexv1connect.NewCodeIndexServiceClient(ts.Client(), ts.URL+"/api")
	ctx := app.WithTenantOrgID(context.Background(), workspaceID)

	if _, err := captureSnapshot(t, client, ctx, &pb.CaptureSnapshotRequest{}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty repository id = %v, want invalid argument", err)
	}
	if _, err := captureSnapshot(t, client, ctx, &pb.CaptureSnapshotRequest{RepositoryId: "missing"}); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("unknown repository = %v, want not found", err)
	}
}
