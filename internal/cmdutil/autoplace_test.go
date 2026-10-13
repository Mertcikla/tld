package cmdutil

import (
	"math"
	"testing"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

func workspaceWith(elements map[string]*workspace.Element, connectors map[string]*workspace.Connector) *workspace.Workspace {
	if connectors == nil {
		connectors = map[string]*workspace.Connector{}
	}
	return &workspace.Workspace{Elements: elements, Connectors: connectors}
}

func TestAutoPlace_FirstElementAtOrigin(t *testing.T) {
	ws := workspaceWith(map[string]*workspace.Element{}, nil)
	x, y, ok := AutoPlace(ws, "root", "a", nil)
	if !ok {
		t.Fatal("expected a placement")
	}
	if x != 0 || y != 0 {
		t.Fatalf("first element placed at (%v, %v), want (0, 0)", x, y)
	}
}

func TestAutoPlace_AvoidsExistingPlacement(t *testing.T) {
	ws := workspaceWith(map[string]*workspace.Element{
		"a": {Name: "a", Placements: []workspace.ViewPlacement{{ParentRef: "root", PositionX: 0, PositionY: 0}}},
	}, nil)
	x, y, ok := AutoPlace(ws, "root", "b", nil)
	if !ok {
		t.Fatal("expected a placement")
	}
	if x == 0 && y == 0 {
		t.Fatalf("new element overlapped the existing one at (%v, %v)", x, y)
	}
}

func TestAutoPlace_AnchorsToConnectorNeighbor(t *testing.T) {
	ws := workspaceWith(map[string]*workspace.Element{
		"a": {Name: "a", Placements: []workspace.ViewPlacement{{ParentRef: "root", PositionX: 0, PositionY: 0}}},
	}, map[string]*workspace.Connector{
		"root/a~b": {View: "root", Source: "a", Target: "b"},
	})
	x, y, ok := AutoPlace(ws, "root", "b", nil)
	if !ok {
		t.Fatal("expected a placement")
	}
	distance := math.Hypot(x, y)
	if distance == 0 || distance > 300 {
		t.Fatalf("connector neighbor expected adjacent, got (%v, %v) distance %v", x, y, distance)
	}
}

func TestFindPlacement_NormalizesEmptyParent(t *testing.T) {
	ws := workspaceWith(map[string]*workspace.Element{
		"a": {Name: "a", Placements: []workspace.ViewPlacement{{ParentRef: "root", PositionX: 12, PositionY: 34}}},
	}, nil)
	got := FindPlacement(ws, "a", "")
	if got == nil || got.PositionX != 12 || got.PositionY != 34 {
		t.Fatalf("expected to find root placement via empty parent, got %+v", got)
	}
}
