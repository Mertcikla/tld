package impact

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/internal/layout"
	"github.com/mertcikla/tld/v2/internal/repolink"
	"github.com/mertcikla/tld/v2/internal/sourcelink"
	"google.golang.org/protobuf/proto"
)

const PipelineVersion = "3"

var pipelineStages = [][2]string{
	{"capture", "Capture inputs"}, {"authored", "Seed authored diagram"},
	{"ground", "Ground and annotate"}, {"mapped", "Fill mapped gaps"},
	{"coverage", "Guarantee direct coverage"}, {"dependencies", "Expand dependencies"},
	{"relationships", "Project relationships"}, {"reduce", "Reduce noise"},
	{"validate", "Place and validate"}, {"freeze", "Freeze and serve"},
}

func PlacementKey(viewID, elementID int64) string {
	return fmt.Sprintf("placement|%d|%d", viewID, elementID)
}
func ConnectorKey(viewID, id int64) string { return fmt.Sprintf("connector|%d|%d", viewID, id) }
func ViewKey(id int64) string              { return fmt.Sprintf("view|%d", id) }

type anchorMatch struct {
	nodes     []*pb.ImpactNode
	paths     []string
	grounding pb.ImpactGrounding
	partial   bool
	contained bool
	scope     string
	anchor    sourcelink.Anchor
}

type sceneAssembly struct {
	helper                     *sceneEvidence
	scene                      *pb.ImpactScene
	views                      map[int64]core.ViewTreeNode
	mapped                     map[int64]cstore.ResourceMapping
	mapViews                   map[int64]bool
	mapConnectors              map[int64]cstore.ResourceMapping
	matches                    map[int64]anchorMatch
	sources                    map[string]bool
	relationshipEvidence       map[[2]string][]string
	repositoryRemote           string
	ambiguousPaths             map[string]bool
	full                       map[string][]string
	all                        map[string][]string
	stages                     map[string]*pb.ImpactStage
	nextElement, nextConnector int32
}

