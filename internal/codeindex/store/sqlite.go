package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/uptrace/bun"
)

// Publish writes an immutable snapshot and its graph in one transaction. A
// repeat publish of the same snapshot id replaces its rows (idempotent retry).
func (s *Store) Publish(ctx context.Context, root string, snap *pb.Snapshot, g *graph.Graph) error {
	if snap == nil || snap.Id == "" {
		return fmt.Errorf("publish: snapshot id is required")
	}
	return s.publish(ctx, root, snap, g, true)
}

// PublishHistorical retains the live repository's latest pointer.
func (s *Store) PublishHistorical(ctx context.Context, root string, snap *pb.Snapshot, g *graph.Graph) error {
	return s.publish(ctx, root, snap, g, false)
}

func (s *Store) publish(ctx context.Context, root string, snap *pb.Snapshot, g *graph.Graph, advanceLatest bool) error {
	if snap == nil || snap.Id == "" {
		return fmt.Errorf("publish: snapshot id is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	repoID := snap.RepositoryId
	return s.bun.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewRaw(`INSERT INTO codeindex_repositories (id, root, latest_snapshot_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET root = excluded.root, latest_snapshot_id = CASE WHEN ? OR codeindex_repositories.latest_snapshot_id = '' THEN excluded.latest_snapshot_id ELSE codeindex_repositories.latest_snapshot_id END, updated_at = excluded.updated_at`,
			repoID, root, snap.Id, now, now, advanceLatest).Exec(ctx); err != nil {
			return fmt.Errorf("upsert repository: %w", err)
		}
		for _, table := range []string{"codeindex_project_artifacts", "codeindex_sources", "codeindex_facts", "codeindex_chunks", "codeindex_edges", "codeindex_snapshot_facts", "codeindex_snapshot_chunks", "codeindex_snapshot_edges"} {
			if _, err := tx.NewRaw("DELETE FROM "+table+" WHERE snapshot_id = ?", snap.Id).Exec(ctx); err != nil {
				return fmt.Errorf("clear %s: %w", table, err)
			}
		}
		if err := saveSnapshotRow(ctx, tx, snap); err != nil {
			return err
		}
		if err := saveSources(ctx, tx, snap, g); err != nil {
			return err
		}
		if err := saveFacts(ctx, tx, snap, g); err != nil {
			return err
		}
		if err := saveChunks(ctx, tx, g); err != nil {
			return err
		}
		if err := saveEdges(ctx, tx, g); err != nil {
			return err
		}
		if err := saveMembership(ctx, tx, snap, g); err != nil {
			return err
		}
		return nil
	})
}

// saveMembership records which entities belong to a snapshot. Membership is
// small and lets reused entities be shared across snapshots.
func saveMembership(ctx context.Context, tx bun.Tx, snap *pb.Snapshot, g *graph.Graph) error {
	for id := range g.Facts {
		if _, err := tx.NewRaw(`INSERT OR IGNORE INTO codeindex_snapshot_facts (snapshot_id, fact_id) VALUES (?, ?)`, snap.Id, id).Exec(ctx); err != nil {
			return fmt.Errorf("membership fact %s: %w", id, err)
		}
	}
	for id := range g.Chunks {
		if _, err := tx.NewRaw(`INSERT OR IGNORE INTO codeindex_snapshot_chunks (snapshot_id, chunk_id) VALUES (?, ?)`, snap.Id, id).Exec(ctx); err != nil {
			return fmt.Errorf("membership chunk %s: %w", id, err)
		}
	}
	for id := range g.EdgeFacts {
		if _, err := tx.NewRaw(`INSERT OR IGNORE INTO codeindex_snapshot_edges (snapshot_id, edge_id) VALUES (?, ?)`, snap.Id, id).Exec(ctx); err != nil {
			return fmt.Errorf("membership edge %s: %w", id, err)
		}
	}
	return nil
}

