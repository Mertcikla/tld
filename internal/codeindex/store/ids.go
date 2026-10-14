package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/mertcikla/codeindex/identity"
	"math"
	"strconv"
)

// AllocateIDs reserves a block from the database-wide 64-bit sequence.
func (s *Store) AllocateIDs(ctx context.Context, count int) (int64, error) {
	if count <= 0 {
		return 0, fmt.Errorf("ID allocation count must be positive")
	}
	var last int64
	err := s.bun.NewRaw(`UPDATE codeindex_id_sequence SET value=value+? WHERE singleton=1 AND value<=?-? RETURNING value`, count, int64(math.MaxInt64), count).Scan(ctx, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("codeindex IDs exhausted")
	}
	return last - int64(count) + 1, err
}
func numericID(id string) bool { n, err := strconv.ParseInt(id, 10, 64); return err == nil && n > 0 }
func storedID(id string) any {
	if id == "" {
		return int64(0)
	}
	return id
}
func (s *Store) RepositoryByRoot(ctx context.Context, root string) (string, bool, error) {
	where, args := scope(ctx).clause("org_id")
	var id string
	err := s.bun.NewRaw(`SELECT id FROM codeindex_repositories WHERE root=?`+where+` ORDER BY id LIMIT 1`, append([]any{root}, args...)...).Scan(ctx, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}

// ResolveRepository also works with the pinned codeindex identity resolver,
// whose fallback uses path hashes. Only database identities become integers.
func (s *Store) ResolveRepository(ctx context.Context, root, remoteURL string, managed bool, org uuid.UUID) (identity.Resolved, error) {
	resolved, err := identity.Resolve(ctx, s, root, "", remoteURL, org)
	if err != nil {
		return resolved, err
	}
	if !numericID(resolved.ID) {
		id, ok, err := s.RepositoryByRoot(ctx, root)
		if err != nil {
			return resolved, err
		}
		if !ok {
			first, err := s.AllocateIDs(ctx, 1)
			if err != nil {
				return resolved, err
			}
			id = strconv.FormatInt(first, 10)
		}
		resolved.ID = id
	}
	resolved.Managed = managed
	err = s.EnsureRepositoryIdentity(ctx, resolved.ID, resolved.Root, resolved.RemoteURL, resolved.RemoteKey, managed)
	if err != nil && resolved.RemoteKey != "" {
		if id, ok, lookupErr := s.RepositoryByRemoteKey(ctx, resolved.RemoteKey); lookupErr == nil && ok {
			resolved.ID = id
			return resolved, nil
		}
	}
	return resolved, err
}

// MappingNamespace is stable across repository-ID conversion. Keys and tags
// continue to identify the same workspace resources after reindexing.
func (s *Store) MappingNamespace(ctx context.Context, repositoryID string) (string, error) {
	where, args := scope(ctx).clause("org_id")
	var key string
	err := s.bun.NewRaw(`SELECT mapping_namespace FROM codeindex_repositories WHERE id=?`+where, append([]any{repositoryID}, args...)...).Scan(ctx, &key)
	if errors.Is(err, sql.ErrNoRows) {
		return repositoryID, nil
	}
	if key == "" {
		key = repositoryID
	}
	return key, err
}
