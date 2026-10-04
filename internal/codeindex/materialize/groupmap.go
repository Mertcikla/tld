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

// GroupMapInput is a dependency-graph grouping to materialize into a workspace.
type GroupMapInput struct {
	RepositoryID   string
	RepositoryName string
	RepositoryRoot string
	// RepositoryRemoteURL is the canonical remote of the repository, when known.
	RepositoryRemoteURL string
	SnapshotID          string
	// Files are the repository's file facts, indexed by group member.
	Files []community.File
	// Groups is the community hierarchy, outermost first.
	Groups []*community.Group
	// Edges are file-to-file dependencies resolved from symbol edges.
	Edges []MapEdge
	// Imports are external imports declared by the materialized files.
	Imports []MapImport
}

// ApplyGroupMap materializes a community hierarchy into the workspace using
// nested views: one element and view per group, direct member files placed in
// their group's view, and every edge drawn at the deepest view where its
// endpoints fall under different children.
func ApplyGroupMap(ctx context.Context, ws core.Store, idx IndexStore, input GroupMapInput, opts MapOptions) (MapResult, error) {
	if len(input.Files) == 0 {
		return MapResult{}, fmt.Errorf("group map requires file facts")
	}
	existing, err := idx.MappingsByRepository(ctx, input.RepositoryID)
	if err != nil {
		return MapResult{}, err
	}
	byKey := make(map[string]cstore.ResourceMapping, len(existing))
	for _, mapping := range existing {
		byKey[mapping.LogicalKey] = mapping
	}
	base := MapInput{
		RepositoryID:        input.RepositoryID,
		RepositoryName:      input.RepositoryName,
		RepositoryRoot:      input.RepositoryRoot,
		RepositoryRemoteURL: input.RepositoryRemoteURL,
		SnapshotID:          input.SnapshotID,
		Files:               input.Files,
		Edges:               input.Edges,
		Imports:             input.Imports,
	}
	m := &mapMaterializer{
		ctx:             ctx,
		ws:              ws,
		idx:             idx,
		input:           base,
		opts:            opts,
		byKey:           byKey,
		kept:            map[string]bool{},
		placed:          map[int64]map[int64]bool{},
		position:        map[int64]int{},
		fileElementIDs:  map[string]int64{},
		fileChains:      map[string][]int64{},
		fileElementPath: map[string][]int64{},
		queued:          map[int64]map[int64]bool{},
		layoutEdges:     map[int64][]layout.Connector{},
		leafViews:       map[int64]bool{},
		maxConnectors:   maxConnectorsPerView(opts),
		maxLeaf:         maxLeafConnectorsPerView(opts),
		total:           countGroupResources(input.Groups) + 1,
	}
	rootKey := mapKeyPrefix + "view|" + input.RepositoryID
	topKey := mapKeyPrefix + "top|" + input.RepositoryID
	legacyMapViewID := int64(0)
	if mapping, ok := byKey[rootKey]; ok && mapping.Kind == cstore.MappingView {
		legacyMapViewID = mapping.ResourceID
	}
	workspaceRootID, err := workspaceRootViewID(ctx, ws, legacyMapViewID)
	if err != nil {
		return MapResult{}, err
	}
	topElementID, err := m.upsertElement(topKey, mapTopElement(base))
	if err != nil {
		return MapResult{}, err
	}
	rootViewID, err := m.upsertView(rootKey, mapViewName(base), "Map", &topElementID)
	if err != nil {
		return MapResult{}, err
	}
	m.result.ViewID = rootViewID
	if err := m.place(workspaceRootID, topElementID); err != nil {
		return MapResult{}, err
	}
	for _, group := range input.Groups {
		if group == nil {
			continue
		}
		if err := m.materializeGroup(group, rootViewID, []int64{rootViewID}, nil); err != nil {
			return m.result, err
		}
	}
	if err := m.materializeConnectors(); err != nil {
		return m.result, err
	}
	if err := m.materializeImports(rootViewID); err != nil {
		return m.result, err
	}
	if err := m.pruneMapPlacements(); err != nil {
		return m.result, err
	}
	if err := m.applyLayout(); err != nil {
		return m.result, err
	}
	if err := m.adjustConnectorHandles(); err != nil {
		return m.result, err
	}
	if err := m.pruneMapResources(); err != nil {
		return m.result, err
	}
	return m.result, nil
}

