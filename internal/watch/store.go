package watch

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"sort"
	"strings"
	"time"

	tldgit "github.com/mertcikla/tld/v2/internal/git"
	"github.com/mertcikla/tld/v2/internal/tagcolors"
	"github.com/viant/sqlite-vec/vector"
)

const LockHeartbeatTimeout = 30 * time.Second
const maxInClauseIDs = 500
const embeddingSimilarityTimeout = 2 * time.Second

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

type materializationState struct {
	ResourceID      int64
	LastWatchHash   *string
	Dirty           bool
	DirtyDetectedAt *string
}

type watchResourceSnapshot struct {
	OwnerType    string
	OwnerKey     string
	ResourceType string
	ResourceID   *int64
	Language     string
	Hash         string
	Summary      string
	LineCount    int
	FilePath     string
	StartLine    int
	EndLine      int
}

type changedRawResources struct {
	Files   map[string]struct{}
	Symbols map[int64]string
}

type FilterDecisionQuery struct {
	OwnerType string
	Decision  string
	Limit     int
	Offset    int
}

type watchMaterializationMapping struct {
	ID              int64
	OwnerType       string
	OwnerKey        string
	ResourceType    string
	ResourceID      int64
	LastWatchHash   *string
	Dirty           bool
	DirtyDetectedAt *string
	UpdatedAt       string
}

func (s *Store) Summary(ctx context.Context, repositoryID int64) (Summary, error) {
	summary := Summary{RepositoryID: repositoryID}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM watch_files WHERE repository_id = ?`, repositoryID).Scan(&summary.Files); err != nil {
		return Summary{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM watch_symbols WHERE repository_id = ?`, repositoryID).Scan(&summary.Symbols); err != nil {
		return Summary{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM watch_references WHERE repository_id = ?`, repositoryID).Scan(&summary.References); err != nil {
		return Summary{}, err
	}
	var finished sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT status, started_at, finished_at
		FROM watch_scan_runs
		WHERE repository_id = ?
		ORDER BY id DESC
		LIMIT 1`, repositoryID).Scan(&summary.LastScanStatus, &summary.LastScanStarted, &finished)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Summary{}, err
	}
	if finished.Valid {
		summary.LastScanFinished = finished.String
	}
	return summary, nil
}

func (s *Store) EnsureEmbeddingModel(ctx context.Context, cfg EmbeddingConfig, configHash string) (int64, error) {
	cfg = normalizeEmbeddingConfig(cfg)
	now := nowString()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO watch_embedding_models(provider, model, dimension, config_hash, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(provider, model, dimension, config_hash) DO NOTHING`,
		cfg.Provider, cfg.Model, cfg.Dimension, configHash, now)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `
		SELECT id FROM watch_embedding_models
		WHERE provider = ? AND model = ? AND dimension = ? AND config_hash = ?`,
		cfg.Provider, cfg.Model, cfg.Dimension, configHash).Scan(&id)
	return id, err
}

func (s *Store) Embedding(ctx context.Context, modelID int64, ownerType, ownerKey, inputHash string) ([]byte, bool, error) {
	var vector []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT vector FROM watch_embeddings
		WHERE model_id = ? AND owner_type = ? AND owner_key = ? AND input_hash = ?`,
		modelID, ownerType, ownerKey, inputHash).Scan(&vector)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return vector, err == nil, err
}

func (s *Store) SaveEmbedding(ctx context.Context, modelID int64, ownerType, ownerKey, inputHash string, vectorData []byte) error {
	if err := s.EnsureEmbeddingVectorSchema(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO watch_embeddings(model_id, owner_type, owner_key, input_hash, vector, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(model_id, owner_type, owner_key, input_hash) DO NOTHING`,
		modelID, ownerType, ownerKey, inputHash, vectorData, nowString())
	if err != nil {
		return err
	}
	var embeddingID int64
	if err := s.db.QueryRowContext(ctx, `
		SELECT id FROM watch_embeddings
		WHERE model_id = ? AND owner_type = ? AND owner_key = ? AND input_hash = ?`,
		modelID, ownerType, ownerKey, inputHash).Scan(&embeddingID); err != nil {
		return err
	}
	encoded, err := vector.EncodeEmbedding(bytesToVector(vectorData))
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO _vec_watch_embedding_vec(dataset_id, id, content, meta, embedding)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(dataset_id, id) DO UPDATE SET
			content = excluded.content,
			meta = excluded.meta,
			embedding = excluded.embedding`,
		embeddingDataset(modelID), fmt.Sprintf("%d", embeddingID), ownerKey, ownerType, encoded)
	return err
}

func (s *Store) SimilarEmbeddings(ctx context.Context, modelID int64, query Vector, limit int) ([]int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if err := s.EnsureEmbeddingVectorSchema(ctx); err != nil {
		return nil, err
	}
	ids, err := s.similarEmbeddingsSQLiteVec(ctx, modelID, query, limit)
	if err == nil {
		return ids, nil
	}
	return s.similarEmbeddingsFallback(ctx, modelID, query, limit)
}

func (s *Store) similarEmbeddingsSQLiteVec(ctx context.Context, modelID int64, query Vector, limit int) ([]int64, error) {
	encoded, err := vector.EncodeEmbedding(query)
	if err != nil {
		return nil, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, embeddingSimilarityTimeout)
	defer cancel()
	rows, err := s.db.QueryContext(queryCtx, `
		SELECT id
		FROM watch_embedding_vec
		WHERE dataset_id = ? AND id MATCH ?
		LIMIT ?`, embeddingDataset(modelID), encoded, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]int64, 0, limit)
	for rows.Next() {
		var rawID string
		if err := rows.Scan(&rawID); err != nil {
			return nil, err
		}
		var id int64
		if _, err := fmt.Sscan(rawID, &id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) similarEmbeddingsFallback(ctx context.Context, modelID int64, query Vector, limit int) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, vector FROM watch_embeddings WHERE model_id = ?`, modelID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	type scored struct {
		ID    int64
		Score float64
	}
	var scoredRows []scored
	for rows.Next() {
		var id int64
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		scoredRows = append(scoredRows, scored{ID: id, Score: CosineSimilarity(query, bytesToVector(data))})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(scoredRows, func(i, j int) bool { return scoredRows[i].Score > scoredRows[j].Score })
	if len(scoredRows) > limit {
		scoredRows = scoredRows[:limit]
	}
	out := make([]int64, 0, len(scoredRows))
	for _, row := range scoredRows {
		out = append(out, row.ID)
	}
	return out, nil
}

func (s *Store) EnsureEmbeddingVectorSchema(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS _vec_watch_embedding_vec (
			dataset_id TEXT NOT NULL,
			id TEXT NOT NULL,
			content TEXT,
			meta TEXT,
			embedding BLOB,
			PRIMARY KEY(dataset_id, id)
		)`); err != nil {
		return err
	}
	dbPath, err := sqliteMainDBPath(ctx, s.db)
	if err != nil {
		return err
	}
	createVirtualTable := `CREATE VIRTUAL TABLE IF NOT EXISTS watch_embedding_vec USING vec(id)`
	if dbPath != "" {
		createVirtualTable = fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS watch_embedding_vec USING vec(id, dbpath='%s')`, strings.ReplaceAll(dbPath, "'", "''"))
	}
	if _, err := s.db.ExecContext(ctx, createVirtualTable); err != nil {
		return err
	}
	return nil
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
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "", nil
}

func (s *Store) MappingState(ctx context.Context, repositoryID int64, ownerType, ownerKey, resourceType string) (materializationState, bool, error) {
	var state materializationState
	var lastHash sql.NullString
	var dirtyAt sql.NullString
	var dirty int
	err := s.db.QueryRowContext(ctx, `
		SELECT resource_id, last_watch_hash, dirty, dirty_detected_at FROM watch_materialization
		WHERE repository_id = ? AND owner_type = ? AND owner_key = ? AND resource_type = ?`,
		repositoryID, ownerType, ownerKey, resourceType).Scan(&state.ResourceID, &lastHash, &dirty, &dirtyAt)
	if errors.Is(err, sql.ErrNoRows) {
		return materializationState{}, false, nil
	}
	if err != nil {
		return materializationState{}, false, err
	}
	if lastHash.Valid {
		state.LastWatchHash = &lastHash.String
	}
	if dirtyAt.Valid {
		state.DirtyDetectedAt = &dirtyAt.String
	}
	state.Dirty = dirty != 0
	return state, true, nil
}

func (s *Store) SaveMapping(ctx context.Context, repositoryID int64, ownerType, ownerKey, resourceType string, resourceID int64) error {
	return s.SaveMappingAt(ctx, repositoryID, ownerType, ownerKey, resourceType, resourceID, nowString())
}

func (s *Store) SaveMappingAt(ctx context.Context, repositoryID int64, ownerType, ownerKey, resourceType string, resourceID int64, updatedAt string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO watch_materialization(repository_id, owner_type, owner_key, resource_type, resource_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repository_id, owner_type, owner_key, resource_type) DO UPDATE SET
			resource_id = excluded.resource_id,
			updated_at = excluded.updated_at`,
		repositoryID, ownerType, ownerKey, resourceType, resourceID, updatedAt, updatedAt)
	return err
}

