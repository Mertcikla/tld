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
	builder := sceneBuilder{diagram: diagram, workspace: workspace, root: repo.Root, mappings: mappings, store: s.Index, ctx: ctx}
	return builder.build(), nil
}

type sceneBuilder struct {
	diagram   *pb.ImpactDiagram
	workspace core.ExploreData
	root      string
	mappings  []cstore.ResourceMapping
	store     *cstore.Store
	ctx       context.Context

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
	// edgeChange records file-pair change kinds between diagram nodes, keyed
	// by normalized repo-relative paths, so workspace connectors spanning a
	// changed file relationship can be marked without a proto change.
	edgeChange map[[2]string]pb.ChangeKind
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
	b.pruneTree(b.workspace.Tree)
	b.expandChildViews()
	for _, view := range b.filterTree(b.workspace.Tree) {
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
	// Authored views are sorted so the payload is deterministic.
	scene.AuthoredViewIds = make([]int64, 0, len(b.authored))
	for viewID := range b.authored {
		scene.AuthoredViewIds = append(scene.AuthoredViewIds, viewID)
	}
	sort.Slice(scene.AuthoredViewIds, func(i, j int) bool { return scene.AuthoredViewIds[i] < scene.AuthoredViewIds[j] })
	return scene
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
	b.edgeChange = map[[2]string]pb.ChangeKind{}
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
	// Index file-pair change kinds by repo-relative path so connectors whose
	// endpoint files share a new, removed, or modified dependency can carry
	// the change as a tag. Node keys are file-scoped ("file|path") or
	// element-scoped ("context|<id>"); both resolve through the node path.
	pathByKey := map[string]string{}
	for _, node := range b.diagram.GetNodes() {
		if node == nil || node.GetKey() == "" {
			continue
		}
		pathByKey[node.GetKey()] = strings.TrimSuffix(normalizeScenePath(node.GetPath()), "/")
	}
	for _, edge := range b.diagram.GetEdges() {
		if edge == nil || edge.GetChange() == pb.ChangeKind_CHANGE_KIND_UNSPECIFIED {
			continue
		}
		from, okFrom := pathByKey[edge.GetFromKey()]
		to, okTo := pathByKey[edge.GetToKey()]
		if !okFrom || !okTo || from == "" || to == "" || from == to {
			continue
		}
		key := [2]string{from, to}
		if changeRank(edge.GetChange()) > changeRank(b.edgeChange[key]) {
			b.edgeChange[key] = edge.GetChange()
		}
	}
}

// changeRank orders file-pair change kinds so a conflicting pair keeps the
// strongest signal: additions first, then removals, then modifications.
func changeRank(change pb.ChangeKind) int {
	switch change {
	case pb.ChangeKind_CHANGE_KIND_ADDED:
		return 3
	case pb.ChangeKind_CHANGE_KIND_REMOVED:
		return 2
	case pb.ChangeKind_CHANGE_KIND_MODIFIED:
		return 1
	default:
		return 0
	}
}

// pairChange returns the file-pair change kind for two repo-relative paths,
// consulting both edge directions and keeping the stronger signal.
func (b *sceneBuilder) pairChange(first, second string) pb.ChangeKind {
	forward := b.edgeChange[[2]string{first, second}]
	backward := b.edgeChange[[2]string{second, first}]
	if changeRank(backward) > changeRank(forward) {
		return backward
	}
	return forward
}

// connectorChangeTag names the workspace-external change state of a scene
// connector. The tag rides the existing connector payload, so the canvas can
// style new/changed edges with no proto change. Empty for unchanged pairs.
func connectorChangeTag(change pb.ChangeKind) string {
	switch change {
	case pb.ChangeKind_CHANGE_KIND_ADDED:
		return "change:added"
	case pb.ChangeKind_CHANGE_KIND_REMOVED:
		return "change:removed"
	case pb.ChangeKind_CHANGE_KIND_MODIFIED:
		return "change:modified"
	default:
		return ""
	}
}

func appendTagUnique(tags []string, tag string) []string {
	if tag == "" {
		return tags
	}
	for _, existing := range tags {
		if existing == tag {
			return tags
		}
	}
	return append(tags, tag)
}

// linkBase parses a source link into its repo-relative base path and whether
// it addresses a folder (trailing slash) rather than a file. Absolute
// checkout paths are relativized against the repository root.
func (b *sceneBuilder) linkBase(link string) (base string, isFolder bool, ok bool) {
	parsed := sourcelink.Parse(strings.TrimSpace(link))
	raw := strings.ReplaceAll(strings.TrimSpace(parsed.BasePath), "\\", "/")
	if raw == "" {
		return "", false, false
	}
	isFolder = strings.HasSuffix(raw, "/")
	rel, relOK := b.repoRelativePath(raw)
	if !relOK || rel == "" {
		return "", false, false
	}
	return strings.TrimSuffix(rel, "/"), isFolder, true
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

// expandChildViews retains the drill-down views owned by elements in retained
// views, recursively, so the canvas can render them nested instead of as flat
// elements. A retained placement with a child view pulls that view's full
// output into the scene even when the child itself holds no repository
// placement; without this the parent renders as a singular flat element.
func (b *sceneBuilder) expandChildViews() {
	childByElement := map[int64]int64{}
	parentOf := map[int64]int64{}
	names := map[int64]string{}
	var walk func(nodes []core.ViewTreeNode)
	walk = func(nodes []core.ViewTreeNode) {
		for _, node := range nodes {
			names[node.ID] = node.Name
			if node.ParentViewID != nil {
				if _, ok := parentOf[node.ID]; !ok {
					parentOf[node.ID] = *node.ParentViewID
				}
			}
			if node.OwnerElementID != nil {
				if _, ok := childByElement[*node.OwnerElementID]; !ok {
					childByElement[*node.OwnerElementID] = node.ID
				}
			}
			walk(node.Children)
		}
	}
	walk(b.workspace.Tree)
	// Navigations mirror the same ownership; prefer them when present so the
	// parent selection matches Explore, but fall back to the tree owner map
	// when an element is placed in several views.
	navChildByElement := map[int64]int64{}
	for _, link := range b.workspace.Navigations {
		if link.RelationType != "child" || link.ElementID == nil {
			continue
		}
		if _, ok := navChildByElement[*link.ElementID]; !ok {
			navChildByElement[*link.ElementID] = link.ToViewID
		}
	}
	queue := b.retainedIDs()
	for len(queue) > 0 {
		from := queue[0]
		queue = queue[1:]
		content := b.workspace.Views[strconv.FormatInt(from, 10)]
		for _, placement := range content.Placements {
			child, ok := navChildByElement[placement.ElementID]
			if !ok {
				child, ok = childByElement[placement.ElementID]
			}
			if !ok {
				continue
			}
			if b.retained[child] {
				continue
			}
			b.retained[child] = true
			if strings.Contains(names[child], retiredImpactViewMarker) {
				delete(b.retained, child)
				continue
			}
			queue = append(queue, child)
			// Keep the tree path intact: ancestors of a newly retained
			// view must also be present for the hierarchy to traverse.
			for parent := parentOf[child]; parent != 0; parent = parentOf[parent] {
				if b.retained[parent] {
					break
				}
				if strings.Contains(names[parent], retiredImpactViewMarker) {
					break
				}
				b.retained[parent] = true
				queue = append(queue, parent)
			}
		}
	}
}

// filterTree rebuilds the scene tree from the final retained set, preserving
// the workspace hierarchy. Views retained via expandChildViews were pruned
// from the initial pass, so the tree is refiltered after expansion.
func (b *sceneBuilder) filterTree(nodes []core.ViewTreeNode) []core.ViewTreeNode {
	out := make([]core.ViewTreeNode, 0, len(nodes))
	for _, view := range nodes {
		if strings.Contains(view.Name, retiredImpactViewMarker) {
			continue
		}
		children := b.filterTree(view.Children)
		if !b.retained[view.ID] && len(children) == 0 {
			continue
		}
		if !b.retained[view.ID] {
			// Structural ancestor of a retained descendant: keep it so the
			// hierarchy traverses, and include its output below.
			b.retained[view.ID] = true
		}
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
	// Endpoint file paths resolve workspace connectors to file-pair changes:
	// a connector spanning a new, removed, or modified dependency carries the
	// change as a tag so readers can style it without a proto change.
	endpointBase := map[int64]string{}
	for _, placement := range content.Placements {
		if placement.FilePath == nil {
			continue
		}
		if base, _, ok := b.linkBase(*placement.FilePath); ok {
			if _, exists := endpointBase[placement.ElementID]; !exists {
				endpointBase[placement.ElementID] = base
			}
		}
	}
	connectors := make([]*diagv1.Connector, 0, len(content.Connectors))
	for _, connector := range content.Connectors {
		item := sceneConnector(connector)
		item.Tags = appendTagUnique(item.Tags, connectorChangeTag(
			b.connectorPairChange(connector.SourceElementID, connector.TargetElementID, endpointBase)))
		connectors = append(connectors, item)
	}
	scene.Views[strconv.FormatInt(viewID, 10)] = &pb.SceneViewContent{Placements: placements, Connectors: connectors}
}

// connectorPairChange resolves a workspace connector's endpoint files to the
// file-pair change kind, matching folder endpoints by prefix in either edge
// direction and keeping the stronger signal.
func (b *sceneBuilder) connectorPairChange(sourceID, targetID int64, endpointBase map[int64]string) pb.ChangeKind {
	source, okSource := endpointBase[sourceID]
	target, okTarget := endpointBase[targetID]
	if !okSource || !okTarget || source == "" || target == "" {
		return pb.ChangeKind_CHANGE_KIND_UNSPECIFIED
	}
	best := pb.ChangeKind_CHANGE_KIND_UNSPECIFIED
	consider := func(first, second string) {
		if change := b.pairChange(first, second); changeRank(change) > changeRank(best) {
			best = change
		}
	}
	consider(source, target)
	for pair := range b.edgeChange {
		if underPrefix(pair[0], source) && underPrefix(pair[1], target) {
			consider(pair[0], pair[1])
		} else if underPrefix(pair[0], target) && underPrefix(pair[1], source) {
			consider(pair[0], pair[1])
		}
	}
	return best
}

// underPrefix reports whether path equals base or lives under it, so folder
// endpoints match every file pair they contain.
func underPrefix(path, base string) bool {
	return path == base || strings.HasPrefix(path, base+"/")
}

// placeMissing appends transient placements for directly changed files that
// have no workspace placement. A file adjacent in the head snapshot's file
// graph to files the user's own elements cover is attached to the covering
// authored view, wired to the covering elements, so the canvas shows it where
// the user already looks. Anything without an authored neighbour keeps the
// previous behaviour: the diagram's closest view, or a synthetic "Changes"
// view when no view was resolved.
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
	ids := make(map[string]int64, len(missing))
	for index, node := range missing {
		ids[node.GetKey()] = -int64(index + 1)
	}
	remaining := b.attachCovered(scene, missing, ids)
	if len(remaining) == 0 {
		return
	}
	targetID := b.diagram.GetViewId()
	target := scene.Views[strconv.FormatInt(targetID, 10)]
	useTarget := target != nil && b.retained[targetID]
	viewID := int64(-1)
	if useTarget {
		viewID = targetID
	}
	placements := make([]*pb.ScenePlacement, 0, len(remaining))
	keep := make(map[string]bool, len(remaining))
	for _, node := range remaining {
		keep[node.GetKey()] = true
	}
	for index, node := range remaining {
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
		if !keep[edge.GetFromKey()] || !keep[edge.GetToKey()] {
			continue
		}
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
			Tags: appendTagUnique(nil, connectorChangeTag(edge.GetChange())),
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

// authoredCover is a user-owned placement whose linked file borders a missing
// change in the file graph.
type authoredCover struct {
	viewID    int64
	elementID int64
	weight    float64
	// pair is the file-pair change kind behind the cover, if the comparison
	// diagram carries it; otherwise the attachment is simply new evidence.
	pair pb.ChangeKind
}

// attachCovered places missing changes adjacent to user-covered files into
// the covering authored view, wired to the covering elements, and returns
// the nodes it did not place. Coverage comes from user-owned placements
// (file, symbol, line, or folder links) in non-generated views; adjacency
// comes from the head snapshot's file pairs, so this is one cheap aggregate
// query, never a path search. A view wins by covering-neighbour count with
// ties broken by view id, keeping the choice deterministic. Files with no
// authored neighbour fall through untouched.
func (b *sceneBuilder) attachCovered(scene *pb.ImpactScene, missing []*pb.ImpactNode, ids map[string]int64) []*pb.ImpactNode {
	pairs := b.filePairs()
	if len(pairs) == 0 {
		return missing
	}
	byPath := map[string][]authoredCover{}
	for viewIDStr, content := range b.workspace.Views {
		viewID, err := strconv.ParseInt(viewIDStr, 10, 64)
		if err != nil || b.mappedViews[viewID] {
			continue
		}
		for _, placement := range content.Placements {
			if placement.FilePath == nil || b.mappedElements[placement.ElementID] {
				continue
			}
			base, _, ok := b.linkBase(*placement.FilePath)
			if !ok {
				continue
			}
			byPath[base] = append(byPath[base], authoredCover{viewID: viewID, elementID: placement.ElementID})
		}
	}
	if len(byPath) == 0 {
		return missing
	}
	covers := func(neighbour string) []authoredCover {
		var out []authoredCover
		out = append(out, byPath[neighbour]...)
		for _, base := range sortedKeys(byPath) {
			if strings.HasPrefix(neighbour, base+"/") {
				out = append(out, byPath[base]...)
			}
		}
		return out
	}
	weight := func(a, c string) float64 {
		if w := pairs[[2]string{a, c}]; w > pairs[[2]string{c, a}] {
			return w
		}
		return pairs[[2]string{c, a}]
	}
	remaining := make([]*pb.ImpactNode, 0, len(missing))
	for _, node := range missing {
		path := node.GetPath()
		byView := map[int64][]authoredCover{}
		for _, neighbour := range sortedKeys(neighboursOf(pairs, path)) {
			for _, cover := range covers(neighbour) {
				cover.weight = weight(path, neighbour)
				cover.pair = b.pairChange(path, neighbour)
				byView[cover.viewID] = append(byView[cover.viewID], cover)
			}
		}
		best := int64(0)
		bestCount := 0
		for viewID, found := range byView {
			if len(found) > bestCount || (len(found) == bestCount && (best == 0 || viewID < best)) {
				best, bestCount = viewID, len(found)
			}
		}
		content := scene.Views[strconv.FormatInt(best, 10)]
		if bestCount == 0 || content == nil {
			remaining = append(remaining, node)
			continue
		}
		id := ids[node.GetKey()]
		index := len(content.GetPlacements())
		content.Placements = append(content.Placements, &pb.ScenePlacement{
			Element: &diagv1.PlacedElement{
				Id: int32(id), ElementId: int32(id), ViewId: int32(best),
				PositionX: float64((index % 3) * 240), PositionY: float64((index / 3) * 150),
				Name: node.GetName(), Kind: proto.String("component"),
				Description: proto.String(node.GetPath()), Repo: proto.String(b.root),
				FilePath: proto.String(node.GetPath()),
				Tags:     []string{},
			},
			Overlay: b.overlay(node),
		})
		seen := map[int64]bool{}
		ordered := append([]authoredCover(nil), byView[best]...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].elementID < ordered[j].elementID })
		for _, cover := range ordered {
			if seen[cover.elementID] {
				continue
			}
			seen[cover.elementID] = true
			// The attachment is new scene evidence; when the underlying file
			// pair itself changed, its kind is the more precise signal.
			change := cover.pair
			if change == pb.ChangeKind_CHANGE_KIND_UNSPECIFIED {
				change = pb.ChangeKind_CHANGE_KIND_ADDED
			}
			content.Connectors = append(content.Connectors, &diagv1.Connector{
				Id: int32(-(len(content.GetConnectors()) + 1)), ViewId: int32(best),
				SourceElementId: int32(cover.elementID), TargetElementId: int32(id),
				Label:     proto.String(strconv.FormatFloat(cover.weight, 'f', -1, 64) + " dependencies"),
				Direction: "forward", Style: "bezier",
				Tags: appendTagUnique(nil, connectorChangeTag(change)),
			})
		}
	}
	return remaining
}

// neighboursOf yields every path sharing a file pair with path. The pairs
// map is undirected: FilePairCounts stores ordered pairs, so both directions
// are consulted.
func neighboursOf(pairs map[[2]string]float64, path string) map[string]bool {
	out := map[string]bool{}
	for pair := range pairs {
		if pair[0] == path {
			out[pair[1]] = true
		} else if pair[1] == path {
			out[pair[0]] = true
		}
	}
	return out
}

// sortedKeys returns the sorted keys of a string-keyed set or index, keeping
// scene assembly deterministic across runs.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// filePairs loads the head snapshot's file adjacency. Any failure degrades to
// the previous fallback behaviour, never to an error.
func (b *sceneBuilder) filePairs() map[[2]string]float64 {
	if b.ctx == nil || b.store == nil {
		return nil
	}
	snapshotID := b.diagram.GetDiff().GetToSnapshotId()
	if snapshotID == "" {
		return nil
	}
	pairs, err := b.store.FilePairCounts(b.ctx, snapshotID)
	if err != nil {
		return nil
	}
	return pairs
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
