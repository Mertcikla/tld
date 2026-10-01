package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/uptrace/bun"
)

// MappingKind identifies the kind of workspace resource a logical key maps to.
type MappingKind string

const (
	MappingElement   MappingKind = "element"
	MappingConnector MappingKind = "connector"
	MappingView      MappingKind = "view"
)

// ResourceMapping binds a codeindex logical key to a materialized workspace
// resource id. Logical keys are canonical, so this mapping is how the projected
// graph is joined back to workspace elements and connectors.
type ResourceMapping struct {
	LogicalKey   string
	Kind         MappingKind
	ResourceID   int64
	RepositoryID string
	SnapshotID   string
}

// SaveMappings upserts resource mappings in one transaction.
func (s *Store) SaveMappings(ctx context.Context, mappings []ResourceMapping) error {
	if len(mappings) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return s.bun.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for _, m := range mappings {
			if m.LogicalKey == "" {
				continue
			}
			kind := m.Kind
			if kind == "" {
				kind = MappingElement
			}
			if _, err := tx.NewRaw(`INSERT INTO codeindex_elements (logical_key, resource_type, resource_id, repository_id, snapshot_id, updated_at)
				VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT(logical_key) DO UPDATE SET
					resource_type = excluded.resource_type,
					resource_id = excluded.resource_id,
					repository_id = excluded.repository_id,
					snapshot_id = excluded.snapshot_id,
					updated_at = excluded.updated_at`,
				m.LogicalKey, string(kind), m.ResourceID, m.RepositoryID, m.SnapshotID, now).Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

// MappingByLogicalKey looks up a resource mapping by logical key.
func (s *Store) MappingByLogicalKey(ctx context.Context, logicalKey string) (ResourceMapping, bool, error) {
	var m ResourceMapping
	err := s.bun.NewRaw(`SELECT logical_key, resource_type, resource_id, repository_id, snapshot_id
		FROM codeindex_elements WHERE logical_key = ?`, logicalKey).Scan(ctx, &m.LogicalKey, &m.Kind, &m.ResourceID, &m.RepositoryID, &m.SnapshotID)
	if err == sql.ErrNoRows {
		return ResourceMapping{}, false, nil
	}
	if err != nil {
		return ResourceMapping{}, false, err
	}
	return m, true, nil
}

// MappingByResource finds the mapping for a workspace resource of a given kind.
// It is how a view is resolved back to the repository whose graph populated it.
func (s *Store) MappingByResource(ctx context.Context, kind MappingKind, resourceID int64) (ResourceMapping, bool, error) {
	var m ResourceMapping
	err := s.bun.NewRaw(`SELECT logical_key, resource_type, resource_id, repository_id, snapshot_id
		FROM codeindex_elements WHERE resource_type = ? AND resource_id = ?`, string(kind), resourceID).
		Scan(ctx, &m.LogicalKey, &m.Kind, &m.ResourceID, &m.RepositoryID, &m.SnapshotID)
	if err == sql.ErrNoRows {
		return ResourceMapping{}, false, nil
	}
	if err != nil {
		return ResourceMapping{}, false, err
	}
	return m, true, nil
}

// MappingsBySnapshot lists mappings recorded for a snapshot.
func (s *Store) MappingsBySnapshot(ctx context.Context, snapshotID string) ([]ResourceMapping, error) {
	rows, err := s.bun.QueryContext(ctx, `SELECT logical_key, resource_type, resource_id, repository_id, snapshot_id
		FROM codeindex_elements WHERE snapshot_id = ?`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []ResourceMapping{}
	for rows.Next() {
		var m ResourceMapping
		if err := rows.Scan(&m.LogicalKey, &m.Kind, &m.ResourceID, &m.RepositoryID, &m.SnapshotID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MappingsByRepository lists mappings recorded for a repository.
func (s *Store) MappingsByRepository(ctx context.Context, repositoryID string) ([]ResourceMapping, error) {
	rows, err := s.bun.QueryContext(ctx, `SELECT logical_key, resource_type, resource_id, repository_id, snapshot_id
		FROM codeindex_elements WHERE repository_id = ?`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []ResourceMapping{}
	for rows.Next() {
		var m ResourceMapping
		if err := rows.Scan(&m.LogicalKey, &m.Kind, &m.ResourceID, &m.RepositoryID, &m.SnapshotID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteMapping removes a single mapping by logical key.
func (s *Store) DeleteMapping(ctx context.Context, logicalKey string) error {
	_, err := s.bun.NewRaw(`DELETE FROM codeindex_elements WHERE logical_key = ?`, logicalKey).Exec(ctx)
	return err
}

// DeleteMappingsForSnapshot removes mappings recorded for a snapshot. It is used
// when a snapshot is superseded and its materialized resources are pruned.
func (s *Store) DeleteMappingsForSnapshot(ctx context.Context, snapshotID string) error {
	_, err := s.bun.NewRaw(`DELETE FROM codeindex_elements WHERE snapshot_id = ?`, snapshotID).Exec(ctx)
	return err
}
