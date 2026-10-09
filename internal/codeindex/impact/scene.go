package impact

import (
	"context"
	"path"
	"sort"
	"strconv"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/internal/layout"
	"github.com/mertcikla/tld/v2/internal/sourcelink"
	"google.golang.org/protobuf/proto"
)

// SceneSchemaVersion identifies the portable scene contract. Bump it when the
// scene shape changes so a stored artifact stays readable by later readers.
const SceneSchemaVersion = "1"

// retiredImpactViewMarker identifies views owned by the retired impact
// materializer. They are hidden while an older watcher is still running.
const retiredImpactViewMarker = " impact · "

// Scene assembles the transient repository change scene for a persisted
// comparison: the repository's workspace subset, its blast-radius neighbours,
// transient placements for changed files that have no workspace element, and
// per-placement change overlays with hop distances. The result is
// self-contained: a viewer renders it without the repository, its index, or its
// snapshots. Nothing is persisted and no workspace resource is written; callers
// scope the scene by radius and standard/plain view locally.
func (s Service) Scene(ctx context.Context, diagram *pb.ImpactDiagram) (*pb.ImpactScene, error) {
	if diagram == nil {
		return &pb.ImpactScene{SchemaVersion: SceneSchemaVersion}, nil
	}
	repo, err := s.Index.Repository(ctx, diagram.GetRepositoryId())
	if err != nil {
		return nil, err
	}
	workspace, err := s.Workspace.Explore(ctx)
	if err != nil {
		return nil, err
	}
	mappings, err := s.Index.MappingsByRepository(ctx, diagram.GetRepositoryId())
	if err != nil {
		return nil, err
	}
	builder := sceneBuilder{diagram: diagram, workspace: workspace, root: repo.Root, mappings: mappings}
	return builder.build(), nil
}

type sceneBuilder struct {
	diagram   *pb.ImpactDiagram
	workspace core.ExploreData
	root      string
	mappings  []cstore.ResourceMapping

	nodes      map[string]*pb.ImpactNode
	sources    map[string]*pb.SourceChange
	contextIDs map[int64]bool
	matched    map[string]bool
	retained   map[int64]bool

	// Codeindex provenance: workspace resources the map pipeline owns. A view
	// is authored when it is not a generated map view and carries a change
	// overlay on a non-generated element.
	mappedElements map[int64]bool
	mappedViews    map[int64]bool
	authored       map[int64]bool
	// authoredOverlays memoizes the overlay match per element. An absent key
	// means the element hasn't been checked; a nil value means it has no
	// authored overlay.
	authoredOverlays map[int64]*pb.ImpactSceneOverlay
	// prefixes holds every directory prefix of a directly changed path, so
	// folder-granularity authored links match in constant time.
	prefixes map[string]bool
}

func (b *sceneBuilder) build() *pb.ImpactScene {
	b.index()
	diff := b.diagram.GetDiff()
	scene := &pb.ImpactScene{
		Views:           map[string]*pb.SceneViewContent{},
		Navigations:     []*diagv1.ElementNavigationInfo{},
		SchemaVersion:   SceneSchemaVersion,
		RepositoryId:    b.diagram.GetRepositoryId(),
		ComparisonKey:   b.diagram.GetComparisonKey(),
		Version:         b.diagram.GetVersion(),
		FromGitRevision: diff.GetFromGitRevision(),
		ToGitRevision:   diff.GetToGitRevision(),
	}
	for _, view := range b.pruneTree(b.workspace.Tree) {
		scene.Tree = append(scene.Tree, sceneView(view))
	}
	for _, viewID := range b.retainedIDs() {
		b.appendView(scene, viewID)
	}
	for _, link := range b.workspace.Navigations {
		if !b.retained[link.FromViewID] || !b.retained[link.ToViewID] {
			continue
		}
		scene.Navigations = append(scene.Navigations, sceneNavigation(link))
	}
	b.placeMissing(scene)
	for _, content := range scene.Views {
		adjustSceneConnectorHandles(content.Placements, content.Connectors)
	}
	// The radius a reader may still select is bounded by what this scene
	// actually carries, not by the wider neighbourhood it was scoped from.
	scene.MaxRadius = sceneMaxRadius(scene)
	// Authored views are sorted so the payload is deterministic.
	scene.AuthoredViewIds = make([]int64, 0, len(b.authored))
	for viewID := range b.authored {
		scene.AuthoredViewIds = append(scene.AuthoredViewIds, viewID)
	}
	sort.Slice(scene.AuthoredViewIds, func(i, j int) bool { return scene.AuthoredViewIds[i] < scene.AuthoredViewIds[j] })
	return scene
}

