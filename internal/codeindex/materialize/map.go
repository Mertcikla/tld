package materialize

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mertcikla/tld/v2/internal/codeindex/community"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/internal/layout"
)

// MapInput is the shared state the map materializer works against. Files are
// the repository's file facts; Edges and Imports are resolved dependencies.
type MapInput struct {
	RepositoryID   string
	RepositoryName string
	RepositoryRoot string
	SnapshotID     string
	Files          []community.File
	// Edges are file-to-file dependencies resolved from symbol edges.
	Edges []MapEdge
	// Imports are external imports declared by the materialized files. They are
	// only populated when the caller opts in.
	Imports []MapImport
}

// MapEdge is a dependency between two file facts (the file facts themselves,
// not the symbols). The connector direction is derived by de-duplicating and
// merging opposite directions.
type MapEdge struct {
	FromFactID string
	ToFactID   string
	Weight     float64
}

// MapImport is one external import declared by a file fact.
type MapImport struct {
	FileFactID string
	Import     string
}

// MapOptions configures a mapper materialization run.
type MapOptions struct {
	ViewName string
	// MaxConnectorsPerView caps how many rolled-up connectors are drawn in any
	// one view, highest weight first, to keep dense views readable. Zero uses
	// DefaultMaxConnectorsPerView.
	MaxConnectorsPerView int
	// MaxLeafConnectorsPerView caps connectors in file-only views, where a dense
	// dependency web reads as a hairball. Zero falls back to
	// MaxConnectorsPerView.
	MaxLeafConnectorsPerView int
	// Progress receives coarse (current, total, detail) updates while resources
	// are created or updated. It may be nil.
	Progress func(current, total int, detail string)
}

// DefaultMaxConnectorsPerView is the per-view connector budget when unset.
const DefaultMaxConnectorsPerView = 40

// DefaultMaxLeafConnectorsPerView keeps file-level views legible; a group's
// internal coupling is summarized in its element description instead.
const DefaultMaxLeafConnectorsPerView = 12

// MapResult summarizes what changed.
type MapResult struct {
	ViewID     int64
	Elements   int
	Views      int
	Connectors int
	Pruned     int
}

const mapKeyPrefix = "map|"

type mapMaterializer struct {
	ctx              context.Context
	ws               core.Store
	idx              IndexStore
	input            MapInput
	opts             MapOptions
	byKey            map[string]cstore.ResourceMapping
	kept             map[string]bool
	mappingBuffers   []cstore.ResourceMapping
	pendingPlaces    []pendingPlacement
	placed           map[int64]map[int64]bool
	desiredPlaces    map[int64]map[int64]bool
	position         map[int64]int
	fileElementIDs   map[string]int64
	fileChains       map[string][]int64
	fileElementPath  map[string][]int64
	queued           map[int64]map[int64]bool
	layoutEdges      map[int64][]layout.Connector
	placedConnectors []placedConnector
	leafViews        map[int64]bool
	maxConnectors    int
	maxLeaf          int
	result           MapResult
	done             int
	total            int
}

// placedConnector records a generated connector so its handles can be
// re-attached to the nearest sides once the final layout is known.
type placedConnector struct {
	id     int64
	viewID int64
	source int64
	target int64
}

// pendingPlacement is an element awaiting layout. Placements are deferred until
// connectors exist so generated maps are laid out instead of grid-placed.
type pendingPlacement struct {
	viewID    int64
	elementID int64
}

// recordFile remembers where a file element sits: the chain of view ids from the
// map root to the view holding the file, and the element placed in each of those
// views that contains the file. Edge roll-up uses the first level where two
// files' elements differ.
func (m *mapMaterializer) recordFile(factID string, elementID int64, views, elements []int64) {
	m.fileElementIDs[factID] = elementID
	m.fileChains[factID] = append([]int64(nil), views...)
	m.fileElementPath[factID] = append([]int64(nil), elements...)
}

// appendView returns a new chain with viewID appended, safe to reuse.
func appendView(chain []int64, viewID int64) []int64 {
	out := make([]int64, len(chain)+1)
	copy(out, chain)
	out[len(chain)] = viewID
	return out
}

// appendElem returns a new chain with elementID appended, safe to reuse.
func appendElem(chain []int64, elementID int64) []int64 {
	out := make([]int64, len(chain)+1)
	copy(out, chain)
	out[len(chain)] = elementID
	return out
}

