package watch

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ensureRepositoryRootElement materializes the workspace root element that
// represents a watch repository. It is created together with the repository so
// the repository is visible in the workspace before the first scan.
//
// The element and its view are owned by the workspace, not the watch schema:
// deleting them never removes the watch_repositories row or its settings. The
// next scan re-materializes them from the repository record.
func (s *Store) ensureRepositoryRootElement(ctx context.Context, repo Repository) error {
	if s == nil {
		return fmt.Errorf("watch store is nil")
	}
	rootViewID, err := s.workspaceRootViewID(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No workspace root view to attach to; nothing to do.
			return nil
		}
		return err
	}
	m := &materializer{
		store:         s,
		repo:          repo,
		runMarker:     nowString(),
		newPlacements: map[int64]map[int64]struct{}{},
	}
	ownerKey := fmt.Sprintf("repository:%d", repo.ID)
	elemID, err := m.upsertElement(ctx, "repository", ownerKey, elementInput{
		Name:   repo.DisplayName,
		Kind:   "repository",
		Repo:   repoIdentity(repo),
		Branch: nullStringValue(repo.Branch),
		Tags:   []string{"view:architecture"},
	})
	if err != nil {
		return err
	}
	if err := m.upsertPlacement(ctx, rootViewID, elemID, 0, 0); err != nil {
		return err
	}
	if _, err := m.upsertView(ctx, "repository", ownerKey, elemID, repo.DisplayName, "Repository"); err != nil {
		return err
	}
	return nil
}

func (s *Store) workspaceRootViewID(ctx context.Context) (int64, error) {
	var id int64
	err := s.rowRaw(ctx, `SELECT id FROM views WHERE owner_element_id IS NULL ORDER BY id LIMIT 1`).Scan(&id)
	return id, err
}
