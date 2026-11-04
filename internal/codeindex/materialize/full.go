package materialize

import (
	"database/sql"
	"errors"
	"path"
	"sort"

	cgraph "github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/internal/layout"
)

// materializeFull preserves the filesystem hierarchy, with files as leaves.
// Dependencies have already been rolled up from symbol edges to file pairs.
func (m *mapMaterializer) materializeFull(root int64) error {
	dirs := map[string]int64{".": root, "": root, "/": root}
	folders := map[string]fullFolder{}
	foldersByFile := map[string]string{}
	var directory func(string) (int64, error)
	directory = func(p string) (int64, error) {
		p = path.Clean(p)
		if id, ok := dirs[p]; ok {
			return id, nil
		}
		parent, err := directory(path.Dir(p))
		if err != nil {
			return 0, err
		}
		key := mapKeyPrefix + "directory|" + m.repositoryKey() + "|" + p
		kind := "directory"
		id, err := m.upsertElement(key, core.LibraryElement{Name: path.Base(p), Kind: &kind})
		if err != nil {
			return 0, err
		}
		view, err := m.upsertView(key+"|view", path.Base(p), "Map", &id)
		if err != nil {
			return 0, err
		}
		if err := m.queuePlacement(parent, id); err != nil {
			return 0, err
		}
		folders[p] = fullFolder{elementID: id, parentView: parent, parent: path.Dir(p), depth: folders[path.Dir(p)].depth + 1}
		dirs[p] = view
		return view, nil
	}
	order := make([]int, len(m.input.Files))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return m.input.Files[order[i]].Path < m.input.Files[order[j]].Path })
	ids, owners := map[string]int64{}, map[string]int64{}
	for _, i := range order {
		file := m.input.Files[i]
		foldersByFile[file.ID] = path.Clean(path.Dir(file.Path))
		parent, err := directory(path.Dir(file.Path))
		if err != nil {
			return err
		}
		key := mapKeyPrefix + "fact|" + m.repositoryKey() + "|" + file.ID
		input := m.fileElement(i)
		if file.Path != "" {
			input.Name = path.Base(file.Path)
		}
		input.Branch = optionalStr(m.input.RepositoryBranch)
		id, err := m.upsertElement(key, input)
		if err != nil {
			return err
		}
		// Replace only the old generated full-path label, preserving authored names.
		existing, err := m.ws.ElementByID(m.ctx, id)
		if err != nil {
			return err
		}
		if existing.Name == file.Path && existing.Name != input.Name {
			if _, err := m.ws.UpdateElement(m.ctx, id, core.LibraryElement{Name: input.Name}); err != nil {
				return err
			}
		}
		if err := m.queuePlacement(parent, id); err != nil {
			return err
		}
		if err := m.removeFullPlacements(id, parent); err != nil {
			return err
		}
		ids[file.ID], owners[file.ID] = id, parent
	}
	edges := fullAnchoredEdges(m.input.Edges)
	sort.Slice(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if a.FromFactID != b.FromFactID {
			return a.FromFactID < b.FromFactID
		}
		if a.ToFactID != b.ToFactID {
			return a.ToFactID < b.ToFactID
		}
		return a.Kind < b.Kind
	})
	for _, e := range edges {
		e.Kind = fullDependencyKind(e.Kind)
		source, target := ids[e.FromFactID], ids[e.ToFactID]
		// Intra-file relationships and external symbols have no distinct file endpoints.
		if source == 0 || target == 0 || source == target {
			continue
		}
		key := mapKeyPrefix + "file-edge|" + m.repositoryKey() + "|" + cgraph.ID(e.FromFactID, e.ToFactID, e.Kind)
		if m.kept[key] {
			continue
		}
		view := owners[e.FromFactID]
		if e.Kind == "" {
			if err := m.reuseFullDependency(key, e.FromFactID, e.ToFactID); err != nil {
				return err
			}
		}
		if mapping, ok := m.byKey[key]; ok {
			old, err := m.ws.ConnectorByID(m.ctx, mapping.ResourceID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if err == nil {
				patch := core.Connector{}
				changed := false
				if old.SourceElementID == source && old.TargetElementID == target && old.ViewID != view {
					patch.ViewID = view
					changed = true
				}
				if e.Kind == "" {
					empty := ""
					if old.Label != nil && isFullDependencyKind(*old.Label) {
						patch.Label = &empty
						changed = true
					}
					if old.Relationship != nil && isFullDependencyKind(*old.Relationship) {
						patch.Relationship = &empty
						changed = true
					}
				}
				if handle := refreshFullLineHandle("right", old.SourceHandle, e.SourceLines); handle != "" && canRefreshFullHandle(old.SourceHandle) {
					if old.SourceHandle == nil || *old.SourceHandle != handle {
						patch.SourceHandle = &handle
						changed = true
					}
				}
				if handle := refreshFullLineHandle("left", old.TargetHandle, e.TargetLines); handle != "" && canRefreshFullHandle(old.TargetHandle) {
					if old.TargetHandle == nil || *old.TargetHandle != handle {
						patch.TargetHandle = &handle
						changed = true
					}
				}
				if changed {
					if _, err := m.ws.UpdateConnector(m.ctx, old.ID, patch); err != nil {
						return err
					}
				}
			}
		}
		label := optionalStr(e.Kind)
		sourceHandle, targetHandle := fullLineHandle("right", e.SourceLines), fullLineHandle("left", e.TargetLines)
		if err := m.upsertConnector(key, core.Connector{ViewID: view, SourceElementID: source, TargetElementID: target, Direction: "forward", Style: "bezier", Label: label, Relationship: label, SourceHandle: optionalStr(sourceHandle), TargetHandle: optionalStr(targetHandle)}); err != nil {
			return err
		}
		if owners[e.ToFactID] == view {
			m.layoutEdges[view] = append(m.layoutEdges[view], layout.Connector{Source: source, Target: target})
		}
	}
	if err := m.materializeFullFolderConnectors(edges, foldersByFile, folders); err != nil {
		return err
	}
	if err := m.retireFullDependencyDuplicates(); err != nil {
		return err
	}
	return m.retireFullSymbolMap()
}
