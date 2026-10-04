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

func TestCompareRepositoryRadiusNoCheckout(t *testing.T) {
	ctx := context.Background()
	ws, routes := newTestServer(t, uuid.New(), nil)
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	root := "/missing/repo"
	repoID := graph.RepositoryID(root)
	for _, id := range []string{"base", "head"} {
		text := "func A() { return " + id + " }"
		g := graph.NewGraph(repoID, id)
		src := &graph.Source{Path: "a.go", Text: []byte(text), Hash: graph.Hash([]byte(text)), Language: "go"}
		g.Sources[src.Path] = src
		g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", src.Anchor(0, len(text)), text, "", nil)
		snap := &pb.Snapshot{Id: id, RepositoryId: repoID, IngestionStatus: "complete", Sources: []*pb.SourceFile{{Path: "a.go", Hash: src.Hash, Size: uint64(len(text))}}}
		if err := idx.Publish(ctx, root, snap, g); err != nil {
			t.Fatal(err)
		}
	}
	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewMapperServiceClient(ts.Client(), ts.URL+"/api")
	stream, err := client.CompareRepository(ctx, connect.NewRequest(&pb.CompareRepositoryRequest{RepositoryId: repoID, Base: &pb.ComparisonTarget{SnapshotId: "base"}, Head: &pb.ComparisonTarget{SnapshotId: "head"}}))
	if err != nil {
		t.Fatal(err)
	}
	var result *pb.ImpactDiagram
	for stream.Receive() {
		if found := stream.Msg().GetResult(); found != nil {
			result = found
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if result == nil || result.ViewId != 0 || len(result.Nodes) != 1 || len(result.Nodes[0].Symbols.Modified) != 1 {
		t.Fatalf("comparison: %+v", result)
	}
	radius, err := client.SetImpactRadius(ctx, connect.NewRequest(&pb.SetImpactRadiusRequest{RepositoryId: repoID, ComparisonKey: result.ComparisonKey, Radius: 3}))
	if err != nil {
		t.Fatal(err)
	}
	if radius.Msg.ViewId != result.ViewId {
		t.Fatal("radius duplicated view")
	}
	snapshots, err := idx.Snapshots(ctx, repoID)
	if err != nil || len(snapshots) != 2 {
		t.Fatal("radius indexed sources")
	}
}
