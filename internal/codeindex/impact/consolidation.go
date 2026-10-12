package impact

import (
	"path"
	"path/filepath"
	"strconv"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
)

const maxRawConsolidationPlacements = 100

// consolidateRawDestination places a direct raw fallback beside selected mapped
// evidence when both belong to the same repository directory. Map clustering
// remains intact: this only reuses one already-selected sibling container, and
// declines when the source directory has multiple candidate containers or the
// destination is already large.
func (a *sceneAssembly) consolidateRawDestination(node *model.Node, destination int64) int64 {
	if node == nil || node.Path == "" || !a.mapViews[destination] {
		return 0
	}
	directory := path.Dir(filepath.ToSlash(node.Path))
	if directory == "." {
		return 0
	}
	destinationView, ok := a.views[destination]
	if !ok || destinationView.ParentViewID == nil {
		return 0
	}
	if a.hasMappedSourceInDirectory(a.scene.Views[strconv.FormatInt(destination, 10)], destination, directory) {
		return 0
	}

	var candidate int64
	for id := range a.mapViews {
		if id == destination || id == 0 || id == -1 {
			continue
		}
		view, exists := a.views[id]
		if !exists || view.ParentViewID == nil || *view.ParentViewID != *destinationView.ParentViewID {
			continue
		}
		content := a.scene.Views[strconv.FormatInt(id, 10)]
		if !a.hasMappedSourceInDirectory(content, id, directory) {
			continue
		}
		// Keep map-run partitions when a selected container is already large.
		if content == nil || len(content.Placements) >= maxRawConsolidationPlacements {
			return 0
		}
		if candidate != 0 {
			return 0
		}
		candidate = id
	}
	return candidate
}

func (a *sceneAssembly) hasMappedSourceInDirectory(content *model.ViewContent, viewID int64, directory string) bool {
	if content == nil {
		return false
	}
	for _, placement := range content.Placements {
		element := placement.GetElement()
		if element == nil || element.GetFilePath() == "" {
			continue
		}
		if a.scene.RepositoryId != "" && element.GetRepositoryId() != a.scene.RepositoryId {
			continue
		}
		metadata := a.scene.Provenance[PlacementKey(viewID, int64(element.GetElementId()))]
		if metadata == nil || metadata.Origin != pb.ImpactOrigin_IMPACT_ORIGIN_MAPPED || metadata.Role != pb.ImpactRole_IMPACT_ROLE_DIRECT {
			continue
		}
		if path.Dir(filepath.ToSlash(element.GetFilePath())) == directory {
			return true
		}
	}
	return false
}
