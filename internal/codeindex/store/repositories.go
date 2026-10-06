package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/repolink"
	"github.com/uptrace/bun"
)

// DeleteRepository removes a repository, every snapshot it published, and all
// snapshot-scoped records (sources, facts, edges, analysis runs and their
// groups), plus any resource mappings recorded for it. It does not touch
// workspace resources materialized from the repository.
func (s *Store) DeleteRepository(ctx context.Context, repositoryID string) error {
	if strings.TrimSpace(repositoryID) == "" {
		return fmt.Errorf("repository id is required")
	}
	owned, err := s.repositoryOwned(ctx, repositoryID)
	if err != nil {
		return err
	}
	if !owned {
		return fmt.Errorf("repository %q not found", repositoryID)
	}
	where, scopeArgs := scope(ctx).clause("org_id")
	snapshotScoped := []string{
		"codeindex_project_artifacts",
		"codeindex_sources",
		"codeindex_snapshot_facts",
		"codeindex_snapshot_edges",
	}
	return s.bun.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewRaw(`DELETE FROM codeindex_group_members WHERE group_id IN (
			SELECT id FROM codeindex_groups WHERE codeindex_groups.org_id = codeindex_group_members.org_id AND run_id IN (
				SELECT id FROM codeindex_analysis_runs WHERE codeindex_analysis_runs.org_id = codeindex_group_members.org_id AND repository_id = ?))`+where, append([]any{repositoryID}, scopeArgs...)...).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_groups WHERE run_id IN (
			SELECT id FROM codeindex_analysis_runs WHERE codeindex_analysis_runs.org_id = codeindex_groups.org_id AND repository_id = ?)`+where, append([]any{repositoryID}, scopeArgs...)...).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_analysis_runs WHERE repository_id = ?`+where, append([]any{repositoryID}, scopeArgs...)...).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_completed_maps WHERE repository_id = ?`+where, append([]any{repositoryID}, scopeArgs...)...).Exec(ctx); err != nil {
			return err
		}
		// The caller retains the indexing lease through deletion and releases
		// it afterward. Clearing it here would admit a concurrent publisher.
		for _, table := range []string{"codeindex_active_maps", "codeindex_impacts", "codeindex_watch_state", "codeindex_repository_settings"} {
			if _, err := tx.NewRaw(`DELETE FROM `+table+` WHERE repository_id = ?`+where, append([]any{repositoryID}, scopeArgs...)...).Exec(ctx); err != nil {
				return err
			}
		}
		for _, table := range snapshotScoped {
			if _, err := tx.NewRaw(`DELETE FROM `+table+` WHERE snapshot_id IN (
				SELECT id FROM codeindex_snapshots WHERE codeindex_snapshots.org_id = `+table+`.org_id AND repository_id = ?)`+where, append([]any{repositoryID}, scopeArgs...)...).Exec(ctx); err != nil {
				return err
			}
		}
		// Shared entities retain the snapshot_id of their first publication,
		// which may no longer exist. Delete by repository ownership instead.
		for _, table := range []string{"codeindex_edges", "codeindex_facts"} {
			if _, err := tx.NewRaw(`DELETE FROM `+table+` WHERE repository_id = ?`+where, append([]any{repositoryID}, scopeArgs...)...).Exec(ctx); err != nil {
				return err
			}
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_snapshots WHERE repository_id = ?`+where, append([]any{repositoryID}, scopeArgs...)...).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_elements WHERE repository_id = ?`+where, append([]any{repositoryID}, scopeArgs...)...).Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewRaw(`DELETE FROM codeindex_repositories WHERE id = ?`+where, append([]any{repositoryID}, scopeArgs...)...).Exec(ctx)
		return err
	})
}

// DeleteSnapshot removes one published snapshot and the records scoped to it
// (sources, project artifacts, analysis runs and their groups, and completed
// maps). Resource ownership survives snapshot deletion. Facts and edges are shared
// across snapshots through membership tables, so only this snapshot's
// membership is removed; entities no longer referenced by any snapshot are
// garbage collected. When the deleted snapshot was the repository's latest, the
// latest pointer moves to the newest remaining snapshot.
func (s *Store) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	if strings.TrimSpace(snapshotID) == "" {
		return fmt.Errorf("snapshot id is required")
	}
	where, scopeArgs := scope(ctx).clause("org_id")
	var repositoryID string
	if err := s.bun.NewRaw(`SELECT repository_id FROM codeindex_snapshots WHERE id = ?`+where, append([]any{snapshotID}, scopeArgs...)...).Scan(ctx, &repositoryID); err != nil {
		return err
	}
	return s.bun.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewRaw(`DELETE FROM codeindex_group_members WHERE group_id IN (
			SELECT id FROM codeindex_groups WHERE codeindex_groups.org_id = codeindex_group_members.org_id AND snapshot_id = ?)`+where, append([]any{snapshotID}, scopeArgs...)...).Exec(ctx); err != nil {
			return err
		}
		for _, table := range []string{
			"codeindex_groups",
			"codeindex_analysis_runs",
			"codeindex_completed_maps",
			"codeindex_project_artifacts",
			"codeindex_sources",
			"codeindex_snapshot_facts",
			"codeindex_snapshot_edges",
		} {
			if _, err := tx.NewRaw("DELETE FROM "+table+" WHERE snapshot_id = ?"+where, append([]any{snapshotID}, scopeArgs...)...).Exec(ctx); err != nil {
				return err
			}
		}
		if _, err := tx.NewRaw(`UPDATE codeindex_elements SET snapshot_id = '' WHERE snapshot_id = ?`+where, append([]any{snapshotID}, scopeArgs...)...).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_edges
			WHERE repository_id = ? AND id NOT IN (SELECT edge_id FROM codeindex_snapshot_edges WHERE codeindex_snapshot_edges.org_id = codeindex_edges.org_id)`+where, append([]any{repositoryID}, scopeArgs...)...).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_facts
			WHERE repository_id = ? AND id NOT IN (SELECT fact_id FROM codeindex_snapshot_facts WHERE codeindex_snapshot_facts.org_id = codeindex_facts.org_id)`+where, append([]any{repositoryID}, scopeArgs...)...).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_snapshots WHERE id = ?`+where, append([]any{snapshotID}, scopeArgs...)...).Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewRaw(`UPDATE codeindex_repositories SET latest_snapshot_id = COALESCE(
			(SELECT id FROM codeindex_snapshots WHERE codeindex_snapshots.org_id = codeindex_repositories.org_id AND repository_id = ?
				ORDER BY created_unix DESC, capture_order DESC, id DESC LIMIT 1), '')
			WHERE id = ? AND latest_snapshot_id = ?`+where, append([]any{repositoryID, repositoryID, snapshotID}, scopeArgs...)...).Exec(ctx)
		return err
	})
}

