package store

import (
	"context"
	"encoding/json"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

func (s *Store) SaveCompletedMap(ctx context.Context, repositoryID string, mapped *pb.CompletedMap) error {
	raw, err := json.Marshal(mapped.Result)
	if err != nil {
		return err
	}
	if mapped.CompletedUnix == 0 {
		mapped.CompletedUnix = time.Now().Unix()
	}
	_, err = s.bun.NewRaw(`INSERT INTO codeindex_completed_maps (run_id, repository_id, snapshot_id, profile, include_imports, config_hash, completed_unix, result_json)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(run_id) DO UPDATE SET completed_unix = excluded.completed_unix, result_json = excluded.result_json`,
		mapped.Result.RunId, repositoryID, mapped.Result.SnapshotId, mapped.Profile, mapped.IncludeImports, mapped.ConfigHash, mapped.CompletedUnix, string(raw)).Exec(ctx)
	return err
}

func (s *Store) CompletedMaps(ctx context.Context, repositoryID string) ([]*pb.CompletedMap, error) {
	rows, err := s.bun.QueryContext(ctx, `SELECT completed_unix, profile, include_imports, config_hash, result_json FROM codeindex_completed_maps WHERE repository_id = ? ORDER BY completed_unix DESC, run_id`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []*pb.CompletedMap{}
	for rows.Next() {
		item := &pb.CompletedMap{Result: &pb.MapResult{}}
		var raw string
		if err := rows.Scan(&item.CompletedUnix, &item.Profile, &item.IncludeImports, &item.ConfigHash, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), item.Result); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
