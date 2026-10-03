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
