package impact

import (
	"context"
	"path"
	"path/filepath"
	"sort"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/internal/layout"
)

// viewIndex resolves a repository path to the workspace view that owns it.
// dirs maps a directory to the view holding its materialized files (usually a
// cluster view); folders maps a folder path to that folder's map view, which
// exists even when no file is placed directly in it. Lookups walk up the folder
// hierarchy so a file with no sibling view lands in the closest ancestor folder.
// placed records the element ID of every repo-owned file that already has a
// placement, so overlay nodes can reuse it instead of being placed again.
type viewIndex struct {
	dirs    map[string]int64
	folders map[string]int64
	placed  map[string]int64
}

// placeAddedFiles chooses the existing base-snapshot view that best matches the
// added files and assigns provisional positions for the transient overlay. The
// chosen view is recorded on the diagram; nothing is written to the workspace.
// Files that are already mapped keep their placement; genuinely new files reuse
// the same layout engine as snapshot mapping, so they land in the nearest
// unoccupied cell next to the elements they depend on. When no added file
// matches a materialized view the diagram keeps view_id 0 and the frontend falls
// back to its unmapped overlay.
func placeAddedFiles(ctx context.Context, ws core.Store, diagram *pb.ImpactDiagram, mappings []cstore.ResourceMapping, data core.ExploreData) error {
	added := make([]*pb.ImpactNode, 0, len(diagram.Nodes))
	changed := make([]*pb.ImpactNode, 0, len(diagram.Nodes))
	for _, node := range diagram.Nodes {
		if node.Context {
			continue
		}
		changed = append(changed, node)
		if node.Change == pb.ChangeKind_CHANGE_KIND_ADDED {
			added = append(added, node)
		}
	}
	voters := added
	if len(voters) == 0 {
		voters = changed
	}
	if len(voters) == 0 {
		return nil
	}
	index := buildViewIndex(data, mappings)
	votes := map[int64]int{}
	for _, node := range voters {
		if view := index.closest(node.Path); view != 0 {
			votes[view]++
		}
	}
	target, best := int64(0), 0
	for view, count := range votes {
		if count > best || (count == best && (target == 0 || view < target)) {
			target, best = view, count
		}
	}
	if target == 0 {
		return nil
	}
	diagram.ViewId = target

	existing, err := ws.ElementPlacements(ctx, target)
	if err != nil {
		return err
	}
	placements := make([]layout.Placement, 0, len(existing))
	positioned := make(map[int64]layout.Placement, len(existing))
	for _, placement := range existing {
		item := layout.Placement{ElementID: placement.ElementID, X: placement.PositionX, Y: placement.PositionY}
		placements = append(placements, item)
		positioned[placement.ElementID] = item
	}

	// Resolve node keys to element IDs for the layout connectors. Context
	// elements use their workspace ID, already-mapped files reuse the ID of
	// their existing placement, and new files get a deterministic transient
	// negative ID that never collides with a real workspace element.
	ids := make(map[string]int64, len(diagram.Nodes))
	for _, node := range diagram.Nodes {
		if node.Context {
			ids[node.Key] = node.ElementId
		}
	}
	sorted := append([]*pb.ImpactNode(nil), changed...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Key < sorted[j].Key })
	targets := make(map[int64]struct{}, len(sorted))
	nextID := int64(0)
	for _, node := range sorted {
		if elementID, ok := index.placed[filepath.ToSlash(node.Path)]; ok {
			ids[node.Key] = elementID
			if item, ok := positioned[elementID]; ok {
				node.X, node.Y = item.X, item.Y
			}
			continue
		}
		nextID--
		ids[node.Key] = nextID
		targets[nextID] = struct{}{}
	}
	if len(targets) == 0 {
		return nil
	}

	connectors := make([]layout.Connector, 0, len(diagram.Edges))
	for _, edge := range diagram.Edges {
		source, sourceOK := ids[edge.FromKey]
		targetID, targetOK := ids[edge.ToKey]
		if !sourceOK || !targetOK {
			continue
		}
		connectors = append(connectors, layout.Connector{Source: source, Target: targetID})
	}
	next := layout.DeterministicLayoutPlacements(placements, targets, connectors)
	for _, node := range sorted {
		elementID, ok := ids[node.Key]
		if !ok {
			continue
		}
		if item, ok := next[elementID]; ok {
			node.X, node.Y = item.X, item.Y
		}
	}
	return nil
}

// folderViewPrefix keys a repository folder's map view. The remaining segment
// after the repository id is the folder path.
const folderViewPrefix = "map|folderview|"

// buildViewIndex tallies the workspace view that owns each directory's
// materialized files, indexes the map's folder views by path, and records which
// repo-owned files already have a placement. Only resources mapped to this
// repository participate, so overlapping paths in other repositories never win.
func buildViewIndex(data core.ExploreData, mappings []cstore.ResourceMapping) viewIndex {
	index := viewIndex{dirs: map[string]int64{}, folders: map[string]int64{}, placed: map[string]int64{}}
	owned := make(map[int64]bool, len(mappings))
	for _, mapping := range mappings {
		switch {
		case mapping.Kind == cstore.MappingView && strings.HasPrefix(mapping.LogicalKey, folderViewPrefix):
			rest := strings.TrimPrefix(mapping.LogicalKey, folderViewPrefix)
			if i := strings.IndexByte(rest, '|'); i >= 0 {
				if folder := rest[i+1:]; folder != "" && folder != "." {
					index.folders[folder] = mapping.ResourceID
				}
			}
		case mapping.Kind == cstore.MappingElement && !strings.HasPrefix(mapping.LogicalKey, "impact|"):
			owned[mapping.ResourceID] = true
		}
	}
	if len(owned) == 0 {
		return index
	}
	tallies := map[string]map[int64]int{}
	for _, view := range data.Views {
		for _, placement := range view.Placements {
			if !owned[placement.ElementID] || placement.FilePath == nil || *placement.FilePath == "" {
				continue
			}
			file := filepath.ToSlash(*placement.FilePath)
			if _, ok := index.placed[file]; !ok {
				index.placed[file] = placement.ElementID
			}
			dir := path.Dir(file)
			if tallies[dir] == nil {
				tallies[dir] = map[int64]int{}
			}
			tallies[dir][placement.ViewID]++
		}
	}
	for dir, tally := range tallies {
		view, count := int64(0), 0
		for candidate, n := range tally {
			if n > count || (n == count && (view == 0 || candidate < view)) {
				view, count = candidate, n
			}
		}
		if view != 0 {
			index.dirs[dir] = view
		}
	}
	return index
}

// closest returns the deepest indexed view containing the file, walking up the
// folder hierarchy until one is found. A view holding sibling files is preferred
// over the folder's own view at the same level.
func (v viewIndex) closest(file string) int64 {
	dir := path.Dir(filepath.ToSlash(file))
	for {
		if view, ok := v.dirs[dir]; ok {
			return view
		}
		if view, ok := v.folders[dir]; ok {
			return view
		}
		parent := path.Dir(dir)
		if parent == dir {
			return 0
		}
		dir = parent
	}
}
