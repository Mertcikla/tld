package store

import (
	"context"
	"encoding/json"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/uptrace/bun"
)

// AnalysisGroup is one persisted grouping produced by an analysis run.
type AnalysisGroup struct {
	ID      string
	Label   string
	Kind    pb.GroupKind
	Members []string
}

// AnalysisRun records the provenance and results of one grouping decision.
type AnalysisRun struct {
	ID           string
	RepositoryID string
	SnapshotID   string
	Algorithm    string
	Params       map[string]string
	CreatedUnix  int64
	Groups       []AnalysisGroup
}

// SaveAnalysis upserts a run and replaces its groups and members atomically.
func (s *Store) SaveAnalysis(ctx context.Context, run AnalysisRun) error {
	params, err := json.Marshal(run.Params)
	if err != nil || run.Params == nil {
		params = []byte("{}")
	}
	created := run.CreatedUnix
	if created == 0 {
		created = time.Now().Unix()
	}
	return s.bun.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewRaw(`DELETE FROM codeindex_group_members WHERE group_id IN (SELECT id FROM codeindex_groups WHERE run_id = ?)`, run.ID).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_groups WHERE run_id = ?`, run.ID).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_analysis_runs WHERE id = ?`, run.ID).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`INSERT INTO codeindex_analysis_runs (id, repository_id, snapshot_id, algorithm, params_json, created_unix)
			VALUES (?, ?, ?, ?, ?, ?)`, run.ID, run.RepositoryID, run.SnapshotID, run.Algorithm, string(params), created).Exec(ctx); err != nil {
			return err
		}
		for _, group := range run.Groups {
			if _, err := tx.NewRaw(`INSERT INTO codeindex_groups (id, run_id, snapshot_id, label, kind, size)
				VALUES (?, ?, ?, ?, ?, ?)`, group.ID, run.ID, run.SnapshotID, group.Label, int(group.Kind), len(group.Members)).Exec(ctx); err != nil {
				return err
			}
			for _, member := range group.Members {
				if _, err := tx.NewRaw(`INSERT INTO codeindex_group_members (group_id, fact_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, group.ID, member).Exec(ctx); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