func maxConnectorsPerView(opts MapOptions) int {
	if opts.MaxConnectorsPerView > 0 {
		return opts.MaxConnectorsPerView
	}
	return DefaultMaxConnectorsPerView
}

func maxLeafConnectorsPerView(opts MapOptions) int {
	if opts.MaxLeafConnectorsPerView > 0 {
		return opts.MaxLeafConnectorsPerView
	}
	return maxConnectorsPerView(opts)
}

// queuePlacement defers an element's placement until connectors exist so the
// view can be laid out as a graph instead of a grid. Elements that already have
// a placement keep it.
func (m *mapMaterializer) queuePlacement(viewID, elementID int64) error {
	if m.desiredPlaces == nil {
		m.desiredPlaces = map[int64]map[int64]bool{}
	}
	if m.desiredPlaces[viewID] == nil {
		m.desiredPlaces[viewID] = map[int64]bool{}
	}
	m.desiredPlaces[viewID][elementID] = true
	existing, err := m.placementsFor(viewID)
	if err != nil {
		return err
	}
	if existing[elementID] {
		return nil
	}
	existing[elementID] = true
	if m.queued[viewID] == nil {
		m.queued[viewID] = map[int64]bool{}
	}
	if m.queued[viewID][elementID] {
		return nil
	}
	m.queued[viewID][elementID] = true
	m.pendingPlaces = append(m.pendingPlaces, pendingPlacement{viewID: viewID, elementID: elementID})
	return nil
}

// applyLayout positions every newly created element. Views whose every element
// is new get a deterministic force-directed layout with directed levels; views
// that keep existing elements only place the new ones next to their neighbors.
func (m *mapMaterializer) applyLayout() error {
	if len(m.pendingPlaces) == 0 {
		return nil
	}
	byView := map[int64][]int64{}
	for _, item := range m.pendingPlaces {
		byView[item.viewID] = append(byView[item.viewID], item.elementID)
	}
	views := make([]int64, 0, len(byView))
	for viewID := range byView {
		views = append(views, viewID)
	}
	sort.Slice(views, func(i, j int) bool { return views[i] < views[j] })
	for _, viewID := range views {
		existing, err := m.ws.ElementPlacements(m.ctx, viewID)
		if err != nil {
			return err
		}
		placements := make([]layout.Placement, 0, len(existing))
		for _, placement := range existing {
			placements = append(placements, layout.Placement{ElementID: placement.ElementID, X: placement.PositionX, Y: placement.PositionY})
		}
		targets := make(map[int64]struct{}, len(byView[viewID]))
		for _, elementID := range byView[viewID] {
			targets[elementID] = struct{}{}
		}
		next := layout.DeterministicLayoutPlacements(placements, targets, m.layoutEdges[viewID])
		for elementID := range targets {
			position, ok := next[elementID]
			if !ok {
				continue
			}
			if _, err := m.ws.AddPlacement(m.ctx, viewID, elementID, position.X, position.Y); err != nil {
				return fmt.Errorf("place map element %d in view %d: %w", elementID, viewID, err)
			}
		}
		m.advance("layout")
	}
	return nil
}

type elementEdgeKey struct {
	viewID int64
	a      int64
	b      int64
}

type elementEdge struct {
	viewID   int64
	a        int64
	b        int64
	weight   float64
	forward  bool
	backward bool
}