// sceneMaxRadius is the highest overlay distance present in the scene. Only
// direct changes (distance 0) means the reader has no radius left to widen.
func sceneMaxRadius(scene *pb.ImpactScene) uint32 {
	maxRadius := uint32(0)
	for _, content := range scene.Views {
		for _, placement := range content.GetPlacements() {
			if distance := placement.GetOverlay().GetDistance(); distance > maxRadius {
				maxRadius = distance
			}
		}
	}
	return maxRadius
}

func (b *sceneBuilder) index() {
	b.nodes = map[string]*pb.ImpactNode{}
	b.sources = map[string]*pb.SourceChange{}
	b.contextIDs = map[int64]bool{}
	b.matched = map[string]bool{}
	b.retained = map[int64]bool{}
	b.mappedElements = map[int64]bool{}
	b.mappedViews = map[int64]bool{}
	b.authored = map[int64]bool{}
	b.authoredOverlays = map[int64]*pb.ImpactSceneOverlay{}
	b.prefixes = map[string]bool{}
	for _, mapping := range b.mappings {
		// The retired impact materializer's resources are never treated as
		// owned by the map pipeline.
		if strings.HasPrefix(mapping.LogicalKey, "impact|") {
			continue
		}
		switch mapping.Kind {
		case cstore.MappingElement:
			b.mappedElements[mapping.ResourceID] = true
		case cstore.MappingView:
			if strings.HasPrefix(mapping.LogicalKey, "map|") {
				b.mappedViews[mapping.ResourceID] = true
			}
		}
	}
	for _, node := range b.diagram.GetNodes() {
		if node == nil {
			continue
		}
		b.nodes[normalizeScenePath(node.GetPath())] = node
		if node.GetDistance() > 0 && node.GetElementId() != 0 {
			b.contextIDs[node.GetElementId()] = true
		}
		if node.GetDistance() == 0 {
			path := strings.TrimSuffix(normalizeScenePath(node.GetPath()), "/")
			for rest := path; ; {
				slash := strings.LastIndexByte(rest, '/')
				if slash < 0 {
					break
				}
				rest = rest[:slash]
				b.prefixes[rest] = true
			}
		}
	}
	for _, source := range b.diagram.GetDiff().GetSources() {
		if source != nil {
			b.sources[source.GetPath()] = source
		}
	}
}

