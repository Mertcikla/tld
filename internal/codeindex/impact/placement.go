package impact

import (
	"context"
	"path"
	"path/filepath"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
)

// Overlay nodes are laid out in a compact grid inside the chosen base view.
const (
	overlayColumns  = 3
	overlaySpacingX = 240.0
	overlaySpacingY = 150.0
)

// viewIndex resolves a repository path to the workspace view that owns it.
// dirs maps a directory to the view holding its materialized files (usually a
// cluster view); folders maps a folder path to that folder's map view, which
// exists even when no file is placed directly in it. Lookups walk up the folder
// hierarchy so a file with no sibling view lands in the closest ancestor folder.
type viewIndex struct {
	dirs    map[string]int64
	folders map[string]int64
}

// placeAddedFiles chooses the existing base-snapshot view that best matches the
// added files and assigns provisional positions for the transient overlay. The
// chosen view is recorded on the diagram; nothing is written to the workspace.
// When no added file matches a materialized view the diagram keeps view_id 0 and
// the frontend falls back to its unmapped overlay.
func placeAddedFiles(ctx context.Context, ws core.Store, diagram *pb.ImpactDiagram, mappings []cstore.ResourceMapping) error {
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
	index, err := buildViewIndex(ctx, ws, mappings)
	if err != nil {
		return err
	}
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
	for i, node := range changed {
		node.X = float64((i % overlayColumns) * overlaySpacingX)
		node.Y = float64((i / overlayColumns) * overlaySpacingY)
	}
	return nil
}

// folderViewPrefix keys a repository folder's map view. The remaining segment
// after the repository id is the folder path.
const folderViewPrefix = "map|folderview|"

// buildViewIndex tallies the workspace view that owns each directory's
// materialized files, and indexes the map's folder views by path. Only
// resources mapped to this repository participate, so overlapping paths in
// other repositories never win.
func buildViewIndex(ctx context.Context, ws core.Store, mappings []cstore.ResourceMapping) (viewIndex, error) {
	index := viewIndex{dirs: map[string]int64{}, folders: map[string]int64{}}
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
		return index, nil
	}
	data, err := ws.Explore(ctx)
	if err != nil {
		return viewIndex{}, err
	}
	tallies := map[string]map[int64]int{}
	for _, view := range data.Views {
		for _, placement := range view.Placements {
			if !owned[placement.ElementID] || placement.FilePath == nil || *placement.FilePath == "" {
				continue
			}
			dir := path.Dir(filepath.ToSlash(*placement.FilePath))
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
	return index, nil
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