// ListRepositories returns every indexed repository with a summary of its
// latest published snapshot. It resolves the latest snapshot and its record
// counts in a single query using the snapshot membership tables, matching the
// counts reported by Snapshot.
func (s *Store) ListRepositories(ctx context.Context) ([]*pb.Repository, error) {
	query := `SELECT
		r.id, r.root, r.latest_snapshot_id, r.remote_url, r.managed,
		COALESCE(s.created_unix, 0), COALESCE(s.git_revision, ''), COALESCE(s.git_branch, ''),
		(SELECT COUNT(*) FROM codeindex_snapshot_facts  WHERE snapshot_id = r.latest_snapshot_id AND org_id = r.org_id),
		(SELECT COUNT(*) FROM codeindex_snapshot_edges  WHERE snapshot_id = r.latest_snapshot_id AND org_id = r.org_id),
		(SELECT COUNT(*) FROM codeindex_sources         WHERE snapshot_id = r.latest_snapshot_id AND org_id = r.org_id)
		FROM codeindex_repositories r
		LEFT JOIN codeindex_snapshots s ON s.id = r.latest_snapshot_id AND s.org_id = r.org_id`
	args := []any{}
	if t := scope(ctx); t.on {
		query += ` WHERE r.org_id = ?`
		args = append(args, t.orgID)
	}
	query += ` ORDER BY r.root, r.id`
	rows, err := s.bun.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]*pb.Repository, 0)
	for rows.Next() {
		var summary pb.Repository
		if err := rows.Scan(
			&summary.Id, &summary.Root, &summary.LatestSnapshotId, &summary.RemoteUrl, &summary.Managed,
			&summary.LatestCreatedUnix, &summary.GitRevision, &summary.GitBranch,
			&summary.Facts, &summary.Edges, &summary.Sources,
		); err != nil {
			return nil, err
		}
		out = append(out, &summary)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// SetRepositoryOrigin records a repository's canonical remote URL, its
// normalized remote key, and whether tld owns the checkout.
func (s *Store) SetRepositoryOrigin(ctx context.Context, repositoryID, remoteURL string, managed bool) error {
	if strings.TrimSpace(repositoryID) == "" {
		return nil
	}
	where, args := scope(ctx).clause("org_id")
	_, err := s.bun.NewRaw(`UPDATE codeindex_repositories SET remote_url = ?, remote_key = ?, managed = ?, updated_at = ? WHERE id = ?`+where,
		append([]any{remoteURL, repolink.RemoteKey(remoteURL), managed, time.Now().UTC().Format(time.RFC3339), repositoryID}, args...)...).Exec(ctx)
	if err != nil {
		return fmt.Errorf("set repository origin: %w", err)
	}
	return nil
}

// SetRepositoryRemoteURL refreshes only the canonical remote URL and remote key,
// preserving the managed flag.
func (s *Store) SetRepositoryRemoteURL(ctx context.Context, repositoryID, remoteURL string) error {
	if strings.TrimSpace(repositoryID) == "" {
		return nil
	}
	where, args := scope(ctx).clause("org_id")
	_, err := s.bun.NewRaw(`UPDATE codeindex_repositories SET remote_url = ?, remote_key = ?, updated_at = ? WHERE id = ?`+where,
		append([]any{remoteURL, repolink.RemoteKey(remoteURL), time.Now().UTC().Format(time.RFC3339), repositoryID}, args...)...).Exec(ctx)
	if err != nil {
		return fmt.Errorf("set repository remote url: %w", err)
	}
	return nil
}

// RepositoryByRemoteKey returns the id of the canonical repository registered
// for a normalized remote key. When legacy duplicates exist, managed rows win,
// then the most recently updated. Rows written before remote_key existed are
// matched by normalizing their remote_url in Go.
func (s *Store) RepositoryByRemoteKey(ctx context.Context, remoteKey string) (string, bool, error) {
	remoteKey = strings.TrimSpace(remoteKey)
	if remoteKey == "" {
		return "", false, nil
	}
	where, scopeArgs := scope(ctx).clause("org_id")
	var id string
	err := s.bun.NewRaw(`SELECT id FROM codeindex_repositories WHERE remote_key = ?`+where+`
		ORDER BY managed DESC, updated_at DESC, id LIMIT 1`, append([]any{remoteKey}, scopeArgs...)...).Scan(ctx, &id)
	if err == nil && id != "" {
		return id, true, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("repository by remote key: %w", err)
	}

	type row struct {
		ID        string `bun:"id"`
		RemoteURL string `bun:"remote_url"`
	}
	var rows []row
	where, scopeArgs = scope(ctx).clause("org_id")
	if err := s.bun.NewRaw(`SELECT id, remote_url FROM codeindex_repositories
		WHERE remote_url <> ''`+where+` ORDER BY managed DESC, updated_at DESC, id`, scopeArgs...).Scan(ctx, &rows); err != nil {
		return "", false, fmt.Errorf("repository by remote key: %w", err)
	}
	for _, item := range rows {
		if strings.EqualFold(repolink.RemoteKey(item.RemoteURL), remoteKey) {
			return item.ID, true, nil
		}
	}
	return "", false, nil
}

// RepositoryExists reports whether a repository id is already registered.
func (s *Store) RepositoryExists(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, nil
	}
	where, scopeArgs := scope(ctx).clause("org_id")
	var count int
	if err := s.bun.NewRaw(`SELECT COUNT(*) FROM codeindex_repositories WHERE id = ?`+where, append([]any{id}, scopeArgs...)...).Scan(ctx, &count); err != nil {
		return false, fmt.Errorf("repository exists: %w", err)
	}
	return count > 0, nil
}

