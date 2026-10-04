package layout

import "testing"

func TestChooseConnectorHandlesFacesNodes(t *testing.T) {
	cases := []struct {
		name         string
		source       Placement
		target       Placement
		sourceHandle string
		targetHandle string
	}{
		{name: "right", source: Placement{X: 0, Y: 0}, target: Placement{X: 400, Y: 0}, sourceHandle: HandleRight, targetHandle: HandleLeft},
		{name: "left", source: Placement{X: 400, Y: 0}, target: Placement{X: 0, Y: 0}, sourceHandle: HandleLeft, targetHandle: HandleRight},
		{name: "below", source: Placement{X: 0, Y: 0}, target: Placement{X: 0, Y: 400}, sourceHandle: HandleBottom, targetHandle: HandleTop},
		{name: "above", source: Placement{X: 0, Y: 400}, target: Placement{X: 0, Y: 0}, sourceHandle: HandleTop, targetHandle: HandleBottom},
		{name: "diagonal", source: Placement{X: 0, Y: 0}, target: Placement{X: 400, Y: 300}, sourceHandle: HandleRight, targetHandle: HandleLeft},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sourceHandle, targetHandle := ChooseConnectorHandles(tc.source, tc.target)
			if sourceHandle != tc.sourceHandle || targetHandle != tc.targetHandle {
				t.Fatalf("handles = %s/%s, want %s/%s", sourceHandle, targetHandle, tc.sourceHandle, tc.targetHandle)
			}
		})
	}
}
