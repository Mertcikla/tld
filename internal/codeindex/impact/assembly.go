package impact

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/filelabel"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/internal/layout"
	"github.com/mertcikla/tld/v2/internal/repolink"
	"github.com/mertcikla/tld/v2/internal/sourcelink"
	"google.golang.org/protobuf/proto"
)

const PipelineVersion = "11"

func PlacementKey(viewID, elementID int64) string {
	return fmt.Sprintf("placement|%d|%d", viewID, elementID)
}
func ConnectorKey(viewID, id int64) string { return fmt.Sprintf("connector|%d|%d", viewID, id) }
func ViewKey(id int64) string              { return fmt.Sprintf("view|%d", id) }

type anchorMatch struct {
	nodes     []*model.Node
	paths     []string
	grounding pb.ImpactGrounding
	partial   bool
	contained bool
	scope     string
	anchor    sourcelink.Anchor
}

type sceneAssembly struct {
	fileNames                  map[string]string
	helper                     *sceneEvidence
	scene                      *model.Assembly
	views                      map[int64]core.ViewTreeNode
	mapped                     map[int64]cstore.ResourceMapping
	mapViews                   map[int64]bool
	mapConnectors              map[int64]cstore.ResourceMapping
	matches                    map[int64]anchorMatch
	sources                    map[string]bool
	sortedSources              []string
	repositoryRemote           string
	ambiguousPaths             map[string]bool
	ambiguousSorted            []string
	nodePaths                  map[string]string
	edgeScores                 map[string]map[string]int
	authoredKeys               []string
	authoredPaths              map[string]map[string]bool
	viewIdx                    *viewIndex
	full                       map[string][]string
	all                        map[string][]string
	nextElement, nextConnector int32
}

// assemble is the only semantic scene construction pipeline for new comparisons.
// Analysis is immutable; only audit-relevant decisions are retained.
func assemble(ctx context.Context, service Service, diagram *model.Diagram, workspace core.ExploreData, mappings []cstore.ResourceMapping, root string, capturedRepositories ...[]*pb.Repository) (*model.Assembly, error) {
	helper := &sceneEvidence{diagram: diagram, workspace: workspace, mappings: mappings, root: root, store: service.Index, ctx: ctx}
	helper.nodes = map[string]*model.Node{}
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
		sources: map[string]bool{}, full: map[string][]string{}, all: map[string][]string{},
		authoredPaths: map[string]map[string]bool{}, nodePaths: make(map[string]string, len(diagram.GetNodes())),
		scene: &model.Assembly{SchemaVersion: PipelineVersion, RepositoryId: diagram.RepositoryId, ComparisonKey: diagram.ComparisonKey,
			Version: diagram.Version, FromGitRevision: diagram.GetDiff().GetFromGitRevision(), ToGitRevision: diagram.GetDiff().GetToGitRevision(),
			Views: map[string]*model.ViewContent{}, Provenance: map[string]*model.Metadata{},
			RequestedRadius: diagram.GetRequestedRadius(), EffectiveRadius: diagram.GetRequestedRadius(),
			Warnings: append([]string(nil), diagram.Warnings...), ManifestComplete: diagram.ManifestComplete,
			AuditReferences: map[string]*model.AuditReference{}, WorkspaceFingerprint: diagram.WorkspaceFingerprint, Comparison: diagram.Comparison, Files: diagram.Files, Dependencies: diagram.Dependencies, BaseSnapshotId: diagram.GetDiff().GetFromSnapshotId(), HeadSnapshotId: diagram.GetDiff().GetToSnapshotId()},
	}
	if a.scene.Comparison != nil {
		a.scene.Comparison.ViewFilter = diagram.ViewFilter
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
	a.fileNames = map[string]string{}
	for _, snapshotID := range []string{diagram.GetDiff().GetFromSnapshotId(), diagram.GetDiff().GetToSnapshotId()} {
		if snapshotID == "" {
			continue
		}
		snapshot, err := service.Index.Snapshot(ctx, snapshotID)
		if err != nil {
			return nil, err
		}
		paths := make([]string, 0, len(snapshot.Sources))
		for _, source := range snapshot.Sources {
			a.sources[source.Path] = true
			paths = append(paths, source.Path)
		}
		// HEAD labels take precedence for surviving files; BASE labels remain
		// available for removals. Mapped and authored names are reused verbatim.
		for p, name := range filelabel.Names(paths) {
			a.fileNames[p] = name
		}

		for _, file := range snapshot.GetManifest().GetFiles() {
			a.sources[file.Path] = true
		}
	}
	for _, node := range diagram.GetNodes() {
		a.nodePaths[node.Key] = node.Path
	}
	for _, file := range diagram.Files {
		if source := helper.sources[file.Path]; source != nil {
			clone := proto.Clone(source).(*pb.SourceChange)
			clone.LinesAdded, clone.LinesRemoved = file.LinesAdded, file.LinesRemoved
			helper.sources[file.Path] = clone
		}
	}
	for _, kind := range []pb.ChangeKind{pb.ChangeKind_CHANGE_KIND_ADDED, pb.ChangeKind_CHANGE_KIND_REMOVED, pb.ChangeKind_CHANGE_KIND_MODIFIED} {
		for _, fact := range factsByKind(diagram.GetDiff().GetFacts(), kind) {
			if fact.Kind != pb.FactKind_FACT_KIND_FILE {
				a.scene.IndexedSymbols = append(a.scene.IndexedSymbols, symbolChange(kind, fact))
			}
		}
	}
	// Manifest-only fallback files lack indexed names. Disambiguate them
	// using the full captured repository, not the selected changed files.
	for p, name := range filelabel.Names(sortedKeys(a.sources)) {
		if a.fileNames[p] == "" {
			a.fileNames[p] = name
		}
	}
	a.sortedSources = sortedKeys(a.sources)
	a.ambiguousSorted = sortedKeys(a.ambiguousPaths)
	if err := a.seedAuthored(); err != nil {
		return nil, err
	}
	a.expandAuthoredChildren()
	a.fillMapped(false)
	a.fillRaw(false)
	a.expandDependencies()
	a.finishHierarchy()
	a.projectRelationships()
	a.placeAdditions()
	if err := a.validate(); err != nil {
		return nil, err
	}
	return a.scene, nil
}

