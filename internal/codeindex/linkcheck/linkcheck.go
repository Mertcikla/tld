// Package linkcheck reads the codeindex file inventory from the local database
// so validation can measure how much of the scanned code a workspace's
// source-linked elements cover. It mirrors mappingcheck: a live read of the
// local database, reported as unavailable rather than failing when absent.
package linkcheck

import (
	"context"
	"path/filepath"
	"sort"

	"github.com/mertcikla/tld/v2/internal/codeindex/mappingcheck"
	"github.com/mertcikla/tld/v2/internal/warnings"
)

// Index returns the indexed repositories and the files each one contains, ready
// for warnings.WithLinkTargets. Files come from each repository's latest
// snapshot, so coverage follows what the codeindex actually scanned rather than
// the working tree. It returns ok=false when no local database is available or
// it holds no indexed repository with files.
func Index(ctx context.Context, dataDir string) ([]warnings.LinkedRepository, bool) {
	idx, closeStore, ok := mappingcheck.OpenStore(ctx, dataDir)
	if !ok {
		return nil, false
	}
	defer closeStore()

	repositories, err := idx.ListRepositories(ctx)
	if err != nil {
		return nil, false
	}
	out := make([]warnings.LinkedRepository, 0, len(repositories))
	for _, repository := range repositories {
		if repository == nil || repository.GetId() == "" || repository.GetRoot() == "" {
			continue
		}
		snapshotID, err := idx.Latest(ctx, repository.GetId())
		if err != nil || snapshotID == "" {
			continue
		}
		sources, err := idx.SnapshotSources(ctx, snapshotID)
		if err != nil || len(sources) == 0 {
			continue
		}
		paths := make([]string, 0, len(sources))
		for filePath := range sources {
			if filePath == "" {
				continue
			}
			paths = append(paths, filePath)
		}
		if len(paths) == 0 {
			continue
		}
		sort.Strings(paths)
		name := repository.GetName()
		if name == "" {
			name = filepath.Base(repository.GetRoot())
		}
		out = append(out, warnings.LinkedRepository{
			ID:        repository.GetId(),
			Name:      name,
			Root:      repository.GetRoot(),
			RemoteURL: repository.GetRemoteUrl(),
			Paths:     paths,
		})
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
