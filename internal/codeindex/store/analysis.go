package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/uptrace/bun"
)

// FactVector pairs a code fact with one of its mean-pooled embedding vectors.
// Path is the fact's source path (CodeFact carries it via its anchor but the
// mapper loader reads it directly).
type FactVector struct {
	Fact   *pb.CodeFact
	Path   string
	Vector []float32
}

// MajorityProfile returns the profile with the most fact embeddings for a
// snapshot, breaking count ties by profile name. An empty result means the
// snapshot has no fact embeddings.
func (s *Store) MajorityProfile(ctx context.Context, snapshotID string) (string, error) {
	var profile string
	err := s.bun.NewRaw(`SELECT profile FROM codeindex_fact_embeddings
		WHERE snapshot_id = ? GROUP BY profile ORDER BY COUNT(*) DESC, profile LIMIT 1`, snapshotID).Scan(ctx, &profile)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return profile, nil
}

// FactEmbeddings loads a snapshot's fact embeddings for a profile, ordered by
// fact id. Rows with missing, empty or misaligned vector blobs are skipped. A
// zero kind selects every fact kind.
func (s *Store) FactEmbeddings(ctx context.Context, snapshotID, profile string, kind pb.FactKind) ([]FactVector, error) {
	query := `SELECT f.id, f.repository_id, f.snapshot_id, f.language, f.kind, f.name, f.qualified_name, f.path, f.logical_key, f.symbol_key, e.vector
		FROM codeindex_fact_embeddings e
		JOIN codeindex_facts f ON f.id = e.fact_id AND f.snapshot_id = e.snapshot_id
		WHERE e.snapshot_id = ? AND e.profile = ?`
	args := []any{snapshotID, profile}
	if kind != pb.FactKind_FACT_KIND_UNSPECIFIED {
		query += ` AND f.kind = ?`
		args = append(args, int(kind))
	}
	query += ` ORDER BY f.id`
	rows, err := s.bun.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]FactVector, 0)
	for rows.Next() {
		var (
			fact      pb.CodeFact
			path      string
			kindValue int
			blob      []byte
		)
		if err := rows.Scan(&fact.Id, &fact.RepositoryId, &fact.SnapshotId, &fact.Language, &kindValue,
			&fact.Name, &fact.QualifiedName, &path, &fact.LogicalKey, &fact.SymbolKey, &blob); err != nil {
			return nil, err
		}
		vector := decodeVector(blob)
		if len(vector) == 0 {
			continue
		}
		fact.Kind = pb.FactKind(kindValue)
		out = append(out, FactVector{Fact: &fact, Path: path, Vector: vector})
	}
	return out, rows.Err()
}

// AnalysisGroup is one persisted cluster/community produced by an analysis run.
type AnalysisGroup struct {
	ID      string
	Label   string
	Kind    pb.GroupKind
	Profile string
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
			if _, err := tx.NewRaw(`INSERT INTO codeindex_groups (id, run_id, snapshot_id, label, kind, profile, size)
				VALUES (?, ?, ?, ?, ?, ?, ?)`, group.ID, run.ID, run.SnapshotID, group.Label, int(group.Kind), group.Profile, len(group.Members)).Exec(ctx); err != nil {
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