func (a *sceneAssembly) record(stage, action, key, reason string) {
	if action != "suppress" && (stage != "dependencies" || !strings.Contains(reason, "Required owner")) {
		return
	}
	if action == "suppress" {
		parts := strings.Split(key, "|")
		if len(parts) >= 2 {
			viewID, _ := strconv.ParseInt(parts[1], 10, 64)
			ref := &model.AuditReference{ViewName: a.views[viewID].Name}
			if len(parts) == 2 {
				ref.Name = ref.ViewName
			} else if parts[0] == "placement" {
				for _, element := range a.helper.workspace.Views[parts[1]].Placements {
					if strconv.FormatInt(element.ElementID, 10) == parts[2] {
						ref.Name = element.Name
						break
					}
				}
			}
			a.scene.AuditReferences[key] = ref
		}
	}
	a.scene.Decisions = append(a.scene.Decisions, &model.Decision{Stage: stage, Action: action, Key: key, Reason: reason})
}

func (a *sceneAssembly) ensureView(id int64, stage string) *model.ViewContent {
	key := strconv.FormatInt(id, 10)
	if content := a.scene.Views[key]; content != nil {
		return content
	}
	a.scene.Views[key] = &model.ViewContent{}
	name, origin := "Changes", pb.ImpactOrigin_IMPACT_ORIGIN_RAW
	if view, exists := a.views[id]; exists {
		name, origin = view.Name, pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED
		if a.mapViews[id] {
			origin = pb.ImpactOrigin_IMPACT_ORIGIN_MAPPED
		}
	}
	provenance := &model.Metadata{Key: ViewKey(id), ObjectKind: "view", ViewId: id, ResourceId: id, Name: name, Origin: origin,
		Role: pb.ImpactRole_IMPACT_ROLE_CONTAINER}
	a.scene.Provenance[provenance.Key] = provenance
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
	match.scope = base
	if (element.RepositoryID == nil || *element.RepositoryID == "") && (element.Repo == nil || *element.Repo == "") {
		if a.ambiguousHas(base, folder) {
			match.grounding = pb.ImpactGrounding_IMPACT_GROUNDING_AMBIGUOUS
			return match
		}
	}
	anchor := sourcelink.Parse(*element.FilePath).Anchor
	match.scope = base
	match.anchor = anchor
	match.partial = anchor.Kind == sourcelink.AnchorSymbol || anchor.Kind == sourcelink.AnchorLine
	if !a.sources[base] {
		folder = a.hasSourceUnder(base)
	}
	match.contained = folder
	for _, path := range a.sourcesUnder(base, folder) {
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
			candidate := &model.Node{Symbols: &pb.CodeFactDelta{Modified: facts}}
			if symbolAnchorMatches(candidate, anchor) {
				resolved = true
			}
		}
		if !resolved {
			match.grounding = pb.ImpactGrounding_IMPACT_GROUNDING_UNRESOLVED
			match.paths = nil
		}
	}
	// Symbol links retain their navigation target, but v1 impact is file-level.
	// Once the symbol is verified, the authored placement covers the file even
	// when the change affected another symbol in it.
	if anchor.Kind == sourcelink.AnchorSymbol && len(match.paths) > 0 {
		match.partial = false
		match.nodes = nil
		for _, path := range match.paths {
			if node := a.helper.nodes[path]; node != nil {
				match.nodes = append(match.nodes, node)
			}
		}
	}
	return match
}