func (s *Store) SaveMappingHashAt(ctx context.Context, repositoryID int64, ownerType, ownerKey, resourceType string, resourceID int64, resourceHash string, updatedAt string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO watch_materialization(repository_id, owner_type, owner_key, resource_type, resource_id, last_watch_hash, dirty, dirty_detected_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, NULL, ?, ?)
		ON CONFLICT(repository_id, owner_type, owner_key, resource_type) DO UPDATE SET
			resource_id = excluded.resource_id,
			last_watch_hash = excluded.last_watch_hash,
			dirty = 0,
			dirty_detected_at = NULL,
			updated_at = excluded.updated_at`,
		repositoryID, ownerType, ownerKey, resourceType, resourceID, resourceHash, updatedAt, updatedAt)
	return err
}

func (s *Store) MarkMappingDirty(ctx context.Context, repositoryID int64, ownerType, ownerKey, resourceType string, resourceID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE watch_materialization
		SET dirty = 1,
		    dirty_detected_at = COALESCE(dirty_detected_at, ?),
		    updated_at = ?
		WHERE repository_id = ? AND owner_type = ? AND owner_key = ? AND resource_type = ? AND resource_id = ?`,
		nowString(), nowString(), repositoryID, ownerType, ownerKey, resourceType, resourceID)
	return err
}