// EnsureRepositoryIdentity upserts a repository's stable identity fields
// without touching its latest snapshot pointer or snapshot data. Non-empty
// values only are applied on conflict, so a later scan from another checkout
// cannot blank a good remote URL or remote key.
func (s *Store) EnsureRepositoryIdentity(ctx context.Context, id, root, remoteURL, remoteKey string, managed bool) error {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.bun.NewRaw(`INSERT INTO codeindex_repositories (id, root, remote_url, remote_key, managed, latest_snapshot_id, created_at, updated_at, org_id)
		VALUES (?, ?, ?, ?, ?, '', ?, ?, ?)
		ON CONFLICT(org_id, id) DO UPDATE SET
			root = CASE WHEN excluded.root <> '' THEN excluded.root ELSE codeindex_repositories.root END,
			remote_url = CASE WHEN excluded.remote_url <> '' THEN excluded.remote_url ELSE codeindex_repositories.remote_url END,
			remote_key = CASE WHEN excluded.remote_key <> '' THEN excluded.remote_key ELSE codeindex_repositories.remote_key END,
			managed = codeindex_repositories.managed OR excluded.managed,
			updated_at = excluded.updated_at`,
		id, root, remoteURL, remoteKey, managed, now, now, scope(ctx).value()).Exec(ctx)
	if err != nil {
		return fmt.Errorf("ensure repository identity: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("ensure repository identity: %w", sql.ErrNoRows)
	}
	return nil
}

// BackfillRemoteKeys derives remote_key for repositories registered before the
// column existed. It is a no-op when every row already has a key. Normalization
// lives in Go because SQL cannot canonicalize git remote forms.
func (s *Store) BackfillRemoteKeys(ctx context.Context) error {
	type row struct {
		ID        string `bun:"id"`
		RemoteURL string `bun:"remote_url"`
	}
	where, scopeArgs := scope(ctx).clause("org_id")
	var rows []row
	if err := s.bun.NewRaw(`SELECT id, remote_url FROM codeindex_repositories WHERE remote_key = '' AND remote_url <> ''`+where, scopeArgs...).Scan(ctx, &rows); err != nil {
		return fmt.Errorf("backfill remote keys: %w", err)
	}
	for _, item := range rows {
		key := repolink.RemoteKey(item.RemoteURL)
		if key == "" {
			continue
		}
		// A different row already owns this remote; leave this one un-backfilled
		// rather than violate the unique index. Resolvers still match it by
		// normalizing remote_url.
		if owner, ok, err := s.RepositoryByRemoteKey(ctx, key); err == nil && ok && owner != item.ID {
			continue
		}
		if _, err := s.bun.NewRaw(`UPDATE codeindex_repositories SET remote_key = ? WHERE id = ?`+where, append([]any{key, item.ID}, scopeArgs...)...).Exec(ctx); err != nil {
			continue
		}
	}
	return nil
}

// RepositoryOrigin returns a repository's canonical remote URL and whether tld
// owns the checkout.
func (s *Store) RepositoryOrigin(ctx context.Context, repositoryID string) (string, bool, error) {
	var row struct {
		RemoteURL string `bun:"remote_url"`
		Managed   bool   `bun:"managed"`
	}
	where, scopeArgs := scope(ctx).clause("org_id")
	err := s.bun.NewRaw(`SELECT remote_url, managed FROM codeindex_repositories WHERE id = ?`+where, append([]any{repositoryID}, scopeArgs...)...).Scan(ctx, &row)
	if err != nil {
		return "", false, fmt.Errorf("repository origin: %w", err)
	}
	return row.RemoteURL, row.Managed, nil
}