// ambiguousHas reports whether an unqualified path collides with a source from
// another repository, searching the sorted index instead of scanning it per
// placement.
func (a *sceneAssembly) ambiguousHas(base string, folder bool) bool {
	i := sort.SearchStrings(a.ambiguousSorted, base)
	if i < len(a.ambiguousSorted) && a.ambiguousSorted[i] == base {
		return true
	}
	if !folder {
		return false
	}
	prefix := base + "/"
	i = sort.SearchStrings(a.ambiguousSorted, prefix)
	return i < len(a.ambiguousSorted) && strings.HasPrefix(a.ambiguousSorted[i], prefix)
}

func (a *sceneAssembly) hasSourceUnder(base string) bool {
	prefix := base + "/"
	i := sort.SearchStrings(a.sortedSources, prefix)
	return i < len(a.sortedSources) && strings.HasPrefix(a.sortedSources[i], prefix)
}

// sourcesUnder returns the snapshot paths a placement's link covers: the exact
// path, plus, for folder links, every path beneath it. The sorted source index
// makes this a range scan instead of a full pass per placement.
func (a *sceneAssembly) sourcesUnder(base string, folder bool) []string {
	var out []string
	if i := sort.SearchStrings(a.sortedSources, base); i < len(a.sortedSources) && a.sortedSources[i] == base {
		out = append(out, base)
	}
	if !folder {
		return out
	}
	prefix := base + "/"
	for i := sort.SearchStrings(a.sortedSources, prefix); i < len(a.sortedSources) && strings.HasPrefix(a.sortedSources[i], prefix); i++ {
		out = append(out, a.sortedSources[i])
	}
	return out
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
				a.record("authored", "suppress", ViewKey(id), "Outside selected authored view scope")
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
				a.record("authored", "suppress", PlacementKey(id, element.ElementID), "Placement belongs to another repository")
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

func (a *sceneAssembly) addPlacement(item *model.Placement, origin pb.ImpactOrigin, stage, reason string, match anchorMatch) string {
	element := item.Element
	key := PlacementKey(int64(element.ViewId), int64(element.ElementId))
	if a.scene.Provenance[key] != nil {
		return key
	}
	content := a.ensureView(int64(element.ViewId), stage)
	content.Placements = append(content.Placements, item)
	p := &model.Metadata{Key: key, ObjectKind: "placement", ViewId: int64(element.ViewId), ResourceId: int64(element.ElementId), Name: element.Name,
		Origin: origin, Grounding: match.grounding, Role: pb.ImpactRole_IMPACT_ROLE_AUTHORED_CONTEXT,
		Paths: append([]string(nil), match.paths...), Partial: match.partial}
	a.scene.Provenance[key] = p
	a.captureMatch(item.Element, match, "accepted", reason)
	if origin == pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED {
		a.authoredKeys = append(a.authoredKeys, key)
		paths := make(map[string]bool, len(match.paths))
		for _, path := range match.paths {
			paths[path] = true
		}
		a.authoredPaths[key] = paths
	}
	a.record(stage, "add", key, reason)
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
	}
	return key
}

