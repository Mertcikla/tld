package materialize

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mertcikla/tld/v2/internal/codeindex/community"
	cgraph "github.com/mertcikla/tld/v2/internal/codeindex/graph"
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
func ApplyGroupMap(ctx context.Context, ws core.Store, idx IndexStore, input GroupMapInput, opts MapOptions) (result MapResult, retErr error) {
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
	// Workspace writes are already committed. Persist their ownership even if
	// cancellation interrupts the run, so retries reuse these resources.
	defer func() {
		if len(m.mappingBuffers) == 0 {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := idx.SaveMappings(cleanupCtx, m.mappingBuffers); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("save map resource mappings: %w", err))
		}
	}()
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
	if err := m.materializeCrossViewConnectors(); err != nil {
		return m.result, err
	}
	if opts.IncludeExternalImports {
		if err := m.materializeImports(rootViewID); err != nil {
			return m.result, err
		}
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
//
// A group layer only makes sense when it structures a view rather than
// describing the whole view. The view already expresses a group whose contents
// are entirely its own files or entirely nested subgroups, so a layer is only
// created when the group mixes both: at least two loose member files alongside
// at least one nested subgroup. The tag is applied to those loose files only,
// leaving the subgroup nodes outside the background.
func (m *mapMaterializer) materializeGroup(group *community.Group, viewID int64, chain, elementPath []int64) error {
	elementID, err := m.upsertElement(groupElementKey(m.input.RepositoryID, group.Key), m.groupElement(group))
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
	// Tag loose files only when nested subgroups share the view, so the group
	// background encloses a proper subset of the view instead of everything.
	groupTagValue := groupTag(m.input.RepositoryID, group.Key)
	hasLayer := m.opts.GroupLayers && len(group.Members) >= 2 && len(group.Children) >= 1
	if hasLayer {
		if err := m.upsertLayer(groupLayerKey(m.input.RepositoryID, group.Key), groupViewID, group.Name, []string{groupTagValue}); err != nil {
			return err
		}
	}
	views := appendView(chain, groupViewID)
	elements := appendElem(elementPath, elementID)
	for _, child := range group.Children {
		if err := m.materializeGroup(child, groupViewID, views, elements); err != nil {
			return err
		}
	}
	// Commit only bounded batches, recording every committed resource before
	// any subsequent context-sensitive work can fail.
	for offset := 0; offset < len(group.Members); offset += 100 {
		members := group.Members[offset:min(offset+100, len(group.Members))]
		var inputs []core.LibraryElement
		var keys []string
		var fresh []int
		ids := make(map[int]int64, len(members))
		batch, canBatch := m.ws.(core.BatchElementCreator)
		for _, member := range members {
			if member < 0 || member >= len(m.input.Files) {
				continue
			}
			fact := m.input.Files[member]
			input := m.fileElement(member)
			if hasLayer {
				input.Tags = unionTags(input.Tags, []string{groupTagValue})
			}
			key := fileKey(m.input.RepositoryID, fact.ID)
			_, exists := m.byKey[key]
			if canBatch && !exists {
				inputs = append(inputs, input)
				keys = append(keys, key)
				fresh = append(fresh, member)
			} else {
				id, err := m.upsertElement(key, input)
				if err != nil {
					return err
				}
				ids[member] = id
			}
		}
		if len(inputs) > 0 {
			created, err := batch.CreateElements(m.ctx, inputs)
			if err != nil {
				return fmt.Errorf("create map file batch: %w", err)
			}
			for i, element := range created {
				key := keys[i]
				m.kept[key] = true
				m.recordMapping(key, cstore.MappingElement, element.ID)
				m.result.Elements++
				ids[fresh[i]] = element.ID
			}
			for range created {
				m.advance("element")
			}
		}
		for _, member := range members {
			id, ok := ids[member]
			if !ok {
				continue
			}
			if err := m.queuePlacement(groupViewID, id); err != nil {
				return err
			}
			m.recordFile(m.input.Files[member].ID, id, appendView(views, groupViewID), appendElem(elements, id))
		}
	}
	return nil
}

// groupLanguages lists the distinct source languages in a group's subtree.
func (m *mapMaterializer) groupLanguages(group *community.Group) []string {
	var tags []string
	for _, member := range collectGroupMembers(group) {
		if member < 0 || member >= len(m.input.Files) {
			continue
		}
		if tag := languageTag(m.input.Files[member].Language); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

// groupElement builds the component element for a community group, adding
// language/test tags and technology. The group layer tag is applied to the
// group's contents, not to this node.
func (m *mapMaterializer) groupElement(group *community.Group) core.LibraryElement {
	kind := "component"
	description := fmt.Sprintf("%d files", group.Files)
	if group.Isolated {
		description += " · no resolved dependencies"
	} else {
		description += fmt.Sprintf(" · %.0f internal · %.0f external deps", group.Internal, group.External)
	}
	input := core.LibraryElement{Name: group.Name, Kind: &kind, Description: &description}
	if m.opts.AnnotateTags {
		tags := distinctStrings(m.groupLanguages(group))
		if group.Isolated {
			tags = append(tags, "isolated")
		}
		if len(tags) > 0 {
			input.Tags = tags
		}
	}
	if m.opts.AnnotateTechnology {
		if label, links := m.groupTechnology(group); label != "" || len(links) > 0 {
			input.Technology = optionalStr(label)
			input.TechnologyConnectors = links
		}
	}
	return input
}

// groupTechnology picks the dominant source languages in a group's subtree and
// derives catalog technology links from them.
func (m *mapMaterializer) groupTechnology(group *community.Group) (string, []core.TechnologyConnector) {
	counts := map[string]int{}
	for _, member := range collectGroupMembers(group) {
		if member < 0 || member >= len(m.input.Files) {
			continue
		}
		if tag := languageTag(m.input.Files[member].Language); tag != "" {
			counts[tag]++
		}
	}
	if len(counts) == 0 {
		return "", nil
	}
	ordered := make([]string, 0, len(counts))
	for tag := range counts {
		ordered = append(ordered, tag)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if counts[ordered[i]] != counts[ordered[j]] {
			return counts[ordered[i]] > counts[ordered[j]]
		}
		return ordered[i] < ordered[j]
	})
	if len(ordered) > 3 {
		ordered = ordered[:3]
	}
	var labels []string
	var links []core.TechnologyConnector
	for i, tag := range ordered {
		label, languageLinks := languageTechnology(tag)
		if label != "" {
			labels = append(labels, label)
		}
		if i > 0 {
			for index := range languageLinks {
				languageLinks[index].IsPrimaryIcon = false
			}
		}
		links = append(links, languageLinks...)
	}
	links = mergeTechnology(links, nil, 3)
	if len(links) == 0 && len(labels) == 0 {
		return "", nil
	}
	return strings.Join(labels, ", "), links
}

// collectGroupMembers flattens a group hierarchy into member file indices.
func collectGroupMembers(group *community.Group) []int {
	var out []int
	var walk func(item *community.Group)
	walk = func(item *community.Group) {
		if item == nil {
			return
		}
		out = append(out, item.Members...)
		for _, child := range item.Children {
			walk(child)
		}
	}
	walk(group)
	return out
}

// distinctStrings preserves order while dropping empty and duplicate values.
func distinctStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// groupTag derives the deterministic editor group tag for a community group.
func groupTag(repositoryID, key string) string {
	sum := cgraph.ID(repositoryID, "group", key)
	return "group:" + formatUUID(sum)
}

// formatUUID renders a hex digest as an 8-4-4-4-12 UUID so the frontend's
// element-group pattern recognizes it. The version (13th hex digit) and variant
// (17th hex digit) nibbles are forced to RFC 4122 values because the frontend
// pattern requires them; the digest otherwise carries no version bits.
func formatUUID(hex string) string {
	if len(hex) < 32 {
		hex += strings.Repeat("0", 32-len(hex))
	}
	raw := []byte(hex[:32])
	raw[12] = '4'
	raw[16] = 'a'
	return string(raw[0:8]) + "-" + string(raw[8:12]) + "-" + string(raw[12:16]) + "-" + string(raw[16:20]) + "-" + string(raw[20:32])
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
// current run. Ownership mappings are persisted when the run exits.
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
		case cstore.MappingLayer:
			_ = m.ws.DeleteLayer(m.ctx, mapping.ResourceID)
		}
		if err := m.idx.DeleteMapping(m.ctx, key); err != nil {
			return err
		}
		m.result.Pruned++
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
