package store

import (
	"context"
	"database/sql"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

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
