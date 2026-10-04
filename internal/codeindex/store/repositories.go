package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/uptrace/bun"
)

// DeleteRepository removes a repository, every snapshot it published, and all
// snapshot-scoped records (sources, facts, chunks, edges, analysis runs and
// their groups), plus any resource mappings recorded for it. It does not touch
// workspace resources materialized from the repository.
func (s *Store) DeleteRepository(ctx context.Context, repositoryID string) error {
	if strings.TrimSpace(repositoryID) == "" {
		return fmt.Errorf("repository id is required")
	}
	snapshotScoped := []string{
		"codeindex_project_artifacts",
		"codeindex_chunks",
		"codeindex_edges",
		"codeindex_facts",
		"codeindex_sources",
	}
	return s.bun.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewRaw(`DELETE FROM codeindex_group_members WHERE group_id IN (
			SELECT id FROM codeindex_groups WHERE run_id IN (
				SELECT id FROM codeindex_analysis_runs WHERE repository_id = ?))`, repositoryID).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_groups WHERE run_id IN (
			SELECT id FROM codeindex_analysis_runs WHERE repository_id = ?)`, repositoryID).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_analysis_runs WHERE repository_id = ?`, repositoryID).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_completed_maps WHERE repository_id = ?`, repositoryID).Exec(ctx); err != nil {
			return err
		}
		for _, table := range []string{"codeindex_impacts", "codeindex_watch_state", "codeindex_leases", "codeindex_repository_settings"} {
			if _, err := tx.NewRaw(`DELETE FROM `+table+` WHERE repository_id = ?`, repositoryID).Exec(ctx); err != nil {
				return err
			}
		}
		for _, table := range snapshotScoped {
			if _, err := tx.NewRaw(`DELETE FROM `+table+` WHERE snapshot_id IN (
				SELECT id FROM codeindex_snapshots WHERE repository_id = ?)`, repositoryID).Exec(ctx); err != nil {
				return err
			}
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_snapshots WHERE repository_id = ?`, repositoryID).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_elements WHERE repository_id = ?`, repositoryID).Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewRaw(`DELETE FROM codeindex_repositories WHERE id = ?`, repositoryID).Exec(ctx)
		return err
	})
}

// DeleteSnapshot removes one published snapshot and the records scoped to it
// (sources, project artifacts, analysis runs and their groups, completed maps,
// and resource mappings). Facts, chunks, and edges are shared
// across snapshots through membership tables, so only this snapshot's
// membership is removed; entities no longer referenced by any snapshot are
// garbage collected. When the deleted snapshot was the repository's latest, the
// latest pointer moves to the newest remaining snapshot.
func (s *Store) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	if strings.TrimSpace(snapshotID) == "" {
		return fmt.Errorf("snapshot id is required")
	}
	var repositoryID string
	if err := s.bun.NewRaw(`SELECT repository_id FROM codeindex_snapshots WHERE id = ?`, snapshotID).Scan(ctx, &repositoryID); err != nil {
		return err
	}
	return s.bun.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewRaw(`DELETE FROM codeindex_group_members WHERE group_id IN (
			SELECT id FROM codeindex_groups WHERE snapshot_id = ?)`, snapshotID).Exec(ctx); err != nil {
			return err
		}
		for _, table := range []string{
			"codeindex_groups",
			"codeindex_analysis_runs",
			"codeindex_completed_maps",
			"codeindex_project_artifacts",
			"codeindex_sources",
			"codeindex_snapshot_facts",
			"codeindex_snapshot_chunks",
			"codeindex_snapshot_edges",
			"codeindex_elements",
		} {
			if _, err := tx.NewRaw("DELETE FROM "+table+" WHERE snapshot_id = ?", snapshotID).Exec(ctx); err != nil {
				return err
			}
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_edges
			WHERE repository_id = ? AND id NOT IN (SELECT edge_id FROM codeindex_snapshot_edges)`, repositoryID).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_chunks
			WHERE id NOT IN (SELECT chunk_id FROM codeindex_snapshot_chunks)
			AND fact_id IN (SELECT id FROM codeindex_facts WHERE repository_id = ?)`, repositoryID).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_facts
			WHERE repository_id = ? AND id NOT IN (SELECT fact_id FROM codeindex_snapshot_facts)`, repositoryID).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_snapshots WHERE id = ?`, snapshotID).Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewRaw(`UPDATE codeindex_repositories SET latest_snapshot_id = COALESCE(
			(SELECT id FROM codeindex_snapshots WHERE repository_id = ?
				ORDER BY created_unix DESC, capture_order DESC, id DESC LIMIT 1), '')
			WHERE id = ? AND latest_snapshot_id = ?`, repositoryID, repositoryID, snapshotID).Exec(ctx)
		return err
	})
}

// ListRepositories returns every indexed repository with a summary of its
// latest published snapshot. It resolves the latest snapshot and its record
// counts in a single query using the snapshot membership tables, matching the
// counts reported by Snapshot.
func (s *Store) ListRepositories(ctx context.Context) ([]*pb.RepositorySummary, error) {
	rows, err := s.bun.QueryContext(ctx, `SELECT
		r.id, r.root, r.latest_snapshot_id, r.remote_url, r.managed,
		COALESCE(s.created_unix, 0), COALESCE(s.git_revision, ''), COALESCE(s.git_branch, ''),
		(SELECT COUNT(*) FROM codeindex_snapshot_facts  WHERE snapshot_id = r.latest_snapshot_id),
		(SELECT COUNT(*) FROM codeindex_snapshot_chunks WHERE snapshot_id = r.latest_snapshot_id),
		(SELECT COUNT(*) FROM codeindex_snapshot_edges  WHERE snapshot_id = r.latest_snapshot_id),
		(SELECT COUNT(*) FROM codeindex_sources         WHERE snapshot_id = r.latest_snapshot_id)
		FROM codeindex_repositories r
		LEFT JOIN codeindex_snapshots s ON s.id = r.latest_snapshot_id
		ORDER BY r.root, r.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]*pb.RepositorySummary, 0)
	for rows.Next() {
		var summary pb.RepositorySummary
		if err := rows.Scan(
			&summary.Id, &summary.Root, &summary.LatestSnapshotId, &summary.RemoteUrl, &summary.Managed,
			&summary.LatestCreatedUnix, &summary.GitRevision, &summary.GitBranch,
			&summary.Facts, &summary.Chunks, &summary.Edges, &summary.Sources,
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

// SetRepositoryOrigin records a repository's canonical remote URL and whether
// tld owns the checkout.
func (s *Store) SetRepositoryOrigin(ctx context.Context, repositoryID, remoteURL string, managed bool) error {
	if strings.TrimSpace(repositoryID) == "" {
		return nil
	}
	_, err := s.bun.NewRaw(`UPDATE codeindex_repositories SET remote_url = ?, managed = ?, updated_at = ? WHERE id = ?`,
		remoteURL, managed, time.Now().UTC().Format(time.RFC3339), repositoryID).Exec(ctx)
	if err != nil {
		return fmt.Errorf("set repository origin: %w", err)
	}
	return nil
}

// SetRepositoryRemoteURL refreshes only the canonical remote URL, preserving
// the managed flag.
func (s *Store) SetRepositoryRemoteURL(ctx context.Context, repositoryID, remoteURL string) error {
	if strings.TrimSpace(repositoryID) == "" {
		return nil
	}
	_, err := s.bun.NewRaw(`UPDATE codeindex_repositories SET remote_url = ?, updated_at = ? WHERE id = ?`,
		remoteURL, time.Now().UTC().Format(time.RFC3339), repositoryID).Exec(ctx)
	if err != nil {
		return fmt.Errorf("set repository remote url: %w", err)
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
	err := s.bun.NewRaw(`SELECT remote_url, managed FROM codeindex_repositories WHERE id = ?`, repositoryID).Scan(ctx, &row)
	if err != nil {
		return "", false, fmt.Errorf("repository origin: %w", err)
	}
	return row.RemoteURL, row.Managed, nil
}
