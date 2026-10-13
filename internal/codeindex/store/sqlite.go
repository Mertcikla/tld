package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	"github.com/uptrace/bun"
)

// insertRows executes a multi-row INSERT for rows of cols values each, chunked
// so a statement stays well under the bind-parameter limits of both SQLite and
// Postgres. Row-by-row inserts made publishing a large snapshot dominate cold
// indexing, because every row cost a separate statement parse and execute.
func insertRows(ctx context.Context, tx bun.Tx, sqlPrefix, sqlSuffix string, cols int, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}
	const maxParams = 4000
	perChunk := maxParams / cols
	if perChunk < 1 {
		perChunk = 1
	}
	var b strings.Builder
	for start := 0; start < len(rows); start += perChunk {
		end := min(start+perChunk, len(rows))
		b.Reset()
		b.WriteString(sqlPrefix)
		args := make([]any, 0, (end-start)*cols)
		for i := start; i < end; i++ {
			if i > start {
				b.WriteString(",")
			}
			b.WriteString("(")
			for col := 0; col < cols; col++ {
				if col > 0 {
					b.WriteString(",")
				}
				b.WriteString("?")
			}
			b.WriteString(")")
			args = append(args, rows[i]...)
		}
		b.WriteString(sqlSuffix)
		if _, err := tx.NewRaw(b.String(), args...).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

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
	factRows := make([][]any, 0, len(g.Facts))
	for id := range g.Facts {
		factRows = append(factRows, []any{snap.Id, id, org})
	}
	if err := insertRows(ctx, tx, `INSERT INTO codeindex_snapshot_facts (snapshot_id, fact_id, org_id) VALUES `, ` ON CONFLICT(org_id, snapshot_id, fact_id) DO NOTHING`, 3, factRows); err != nil {
		return fmt.Errorf("membership facts: %w", err)
	}
	edgeRows := make([][]any, 0, len(g.EdgeFacts))
	for id := range g.EdgeFacts {
		edgeRows = append(edgeRows, []any{snap.Id, id, org})
	}
	if err := insertRows(ctx, tx, `INSERT INTO codeindex_snapshot_edges (snapshot_id, edge_id, org_id) VALUES `, ` ON CONFLICT(org_id, snapshot_id, edge_id) DO NOTHING`, 3, edgeRows); err != nil {
		return fmt.Errorf("membership edges: %w", err)
	}
	return nil
}

func saveSnapshotRow(ctx context.Context, tx bun.Tx, snap *pb.Snapshot) error {
	projects, _ := marshalJSON(snap.Projects)
	warnings, _ := marshalJSON(snap.Warnings)
	tools, _ := marshalJSON(snap.ToolVersions)
	manifest, err := marshalJSON(snap.Manifest)
	if err != nil {
		return err
	}
	res, err := tx.NewRaw(`INSERT INTO codeindex_snapshots
		(id, repository_id, created_unix, git_revision, git_branch, ingestion_status, config_hash, projects_json, warnings_json, tool_versions_json, provenance, content_fingerprint, commit_message, capture_order, org_id, manifest_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
        commit_message = excluded.commit_message, manifest_json = excluded.manifest_json`,
		snap.Id, snap.RepositoryId, snap.CreatedUnix, snap.GitRevision, snap.GitBranch,
		snap.IngestionStatus, snap.ConfigHash, projects, warnings, tools, snap.Provenance, snap.ContentFingerprint, snap.CommitMessage, time.Now().UnixNano(), scope(ctx).value(), manifest).Exec(ctx)
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

// saveSources writes a snapshot's source list and its project artifacts. The
// heavy payloads (file text, syntax caches, SCIP indexes) live in content-
// addressed tables keyed by hash or fingerprint; a publish only binds bytes
// that no earlier snapshot already stored.
func saveSources(ctx context.Context, tx bun.Tx, snap *pb.Snapshot, g *graph.Graph) error {
	org := scope(ctx).value()
	if g != nil && len(g.ProjectArtifacts) > 0 {
		artifactKeys := make([][2]string, 0, len(g.ProjectArtifacts))
		for key, artifact := range g.ProjectArtifacts {
			artifactKeys = append(artifactKeys, [2]string{key, artifact.Fingerprint})
		}
		existing, err := existingArtifactKeys(ctx, tx, org, artifactKeys)
		if err != nil {
			return err
		}
		artifactRows := make([][]any, 0, len(artifactKeys))
		snapshotRows := make([][]any, 0, len(artifactKeys))
		for _, key := range artifactKeys {
			if !existing[key] {
				artifactRows = append(artifactRows, []any{org, key[0], key[1], g.ProjectArtifacts[key[0]].Data})
			}
			snapshotRows = append(snapshotRows, []any{snap.Id, key[0], key[1], org})
		}
		if err := insertRows(ctx, tx, `INSERT INTO codeindex_artifacts (org_id, project_key, fingerprint, data) VALUES `, ` ON CONFLICT(org_id, project_key, fingerprint) DO NOTHING`, 4, artifactRows); err != nil {
			return err
		}
		if err := insertRows(ctx, tx, `INSERT INTO codeindex_project_artifacts (snapshot_id, project_key, fingerprint, org_id) VALUES `, ``, 4, snapshotRows); err != nil {
			return err
		}
	}

	type blobKey struct{ hash, language string }
	blobIndex := map[blobKey]int{}
	var blobRows [][]any
	sourceRows := make([][]any, 0, len(snap.Sources))
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
		if src.Hash != "" {
			key := blobKey{hash: src.Hash, language: language}
			if _, ok := blobIndex[key]; !ok {
				blobIndex[key] = len(blobRows)
				blobRows = append(blobRows, []any{org, src.Hash, language, content, cache, fileCache})
			}
		}
		sourceRows = append(sourceRows, []any{snap.Id, src.Path, src.Hash, src.Size, language, blob, dirty, org})
	}
	keys := make([][2]string, 0, len(blobRows))
	for _, row := range blobRows {
		keys = append(keys, [2]string{row[1].(string), row[2].(string)})
	}
	existing, err := existingBlobKeys(ctx, tx, org, keys)
	if err != nil {
		return err
	}
	missing := make([][]any, 0, len(blobRows))
	for i, row := range blobRows {
		if !existing[keys[i]] {
			missing = append(missing, row)
		}
	}
	if err := insertRows(ctx, tx, `INSERT INTO codeindex_blobs (org_id, hash, language, content, syntax_cache, file_cache) VALUES `, ` ON CONFLICT(org_id, hash, language) DO NOTHING`, 6, missing); err != nil {
		return err
	}
	if err := insertRows(ctx, tx, `INSERT INTO codeindex_sources (snapshot_id, path, hash, size, language, input_blob, dirty, org_id) VALUES `, ``, 8, sourceRows); err != nil {
		return fmt.Errorf("insert sources: %w", err)
	}
	return nil
}