// materializeConnectors rolls file-level dependencies up the map hierarchy.
// Every edge is drawn at the deepest view where the two files fall under
// different visible child elements, connecting those two elements. Edges are
// aggregated per element pair (weight = number of underlying file edges,
// opposite directions merged) and each view is capped to its heaviest
// connectors, so dense views stay readable and internal edges never clutter a
// higher level.
func (m *mapMaterializer) materializeConnectors() error {
	if len(m.input.Edges) == 0 {
		return nil
	}
	grouped := map[elementEdgeKey]*elementEdge{}
	for _, edge := range m.input.Edges {
		if edge.FromFactID == "" || edge.ToFactID == "" || edge.FromFactID == edge.ToFactID {
			continue
		}
		fromViews, fromElems := m.fileChains[edge.FromFactID], m.fileElementPath[edge.FromFactID]
		toElems := m.fileElementPath[edge.ToFactID]
		if len(fromElems) == 0 || len(toElems) == 0 {
			continue
		}
		index := -1
		limit := min(len(fromElems), len(toElems))
		for i := 0; i < limit; i++ {
			if fromElems[i] != toElems[i] {
				index = i
				break
			}
		}
		if index < 0 {
			continue
		}
		a, b := fromElems[index], toElems[index]
		forward := true
		if a > b {
			a, b = b, a
			forward = false
		}
		key := elementEdgeKey{viewID: fromViews[index], a: a, b: b}
		entry := grouped[key]
		if entry == nil {
			entry = &elementEdge{viewID: key.viewID, a: a, b: b}
			grouped[key] = entry
		}
		if forward {
			entry.forward = true
		} else {
			entry.backward = true
		}
		weight := edge.Weight
		if weight <= 0 {
			weight = 1
		}
		entry.weight += weight
	}
	byView := map[int64][]*elementEdge{}
	for _, entry := range grouped {
		byView[entry.viewID] = append(byView[entry.viewID], entry)
	}
	views := make([]int64, 0, len(byView))
	for viewID := range byView {
		views = append(views, viewID)
	}
	sort.Slice(views, func(i, j int) bool { return views[i] < views[j] })
	for _, viewID := range views {
		entries := byView[viewID]
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].weight != entries[j].weight {
				return entries[i].weight > entries[j].weight
			}
			if entries[i].a != entries[j].a {
				return entries[i].a < entries[j].a
			}
			return entries[i].b < entries[j].b
		})
		limit := m.maxConnectors
		if m.leafViews[viewID] {
			limit = m.maxLeaf
		}
		if limit <= 0 || limit > len(entries) {
			limit = len(entries)
		}
		for _, entry := range entries[:limit] {
			direction := "forward"
			switch {
			case entry.forward && entry.backward:
				direction = "both"
			case entry.backward:
				direction = "backward"
			}
			if err := m.upsertConnector(elementConnectorKey(m.input.RepositoryID, entry.viewID, entry.a, entry.b), core.Connector{
				ViewID:          entry.viewID,
				SourceElementID: entry.a,
				TargetElementID: entry.b,
				Direction:       direction,
				Style:           "bezier",
			}); err != nil {
				return err
			}
			m.layoutEdges[entry.viewID] = append(m.layoutEdges[entry.viewID], layout.Connector{Source: entry.a, Target: entry.b})
		}
	}
	return nil
}

// materializeImports materializes external imports under a single External
// element: the External element owns a child view containing one element per
// distinct import. Importing top-level components are connected to the External
// container (rather than one connector per file/import), so the connector count
// stays bounded by the number of top-level components.
func (m *mapMaterializer) materializeImports(rootViewID int64) error {
	if len(m.input.Imports) == 0 {
		return nil
	}
	containerID, err := m.upsertElement(externalKey(m.input.RepositoryID), core.LibraryElement{
		Name:        "External",
		Kind:        strPtr("external"),
		Description: strPtr("External imports"),
	})
	if err != nil {
		return err
	}
	externalViewID, err := m.upsertView(externalViewKey(m.input.RepositoryID), "External", "Map", &containerID)
	if err != nil {
		return err
	}
	if err := m.queuePlacement(rootViewID, containerID); err != nil {
		return err
	}

	distinct := map[string]bool{}
	for _, item := range m.input.Imports {
		distinct[item.Import] = true
	}
	names := make([]string, 0, len(distinct))
	for name := range distinct {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		elementID, err := m.upsertElement(importKey(m.input.RepositoryID, name), core.LibraryElement{
			Name: name,
			Kind: strPtr("import"),
		})
		if err != nil {
			return err
		}
		if err := m.queuePlacement(externalViewID, elementID); err != nil {
			return err
		}
	}

	// Aggregate unique (file, import) usages by the file's top-level component
	// element. That element and the External container are both placed in the
	// map root view, so the rolled-up connectors are visible and capped by the
	// view's connector budget.
	type componentEdges struct {
		from   int64
		weight float64
	}
	seen := map[[2]string]bool{}
	grouped := map[int64]*componentEdges{}
	for _, item := range m.input.Imports {
		key := [2]string{item.FileFactID, item.Import}
		if seen[key] {
			continue
		}
		seen[key] = true
		path := m.fileElementPath[item.FileFactID]
		if len(path) < 2 {
			continue
		}
		entry := grouped[path[0]]
		if entry == nil {
			entry = &componentEdges{from: path[0]}
			grouped[path[0]] = entry
		}
		entry.weight++
	}
	entries := make([]*componentEdges, 0, len(grouped))
	for _, entry := range grouped {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].weight != entries[j].weight {
			return entries[i].weight > entries[j].weight
		}
		return entries[i].from < entries[j].from
	})
	limit := m.maxConnectors
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}
	for _, entry := range entries[:limit] {
		if err := m.upsertConnector(elementConnectorKey(m.input.RepositoryID, rootViewID, entry.from, containerID), core.Connector{
			ViewID:          rootViewID,
			SourceElementID: entry.from,
			TargetElementID: containerID,
			Direction:       "forward",
			Style:           "bezier",
		}); err != nil {
			return err
		}
		m.layoutEdges[rootViewID] = append(m.layoutEdges[rootViewID], layout.Connector{Source: entry.from, Target: containerID})
	}
	return nil
}