func (b *sceneBuilder) retainedIDs() []int64 {
	out := make([]int64, 0, len(b.retained))
	for id := range b.retained {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// belongs reports whether a placement is part of the compared repository or an
// eligible blast-radius neighbour. Mirrors the frontend membership rule.
func (b *sceneBuilder) belongs(element core.PlacedElement) bool {
	if element.RepositoryID != nil && *element.RepositoryID != "" {
		return *element.RepositoryID == b.diagram.GetRepositoryId()
	}
	if element.Repo != nil && normalizeScenePath(*element.Repo) == normalizeScenePath(b.root) {
		return true
	}
	return b.contextIDs[element.ElementID]
}

// pruneTree drops views with no repository placement and no retained children,
// plus views owned by the retired impact materializer. A view is also kept
// when a user-authored element in it matches a changed path, so hand-drawn
// diagrams survive the membership filter.
func (b *sceneBuilder) pruneTree(nodes []core.ViewTreeNode) []core.ViewTreeNode {
	out := make([]core.ViewTreeNode, 0, len(nodes))
	for _, view := range nodes {
		if strings.Contains(view.Name, retiredImpactViewMarker) {
			continue
		}
		children := b.pruneTree(view.Children)
		content := b.workspace.Views[strconv.FormatInt(view.ID, 10)]
		visible := false
		for _, placement := range content.Placements {
			if b.belongs(placement) {
				visible = true
				break
			}
		}
		if !visible {
			for _, placement := range content.Placements {
				if b.mappedElements[placement.ElementID] {
					continue
				}
				if b.matchAuthoredOverlay(placement) != nil {
					visible = true
					break
				}
			}
		}
		if len(children) == 0 && !visible {
			continue
		}
		b.retained[view.ID] = true
		view.Children = children
		out = append(out, view)
	}
	return out
}

// appendView copies a retained view's placements and connectors, attaching a
// change overlay to every placement that matches a diagram node. Placements
// the generated map does not own are resolved with the authored matcher, so an
// authored element anchored to a file, symbol, line, or folder is annotated
// the same way a mapped element is. A retained view that is not a generated
// map view and carries an overlay is recorded as authored.
func (b *sceneBuilder) appendView(scene *pb.ImpactScene, viewID int64) {
	content := b.workspace.Views[strconv.FormatInt(viewID, 10)]
	placements := make([]*pb.ScenePlacement, 0, len(content.Placements))
	carriesOverlay := false
	for _, placement := range content.Placements {
		item := scenePlacement(placement)
		if b.belongs(placement) {
			if node := b.nodes[normalizeScenePath(filePathValue(placement.FilePath))]; node != nil {
				if node.GetDistance() == 0 {
					b.matched[node.GetPath()] = true
				}
				item.Overlay = b.overlay(node)
			}
		}
		if item.Overlay == nil && !b.mappedElements[placement.ElementID] {
			if overlay := b.matchAuthoredOverlay(placement); overlay != nil {
				item.Overlay = overlay
			}
		}
		if item.Overlay != nil {
			carriesOverlay = true
		}
		placements = append(placements, item)
	}
	if carriesOverlay && !b.mappedViews[viewID] {
		b.authored[viewID] = true
	}
	connectors := make([]*diagv1.Connector, 0, len(content.Connectors))
	for _, connector := range content.Connectors {
		connectors = append(connectors, sceneConnector(connector))
	}
	scene.Views[strconv.FormatInt(viewID, 10)] = &pb.SceneViewContent{Placements: placements, Connectors: connectors}
}

// placeMissing appends transient placements for directly changed files that
// have no workspace placement, either to the diagram's closest view or to a
// synthetic "Changes" view when no view was resolved.
func (b *sceneBuilder) placeMissing(scene *pb.ImpactScene) {
	missing := make([]*pb.ImpactNode, 0)
	for _, node := range b.diagram.GetNodes() {
		if node == nil || node.GetDistance() > 0 || b.matched[node.GetPath()] {
			continue
		}
		missing = append(missing, node)
	}
	if len(missing) == 0 {
		return
	}
	targetID := b.diagram.GetViewId()
	target := scene.Views[strconv.FormatInt(targetID, 10)]
	useTarget := target != nil && b.retained[targetID]
	viewID := int64(-1)
	if useTarget {
		viewID = targetID
	}
	ids := make(map[string]int64, len(missing))
	for index, node := range missing {
		ids[node.GetKey()] = -int64(index + 1)
	}
	placements := make([]*pb.ScenePlacement, 0, len(missing))
	for index, node := range missing {
		id := ids[node.GetKey()]
		x, y := float64((index%3)*240), float64((index/3)*150)
		if useTarget {
			x, y = node.GetX(), node.GetY()
		}
		placements = append(placements, &pb.ScenePlacement{
			Element: &diagv1.PlacedElement{
				Id: int32(id), ElementId: int32(id), ViewId: int32(viewID),
				PositionX: x, PositionY: y,
				Name: node.GetName(), Kind: proto.String("component"),
				Description: proto.String(node.GetPath()), Repo: proto.String(b.root),
				FilePath: proto.String(node.GetPath()),
				Tags:     []string{},
			},
			Overlay: b.overlay(node),
		})
	}
	connectors := make([]*diagv1.Connector, 0)
	for _, edge := range b.diagram.GetEdges() {
		source, sourceOK := ids[edge.GetFromKey()]
		target, targetOK := ids[edge.GetToKey()]
		if !sourceOK || !targetOK {
			continue
		}
		connectors = append(connectors, &diagv1.Connector{
			Id: int32(-(len(connectors) + 1)), ViewId: int32(viewID),
			SourceElementId: int32(source), TargetElementId: int32(target),
			Label:     proto.String(strconv.FormatFloat(edge.GetWeight(), 'f', -1, 64) + " dependencies"),
			Direction: "forward", Style: "bezier",
		})
	}
	if useTarget {
		target.Placements = append(target.Placements, placements...)
		target.Connectors = append(target.Connectors, connectors...)
		return
	}
	scene.FallbackViewId = viewID
	scene.Tree = append(scene.Tree, &diagv1.View{Id: int32(viewID), Name: "Changes", Children: []*diagv1.View{}})
	scene.Views[strconv.FormatInt(viewID, 10)] = &pb.SceneViewContent{Placements: placements, Connectors: connectors}
}

// matchAuthoredOverlay resolves a change overlay for a placement the
// generated map does not own, so an authored element anchored to a file,
// symbol, line, or folder is annotated the same way a mapped element is.
// Results are memoized per element: an element carries the same file_path in
// every view, so pruneTree and appendView always agree. Every consumed
// direct-change node is recorded in matched, so placeMissing never duplicates
// a file that an authored element already represents.
func (b *sceneBuilder) matchAuthoredOverlay(placement core.PlacedElement) *pb.ImpactSceneOverlay {
	if overlay, checked := b.authoredOverlays[placement.ElementID]; checked {
		return overlay
	}
	overlay := b.authoredOverlayFor(placement)
	b.authoredOverlays[placement.ElementID] = overlay
	return overlay
}

func (b *sceneBuilder) authoredOverlayFor(placement core.PlacedElement) *pb.ImpactSceneOverlay {
	if placement.FilePath == nil {
		return nil
	}
	parsed := sourcelink.Parse(*placement.FilePath)
	raw := strings.ReplaceAll(strings.TrimSpace(parsed.BasePath), "\\", "/")
	if raw == "" {
		return nil
	}
	base, ok := b.repoRelativePath(strings.TrimSuffix(raw, "/"))
	if !ok {
		return nil
	}
	// A trailing slash is an explicit folder link; a path that names no
	// changed file but prefixes one is an implicit one. Both roll up.
	node := b.nodes[base]
	if strings.HasSuffix(raw, "/") || (node == nil && b.prefixes[base]) {
		return b.folderOverlay(base)
	}
	if node == nil || node.GetDistance() > 0 {
		return nil
	}
	switch parsed.Anchor.Kind {
	case sourcelink.AnchorNone:
		b.matched[node.GetPath()] = true
		return b.overlay(node)
	case sourcelink.AnchorLine, sourcelink.AnchorSymbol:
		if !symbolAnchorMatches(node, parsed.Anchor) {
			return nil
		}
		b.matched[node.GetPath()] = true
		return b.overlay(node)
	default:
		return nil
	}
}

// repoRelativePath normalizes an authored link's base path to the
// repository-relative form diagram nodes carry. Absolute paths that live under
// the compared repository's checkout are stripped to their relative form;
// relative paths are kept. Anything that resolves outside the repository
// returns false and never matches.
func (b *sceneBuilder) repoRelativePath(base string) (string, bool) {
	if strings.HasPrefix(base, "/") {
		if strings.TrimSpace(b.root) == "" {
			return "", false
		}
		root := strings.TrimSuffix(strings.ReplaceAll(b.root, "\\", "/"), "/")
		rel, ok := strings.CutPrefix(base, root+"/")
		if !ok {
			return "", false
		}
		base = rel
	}
	cleaned := path.Clean(base)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}

// symbolAnchorMatches reports whether a changed node carries a symbol change
// on the authored anchor: a symbol anchor matches by declaration name, a line
// anchor by overlap between the author's line range and a changed symbol's
// anchor lines. Authored line anchors are 1-based (editor convention) while
// stored fact anchors are 0-based, so the author range is shifted down first.
func symbolAnchorMatches(node *pb.ImpactNode, anchor sourcelink.Anchor) bool {
	if node == nil {
		return false
	}
	for _, kind := range []pb.ChangeKind{
		pb.ChangeKind_CHANGE_KIND_ADDED,
		pb.ChangeKind_CHANGE_KIND_REMOVED,
		pb.ChangeKind_CHANGE_KIND_MODIFIED,
	} {
		for _, fact := range factsByKind(node.GetSymbols(), kind) {
			if fact == nil {
				continue
			}
			switch anchor.Kind {
			case sourcelink.AnchorSymbol:
				if anchor.Symbol != "" && fact.GetName() == anchor.Symbol {
					return true
				}
			case sourcelink.AnchorLine:
				if anchor.StartLine <= 0 {
					continue
				}
				end := anchor.EndLine
				if end <= 0 {
					end = anchor.StartLine
				}
				if factAnchor := fact.GetAnchor(); factAnchor != nil &&
					int(factAnchor.GetStartLine()) <= end-1 && anchor.StartLine-1 <= int(factAnchor.GetEndLine()) {
					return true
				}
			}
		}
	}
	return false
}

// folderOverlay aggregates the direct changes under an authored folder link
// into one roll-up overlay. Only files that changed themselves are included:
// folders that merely neighbour a change stay unlit, keeping the authored
// toggle quiet. The change is the single kind present, or modified when the
// folder mixes kinds. Every consumed node is recorded in matched so
// placeMissing never duplicates a file the folder already represents.
func (b *sceneBuilder) folderOverlay(base string) *pb.ImpactSceneOverlay {
	if base == "" || !b.prefixes[base] {
		return nil
	}
	matched := make([]*pb.ImpactNode, 0)
	for _, node := range b.diagram.GetNodes() {
		if node == nil || node.GetDistance() > 0 {
			continue
		}
		changed := strings.TrimSuffix(normalizeScenePath(node.GetPath()), "/")
		if changed != base && !strings.HasPrefix(changed, base+"/") {
			continue
		}
		matched = append(matched, node)
	}
	if len(matched) == 0 {
		return nil
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].GetPath() < matched[j].GetPath() })
	overlay := &pb.ImpactSceneOverlay{Change: pb.ChangeKind_CHANGE_KIND_MODIFIED, Path: base + "/", Distance: 0, Reason: pb.OverlayReason_OVERLAY_REASON_CONTAINED}
	kinds := map[pb.ChangeKind]bool{}
	var linesAdded, linesRemoved uint32
	for _, node := range matched {
		kinds[node.GetChange()] = true
		if source := b.sources[node.GetPath()]; source != nil {
			if source.LinesAdded != nil {
				linesAdded += *source.LinesAdded
			}
			if source.LinesRemoved != nil {
				linesRemoved += *source.LinesRemoved
			}
		}
		overlay.Symbols = append(overlay.Symbols, b.overlay(node).GetSymbols()...)
		b.matched[node.GetPath()] = true
	}
	if len(kinds) == 1 {
		for kind := range kinds {
			overlay.Change = kind
		}
	}
	if linesAdded > 0 {
		overlay.LinesAdded = proto.Uint32(linesAdded)
	}
	if linesRemoved > 0 {
		overlay.LinesRemoved = proto.Uint32(linesRemoved)
	}
	return overlay
}