// existingBlobKeys reports which (hash, language) pairs are already stored, so
// a publish binds only the bytes that are new. Only hash strings cross the
// wire; blob contents stay in the database.
func existingBlobKeys(ctx context.Context, tx bun.Tx, org any, keys [][2]string) (map[[2]string]bool, error) {
	existing := map[[2]string]bool{}
	const chunk = 500
	for start := 0; start < len(keys); start += chunk {
		end := min(start+chunk, len(keys))
		placeholders := make([]string, 0, end-start)
		args := make([]any, 0, 1+(end-start))
		args = append(args, org)
		for _, key := range keys[start:end] {
			placeholders = append(placeholders, "?")
			args = append(args, key[0])
		}
		rows, err := tx.QueryContext(ctx, `SELECT hash, language FROM codeindex_blobs WHERE org_id = ? AND hash IN (`+strings.Join(placeholders, ",")+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var hash, language string
			if err := rows.Scan(&hash, &language); err != nil {
				_ = rows.Close()
				return nil, err
			}
			existing[[2]string{hash, language}] = true
		}
		rowErr := rows.Err()
		_ = rows.Close()
		if rowErr != nil {
			return nil, rowErr
		}
	}
	return existing, nil
}

// existingArtifactKeys reports which (project_key, fingerprint) artifacts are
// already stored; an unchanged project's SCIP index is therefore written once
// no matter how many snapshots reuse it.
func existingArtifactKeys(ctx context.Context, tx bun.Tx, org any, keys [][2]string) (map[[2]string]bool, error) {
	existing := map[[2]string]bool{}
	const chunk = 500
	for start := 0; start < len(keys); start += chunk {
		end := min(start+chunk, len(keys))
		placeholders := make([]string, 0, end-start)
		args := make([]any, 0, 1+(end-start))
		args = append(args, org)
		for _, key := range keys[start:end] {
			placeholders = append(placeholders, "?")
			args = append(args, key[1])
		}
		rows, err := tx.QueryContext(ctx, `SELECT project_key, fingerprint FROM codeindex_artifacts WHERE org_id = ? AND fingerprint IN (`+strings.Join(placeholders, ",")+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var projectKey, fingerprint string
			if err := rows.Scan(&projectKey, &fingerprint); err != nil {
				_ = rows.Close()
				return nil, err
			}
			existing[[2]string{projectKey, fingerprint}] = true
		}
		rowErr := rows.Err()
		_ = rows.Close()
		if rowErr != nil {
			return nil, rowErr
		}
	}
	return existing, nil
}