func (m *mapMaterializer) upsertConnector(logicalKey string, input core.Connector) error {
	m.kept[logicalKey] = true
	id := int64(0)
	if mapping, ok := m.byKey[logicalKey]; ok && mapping.Kind == cstore.MappingConnector {
		if updated, err := m.ws.UpdateConnector(m.ctx, mapping.ResourceID, connectorSourceOnly(input)); err == nil {
			id = updated.ID
			m.recordMapping(logicalKey, cstore.MappingConnector, id)
		}
	}
	if id == 0 {
		created, err := m.ws.CreateConnector(m.ctx, input)
		if err != nil {
			return fmt.Errorf("create map connector %q: %w", logicalKey, err)
		}
		id = created.ID
		m.mappingBuffers = append(m.mappingBuffers, cstore.ResourceMapping{
			LogicalKey:   logicalKey,
			Kind:         cstore.MappingConnector,
			ResourceID:   created.ID,
			RepositoryID: m.input.RepositoryID,
			SnapshotID:   m.input.SnapshotID,
		})
	}
	m.placedConnectors = append(m.placedConnectors, placedConnector{
		id:     id,
		viewID: input.ViewID,
		source: input.SourceElementID,
		target: input.TargetElementID,
	})
	m.result.Connectors++
	m.advance("connector")
	return nil
}

// adjustConnectorHandles re-attaches each generated connector to the source and
// target handle that yields the shortest anchor distance for the final layout,
// mirroring the view editor's "Adjust Connectors" pass so map connectors do not
// cross node bodies.
func (m *mapMaterializer) adjustConnectorHandles() error {
	if len(m.placedConnectors) == 0 {
		return nil
	}
	positionsByView := map[int64]map[int64]layout.Placement{}
	currentByView := map[int64]map[int64]core.Connector{}
	for _, connector := range m.placedConnectors {
		positions, ok := positionsByView[connector.viewID]
		if !ok {
			placements, err := m.ws.ElementPlacements(m.ctx, connector.viewID)
			if err != nil {
				return err
			}
			positions = make(map[int64]layout.Placement, len(placements))
			for _, placement := range placements {
				positions[placement.ElementID] = layout.Placement{ElementID: placement.ElementID, X: placement.PositionX, Y: placement.PositionY}
			}
			positionsByView[connector.viewID] = positions

			existing, err := m.ws.Connectors(m.ctx, connector.viewID)
			if err != nil {
				return err
			}
			current := make(map[int64]core.Connector, len(existing))
			for _, item := range existing {
				current[item.ID] = item
			}
			currentByView[connector.viewID] = current
		}
		source, sourceOK := positions[connector.source]
		target, targetOK := positions[connector.target]
		if !sourceOK || !targetOK {
			continue
		}
		sourceHandle, targetHandle := layout.ChooseConnectorHandles(source, target)
		if existing, ok := currentByView[connector.viewID][connector.id]; ok &&
			existing.SourceHandle != nil && *existing.SourceHandle == sourceHandle &&
			existing.TargetHandle != nil && *existing.TargetHandle == targetHandle {
			continue
		}
		if _, err := m.ws.UpdateConnector(m.ctx, connector.id, core.Connector{
			SourceHandle: &sourceHandle,
			TargetHandle: &targetHandle,
		}); err != nil {
			return fmt.Errorf("adjust connector %d handles: %w", connector.id, err)
		}
	}
	return nil
}

