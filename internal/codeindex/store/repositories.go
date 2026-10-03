package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/uptrace/bun"
)

// DeleteRepository removes a repository, every snapshot it published, and all
// snapshot-scoped records (sources, facts, chunks, edges, embeddings, analysis
// runs and their groups), plus any resource mappings recorded for it. It does
// not touch workspace resources materialized from the repository.
func (s *Store) DeleteRepository(ctx context.Context, repositoryID string) error {
	if strings.TrimSpace(repositoryID) == "" {
		return fmt.Errorf("repository id is required")
	}
	snapshotScoped := []string{
		"codeindex_embeddings",
		"codeindex_fact_embeddings",
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

// ListRepositories returns every indexed repository with a summary of its
// latest published snapshot.
func (s *Store) ListRepositories(ctx context.Context) ([]*pb.RepositorySummary, error) {
	rows, err := s.bun.QueryContext(ctx, `SELECT id, root, latest_snapshot_id FROM codeindex_repositories ORDER BY root, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	type repoRow struct {
		id     string
		root   string
		latest string
	}
	var repos []repoRow
	for rows.Next() {
		var r repoRow
		if err := rows.Scan(&r.id, &r.root, &r.latest); err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]*pb.RepositorySummary, 0, len(repos))
	for _, r := range repos {
		summary := &pb.RepositorySummary{Id: r.id, Root: r.root, LatestSnapshotId: r.latest}
		if r.latest != "" {
			if err := s.fillSnapshotSummary(ctx, summary); err != nil {
				return nil, err
			}
		}
		out = append(out, summary)
	}
	return out, nil
}

func (s *Store) fillSnapshotSummary(ctx context.Context, summary *pb.RepositorySummary) error {
	var created int64
	var revision, branch string
	err := s.bun.NewRaw(`SELECT created_unix, git_revision, git_branch FROM codeindex_snapshots WHERE id = ?`, summary.LatestSnapshotId).
		Scan(ctx, &created, &revision, &branch)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	summary.LatestCreatedUnix = created
	summary.GitRevision = revision
	summary.GitBranch = branch

	counts := []struct {
		table string
		dest  *uint32
	}{
		{"codeindex_facts", &summary.Facts},
		{"codeindex_chunks", &summary.Chunks},
		{"codeindex_edges", &summary.Edges},
		{"codeindex_sources", &summary.Sources},
	}
	for _, c := range counts {
		var n int64
		if err := s.bun.NewRaw("SELECT COUNT(*) FROM "+c.table+" WHERE snapshot_id = ?", summary.LatestSnapshotId).Scan(ctx, &n); err != nil {
			return err
		}
		*c.dest = uint32(n)
	}
	return nil
}