// assemble is the only semantic scene construction pipeline for new comparisons.
// The diagram's file analysis is immutable; every canvas mutation is recorded.
func assemble(ctx context.Context, service Service, diagram *pb.ImpactDiagram, workspace core.ExploreData, mappings []cstore.ResourceMapping, root string, capturedRepositories ...[]*pb.Repository) (*pb.ImpactScene, error) {
	helper := &sceneEvidence{diagram: diagram, workspace: workspace, mappings: mappings, root: root, store: service.Index, ctx: ctx}
	helper.nodes = map[string]*pb.ImpactNode{}
	helper.sources = map[string]*pb.SourceChange{}
	for _, node := range diagram.GetNodes() {
		helper.nodes[normalizeScenePath(node.Path)] = node
	}
	for _, source := range diagram.GetDiff().GetSources() {
		helper.sources[source.Path] = source
	}
	a := &sceneAssembly{
		helper: helper, views: map[int64]core.ViewTreeNode{}, mapped: map[int64]cstore.ResourceMapping{},
		mapViews: map[int64]bool{}, mapConnectors: map[int64]cstore.ResourceMapping{}, matches: map[int64]anchorMatch{},
		sources: map[string]bool{}, relationshipEvidence: map[[2]string][]string{}, full: map[string][]string{}, all: map[string][]string{}, stages: map[string]*pb.ImpactStage{},
		scene: &pb.ImpactScene{SchemaVersion: PipelineVersion, RepositoryId: diagram.RepositoryId, ComparisonKey: diagram.ComparisonKey,
			Version: diagram.Version, FromGitRevision: diagram.GetDiff().GetFromGitRevision(), ToGitRevision: diagram.GetDiff().GetToGitRevision(),
			Views: map[string]*pb.SceneViewContent{}, Provenance: map[string]*pb.ImpactProvenance{},
			RequestedRadius: diagram.GetRequestedRadius(), EffectiveRadius: diagram.GetRequestedRadius(),
			Warnings: append([]string(nil), diagram.Warnings...), ManifestComplete: diagram.ManifestComplete,
			WorkspaceFingerprint: diagram.WorkspaceFingerprint},
	}
	for _, stage := range pipelineStages {
		entry := &pb.ImpactStage{Id: stage[0], Name: stage[1]}
		a.stages[entry.Id] = entry
		a.scene.Stages = append(a.scene.Stages, entry)
	}
	for _, mapping := range mappings {
		if strings.HasPrefix(mapping.LogicalKey, "impact|") {
			continue
		}
		switch mapping.Kind {
		case cstore.MappingElement:
			a.mapped[mapping.ResourceID] = mapping
		case cstore.MappingView:
			a.mapViews[mapping.ResourceID] = true
		case cstore.MappingConnector:
			a.mapConnectors[mapping.ResourceID] = mapping
		}
	}
	var walk func([]core.ViewTreeNode)
	walk = func(views []core.ViewTreeNode) {
		for _, view := range views {
			a.views[view.ID] = view
			walk(view.Children)
		}
	}
	walk(workspace.Tree)
	repo, err := service.Index.Repository(ctx, diagram.RepositoryId)
	if err != nil {
		return nil, err
	}
	a.repositoryRemote = repo.RemoteUrl
	a.ambiguousPaths = map[string]bool{}
	var repositories []*pb.Repository
	if len(capturedRepositories) > 0 {
		repositories = capturedRepositories[0]
	} else {
		repositories, err = service.Index.ListRepositories(ctx)
		if err != nil {
			return nil, err
		}
	}
	for _, candidate := range repositories {
		if candidate.Id == diagram.RepositoryId {
			a.repositoryRemote = candidate.RemoteUrl
			continue
		}
		if candidate.LatestSnapshotId == "" {
			continue
		}
		snapshot, err := service.Index.Snapshot(ctx, candidate.LatestSnapshotId)
		if err != nil {
			return nil, err
		}
		for _, source := range snapshot.Sources {
			a.ambiguousPaths[source.Path] = true
		}
		for _, file := range snapshot.GetManifest().GetFiles() {
			a.ambiguousPaths[file.Path] = true
		}
	}
	for _, snapshotID := range []string{diagram.GetDiff().GetFromSnapshotId(), diagram.GetDiff().GetToSnapshotId()} {
		if snapshotID == "" {
			continue
		}
		snapshot, err := service.Index.Snapshot(ctx, snapshotID)
		if err != nil {
			return nil, err
		}
		kinds, err := service.Index.FilePairKinds(ctx, snapshotID)
		if err != nil {
			return nil, err
		}
		for pair, values := range kinds {
			for _, kind := range values {
				a.relationshipEvidence[pair] = append(a.relationshipEvidence[pair], snapshotID+": "+kind.String())
			}
		}
		for _, source := range snapshot.Sources {
			a.sources[source.Path] = true
		}
		for _, file := range snapshot.GetManifest().GetFiles() {
			a.sources[file.Path] = true
		}
	}
	a.record("capture", "reuse", "inputs", "Pinned snapshots, workspace and mappings", nil)
	if err := a.seedAuthored(); err != nil {
		return nil, err
	}
	a.expandAuthoredChildren()
	a.fillMapped(false)
	a.fillRaw(false)
	a.expandDependencies()
	a.finishHierarchy()
	a.projectRelationships()
	a.record("reduce", "reuse", "policy", "Standard preserves the authored skeleton; generated duplicates were suppressed by coverage", nil)
	a.placeAdditions()
	if err := a.validate(); err != nil {
		return nil, err
	}
	a.record("freeze", "reuse", "scene", "Frozen portable scene; no workspace resources written", nil)
	return a.scene, nil
}

func (a *sceneAssembly) record(stage, action, key, reason string, paths []string) {
	a.scene.Decisions = append(a.scene.Decisions, &pb.ImpactDecision{Stage: stage, Action: action, Key: key, Reason: reason, Paths: append([]string(nil), paths...)})
	entry := a.stages[stage]
	switch action {
	case "add":
		entry.Added++
	case "annotate", "place":
		entry.Annotated++
	case "reuse":
		entry.Reused++
	case "suppress", "aggregate":
		entry.Suppressed++
	}
}

func (a *sceneAssembly) ensureView(id int64, stage string) *pb.SceneViewContent {
	key := strconv.FormatInt(id, 10)
	if content := a.scene.Views[key]; content != nil {
		return content
	}
	a.scene.Views[key] = &pb.SceneViewContent{}
	name, origin := "Changes", pb.ImpactOrigin_IMPACT_ORIGIN_RAW
	if view, exists := a.views[id]; exists {
		name, origin = view.Name, pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED
		if a.mapViews[id] {
			origin = pb.ImpactOrigin_IMPACT_ORIGIN_MAPPED
		}
	}
	provenance := &pb.ImpactProvenance{Key: ViewKey(id), ObjectKind: "view", ViewId: id, ResourceId: id, Name: name, Origin: origin,
		Role: pb.ImpactRole_IMPACT_ROLE_CONTAINER, Stage: stage, Reason: "Contains retained impact placements"}
	a.scene.Provenance[provenance.Key] = provenance
	a.record(stage, "add", provenance.Key, provenance.Reason, nil)
	return a.scene.Views[key]
}

