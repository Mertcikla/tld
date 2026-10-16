package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/mertcikla/codeindex/identity"
	"github.com/mertcikla/tld/v2/internal/repolink"
	"github.com/uptrace/bun"
)

// AllocateIDs reserves a block from the database-wide 64-bit sequence.
func (s *Store) AllocateIDs(ctx context.Context, count int) (int64, error) {
	return allocateIDs(ctx, s.bun, count)
}

func allocateIDs(ctx context.Context, db bun.IDB, count int) (int64, error) {
	if count <= 0 {
		return 0, fmt.Errorf("ID allocation count must be positive")
	}
	var last int64
	err := db.NewRaw(`UPDATE codeindex_id_sequence SET value=value+? WHERE singleton=1 AND value<=?-? RETURNING value`, count, int64(math.MaxInt64), count).Scan(ctx, &last)
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
	return repositoryByRoot(ctx, s.bun, root)
}

func repositoryByRoot(ctx context.Context, db bun.IDB, root string) (string, bool, error) {
	where, args := scope(ctx).clause("org_id")
	var id string
	err := db.NewRaw(`SELECT id FROM codeindex_repositories WHERE root=?`+where+` ORDER BY id LIMIT 1`, append([]any{root}, args...)...).Scan(ctx, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}

// ResolveRepository atomically finds or creates an integer repository identity.
// Tenant isolation comes from ctx, as with the other store operations.
func (s *Store) ResolveRepository(ctx context.Context, root, remoteURL string, managed bool, _ uuid.UUID) (identity.Resolved, error) {
	root = filepath.Clean(root)
	if strings.TrimSpace(remoteURL) == "" {
		remoteURL = repolink.GitRemoteURL(ctx, root)
	}
	resolved := identity.Resolved{Root: root, RemoteURL: remoteURL, RemoteKey: repolink.RemoteKey(remoteURL), Managed: managed}
	err := s.bun.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Lock the existing sequence row before any identity lookup. This takes
		// SQLite's write lock and a PostgreSQL row lock, so another connection
		// cannot create an identity from a stale lookup, even without a remote.
		if _, err := tx.NewRaw(`UPDATE codeindex_id_sequence SET value=value WHERE singleton=1`).Exec(ctx); err != nil {
			return err
		}
		if resolved.RemoteKey != "" {
			id, found, err := repositoryByRemoteKey(ctx, tx, resolved.RemoteKey)
			if err != nil {
				return err
			}
			if found {
				resolved.ID = id
			}
		}
		if resolved.ID == "" {
			id, _, err := repositoryByRoot(ctx, tx, root)
			if err != nil {
				return err
			}
			resolved.ID = id
		}
		if resolved.ID == "" {
			first, err := allocateIDs(ctx, tx, 1)
			if err != nil {
				return err
			}
			resolved.ID = strconv.FormatInt(first, 10)
		}
		return ensureRepositoryIdentity(ctx, tx, resolved.ID, resolved.Root, resolved.RemoteURL, resolved.RemoteKey, managed)
	})
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