func (m *mapMaterializer) upsertElement(logicalKey string, input core.LibraryElement) (int64, error) {
	m.kept[logicalKey] = true
	if mapping, ok := m.byKey[logicalKey]; ok && mapping.Kind == cstore.MappingElement {
		if updated, err := m.ws.UpdateElement(m.ctx, mapping.ResourceID, sourceOnly(input)); err == nil {
			m.recordMapping(logicalKey, cstore.MappingElement, updated.ID)
			m.result.Elements++
			m.advance("element")
			return updated.ID, nil
		}
	}
	created, err := m.ws.CreateElement(m.ctx, input)
	if err != nil {
		return 0, fmt.Errorf("create map element %q: %w", logicalKey, err)
	}
	m.mappingBuffers = append(m.mappingBuffers, cstore.ResourceMapping{
		LogicalKey:   logicalKey,
		Kind:         cstore.MappingElement,
		ResourceID:   created.ID,
		RepositoryID: m.input.RepositoryID,
		SnapshotID:   m.input.SnapshotID,
	})
	m.result.Elements++
	m.advance("element")
	return created.ID, nil
}

func (m *mapMaterializer) upsertView(logicalKey, name, label string, ownerElementID *int64) (int64, error) {
	m.kept[logicalKey] = true
	if mapping, ok := m.byKey[logicalKey]; ok && mapping.Kind == cstore.MappingView {
		if node, err := m.ws.ViewByID(m.ctx, mapping.ResourceID); err == nil {
			if ownerMatches(node.OwnerElementID, ownerElementID) {
				// Preserve a user-renamed view: pass nil name/tags so the store
				// keeps them, while refreshing the generated level label.
				if _, err := m.ws.UpdateView(m.ctx, mapping.ResourceID, nil, nil, &label, nil); err == nil {
					m.recordMapping(logicalKey, cstore.MappingView, mapping.ResourceID)
					m.result.Views++
					m.advance("view")
					return mapping.ResourceID, nil
				}
			} else {
				// The view's owner changed (e.g. a map view created before it
				// was nested under the workspace root). Keep the user's name,
				// then recreate it so the hierarchy is correct.
				if strings.TrimSpace(node.Name) != "" {
					name = node.Name
				}
				_ = m.ws.DeleteView(m.ctx, mapping.ResourceID)
			}
		}
	}
	view, err := m.ws.CreateView(m.ctx, name, &label, ownerElementID)
	if err != nil {
		return 0, fmt.Errorf("create map view %q: %w", logicalKey, err)
	}
	m.mappingBuffers = append(m.mappingBuffers, cstore.ResourceMapping{
		LogicalKey:   logicalKey,
		Kind:         cstore.MappingView,
		ResourceID:   view.ID,
		RepositoryID: m.input.RepositoryID,
		SnapshotID:   m.input.SnapshotID,
	})
	m.result.Views++
	m.advance("view")
	return view.ID, nil
}

func (m *mapMaterializer) place(viewID, elementID int64) error {
	existing, err := m.placementsFor(viewID)
	if err != nil {
		return err
	}
	if existing[elementID] {
		return nil
	}
	index := m.position[viewID]
	m.position[viewID] = index + 1
	x, y := gridPositionCols(index, 5)
	if _, err := m.ws.AddPlacement(m.ctx, viewID, elementID, x, y); err != nil {
		return fmt.Errorf("place map element %d in view %d: %w", elementID, viewID, err)
	}
	existing[elementID] = true
	return nil
}

func (m *mapMaterializer) placementsFor(viewID int64) (map[int64]bool, error) {
	if existing, ok := m.placed[viewID]; ok {
		return existing, nil
	}
	list, err := m.ws.ElementPlacements(m.ctx, viewID)
	if err != nil {
		return nil, err
	}
	existing := make(map[int64]bool, len(list))
	for _, placement := range list {
		existing[placement.ElementID] = true
	}
	m.placed[viewID] = existing
	if _, ok := m.position[viewID]; !ok {
		m.position[viewID] = len(list)
	}
	return existing, nil
}

func (m *mapMaterializer) advance(detail string) {
	m.done++
	if m.opts.Progress != nil && (m.done%25 == 0 || m.done >= m.total) {
		m.opts.Progress(m.done, m.total, detail)
	}
}

