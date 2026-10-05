package layout

import "math"

// Connector handle ids understood by the view renderer.
const (
	HandleTop    = "top"
	HandleBottom = "bottom"
	HandleLeft   = "left"
	HandleRight  = "right"
)

// ChooseConnectorHandles returns the source and target handles whose node
// anchor points are closest, so a connector leaves and enters its nodes on
// facing sides instead of crossing the node bodies. It mirrors the view
// editor's "Adjust Connectors" handle optimisation.
func ChooseConnectorHandles(source, target Placement) (string, string) {
	sourceAnchors := handleAnchors(source)
	targetAnchors := handleAnchors(target)
	bestSource, bestTarget := HandleTop, HandleTop
	bestDistance := math.Inf(1)
	for _, s := range sourceAnchors {
		for _, t := range targetAnchors {
			distance := math.Hypot(s.x-t.x, s.y-t.y)
			if distance < bestDistance {
				bestDistance = distance
				bestSource, bestTarget = s.name, t.name
			}
		}
	}
	return bestSource, bestTarget
}

type handleAnchor struct {
	name string
	x, y float64
}

func handleAnchors(p Placement) []handleAnchor {
	return []handleAnchor{
		{HandleTop, p.X + NodeWidth/2, p.Y},
		{HandleBottom, p.X + NodeWidth/2, p.Y + NodeHeight},
		{HandleLeft, p.X, p.Y + NodeHeight/2},
		{HandleRight, p.X + NodeWidth, p.Y + NodeHeight/2},
	}
}
