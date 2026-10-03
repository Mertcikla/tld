package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
)

// FactScore is one ranked fact returned by similarity search.
type FactScore struct {
	FactID string
	Score  float32
}

// EnsureVectorSchema creates the sqlite-vec virtual table (SQLite) or the
// pgvector extension (Postgres). It is idempotent and cached per store.
func (s *Store) EnsureVectorSchema(ctx context.Context) error {
	s.vecOnce.Do(func() { s.vecErr = s.ensureVectorSchema(ctx) })
	return s.vecErr
}

func (s *Store) ensureVectorSchema(ctx context.Context) error {
	if s.dialect == dbrepo.DialectPostgres {
		_, err := s.bun.NewRaw(`CREATE EXTENSION IF NOT EXISTS vector`).Exec(ctx)
		return err
	}
	if _, err := s.bun.NewRaw(`CREATE TABLE IF NOT EXISTS _vec_codeindex_embedding_vec (
		dataset_id TEXT NOT NULL,
		id TEXT NOT NULL,
		content TEXT,
		meta TEXT,
		embedding BLOB,
		PRIMARY KEY(dataset_id, id)
	)`).Exec(ctx); err != nil {
		return err
	}
	dbPath, err := sqliteMainDBPath(ctx, s.db)
	if err != nil {
		return err
	}
	create := `CREATE VIRTUAL TABLE IF NOT EXISTS codeindex_embedding_vec USING vec(id)`
	if dbPath != "" {
		create = fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS codeindex_embedding_vec USING vec(id, dbpath='%s')`, strings.ReplaceAll(dbPath, "'", "''"))
	}
	_, err = s.bun.NewRaw(create).Exec(ctx)
	return err
}

// indexFactVector mirrors a fact embedding into the ANN index. It is a no-op
// when no embedding vector is present.
func (s *Store) indexFactVector(ctx context.Context, id string, e *pb.Embedding) error {
	if len(e.Vector) == 0 {
		return nil
	}
	if err := s.EnsureVectorSchema(ctx); err != nil {
		return err
	}
	if s.dialect == dbrepo.DialectPostgres {
		_, err := s.bun.NewRaw(`UPDATE codeindex_fact_embeddings SET embedding = ?::vector WHERE id = ?`,
			pgVectorLiteral(e.Vector), id).Exec(ctx)
		return err
	}
	_, err := s.bun.NewRaw(`INSERT INTO _vec_codeindex_embedding_vec(dataset_id, id, content, meta, embedding)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(dataset_id, id) DO UPDATE SET
			content = excluded.content,
			meta = excluded.meta,
			embedding = excluded.embedding`,
		e.Profile, id, e.FactId, e.SnapshotId, encodeVector(e.Vector)).Exec(ctx)
	return err
}

// SimilarFacts returns a snapshot's facts ordered by embedding similarity to
// query for the given profile. It uses sqlite-vec/pgvector when available and
// falls back to an in-Go cosine scan.
func (s *Store) SimilarFacts(ctx context.Context, snapshotID, profile string, query []float32, limit int) ([]FactScore, error) {
	if len(query) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	if err := s.EnsureVectorSchema(ctx); err == nil {
		if scores, err := s.similarFactsIndexed(ctx, snapshotID, profile, query, limit); err == nil && len(scores) > 0 {
			return scores, nil
		}
	}
	return s.similarFactsFallback(ctx, snapshotID, profile, query, limit)
}

func (s *Store) similarFactsIndexed(ctx context.Context, snapshotID, profile string, query []float32, limit int) ([]FactScore, error) {
	var (
		ids []string
		err error
	)
	if s.dialect == dbrepo.DialectPostgres {
		err = s.bun.NewRaw(`SELECT id FROM codeindex_fact_embeddings
			WHERE snapshot_id = ? AND profile = ? AND embedding IS NOT NULL
			ORDER BY embedding <=> ?::vector LIMIT ?`, snapshotID, profile, pgVectorLiteral(query), limit).Scan(ctx, &ids)
	} else {
		encoded, encErr := encodeVectorChecked(query)
		if encErr != nil {
			return nil, encErr
		}
		err = s.bun.NewRaw(`SELECT id FROM codeindex_embedding_vec
			WHERE dataset_id = ? AND id MATCH ?
			LIMIT ?`, profile, encoded, limit).Scan(ctx, &ids)
	}
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	// Resolve index rows to fact ids that belong to the snapshot, preserving
	// rank order.
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+2)
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, profile, snapshotID)
	rows, err := s.bun.QueryContext(ctx, `SELECT fe.id, fe.fact_id FROM codeindex_fact_embeddings fe
		WHERE fe.id IN (`+placeholders+`) AND fe.profile = ? AND fe.fact_id IN (SELECT fact_id FROM codeindex_snapshot_facts WHERE snapshot_id = ?)`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	factByID := map[string]string{}
	for rows.Next() {
		var id, factID string
		if err := rows.Scan(&id, &factID); err != nil {
			return nil, err
		}
		factByID[id] = factID
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]FactScore, 0, len(ids))
	for _, id := range ids {
		if factID, ok := factByID[id]; ok {
			out = append(out, FactScore{FactID: factID})
		}
	}
	return out, nil
}

func (s *Store) similarFactsFallback(ctx context.Context, snapshotID, profile string, query []float32, limit int) ([]FactScore, error) {
	rows, err := s.bun.QueryContext(ctx, `SELECT fe.fact_id, fe.vector FROM codeindex_fact_embeddings fe
		WHERE fe.profile = ? AND fe.fact_id IN (SELECT fact_id FROM codeindex_snapshot_facts WHERE snapshot_id = ?)`, profile, snapshotID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []FactScore
	for rows.Next() {
		var factID string
		var blob []byte
		if err := rows.Scan(&factID, &blob); err != nil {
			return nil, err
		}
		out = append(out, FactScore{FactID: factID, Score: cosineSimilarity(query, decodeVector(blob))})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// FactSimilarities scores a specific set of facts against the query vector
// using their stored embeddings. It backs populate, which ranks a bounded set
// of vector-search hits and needs a numeric score for display.
func (s *Store) FactSimilarities(ctx context.Context, snapshotID, profile string, query []float32, factIDs []string) (map[string]float32, error) {
	out := make(map[string]float32, len(factIDs))
	if len(factIDs) == 0 || len(query) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(factIDs)), ",")
	args := make([]any, 0, len(factIDs)+2)
	for _, id := range factIDs {
		args = append(args, id)
	}
	args = append(args, profile)
	rows, err := s.bun.QueryContext(ctx, `SELECT fe.fact_id, fe.vector FROM codeindex_fact_embeddings fe
		WHERE fe.fact_id IN (`+placeholders+`) AND fe.profile = ?`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var factID string
		var blob []byte
		if err := rows.Scan(&factID, &blob); err != nil {
			return nil, err
		}
		out[factID] = cosineSimilarity(query, decodeVector(blob))
	}
	return out, rows.Err()
}

func encodeVectorChecked(v []float32) ([]byte, error) {
	if len(v) == 0 {
		return nil, fmt.Errorf("empty vector")
	}
	return encodeVector(v), nil
}

func cosineSimilarity(left, right []float32) float32 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	var dot, leftNorm, rightNorm float64
	for i := range left {
		l := float64(left[i])
		r := float64(right[i])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm)))
}

func sqliteMainDBPath(ctx context.Context, db *sql.DB) (string, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA database_list`)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var seq int
		var name, file string
		if err := rows.Scan(&seq, &name, &file); err != nil {
			return "", err
		}
		if name == "main" {
			return file, nil
		}
	}
	return "", rows.Err()
}

func pgVectorLiteral(v []float32) string {
	parts := make([]string, 0, len(v))
	for _, value := range v {
		parts = append(parts, fmt.Sprintf("%g", value))
	}
	return "[" + strings.Join(parts, ",") + "]"
}