func saveSnapshotRow(ctx context.Context, tx bun.Tx, snap *pb.Snapshot) error {
	projects, _ := marshalJSON(snap.Projects)
	warnings, _ := marshalJSON(snap.Warnings)
	tools, _ := marshalJSON(snap.ToolVersions)
	_, err := tx.NewRaw(`INSERT INTO codeindex_snapshots
		(id, repository_id, created_unix, git_revision, git_branch, ingestion_status, embedding_status, config_hash, projects_json, warnings_json, tool_versions_json, provenance, content_fingerprint, capture_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			repository_id = excluded.repository_id,
			git_revision = excluded.git_revision,
			git_branch = excluded.git_branch,
			ingestion_status = excluded.ingestion_status,
			embedding_status = excluded.embedding_status,
			config_hash = excluded.config_hash,
			projects_json = excluded.projects_json,
			warnings_json = excluded.warnings_json,
			tool_versions_json = excluded.tool_versions_json,
        provenance = excluded.provenance, content_fingerprint = excluded.content_fingerprint`,
		snap.Id, snap.RepositoryId, snap.CreatedUnix, snap.GitRevision, snap.GitBranch,
		snap.IngestionStatus, snap.EmbeddingStatus, snap.ConfigHash, projects, warnings, tools, snap.Provenance, snap.ContentFingerprint, time.Now().UnixNano()).Exec(ctx)
	if err != nil {
		return fmt.Errorf("upsert snapshot: %w", err)
	}
	return nil
}

func saveSources(ctx context.Context, tx bun.Tx, snap *pb.Snapshot, g *graph.Graph) error {
	if g != nil {
		for key, artifact := range g.ProjectArtifacts {
			if _, err := tx.NewRaw(`INSERT INTO codeindex_project_artifacts (snapshot_id, project_key, fingerprint, data) VALUES (?, ?, ?, ?)`, snap.Id, key, artifact.Fingerprint, artifact.Data).Exec(ctx); err != nil {
				return err
			}
		}
	}
	for _, src := range snap.Sources {
		var content []byte
		var language, blob, cache, fileCache string
		var dirty bool
		if g != nil {
			if s := g.Sources[src.Path]; s != nil {
				content = s.Text
				language, blob, cache, fileCache, dirty = s.Language, s.InputBlob, s.SyntaxCache, s.FileCache, s.Dirty
			}
		}
		if _, err := tx.NewRaw(`INSERT INTO codeindex_sources (snapshot_id, path, hash, size, content, language, input_blob, dirty, syntax_cache, file_cache)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, snap.Id, src.Path, src.Hash, src.Size, content, language, blob, dirty, cache, fileCache).Exec(ctx); err != nil {
			return fmt.Errorf("insert source %s: %w", src.Path, err)
		}
	}
	return nil
}

func saveFacts(ctx context.Context, tx bun.Tx, snap *pb.Snapshot, g *graph.Graph) error {
	if g == nil {
		return nil
	}
	for _, f := range sortedFacts(g) {
		if g.Reused[f.Id] {
			continue
		}
		anchor, _ := marshalJSON(f.Anchor)
		evidence, _ := marshalJSON(f.Evidence)
		imports, _ := marshalJSON(f.Imports)
		path := ""
		if f.Anchor != nil {
			path = f.Anchor.Path
		}
		if _, err := tx.NewRaw(`INSERT OR IGNORE INTO codeindex_facts
			(id, repository_id, snapshot_id, language, kind, name, qualified_name, symbol_key, signature, documentation, code, parent_fact_id, logical_key, path, anchor_json, evidence_json, imports_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			f.Id, f.RepositoryId, f.SnapshotId, f.Language, int(f.Kind), f.Name, f.QualifiedName, f.SymbolKey,
			f.Signature, f.Documentation, f.Code, f.ParentFactId, f.LogicalKey, path, anchor, evidence, imports).Exec(ctx); err != nil {
			return fmt.Errorf("insert fact %s: %w", f.Id, err)
		}
	}
	return nil
}

func saveChunks(ctx context.Context, tx bun.Tx, g *graph.Graph) error {
	if g == nil {
		return nil
	}
	for _, c := range sortedChunks(g) {
		if g.Reused[c.Id] {
			continue
		}
		anchor, _ := marshalJSON(c.Anchor)
		if _, err := tx.NewRaw(`INSERT OR IGNORE INTO codeindex_chunks
			(id, fact_id, snapshot_id, anchor_json, text, context, idx, total)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			c.Id, c.FactId, c.SnapshotId, anchor, c.Text, c.Context, c.Index, c.Total).Exec(ctx); err != nil {
			return fmt.Errorf("insert chunk %s: %w", c.Id, err)
		}
	}
	return nil
}

func saveEdges(ctx context.Context, tx bun.Tx, g *graph.Graph) error {
	if g == nil {
		return nil
	}
	for _, e := range sortedEdges(g) {
		if g.Reused[e.Id] {
			continue
		}
		anchor, _ := marshalJSON(e.Anchor)
		evidence, _ := marshalJSON(e.Evidence)
		if _, err := tx.NewRaw(`INSERT OR IGNORE INTO codeindex_edges
			(id, repository_id, snapshot_id, kind, from_fact_id, to_fact_id, target_symbol_key, logical_key, weight, anchor_json, evidence_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			e.Id, e.RepositoryId, e.SnapshotId, int(e.Kind), e.FromFactId, e.ToFactId, e.TargetSymbolKey,
			e.LogicalKey, e.Weight, anchor, evidence).Exec(ctx); err != nil {
			return fmt.Errorf("insert edge %s: %w", e.Id, err)
		}
	}
	return nil
}

func sortedFacts(g *graph.Graph) []*pb.CodeFact {
	out := make([]*pb.CodeFact, 0, len(g.Facts))
	for _, f := range g.Facts {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Id < out[j].Id })
	return out
}

func sortedChunks(g *graph.Graph) []*pb.Chunk {
	out := make([]*pb.Chunk, 0, len(g.Chunks))
	for _, c := range g.Chunks {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Id < out[j].Id })
	return out
}