func (m *mapMaterializer) fileElement(member int) core.LibraryElement {
	fact := m.input.Files[member]
	kind := "file"
	// File facts carry their path as the display name; show only the file name
	// and keep the full path on FilePath for source linking.
	name := fact.DisplayName
	if fact.Path != "" {
		name = folderName(fact.Path)
	}
	input := core.LibraryElement{
		Name: name,
		Kind: &kind,
	}
	if fact.Path != "" {
		path := fact.Path
		input.FilePath = &path
	}
	if fact.Language != "" {
		language := fact.Language
		input.Language = &language
	}
	if repo := m.input.RepositoryRoot; repo != "" {
		input.Repo = &repo
	} else if m.input.RepositoryName != "" {
		repo := m.input.RepositoryName
		input.Repo = &repo
	}
	if m.input.RepositoryID != "" {
		repositoryID := m.input.RepositoryID
		input.RepositoryID = &repositoryID
	}
	return input
}

func mapViewName(input MapInput) string {
	if input.RepositoryName != "" {
		return input.RepositoryName
	}
	return "Repository Map"
}

func mapTopElement(input MapInput) core.LibraryElement {
	kind := "map"
	description := "Repository map"
	if input.RepositoryRoot != "" {
		description = "Repository map · " + input.RepositoryRoot
	}
	return core.LibraryElement{Name: mapViewName(input), Kind: &kind, Description: &description}
}

// ownerMatches reports whether an existing view's owner satisfies the desired
// owner. A nil desired owner accepts any existing owner; a non-nil desired owner
// requires an exact match.
func ownerMatches(current, desired *int64) bool {
	if desired == nil {
		return true
	}
	return current != nil && *current == *desired
}

// workspaceRootViewID resolves the view new top-level elements should be placed
// in: the first root view (preferring one with no owner), excluding the map's
// own view. A workspace root is created if the database has no views at all.
func workspaceRootViewID(ctx context.Context, ws core.Store, exclude int64) (int64, error) {
	summaries, err := ws.Views(ctx)
	if err != nil {
		return 0, err
	}
	firstRoot := int64(0)
	fallback := int64(0)
	for _, summary := range summaries {
		if summary.ID == exclude {
			continue
		}
		if summary.IsRoot {
			if summary.Name == "Workspace" {
				return summary.ID, nil
			}
			if firstRoot == 0 {
				firstRoot = summary.ID
			}
		}
		if fallback == 0 {
			fallback = summary.ID
		}
	}
	if firstRoot != 0 {
		return firstRoot, nil
	}
	if fallback != 0 {
		return fallback, nil
	}
	label := "Root"
	view, err := ws.CreateView(ctx, "Workspace", &label, nil)
	if err != nil {
		return 0, fmt.Errorf("create workspace root view: %w", err)
	}
	return view.ID, nil
}

// folderName is the last path segment of a file path, so a file nested under
// A/B is shown as c.go rather than src/deep/c.go.
func folderName(path string) string {
	if path == "." || path == "" {
		return "root"
	}
	path = strings.ReplaceAll(path, "\\", "/")
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

func strPtr(value string) *string { return &value }

func gridPositionCols(index, cols int) (float64, float64) {
	if cols < 1 {
		cols = 1
	}
	col := index % cols
	row := index / cols
	return float64(120 + col*240), float64(120 + row*180)
}

func fileKey(repositoryID, factID string) string {
	return mapKeyPrefix + "fact|" + repositoryID + "|" + factID
}

func elementConnectorKey(repositoryID string, viewID, a, b int64) string {
	return fmt.Sprintf("%sconn|%s|%d|%d|%d", mapKeyPrefix, repositoryID, viewID, a, b)
}

func externalKey(repositoryID string) string {
	return mapKeyPrefix + "external|" + repositoryID
}

func externalViewKey(repositoryID string) string {
	return mapKeyPrefix + "externalview|" + repositoryID
}

func importKey(repositoryID, importPath string) string {
	return mapKeyPrefix + "import|" + repositoryID + "|" + importPath
}

func (m *mapMaterializer) recordMapping(key string, kind cstore.MappingKind, id int64) {
	m.mappingBuffers = append(m.mappingBuffers, cstore.ResourceMapping{
		LogicalKey: key, Kind: kind, ResourceID: id, RepositoryID: m.input.RepositoryID, SnapshotID: m.input.SnapshotID,
	})
}
