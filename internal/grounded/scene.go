package grounded

import (
	"sort"
	"strconv"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

// FromScene projects the assembly ledger; it makes no grounding or selection
// decisions independently of the portable scene.
func FromScene(scene *pb.ImpactScene, viewFilter string, allEdges bool) Summary {
	summary := Summary{Mode: "grounded", ViewFilter: viewFilter, Views: []ViewMatch{}, Elements: []ElementStatus{}, FallbackToRaw: len(scene.GetAuthoredViewIds()) == 0}
	views := map[int64]*ViewMatch{}
	keys := make([]string, 0, len(scene.GetProvenance()))
	for key := range scene.GetProvenance() {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		p := scene.Provenance[key]
		if p.ObjectKind != "placement" || p.Origin != pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED {
			continue
		}
		view := views[p.ViewId]
		if view == nil {
			view = &ViewMatch{ViewID: p.ViewId}
			for _, candidate := range scene.Provenance {
				if candidate.ObjectKind == "view" && candidate.ViewId == p.ViewId {
					view.ViewName = candidate.Name
					break
				}
			}
			views[p.ViewId] = view
		}
		view.Total++
		status := StatusContext
		switch {
		case p.Grounding == pb.ImpactGrounding_IMPACT_GROUNDING_UNLINKED || p.Grounding == pb.ImpactGrounding_IMPACT_GROUNDING_UNRESOLVED || p.Grounding == pb.ImpactGrounding_IMPACT_GROUNDING_AMBIGUOUS:
			status = StatusUngrounded
			summary.Ungrounded++
		case p.Role == pb.ImpactRole_IMPACT_ROLE_DIRECT || p.Role == pb.ImpactRole_IMPACT_ROLE_CONTAINED:
			status = StatusAffected
			summary.Affected++
			view.Affected++
		default:
			summary.Context++
			if p.Role == pb.ImpactRole_IMPACT_ROLE_DEPENDENCY_CONTEXT {
				view.Indirect++
			}
		}
		entry := ElementStatus{ViewID: p.ViewId, ViewName: view.ViewName, ElementID: p.ResourceId, Name: p.Name, Status: status}
		if len(p.Paths) > 0 {
			entry.Link = p.Paths[0]
		}
		for _, witness := range p.Witnesses {
			if len(witness.Paths) > 1 {
				entry.Near = append(entry.Near, witness.Paths[0])
			}
		}
		summary.Elements = append(summary.Elements, entry)
	}
	ids := make([]int64, 0, len(views))
	for id := range views {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		summary.Views = append(summary.Views, *views[id])
	}
	for _, coverage := range scene.GetCoverage() {
		workspaceCovered := false
		for _, key := range coverage.PlacementKeys {
			if p := scene.Provenance[key]; p != nil && p.Origin != pb.ImpactOrigin_IMPACT_ORIGIN_RAW {
				workspaceCovered = true
			}
		}
		if !workspaceCovered {
			summary.Uncovered = append(summary.Uncovered, coverage.Path)
		}
	}
	for _, content := range scene.GetViews() {
		for _, edge := range content.Connectors {
			key := "connector|" + strconv.Itoa(int(edge.ViewId)) + "|" + strconv.Itoa(int(edge.Id))
			if p := scene.Provenance[key]; p == nil || p.Origin != pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED {
				continue
			}
			summary.Connectors = append(summary.Connectors, GroundedConnector{ViewID: int64(edge.ViewId), Source: strconv.Itoa(int(edge.SourceElementId)), Target: strconv.Itoa(int(edge.TargetElementId)), Label: edge.GetLabel()})
		}
	}
	sort.Slice(summary.Connectors, func(i, j int) bool {
		a, b := summary.Connectors[i], summary.Connectors[j]
		if a.ViewID != b.ViewID {
			return a.ViewID < b.ViewID
		}
		return a.Source+"|"+a.Target < b.Source+"|"+b.Target
	})
	if !allEdges {
		projected := &pb.ImpactDiagram{}
		seen := map[string]bool{}
		for _, content := range scene.GetViews() {
			for _, connector := range content.Connectors {
				p := scene.Provenance["connector|"+strconv.Itoa(int(connector.ViewId))+"|"+strconv.Itoa(int(connector.Id))]
				if p == nil || len(p.EndpointKeys) != 2 {
					continue
				}
				for _, key := range p.EndpointKeys {
					if !seen[key] {
						seen[key] = true
						name := key
						if endpoint := scene.Provenance[key]; endpoint != nil && len(endpoint.Paths) > 0 {
							name = endpoint.Paths[0]
						}
						projected.Nodes = append(projected.Nodes, &pb.ImpactNode{Key: key, Path: name})
					}
				}
				projected.Edges = append(projected.Edges, &pb.ImpactEdge{FromKey: p.EndpointKeys[0], ToKey: p.EndpointKeys[1]})
			}
		}
		summary.Fanout = RollupFanout(projected, MaxFanoutShown)
	}
	return summary
}