func (a *sceneAssembly) match(element core.PlacedElement) anchorMatch {
	if match, found := a.matches[element.ElementID]; found {
		return match
	}
	match := anchorMatch{grounding: pb.ImpactGrounding_IMPACT_GROUNDING_UNLINKED}
	defer func() { a.matches[element.ElementID] = match }()
	if element.RepositoryID != nil && *element.RepositoryID != "" && *element.RepositoryID != a.scene.RepositoryId {
		match.grounding = pb.ImpactGrounding_IMPACT_GROUNDING_OUTSIDE_REPOSITORY
		return match
	}
	if element.Repo != nil && *element.Repo != "" && normalizeScenePath(*element.Repo) != normalizeScenePath(a.helper.root) && (repolink.RemoteKey(*element.Repo) == "" || repolink.RemoteKey(*element.Repo) != repolink.RemoteKey(a.repositoryRemote)) && (element.RepositoryID == nil || *element.RepositoryID == "") {
		match.grounding = pb.ImpactGrounding_IMPACT_GROUNDING_OUTSIDE_REPOSITORY
		return match
	}
	if element.FilePath == nil || strings.TrimSpace(*element.FilePath) == "" {
		return match
	}
	base, folder, ok := a.helper.linkBase(*element.FilePath)
	match.grounding = pb.ImpactGrounding_IMPACT_GROUNDING_UNRESOLVED
	if !ok {
		match.grounding = pb.ImpactGrounding_IMPACT_GROUNDING_OUTSIDE_REPOSITORY
		return match
	}
	if (element.RepositoryID == nil || *element.RepositoryID == "") && (element.Repo == nil || *element.Repo == "") {
		for path := range a.ambiguousPaths {
			if path == base || (folder && strings.HasPrefix(path, base+"/")) {
				match.grounding = pb.ImpactGrounding_IMPACT_GROUNDING_AMBIGUOUS
				return match
			}
		}
	}
	anchor := sourcelink.Parse(*element.FilePath).Anchor
	match.scope = base
	match.anchor = anchor
	match.partial = anchor.Kind == sourcelink.AnchorSymbol || anchor.Kind == sourcelink.AnchorLine
	if !a.sources[base] {
		for path := range a.sources {
			if strings.HasPrefix(path, base+"/") {
				folder = true
				break
			}
		}
	}
	match.contained = folder
	for _, path := range sortedKeys(a.sources) {
		if path != base && (!folder || !strings.HasPrefix(path, base+"/")) {
			continue
		}
		match.paths = append(match.paths, path)
		match.grounding = pb.ImpactGrounding_IMPACT_GROUNDING_RESOLVED
		if node := a.helper.nodes[path]; node != nil {
			if match.partial && !symbolAnchorMatches(node, anchor) {
				continue
			}
			match.nodes = append(match.nodes, node)
		}
	}
	// Verify symbol identity against both complete snapshots, even when unchanged.
	if match.partial && len(match.nodes) == 0 && len(match.paths) > 0 {
		resolved := false
		for _, id := range []string{a.helper.diagram.GetDiff().GetFromSnapshotId(), a.helper.diagram.GetDiff().GetToSnapshotId()} {
			facts, err := a.helper.store.Facts(a.helper.ctx, id, pb.FactKind_FACT_KIND_UNSPECIFIED, base, "", 1000000)
			if err != nil {
				continue
			}
			candidate := &pb.ImpactNode{Symbols: &pb.CodeFactDelta{Modified: facts}}
			if symbolAnchorMatches(candidate, anchor) {
				resolved = true
			}
		}
		if !resolved {
			match.grounding = pb.ImpactGrounding_IMPACT_GROUNDING_UNRESOLVED
			match.paths = nil
		}
	}
	return match
}

