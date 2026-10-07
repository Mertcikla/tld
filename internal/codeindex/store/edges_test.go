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
	if edges[0].Kind != pb.EdgeKind_EDGE_KIND_CALLS || edges[1].Kind != pb.EdgeKind_EDGE_KIND_CALLS {
		t.Fatalf("edge kinds = %v/%v, want calls", edges[0].Kind, edges[1].Kind)
	}
}

func TestAggregatedFileEdgesDominantKind(t *testing.T) {
	ctx := context.Background()
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()

	root := "/repo"
	repoID := graph.RepositoryID(root)
	snap := &pb.Snapshot{Id: "snap-kind", RepositoryId: repoID, CreatedUnix: 100}
	g := graph.NewGraph(repoID, snap.Id)
	for _, id := range []string{"file-a", "file-b"} {
		g.Facts[id] = &pb.CodeFact{Id: id, RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FILE, Anchor: &pb.SourceAnchor{Path: id + ".go"}}
	}
	g.Facts["sym-a"] = &pb.CodeFact{Id: "sym-a", RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FUNCTION, Anchor: &pb.SourceAnchor{Path: "file-a.go", StartByte: 1}}
	g.Facts["sym-b"] = &pb.CodeFact{Id: "sym-b", RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FUNCTION, Anchor: &pb.SourceAnchor{Path: "file-b.go", StartByte: 2}}
	ref := g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_REFERENCES, "sym-a", "sym-b", "", &pb.SourceAnchor{Path: "file-a.go", StartByte: 1}, nil)
	call := g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, "sym-a", "sym-b", "", &pb.SourceAnchor{Path: "file-a.go", StartByte: 3}, nil)
	ref.Weight, call.Weight = 1, 5
	if err := st.Publish(ctx, root, snap, g); err != nil {
		t.Fatalf("publish: %v", err)
	}

	edges, err := st.AggregatedFileEdges(ctx, snap.Id)
	if err != nil {
		t.Fatalf("aggregated edges: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("edges = %+v, want 1", edges)
	}
	if edges[0].Kind != pb.EdgeKind_EDGE_KIND_CALLS {
		t.Fatalf("kind = %v, want calls (weight 5 > references 1)", edges[0].Kind)
	}
	if edges[0].Weight != 6 {
		t.Fatalf("weight = %v, want 6", edges[0].Weight)
	}
}

