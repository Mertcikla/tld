package impact

import (
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

func scopeDiagram() *pb.ImpactDiagram {
	return &pb.ImpactDiagram{
		RepositoryId:  "repo",
		ComparisonKey: "key",
		Nodes: []*pb.ImpactNode{
			{Key: "file|a.go", Path: "a.go", Name: "a.go", Change: pb.ChangeKind_CHANGE_KIND_MODIFIED},
			{Key: "context|1", Path: "b.go", Name: "b.go", Distance: 1, ElementId: 1},
			{Key: "context|2", Path: "c.go", Name: "c.go", Distance: 2, ElementId: 2},
		},
		Edges: []*pb.ImpactEdge{
			{FromKey: "file|a.go", ToKey: "file|a.go", Weight: 1},
			{FromKey: "file|a.go", ToKey: "context|1", Weight: 2},
			{FromKey: "context|1", ToKey: "context|2", Weight: 3},
		},
		Groups: []*pb.ImpactGroup{
			{Key: "view:1", Name: "repo", NodeKeys: []string{"file|a.go"}, Children: []*pb.ImpactGroup{
				{Key: "view:2", Name: "pkg", NodeKeys: []string{"context|1", "context|2"}},
			}},
		},
	}
}

func TestScopeKeepsDirectChangesAndRadiusNeighbours(t *testing.T) {
	scoped := Scope(scopeDiagram(), 1)
	if got := nodeKeys(scoped); len(got) != 2 {
		t.Fatalf("scoped nodes = %v", got)
	}
	if len(scoped.GetEdges()) != 2 {
		t.Fatalf("scoped edges = %+v", scoped.GetEdges())
	}
	if len(scoped.GetGroups()) != 1 || len(scoped.GetGroups()[0].GetChildren()) != 1 {
		t.Fatalf("scoped groups = %+v", scoped.GetGroups())
	}
	if keys := scoped.GetGroups()[0].GetChildren()[0].GetNodeKeys(); len(keys) != 1 || keys[0] != "context|1" {
		t.Fatalf("scoped group keys = %v", keys)
	}
}

func TestScopeDropsEmptyGroups(t *testing.T) {
	scoped := Scope(scopeDiagram(), 0)
	if len(scoped.GetGroups()) != 1 || len(scoped.GetGroups()[0].GetChildren()) != 0 {
		t.Fatalf("scoped groups = %+v", scoped.GetGroups())
	}
	empty := Scope(scopeDiagram(), 0)
	empty.Nodes = nil
	if groups := scopeGroups(empty.GetGroups(), map[string]bool{}); len(groups) != 0 {
		t.Fatalf("empty groups = %+v", groups)
	}
}

func TestFitRadiusNarrowsToBudget(t *testing.T) {
	diagram := scopeDiagram()
	if radius, limited := FitRadius(diagram, 2, 10); radius != 2 || limited {
		t.Fatalf("fits: radius=%d limited=%v", radius, limited)
	}
	if radius, limited := FitRadius(diagram, 2, 2); radius != 1 || !limited {
		t.Fatalf("narrow: radius=%d limited=%v", radius, limited)
	}
	if radius, limited := FitRadius(diagram, 3, 0); radius != 3 || limited {
		t.Fatalf("disabled: radius=%d limited=%v", radius, limited)
	}
}

func nodeKeys(diagram *pb.ImpactDiagram) []string {
	out := make([]string, 0, len(diagram.GetNodes()))
	for _, node := range diagram.GetNodes() {
		out = append(out, node.GetKey())
	}
	return out
}