// overlay shapes a diagram node into the transient annotation the canvas
// renders. Context nodes keep their hop distance so clients can scope them.
// Symbol changes are carried structured: a reader labels them itself, so no
// pre-shaped text has to stay in sync with the underlying fact. Every overlay
// this helper shapes is a direct hit on its own path; roll-ups set CONTAINED
// themselves.
func (b *sceneBuilder) overlay(node *pb.ImpactNode) *pb.ImpactSceneOverlay {
	if node == nil {
		return nil
	}
	overlay := &pb.ImpactSceneOverlay{Change: node.GetChange(), Path: node.GetPath(), Distance: node.GetDistance(), Reason: pb.OverlayReason_OVERLAY_REASON_DIRECT}
	if source := b.sources[node.GetPath()]; source != nil {
		overlay.LinesAdded = source.LinesAdded
		overlay.LinesRemoved = source.LinesRemoved
	}
	delta := node.GetSymbols()
	for _, kind := range []pb.ChangeKind{
		pb.ChangeKind_CHANGE_KIND_ADDED,
		pb.ChangeKind_CHANGE_KIND_REMOVED,
		pb.ChangeKind_CHANGE_KIND_MODIFIED,
	} {
		for _, fact := range factsByKind(delta, kind) {
			if fact == nil {
				continue
			}
			overlay.Symbols = append(overlay.Symbols, symbolChange(kind, fact))
		}
	}
	return overlay
}

