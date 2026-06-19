package mermaid

import (
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
)

func TestLayoutImportPreservesMetadataPositions(t *testing.T) {
	t.Parallel()

	x1, y1 := 120.0, 80.0
	x2, y2 := 460.0, 240.0
	parsed := &ParsedDiagram{
		Direction: DirectionLR,
		Elements: []*diagv1.PlanElement{
			{Ref: "node_1", Name: "API", Placements: []*diagv1.PlanViewPlacement{{ParentRef: "root", PositionX: &x1, PositionY: &y1}}},
			{Ref: "node_2", Name: "DB", Placements: []*diagv1.PlanViewPlacement{{ParentRef: "root", PositionX: &x2, PositionY: &y2}}},
		},
		Connectors: []*diagv1.PlanConnector{{SourceElementRef: "node_1", TargetElementRef: "node_2"}},
	}

	positions := LayoutImport(parsed, Point{X: 900, Y: 700})
	if got := positions["node_1"]; got != (Point{X: x1, Y: y1}) {
		t.Fatalf("node_1 position = %+v, want metadata position %f/%f", got, x1, y1)
	}
	if got := positions["node_2"]; got != (Point{X: x2, Y: y2}) {
		t.Fatalf("node_2 position = %+v, want metadata position %f/%f", got, x2, y2)
	}
}