func (s *Store) Materialization(ctx context.Context, repositoryID int64) ([]MaterializationMapping, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, repository_id, owner_type, owner_key, resource_type, resource_id, last_watch_hash, dirty, dirty_detected_at, created_at, updated_at
		FROM watch_materialization
		WHERE repository_id = ?
		ORDER BY owner_type, owner_key, resource_type`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []MaterializationMapping
	for rows.Next() {
		var item MaterializationMapping
		var lastHash sql.NullString
		var dirtyAt sql.NullString
		var dirty int
		if err := rows.Scan(&item.ID, &item.RepositoryID, &item.OwnerType, &item.OwnerKey, &item.ResourceType, &item.ResourceID, &lastHash, &dirty, &dirtyAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if lastHash.Valid {
			item.LastWatchHash = &lastHash.String
		}
		item.Dirty = dirty != 0
		if dirtyAt.Valid {
			item.DirtyDetectedAt = &dirtyAt.String
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ReplaceArchitectureBindings(ctx context.Context, repositoryID int64, bindings []ArchitectureBinding) error {
	if err := s.ensureArchitectureLinksTable(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM watch_architecture_links WHERE repository_id = ?`, repositoryID); err != nil {
		return err
	}
	now := nowString()
	for _, binding := range bindings {
		evidence, _ := json.Marshal(binding.Evidence)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO watch_architecture_links(
				repository_id, component_key, target_repository_id, target_owner_type, target_owner_key,
				target_resource_type, target_resource_id, role, confidence, evidence_json, created_at, updated_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			repositoryID,
			binding.ComponentKey,
			binding.TargetRepositoryID,
			binding.TargetOwnerType,
			binding.TargetOwnerKey,
			binding.TargetResourceType,
			binding.TargetResourceID,
			binding.Role,
			binding.Confidence,
			string(evidence),
			now,
			now,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ArchitectureBindings(ctx context.Context, repositoryID int64) ([]ArchitectureBinding, error) {
	if err := s.ensureArchitectureLinksTable(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, repository_id, component_key, target_repository_id, target_owner_type, target_owner_key,
		       target_resource_type, target_resource_id, role, confidence, evidence_json, created_at, updated_at
		FROM watch_architecture_links
		WHERE repository_id = ?
		ORDER BY component_key, role, confidence DESC, target_owner_type, target_owner_key`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []ArchitectureBinding
	for rows.Next() {
		var item ArchitectureBinding
		var evidenceJSON string
		if err := rows.Scan(
			&item.ID,
			&item.RepositoryID,
			&item.ComponentKey,
			&item.TargetRepositoryID,
			&item.TargetOwnerType,
			&item.TargetOwnerKey,
			&item.TargetResourceType,
			&item.TargetResourceID,
			&item.Role,
			&item.Confidence,
			&evidenceJSON,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(evidenceJSON), &item.Evidence)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ArchitectureBindingTargets(ctx context.Context) ([]ArchitectureBindingTarget, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT wm.repository_id, wm.owner_type, wm.owner_key, wm.resource_type, wm.resource_id,
		       COALESCE(v.id, 0), e.name, COALESCE(e.kind, ''), COALESCE(e.file_path, ''),
		       COALESCE(e.language, ''), COALESCE(e.tags, '[]')
		FROM watch_materialization wm
		JOIN elements e ON e.id = wm.resource_id
		LEFT JOIN views v ON v.owner_element_id = e.id
		WHERE wm.resource_type = 'element'
		  AND wm.owner_type IN ('folder', 'file', 'symbol', 'cluster', 'fact', 'fact-summary')
		ORDER BY wm.repository_id, wm.owner_type, wm.owner_key`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []ArchitectureBindingTarget
	for rows.Next() {
		var item ArchitectureBindingTarget
		var tagsJSON string
		if err := rows.Scan(
			&item.RepositoryID,
			&item.OwnerType,
			&item.OwnerKey,
			&item.ResourceType,
			&item.ResourceID,
			&item.ViewID,
			&item.Name,
			&item.Kind,
			&item.FilePath,
			&item.Language,
			&tagsJSON,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tagsJSON), &item.Tags)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ensureArchitectureLinksTable(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS watch_architecture_links (
		  id INTEGER PRIMARY KEY AUTOINCREMENT,
		  repository_id INTEGER NOT NULL,
		  component_key TEXT NOT NULL,
		  target_repository_id INTEGER NOT NULL,
		  target_owner_type TEXT NOT NULL,
		  target_owner_key TEXT NOT NULL,
		  target_resource_type TEXT NOT NULL,
		  target_resource_id INTEGER NOT NULL,
		  role TEXT NOT NULL,
		  confidence REAL NOT NULL,
		  evidence_json TEXT NOT NULL DEFAULT '[]',
		  created_at TEXT NOT NULL,
		  updated_at TEXT NOT NULL,
		  UNIQUE(repository_id, component_key, target_repository_id, target_owner_type, target_owner_key, target_resource_type, role),
		  FOREIGN KEY (repository_id) REFERENCES watch_repositories(id) ON DELETE CASCADE,
		  FOREIGN KEY (target_repository_id) REFERENCES watch_repositories(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_watch_architecture_links_repository_id
		  ON watch_architecture_links(repository_id);
		CREATE INDEX IF NOT EXISTS idx_watch_architecture_links_target
		  ON watch_architecture_links(target_repository_id, target_owner_type, target_owner_key);
	`)
	return err
}

func (s *Store) staleMaterializationMappings(ctx context.Context, repositoryID int64, runMarker string) ([]watchMaterializationMapping, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, owner_type, owner_key, resource_type, resource_id, last_watch_hash, dirty, dirty_detected_at, updated_at
		FROM watch_materialization
		WHERE repository_id = ? AND updated_at != ?`, repositoryID, runMarker)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []watchMaterializationMapping
	for rows.Next() {
		var item watchMaterializationMapping
		var lastHash sql.NullString
		var dirtyAt sql.NullString
		var dirty int
		if err := rows.Scan(&item.ID, &item.OwnerType, &item.OwnerKey, &item.ResourceType, &item.ResourceID, &lastHash, &dirty, &dirtyAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if lastHash.Valid {
			item.LastWatchHash = &lastHash.String
		}
		item.Dirty = dirty != 0
		if dirtyAt.Valid {
			item.DirtyDetectedAt = &dirtyAt.String
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sortMaterializationMappingsForDelete(out)
	return out, nil
}

func (s *Store) allMaterializationMappings(ctx context.Context, repositoryID int64) ([]watchMaterializationMapping, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, owner_type, owner_key, resource_type, resource_id, last_watch_hash, dirty, dirty_detected_at, updated_at
		FROM watch_materialization
		WHERE repository_id = ?`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []watchMaterializationMapping
	for rows.Next() {
		var item watchMaterializationMapping
		var lastHash sql.NullString
		var dirtyAt sql.NullString
		var dirty int
		if err := rows.Scan(&item.ID, &item.OwnerType, &item.OwnerKey, &item.ResourceType, &item.ResourceID, &lastHash, &dirty, &dirtyAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if lastHash.Valid {
			item.LastWatchHash = &lastHash.String
		}
		item.Dirty = dirty != 0
		if dirtyAt.Valid {
			item.DirtyDetectedAt = &dirtyAt.String
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sortMaterializationMappingsForDelete(out)
	return out, nil
}

func (s *Store) deleteMaterializationMapping(ctx context.Context, mapping watchMaterializationMapping) error {
	var query string
	switch mapping.ResourceType {
	case "connector":
		query = `DELETE FROM connectors WHERE id = ?`
	case "view":
		query = `DELETE FROM views WHERE id = ?`
	case "element":
		query = `DELETE FROM elements WHERE id = ?`
	default:
		query = ""
	}
	if query != "" {
		if _, err := s.db.ExecContext(ctx, query, mapping.ResourceID); err != nil {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM watch_materialization WHERE id = ?`, mapping.ID)
	return err
}

func (s *Store) PruneStaleMaterializedResources(ctx context.Context, repositoryID int64, runMarker string) (int, error) {
	if runMarker == "" {
		return 0, nil
	}
	mappings, err := s.staleMaterializationMappings(ctx, repositoryID, runMarker)
	if err != nil {
		return 0, err
	}
	deletedElementIDs, err := s.deletedMaterializedElementIDs(ctx, repositoryID)
	if err != nil {
		return 0, err
	}
	preserved := 0
	for _, mapping := range mappings {
		tombstoned, err := s.materializationMappingTombstoned(ctx, repositoryID, mapping, deletedElementIDs)
		if err != nil {
			return preserved, err
		}
		if tombstoned {
			continue
		}
		dirty, err := s.mappingResourceDirty(ctx, repositoryID, mapping)
		if err != nil {
			return preserved, err
		}
		if dirty {
			preserved++
			continue
		}
		if err := s.deleteMaterializationMapping(ctx, mapping); err != nil {
			return preserved, err
		}
	}
	return preserved, nil
}

func (s *Store) PruneDeletedMaterializedResources(ctx context.Context, repositoryID int64) error {
	mappings, err := s.allMaterializationMappings(ctx, repositoryID)
	if err != nil {
		return err
	}
	deletedElementIDs, err := s.deletedMaterializedElementIDs(ctx, repositoryID)
	if err != nil {
		return err
	}
	for _, mapping := range mappings {
		tombstoned, err := s.materializationMappingTombstoned(ctx, repositoryID, mapping, deletedElementIDs)
		if err != nil {
			return err
		}
		if !tombstoned {
			continue
		}
		dirty, err := s.mappingResourceDirty(ctx, repositoryID, mapping)
		if err != nil {
			return err
		}
		if dirty {
			continue
		}
		if err := s.deleteMaterializationMapping(ctx, mapping); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) mappingResourceDirty(ctx context.Context, repositoryID int64, mapping watchMaterializationMapping) (bool, error) {
	if mapping.Dirty {
		return true, nil
	}
	if mapping.LastWatchHash == nil {
		return false, nil
	}
	currentHash, exists, err := s.WatchResourceHash(ctx, mapping.ResourceType, mapping.ResourceID)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	if currentHash == *mapping.LastWatchHash {
		return false, nil
	}
	if err := s.MarkMappingDirty(ctx, repositoryID, mapping.OwnerType, mapping.OwnerKey, mapping.ResourceType, mapping.ResourceID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) RepositoryMaterializationCount(ctx context.Context, repositoryID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM watch_materialization WHERE repository_id = ?`, repositoryID).Scan(&count)
	return count, err
}

func (s *Store) FilterDecisions(ctx context.Context, repositoryID int64, q FilterDecisionQuery) ([]FilterDecision, error) {
	runID, err := s.latestFilterRunID(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	if runID == 0 {
		return []FilterDecision{}, nil
	}
	query := `
		SELECT id, filter_run_id, owner_type, owner_id, owner_key, decision, reason, score, tier, signals_json
		FROM watch_filter_decisions
		WHERE filter_run_id = ?`
	args := []any{runID}
	if q.OwnerType != "" {
		query += ` AND owner_type = ?`
		args = append(args, q.OwnerType)
	}
	if q.Decision != "" {
		query += ` AND decision = ?`
		args = append(args, q.Decision)
	}
	query += ` ORDER BY id`
	if q.Limit == 0 {
		q.Limit = 100
	}
	query += ` LIMIT ? OFFSET ?`
	args = append(args, q.Limit, q.Offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []FilterDecision
	for rows.Next() {
		var item FilterDecision
		var score sql.NullFloat64
		if err := rows.Scan(&item.ID, &item.FilterRunID, &item.OwnerType, &item.OwnerID, &item.OwnerKey, &item.Decision, &item.Reason, &score, &item.Tier, &item.SignalsJSON); err != nil {
			return nil, err
		}
		if score.Valid {
			item.Score = &score.Float64
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) RepresentationSummary(ctx context.Context, repositoryID int64) (RepresentationSummary, error) {
	summary := RepresentationSummary{RepositoryID: repositoryID}
	var finished sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT raw_graph_hash, filter_settings_hash, representation_hash, status, started_at, finished_at,
		       elements_created, elements_updated, connectors_created, connectors_updated, views_created
		FROM watch_representation_runs
		WHERE repository_id = ?
		ORDER BY id DESC
		LIMIT 1`, repositoryID).Scan(
		&summary.RawGraphHash, &summary.SettingsHash, &summary.RepresentationHash, &summary.LastStatus, &summary.LastStartedAt, &finished,
		&summary.ElementsCreated, &summary.ElementsUpdated, &summary.ConnectorsCreated, &summary.ConnectorsUpdated, &summary.ViewsCreated,
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return RepresentationSummary{}, err
	}
	if finished.Valid {
		summary.LastFinishedAt = &finished.String
	}
	var filterFinished sql.NullString
	err = s.db.QueryRowContext(ctx, `
		SELECT visible_symbols, hidden_symbols, visible_references, hidden_references, finished_at
		FROM watch_filter_runs
		WHERE repository_id = ?
		ORDER BY id DESC
		LIMIT 1`, repositoryID).Scan(&summary.VisibleSymbols, &summary.HiddenSymbols, &summary.VisibleReferences, &summary.HiddenReferences, &filterFinished)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return RepresentationSummary{}, err
	}
	return summary, nil
}

func (s *Store) RawGraphHash(ctx context.Context, repositoryID int64) (string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT stable_key, signature_hash, content_hash
		FROM watch_symbols
		WHERE repository_id = ?
		ORDER BY stable_key`, repositoryID)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	h := sha256.New()
	for rows.Next() {
		var stableKey, signatureHash, contentHash string
		if err := rows.Scan(&stableKey, &signatureHash, &contentHash); err != nil {
			return "", err
		}
		_, _ = h.Write([]byte("s:" + stableKey + ":" + signatureHash + ":" + contentHash + "\n"))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	refRows, err := s.db.QueryContext(ctx, `
		SELECT source.stable_key, target.stable_key, r.kind, r.evidence_hash
		FROM watch_references r
		JOIN watch_symbols source ON source.id = r.source_symbol_id
		JOIN watch_symbols target ON target.id = r.target_symbol_id
		WHERE r.repository_id = ?
		ORDER BY source.stable_key, target.stable_key, r.kind, r.evidence_hash`, repositoryID)
	if err != nil {
		return "", err
	}
	defer func() { _ = refRows.Close() }()
	for refRows.Next() {
		var sourceKey, targetKey string
		var kind, evidenceHash string
		if err := refRows.Scan(&sourceKey, &targetKey, &kind, &evidenceHash); err != nil {
			return "", err
		}
		_, _ = fmt.Fprintf(h, "r:%s:%s:%s:%s\n", sourceKey, targetKey, kind, evidenceHash)
	}
	if err := refRows.Err(); err != nil {
		return "", err
	}
	factRows, err := s.db.QueryContext(ctx, `
		SELECT enricher, stable_key, type, fact_hash
		FROM watch_facts
		WHERE repository_id = ?
		ORDER BY enricher, stable_key, type`, repositoryID)
	if err != nil {
		return "", err
	}
	defer func() { _ = factRows.Close() }()
	for factRows.Next() {
		var enricher, stableKey, factType, factHash string
		if err := factRows.Scan(&enricher, &stableKey, &factType, &factHash); err != nil {
			return "", err
		}
		_, _ = fmt.Fprintf(h, "f:%s:%s:%s:%s\n", enricher, stableKey, factType, factHash)
	}
	if err := factRows.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *Store) AcquireLock(ctx context.Context, repositoryID int64, pid int, token string, staleAfter time.Duration) (Lock, error) {
	if staleAfter <= 0 {
		staleAfter = LockHeartbeatTimeout
	}
	now := nowString()
	cutoff := time.Now().UTC().Add(-staleAfter).Format(time.RFC3339)
	if err := s.markStaleLocks(ctx, cutoff); err != nil {
		return Lock{}, err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO watch_locks(id, repository_id, pid, token, started_at, heartbeat_at, status)
		VALUES (1, ?, ?, ?, ?, ?, 'active')
		ON CONFLICT(id) DO UPDATE SET
			repository_id = excluded.repository_id,
			pid = excluded.pid,
			token = excluded.token,
			started_at = excluded.started_at,
			heartbeat_at = excluded.heartbeat_at,
			status = 'active'
		WHERE watch_locks.status NOT IN ('active', 'paused', 'stopping') OR watch_locks.heartbeat_at < ?`,
		repositoryID, pid, token, now, now, cutoff)
	if err != nil {
		return Lock{}, err
	}
	lock, err := s.ActiveLock(ctx)
	if err != nil {
		return Lock{}, err
	}
	if lock.RepositoryID != repositoryID || lock.Token != token {
		return Lock{}, fmt.Errorf("repository is already watched by pid %d", lock.PID)
	}
	return lock, nil
}

func (s *Store) markStaleLocks(ctx context.Context, cutoff string) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE watch_locks SET status = 'stale' WHERE status IN ('active', 'paused', 'stopping') AND heartbeat_at < ?`, cutoff); err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, pid, token
		FROM watch_locks
		WHERE status IN ('active', 'paused', 'stopping')`)
	if err != nil {
		return err
	}
	type staleItem struct {
		id    int64
		token string
	}
	var staleItems []staleItem
	for rows.Next() {
		var id int64
		var pid int
		var token string
		if err := rows.Scan(&id, &pid, &token); err != nil {
			return err
		}
		if !watchProcessIsRunning(pid) {
			staleItems = append(staleItems, staleItem{id: id, token: token})
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, si := range staleItems {
		if _, err := s.db.ExecContext(ctx, `UPDATE watch_locks SET status = 'stale' WHERE id = ? AND token = ? AND status IN ('active', 'paused', 'stopping')`, si.id, si.token); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ActiveLock(ctx context.Context) (Lock, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, repository_id, pid, token, started_at, heartbeat_at, status
		FROM watch_locks
		WHERE status IN ('active', 'paused', 'stopping')
		ORDER BY id
		LIMIT 1`)
	return scanLock(row)
}

func (s *Store) lockByRepositoryToken(ctx context.Context, repositoryID int64, token string) (Lock, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, repository_id, pid, token, started_at, heartbeat_at, status
		FROM watch_locks
		WHERE repository_id = ? AND token = ?
		LIMIT 1`, repositoryID, token)
	return scanLock(row)
}

func (s *Store) ActiveLiveLock(ctx context.Context, staleAfter time.Duration) (Lock, bool, error) {
	if staleAfter <= 0 {
		staleAfter = LockHeartbeatTimeout
	}
	lock, err := s.ActiveLock(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Lock{}, false, nil
	}
	if err != nil {
		return Lock{}, false, err
	}
	heartbeat, err := time.Parse(time.RFC3339, lock.HeartbeatAt)
	if err != nil || time.Since(heartbeat) > staleAfter || !watchProcessIsRunning(lock.PID) || lock.Status == "stale" || lock.Status == "released" {
		_, _ = s.db.ExecContext(ctx, `UPDATE watch_locks SET status = 'stale' WHERE id = ? AND token = ? AND status IN ('active', 'paused', 'stopping')`, lock.ID, lock.Token)
		return lock, false, nil
	}
	return lock, true, nil
}

func (s *Store) HeartbeatLock(ctx context.Context, repositoryID int64, token string) (Lock, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE watch_locks
		SET heartbeat_at = ?
		WHERE repository_id = ? AND token = ? AND status IN ('active', 'paused')`,
		nowString(), repositoryID, token)
	if err != nil {
		return Lock{}, err
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return Lock{}, sql.ErrNoRows
	}
	return s.lockByRepositoryToken(ctx, repositoryID, token)
}

func (s *Store) RequestStop(ctx context.Context, repositoryID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE watch_locks SET status = 'stopping', heartbeat_at = ? WHERE repository_id = ? AND status IN ('active', 'paused')`, nowString(), repositoryID)
	return err
}

func (s *Store) RequestStopActive(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE watch_locks SET status = 'stopping', heartbeat_at = ? WHERE status IN ('active', 'paused')`, nowString())
	return err
}

func (s *Store) RequestPause(ctx context.Context, repositoryID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE watch_locks SET status = 'paused', heartbeat_at = ? WHERE repository_id = ? AND status = 'active'`, nowString(), repositoryID)
	return err
}

func (s *Store) RequestPauseActive(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE watch_locks SET status = 'paused', heartbeat_at = ? WHERE status = 'active'`, nowString())
	return err
}

func (s *Store) RequestResume(ctx context.Context, repositoryID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE watch_locks SET status = 'active', heartbeat_at = ? WHERE repository_id = ? AND status = 'paused'`, nowString(), repositoryID)
	return err
}

func (s *Store) RequestResumeActive(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE watch_locks SET status = 'active', heartbeat_at = ? WHERE status = 'paused'`, nowString())
	return err
}

func (s *Store) LockStatus(ctx context.Context, repositoryID int64, token string) (string, error) {
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM watch_locks WHERE repository_id = ? AND token = ?`, repositoryID, token).Scan(&status)
	return status, err
}

func (s *Store) ReleaseLock(ctx context.Context, repositoryID int64, token string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE watch_locks SET status = 'released', heartbeat_at = ? WHERE repository_id = ? AND token = ?`, nowString(), repositoryID, token)
	return err
}

func (s *Store) AcquireApplyLock(ctx context.Context, repositoryID int64, pid int, token string, staleAfter time.Duration) error {
	if staleAfter <= 0 {
		staleAfter = LockHeartbeatTimeout
	}
	now := nowString()
	cutoff := time.Now().UTC().Add(-staleAfter).Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO watch_apply_locks(id, repository_id, pid, token, started_at, heartbeat_at, status)
		VALUES (1, ?, ?, ?, ?, ?, 'active')
		ON CONFLICT(id) DO UPDATE SET
			repository_id = excluded.repository_id,
			pid = excluded.pid,
			token = excluded.token,
			started_at = excluded.started_at,
			heartbeat_at = excluded.heartbeat_at,
			status = 'active'
		WHERE watch_apply_locks.status != 'active' OR watch_apply_locks.heartbeat_at < ?`,
		repositoryID, pid, token, now, now, cutoff)
	if err != nil {
		return err
	}
	live, err := s.ActiveApplyLock(ctx, staleAfter)
	if err != nil || !live {
		return err
	}
	var got string
	err = s.db.QueryRowContext(ctx, `SELECT token FROM watch_apply_locks WHERE id = 1 AND status = 'active'`).Scan(&got)
	if err != nil {
		return err
	}
	if got != token {
		return fmt.Errorf("watch apply is already active")
	}
	return nil
}

func (s *Store) ActiveApplyLock(ctx context.Context, staleAfter time.Duration) (bool, error) {
	if staleAfter <= 0 {
		staleAfter = LockHeartbeatTimeout
	}
	var id int64
	var pid int
	var token, heartbeatAt, status string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, pid, token, heartbeat_at, status
		FROM watch_apply_locks
		WHERE id = 1 AND status = 'active'`).Scan(&id, &pid, &token, &heartbeatAt, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	heartbeat, err := time.Parse(time.RFC3339, heartbeatAt)
	if err != nil || time.Since(heartbeat) > staleAfter || !watchProcessIsRunning(pid) || status != "active" {
		_, _ = s.db.ExecContext(ctx, `UPDATE watch_apply_locks SET status = 'stale' WHERE id = ? AND token = ? AND status = 'active'`, id, token)
		return false, nil
	}
	return true, nil
}

func (s *Store) ReleaseApplyLock(ctx context.Context, repositoryID int64, token string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE watch_apply_locks SET status = 'released', heartbeat_at = ? WHERE repository_id = ? AND token = ?`, nowString(), repositoryID, token)
	return err
}

func (s *Store) EnsureGitTags(ctx context.Context) error {
	return tagcolors.Ensure(ctx, s.db, managedGitTags())
}

func (s *Store) ApplyGitTags(ctx context.Context, repositoryID int64, status GitStatus) (GitTagUpdateResult, error) {
	if err := s.EnsureGitTags(ctx); err != nil {
		return GitTagUpdateResult{}, err
	}
	files := map[string][]string{}
	addTags := func(paths []string, tag string) {
		for _, p := range paths {
			files[filepathToSlash(p)] = append(files[filepathToSlash(p)], tag)
		}
	}
	addTags(status.Staged, "git:staged")
	addTags(status.Unstaged, "git:unstaged")
	addTags(status.Untracked, "git:untracked")
	addTags(status.Deleted, "watch:deleted")
	rows, err := s.db.QueryContext(ctx, `
		SELECT resource_id, owner_type, owner_key
		FROM watch_materialization
		WHERE repository_id = ? AND resource_type = 'element' AND owner_type IN ('file', 'symbol')`, repositoryID)
	if err != nil {
		return GitTagUpdateResult{}, err
	}
	defer func() { _ = rows.Close() }()
	type update struct {
		id   int64
		tags []string
	}
	var updates []update
	var allElementIDs []int64
	type elementOwner struct {
		id        int64
		ownerType string
		ownerKey  string
	}
	var owners []elementOwner
	for rows.Next() {
		var id int64
		var ownerType, ownerKey string
		if err := rows.Scan(&id, &ownerType, &ownerKey); err != nil {
			return GitTagUpdateResult{}, err
		}
		allElementIDs = append(allElementIDs, id)
		owners = append(owners, elementOwner{id: id, ownerType: ownerType, ownerKey: ownerKey})
	}
	if err := rows.Err(); err != nil {
		return GitTagUpdateResult{}, err
	}
	if err := rows.Close(); err != nil {
		return GitTagUpdateResult{}, err
	}
	for _, owner := range owners {
		file, ok, err := s.materializedOwnerFilePath(ctx, repositoryID, owner.ownerType, owner.ownerKey)
		if err != nil {
			return GitTagUpdateResult{}, err
		}
		if !ok {
			continue
		}
		if tags := files[file]; len(tags) > 0 {
			updates = append(updates, update{id: owner.id, tags: tags})
		}
	}
	var result GitTagUpdateResult
	desiredByID := make(map[int64]map[string]struct{}, len(updates))
	for _, item := range updates {
		if desiredByID[item.id] == nil {
			desiredByID[item.id] = map[string]struct{}{}
		}
		for _, tag := range item.tags {
			desiredByID[item.id][tag] = struct{}{}
		}
	}
	for _, id := range allElementIDs {
		var stale []string
		for _, tag := range managedGitTags() {
			if _, keep := desiredByID[id][tag]; !keep {
				stale = append(stale, tag)
			}
		}
		removed, err := s.removeElementTags(ctx, id, stale)
		if err != nil {
			return GitTagUpdateResult{}, err
		}
		result.TagsRemoved += removed
	}
	for _, item := range updates {
		desired := sortedSetValues(desiredByID[item.id])
		added, err := s.addElementTags(ctx, item.id, desired)
		if err != nil {
			return GitTagUpdateResult{}, err
		}
		result.TagsAdded += added
	}
	return result, nil
}

func (s *Store) ChangedRawResourcesSinceLatest(ctx context.Context, repositoryID int64) (changedRawResources, error) {
	changed := changedRawResources{Files: map[string]struct{}{}, Symbols: map[int64]string{}}
	latest, found, err := s.LatestWatchVersion(ctx, repositoryID)
	if err != nil || !found {
		return changed, err
	}
	previous, err := s.WatchVersionResourceSnapshots(ctx, latest.ID)
	if err != nil {
		return changed, err
	}
	current, err := s.CurrentWatchResourceSnapshots(ctx, repositoryID)
	if err != nil {
		return changed, err
	}
	for key, next := range current {
		if next.OwnerType != next.ResourceType {
			continue
		}
		if next.ResourceType != "file" && next.ResourceType != "symbol" {
			continue
		}
		prev, ok := previous[key]
		if ok && prev.Hash == next.Hash {
			continue
		}
		switch next.ResourceType {
		case "file":
			changed.Files[next.OwnerKey] = struct{}{}
		case "symbol":
			if next.ResourceID != nil {
				reason := "changed since latest watch version"
				if !ok {
					reason = "added since latest watch version"
				}
				changed.Symbols[*next.ResourceID] = reason
			}
		}
	}
	for key, prev := range previous {
		if prev.OwnerType != prev.ResourceType {
			continue
		}
		if prev.ResourceType != "file" {
			continue
		}
		if _, ok := current[key]; !ok {
			changed.Files[prev.OwnerKey] = struct{}{}
		}
	}
	return changed, nil
}

func (s *Store) FileLanguages(ctx context.Context, repositoryID int64) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT path, language FROM watch_files WHERE repository_id = ?`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var path, language string
		if err := rows.Scan(&path, &language); err != nil {
			return nil, err
		}
		out[path] = language
	}
	return out, rows.Err()
}

func (s *Store) BuildWatchDiffs(ctx context.Context, repositoryID int64, representationHash string) ([]RepresentationDiff, error) {
	current, err := s.CurrentWatchResourceSnapshots(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	latest, found, err := s.LatestWatchVersion(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	previous := map[string]watchResourceSnapshot{}
	if found {
		previous, err = s.WatchVersionResourceSnapshots(ctx, latest.ID)
		if err != nil {
			return nil, err
		}
	}
	previousBaseline := cloneWatchResourceSnapshots(previous)
	lineDiffs := s.gitLineDiffsAgainstHead(ctx, repositoryID)
	lineHunks := s.gitLineHunksAgainstHead(ctx, repositoryID)
	worktreeChanges := s.gitWorktreeChangesAgainstHead(ctx, repositoryID)
	var diffs []RepresentationDiff
	repoKey := fmt.Sprintf("%d", repositoryID)
	repoSummary := "Representation initialized"
	change := "initialized"
	if found {
		change = "updated"
		repoSummary = "workspace updated"
	} else if len(worktreeChanges) > 0 {
		repoSummary = "Workspace initialized from dirty worktree"
	}
	diffs = append(diffs, RepresentationDiff{OwnerType: "repository", OwnerKey: repoKey, ChangeType: change, BeforeHash: stringPtrIf(found, latest.RepresentationHash), AfterHash: &representationHash, Summary: &repoSummary})
	for key, next := range current {
		if rawFactSnapshot(next) {
			delete(previous, key)
			continue
		}
		prev, ok := previous[key]
		if !ok {
			if prevRaw, previousKey, consumePrevious, rawOK := previousRawSnapshotForMaterialized(previousBaseline, current, next); rawOK {
				before, after := prevRaw.Hash, next.Hash
				diff := snapshotDiff(next, "updated", &before, &after, &prevRaw)
				applyGitLineDiff(&diff, next, &prevRaw, lineDiffs, lineHunks)
				diffs = append(diffs, diff)
				if consumePrevious {
					delete(previous, previousKey)
				}
				continue
			}
			changeType := "added"
			if !found {
				var emit bool
				changeType, emit = shouldEmitInitialSnapshotDiff(next, worktreeChanges)
				if !emit {
					continue
				}
			}
			diff := snapshotDiff(next, changeType, nil, &next.Hash, nil)
			applyGitLineDiff(&diff, next, nil, lineDiffs, lineHunks)
			diffs = append(diffs, diff)
			continue
		}
		if prev.Hash != next.Hash || ptrInt64Value(prev.ResourceID) != ptrInt64Value(next.ResourceID) {
			before, after := prev.Hash, next.Hash
			diff := snapshotDiff(next, "updated", &before, &after, &prev)
			applyGitLineDiff(&diff, next, &prev, lineDiffs, lineHunks)
			diffs = append(diffs, diff)
		}
		delete(previous, key)
	}
	for _, prev := range previous {
		if rawFactSnapshot(prev) {
			continue
		}
		before := prev.Hash
		diff := snapshotDiff(prev, "deleted", &before, nil, nil)
		applyGitLineDiff(&diff, prev, nil, lineDiffs, lineHunks)
		diffs = append(diffs, diff)
	}
	if synthetic, err := s.syntheticDeletedImportDiffs(ctx, repositoryID, lineHunks, diffs); err == nil {
		diffs = append(diffs, synthetic...)
	}
	diffs = suppressDependencyImportSummaryDiffs(diffs)
	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].OwnerType == diffs[j].OwnerType {
			return diffs[i].OwnerKey < diffs[j].OwnerKey
		}
		return diffs[i].OwnerType < diffs[j].OwnerType
	})
	return diffs, nil
}

func suppressDependencyImportSummaryDiffs(diffs []RepresentationDiff) []RepresentationDiff {
	out := diffs[:0]
	for _, diff := range diffs {
		if diff.OwnerType == "fact-summary" {
			_, factType, ok := factSummaryOwnerIdentity(diff.OwnerKey)
			if ok && factType == "dependency.import" {
				continue
			}
		}
		out = append(out, diff)
	}
	return out
}

func (s *Store) syntheticDeletedImportDiffs(ctx context.Context, repositoryID int64, lineHunks map[string][]tldgit.LineHunk, existing []RepresentationDiff) ([]RepresentationDiff, error) {
	if len(lineHunks) == 0 {
		return nil, nil
	}
	summaryFiles := map[string]struct{}{}
	existingOwners := map[string]struct{}{}
	for _, diff := range existing {
		existingOwners[diff.OwnerType+"\x00"+diff.OwnerKey+"\x00"+resourceTypeValue(diff.ResourceType)] = struct{}{}
		if diff.OwnerType != "fact-summary" || diff.ChangeType != "deleted" {
			continue
		}
		file, factType, ok := factSummaryOwnerIdentity(diff.OwnerKey)
		if ok && factType == "dependency.import" {
			summaryFiles[file] = struct{}{}
		}
	}
	if len(summaryFiles) == 0 {
		return nil, nil
	}
	repo, err := s.Repository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	var out []RepresentationDiff
	for file := range summaryFiles {
		if !strings.HasSuffix(file, ".go") {
			continue
		}
		lines := removedLinesFromHead(repo.RepoRoot, file)
		if len(lines) == 0 {
			continue
		}
		for _, hunk := range lineHunks[file] {
			for _, lineNo := range hunk.RemovedLines {
				module := removedGoImportModule(lines, lineNo)
				if module == "" {
					continue
				}
				ownerKey := fmt.Sprintf("fact:dependency.inventory:dependency.import:%s:%s:%d", file, module, lineNo)
				elementKey := "fact\x00" + ownerKey + "\x00element"
				if _, ok := existingOwners[elementKey]; ok {
					continue
				}
				before := hashString(ownerKey + ":deleted")
				resourceType := "element"
				language := "go"
				summary := module
				out = append(out, RepresentationDiff{OwnerType: "fact", OwnerKey: ownerKey, ChangeType: "deleted", BeforeHash: &before, ResourceType: &resourceType, Language: &language, Summary: &summary, RemovedLines: 1})
				existingOwners[elementKey] = struct{}{}

				connectorOwner := ownerKey + ":file"
				connectorKey := "fact-import-connector\x00" + connectorOwner + "\x00connector"
				if _, ok := existingOwners[connectorKey]; ok {
					continue
				}
				connectorHash := hashString(connectorOwner + ":deleted")
				connectorType := "connector"
				connectorSummary := path.Base(file) + "->" + module
				out = append(out, RepresentationDiff{OwnerType: "fact-import-connector", OwnerKey: connectorOwner, ChangeType: "deleted", BeforeHash: &connectorHash, ResourceType: &connectorType, Language: stringPtr(""), Summary: &connectorSummary, RemovedLines: 1})
				existingOwners[connectorKey] = struct{}{}
			}
		}
	}
	return out, nil
}

func removedLinesFromHead(repoRoot, file string) map[int]string {
	if strings.TrimSpace(repoRoot) == "" || strings.TrimSpace(file) == "" {
		return nil
	}
	cmd := exec.Command("git", "-C", repoRoot, "show", "HEAD:"+file)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	result := make(map[int]string, len(lines))
	for i, line := range lines {
		result[i+1] = line
	}
	return result
}

func removedGoImportModule(lines map[int]string, lineNo int) string {
	line := strings.TrimSpace(lines[lineNo])
	if line == "" || strings.HasPrefix(line, "//") {
		return ""
	}
	line = strings.TrimPrefix(line, "import ")
	start := strings.Index(line, `"`)
	if start < 0 {
		return ""
	}
	end := strings.Index(line[start+1:], `"`)
	if end < 0 {
		return ""
	}
	module := strings.TrimSpace(line[start+1 : start+1+end])
	if module == "" || strings.ContainsAny(module, " \t") {
		return ""
	}
	return module
}

func resourceTypeValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPtr(value string) *string {
	return &value
}

func (s *Store) CurrentWatchResourceSnapshots(ctx context.Context, repositoryID int64) (map[string]watchResourceSnapshot, error) {
	out := map[string]watchResourceSnapshot{}
	fileRows, err := s.db.QueryContext(ctx, `SELECT id, path, language, worktree_hash FROM watch_files WHERE repository_id = ?`, repositoryID)
	if err != nil {
		return nil, err
	}
	for fileRows.Next() {
		var id int64
		var path, language, hash string
		if err := fileRows.Scan(&id, &path, &language, &hash); err != nil {
			_ = fileRows.Close()
			return nil, err
		}
		out[resourceSnapshotKey("file", path, "file")] = watchResourceSnapshot{OwnerType: "file", OwnerKey: path, ResourceType: "file", ResourceID: &id, Language: language, Hash: hash, Summary: path}
	}
	if err := fileRows.Close(); err != nil {
		return nil, err
	}
	symRows, err := s.db.QueryContext(ctx, `
		SELECT s.id, COALESCE(i.identity_key, s.stable_key), s.stable_key, f.path, s.content_hash, s.signature_hash, s.qualified_name, s.start_line, s.end_line
		FROM watch_symbols s
		JOIN watch_files f ON f.id = s.file_id
		LEFT JOIN watch_symbol_identities i ON i.repository_id = s.repository_id AND i.current_stable_key = s.stable_key
		WHERE s.repository_id = ?`, repositoryID)
	if err != nil {
		return nil, err
	}
	for symRows.Next() {
		var id int64
		var key, stableKey, filePath, contentHash, signatureHash, name string
		var startLine int
		var endLine sql.NullInt64
		if err := symRows.Scan(&id, &key, &stableKey, &filePath, &contentHash, &signatureHash, &name, &startLine, &endLine); err != nil {
			_ = symRows.Close()
			return nil, err
		}
		hash := hashString(contentHash + ":" + signatureHash)
		end := normalizedEndLine(startLine, endLine)
		out[resourceSnapshotKey("symbol", key, "symbol")] = watchResourceSnapshot{OwnerType: "symbol", OwnerKey: key, ResourceType: "symbol", ResourceID: &id, Language: languageFromStableKey(stableKey), Hash: hash, Summary: name, LineCount: lineCountFromRange(startLine, endLine), FilePath: filepathToSlash(filePath), StartLine: startLine, EndLine: end}
	}
	if err := symRows.Close(); err != nil {
		return nil, err
	}
	factRows, err := s.db.QueryContext(ctx, `
		SELECT enricher, stable_key, type, fact_hash, name, file_path, start_line, end_line
		FROM watch_facts
		WHERE repository_id = ?`, repositoryID)
	if err != nil {
		return nil, err
	}
	for factRows.Next() {
		var enricher, stableKey, factType, factHash, name, filePath string
		var startLine int
		var endLine sql.NullInt64
		if err := factRows.Scan(&enricher, &stableKey, &factType, &factHash, &name, &filePath, &startLine, &endLine); err != nil {
			_ = factRows.Close()
			return nil, err
		}
		ownerKey := "fact:" + enricher + ":" + stableKey
		end := normalizedEndLine(startLine, endLine)
		out[resourceSnapshotKey("fact", ownerKey, "fact")] = watchResourceSnapshot{OwnerType: "fact", OwnerKey: ownerKey, ResourceType: "fact", Language: "", Hash: factHash, Summary: firstNonEmpty(name, factType), LineCount: lineCountFromRange(startLine, endLine), FilePath: filepathToSlash(filePath), StartLine: startLine, EndLine: end}
	}
	if err := factRows.Close(); err != nil {
		return nil, err
	}
	deletedElementIDs, err := s.deletedMaterializedElementIDs(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	mapRows, err := s.db.QueryContext(ctx, `
		SELECT id, owner_type, owner_key, resource_type, resource_id, updated_at
		FROM watch_materialization
		WHERE repository_id = ?`, repositoryID)
	if err != nil {
		return nil, err
	}
	var mappings []watchMaterializationMapping
	for mapRows.Next() {
		var mapping watchMaterializationMapping
		if err := mapRows.Scan(&mapping.ID, &mapping.OwnerType, &mapping.OwnerKey, &mapping.ResourceType, &mapping.ResourceID, &mapping.UpdatedAt); err != nil {
			_ = mapRows.Close()
			return nil, err
		}
		mappings = append(mappings, mapping)
	}
	if err := mapRows.Close(); err != nil {
		return nil, err
	}
	for _, mapping := range mappings {
		tombstoned, err := s.materializationMappingTombstoned(ctx, repositoryID, mapping, deletedElementIDs)
		if err != nil {
			return nil, err
		}
		if tombstoned {
			continue
		}
		hash, summary, language, lineCount, err := s.materializedResourceHash(ctx, repositoryID, mapping.OwnerType, mapping.OwnerKey, mapping.ResourceType, mapping.ResourceID)
		if err != nil {
			continue
		}
		id := mapping.ResourceID
		filePath, startLine, endLine := materializedSourceRange(ctx, s.db, repositoryID, mapping.OwnerType, mapping.OwnerKey, "")
		out[resourceSnapshotKey(mapping.OwnerType, mapping.OwnerKey, mapping.ResourceType)] = watchResourceSnapshot{OwnerType: mapping.OwnerType, OwnerKey: mapping.OwnerKey, ResourceType: mapping.ResourceType, ResourceID: &id, Language: language, Hash: hash, Summary: summary, LineCount: lineCount, FilePath: filePath, StartLine: startLine, EndLine: endLine}
	}
	return out, nil
}

func (s *Store) materializedResourceHash(ctx context.Context, repositoryID int64, ownerType, ownerKey, resourceType string, resourceID int64) (string, string, string, int, error) {
	switch resourceType {
	case "element":
		var name, kind, description, repo, branch, filePath, language sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT name, kind, description, repo, branch, file_path, language FROM elements WHERE id = ?`, resourceID).Scan(&name, &kind, &description, &repo, &branch, &filePath, &language)
		if err != nil {
			return "", "", "", 0, err
		}
		descriptionForHash := description.String
		if ownerType == "symbol" {
			if path := sourceAnchorFilePath(descriptionForHash); path != "" {
				descriptionForHash = path
			}
		}
		raw := strings.Join([]string{name.String, kind.String, descriptionForHash, repo.String, branch.String, filePath.String, language.String}, "\n")
		if ownerType == "symbol" {
			raw += "\n" + symbolSnapshotHash(ctx, s.db, repositoryID, ownerKey)
		}
		return hashString(raw), name.String, language.String, materializedLineCount(ctx, s.db, repositoryID, ownerType, ownerKey, filePath.String), nil
	case "view":
		var name, label sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT name, level_label FROM views WHERE id = ?`, resourceID).Scan(&name, &label)
		if err != nil {
			return "", "", "", 0, err
		}
		return hashString(name.String + "\n" + label.String), name.String, "", 0, nil
	case "connector":
		var viewID, sourceID, targetID int64
		var label, relationship, direction sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT view_id, source_element_id, target_element_id, label, relationship, direction FROM connectors WHERE id = ?`, resourceID).Scan(&viewID, &sourceID, &targetID, &label, &relationship, &direction)
		if err != nil {
			return "", "", "", 0, err
		}
		raw := fmt.Sprintf("%d:%d:%d:%s:%s:%s", viewID, sourceID, targetID, label.String, relationship.String, direction.String)
		return hashString(raw), s.connectorSummary(ctx, sourceID, targetID, direction.String), "", materializedLineCount(ctx, s.db, repositoryID, ownerType, ownerKey, ""), nil
	default:
		return "", "", "", 0, fmt.Errorf("unsupported resource type %q", resourceType)
	}
}

func (s *Store) connectorSummary(ctx context.Context, sourceID, targetID int64, direction string) string {
	sourceName := elementName(ctx, s.db, sourceID)
	targetName := elementName(ctx, s.db, targetID)
	if sourceName == "" {
		sourceName = fmt.Sprintf("element %d", sourceID)
	}
	if targetName == "" {
		targetName = fmt.Sprintf("element %d", targetID)
	}
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "both", "bidirectional":
		return sourceName + "<->" + targetName
	case "backward":
		return targetName + "->" + sourceName
	case "none":
		return sourceName + "--" + targetName
	default:
		return sourceName + "->" + targetName
	}
}

func (s *Store) WatchVersionResourceSnapshots(ctx context.Context, versionID int64) (map[string]watchResourceSnapshot, error) {
	if err := s.ensureWatchVersionResourceRangeColumns(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT owner_type, owner_key, resource_type, resource_id, language, resource_hash, summary, line_count, file_path, start_line, end_line
		FROM watch_version_resources
		WHERE version_id = ?`, versionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]watchResourceSnapshot{}
	for rows.Next() {
		var item watchResourceSnapshot
		var resourceID sql.NullInt64
		var language, summary, filePath sql.NullString
		if err := rows.Scan(&item.OwnerType, &item.OwnerKey, &item.ResourceType, &resourceID, &language, &item.Hash, &summary, &item.LineCount, &filePath, &item.StartLine, &item.EndLine); err != nil {
			return nil, err
		}
		if resourceID.Valid {
			item.ResourceID = &resourceID.Int64
		}
		item.Language = language.String
		item.Summary = summary.String
		item.FilePath = filepathToSlash(filePath.String)
		out[resourceSnapshotKey(item.OwnerType, item.OwnerKey, item.ResourceType)] = item
	}
	return out, rows.Err()
}

