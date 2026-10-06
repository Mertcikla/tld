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
// repeat publish replaces snapshot-scoped rows while preserving shared immutable
// entities (idempotent retry).
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
		res, err := tx.NewRaw(`INSERT INTO codeindex_repositories (id, root, latest_snapshot_id, created_at, updated_at, org_id)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(org_id, id) DO UPDATE SET root = excluded.root, latest_snapshot_id = CASE WHEN ? OR codeindex_repositories.latest_snapshot_id = '' THEN excluded.latest_snapshot_id ELSE codeindex_repositories.latest_snapshot_id END, updated_at = excluded.updated_at`,
			repoID, root, snap.Id, now, now, scope(ctx).value(), advanceLatest).Exec(ctx)
		if err != nil {
			return fmt.Errorf("upsert repository: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return fmt.Errorf("upsert repository: %w", sql.ErrNoRows)
		}
		for _, table := range []string{"codeindex_project_artifacts", "codeindex_sources", "codeindex_snapshot_facts", "codeindex_snapshot_edges"} {
			where, scopeArgs := scope(ctx).clause("org_id")
			if _, err := tx.NewRaw("DELETE FROM "+table+" WHERE snapshot_id = ?"+where, append([]any{snap.Id}, scopeArgs...)...).Exec(ctx); err != nil {
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
	org := scope(ctx).value()
	for id := range g.Facts {
		if _, err := tx.NewRaw(`INSERT INTO codeindex_snapshot_facts (snapshot_id, fact_id, org_id) VALUES (?, ?, ?) ON CONFLICT(org_id, snapshot_id, fact_id) DO NOTHING`, snap.Id, id, org).Exec(ctx); err != nil {
			return fmt.Errorf("membership fact %s: %w", id, err)
		}
	}
	for id := range g.EdgeFacts {
		if _, err := tx.NewRaw(`INSERT INTO codeindex_snapshot_edges (snapshot_id, edge_id, org_id) VALUES (?, ?, ?) ON CONFLICT(org_id, snapshot_id, edge_id) DO NOTHING`, snap.Id, id, org).Exec(ctx); err != nil {
			return fmt.Errorf("membership edge %s: %w", id, err)
		}
	}
	return nil
}

func saveSnapshotRow(ctx context.Context, tx bun.Tx, snap *pb.Snapshot) error {
	projects, _ := marshalJSON(snap.Projects)
	warnings, _ := marshalJSON(snap.Warnings)
	tools, _ := marshalJSON(snap.ToolVersions)
	res, err := tx.NewRaw(`INSERT INTO codeindex_snapshots
		(id, repository_id, created_unix, git_revision, git_branch, ingestion_status, config_hash, projects_json, warnings_json, tool_versions_json, provenance, content_fingerprint, commit_message, capture_order, org_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(org_id, id) DO UPDATE SET
			repository_id = excluded.repository_id,
			git_revision = excluded.git_revision,
			git_branch = excluded.git_branch,
			ingestion_status = excluded.ingestion_status,
			config_hash = excluded.config_hash,
			projects_json = excluded.projects_json,
			warnings_json = excluded.warnings_json,
			tool_versions_json = excluded.tool_versions_json,
        provenance = excluded.provenance, content_fingerprint = excluded.content_fingerprint,
        commit_message = excluded.commit_message`,
		snap.Id, snap.RepositoryId, snap.CreatedUnix, snap.GitRevision, snap.GitBranch,
		snap.IngestionStatus, snap.ConfigHash, projects, warnings, tools, snap.Provenance, snap.ContentFingerprint, snap.CommitMessage, time.Now().UnixNano(), scope(ctx).value()).Exec(ctx)
	if err != nil {
		return fmt.Errorf("upsert snapshot: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("upsert snapshot: %w", sql.ErrNoRows)
	}
	return nil
}

func saveSources(ctx context.Context, tx bun.Tx, snap *pb.Snapshot, g *graph.Graph) error {
	org := scope(ctx).value()
	if g != nil {
		for key, artifact := range g.ProjectArtifacts {
			if _, err := tx.NewRaw(`INSERT INTO codeindex_project_artifacts (snapshot_id, project_key, fingerprint, data, org_id) VALUES (?, ?, ?, ?, ?)`, snap.Id, key, artifact.Fingerprint, artifact.Data, org).Exec(ctx); err != nil {
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
		if _, err := tx.NewRaw(`INSERT INTO codeindex_sources (snapshot_id, path, hash, size, content, language, input_blob, dirty, syntax_cache, file_cache, org_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, snap.Id, src.Path, src.Hash, src.Size, content, language, blob, dirty, cache, fileCache, org).Exec(ctx); err != nil {
			return fmt.Errorf("insert source %s: %w", src.Path, err)
		}
	}
	return nil
}

func saveFacts(ctx context.Context, tx bun.Tx, snap *pb.Snapshot, g *graph.Graph) error {
	if g == nil {
		return nil
	}
	org := scope(ctx).value()
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
		if _, err := tx.NewRaw(`INSERT INTO codeindex_facts
			(id, repository_id, snapshot_id, language, kind, name, qualified_name, symbol_key, signature, documentation, body_hash, parent_fact_id, logical_key, path, anchor_json, evidence_json, imports_json, org_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(org_id, id) DO NOTHING`,
			f.Id, f.RepositoryId, f.SnapshotId, f.Language, int(f.Kind), f.Name, f.QualifiedName, f.SymbolKey,
			f.Signature, f.Documentation, f.BodyHash, f.ParentFactId, f.LogicalKey, path, anchor, evidence, imports, org).Exec(ctx); err != nil {
			return fmt.Errorf("insert fact %s: %w", f.Id, err)
		}
	}
	return nil
}

func saveEdges(ctx context.Context, tx bun.Tx, g *graph.Graph) error {
	if g == nil {
		return nil
	}
	org := scope(ctx).value()
	for _, e := range sortedEdges(g) {
		if g.Reused[e.Id] {
			continue
		}
		anchor, _ := marshalJSON(e.Anchor)
		evidence, _ := marshalJSON(e.Evidence)
		if _, err := tx.NewRaw(`INSERT INTO codeindex_edges
			(id, repository_id, snapshot_id, kind, from_fact_id, to_fact_id, target_symbol_key, logical_key, weight, anchor_json, evidence_json, org_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(org_id, id) DO NOTHING`,
			e.Id, e.RepositoryId, e.SnapshotId, int(e.Kind), e.FromFactId, e.ToFactId, e.TargetSymbolKey,
			e.LogicalKey, e.Weight, anchor, evidence, org).Exec(ctx); err != nil {
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
		snap                              pb.Snapshot
		projects, warnings, tools         string
		createdUnix                       int64
		gitRevision, gitBranch            string
		ingestion, configHash, repository string
	)
	where, scopeArgs := scope(ctx).clause("org_id")
	err := s.bun.NewRaw(`SELECT repository_id, created_unix, git_revision, git_branch, ingestion_status, config_hash, projects_json, warnings_json, tool_versions_json, provenance, content_fingerprint, commit_message
		FROM codeindex_snapshots WHERE id = ?`+where, append([]any{id}, scopeArgs...)...).
		Scan(ctx, &repository, &createdUnix, &gitRevision, &gitBranch, &ingestion, &configHash, &projects, &warnings, &tools, &snap.Provenance, &snap.ContentFingerprint, &snap.CommitMessage)
	if err != nil {
		return nil, err
	}
	snap.Id = id
	snap.RepositoryId = repository
	snap.CreatedUnix = createdUnix
	snap.GitRevision = gitRevision
	snap.GitBranch = gitBranch
	snap.IngestionStatus = ingestion
	snap.ConfigHash = configHash
	_ = json.Unmarshal([]byte(projects), &snap.Projects)
	_ = json.Unmarshal([]byte(warnings), &snap.Warnings)
	_ = json.Unmarshal([]byte(tools), &snap.ToolVersions)
	sourceWhere, sourceArgs := scope(ctx).clause("org_id")
	rows, err := s.bun.QueryContext(ctx, `SELECT path, hash, size FROM codeindex_sources WHERE snapshot_id = ?`+sourceWhere+` ORDER BY path`, append([]any{id}, sourceArgs...)...)
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
	statQuery := `SELECT
		(SELECT COUNT(*) FROM codeindex_snapshot_facts WHERE snapshot_id = ?),
		(SELECT COUNT(*) FROM codeindex_snapshot_edges WHERE snapshot_id = ?),
		(SELECT COUNT(*) FROM codeindex_sources WHERE snapshot_id = ?)`
	statArgs := []any{id, id, id}
	if statsWhere, statsScopeArgs := scope(ctx).clause("org_id"); statsWhere != "" {
		statQuery = `SELECT
			(SELECT COUNT(*) FROM codeindex_snapshot_facts WHERE snapshot_id = ?` + statsWhere + `),
			(SELECT COUNT(*) FROM codeindex_snapshot_edges WHERE snapshot_id = ?` + statsWhere + `),
			(SELECT COUNT(*) FROM codeindex_sources WHERE snapshot_id = ?` + statsWhere + `)`
		statArgs = []any{}
		for i := 0; i < 3; i++ {
			statArgs = append(statArgs, id)
			statArgs = append(statArgs, statsScopeArgs...)
		}
	}
	if err := s.bun.NewRaw(statQuery, statArgs...).
		Scan(ctx, &snap.Statistics.Facts, &snap.Statistics.Edges, &snap.Statistics.Sources); err != nil {
		return nil, fmt.Errorf("load snapshot statistics: %w", err)
	}
	return &snap, nil
}

// Snapshots lists a repository's snapshots oldest first. It loads every
// snapshot's metadata, toolchain configuration, and record statistics in a
// single query, but not its per-source manifest; call Snapshot for the full
// record.
func (s *Store) Snapshots(ctx context.Context, repositoryID string) ([]*pb.Snapshot, error) {
	where, scopeArgs := scope(ctx).clause("org_id")
	rows, err := s.bun.QueryContext(ctx, `SELECT
		id, repository_id, created_unix, git_revision, git_branch,
		ingestion_status, config_hash,
		projects_json, warnings_json, tool_versions_json, provenance, content_fingerprint, commit_message,
		(SELECT COUNT(*) FROM codeindex_snapshot_facts  WHERE snapshot_id = codeindex_snapshots.id AND org_id = codeindex_snapshots.org_id),
		(SELECT COUNT(*) FROM codeindex_snapshot_edges  WHERE snapshot_id = codeindex_snapshots.id AND org_id = codeindex_snapshots.org_id),
		(SELECT COUNT(*) FROM codeindex_sources         WHERE snapshot_id = codeindex_snapshots.id AND org_id = codeindex_snapshots.org_id)
		FROM codeindex_snapshots
		WHERE repository_id = ?`+where+`
		ORDER BY created_unix, capture_order, id`, append([]any{repositoryID}, scopeArgs...)...)
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
			&snap.IngestionStatus, &snap.ConfigHash,
			&projects, &warnings, &tools, &snap.Provenance, &snap.ContentFingerprint, &snap.CommitMessage,
			&snap.Statistics.Facts, &snap.Statistics.Edges, &snap.Statistics.Sources,
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
	where, scopeArgs := scope(ctx).clause("org_id")
	var latest string
	err := s.bun.NewRaw(`SELECT latest_snapshot_id FROM codeindex_repositories WHERE id = ?`+where, append([]any{repositoryID}, scopeArgs...)...).Scan(ctx, &latest)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return latest, nil
}

// SetSnapshotProvenance rewrites one snapshot's provenance marker. It promotes
// the initial working-tree index captured when a repository is added into a
// durable saved point instead of the transient working_tree marker.
func (s *Store) SetSnapshotProvenance(ctx context.Context, id, provenance string) error {
	where, scopeArgs := scope(ctx).clause("org_id")
	res, err := s.bun.NewRaw(`UPDATE codeindex_snapshots SET provenance = ? WHERE id = ?`+where, append([]any{provenance, id}, scopeArgs...)...).Exec(ctx)
	if err != nil {
		return fmt.Errorf("set snapshot provenance: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Repository loads repository metadata.
func (s *Store) Repository(ctx context.Context, id string) (*pb.Repository, error) {
	where, scopeArgs := scope(ctx).clause("org_id")
	var root, latest string
	if err := s.bun.NewRaw(`SELECT root, latest_snapshot_id FROM codeindex_repositories WHERE id = ?`+where, append([]any{id}, scopeArgs...)...).Scan(ctx, &root, &latest); err != nil {
		return nil, err
	}
	return &pb.Repository{Id: id, Root: root, LatestSnapshotId: latest}, nil
}

// Source returns captured file content by content hash.
func (s *Store) Source(ctx context.Context, hash string) ([]byte, error) {
	where, scopeArgs := scope(ctx).clause("org_id")
	var content []byte
	err := s.bun.NewRaw(`SELECT content FROM codeindex_sources WHERE hash = ?`+where+` LIMIT 1`, append([]any{hash}, scopeArgs...)...).Scan(ctx, &content)
	if err != nil {
		return nil, err
	}
	return content, nil
}

// Fact loads a single fact.
func (s *Store) Fact(ctx context.Context, id string) (*pb.CodeFact, error) {
	where, scopeArgs := scope(ctx).clause("org_id")
	facts, err := s.scanFacts(ctx, `SELECT `+factColumns+` FROM codeindex_facts WHERE id = ?`+where, append([]any{id}, scopeArgs...)...)
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
	query := `SELECT ` + factColumnsQualified + ` FROM codeindex_facts f JOIN codeindex_snapshot_facts m ON m.fact_id = f.id AND m.org_id = f.org_id WHERE m.snapshot_id = ?`
	args := []any{snapshotID}
	if where, scopeArgs := scope(ctx).clause("f.org_id"); where != "" {
		query += where
		args = append(args, scopeArgs...)
	}
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
	where, scopeArgs := scope(ctx).clause("org_id")
	query := `SELECT ` + factColumns + ` FROM codeindex_facts WHERE logical_key = ?` + where + ` ORDER BY snapshot_id DESC`
	return s.scanFacts(ctx, query, append([]any{logicalKey}, scopeArgs...)...)
}

// EdgeFact loads a single edge.
func (s *Store) EdgeFact(ctx context.Context, id string) (*pb.EdgeFact, error) {
	where, scopeArgs := scope(ctx).clause("org_id")
	edges, err := s.scanEdges(ctx, `SELECT `+edgeColumns+` FROM codeindex_edges WHERE id = ?`+where, append([]any{id}, scopeArgs...)...)
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
	query := `SELECT ` + edgeColumnsQualified + ` FROM codeindex_edges e JOIN codeindex_snapshot_edges m ON m.edge_id = e.id AND m.org_id = e.org_id WHERE m.snapshot_id = ?`
	args := []any{snapshotID}
	if where, scopeArgs := scope(ctx).clause("e.org_id"); where != "" {
		query += where
		args = append(args, scopeArgs...)
	}
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
	where, scopeArgs := scope(ctx).clause("org_id")
	return s.scanEdges(ctx, `SELECT `+edgeColumns+` FROM codeindex_edges WHERE logical_key = ?`+where+` ORDER BY snapshot_id DESC`, append([]any{logicalKey}, scopeArgs...)...)
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
