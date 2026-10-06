package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/uptrace/bun"
)

// GroupKind classifies a persisted analysis group.
type GroupKind int

const (
	GroupKindUnspecified GroupKind = 0
	GroupKindCommunity   GroupKind = 1
	GroupKindCluster     GroupKind = 2
)

// AnalysisGroup is one persisted grouping produced by an analysis run.
type AnalysisGroup struct {
	ID      string
	Label   string
	Kind    GroupKind
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
	org := scope(ctx).value()
	memberWhere, memberArgs := scope(ctx).clause("org_id")
	groupWhere, groupArgs := scope(ctx).clause("org_id")
	runWhere, runArgs := scope(ctx).clause("org_id")
	return s.bun.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewRaw(`DELETE FROM codeindex_group_members WHERE group_id IN (SELECT id FROM codeindex_groups WHERE org_id = codeindex_group_members.org_id AND run_id = ?)`+memberWhere, append([]any{run.ID}, memberArgs...)...).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_groups WHERE run_id = ?`+groupWhere, append([]any{run.ID}, groupArgs...)...).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`DELETE FROM codeindex_analysis_runs WHERE id = ?`+runWhere, append([]any{run.ID}, runArgs...)...).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(`INSERT INTO codeindex_analysis_runs (id, repository_id, snapshot_id, algorithm, params_json, created_unix, org_id)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, run.ID, run.RepositoryID, run.SnapshotID, run.Algorithm, string(params), created, org).Exec(ctx); err != nil {
			return err
		}
		for _, group := range run.Groups {
			if _, err := tx.NewRaw(`INSERT INTO codeindex_groups (id, run_id, snapshot_id, label, kind, size, org_id)
				VALUES (?, ?, ?, ?, ?, ?, ?)`, group.ID, run.ID, run.SnapshotID, group.Label, int(group.Kind), len(group.Members), org).Exec(ctx); err != nil {
				return err
			}
			for _, member := range group.Members {
				if _, err := tx.NewRaw(`INSERT INTO codeindex_group_members (group_id, fact_id, org_id) VALUES (?, ?, ?) ON CONFLICT(org_id, group_id, fact_id) DO NOTHING`, group.ID, member, org).Exec(ctx); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