func sortedEdges(g *graph.Graph) []*pb.EdgeFact {
	out := make([]*pb.EdgeFact, 0, len(g.EdgeFacts))
	for _, e := range g.EdgeFacts {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Id < out[j].Id })
	return out
}

// Snapshot loads a published snapshot and its source manifest.
func (s *Store) Snapshot(ctx context.Context, id string) (*pb.Snapshot, error) {
	var (
		snap                                         pb.Snapshot
		projects, warnings, tools                    string
		createdUnix                                  int64
		gitRevision, gitBranch                       string
		ingestion, embedding, configHash, repository string
	)
	err := s.bun.NewRaw(`SELECT repository_id, created_unix, git_revision, git_branch, ingestion_status, embedding_status, config_hash, projects_json, warnings_json, tool_versions_json, provenance, content_fingerprint
		FROM codeindex_snapshots WHERE id = ?`, id).
		Scan(ctx, &repository, &createdUnix, &gitRevision, &gitBranch, &ingestion, &embedding, &configHash, &projects, &warnings, &tools, &snap.Provenance, &snap.ContentFingerprint)
	if err != nil {
		return nil, err
	}
	snap.Id = id
	snap.RepositoryId = repository
	snap.CreatedUnix = createdUnix
	snap.GitRevision = gitRevision
	snap.GitBranch = gitBranch
	snap.IngestionStatus = ingestion
	snap.EmbeddingStatus = embedding
	snap.ConfigHash = configHash
	_ = json.Unmarshal([]byte(projects), &snap.Projects)
	_ = json.Unmarshal([]byte(warnings), &snap.Warnings)
	_ = json.Unmarshal([]byte(tools), &snap.ToolVersions)
	rows, err := s.bun.QueryContext(ctx, `SELECT path, hash, size FROM codeindex_sources WHERE snapshot_id = ? ORDER BY path`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var path, hash string
		var size int64
		if err := rows.Scan(&path, &hash, &size); err != nil {
			return nil, err
		}
		snap.Sources = append(snap.Sources, &pb.SourceFile{Path: path, Hash: hash, Size: uint64(size)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	snap.Statistics = &pb.SnapshotStatistics{}
	if err := s.bun.NewRaw(`SELECT
		(SELECT COUNT(*) FROM codeindex_snapshot_facts WHERE snapshot_id = ?),
		(SELECT COUNT(*) FROM codeindex_snapshot_edges WHERE snapshot_id = ?),
		(SELECT COUNT(*) FROM codeindex_sources WHERE snapshot_id = ?),
		(SELECT COUNT(*) FROM codeindex_snapshot_chunks WHERE snapshot_id = ?)`, id, id, id, id).
		Scan(ctx, &snap.Statistics.Facts, &snap.Statistics.Edges, &snap.Statistics.Sources, &snap.Statistics.Chunks); err != nil {
		return nil, fmt.Errorf("load snapshot statistics: %w", err)
	}
	return &snap, nil
}

// Snapshots lists a repository's snapshots oldest first. It loads every
// snapshot's metadata, toolchain configuration, and record statistics in a
// single query, but not its per-source manifest; call Snapshot for the full
// record.
func (s *Store) Snapshots(ctx context.Context, repositoryID string) ([]*pb.Snapshot, error) {
	rows, err := s.bun.QueryContext(ctx, `SELECT
		id, repository_id, created_unix, git_revision, git_branch,
		ingestion_status, embedding_status, config_hash,
		projects_json, warnings_json, tool_versions_json, provenance, content_fingerprint,
		(SELECT COUNT(*) FROM codeindex_snapshot_facts  WHERE snapshot_id = codeindex_snapshots.id),
		(SELECT COUNT(*) FROM codeindex_snapshot_edges  WHERE snapshot_id = codeindex_snapshots.id),
		(SELECT COUNT(*) FROM codeindex_sources         WHERE snapshot_id = codeindex_snapshots.id),
		(SELECT COUNT(*) FROM codeindex_snapshot_chunks WHERE snapshot_id = codeindex_snapshots.id)
		FROM codeindex_snapshots
		WHERE repository_id = ?
		ORDER BY created_unix, capture_order, id`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]*pb.Snapshot, 0)
	for rows.Next() {
		var (
			snap                      pb.Snapshot
			projects, warnings, tools string
			createdUnix               int64
		)
		snap.Statistics = &pb.SnapshotStatistics{}
		if err := rows.Scan(
			&snap.Id, &snap.RepositoryId, &createdUnix, &snap.GitRevision, &snap.GitBranch,
			&snap.IngestionStatus, &snap.EmbeddingStatus, &snap.ConfigHash,
			&projects, &warnings, &tools, &snap.Provenance, &snap.ContentFingerprint,
			&snap.Statistics.Facts, &snap.Statistics.Edges, &snap.Statistics.Sources, &snap.Statistics.Chunks,
		); err != nil {
			return nil, err
		}
		snap.CreatedUnix = createdUnix
		_ = json.Unmarshal([]byte(projects), &snap.Projects)
		_ = json.Unmarshal([]byte(warnings), &snap.Warnings)
		_ = json.Unmarshal([]byte(tools), &snap.ToolVersions)
		out = append(out, &snap)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Latest returns a repository's latest snapshot id, or "" when none exists.
func (s *Store) Latest(ctx context.Context, repositoryID string) (string, error) {
	var latest string
	err := s.bun.NewRaw(`SELECT latest_snapshot_id FROM codeindex_repositories WHERE id = ?`, repositoryID).Scan(ctx, &latest)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return latest, nil
}

// Repository loads repository metadata.
func (s *Store) Repository(ctx context.Context, id string) (*pb.Repository, error) {
	var root, latest string
	if err := s.bun.NewRaw(`SELECT root, latest_snapshot_id FROM codeindex_repositories WHERE id = ?`, id).Scan(ctx, &root, &latest); err != nil {
		return nil, err
	}
	return &pb.Repository{Id: id, Root: root, LatestSnapshotId: latest}, nil
}

// Source returns captured file content by content hash.
func (s *Store) Source(ctx context.Context, hash string) ([]byte, error) {
	var content []byte
	err := s.bun.NewRaw(`SELECT content FROM codeindex_sources WHERE hash = ? LIMIT 1`, hash).Scan(ctx, &content)
	if err != nil {
		return nil, err
	}
	return content, nil
}

// Fact loads a single fact.
func (s *Store) Fact(ctx context.Context, id string) (*pb.CodeFact, error) {
	facts, err := s.scanFacts(ctx, `SELECT `+factColumns+` FROM codeindex_facts WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(facts) == 0 {
		return nil, notFound("fact", id)
	}
	return facts[0], nil
}

// Facts lists facts for a snapshot via membership, optionally filtered by kind
// and path prefix.
func (s *Store) Facts(ctx context.Context, snapshotID string, kind pb.FactKind, pathPrefix, after string, limit int) ([]*pb.CodeFact, error) {
	query := `SELECT ` + factColumnsQualified + ` FROM codeindex_facts f JOIN codeindex_snapshot_facts m ON m.fact_id = f.id WHERE m.snapshot_id = ?`
	args := []any{snapshotID}
	if kind != pb.FactKind_FACT_KIND_UNSPECIFIED {
		query += ` AND f.kind = ?`
		args = append(args, int(kind))
	}
	if pathPrefix != "" {
		query += ` AND f.path LIKE ?`
		args = append(args, escapeLike(pathPrefix)+"%")
	}
	if after != "" {
		query += ` AND f.id > ?`
		args = append(args, after)
	}
	query += ` ORDER BY f.id LIMIT ?`
	args = append(args, clampLimit(limit))
	facts, err := s.scanFacts(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	for _, f := range facts {
		f.SnapshotId = snapshotID
	}
	return facts, nil
}

// FactVersions returns every snapshot's version of a logical fact, newest first.
func (s *Store) FactVersions(ctx context.Context, logicalKey string) ([]*pb.CodeFact, error) {
	query := `SELECT ` + factColumns + ` FROM codeindex_facts WHERE logical_key = ? ORDER BY snapshot_id DESC`
	return s.scanFacts(ctx, query, logicalKey)
}

// EdgeFact loads a single edge.
func (s *Store) EdgeFact(ctx context.Context, id string) (*pb.EdgeFact, error) {
	edges, err := s.scanEdges(ctx, `SELECT `+edgeColumns+` FROM codeindex_edges WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(edges) == 0 {
		return nil, notFound("edge fact", id)
	}
	return edges[0], nil
}

// EdgeFacts lists edges for a snapshot via membership, optionally filtered by
// kind and logical key.
func (s *Store) EdgeFacts(ctx context.Context, snapshotID string, kind pb.EdgeKind, logicalKey, after string, limit int) ([]*pb.EdgeFact, error) {
	query := `SELECT ` + edgeColumnsQualified + ` FROM codeindex_edges e JOIN codeindex_snapshot_edges m ON m.edge_id = e.id WHERE m.snapshot_id = ?`
	args := []any{snapshotID}
	if kind != pb.EdgeKind_EDGE_KIND_UNSPECIFIED {
		query += ` AND e.kind = ?`
		args = append(args, int(kind))
	}
	if logicalKey != "" {
		query += ` AND e.logical_key = ?`
		args = append(args, logicalKey)
	}
	if after != "" {
		query += ` AND e.id > ?`
		args = append(args, after)
	}
	query += ` ORDER BY e.id LIMIT ?`
	args = append(args, clampLimit(limit))
	edges, err := s.scanEdges(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	for _, e := range edges {
		e.SnapshotId = snapshotID
	}
	return edges, nil
}

// EdgeVersions returns every snapshot's observation of a logical edge.
func (s *Store) EdgeVersions(ctx context.Context, logicalKey string) ([]*pb.EdgeFact, error) {
	return s.scanEdges(ctx, `SELECT `+edgeColumns+` FROM codeindex_edges WHERE logical_key = ? ORDER BY snapshot_id DESC`, logicalKey)
}

// Chunk loads a single chunk.
func (s *Store) Chunk(ctx context.Context, id string) (*pb.Chunk, error) {
	chunks, err := s.scanChunks(ctx, `SELECT `+chunkColumns+` FROM codeindex_chunks WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, notFound("chunk", id)
	}
	return chunks[0], nil
}

// Chunks lists a snapshot's chunks via membership.
func (s *Store) Chunks(ctx context.Context, snapshotID string) ([]*pb.Chunk, error) {
	chunks, err := s.scanChunks(ctx, `SELECT `+chunkColumnsQualified+` FROM codeindex_chunks c JOIN codeindex_snapshot_chunks m ON m.chunk_id = c.id WHERE m.snapshot_id = ? ORDER BY c.id`, snapshotID)
	if err != nil {
		return nil, err
	}
	for _, c := range chunks {
		c.SnapshotId = snapshotID
	}
	return chunks, nil
}

// UpdateSnapshot updates mutable status fields on an existing snapshot.
func (s *Store) UpdateSnapshot(ctx context.Context, snap *pb.Snapshot) error {
	warnings, _ := marshalJSON(snap.Warnings)
	res, err := s.bun.NewRaw(`UPDATE codeindex_snapshots SET embedding_status = ?, warnings_json = ? WHERE id = ?`,
		snap.EmbeddingStatus, warnings, snap.Id).Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return notFound("snapshot", snap.Id)
	}
	return nil
}

func (s *Store) GetCachedEmbedding(ctx context.Context, key string) ([]float32, error) {
	var blob []byte
	err := s.bun.NewRaw(`SELECT vector FROM codeindex_embedding_cache WHERE key = ?`, key).Scan(ctx, &blob)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeVector(blob), nil
}

func (s *Store) CacheEmbedding(ctx context.Context, key string, vector []float32) error {
	_, err := s.bun.NewRaw(`INSERT INTO codeindex_embedding_cache (key, profile, input_hash, dimensions, vector)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET dimensions = excluded.dimensions, vector = excluded.vector`,
		key, "", "", len(vector), encodeVector(vector)).Exec(ctx)
	return err
}

func (s *Store) SaveEmbedding(ctx context.Context, e *pb.Embedding) error {
	if e == nil {
		return fmt.Errorf("save embedding: nil embedding")
	}
	id := e.Id
	if id == "" {
		id = graph.ID(e.ChunkId, e.Profile)
	}
	_, err := s.bun.NewRaw(`INSERT INTO codeindex_embeddings (id, chunk_id, fact_id, snapshot_id, profile, model, dimensions, input_hash, vector)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET vector = excluded.vector, dimensions = excluded.dimensions, snapshot_id = excluded.snapshot_id
		WHERE codeindex_embeddings.input_hash <> excluded.input_hash`,
		id, e.ChunkId, e.FactId, e.SnapshotId, e.Profile, e.Model, e.Dimensions, e.InputHash, encodeVector(e.Vector)).Exec(ctx)
	return err
}

func (s *Store) SaveFactEmbedding(ctx context.Context, e *pb.Embedding) error {
	if e == nil {
		return fmt.Errorf("save fact embedding: nil embedding")
	}
	id := e.Id
	if id == "" {
		id = graph.ID("fact-embedding", e.FactId, e.Profile)
	}
	// A reused fact's embedding is content-identical, so skip the vector rewrite
	// and ANN update to keep incremental publishes cheap.
	var existing string
	if err := s.bun.NewRaw(`SELECT input_hash FROM codeindex_fact_embeddings WHERE id = ?`, id).Scan(ctx, &existing); err == nil && existing == e.InputHash {
		return nil
	}
	if _, err := s.bun.NewRaw(`INSERT INTO codeindex_fact_embeddings (id, fact_id, snapshot_id, profile, model, dimensions, input_hash, vector)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET vector = excluded.vector, dimensions = excluded.dimensions, snapshot_id = excluded.snapshot_id`,
		id, e.FactId, e.SnapshotId, e.Profile, e.Model, e.Dimensions, e.InputHash, encodeVector(e.Vector)).Exec(ctx); err != nil {
		return err
	}
	return s.indexFactVector(ctx, id, e)
}

func escapeLike(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case '%', '_', '\\':
			out = append(out, '\\', r)
		default:
			out = append(out, r)
		}
	}
	return string(out)
}