func (a *sceneAssembly) matchOverlay(match anchorMatch) *model.Overlay {
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
				if symbolAnchorMatches(&model.Node{Symbols: &pb.CodeFactDelta{Modified: []*pb.CodeFact{fact}}}, match.anchor) {
					*entry.target = append(*entry.target, fact)
				}
			}
		}
		out.Symbols = a.helper.overlay(&model.Node{Symbols: filtered}).Symbols
	}
	if len(match.nodes) == 1 {
		if match.contained {
			out.Contained = true
			out.Path = match.scope + "/"
		}
		return out
	}
	out = &model.Overlay{Path: strings.Join(match.paths, ", "), Change: first.Change, Contained: true}
	var added, removed uint32
	complete := true
	for _, node := range match.nodes {
		if node.Change != out.Change {
			out.Change = pb.ChangeKind_CHANGE_KIND_MODIFIED
		}
		overlay := a.helper.overlay(node)
		complete = complete && overlay.LinesAdded != nil && overlay.LinesRemoved != nil
		added += overlay.GetLinesAdded()
		removed += overlay.GetLinesRemoved()
		out.Symbols = append(out.Symbols, overlay.Symbols...)
	}
	if match.contained {
		out.Path = match.scope + "/"
	}
	if complete {
		out.LinesAdded, out.LinesRemoved = proto.Uint32(added), proto.Uint32(removed)
	}
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
			var relevant []*model.Node
			for _, node := range match.nodes {
				if (node.Distance > 0) == contextOnly {
					relevant = append(relevant, node)
				}
			}
			if len(relevant) == 0 {
				continue
			}
			if mapping.SnapshotID != a.helper.diagram.GetDiff().GetFromSnapshotId() && mapping.SnapshotID != a.helper.diagram.GetDiff().GetToSnapshotId() {
				a.record(stage, "suppress", PlacementKey(id, element.ElementID), "Stale map snapshot; using verified fallback")
				a.captureMatch(scenePlacement(element).Element, match, "rejected", "stale_map")
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
				a.record(stage, "suppress", PlacementKey(id, element.ElementID), "Scope already represented at higher priority")
				a.captureMatch(scenePlacement(element).Element, match, "rejected", "higher_priority_representative")
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

func (a *sceneAssembly) destination(node *model.Node) (int64, string) {
	a.prepareDestination()
	witnessCounts := map[string]int{}
	for _, witness := range node.Witnesses {
		for _, step := range witness.Paths {
			witnessCounts[step]++
		}
	}
	best, bestScore := int64(0), 0
	for _, key := range a.authoredKeys {
		p := a.scene.Provenance[key]
		paths := a.authoredPaths[key]
		if p == nil || len(paths) == 0 {
			continue
		}
		score := 0
		if paths[node.Path] {
			score += 100
		}
		for path := range paths {
			score += witnessCounts[path]
			score += 10 * a.edgeScores[node.Path][path]
		}
		if score > bestScore || (score > 0 && score == bestScore && p.ViewId < best) {
			best, bestScore = p.ViewId, score
		}
	}
	if bestScore > 0 {
		return best, "Placed beside authored source coverage or dependency witness"
	}
	if a.viewIdx == nil {
		index := buildViewIndex(a.helper.workspace, a.helper.mappings)
		a.viewIdx = &index
	}
	if id := a.viewIdx.closest(node.Path); id != 0 {
		if _, ok := a.views[id]; ok {
			if node.Distance == 0 {
				if sibling := a.consolidateRawDestination(node, id); sibling != 0 {
					return sibling, "Consolidated compatible raw file into its selected mapped source container"
				}
			}
			return id, "Placed in nearest mapped source container"
		}
	}
	return -1, "No authored or mapped destination; using Changes view"
}

// prepareDestination indexes the diagram's file edges once for destination
// scoring. The previous implementation walked every edge and re-sorted the
// provenance map per candidate node, which made assembly quadratic on large
// comparisons.
func (a *sceneAssembly) prepareDestination() {
	if a.edgeScores != nil {
		return
	}
	a.edgeScores = map[string]map[string]int{}
	for _, edge := range a.helper.diagram.Edges {
		from, to := a.nodePath(edge.FromKey), a.nodePath(edge.ToKey)
		if from == "" || to == "" || from == to {
			continue
		}
		a.bumpEdgeScore(from, to)
		a.bumpEdgeScore(to, from)
	}
}

func (a *sceneAssembly) bumpEdgeScore(from, to string) {
	if a.edgeScores[from] == nil {
		a.edgeScores[from] = map[string]int{}
	}
	a.edgeScores[from][to]++
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
		item := &model.Placement{Element: &diagv1.PlacedElement{Id: elementID, ElementId: elementID, ViewId: int32(id), Name: a.rawFileName(node.Path),
			Kind: proto.String("component"), FilePath: proto.String(node.Path), RepositoryId: proto.String(a.scene.RepositoryId), Repo: proto.String(a.helper.root)}}
		match := anchorMatch{nodes: []*model.Node{node}, paths: []string{node.Path}, grounding: pb.ImpactGrounding_IMPACT_GROUNDING_RESOLVED}
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
	return a.nodePaths[key]
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
				a.record("relationships", "suppress", key, "Connector endpoint is outside the impact scene")
				continue
			}
			origin := pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED
			if _, mapped := a.mapConnectors[connector.ID]; mapped {
				origin = pb.ImpactOrigin_IMPACT_ORIGIN_MAPPED
			}
			item := sceneConnector(connector)
			content.Connectors = append(content.Connectors, item)
			p := &model.Metadata{Key: key, ObjectKind: "connector", ViewId: id, ResourceId: connector.ID, Name: filePathValue(connector.Label),
				Origin: origin, Grounding: pb.ImpactGrounding_IMPACT_GROUNDING_UNRESOLVED}
			a.scene.Provenance[key] = p
		}
	}
	for _, edge := range a.helper.diagram.Edges {
		fromPath, toPath := a.nodePath(edge.FromKey), a.nodePath(edge.ToKey)
		for _, fromKey := range a.full[fromPath] {
			for _, toKey := range a.full[toPath] {
				from, to := a.scene.Provenance[fromKey], a.scene.Provenance[toKey]
				if fromKey == toKey {
					continue
				}
				// Preserve true endpoints across views. The renderer uses the
				// navigation graph to project cross-view connectors.
				content := a.scene.Views[strconv.FormatInt(from.ViewId, 10)]
				var connector *diagv1.Connector
				for _, candidate := range content.Connectors {
					if connectorRepresentsDependency(candidate, int32(from.ResourceId), int32(to.ResourceId)) {
						connector = candidate
						break
					}
				}
				if connector == nil {
					a.nextConnector--
					connector = &diagv1.Connector{Id: a.nextConnector, ViewId: int32(from.ViewId), SourceElementId: int32(from.ResourceId), TargetElementId: int32(to.ResourceId), Direction: "forward", Style: "bezier"}
					content.Connectors = append(content.Connectors, connector)
					key := ConnectorKey(from.ViewId, int64(connector.Id))
					a.scene.Provenance[key] = &model.Metadata{Key: key, ObjectKind: "connector", ViewId: from.ViewId, ResourceId: int64(connector.Id), Origin: pb.ImpactOrigin_IMPACT_ORIGIN_RAW}
				}
				p := a.scene.Provenance[ConnectorKey(from.ViewId, int64(connector.Id))]
				p.Grounding = pb.ImpactGrounding_IMPACT_GROUNDING_RESOLVED
				p.EndpointKeys = []string{fromKey, toKey}
				p.Paths = appendTagUnique(appendTagUnique(p.Paths, fromPath), toPath)
				connector.Tags = appendTagUnique(connector.Tags, connectorChangeTag(edge.Change))
			}
		}
	}
}

