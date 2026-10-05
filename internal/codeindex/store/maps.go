package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/uptrace/bun"
)

func (s *Store) SaveCompletedMap(ctx context.Context, repositoryID string, mapped *pb.CompletedMap) error {
	raw, err := json.Marshal(mapped.Result)
	if err != nil {
		return err
	}
	if mapped.CompletedUnix == 0 {
		mapped.CompletedUnix = time.Now().Unix()
	}
	org := scope(ctx).value()
	return s.bun.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewRaw(`INSERT INTO codeindex_completed_maps (run_id, repository_id, snapshot_id, config_hash, completed_unix, result_json, org_id)
 VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(run_id) DO UPDATE SET completed_unix = excluded.completed_unix, result_json = excluded.result_json, org_id = COALESCE(codeindex_completed_maps.org_id, excluded.org_id)`,
			mapped.Result.RunId, repositoryID, mapped.Result.SnapshotId, mapped.ConfigHash, mapped.CompletedUnix, string(raw), org).Exec(ctx)
		if err != nil {
			return err
		}
		_, err = tx.NewRaw(`INSERT INTO codeindex_active_maps (repository_id, run_id, org_id) VALUES (?, ?, ?)
 ON CONFLICT(repository_id) DO UPDATE SET run_id = excluded.run_id, org_id = COALESCE(codeindex_active_maps.org_id, excluded.org_id)`, repositoryID, mapped.Result.RunId, org).Exec(ctx)
		return err
	})
}

func (s *Store) CompletedMaps(ctx context.Context, repositoryID string) ([]*pb.CompletedMap, error) {
	where, scopeArgs := scope(ctx).clause("org_id")
	rows, err := s.bun.QueryContext(ctx, `SELECT completed_unix, config_hash, result_json FROM codeindex_completed_maps WHERE repository_id = ?`+where+` ORDER BY completed_unix DESC, run_id`, append([]any{repositoryID}, scopeArgs...)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []*pb.CompletedMap{}
	for rows.Next() {
		item := &pb.CompletedMap{Result: &pb.MapResult{}}
		var raw string
		if err := rows.Scan(&item.CompletedUnix, &item.ConfigHash, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), item.Result); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// ActiveMap returns the run currently materialized in the repository's shared views.
func (s *Store) ActiveMap(ctx context.Context, repositoryID string) (*pb.CompletedMap, error) {
	item := &pb.CompletedMap{Result: &pb.MapResult{}}
	var raw string
	where, scopeArgs := scope(ctx).clause("a.org_id")
	err := s.bun.QueryRowContext(ctx, `SELECT m.completed_unix, m.config_hash, m.result_json
 FROM codeindex_completed_maps m JOIN codeindex_active_maps a ON a.run_id = m.run_id
 WHERE a.repository_id = ?`+where, append([]any{repositoryID}, scopeArgs...)...).Scan(&item.CompletedUnix, &item.ConfigHash, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(raw), item.Result); err != nil {
		return nil, err
	}
	return item, nil
}

// InvalidateActiveMap clears the cache before workspace mutations, including runs
// that fail partway through materialization.
func (s *Store) InvalidateActiveMap(ctx context.Context, repositoryID string) error {
	where, scopeArgs := scope(ctx).clause("org_id")
	_, err := s.bun.NewRaw(`DELETE FROM codeindex_active_maps WHERE repository_id = ?`+where, append([]any{repositoryID}, scopeArgs...)...).Exec(ctx)
	return err
}