// symbolChange keeps only the portable identity of a changed fact: name, kind,
// anchor lines, and body fingerprint. Snapshot provenance, qualified SCIP
// names, evidence, and imports are left behind so a scene carries no
// index-only weight.
func symbolChange(kind pb.ChangeKind, fact *pb.CodeFact) *pb.ImpactSymbolChange {
	change := &pb.ImpactSymbolChange{
		Change:   kind,
		Name:     fact.GetName(),
		Kind:     fact.GetKind(),
		BodyHash: fact.GetBodyHash(),
	}
	if anchor := fact.GetAnchor(); anchor != nil {
		change.Anchor = &pb.ImpactSymbolAnchor{
			Path:      anchor.GetPath(),
			StartLine: anchor.GetStartLine(),
			EndLine:   anchor.GetEndLine(),
		}
	}
	return change
}

func sceneView(view core.ViewTreeNode) *diagv1.View {
	out := &diagv1.View{
		Id: int32(view.ID), OwnerElementId: int32Pointer(view.OwnerElementID), Name: view.Name,
		Description: view.Description, LevelLabel: view.LevelLabel,
		Tags: append([]string(nil), view.Tags...), Level: int32(view.Level), Depth: int32(view.Depth),
		ParentViewId: int32Pointer(view.ParentViewID),
		Children:     make([]*diagv1.View, 0, len(view.Children)),
	}
	for _, child := range view.Children {
		out.Children = append(out.Children, sceneView(child))
	}
	return out
}

