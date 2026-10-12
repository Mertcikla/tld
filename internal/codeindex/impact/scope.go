package impact

import (
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
)

// DiagramRadius recovers the retained context from a cached diagram, including
// older artifacts. Scene exports must preserve it instead of applying a default.
func DiagramRadius(diagram *model.Diagram) uint32 {
	if diagram != nil && diagram.RequestedRadius != nil {
		return diagram.GetRequestedRadius()
	}
	var radius uint32
	for _, node := range diagram.GetNodes() {
		if node.GetDistance() > radius {
			radius = node.GetDistance()
		}
	}
	return radius
}

// Scope filters a comparison diagram down to the requested blast radius.
// Direct changes (distance 0) are always kept; unchanged context farther than
// radius is dropped, along with edges that reference it.
func Scope(diagram *model.Diagram, radius uint32) *model.Diagram {
	if diagram == nil {
		return nil
	}
	scoped := model.Clone(diagram)
	nodes := make([]*model.Node, 0, len(scoped.GetNodes()))
	kept := map[string]bool{}
	for _, node := range scoped.GetNodes() {
		if node == nil || node.GetDistance() > radius {
			continue
		}
		nodes = append(nodes, node)
		kept[node.GetKey()] = true
	}
	scoped.Nodes = nodes

	edges := make([]*model.Edge, 0, len(scoped.GetEdges()))
	for _, edge := range scoped.GetEdges() {
		if edge == nil || !kept[edge.GetFromKey()] || !kept[edge.GetToKey()] {
			continue
		}
		edges = append(edges, edge)
	}
	scoped.Edges = edges
	if scoped.Scene != nil {
		scoped.Scene = SelectScene(scoped.Scene, radius, scoped.Scene.GetComparison().GetPlain())
	}
	return scoped
}

// FitRadius picks the widest blast radius at or below maxRadius whose node
// count fits maxNodes. limited reports whether the widest radius had to be
// narrowed. Direct changes cannot be narrowed away, so a diagram whose direct
// changes alone exceed the budget is returned at radius 0 with limited true.
func FitRadius(diagram *model.Diagram, maxRadius uint32, maxNodes int) (radius uint32, limited bool) {
	if diagram == nil {
		return maxRadius, false
	}
	if maxNodes <= 0 {
		return maxRadius, false
	}
	count := func(r uint32) int {
		if diagram.Scene != nil {
			return SceneNodeCountInRadius(diagram.Scene, r, false)
		}
		total := 0
		for _, node := range diagram.GetNodes() {
			if node != nil && node.GetDistance() <= r {
				total++
			}
		}
		return total
	}
	if count(maxRadius) <= maxNodes {
		return maxRadius, false
	}
	for r := int(maxRadius) - 1; r > 0; r-- {
		if count(uint32(r)) <= maxNodes {
			return uint32(r), true
		}
	}
	return 0, true
}
