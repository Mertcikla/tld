package store

import (
	"context"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
)

func publishEdgesFixture(t *testing.T, idx *Store, id string, build func(*graph.Graph, *pb.Snapshot, func(path, name string) *pb.CodeFact, func(string, *pb.CodeFact, *pb.CodeFact, uint32))) {
	t.Helper()
	snap := &pb.Snapshot{Id: id, RepositoryId: "repo", GitRevision: id, Provenance: "commit", IngestionStatus: "complete"}
	g := graph.NewGraph("repo", id)
	fact := func(path, name string) *pb.CodeFact {
		text := "func " + name + "() {}"
		src := &graph.Source{Path: path, Text: []byte(text), Hash: graph.Hash([]byte(text))}
		if _, exists := g.Sources[path]; !exists {
			g.Sources[path] = src
			snap.Sources = append(snap.Sources, &pb.SourceFile{Path: path, Hash: src.Hash})
		}
		return g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, name, "go", src.Anchor(0, len(text)), text, "", nil)
	}
	edge := func(path string, from, to *pb.CodeFact, off uint32) {
		g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, from.Id, to.Id, "", &pb.SourceAnchor{Path: path, StartByte: off, EndByte: off + 1}, nil)
	}
	build(g, snap, fact, edge)
	if err := idx.Publish(context.Background(), "/repo", snap, g); err != nil {
		t.Fatal(err)
	}
}

func TestFilePairCounts(t *testing.T) {
	ctx := context.Background()
	idx, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()

	publishEdgesFixture(t, idx, "snap", func(_ *graph.Graph, _ *pb.Snapshot, fact func(path, name string) *pb.CodeFact, edge func(string, *pb.CodeFact, *pb.CodeFact, uint32)) {
		a := fact("a.go", "A")
		a2 := fact("a.go", "A2")
		b := fact("b.go", "B")
		c := fact("c.go", "C")
		edge("a.go", a, b, 1)
		edge("a.go", a, b, 2)
		edge("b.go", b, c, 1)
		edge("a.go", a, a2, 1) // intra-file, excluded
	})

	counts, err := idx.FilePairCounts(ctx, "snap")
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 2 {
		t.Fatalf("file pair counts = %v, want 2 pairs", counts)
	}
	if counts[[2]string{"a.go", "b.go"}] != 2 || counts[[2]string{"b.go", "c.go"}] != 1 {
		t.Fatalf("file pair counts = %v", counts)
	}
}

// ImpactDiff keeps the source and symbol deltas but never materializes the edge
// delta: impact diagrams describe dependencies through FilePairCounts instead,
// and the full edge delta was the bulk of every stored comparison.
func TestImpactDiffOmitsEdges(t *testing.T) {
	ctx := context.Background()
	idx, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()

	publishEdgesFixture(t, idx, "base", func(_ *graph.Graph, _ *pb.Snapshot, fact func(path, name string) *pb.CodeFact, edge func(string, *pb.CodeFact, *pb.CodeFact, uint32)) {
		a := fact("a.go", "A")
		b := fact("b.go", "B")
		edge("a.go", a, b, 1)
	})
	publishEdgesFixture(t, idx, "head", func(_ *graph.Graph, _ *pb.Snapshot, fact func(path, name string) *pb.CodeFact, edge func(string, *pb.CodeFact, *pb.CodeFact, uint32)) {
		fact("a.go", "A")
		b := fact("b.go", "B")
		edge("a.go", b, b, 1)
	})

	diff, err := idx.ImpactDiff(ctx, "base", "head")
	if err != nil {
		t.Fatal(err)
	}
	if diff.GetEdgeFacts() != nil {
		t.Fatalf("impact diff carried an edge delta: %+v", diff.GetEdgeFacts())
	}
	if diff.GetFacts() == nil {
		t.Fatal("impact diff dropped the symbol delta")
	}

	full, err := idx.Diff(ctx, "base", "head", false)
	if err != nil {
		t.Fatal(err)
	}
	if full.GetEdgeFacts() == nil {
		t.Fatal("snapshot diff lost its edge delta")
	}
}