// connectorRepresentsDependency compares the effective direction of a
// workspace connector with a directed dependency. Mermaid mappings can store
// the same edge with reversed endpoints and Direction="backward"; treating
// that as a second edge would duplicate one logical relationship in the scene.
func connectorRepresentsDependency(connector *diagv1.Connector, source, target int32) bool {
	if connector == nil {
		return false
	}
	from, to := connector.GetSourceElementId(), connector.GetTargetElementId()
	switch strings.ToLower(strings.TrimSpace(connector.GetDirection())) {
	case "backward":
		from, to = to, from
	case "both", "bidirectional":
		return (from == source && to == target) || (from == target && to == source)
	case "", "forward":
	default:
		return false
	}
	return from == source && to == target
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
	for _, file := range a.scene.Files {
		keys := a.full[file.Path]
		if len(keys) == 0 {
			return fmt.Errorf("impact coverage missing changed file %s", file.Path)
		}
		for _, key := range keys {
			file.Representatives = append(file.Representatives, placementRef(key))
		}
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
	for _, match := range a.scene.Matches {
		for _, coverage := range a.scene.Files {
			if coverage.Path == match.Path {
				match.Representatives = append(match.Representatives, coverage.Representatives...)
			}
		}
	}
	return nil
}

func (a *sceneAssembly) rawFileName(p string) string {
	if name := a.fileNames[p]; name != "" {
		return name
	}
	return transientName(p)
}