// materializeGroup creates the group element and view, recurses into children,
// and places direct member files in the group's own view.
func (m *mapMaterializer) materializeGroup(group *community.Group, viewID int64, chain, elementPath []int64) error {
	elementID, err := m.upsertElement(groupElementKey(m.input.RepositoryID, group.Key), groupElement(group))
	if err != nil {
		return err
	}
	groupViewID, err := m.upsertView(groupViewKey(m.input.RepositoryID, group.Key), group.Name, "Map", &elementID)
	if err != nil {
		return err
	}
	m.leafViews[groupViewID] = len(group.Children) == 0
	if err := m.queuePlacement(viewID, elementID); err != nil {
		return err
	}
	views := appendView(chain, groupViewID)
	elements := appendElem(elementPath, elementID)
	for _, child := range group.Children {
		if err := m.materializeGroup(child, groupViewID, views, elements); err != nil {
			return err
		}
	}
	for _, member := range group.Members {
		if member < 0 || member >= len(m.input.Files) {
			continue
		}
		fact := m.input.Files[member]
		fileID, err := m.upsertElement(fileKey(m.input.RepositoryID, fact.ID), m.fileElement(member))
		if err != nil {
			return err
		}
		if err := m.queuePlacement(groupViewID, fileID); err != nil {
			return err
		}
		m.recordFile(fact.ID, fileID, appendView(views, groupViewID), appendElem(elements, fileID))
	}
	return nil
}

func groupElement(group *community.Group) core.LibraryElement {
	kind := "component"
	description := fmt.Sprintf("%d files", group.Files)
	if group.Isolated {
		description += " · no resolved dependencies"
	} else {
		description += fmt.Sprintf(" · %.0f internal · %.0f external deps", group.Internal, group.External)
	}
	return core.LibraryElement{Name: group.Name, Kind: &kind, Description: &description}
}

func groupElementKey(repositoryID, key string) string {
	return mapKeyPrefix + "group|" + repositoryID + "|" + key
}

func groupViewKey(repositoryID, key string) string {
	return mapKeyPrefix + "groupview|" + repositoryID + "|" + key
}

func countGroupResources(groups []*community.Group) int {
	total := 1
	var walk func(group *community.Group)
	walk = func(group *community.Group) {
		total += 2
		total += len(group.Members)
		for _, child := range group.Children {
			walk(child)
		}
	}
	for _, group := range groups {
		if group != nil {
			walk(group)
		}
	}
	return total
}

// pruneMapResources deletes map resources that are no longer part of the
// current run and persists the pending logical-key mappings.
func (m *mapMaterializer) pruneMapResources() error {
	for key, mapping := range m.byKey {
		if !strings.HasPrefix(key, mapKeyPrefix) || m.kept[key] {
			continue
		}
		switch mapping.Kind {
		case cstore.MappingView:
			_ = m.ws.DeleteView(m.ctx, mapping.ResourceID)
		case cstore.MappingElement:
			_ = m.ws.DeleteElement(m.ctx, mapping.ResourceID)
		case cstore.MappingConnector:
			_ = m.ws.DeleteConnector(m.ctx, mapping.ResourceID)
		}
		if err := m.idx.DeleteMapping(m.ctx, key); err != nil {
			return err
		}
		m.result.Pruned++
	}
	if err := m.idx.SaveMappings(m.ctx, m.mappingBuffers); err != nil {
		return err
	}
	return nil
}

// SortGroups orders a group hierarchy for stable placement: largest first, then
// by key. It is used before materialization so grid positions are deterministic.
func SortGroups(groups []*community.Group) {
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Files != groups[j].Files {
			return groups[i].Files > groups[j].Files
		}
		return groups[i].Key < groups[j].Key
	})
	for _, group := range groups {
		SortGroups(group.Children)
	}
}

// pruneMapPlacements reconciles generated placements in surviving map views.
// User-owned elements and views are outside this ownership boundary.
func (m *mapMaterializer) pruneMapPlacements() error {
	owned := map[int64]bool{}
	for key, mapping := range m.byKey {
		if strings.HasPrefix(key, mapKeyPrefix) && mapping.Kind == cstore.MappingElement {
			owned[mapping.ResourceID] = true
		}
	}
	for _, mapping := range m.mappingBuffers {
		if !strings.HasPrefix(mapping.LogicalKey, mapKeyPrefix) || mapping.Kind != cstore.MappingView {
			continue
		}
		placements, err := m.ws.ElementPlacements(m.ctx, mapping.ResourceID)
		if err != nil {
			return err
		}
		for _, placement := range placements {
			if owned[placement.ElementID] && !m.desiredPlaces[mapping.ResourceID][placement.ElementID] {
				if err := m.ws.DeletePlacement(m.ctx, mapping.ResourceID, placement.ElementID); err != nil {
					return err
				}
				delete(m.placed[mapping.ResourceID], placement.ElementID)
			}
		}
	}
	return nil
}
