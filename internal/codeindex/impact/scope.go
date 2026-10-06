package impact

import (
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"google.golang.org/protobuf/proto"
)

// Scope filters a comparison diagram down to the requested blast radius.
// Direct changes (distance 0) are always kept; unchanged context farther than
// radius is dropped, along with edges and group entries that reference it.
func Scope(diagram *pb.ImpactDiagram, radius uint32) *pb.ImpactDiagram {
	if diagram == nil {
		return nil
	}
	scoped := proto.Clone(diagram).(*pb.ImpactDiagram)
	nodes := make([]*pb.ImpactNode, 0, len(scoped.GetNodes()))
	kept := map[string]bool{}
	for _, node := range scoped.GetNodes() {
		if node == nil || node.GetDistance() > radius {
			continue
		}
		nodes = append(nodes, node)
		kept[node.GetKey()] = true
	}
	scoped.Nodes = nodes

	edges := make([]*pb.ImpactEdge, 0, len(scoped.GetEdges()))
	for _, edge := range scoped.GetEdges() {
		if edge == nil || !kept[edge.GetFromKey()] || !kept[edge.GetToKey()] {
			continue
		}
		edges = append(edges, edge)
	}
	scoped.Edges = edges
	scoped.Groups = scopeGroups(scoped.GetGroups(), kept)
	return scoped
}

func scopeGroups(groups []*pb.ImpactGroup, kept map[string]bool) []*pb.ImpactGroup {
	out := make([]*pb.ImpactGroup, 0, len(groups))
	for _, group := range groups {
		if group == nil {
			continue
		}
		children := scopeGroups(group.GetChildren(), kept)
		keys := make([]string, 0, len(group.GetNodeKeys()))
		for _, key := range group.GetNodeKeys() {
			if kept[key] {
				keys = append(keys, key)
			}
		}
		if len(keys) == 0 && len(children) == 0 {
			continue
		}
		clone := proto.Clone(group).(*pb.ImpactGroup)
		clone.NodeKeys = keys
		clone.Children = children
		out = append(out, clone)
	}
	return out
}

// FitRadius picks the widest blast radius at or below maxRadius whose node
// count fits maxNodes. limited reports whether the widest radius had to be
// narrowed. Direct changes cannot be narrowed away, so a diagram whose direct
// changes alone exceed the budget is returned at radius 0 with limited true.
func FitRadius(diagram *pb.ImpactDiagram, maxRadius uint32, maxNodes int) (radius uint32, limited bool) {
	if diagram == nil {
		return maxRadius, false
	}
	if maxNodes <= 0 {
		return maxRadius, false
	}
	count := func(r uint32) int {
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
