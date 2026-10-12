package impact

import (
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
)

func scopeDiagram() *model.Diagram {
	return &model.Diagram{
		RepositoryId:  "repo",
		ComparisonKey: "key",
		Nodes: []*model.Node{
			{Key: "file|a.go", Path: "a.go", Name: "a.go", Change: pb.ChangeKind_CHANGE_KIND_MODIFIED},
			{Key: "context|1", Path: "b.go", Name: "b.go", Distance: 1},
			{Key: "context|2", Path: "c.go", Name: "c.go", Distance: 2},
		},
		Edges: []*model.Edge{
			{FromKey: "file|a.go", ToKey: "file|a.go", Weight: 1},
			{FromKey: "file|a.go", ToKey: "context|1", Weight: 2},
			{FromKey: "context|1", ToKey: "context|2", Weight: 3},
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

func nodeKeys(diagram *model.Diagram) []string {
	out := make([]string, 0, len(diagram.GetNodes()))
	for _, node := range diagram.GetNodes() {
		out = append(out, node.GetKey())
	}
	return out
}