func scenePlacement(placement core.PlacedElement) *pb.ScenePlacement {
	element := &diagv1.PlacedElement{
		Id: int32(placement.ID), ViewId: int32(placement.ViewID), ElementId: int32(placement.ElementID),
		PositionX: placement.PositionX, PositionY: placement.PositionY,
		Name: placement.Name, Description: placement.Description, Kind: placement.Kind,
		Technology: placement.Technology, Url: placement.URL, LogoUrl: placement.LogoURL,
		Tags: append([]string(nil), placement.Tags...),
		Repo: placement.Repo, RepositoryId: placement.RepositoryID, Branch: placement.Branch,
		FilePath: placement.FilePath, Language: placement.Language,
		HasView: placement.HasView, ViewLabel: placement.ViewLabel,
		BypassNoiseGate: placement.BypassNoiseGate,
	}
	for _, connector := range placement.TechnologyConnectors {
		slug := connector.Slug
		element.TechnologyLinks = append(element.TechnologyLinks, &diagv1.TechnologyLink{
			Type: connector.Type, Slug: &slug, Label: connector.Label, IsPrimaryIcon: connector.IsPrimaryIcon,
		})
	}
	return &pb.ScenePlacement{Element: element}
}

func sceneConnector(connector core.Connector) *diagv1.Connector {
	return &diagv1.Connector{
		Id: int32(connector.ID), ViewId: int32(connector.ViewID),
		SourceElementId: int32(connector.SourceElementID), TargetElementId: int32(connector.TargetElementID),
		Label: connector.Label, Description: connector.Description, Relationship: connector.Relationship,
		Direction: connector.Direction, Style: connector.Style, Url: connector.URL,
		SourceHandle: connector.SourceHandle, TargetHandle: connector.TargetHandle,
		Tags: append([]string(nil), connector.Tags...),
	}
}

