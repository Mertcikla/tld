package store

import (
	"context"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

func TestFileEdgesResolveSymbolsToFiles(t *testing.T) {
	ctx := context.Background()
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()

	root := "/repo"
	repoID := graph.RepositoryID(root)
	snap := &pb.Snapshot{Id: "snap-1", RepositoryId: repoID, CreatedUnix: 100}
	g := graph.NewGraph(repoID, snap.Id)
	g.Facts["file-a"] = &pb.CodeFact{Id: "file-a", RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FILE, Anchor: &pb.SourceAnchor{Path: "a.go"}}
	g.Facts["file-b"] = &pb.CodeFact{Id: "file-b", RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FILE, Anchor: &pb.SourceAnchor{Path: "b.go"}}
	g.Facts["sym-a"] = &pb.CodeFact{Id: "sym-a", RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FUNCTION, Anchor: &pb.SourceAnchor{Path: "a.go"}}
	g.Facts["sym-b"] = &pb.CodeFact{Id: "sym-b", RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FUNCTION, Anchor: &pb.SourceAnchor{Path: "b.go"}}
	g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, "sym-a", "sym-b", "", &pb.SourceAnchor{Path: "a.go"}, nil)
	// Intra-file edge must be dropped.
	g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, "sym-a", "sym-a", "", &pb.SourceAnchor{Path: "a.go"}, nil)
	if err := st.Publish(ctx, root, snap, g); err != nil {
		t.Fatalf("publish: %v", err)
	}

	edges, err := st.FileEdges(ctx, snap.Id)
	if err != nil {
		t.Fatalf("file edges: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("edges = %+v, want 1", edges)
	}
	if edges[0].FromFactID != "file-a" || edges[0].ToFactID != "file-b" {
		t.Fatalf("edge = %+v, want file-a -> file-b", edges[0])
	}
}

func TestAggregatedFileEdges(t *testing.T) {
	ctx := context.Background()
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()

	root := "/repo"
	repoID := graph.RepositoryID(root)
	snap := &pb.Snapshot{Id: "snap-agg", RepositoryId: repoID, CreatedUnix: 100}
	g := graph.NewGraph(repoID, snap.Id)
	for _, id := range []string{"file-a", "file-b"} {
		g.Facts[id] = &pb.CodeFact{Id: id, RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FILE, Anchor: &pb.SourceAnchor{Path: id + ".go"}}
	}
	g.Facts["sym-a1"] = &pb.CodeFact{Id: "sym-a1", RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FUNCTION, Anchor: &pb.SourceAnchor{Path: "file-a.go", StartByte: 1}}
	g.Facts["sym-a2"] = &pb.CodeFact{Id: "sym-a2", RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FUNCTION, Anchor: &pb.SourceAnchor{Path: "file-a.go", StartByte: 2}}
	g.Facts["sym-b1"] = &pb.CodeFact{Id: "sym-b1", RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FUNCTION, Anchor: &pb.SourceAnchor{Path: "file-b.go", StartByte: 3}}
	g.Facts["sym-b2"] = &pb.CodeFact{Id: "sym-b2", RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FUNCTION, Anchor: &pb.SourceAnchor{Path: "file-b.go", StartByte: 4}}
	a1 := g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, "sym-a1", "sym-b1", "", &pb.SourceAnchor{Path: "file-a.go", StartByte: 1}, nil)
	a2 := g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, "sym-a2", "sym-b2", "", &pb.SourceAnchor{Path: "file-a.go", StartByte: 2}, nil)
	b1 := g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, "sym-b1", "sym-a1", "", &pb.SourceAnchor{Path: "file-b.go", StartByte: 3}, nil)
	a1.Weight, a2.Weight, b1.Weight = 1.5, 2.5, 4
	if err := st.Publish(ctx, root, snap, g); err != nil {
		t.Fatalf("publish: %v", err)
	}

	edges, err := st.AggregatedFileEdges(ctx, snap.Id)
	if err != nil {
		t.Fatalf("aggregated edges: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("edges = %+v, want 2", edges)
	}
	if edges[0].FromFactID != "file-a" || edges[0].ToFactID != "file-b" || edges[0].Weight != 4 {
		t.Fatalf("forward edge = %+v, want file-a -> file-b weight 4", edges[0])
	}
	if edges[1].FromFactID != "file-b" || edges[1].ToFactID != "file-a" || edges[1].Weight != 4 {
		t.Fatalf("backward edge = %+v, want file-b -> file-a weight 4", edges[1])
	}
}

func TestFileImportsDedupeAndSkipRelative(t *testing.T) {
	ctx := context.Background()
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()

	root := "/repo"
	repoID := graph.RepositoryID(root)
	snap := &pb.Snapshot{Id: "snap-1", RepositoryId: repoID, CreatedUnix: 100}
	g := graph.NewGraph(repoID, snap.Id)
	g.Facts["file-a"] = &pb.CodeFact{Id: "file-a", RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FILE, Anchor: &pb.SourceAnchor{Path: "a.go"}}
	g.Facts["sym-a1"] = &pb.CodeFact{Id: "sym-a1", RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FUNCTION, Anchor: &pb.SourceAnchor{Path: "a.go"}, Imports: []string{"celery", "flask", ".", "..pkg", "./rel"}}
	g.Facts["sym-a2"] = &pb.CodeFact{Id: "sym-a2", RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FUNCTION, Anchor: &pb.SourceAnchor{Path: "a.go"}, Imports: []string{"flask", "os"}}
	if err := st.Publish(ctx, root, snap, g); err != nil {
		t.Fatalf("publish: %v", err)
	}

	imports, err := st.FileImports(ctx, snap.Id)
	if err != nil {
		t.Fatalf("file imports: %v", err)
	}
	got := make([]string, 0, len(imports))
	for _, item := range imports {
		if item.FileFactID != "file-a" {
			t.Fatalf("import resolved to %q, want file-a", item.FileFactID)
		}
		got = append(got, item.Import)
	}
	want := []string{"celery", "flask", "os"}
	if len(got) != len(want) {
		t.Fatalf("imports = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("imports = %v, want %v", got, want)
		}
	}
}
