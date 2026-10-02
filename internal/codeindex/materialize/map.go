package materialize

import (
	"context"
	"fmt"
	"strings"

	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/internal/mapper"
)

// MapInput is the mapper pipeline output to materialize into the workspace.
type MapInput struct {
	RepositoryID   string
	RepositoryName string
	RepositoryRoot string
	SnapshotID     string
	RunID          string
	Dataset        *mapper.Dataset
	Bins           *mapper.BinningResult
}

// MapOptions configures a mapper materialization run.
type MapOptions struct {
	ViewName string
	// Progress receives coarse (current, total, detail) updates while resources
	// are created or updated. It may be nil.
	Progress func(current, total int, detail string)
}

// MapResult summarizes what changed.
type MapResult struct {
	ViewID   int64
	Elements int
	Views    int
	Pruned   int
}

const mapKeyPrefix = "map|"

// ApplyMap materializes the mapper folder/bin/cluster hierarchy into the
// workspace. A top element representing the map is placed in the workspace root
// view and owns the map view: the map view contains folder, bin and
// standalone-file elements; each bin view contains its cluster elements; each
// cluster view contains its member file elements. Resources are keyed by
// canonical logical keys so reruns upsert instead of duplicating, and stale map
// resources are pruned.
func ApplyMap(ctx context.Context, ws core.Store, idx IndexStore, input MapInput, opts MapOptions) (MapResult, error) {
	if input.Dataset == nil || input.Bins == nil {
		return MapResult{}, fmt.Errorf("map materialize requires a dataset and binning result")
	}
	existing, err := idx.MappingsByRepository(ctx, input.RepositoryID)
	if err != nil {
		return MapResult{}, err
	}
	byKey := make(map[string]cstore.ResourceMapping, len(existing))
	for _, mapping := range existing {
		byKey[mapping.LogicalKey] = mapping
	}
	naming, clusterNames, err := inferClusterNames(input.Dataset, input.Bins)
	if err != nil {
		return MapResult{}, err
	}
	m := &mapMaterializer{
		ctx:          ctx,
		ws:           ws,
		input:        input,
		opts:         opts,
		byKey:        byKey,
		kept:         map[string]bool{},
		placed:       map[int64]map[int64]bool{},
		position:     map[int64]int{},
		naming:       naming,
		clusterNames: clusterNames,
		total:        countMapResources(input.Bins.Tree) + 1,
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
	topElementID, err := m.upsertElement(topKey, mapTopElement(input))
	if err != nil {
		return MapResult{}, err
	}
	rootViewID, err := m.upsertView(rootKey, mapViewName(input), "Map", &topElementID)
	if err != nil {
		return MapResult{}, err
	}
	m.result.ViewID = rootViewID
	if err := m.place(workspaceRootID, topElementID); err != nil {
		return MapResult{}, err
	}
	if err := m.materializeFolder(input.Bins.Tree, rootViewID); err != nil {
		return MapResult{}, err
	}
	for key, mapping := range byKey {
		if !strings.HasPrefix(key, mapKeyPrefix) || m.kept[key] {
			continue
		}
		switch mapping.Kind {
		case cstore.MappingView:
			_ = ws.DeleteView(ctx, mapping.ResourceID)
		case cstore.MappingElement:
			_ = ws.DeleteElement(ctx, mapping.ResourceID)
		}
		if err := idx.DeleteMapping(ctx, key); err != nil {
			return m.result, err
		}
		m.result.Pruned++
	}
	if err := idx.SaveMappings(ctx, m.pending); err != nil {
		return m.result, err
	}
	return m.result, nil
}

type mapMaterializer struct {
	ctx          context.Context
	ws           core.Store
	input        MapInput
	opts         MapOptions
	byKey        map[string]cstore.ResourceMapping
	kept         map[string]bool
	pending      []cstore.ResourceMapping
	placed       map[int64]map[int64]bool
	position     map[int64]int
	naming       *mapper.NameIndex
	clusterNames []string
	result       MapResult
	done         int
	total        int
}

func (m *mapMaterializer) materializeFolder(node mapper.FolderNode, viewID int64) error {
	for _, child := range node.Children {
		element, err := m.upsertElement(folderKey(m.input.RepositoryID, child.Path), m.folderElement(child))
		if err != nil {
			return err
		}
		childViewID, err := m.upsertView(folderViewKey(m.input.RepositoryID, child.Path), folderName(child.Path), "Map", &element)
		if err != nil {
			return err
		}
		if err := m.place(viewID, element); err != nil {
			return err
		}
		if err := m.materializeFolder(child, childViewID); err != nil {
			return err
		}
	}
	for binIndex, bin := range node.Bins {
		name := m.binName(bin, binIndex)
		element, err := m.upsertElement(binKey(m.input.RepositoryID, node.Path, binIndex), binElement(name, bin))
		if err != nil {
			return err
		}
		binViewID, err := m.upsertView(binViewKey(m.input.RepositoryID, node.Path, binIndex), name, "Map", &element)
		if err != nil {
			return err
		}
		if err := m.place(viewID, element); err != nil {
			return err
		}
		for _, clusterIndex := range bin.Clusters {
			if clusterIndex < 0 || clusterIndex >= len(m.input.Bins.Units) {
				continue
			}
			unit := m.input.Bins.Units[clusterIndex]
			clusterInput := clusterElement(unit, m.clusterName(clusterIndex))
			clusterKeyValue := clusterKey(m.input.RepositoryID, node.Path, binIndex, unit.Rank)
			clusterElementID, err := m.upsertElement(clusterKeyValue, clusterInput)
			if err != nil {
				return err
			}
			clusterViewID, err := m.upsertView(clusterViewKey(m.input.RepositoryID, node.Path, binIndex, unit.Rank), clusterInput.Name, "Map", &clusterElementID)
			if err != nil {
				return err
			}
			if err := m.place(binViewID, clusterElementID); err != nil {
				return err
			}
			for _, member := range unit.Members {
				if member < 0 || member >= len(m.input.Dataset.Facts) {
					continue
				}
				fileID, err := m.upsertElement(fileKey(m.input.RepositoryID, m.input.Dataset.Facts[member].ID), m.fileElement(member))
				if err != nil {
					return err
				}
				if err := m.place(clusterViewID, fileID); err != nil {
					return err
				}
			}
		}
	}
	for _, member := range node.Standalone {
		if member < 0 || member >= len(m.input.Dataset.Facts) {
			continue
		}
		fileID, err := m.upsertElement(fileKey(m.input.RepositoryID, m.input.Dataset.Facts[member].ID), m.fileElement(member))
		if err != nil {
			return err
		}
		if err := m.place(viewID, fileID); err != nil {
			return err
		}
	}
	return nil
}

func (m *mapMaterializer) upsertElement(logicalKey string, input core.LibraryElement) (int64, error) {
	m.kept[logicalKey] = true
	if mapping, ok := m.byKey[logicalKey]; ok && mapping.Kind == cstore.MappingElement {
		if updated, err := m.ws.UpdateElement(m.ctx, mapping.ResourceID, input); err == nil {
			m.result.Elements++
			m.advance("element")
			return updated.ID, nil
		}
	}
	created, err := m.ws.CreateElement(m.ctx, input)
	if err != nil {
		return 0, fmt.Errorf("create map element %q: %w", logicalKey, err)
	}
	m.pending = append(m.pending, cstore.ResourceMapping{
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
				if _, err := m.ws.UpdateView(m.ctx, mapping.ResourceID, &name, nil, &label, nil); err == nil {
					m.result.Views++
					m.advance("view")
					return mapping.ResourceID, nil
				}
			} else {
				// The view's owner changed (e.g. a map view created before it
				// was nested under the workspace root). Recreate it so the
				// hierarchy is correct, then re-place its children.
				_ = m.ws.DeleteView(m.ctx, mapping.ResourceID)
			}
		}
	}
	view, err := m.ws.CreateView(m.ctx, name, &label, ownerElementID)
	if err != nil {
		return 0, fmt.Errorf("create map view %q: %w", logicalKey, err)
	}
	m.pending = append(m.pending, cstore.ResourceMapping{
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
	return existing, nil
}

func (m *mapMaterializer) advance(detail string) {
	m.done++
	if m.opts.Progress != nil && (m.done%25 == 0 || m.done >= m.total) {
		m.opts.Progress(m.done, m.total, detail)
	}
}

func (m *mapMaterializer) fileElement(member int) core.LibraryElement {
	fact := m.input.Dataset.Facts[member]
	kind := "file"
	input := core.LibraryElement{
		Name: fact.DisplayName,
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

func folderName(path string) string {
	if path == "." || path == "" {
		return "root"
	}
	return path
}

// inferClusterNames builds the lexical naming index and names the binning
// units, which are in the same rank order as the pipeline domains. The index is
// reused to name bins and folders over their aggregated members.
func inferClusterNames(dataset *mapper.Dataset, bins *mapper.BinningResult) (*mapper.NameIndex, []string, error) {
	index, err := mapper.NewNameIndex(dataset, mapper.DefaultNameOptions())
	if err != nil {
		return nil, nil, err
	}
	names := make([]string, len(bins.Units))
	for i, unit := range bins.Units {
		names[i] = index.Name(unit.Members)
	}
	return index, names, nil
}

func (m *mapMaterializer) clusterName(index int) string {
	if index >= 0 && index < len(m.clusterNames) && m.clusterNames[index] != "" {
		return m.clusterNames[index]
	}
	if index >= 0 && index < len(m.input.Bins.Units) {
		unit := m.input.Bins.Units[index]
		if unit.Folder != "" {
			return fmt.Sprintf("%s · %d files", unit.Folder, unit.Size)
		}
		return fmt.Sprintf("cluster %d", unit.Rank)
	}
	return fmt.Sprintf("cluster %d", index+1)
}

// membersUnderClusters returns every member fact of the given cluster indices.
func (m *mapMaterializer) membersUnderClusters(indices []int) []int {
	seen := map[int]bool{}
	members := make([]int, 0)
	for _, index := range indices {
		if index < 0 || index >= len(m.input.Bins.Units) {
			continue
		}
		for _, member := range m.input.Bins.Units[index].Members {
			if seen[member] {
				continue
			}
			seen[member] = true
			members = append(members, member)
		}
	}
	return members
}

// membersUnderNode returns every member fact under a folder subtree, including
// standalone facts.
func (m *mapMaterializer) membersUnderNode(node mapper.FolderNode) []int {
	seen := map[int]bool{}
	members := make([]int, 0)
	add := func(values []int) {
		for _, value := range values {
			if seen[value] {
				continue
			}
			seen[value] = true
			members = append(members, value)
		}
	}
	for _, bin := range node.Bins {
		add(m.membersUnderClusters(bin.Clusters))
	}
	add(node.Standalone)
	for _, child := range node.Children {
		add(m.membersUnderNode(child))
	}
	return members
}

// inferName returns the single inferred name for a member set, or the fallback
// when no token qualifies. It never concatenates multiple tokens.
func (m *mapMaterializer) inferName(members []int, fallback string) string {
	if m.naming == nil || len(members) == 0 {
		return fallback
	}
	if name := m.naming.Name(members); name != "" {
		return name
	}
	return fallback
}

// namesUnderClusters returns the distinct inferred names for cluster indices,
// used only to decide whether a folder aggregates more than one cluster.
func (m *mapMaterializer) namesUnderClusters(indices []int) []string {
	seen := map[string]bool{}
	names := make([]string, 0, len(indices))
	for _, index := range indices {
		name := m.clusterName(index)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

// namesUnderNode returns every distinct cluster name in a folder subtree.
func (m *mapMaterializer) namesUnderNode(node mapper.FolderNode) []string {
	seen := map[string]bool{}
	names := make([]string, 0)
	for _, bin := range node.Bins {
		for _, name := range m.namesUnderClusters(bin.Clusters) {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	for _, child := range node.Children {
		for _, name := range m.namesUnderNode(child) {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}

// binName infers one name from all facts in the bin rather than joining the
// names of its clusters.
func (m *mapMaterializer) binName(bin mapper.Bin, index int) string {
	fallback := fmt.Sprintf("Bin %d · %d files", index+1, bin.Size)
	return m.inferName(m.membersUnderClusters(bin.Clusters), fallback)
}

// folderElement names a folder after the single most distinctive token across
// its subtree facts when it spans more than one cluster. Single-cluster folders
// keep their path.
func (m *mapMaterializer) folderElement(node mapper.FolderNode) core.LibraryElement {
	element := folderElement(node)
	if len(m.namesUnderNode(node)) > 1 {
		element.Name = m.inferName(m.membersUnderNode(node), folderName(node.Path))
		if element.Description != nil {
			description := node.Path + " · " + *element.Description
			element.Description = &description
		}
	}
	return element
}

func folderElement(node mapper.FolderNode) core.LibraryElement {
	kind := "folder"
	return core.LibraryElement{
		Name:        folderName(node.Path),
		Kind:        &kind,
		Description: strPtr(fmt.Sprintf("%d facts · %d bins · %d clusters · %d standalone", node.Counts.Facts, node.Counts.Bins, node.Counts.Clusters, node.Counts.Standalone)),
	}
}

func binElement(name string, bin mapper.Bin) core.LibraryElement {
	kind := "bin"
	return core.LibraryElement{
		Name:        name,
		Kind:        &kind,
		Description: strPtr(fmt.Sprintf("%d files · %d clusters", bin.Size, len(bin.Clusters))),
	}
}

func clusterElement(unit mapper.ClusterUnit, name string) core.LibraryElement {
	kind := "cluster"
	if name == "" {
		name = fmt.Sprintf("cluster %d", unit.Rank)
	}
	description := fmt.Sprintf("cluster %d · size %d · tightness %.3f", unit.Rank, unit.Size, unit.Tightness)
	if unit.Folder != "" {
		description += " · folder: " + unit.Folder
	}
	if len(unit.Spans) > 0 {
		description += " · spans: " + strings.Join(unit.Spans, ", ")
	}
	return core.LibraryElement{Name: name, Kind: &kind, Description: &description}
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

func folderKey(repositoryID, path string) string {
	return mapKeyPrefix + "folder|" + repositoryID + "|" + path
}

func folderViewKey(repositoryID, path string) string {
	return mapKeyPrefix + "folderview|" + repositoryID + "|" + path
}

func binKey(repositoryID, path string, index int) string {
	return fmt.Sprintf("%sbin|%s|%s|%d", mapKeyPrefix, repositoryID, path, index)
}

func binViewKey(repositoryID, path string, index int) string {
	return fmt.Sprintf("%sbinview|%s|%s|%d", mapKeyPrefix, repositoryID, path, index)
}

func clusterKey(repositoryID, path string, index, rank int) string {
	return fmt.Sprintf("%scluster|%s|%s|%d|%d", mapKeyPrefix, repositoryID, path, index, rank)
}

func clusterViewKey(repositoryID, path string, index, rank int) string {
	return fmt.Sprintf("%sclusterview|%s|%s|%d|%d", mapKeyPrefix, repositoryID, path, index, rank)
}

func fileKey(repositoryID, factID string) string {
	return mapKeyPrefix + "fact|" + repositoryID + "|" + factID
}

func countMapResources(tree mapper.FolderNode) int {
	total := 1 // map view
	var walk func(node mapper.FolderNode)
	walk = func(node mapper.FolderNode) {
		for _, child := range node.Children {
			total += 2 // folder element + view
			walk(child)
		}
		for _, bin := range node.Bins {
			total += 2 // bin element + view
			total += len(bin.Clusters) * 2
			total += bin.Size
		}
		total += len(node.Standalone)
	}
	walk(tree)
	return total
}