func sceneNavigation(link core.ViewConnector) *diagv1.ElementNavigationInfo {
	return &diagv1.ElementNavigationInfo{
		Id: int32(link.ID), ElementId: int32Pointer(link.ElementID),
		FromViewId: int32(link.FromViewID), ToViewId: int32(link.ToViewID),
		ToViewName: link.ToViewName, RelationType: link.RelationType,
	}
}

// int32Pointer narrows an optional int64 id into the workspace proto's int32
// id space. Scene ids fit int32: workspace rows and transient negatives.
func int32Pointer(value *int64) *int32 {
	if value == nil {
		return nil
	}
	narrowed := int32(*value)
	return &narrowed
}

// adjustSceneConnectorHandles re-attaches connectors to the handles that yield
// the shortest anchor distance for the scene's final placements, mirroring the
// map pipeline's "Adjust Connectors" pass.
func adjustSceneConnectorHandles(placements []*pb.ScenePlacement, connectors []*diagv1.Connector) {
	if len(connectors) == 0 {
		return
	}
	positions := make(map[int64]layout.Placement, len(placements))
	for _, placement := range placements {
		element := placement.GetElement()
		positions[int64(element.GetElementId())] = layout.Placement{ElementID: int64(element.GetElementId()), X: element.GetPositionX(), Y: element.GetPositionY()}
	}
	for _, connector := range connectors {
		source, sourceOK := positions[int64(connector.GetSourceElementId())]
		target, targetOK := positions[int64(connector.GetTargetElementId())]
		if !sourceOK || !targetOK {
			continue
		}
		sourceHandle, targetHandle := layout.ChooseConnectorHandles(source, target)
		connector.SourceHandle = proto.String(sourceHandle)
		connector.TargetHandle = proto.String(targetHandle)
	}
}

func factsByKind(delta *pb.CodeFactDelta, kind pb.ChangeKind) []*pb.CodeFact {
	if delta == nil {
		return nil
	}
	switch kind {
	case pb.ChangeKind_CHANGE_KIND_ADDED:
		return delta.GetAdded()
	case pb.ChangeKind_CHANGE_KIND_REMOVED:
		return delta.GetRemoved()
	case pb.ChangeKind_CHANGE_KIND_MODIFIED:
		return delta.GetModified()
	default:
		return nil
	}
}

func filePathValue(path *string) string {
	if path == nil {
		return ""
	}
	return *path
}

func normalizeScenePath(path string) string {
	return strings.TrimSuffix(strings.ReplaceAll(path, "\\", "/"), "/")
}