func saveFacts(ctx context.Context, tx bun.Tx, snap *pb.Snapshot, g *graph.Graph) error {
	if g == nil {
		return nil
	}
	org := scope(ctx).value()
	var rows [][]any
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
		rows = append(rows, []any{
			f.Id, f.RepositoryId, f.SnapshotId, f.Language, int(f.Kind), f.Name, f.QualifiedName, f.SymbolKey,
			f.Signature, f.Documentation, f.BodyHash, f.ParentFactId, f.LogicalKey, path, anchor, evidence, imports, org,
		})
	}
	if err := insertRows(ctx, tx, `INSERT INTO codeindex_facts
		(id, repository_id, snapshot_id, language, kind, name, qualified_name, symbol_key, signature, documentation, body_hash, parent_fact_id, logical_key, path, anchor_json, evidence_json, imports_json, org_id) VALUES `,
		` ON CONFLICT(org_id, id) DO NOTHING`, 18, rows); err != nil {
		return fmt.Errorf("insert facts: %w", err)
	}
	return nil
}

func saveEdges(ctx context.Context, tx bun.Tx, g *graph.Graph) error {
	if g == nil {
		return nil
	}
	org := scope(ctx).value()
	var rows [][]any
	for _, e := range sortedEdges(g) {
		if g.Reused[e.Id] {
			continue
		}
		anchor, _ := marshalJSON(e.Anchor)
		evidence, _ := marshalJSON(e.Evidence)
		rows = append(rows, []any{
			e.Id, e.RepositoryId, e.SnapshotId, int(e.Kind), e.FromFactId, e.ToFactId, e.TargetSymbolKey,
			e.LogicalKey, e.Weight, anchor, evidence, org,
		})
	}
	if err := insertRows(ctx, tx, `INSERT INTO codeindex_edges
		(id, repository_id, snapshot_id, kind, from_fact_id, to_fact_id, target_symbol_key, logical_key, weight, anchor_json, evidence_json, org_id) VALUES `,
		` ON CONFLICT(org_id, id) DO NOTHING`, 12, rows); err != nil {
		return fmt.Errorf("insert edges: %w", err)
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
		snap                                pb.Snapshot
		projects, warnings, tools, manifest string
		createdUnix                         int64
		gitRevision, gitBranch              string
		ingestion, configHash, repository   string
	)
	where, scopeArgs := scope(ctx).clause("org_id")
	err := s.bun.NewRaw(`SELECT repository_id, created_unix, git_revision, git_branch, ingestion_status, config_hash, projects_json, warnings_json, tool_versions_json, provenance, content_fingerprint, commit_message, manifest_json
		FROM codeindex_snapshots WHERE id = ?`+where, append([]any{id}, scopeArgs...)...).
		Scan(ctx, &repository, &createdUnix, &gitRevision, &gitBranch, &ingestion, &configHash, &projects, &warnings, &tools, &snap.Provenance, &snap.ContentFingerprint, &snap.CommitMessage, &manifest)
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
	if err := json.Unmarshal([]byte(manifest), &snap.Manifest); err != nil {
		return nil, fmt.Errorf("load snapshot manifest: %w", err)
	}
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

// SnapshotRefs lists a repository's snapshots oldest first with only the
// metadata revision resolution needs. Unlike Snapshots it skips the per-record
// statistics counts, which dominate the listing on large repositories.
func (s *Store) SnapshotRefs(ctx context.Context, repositoryID string) ([]*pb.Snapshot, error) {
	where, scopeArgs := scope(ctx).clause("org_id")
	rows, err := s.bun.QueryContext(ctx, `SELECT
		id, repository_id, created_unix, git_revision, git_branch,
		ingestion_status, config_hash,
		projects_json, warnings_json, tool_versions_json, provenance, content_fingerprint, commit_message
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
		if err := rows.Scan(
			&snap.Id, &snap.RepositoryId, &createdUnix, &snap.GitRevision, &snap.GitBranch,
			&snap.IngestionStatus, &snap.ConfigHash,
			&projects, &warnings, &tools, &snap.Provenance, &snap.ContentFingerprint, &snap.CommitMessage,
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

// Source returns captured file content by content hash from the shared blob
// store, so every snapshot with the same file content reads one row.
func (s *Store) Source(ctx context.Context, hash string) ([]byte, error) {
	where, scopeArgs := scope(ctx).clause("org_id")
	var content []byte
	err := s.bun.NewRaw(`SELECT content FROM codeindex_blobs WHERE hash = ?`+where+` LIMIT 1`, append([]any{hash}, scopeArgs...)...).Scan(ctx, &content)
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
		query += ` AND f.path LIKE ? ESCAPE '!'`
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
	// Use an explicit escape character that does not depend on the dialect's
	// handling of backslashes in SQL string literals or LIKE patterns.
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case '%', '_', '!':
			out = append(out, '!', r)
		default:
			out = append(out, r)
		}
	}
	return string(out)
}