// TestFileEdgesResolveReusedFactsThroughMembership covers incremental snapshots
// whose facts are shared immutable rows written by an earlier snapshot. Their
// codeindex_facts.snapshot_id points at the base, so file-edge resolution must
// go through codeindex_snapshot_facts membership.
func TestFileEdgesResolveReusedFactsThroughMembership(t *testing.T) {
	ctx := context.Background()
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()

	root := "/repo"
	repoID := graph.RepositoryID(root)
	base := &pb.Snapshot{Id: "snap-base", RepositoryId: repoID, CreatedUnix: 100}
	g := graph.NewGraph(repoID, base.Id)
	g.Facts["file-a"] = &pb.CodeFact{Id: "file-a", RepositoryId: repoID, SnapshotId: base.Id, Kind: pb.FactKind_FACT_KIND_FILE, Anchor: &pb.SourceAnchor{Path: "a.go"}}
	g.Facts["file-b"] = &pb.CodeFact{Id: "file-b", RepositoryId: repoID, SnapshotId: base.Id, Kind: pb.FactKind_FACT_KIND_FILE, Anchor: &pb.SourceAnchor{Path: "b.go"}}
	g.Facts["sym-a"] = &pb.CodeFact{Id: "sym-a", RepositoryId: repoID, SnapshotId: base.Id, Kind: pb.FactKind_FACT_KIND_FUNCTION, Anchor: &pb.SourceAnchor{Path: "a.go"}, Imports: []string{"flask"}}
	g.Facts["sym-b"] = &pb.CodeFact{Id: "sym-b", RepositoryId: repoID, SnapshotId: base.Id, Kind: pb.FactKind_FACT_KIND_FUNCTION, Anchor: &pb.SourceAnchor{Path: "b.go"}}
	g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, "sym-a", "sym-b", "", &pb.SourceAnchor{Path: "a.go"}, nil)
	if err := st.Publish(ctx, root, base, g); err != nil {
		t.Fatalf("publish base: %v", err)
	}

	// Incremental snapshot: reuse every fact row, add one new edge.
	inc := &pb.Snapshot{Id: "snap-inc", RepositoryId: repoID, CreatedUnix: 200}
	g2 := graph.NewGraph(repoID, inc.Id)
	for id, f := range g.Facts {
		g2.Facts[id] = &pb.CodeFact{
			Id: id, RepositoryId: repoID, SnapshotId: inc.Id, Kind: f.Kind,
			Name: f.Name, Anchor: f.Anchor, Imports: f.Imports,
		}
		g2.Reused[id] = true
	}
	newEdge := g2.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, "sym-b", "sym-a", "", &pb.SourceAnchor{Path: "b.go", StartByte: 7}, nil)
	newEdge.Weight = 3
	if err := st.Publish(ctx, root, inc, g2); err != nil {
		t.Fatalf("publish incremental: %v", err)
	}

	// The reused fact rows still belong to the base snapshot.
	var reusedRows int
	if err := st.bun.QueryRowContext(ctx, `SELECT COUNT(*) FROM codeindex_facts WHERE snapshot_id = ?`, inc.Id).Scan(&reusedRows); err != nil {
		t.Fatalf("count facts: %v", err)
	}
	if reusedRows != 0 {
		t.Fatalf("incremental snapshot wrote %d fact rows, want 0 (all reused)", reusedRows)
	}

	edges, err := st.FileEdges(ctx, inc.Id)
	if err != nil {
		t.Fatalf("file edges: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("file edges = %+v, want 1", edges)
	}
	if edges[0].FromFactID != "file-b" || edges[0].ToFactID != "file-a" {
		t.Fatalf("edge = %+v, want file-b -> file-a", edges[0])
	}

	agg, err := st.AggregatedFileEdges(ctx, inc.Id)
	if err != nil {
		t.Fatalf("aggregated edges: %v", err)
	}
	if len(agg) != 1 || agg[0].FromFactID != "file-b" || agg[0].ToFactID != "file-a" || agg[0].Weight != 3 {
		t.Fatalf("aggregated = %+v, want one file-b -> file-a weight 3", agg)
	}

	imports, err := st.FileImports(ctx, inc.Id)
	if err != nil {
		t.Fatalf("file imports: %v", err)
	}
	if len(imports) != 1 || imports[0].FileFactID != "file-a" || imports[0].Import != "flask" {
		t.Fatalf("imports = %+v, want file-a:flask from reused fact", imports)
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

func TestFileEdgesReusedAcrossSnapshots(t *testing.T) {
	ctx := context.Background()
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	repo := graph.RepositoryID("/repo")
	g1 := graph.NewGraph(repo, "edges-base")
	for _, path := range []string{"a.go", "b.go"} {
		a := &pb.SourceAnchor{Path: path, StartByte: 0, EndByte: 10}
		g1.AddFact(pb.FactKind_FACT_KIND_FILE, path, "go", a, "", "", nil)
		g1.AddFact(pb.FactKind_FACT_KIND_FUNCTION, path, "go", a, "", "", nil)
	}
	var from, to string
	for _, f := range g1.Facts {
		if f.Kind == pb.FactKind_FACT_KIND_FUNCTION {
			if f.Name == "a.go" {
				from = f.Id
			} else {
				to = f.Id
			}
		}
	}
	e := g1.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, from, to, "", &pb.SourceAnchor{Path: "a.go", StartByte: 2, EndByte: 3}, nil)
	e.Weight = 2.5
	if err := st.Publish(ctx, "/repo", &pb.Snapshot{Id: g1.SnapshotID, RepositoryId: repo}, g1); err != nil {
		t.Fatal(err)
	}
	g2 := graph.NewGraph(repo, "edges-head")
	for id, f := range g1.Facts {
		g2.Facts[id] = f
		g2.Reused[id] = true
	}
	g2.EdgeFacts[e.Id] = e
	g2.Reused[e.Id] = true
	if err := st.Publish(ctx, "/repo", &pb.Snapshot{Id: g2.SnapshotID, RepositoryId: repo}, g2); err != nil {
		t.Fatal(err)
	}
	for _, load := range []struct {
		name string
		fn   func(context.Context, string) ([]FileEdge, error)
	}{{"raw", st.FileEdges}, {"aggregated", st.AggregatedFileEdges}} {
		t.Run(load.name, func(t *testing.T) {
			base, err := load.fn(ctx, g1.SnapshotID)
			if err != nil {
				t.Fatal(err)
			}
			head, err := load.fn(ctx, g2.SnapshotID)
			if err != nil {
				t.Fatal(err)
			}
			if len(base) != 1 || len(head) != 1 || base[0] != head[0] {
				t.Fatalf("reused edge missing or changed: base=%v head=%v", base, head)
			}
		})
	}
	// A third snapshot omitting the shared edge must not resurrect it.
	g3 := graph.NewGraph(repo, "edges-removed")
	for id, f := range g1.Facts {
		g3.Facts[id] = f
		g3.Reused[id] = true
	}
	if err := st.Publish(ctx, "/repo", &pb.Snapshot{Id: g3.SnapshotID, RepositoryId: repo}, g3); err != nil {
		t.Fatal(err)
	}
	edges, err := st.AggregatedFileEdges(ctx, g3.SnapshotID)
	if err != nil || len(edges) != 0 {
		t.Fatalf("removed edges returned: %v, %v", edges, err)
	}
}
