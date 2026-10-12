package impact

import (
	"fmt"
	"strconv"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"google.golang.org/protobuf/proto"
)

func SceneNodeCount(scene *pb.ImpactScene) int {
	count := 0
	for _, content := range scene.GetViews() {
		count += len(content.Placements)
	}
	return count
}

// SceneNodeCountInRadius counts the placements SelectScene would retain at the
// given radius and mode without cloning the scene payload, so budget narrowing
// stays cheap on multi-megabyte diagrams.
func SceneNodeCountInRadius(scene *pb.ImpactScene, radius uint32, plain bool) int {
	if scene == nil {
		return 0
	}
	kept := sceneKept(scene, radius, plain)
	total := 0
	for key, content := range scene.Views {
		for _, placement := range content.Placements {
			if kept[key][placement.Element.ElementId] {
				total++
			}
		}
	}
	return total
}

// sceneKept decides which placements the display policy retains: placements
// with provenance, dependency context within radius, and authored context
// unless Plain mode hides it. Containers that still lead to visible child
// evidence are retained to a fixpoint through the navigation graph.
func sceneKept(scene *pb.ImpactScene, radius uint32, plain bool) map[string]map[int32]bool {
	kept := map[string]map[int32]bool{}
	for key, content := range scene.Views {
		set := map[int32]bool{}
		for _, placement := range content.Placements {
			element := placement.Element
			p := scene.Provenance[PlacementKey(int64(element.ViewId), int64(element.ElementId))]
			keep := p != nil
			if p != nil && p.Role == pb.ImpactRole_IMPACT_ROLE_DEPENDENCY_CONTEXT && p.Distance > radius {
				keep = false
			}
			if plain && p != nil && p.Role == pb.ImpactRole_IMPACT_ROLE_AUTHORED_CONTEXT {
				keep = false
			}
			if keep {
				set[element.ElementId] = true
			}
		}
		kept[key] = set
	}
	for changed := true; changed; {
		changed = false
		for _, link := range scene.Navigations {
			if link.RelationType != "child" || link.ElementId == nil {
				continue
			}
			from, to := strconv.Itoa(int(link.FromViewId)), strconv.Itoa(int(link.ToViewId))
			if len(kept[to]) == 0 || kept[from] == nil || kept[from][*link.ElementId] {
				continue
			}
			kept[from][*link.ElementId] = true
			changed = true
		}
	}
	return kept
}

// SelectScene applies display/budget policy to a frozen scene. Authored context
// is kept in Standard mode; every suppression is explained in the trace.
func SelectScene(scene *pb.ImpactScene, radius uint32, plain bool) *pb.ImpactScene {
	if scene == nil {
		return nil
	}
	out := proto.Clone(scene).(*pb.ImpactScene)
	out.EffectiveRadius = radius
	kept := sceneKept(out, radius, plain)
	for _, key := range sortedKeys(out.Views) {
		content := out.Views[key]
		for _, placement := range content.Placements {
			element := placement.Element
			if kept[key][element.ElementId] {
				continue
			}
			p := out.Provenance[PlacementKey(int64(element.ViewId), int64(element.ElementId))]
			reason := "Outside effective dependency radius"
			if plain && p != nil && p.Role == pb.ImpactRole_IMPACT_ROLE_AUTHORED_CONTEXT {
				reason = "Hidden authored context in Plain mode"
			}
			if p != nil {
				out.OmittedContext = append(out.OmittedContext, p.Paths...)
				selectionDecision(out, p.Key, reason, p.Paths)
				delete(out.Provenance, p.Key)
			}
		}
	}
	global := map[int32]bool{}
	for _, set := range kept {
		for id := range set {
			global[id] = true
		}
	}
	for _, key := range sortedKeys(out.Views) {
		content := out.Views[key]
		var placements []*pb.ScenePlacement
		for _, placement := range content.Placements {
			if kept[key][placement.Element.ElementId] {
				placements = append(placements, placement)
			}
		}
		content.Placements = placements
		var connectors []*diagv1.Connector
		for _, edge := range content.Connectors {
			if kept[key][edge.SourceElementId] && global[edge.TargetElementId] {
				connectors = append(connectors, edge)
				continue
			}
			pkey := ConnectorKey(int64(edge.ViewId), int64(edge.Id))
			selectionDecision(out, pkey, "Connector endpoint was suppressed by display policy", nil)
			delete(out.Provenance, pkey)
		}
		content.Connectors = connectors
		if len(placements) == 0 {
			delete(out.Views, key)
			id, _ := strconv.ParseInt(key, 10, 64)
			delete(out.Provenance, ViewKey(id))
		}
	}
	out.Tree = pruneScopedSceneTree(out.Tree, kept)
	var navigations []*diagv1.ElementNavigationInfo
	for _, link := range out.Navigations {
		if len(kept[strconv.Itoa(int(link.FromViewId))]) > 0 && len(kept[strconv.Itoa(int(link.ToViewId))]) > 0 {
			navigations = append(navigations, link)
		}
	}
	out.Navigations = navigations
	var authored []int64
	for _, id := range out.AuthoredViewIds {
		if out.Views[strconv.FormatInt(id, 10)] != nil {
			authored = append(authored, id)
		}
	}
	out.AuthoredViewIds = authored
	if out.Views[strconv.FormatInt(out.FallbackViewId, 10)] == nil {
		out.FallbackViewId = 0
	}
	return out
}

func selectionDecision(scene *pb.ImpactScene, key, reason string, paths []string) {
	if len(scene.Stages) == 0 {
		return
	}
	scene.Decisions = append(scene.Decisions, &pb.ImpactDecision{Stage: "reduce", Action: "suppress", Key: key, Reason: reason, Paths: paths})
	for _, stage := range scene.Stages {
		if stage.Id == "reduce" {
			stage.Suppressed++
		}
	}
}

// PublicScene never exposes the detailed trace unless explicitly requested.
func PublicScene(scene *pb.ImpactScene, debug bool) *pb.ImpactScene {
	if scene == nil {
		return nil
	}
	out := proto.Clone(scene).(*pb.ImpactScene)
	if !debug {
		out.Stages, out.Decisions = nil, nil
	}
	return out
}

func ValidateCoverage(scene *pb.ImpactScene) error {
	visible := map[string]bool{}
	for _, content := range scene.GetViews() {
		for _, placement := range content.Placements {
			e := placement.Element
			visible[PlacementKey(int64(e.ViewId), int64(e.ElementId))] = true
		}
	}
	for _, coverage := range scene.GetCoverage() {
		if len(coverage.PlacementKeys) == 0 {
			return fmt.Errorf("impact coverage missing %s", coverage.Path)
		}
		for _, key := range coverage.PlacementKeys {
			if scene.Provenance[key] == nil || !visible[key] {
				return fmt.Errorf("impact coverage %s references absent placement %s", coverage.Path, key)
			}
		}
	}
	return nil
}