func (a *sceneAssembly) seedAuthored() error {
	selected := map[int64]bool{}
	filter := a.helper.diagram.ViewFilter
	for _, id := range sortedViewIDs(a.helper.workspace) {
		if a.mapViews[id] {
			continue
		}
		if filter != "" {
			if strconv.FormatInt(id, 10) == filter || strings.Contains(strings.ToLower(a.views[id].Name), strings.ToLower(filter)) {
				selected[id] = true
			}
			continue
		}
		for _, element := range a.helper.workspace.Views[strconv.FormatInt(id, 10)].Placements {
			if _, mapped := a.mapped[element.ElementID]; mapped {
				continue
			}
			if len(a.match(element).nodes) > 0 {
				selected[id] = true
				break
			}
		}
	}
	if filter != "" && len(selected) == 0 {
		return fmt.Errorf("authored view %q was not found", filter)
	}
	for _, id := range sortedViewIDs(a.helper.workspace) {
		if !selected[id] {
			if !a.mapViews[id] {
				a.record("authored", "suppress", ViewKey(id), "Outside selected authored view scope", nil)
			}
			continue
		}
		a.ensureView(id, "authored")
		a.scene.AuthoredViewIds = append(a.scene.AuthoredViewIds, id)
		for _, element := range sortedPlacements(a.helper.workspace.Views[strconv.FormatInt(id, 10)].Placements) {
			if _, mapped := a.mapped[element.ElementID]; mapped {
				continue
			}
			match := a.match(element)
			if match.grounding == pb.ImpactGrounding_IMPACT_GROUNDING_OUTSIDE_REPOSITORY {
				a.record("authored", "suppress", PlacementKey(id, element.ElementID), "Placement belongs to another repository", nil)
				continue
			}
			a.addPlacement(scenePlacement(element), pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED, "authored", "Copied from user's diagram", match)
		}
	}
	return nil
}

