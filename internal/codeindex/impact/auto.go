package impact

import (
	"sort"
	"strconv"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
)

// AutoContext removes optional visual noise from a selected scene. Changes,
// their representatives, and the paths to more distant context are preserved.
// It never mutates the cached assembly.
func AutoContext(scene *model.Assembly) *model.Assembly {
	out := model.Clone(scene)
	if out == nil {
		return nil
	}
	for _, view := range out.Views {
		edges := view.Connectors[:0]
		for _, edge := range view.Connectors {
			key := ConnectorKey(int64(edge.ViewId), int64(edge.Id))
			p := out.Provenance[key]
			var from, to *model.Metadata
			if len(p.GetEndpointKeys()) == 2 {
				from, to = out.Provenance[p.EndpointKeys[0]], out.Provenance[p.EndpointKeys[1]]
			}
			changed := false
			for _, tag := range edge.Tags {
				changed = changed || tag == "change:added" || tag == "change:removed" || tag == "change:modified"
			}
			if !changed && from != nil && to != nil &&
				from.Role == pb.ImpactRole_IMPACT_ROLE_DEPENDENCY_CONTEXT &&
				to.Role == pb.ImpactRole_IMPACT_ROLE_DEPENDENCY_CONTEXT && from.Distance == to.Distance {
				selectionDecision(out, key, "Auto: unchanged relationship between neighbours at the same distance")
				delete(out.Provenance, key)
				continue
			}
			edges = append(edges, edge)
		}
		view.Connectors = edges
	}
	graph := sceneComponents(out)
	kept := sceneKept(out, out.EffectiveRadius, out.GetComparison().GetPlain())
	for _, group := range graph {
		if len(group) != 1 {
			continue
		}
		p := out.Provenance[group[0]]
		if p != nil && (p.Role == pb.ImpactRole_IMPACT_ROLE_AUTHORED_CONTEXT || p.Role == pb.ImpactRole_IMPACT_ROLE_DEPENDENCY_CONTEXT) && !representsFile(out, group[0]) && !hasSceneConnector(out, group[0]) {
			delete(kept[strconv.FormatInt(p.ViewId, 10)], int32(p.ResourceId))
		}
	}
	pruneAutoLeaves(out, kept)
	return selectSceneKeys(out, kept, "Auto: optional unchanged context", true)
}

// Wide stars become unreadable when scaled to a PR comment. Trim only optional
// context leaves, preferring nearer and reciprocally connected neighbours. A
// bridge, changed connector endpoint, or file representative cannot be trimmed.
func pruneAutoLeaves(scene *model.Assembly, kept map[string]map[int32]bool) {
	limit := 12
	if len(scene.Files) <= 2 {
		limit = 6
	}
	adj := sceneAdjacency(scene)
	protected, degree := map[string]bool{}, map[string]int{}
	protectedIDs, occurrences := map[int32]bool{}, map[int32]int{}
	for _, view := range scene.Views {
		for _, placement := range view.Placements {
			occurrences[placement.Element.ElementId]++
		}
	}
	for _, nav := range scene.Navigations {
		for _, id := range []int32{nav.FromViewId, nav.ToViewId} {
			if view := scene.Views[strconv.Itoa(int(id))]; view != nil {
				for _, placement := range view.Placements {
					protectedIDs[placement.Element.ElementId] = true
				}
			}
		}
	}
	for _, view := range scene.Views {
		for _, edge := range view.Connectors {
			keys := []string{PlacementKey(int64(edge.ViewId), int64(edge.SourceElementId)), PlacementKey(int64(edge.ViewId), int64(edge.TargetElementId))}
			if p := scene.Provenance[ConnectorKey(int64(edge.ViewId), int64(edge.Id))]; len(p.GetEndpointKeys()) == 2 {
				keys = p.EndpointKeys
			}
			for _, key := range keys {
				degree[key]++
				for _, tag := range edge.Tags {
					if tag == "change:added" || tag == "change:removed" || tag == "change:modified" {
						protected[key] = true
						protectedIDs[edge.SourceElementId], protectedIDs[edge.TargetElementId] = true, true
					}
				}
			}
		}
	}
	var leaves []*model.Metadata
	count := 0
	for key, neighbours := range adj {
		p := scene.Provenance[key]
		if p == nil || p.Role != pb.ImpactRole_IMPACT_ROLE_DEPENDENCY_CONTEXT || !kept[strconv.FormatInt(p.ViewId, 10)][int32(p.ResourceId)] {
			continue
		}
		count++
		if len(neighbours) == 1 && !neighbours[key] && !protected[key] && !protectedIDs[int32(p.ResourceId)] && occurrences[int32(p.ResourceId)] == 1 && !representsFile(scene, key) {
			leaves = append(leaves, p)
		}
	}
	sort.Slice(leaves, func(i, j int) bool {
		a, b := leaves[i], leaves[j]
		if a.Distance != b.Distance {
			return a.Distance > b.Distance
		}
		if degree[a.Key] != degree[b.Key] {
			return degree[a.Key] < degree[b.Key]
		}
		if a.Name != b.Name {
			return a.Name > b.Name
		}
		return a.Key > b.Key
	})
	for _, p := range leaves {
		if count <= limit {
			break
		}
		delete(kept[strconv.FormatInt(p.ViewId, 10)], int32(p.ResourceId))
		selectionDecision(scene, p.Key, "Auto: optional context leaf exceeds neighbour budget")
		count--
	}
}

