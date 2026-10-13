package impact

import (
	"path"
	"path/filepath"
	"strings"

	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
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

// folderViewPrefix keys a repository folder's map view. The remaining segment
// after the repository id is the folder path.
const folderViewPrefix = "map|folderview|"

// buildViewIndex tallies the workspace view that owns each directory's
// materialized files and indexes map folder views by path. Only resources
// mapped to this repository participate, so overlapping paths never win.
func buildViewIndex(data core.ExploreData, mappings []cstore.ResourceMapping) viewIndex {
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
		return index
	}
	tallies := map[string]map[int64]int{}
	for _, view := range data.Views {
		for _, placement := range view.Placements {
			if !owned[placement.ElementID] || placement.FilePath == nil || *placement.FilePath == "" {
				continue
			}
			file := filepath.ToSlash(*placement.FilePath)
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
