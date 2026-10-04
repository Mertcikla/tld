package layout

import (
	"reflect"
	"testing"
)

func TestDeterministicPlacementLayoutReproducible(t *testing.T) {
	targets := map[int64]struct{}{3: {}, 1: {}, 2: {}, 4: {}}
	connectors := []Connector{
		{Source: 1, Target: 2},
		{Source: 2, Target: 3},
		{Source: 3, Target: 4},
		{Source: 4, Target: 1},
	}
	first := DeterministicPlacementLayout(targets, connectors)
	second := DeterministicPlacementLayout(targets, connectors)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("layout is not reproducible:\n%+v\n%+v", first, second)
	}
	for id := range targets {
		if _, ok := first[id]; !ok {
			t.Fatalf("target %d has no placement", id)
		}
	}
}

func TestDeterministicPlacementLayoutPlacesConnectedNodesNearby(t *testing.T) {
	targets := map[int64]struct{}{1: {}, 2: {}, 3: {}, 4: {}, 5: {}, 6: {}}
	connectors := []Connector{
		{Source: 1, Target: 2},
		{Source: 2, Target: 3},
		{Source: 4, Target: 5},
		{Source: 5, Target: 6},
	}
	positions := DeterministicPlacementLayout(targets, connectors)
	linked := PlacementDistance(positions[1], positions[2]) + PlacementDistance(positions[4], positions[5])
	unlinked := PlacementDistance(positions[1], positions[5]) + PlacementDistance(positions[2], positions[4])
	if linked >= unlinked {
		t.Fatalf("linked nodes are not placed closer than unlinked nodes: linked=%f unlinked=%f", linked, unlinked)
	}
}

func TestReducePlacementCrossings(t *testing.T) {
	a := &Node{ID: 1}
	b := &Node{ID: 2}
	c := &Node{ID: 3}
	d := &Node{ID: 4}
	byLevel := map[int][]*Node{0: {a, b}, 1: {c, d}}
	// a links to d and b links to c, so c before d crosses; d must come first.
	connectors := []Connector{{Source: 1, Target: 4}, {Source: 2, Target: 3}}
	ReducePlacementCrossings(byLevel, []int{0, 1}, connectors)
	got := byLevel[1]
	if got[0].ID != 4 || got[1].ID != 3 {
		t.Fatalf("level order = [%d %d], want [4 3]", got[0].ID, got[1].ID)
	}
}

func TestDeterministicLayoutPlacementsKeepsPreservedPositions(t *testing.T) {
	preserved := Placement{ElementID: 9, X: 1234, Y: 5678}
	targets := map[int64]struct{}{1: {}, 2: {}}
	connectors := []Connector{{Source: 1, Target: 2}}
	next := DeterministicLayoutPlacements([]Placement{preserved}, targets, connectors)
	if _, ok := next[9]; ok {
		t.Fatal("preserved element was relaid out")
	}
	if _, ok := next[1]; !ok {
		t.Fatal("new element was not placed")
	}
	if _, ok := next[2]; !ok {
		t.Fatal("new element was not placed")
	}
}