func hasSceneConnector(scene *model.Assembly, key string) bool {
	ref := placementRef(key)
	for _, view := range scene.Views {
		for _, edge := range view.Connectors {
			if (edge.ViewId == ref.ViewId && edge.SourceElementId == ref.ElementId) || edge.TargetElementId == ref.ElementId {
				return true
			}
		}
	}
	return false
}

// SeparateAutoFiles moves isolated raw file representations into a Markdown
// ledger. The returned scene and ledger together cover the input; the canonical
// scene used for JSON, URLs, and reports remains unchanged.
func SeparateAutoFiles(scene *model.Assembly) (*model.Assembly, []*pb.ImpactFile) {
	out := model.Clone(scene)
	kept := sceneKept(out, out.EffectiveRadius, out.GetComparison().GetPlain())
	listed := map[string]bool{}
	for _, group := range sceneComponents(out) {
		if len(group) != 1 || hasSceneConnector(out, group[0]) {
			continue
		}
		p := out.Provenance[group[0]]
		if p != nil && p.Origin == pb.ImpactOrigin_IMPACT_ORIGIN_RAW && representsFile(out, group[0]) {
			listed[group[0]] = true
			delete(kept[strconv.FormatInt(p.ViewId, 10)], int32(p.ResourceId))
		}
	}
	var files []*pb.ImpactFile
	retained := out.Files[:0]
	for _, file := range out.Files {
		if len(file.Representatives) > 0 && listed[PlacementKey(int64(file.Representatives[0].ViewId), int64(file.Representatives[0].ElementId))] {
			files = append(files, file)
		} else {
			retained = append(retained, file)
		}
	}
	out.Files = retained
	return selectSceneKeys(out, kept, "Auto: represented in the changed-file list", true), files
}

func representsFile(scene *model.Assembly, key string) bool {
	for _, file := range scene.Files {
		for _, ref := range file.Representatives {
			if PlacementKey(int64(ref.ViewId), int64(ref.ElementId)) == key {
				return true
			}
		}
	}
	return false
}

