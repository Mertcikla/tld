package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

// MappingKind identifies the kind of workspace resource a logical key maps to.
type MappingKind string

const (
	MappingElement   MappingKind = "element"
	MappingConnector MappingKind = "connector"
	MappingView      MappingKind = "view"
	MappingLayer     MappingKind = "layer"
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
	org := scope(ctx).value()
	return s.bun.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for _, m := range mappings {
			if m.LogicalKey == "" {
				continue
			}
			kind := m.Kind
			if kind == "" {
				kind = MappingElement
			}
			if _, err := tx.NewRaw(`INSERT INTO codeindex_elements (logical_key, resource_type, resource_id, repository_id, snapshot_id, updated_at, org_id)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(org_id, logical_key) DO UPDATE SET
					resource_type = excluded.resource_type,
					resource_id = excluded.resource_id,
					repository_id = excluded.repository_id,
					snapshot_id = excluded.snapshot_id,
					updated_at = excluded.updated_at`,
				m.LogicalKey, string(kind), m.ResourceID, m.RepositoryID, m.SnapshotID, now, org).Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

// MappingByLogicalKey looks up a resource mapping by logical key.
func (s *Store) MappingByLogicalKey(ctx context.Context, logicalKey string) (ResourceMapping, bool, error) {
	where, scopeArgs := scope(ctx).clause("org_id")
	var m ResourceMapping
	err := s.bun.NewRaw(`SELECT logical_key, resource_type, resource_id, repository_id, snapshot_id
		FROM codeindex_elements WHERE logical_key = ?`+where, append([]any{logicalKey}, scopeArgs...)...).Scan(ctx, &m.LogicalKey, &m.Kind, &m.ResourceID, &m.RepositoryID, &m.SnapshotID)
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
	where, scopeArgs := scope(ctx).clause("org_id")
	var m ResourceMapping
	err := s.bun.NewRaw(`SELECT logical_key, resource_type, resource_id, repository_id, snapshot_id
		FROM codeindex_elements WHERE resource_type = ? AND resource_id = ?`+where, append([]any{string(kind), resourceID}, scopeArgs...)...).
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
	where, scopeArgs := scope(ctx).clause("org_id")
	rows, err := s.bun.QueryContext(ctx, `SELECT logical_key, resource_type, resource_id, repository_id, snapshot_id
		FROM codeindex_elements WHERE snapshot_id = ?`+where, append([]any{snapshotID}, scopeArgs...)...)
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
	where, scopeArgs := scope(ctx).clause("org_id")
	rows, err := s.bun.QueryContext(ctx, `SELECT logical_key, resource_type, resource_id, repository_id, snapshot_id
		FROM codeindex_elements WHERE repository_id = ?`+where, append([]any{repositoryID}, scopeArgs...)...)
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

// ElementSourceKey identifies a workspace element by its codeindex source
// location (repository id + file path). It is the join key between a YAML
// workspace element and its materialized database row.
func ElementSourceKey(repositoryID, filePath string) string {
	return repositoryID + "\x00" + filePath
}

// ElementNameKey identifies a path-less workspace element (a codeindex group or
// repository root) by kind and display name. It is a fallback identity for
// materialized elements that carry no file path.
func ElementNameKey(kind, name string) string {
	return strings.ToLower(strings.TrimSpace(kind)) + "\x00" + strings.TrimSpace(name)
}

// MappedElementIndex indexes workspace elements that were materialized from the
// codeindex. File-backed elements are keyed by source location; path-less
// elements (groups, repository roots) are keyed by kind and name.
type MappedElementIndex struct {
	Sources map[string]struct{}
	Names   map[string]struct{}
}

// MappedElementIndex is a live read of the codeindex_elements mapping table
// joined to the elements it points at, so a caller can tell genuinely
// codeindex-owned elements apart from user-authored ones.
func (s *Store) MappedElementIndex(ctx context.Context) (MappedElementIndex, error) {
	where, scopeArgs := scope(ctx).clause("m.org_id")
	rows, err := s.bun.QueryContext(ctx, `SELECT e.repository_id, e.file_path, e.kind, e.name
		FROM codeindex_elements m
		JOIN elements e ON e.id = m.resource_id
		WHERE m.resource_type = ?`+where, append([]any{string(MappingElement)}, scopeArgs...)...)
	if err != nil {
		return MappedElementIndex{}, err
	}
	defer func() { _ = rows.Close() }()
	index := MappedElementIndex{
		Sources: make(map[string]struct{}),
		Names:   make(map[string]struct{}),
	}
	for rows.Next() {
		var repositoryID, filePath, kind, name sql.NullString
		if err := rows.Scan(&repositoryID, &filePath, &kind, &name); err != nil {
			return MappedElementIndex{}, err
		}
		if filePath.Valid && strings.TrimSpace(filePath.String) != "" {
			index.Sources[ElementSourceKey(repositoryID.String, filePath.String)] = struct{}{}
			continue
		}
		if strings.TrimSpace(name.String) != "" {
			index.Names[ElementNameKey(kind.String, name.String)] = struct{}{}
		}
	}
	return index, rows.Err()
}

// DeleteMapping removes a single mapping by logical key.
func (s *Store) DeleteMapping(ctx context.Context, logicalKey string) error {
	where, scopeArgs := scope(ctx).clause("org_id")
	_, err := s.bun.NewRaw(`DELETE FROM codeindex_elements WHERE logical_key = ?`+where, append([]any{logicalKey}, scopeArgs...)...).Exec(ctx)
	return err
}

// DeleteMappingsForSnapshot removes mappings recorded for a snapshot. It is used
// when a snapshot is superseded and its materialized resources are pruned.
func (s *Store) DeleteMappingsForSnapshot(ctx context.Context, snapshotID string) error {
	where, scopeArgs := scope(ctx).clause("org_id")
	_, err := s.bun.NewRaw(`DELETE FROM codeindex_elements WHERE snapshot_id = ?`+where, append([]any{snapshotID}, scopeArgs...)...).Exec(ctx)
	return err
}