func (s *Store) SaveWatchVersionResources(ctx context.Context, versionID, repositoryID int64) error {
	if err := s.ensureWatchVersionResourceRangeColumns(ctx); err != nil {
		return err
	}
	snapshots, err := s.CurrentWatchResourceSnapshots(ctx, repositoryID)
	if err != nil {
		return err
	}
	for _, item := range snapshots {
		_, err := s.db.ExecContext(ctx, `
			INSERT OR REPLACE INTO watch_version_resources(version_id, owner_type, owner_key, resource_type, resource_id, language, resource_hash, summary, line_count, file_path, start_line, end_line)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			versionID, item.OwnerType, item.OwnerKey, item.ResourceType, item.ResourceID, nullString(item.Language), item.Hash, nullString(item.Summary), item.LineCount, nullString(item.FilePath), item.StartLine, item.EndLine)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensureWatchVersionResourceRangeColumns(ctx context.Context) error {
	for _, stmt := range []string{
		`ALTER TABLE watch_version_resources ADD COLUMN file_path TEXT NULL`,
		`ALTER TABLE watch_version_resources ADD COLUMN start_line INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE watch_version_resources ADD COLUMN end_line INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	return nil
}

func (s *Store) gitLineDiffsAgainstHead(ctx context.Context, repositoryID int64) map[string]tldgit.LineDiff {
	repo, err := s.Repository(ctx, repositoryID)
	if err != nil || strings.TrimSpace(repo.RepoRoot) == "" {
		return nil
	}
	diffs, err := tldgit.LineDiffsAgainstHead(repo.RepoRoot)
	if err != nil {
		return nil
	}
	return diffs
}

func (s *Store) gitLineHunksAgainstHead(ctx context.Context, repositoryID int64) map[string][]tldgit.LineHunk {
	repo, err := s.Repository(ctx, repositoryID)
	if err != nil || strings.TrimSpace(repo.RepoRoot) == "" {
		return nil
	}
	hunks, err := tldgit.LineHunksAgainstHead(repo.RepoRoot)
	if err != nil {
		return nil
	}
	return hunks
}

func (s *Store) gitWorktreeChangesAgainstHead(ctx context.Context, repositoryID int64) map[string]tldgit.WorktreeChange {
	repo, err := s.Repository(ctx, repositoryID)
	if err != nil || strings.TrimSpace(repo.RepoRoot) == "" {
		return nil
	}
	changes, err := tldgit.WorktreeChangesAgainstHead(repo.RepoRoot)
	if err != nil {
		return nil
	}
	return changes
}
