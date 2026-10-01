package project

import (
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

func anchor(path string) *pb.SourceAnchor {
	return &pb.SourceAnchor{Path: path, StartByte: 0, EndByte: 3, SourceHash: "h"}
}

func TestProjectElementsAndConnectors(t *testing.T) {
	g := graph.NewGraph("repo", "snap")
	a := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", anchor("a.go"), "func A(){}", "func A()", &pb.Evidence{Producer: "tree-sitter"})
	b := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "B", "go", anchor("b.go"), "func B(){}", "func B()", &pb.Evidence{Producer: "tree-sitter"})
	if a == nil || b == nil {
		t.Fatal("facts not created")
	}
	if e := g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, a.Id, b.Id, "", anchor("a.go"), &pb.Evidence{Producer: "scip"}); e == nil {
		t.Fatal("edge not created")
	}
	// An edge to an unresolved external symbol and a self edge must both be dropped.
	if e := g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_REFERENCES, a.Id, "", "external.Println", anchor("a.go"), &pb.Evidence{Producer: "scip"}); e == nil {
		t.Fatal("external edge not created")
	}
	if e := g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, a.Id, a.Id, "", anchor("a.go"), &pb.Evidence{Producer: "scip"}); e == nil {
		t.Fatal("self edge not created")
	}

	snap := &pb.Snapshot{Id: "snap", RepositoryId: "repo"}
	res := Project(snap, g)

	if len(res.Elements) != 2 {
		t.Fatalf("elements = %d, want 2 (%+v)", len(res.Elements), res.Elements)
	}
	if res.Elements[0].Ref > res.Elements[1].Ref {
		t.Fatalf("elements not sorted: %q > %q", res.Elements[0].Ref, res.Elements[1].Ref)
	}
	if res.Elements[0].Name != "A" || res.Elements[0].Kind != pb.FactKind_FACT_KIND_FUNCTION || res.Elements[0].FilePath != "a.go" {
		t.Fatalf("unexpected element: %+v", res.Elements[0])
	}

	if len(res.Connectors) != 1 {
		t.Fatalf("connectors = %d, want 1 (%+v)", len(res.Connectors), res.Connectors)
	}
	c := res.Connectors[0]
	if c.Kind != pb.EdgeKind_EDGE_KIND_CALLS || c.FromRef != a.LogicalKey || c.ToRef != b.LogicalKey {
		t.Fatalf("unexpected connector: %+v", c)
	}
	if c.Weight != 1 {
		t.Fatalf("connector weight = %v, want 1", c.Weight)
	}
}

func TestProjectAggregatesRepeatedObservations(t *testing.T) {
	g := graph.NewGraph("repo", "snap")
	a := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", anchor("a.go"), "func A(){}", "func A()", &pb.Evidence{Producer: "tree-sitter"})
	b := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "B", "go", anchor("b.go"), "func B(){}", "func B()", &pb.Evidence{Producer: "tree-sitter"})
	// Two call observations: distinct anchors, same logical edge.
	src := &graph.Source{Path: "a.go", Language: "go", Text: []byte("func A(){ B(); B() }")}
	first := g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, a.Id, b.Id, "", src.Anchor(10, 13), &pb.Evidence{Producer: "scip"})
	second := g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, a.Id, b.Id, "", src.Anchor(15, 18), &pb.Evidence{Producer: "scip"})
	if first == nil || second == nil || first.LogicalKey != second.LogicalKey {
		t.Fatalf("observations did not share a logical key: %v %v", first, second)
	}

	res := Project(&pb.Snapshot{Id: "snap", RepositoryId: "repo"}, g)
	if len(res.Connectors) != 1 {
		t.Fatalf("connectors = %d, want 1 aggregated", len(res.Connectors))
	}
	if res.Connectors[0].Weight != 2 {
		t.Fatalf("aggregated weight = %v, want 2", res.Connectors[0].Weight)
	}
}

func TestProjectUnresolvedBySymbolKey(t *testing.T) {
	g := graph.NewGraph("repo", "snap")
	a := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", anchor("a.go"), "func A(){}", "func A()", &pb.Evidence{Producer: "tree-sitter"})
	b := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "B", "go", anchor("b.go"), "func B(){}", "func B()", &pb.Evidence{Producer: "tree-sitter"})
	b.SymbolKey = "example.com/pkg.B"
	// Unresolved at import time but resolvable by symbol key.
	if e := g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_REFERENCES, a.Id, "", "example.com/pkg.B", anchor("a.go"), &pb.Evidence{Producer: "scip"}); e == nil {
		t.Fatal("edge not created")
	}
	res := Project(&pb.Snapshot{Id: "snap", RepositoryId: "repo"}, g)
	if len(res.Connectors) != 1 || res.Connectors[0].ToRef != b.LogicalKey {
		t.Fatalf("symbol-key resolution failed: %+v", res.Connectors)
	}
}
