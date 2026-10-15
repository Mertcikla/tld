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
	"github.com/mertcikla/codeindex/indexer"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/pkg/app"
)

func TestIncompatibleSnapshotsAreListedButCannotBeUsed(t *testing.T) {
	org := uuid.New()
	ws, routes := newTestServer(t, org, nil)
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	ctx := app.WithTenantOrgID(context.Background(), org)
	for _, id := range []string{"current", "legacy"} {
		snap := &pb.Snapshot{Id: id, RepositoryId: "repo", IngestionStatus: "complete"}
		if id == "current" {
			snap.ToolVersions = indexer.SnapshotVersions()
		}
		if err := idx.Publish(ctx, "/repo", snap, graph.NewGraph("repo", id)); err != nil {
			t.Fatal(err)
		}
	}
	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewCodeIndexServiceClient(ts.Client(), ts.URL+"/api")
	listed, err := client.ListSnapshots(ctx, connect.NewRequest(&pb.ID{Id: "repo"}))
	if err != nil || len(listed.Msg.Snapshots) != 2 {
		t.Fatalf("list: %v: %v", listed, err)
	}
	for _, snap := range listed.Msg.Snapshots {
		if snap.Compatible != (snap.Id == "current") || (snap.IncompatibilityReason != "") != (snap.Id == "legacy") {
			t.Fatalf("compatibility metadata: %v", snap)
		}
	}
	// Both an explicit snapshot and the repository's default latest target
	// must fail with the same actionable precondition error.
	for _, revision := range []*pb.Revision{nil, {SnapshotId: "legacy"}} {
		stream, err := client.MapRepository(ctx, connect.NewRequest(&pb.MapRepositoryRequest{RepositoryId: "repo", Revision: revision}))
		if err == nil {
			for stream.Receive() {
				if stream.Msg().GetResult() != nil {
					t.Fatal("legacy snapshot was mapped")
				}
			}
			err = stream.Err()
		}
		if connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Fatalf("map error = %v", err)
		}
	}
	compare, err := client.CompareRepository(ctx, connect.NewRequest(&pb.CompareRepositoryRequest{
		RepositoryId: "repo", Base: &pb.Revision{SnapshotId: "legacy"}, Head: &pb.Revision{SnapshotId: "current"},
	}))
	if err == nil {
		for compare.Receive() {
			if compare.Msg().GetResult() != nil {
				t.Fatal("legacy snapshot was compared")
			}
		}
		err = compare.Err()
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("compare error = %v", err)
	}
	if _, err := client.DiffSnapshots(ctx, connect.NewRequest(&pb.SnapshotDiffRequest{FromSnapshotId: "legacy", ToSnapshotId: "current"})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("diff error = %v", err)
	}
	if _, err := client.DeleteSnapshot(ctx, connect.NewRequest(&pb.ID{Id: "legacy"})); err != nil {
		t.Fatalf("legacy snapshot must remain deletable: %v", err)
	}
}
