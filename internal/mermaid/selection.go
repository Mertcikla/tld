package mermaid

import (
	"strconv"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"google.golang.org/protobuf/proto"
)

// SceneMembership is Mermaid's bounded presentation policy. Hierarchy is not
// an adjacency edge: only declared connectors can bring authored context into
// the radius. Changed evidence and file representatives are always protected.
func SceneMembership(scene *pb.ImpactScene, radius uint32, plain bool) map[string]map[int32]bool {
	kept := map[string]map[int32]bool{}
	placements := map[string]*pb.ScenePlacement{}
	adj := map[string][]string{}
	distances := map[string]uint32{}
	var queue []string
	seed := func(key string, d uint32) {
		if old, ok := distances[key]; !ok || d < old {
			distances[key] = d
			queue = append(queue, key)
		}
	}
	for view, content := range scene.GetViews() {
		kept[view] = map[int32]bool{}
		for _, p := range content.Placements {
			if p.Element == nil {
				continue
			}
			key := placementKey(int64(p.Element.ViewId), int64(p.Element.ElementId))
			placements[key] = p
			switch p.Role {
			case pb.ImpactRole_IMPACT_ROLE_DIRECT, pb.ImpactRole_IMPACT_ROLE_CONTAINED, pb.ImpactRole_IMPACT_ROLE_UNSPECIFIED:
				seed(key, 0)
			case pb.ImpactRole_IMPACT_ROLE_DEPENDENCY_CONTEXT:
				if p.Distance <= radius {
					seed(key, p.Distance)
				}
			}
			if p.GetOverlay().GetChange() != pb.ChangeKind_CHANGE_KIND_UNSPECIFIED {
				seed(key, 0)
			}
		}
	}
	for _, file := range scene.GetFiles() {
		for _, ref := range file.Representatives {
			seed(placementKey(int64(ref.ViewId), int64(ref.ElementId)), 0)
		}
	}
	for view, content := range scene.GetViews() {
		viewID, _ := strconv.ParseInt(view, 10, 32)
		for _, edge := range content.Connectors {
			a := placementKey(viewID, int64(edge.SourceElementId))
			b := placementKey(viewID, int64(edge.TargetElementId))
			if placements[b] == nil {
				if ref := content.ConnectorTargets[strconv.Itoa(int(edge.Id))]; ref != nil {
					b = placementKey(int64(ref.ViewId), int64(ref.ElementId))
				}
			}
			if placements[a] == nil || placements[b] == nil {
				continue
			}
			adj[a] = append(adj[a], b)
			adj[b] = append(adj[b], a)
			if edgeChangeFromTags(edge.Tags) != "" {
				seed(a, 0)
				seed(b, 0)
			}
		}
	}
	for i := 0; i < len(queue); i++ {
		key := queue[i]
		d := distances[key]
		if d >= radius {
			continue
		}
		for _, next := range adj[key] {
			p := placements[next]
			if p == nil || p.Role == pb.ImpactRole_IMPACT_ROLE_CONTAINER {
				continue
			}
			if plain && p.Role == pb.ImpactRole_IMPACT_ROLE_AUTHORED_CONTEXT {
				continue
			}
			if p.Role == pb.ImpactRole_IMPACT_ROLE_DEPENDENCY_CONTEXT && p.Distance > radius {
				continue
			}
			seed(next, d+1)
		}
	}
	for key, d := range distances {
		if p := placements[key]; p != nil && d <= radius {
			kept[strconv.Itoa(int(p.Element.ViewId))][p.Element.ElementId] = true
		}
	}
	return kept
}

func placementKey(view, element int64) string {
	return strconv.FormatInt(view, 10) + "|" + strconv.FormatInt(element, 10)
}

// SelectImpactScene projects a wire scene for server and offline exports.
func SelectImpactScene(scene *pb.ImpactScene, radius uint32, plain bool) *pb.ImpactScene {
	if scene == nil {
		return nil
	}
	kept := SceneMembership(scene, radius, plain)
	out := proto.Clone(scene).(*pb.ImpactScene)
	for view, content := range out.Views {
		content.Placements = nil
		for _, p := range scene.Views[view].Placements {
			if p.Element != nil && kept[view][p.Element.ElementId] {
				content.Placements = append(content.Placements, proto.Clone(p).(*pb.ScenePlacement))
			}
		}
		content.Connectors = nil
		for _, e := range scene.Views[view].Connectors {
			target := view
			if !kept[view][e.TargetElementId] {
				if ref := content.ConnectorTargets[strconv.Itoa(int(e.Id))]; ref != nil {
					target = strconv.Itoa(int(ref.ViewId))
				}
			}
			if kept[view][e.SourceElementId] && kept[target][e.TargetElementId] {
				content.Connectors = append(content.Connectors, proto.Clone(e).(*diagv1.Connector))
			} else {
				delete(content.ConnectorTargets, strconv.Itoa(int(e.Id)))
			}
		}
	}
	var prune func([]*diagv1.View) []*diagv1.View
	relevant := map[int32]bool{}
	prune = func(views []*diagv1.View) []*diagv1.View {
		var result []*diagv1.View
		for _, v := range views {
			v.Children = prune(v.Children)
			if len(kept[strconv.Itoa(int(v.Id))]) > 0 || len(v.Children) > 0 {
				result = append(result, v)
				relevant[v.Id] = true
			}
		}
		return result
	}
	out.Tree = prune(out.Tree)
	for key := range out.Views {
		id, _ := strconv.ParseInt(key, 10, 32)
		if len(kept[key]) == 0 && !relevant[int32(id)] {
			delete(out.Views, key)
		}
	}
	if out.Comparison != nil {
		out.Comparison.EffectiveRadius = radius
		out.Comparison.Plain = plain
	}
	if out.Summary != nil {
		s := out.Summary
		s.AffectedPlacements = 0
		s.DependencyNeighbours = 0
		s.AuthoredContext = 0
		s.Containers = 0
		s.Ungrounded = 0
		for _, view := range out.Views {
			for _, p := range view.Placements {
				switch p.Role {
				case pb.ImpactRole_IMPACT_ROLE_DIRECT, pb.ImpactRole_IMPACT_ROLE_CONTAINED:
					s.AffectedPlacements++
				case pb.ImpactRole_IMPACT_ROLE_DEPENDENCY_CONTEXT:
					s.DependencyNeighbours++
				case pb.ImpactRole_IMPACT_ROLE_AUTHORED_CONTEXT:
					s.AuthoredContext++
				case pb.ImpactRole_IMPACT_ROLE_CONTAINER:
					s.Containers++
				}
				if p.Grounding == pb.ImpactGrounding_IMPACT_GROUNDING_UNLINKED || p.Grounding == pb.ImpactGrounding_IMPACT_GROUNDING_UNRESOLVED || p.Grounding == pb.ImpactGrounding_IMPACT_GROUNDING_AMBIGUOUS {
					s.Ungrounded++
				}
			}
		}
	}
	return out
}