func sortedViewIDs(workspace core.ExploreData) []int64 {
	ids := make([]int64, 0, len(workspace.Views))
	for key := range workspace.Views {
		if id, err := strconv.ParseInt(key, 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func sortedPlacements(elements []core.PlacedElement) []core.PlacedElement {
	out := append([]core.PlacedElement(nil), elements...)
	sort.Slice(out, func(i, j int) bool { return out[i].ElementID < out[j].ElementID })
	return out
}

func (a *sceneAssembly) addPlacement(item *pb.ScenePlacement, origin pb.ImpactOrigin, stage, reason string, match anchorMatch) string {
	element := item.Element
	key := PlacementKey(int64(element.ViewId), int64(element.ElementId))
	if a.scene.Provenance[key] != nil {
		return key
	}
	content := a.ensureView(int64(element.ViewId), stage)
	content.Placements = append(content.Placements, item)
	p := &pb.ImpactProvenance{Key: key, ObjectKind: "placement", ViewId: int64(element.ViewId), ResourceId: int64(element.ElementId), Name: element.Name,
		Origin: origin, Grounding: match.grounding, Role: pb.ImpactRole_IMPACT_ROLE_AUTHORED_CONTEXT, Stage: stage, Reason: reason,
		Paths: append([]string(nil), match.paths...), Partial: match.partial}
	if mapping, ok := a.mapped[int64(element.ElementId)]; ok {
		p.LogicalKey, p.MapSnapshotId = mapping.LogicalKey, mapping.SnapshotID
	}
	a.scene.Provenance[key] = p
	a.record(stage, "add", key, reason, match.paths)
	if len(match.nodes) > 0 {
		p.Distance = match.nodes[0].Distance
		for _, node := range match.nodes {
			if node.Distance < p.Distance {
				p.Distance = node.Distance
			}
			p.Witnesses = append(p.Witnesses, node.Witnesses...)
		}
		p.Role = pb.ImpactRole_IMPACT_ROLE_DEPENDENCY_CONTEXT
		if p.Distance == 0 {
			p.Role = pb.ImpactRole_IMPACT_ROLE_DIRECT
			if match.contained {
				p.Role = pb.ImpactRole_IMPACT_ROLE_CONTAINED
			}
		}
		item.Overlay = a.matchOverlay(match)
		for _, node := range match.nodes {
			a.all[node.Path] = append(a.all[node.Path], key)
			if !match.partial {
				a.full[node.Path] = append(a.full[node.Path], key)
			}
			if source := a.helper.sources[node.Path]; source != nil && source.FactsUnavailable {
				p.Grounding = pb.ImpactGrounding_IMPACT_GROUNDING_MANIFEST_ONLY
			}
		}
		annotationStage := stage
		if origin == pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED {
			annotationStage = "ground"
		}
		a.record(annotationStage, "annotate", key, "Matched source scope against pinned comparison", p.Paths)
	}
	return key
}

func (a *sceneAssembly) matchOverlay(match anchorMatch) *pb.ImpactSceneOverlay {
	first := match.nodes[0]
	out := a.helper.overlay(first)
	if match.partial {
		out.LinesAdded, out.LinesRemoved = nil, nil
		filtered := &pb.CodeFactDelta{}
		for _, entry := range []struct {
			source []*pb.CodeFact
			target *[]*pb.CodeFact
		}{{first.GetSymbols().GetAdded(), &filtered.Added}, {first.GetSymbols().GetRemoved(), &filtered.Removed}, {first.GetSymbols().GetModified(), &filtered.Modified}} {
			for _, fact := range entry.source {
				if symbolAnchorMatches(&pb.ImpactNode{Symbols: &pb.CodeFactDelta{Modified: []*pb.CodeFact{fact}}}, match.anchor) {
					*entry.target = append(*entry.target, fact)
				}
			}
		}
		out.Symbols = a.helper.overlay(&pb.ImpactNode{Symbols: filtered}).Symbols
	}
	if len(match.nodes) == 1 {
		if match.contained {
			out.Reason = pb.OverlayReason_OVERLAY_REASON_CONTAINED
			out.Path = match.scope + "/"
		}
		return out
	}
	out = &pb.ImpactSceneOverlay{Path: strings.Join(match.paths, ", "), Distance: first.Distance, Change: first.Change, Reason: pb.OverlayReason_OVERLAY_REASON_CONTAINED}
	var added, removed uint32
	for _, node := range match.nodes {
		if node.Distance < out.Distance {
			out.Distance = node.Distance
		}
		if node.Change != out.Change {
			out.Change = pb.ChangeKind_CHANGE_KIND_MODIFIED
		}
		overlay := a.helper.overlay(node)
		added += overlay.GetLinesAdded()
		removed += overlay.GetLinesRemoved()
		out.Symbols = append(out.Symbols, overlay.Symbols...)
	}
	if match.contained {
		out.Path = match.scope + "/"
	}
	out.LinesAdded, out.LinesRemoved = proto.Uint32(added), proto.Uint32(removed)
	return out
}

func (a *sceneAssembly) fillMapped(contextOnly bool) {
	stage := "mapped"
	if contextOnly {
		stage = "dependencies"
	}
	for _, id := range sortedViewIDs(a.helper.workspace) {
		for _, element := range sortedPlacements(a.helper.workspace.Views[strconv.FormatInt(id, 10)].Placements) {
			mapping, mapped := a.mapped[element.ElementID]
			if !mapped {
				continue
			}
			match := a.match(element)
			var relevant []*pb.ImpactNode
			for _, node := range match.nodes {
				if (node.Distance > 0) == contextOnly {
					relevant = append(relevant, node)
				}
			}
			if len(relevant) == 0 {
				continue
			}
			if mapping.SnapshotID != a.helper.diagram.GetDiff().GetFromSnapshotId() && mapping.SnapshotID != a.helper.diagram.GetDiff().GetToSnapshotId() {
				a.record(stage, "suppress", PlacementKey(id, element.ElementID), "Stale map snapshot; using verified fallback", match.paths)
				continue
			}
			uncovered := false
			for _, node := range relevant {
				if len(a.full[node.Path]) == 0 {
					uncovered = true
					break
				}
			}
			if !uncovered {
				a.record(stage, "suppress", PlacementKey(id, element.ElementID), "Scope already represented at higher priority", match.paths)
				for _, path := range match.paths {
					a.scene.Decisions[len(a.scene.Decisions)-1].RepresentativeKeys = append(a.scene.Decisions[len(a.scene.Decisions)-1].RepresentativeKeys, a.full[path]...)
				}
				continue
			}
			match.nodes = relevant
			a.addPlacement(scenePlacement(element), pb.ImpactOrigin_IMPACT_ORIGIN_MAPPED, stage, "Fills scope missing from authored primitives", match)
		}
	}
}

func (a *sceneAssembly) destination(node *pb.ImpactNode) (int64, string) {
	best, bestScore := int64(0), 0
	for _, key := range sortedKeys(a.scene.Provenance) {
		p := a.scene.Provenance[key]
		if p.ObjectKind != "placement" || p.Origin != pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED {
			continue
		}
		score := 0
		for _, path := range p.Paths {
			if path == node.Path {
				score += 100
			}
			for _, witness := range node.Witnesses {
				for _, step := range witness.Paths {
					if step == path {
						score++
					}
				}
			}
			for _, edge := range a.helper.diagram.Edges {
				from, to := a.nodePath(edge.FromKey), a.nodePath(edge.ToKey)
				if (from == node.Path && to == path) || (to == node.Path && from == path) {
					score += 10
				}
			}
		}
		if score > bestScore || (score > 0 && score == bestScore && p.ViewId < best) {
			best, bestScore = p.ViewId, score
		}
	}
	if bestScore > 0 {
		return best, "Placed beside authored source coverage or dependency witness"
	}
	if id := buildViewIndex(a.helper.workspace, a.helper.mappings).closest(node.Path); id != 0 {
		if _, ok := a.views[id]; ok {
			return id, "Placed in nearest mapped source container"
		}
	}
	return -1, "No authored or mapped destination; using Changes view"
}

func (a *sceneAssembly) fillRaw(contextOnly bool) {
	stage := "coverage"
	if contextOnly {
		stage = "dependencies"
	}
	for _, node := range a.helper.diagram.Nodes {
		if (node.Distance > 0) != contextOnly || len(a.full[node.Path]) > 0 {
			continue
		}
		id, reason := a.destination(node)
		a.nextElement--
		elementID := a.nextElement
		if id == -1 {
			a.scene.FallbackViewId = -1
		}
		item := &pb.ScenePlacement{Element: &diagv1.PlacedElement{Id: elementID, ElementId: elementID, ViewId: int32(id), Name: transientName(node.Path),
			Kind: proto.String("component"), FilePath: proto.String(node.Path), RepositoryId: proto.String(a.scene.RepositoryId), Repo: proto.String(a.helper.root)}}
		match := anchorMatch{nodes: []*pb.ImpactNode{node}, paths: []string{node.Path}, grounding: pb.ImpactGrounding_IMPACT_GROUNDING_RESOLVED}
		a.addPlacement(item, pb.ImpactOrigin_IMPACT_ORIGIN_RAW, stage, reason, match)
	}
}

func (a *sceneAssembly) expandDependencies() {
	// Seeding retained relevant authored neighbours too; now guarantee the
	// remaining neighbourhood independently of whether any file was mapped.
	a.fillMapped(true)
	a.fillRaw(true)
}

func (a *sceneAssembly) nodePath(key string) string {
	for _, node := range a.helper.diagram.Nodes {
		if node.Key == key {
			return node.Path
		}
	}
	return ""
}

func (a *sceneAssembly) expandAuthoredChildren() {
	for changed := true; changed; {
		changed = false
		for _, navigation := range a.helper.workspace.Navigations {
			if navigation.RelationType != "child" || navigation.ElementID == nil || a.mapViews[navigation.ToViewID] {
				continue
			}
			if a.scene.Provenance[PlacementKey(navigation.FromViewID, *navigation.ElementID)] == nil {
				relevantOwner := false
				if a.helper.diagram.ViewFilter == "" || a.scene.Views[strconv.FormatInt(navigation.FromViewID, 10)] != nil {
					for _, owner := range a.helper.workspace.Views[strconv.FormatInt(navigation.FromViewID, 10)].Placements {
						if owner.ElementID == *navigation.ElementID && len(a.match(owner).nodes) > 0 {
							relevantOwner = true
							break
						}
					}
				}
				if !relevantOwner {
					continue
				}
			}
			if a.scene.Views[strconv.FormatInt(navigation.ToViewID, 10)] != nil {
				continue
			}
			a.ensureView(navigation.ToViewID, "authored")
			a.scene.AuthoredViewIds = append(a.scene.AuthoredViewIds, navigation.ToViewID)
			for _, element := range sortedPlacements(a.helper.workspace.Views[strconv.FormatInt(navigation.ToViewID, 10)].Placements) {
				if _, mapped := a.mapped[element.ElementID]; mapped {
					continue
				}
				a.addPlacement(scenePlacement(element), pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED, "authored", "Authored drill-down skeleton of retained element", a.match(element))
			}
			changed = true
		}
	}
}

func (a *sceneAssembly) projectRelationships() {
	for _, id := range sortedViewIDs(a.helper.workspace) {
		content := a.scene.Views[strconv.FormatInt(id, 10)]
		if content == nil {
			continue
		}
		visible := map[int32]bool{}
		for _, placement := range content.Placements {
			visible[placement.Element.ElementId] = true
		}
		connectors := append([]core.Connector(nil), a.helper.workspace.Views[strconv.FormatInt(id, 10)].Connectors...)
		sort.Slice(connectors, func(i, j int) bool { return connectors[i].ID < connectors[j].ID })
		for _, connector := range connectors {
			key := ConnectorKey(id, connector.ID)
			if !visible[int32(connector.SourceElementID)] || !visible[int32(connector.TargetElementID)] {
				a.record("relationships", "suppress", key, "Connector endpoint is outside the impact scene", nil)
				continue
			}
			origin := pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED
			if _, mapped := a.mapConnectors[connector.ID]; mapped {
				origin = pb.ImpactOrigin_IMPACT_ORIGIN_MAPPED
			}
			item := sceneConnector(connector)
			content.Connectors = append(content.Connectors, item)
			p := &pb.ImpactProvenance{Key: key, ObjectKind: "connector", ViewId: id, ResourceId: connector.ID, Name: filePathValue(connector.Label),
				Origin: origin, Grounding: pb.ImpactGrounding_IMPACT_GROUNDING_UNRESOLVED, Stage: "relationships", Reason: "Preserved declared workspace relationship"}
			a.scene.Provenance[key] = p
			a.record("relationships", "add", key, p.Reason, nil)
		}
	}
	for _, edge := range a.helper.diagram.Edges {
		fromPath, toPath := a.nodePath(edge.FromKey), a.nodePath(edge.ToKey)
		for _, fromKey := range a.full[fromPath] {
			for _, toKey := range a.full[toPath] {
				from, to := a.scene.Provenance[fromKey], a.scene.Provenance[toKey]
				if fromKey == toKey {
					from.Evidence = appendTagUnique(from.Evidence, fromPath+" → "+toPath+" (internal)")
					continue
				}
				// Preserve true endpoints across views. The renderer uses the
				// navigation graph to project cross-view connectors.
				content := a.scene.Views[strconv.FormatInt(from.ViewId, 10)]
				var connector *diagv1.Connector
				for _, candidate := range content.Connectors {
					if int64(candidate.SourceElementId) == from.ResourceId && int64(candidate.TargetElementId) == to.ResourceId {
						connector = candidate
						break
					}
				}
				if connector == nil {
					a.nextConnector--
					connector = &diagv1.Connector{Id: a.nextConnector, ViewId: int32(from.ViewId), SourceElementId: int32(from.ResourceId), TargetElementId: int32(to.ResourceId), Direction: "forward", Style: "bezier", Label: proto.String("dependency")}
					content.Connectors = append(content.Connectors, connector)
					key := ConnectorKey(from.ViewId, int64(connector.Id))
					a.scene.Provenance[key] = &pb.ImpactProvenance{Key: key, ObjectKind: "connector", ViewId: from.ViewId, ResourceId: int64(connector.Id), Name: "dependency", Origin: pb.ImpactOrigin_IMPACT_ORIGIN_RAW, Stage: "relationships", Reason: "Observed dependency between chosen source representatives"}
					a.record("relationships", "add", key, "Projected observed source dependency", []string{fromPath, toPath})
				}
				p := a.scene.Provenance[ConnectorKey(from.ViewId, int64(connector.Id))]
				p.Grounding = pb.ImpactGrounding_IMPACT_GROUNDING_RESOLVED
				p.EndpointKeys = []string{fromKey, toKey}
				for _, evidence := range a.relationshipEvidence[[2]string{fromPath, toPath}] {
					p.Evidence = appendTagUnique(p.Evidence, evidence)
				}
				p.Paths = appendTagUnique(appendTagUnique(p.Paths, fromPath), toPath)
				p.Evidence = appendTagUnique(p.Evidence, fromPath+" → "+toPath+" · "+edge.Change.String()+" · weight="+strconv.FormatFloat(edge.Weight, 'f', -1, 64))
				connector.Tags = appendTagUnique(connector.Tags, connectorChangeTag(edge.Change))
				a.record("relationships", "annotate", p.Key, "Source change status is independent of scene addition", []string{fromPath, toPath})
			}
		}
	}
}

func (a *sceneAssembly) placeAdditions() {
	for _, key := range sortedKeys(a.scene.Views) {
		content := a.scene.Views[key]
		var existing []layout.Placement
		targets := map[int64]struct{}{}
		for _, placement := range content.Placements {
			element := placement.Element
			if element.ElementId < 0 {
				targets[int64(element.ElementId)] = struct{}{}
			} else {
				existing = append(existing, layout.Placement{ElementID: int64(element.ElementId), X: element.PositionX, Y: element.PositionY})
			}
		}
		var connectors []layout.Connector
		for _, edge := range content.Connectors {
			connectors = append(connectors, layout.Connector{Source: int64(edge.SourceElementId), Target: int64(edge.TargetElementId)})
		}
		positions := layout.DeterministicLayoutPlacements(existing, targets, connectors)
		for _, placement := range content.Placements {
			element := placement.Element
			if _, added := targets[int64(element.ElementId)]; !added {
				continue
			}
			position := positions[int64(element.ElementId)]
			element.PositionX, element.PositionY = position.X, position.Y
			a.record("validate", "place", PlacementKey(int64(element.ViewId), int64(element.ElementId)), "Positioned new primitive around fixed workspace placements", nil)
		}
		for _, edge := range content.Connectors {
			if edge.Id < 0 {
				adjustSceneConnectorHandles(content.Placements, []*diagv1.Connector{edge})
			}
		}
	}
}

func (a *sceneAssembly) finishHierarchy() {
	// Keep owners and ancestors to a fixpoint, without importing unrelated
	// generated siblings. Authored containers retain their original position.
	for changed := true; changed; {
		changed = false
		for _, navigation := range a.helper.workspace.Navigations {
			if navigation.RelationType != "child" || navigation.ElementID == nil {
				continue
			}
			if a.scene.Views[strconv.FormatInt(navigation.ToViewID, 10)] == nil {
				continue
			}
			key := PlacementKey(navigation.FromViewID, *navigation.ElementID)
			if a.scene.Provenance[key] != nil {
				continue
			}
			for _, element := range a.helper.workspace.Views[strconv.FormatInt(navigation.FromViewID, 10)].Placements {
				if element.ElementID != *navigation.ElementID {
					continue
				}
				origin := pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED
				if _, mapped := a.mapped[element.ElementID]; mapped {
					origin = pb.ImpactOrigin_IMPACT_ORIGIN_MAPPED
				}
				a.addPlacement(scenePlacement(element), origin, "dependencies", "Required owner of retained child view", anchorMatch{grounding: pb.ImpactGrounding_IMPACT_GROUNDING_UNLINKED})
				a.scene.Provenance[key].Role = pb.ImpactRole_IMPACT_ROLE_CONTAINER
				changed = true
			}
		}
	}
	for _, navigation := range a.helper.workspace.Navigations {
		if a.scene.Views[strconv.FormatInt(navigation.FromViewID, 10)] != nil && a.scene.Views[strconv.FormatInt(navigation.ToViewID, 10)] != nil {
			a.scene.Navigations = append(a.scene.Navigations, sceneNavigation(navigation))
		}
	}
	var filter func([]core.ViewTreeNode) []*diagv1.View
	filter = func(nodes []core.ViewTreeNode) []*diagv1.View {
		var out []*diagv1.View
		for _, node := range nodes {
			children := filter(node.Children)
			if a.scene.Views[strconv.FormatInt(node.ID, 10)] == nil && len(children) == 0 {
				continue
			}
			a.ensureView(node.ID, "dependencies")
			view := sceneView(node)
			view.Children = children
			out = append(out, view)
		}
		return out
	}
	a.scene.Tree = filter(a.helper.workspace.Tree)
	if a.scene.FallbackViewId != 0 {
		a.scene.Tree = append(a.scene.Tree, &diagv1.View{Id: -1, Name: "Changes"})
	}
}

func (a *sceneAssembly) validate() error {
	for _, change := range a.helper.diagram.GetDiff().GetSources() {
		keys := a.full[change.Path]
		if len(keys) == 0 {
			return fmt.Errorf("impact coverage missing changed file %s", change.Path)
		}
		covered := proto.Clone(change).(*pb.SourceChange)
		covered.PlacementKeys = append([]string(nil), keys...)
		a.scene.Coverage = append(a.scene.Coverage, covered)
	}
	visible := map[int32]bool{}
	for _, content := range a.scene.Views {
		for _, placement := range content.Placements {
			visible[placement.Element.ElementId] = true
		}
	}
	for viewID, content := range a.scene.Views {
		id, err := strconv.ParseInt(viewID, 10, 64)
		if err != nil {
			return err
		}
		if a.scene.Provenance[ViewKey(id)] == nil {
			return fmt.Errorf("impact view lacks provenance")
		}
		seen := map[int32]bool{}
		for _, placement := range content.Placements {
			element := placement.Element
			if seen[element.ElementId] {
				return fmt.Errorf("duplicate impact placement %d", element.ElementId)
			}
			seen[element.ElementId] = true
			if a.scene.Provenance[PlacementKey(int64(element.ViewId), int64(element.ElementId))] == nil {
				return fmt.Errorf("impact placement lacks provenance")
			}
		}
		for _, connector := range content.Connectors {
			if a.scene.Provenance[ConnectorKey(int64(connector.ViewId), int64(connector.Id))] == nil {
				return fmt.Errorf("impact connector lacks provenance")
			}
			if !visible[connector.SourceElementId] || !visible[connector.TargetElementId] {
				return fmt.Errorf("impact connector %d has a missing endpoint", connector.Id)
			}
		}
	}
	a.record("validate", "reuse", "coverage", fmt.Sprintf("%d/%d changed files represented", len(a.scene.Coverage), len(a.helper.diagram.GetDiff().GetSources())), nil)
	return nil
}