// SplitAutoScene partitions a larger scene into at most two disconnected
// panels. Edges, navigation, repeated elements, and all representatives of a
// changed file are indivisible. Small scenes and single islands stay together.
func SplitAutoScene(scene *model.Assembly) []*model.Assembly {
	if SceneNodeCount(scene) <= 12 {
		return []*model.Assembly{scene}
	}
	groups := sceneComponents(scene)
	if len(groups) < 2 || len(groups[1]) < 3 {
		return []*model.Assembly{scene}
	}
	panels := [2][]string{}
	for _, group := range groups {
		i := 0
		if len(panels[1]) < len(panels[0]) {
			i = 1
		}
		panels[i] = append(panels[i], group...)
	}
	result := make([]*model.Assembly, 0, 2)
	for _, panel := range panels {
		out := model.Clone(scene)
		kept := map[string]map[int32]bool{}
		for key := range out.Views {
			kept[key] = map[int32]bool{}
		}
		members := map[string]bool{}
		for _, key := range panel {
			members[key] = true
			ref := placementRef(key)
			kept[strconv.Itoa(int(ref.ViewId))][ref.ElementId] = true
		}
		files := out.Files[:0]
		for _, file := range out.Files {
			if len(file.Representatives) > 0 && members[PlacementKey(int64(file.Representatives[0].ViewId), int64(file.Representatives[0].ElementId))] {
				files = append(files, file)
			}
		}
		out.Files = files
		result = append(result, selectSceneKeys(out, kept, "Auto: represented in the other diagram", true))
	}
	return result
}

// sceneComponents uses weak connectivity only for presentation; edge direction
// and styling are never changed. Sorting makes panel choice deterministic.
func sceneComponents(scene *model.Assembly) [][]string {
	adj := sceneAdjacency(scene)
	seen := map[string]bool{}
	var groups [][]string
	for _, key := range sortedKeys(adj) {
		if seen[key] {
			continue
		}
		seen[key] = true
		group := []string{key}
		for i := 0; i < len(group); i++ {
			for next := range adj[group[i]] {
				if !seen[next] {
					seen[next] = true
					group = append(group, next)
				}
			}
		}
		sort.Strings(group)
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if len(groups[i]) != len(groups[j]) {
			return len(groups[i]) > len(groups[j])
		}
		return strings.Compare(groups[i][0], groups[j][0]) < 0
	})
	return groups
}

func sceneAdjacency(scene *model.Assembly) map[string]map[string]bool {
	adj := map[string]map[string]bool{}
	byElement := map[int32][]string{}
	for _, view := range scene.Views {
		for _, placement := range view.Placements {
			e := placement.Element
			key := PlacementKey(int64(e.ViewId), int64(e.ElementId))
			adj[key] = map[string]bool{}
			byElement[e.ElementId] = append(byElement[e.ElementId], key)
		}
	}
	join := func(a, b string) {
		if adj[a] != nil && adj[b] != nil {
			adj[a][b], adj[b][a] = true, true
		}
	}
	for _, refs := range byElement {
		for _, key := range refs[1:] {
			join(refs[0], key)
		}
	}
	for _, view := range scene.Views {
		for _, edge := range view.Connectors {
			p := scene.Provenance[ConnectorKey(int64(edge.ViewId), int64(edge.Id))]
			if len(p.GetEndpointKeys()) == 2 {
				join(p.EndpointKeys[0], p.EndpointKeys[1])
			} else {
				for _, target := range byElement[edge.TargetElementId] {
					join(PlacementKey(int64(edge.ViewId), int64(edge.SourceElementId)), target)
				}
			}
		}
	}
	for _, file := range scene.Files {
		if len(file.Representatives) == 0 {
			continue
		}
		first := file.Representatives[0]
		for _, ref := range file.Representatives[1:] {
			join(PlacementKey(int64(first.ViewId), int64(first.ElementId)), PlacementKey(int64(ref.ViewId), int64(ref.ElementId)))
		}
	}
	for _, nav := range scene.Navigations {
		// Keep navigation targets with their source; no panel may have a dead link.
		from := scene.Views[strconv.Itoa(int(nav.FromViewId))]
		to := scene.Views[strconv.Itoa(int(nav.ToViewId))]
		if from == nil || to == nil {
			continue
		}
		for _, a := range from.Placements {
			if nav.ElementId != nil && a.Element.ElementId != *nav.ElementId {
				continue
			}
			for _, b := range to.Placements {
				join(PlacementKey(int64(nav.FromViewId), int64(a.Element.ElementId)), PlacementKey(int64(nav.ToViewId), int64(b.Element.ElementId)))
			}
		}
	}
	return adj
}
