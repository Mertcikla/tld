package watch

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

type RepositoryInput struct {
	RemoteURL      string
	RepoRoot       string
	DisplayName    string
	Branch         string
	HeadCommit     string
	IdentityStatus string
	SettingsHash   string
}

type factScanner interface {
	Scan(dest ...any) error
}

type SymbolQuery struct {
	Search string
	File   string
	Kind   string
	Limit  int
	Offset int
}

type storedSymbolIdentity struct {
	IdentityKey   string
	StableKey     string
	FilePath      string
	Kind          string
	Name          string
	QualifiedName string
	StartLine     int
	ContentHash   string
	MissingFile   bool
}
type ReferenceQuery struct {
	SymbolID int64
	Limit    int
	Offset   int
}

func (s *Store) BeginFilterRun(ctx context.Context, repositoryID int64, settingsHash, rawGraphHash string) (int64, error) {
	row := &filterRunModel{RepositoryID: repositoryID, SettingsHash: settingsHash, RawGraphHash: rawGraphHash, StartedAt: nowString(), Status: "running"}
	_, err := s.bun.NewInsert().Model(row).Exec(ctx)
	if err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (s *Store) SaveFilterDecision(ctx context.Context, filterRunID int64, ownerType string, ownerID int64, ownerKey string, decision, reason string, score *float64, tier int, signalsJSON string) error {
	if signalsJSON == "" {
		signalsJSON = "[]"
	}
	row := &filterDecisionModel{FilterRunID: filterRunID, OwnerType: ownerType, OwnerID: ownerID, OwnerKey: ownerKey, Decision: decision, Reason: reason, Score: score, Tier: tier, SignalsJSON: signalsJSON}
	_, err := s.bun.NewInsert().Model(row).Exec(ctx)
	return err
}

func (s *Store) FinishFilterRun(ctx context.Context, id int64, status string, visibleSymbols, hiddenSymbols, visibleReferences, hiddenReferences int) error {
	_, err := s.bun.NewUpdate().
		Model((*filterRunModel)(nil)).
		Set("finished_at = ?", nowString()).
		Set("status = ?", status).
		Set("visible_symbols = ?", visibleSymbols).
		Set("hidden_symbols = ?", hiddenSymbols).
		Set("visible_references = ?", visibleReferences).
		Set("hidden_references = ?", hiddenReferences).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (s *Store) UpsertCluster(ctx context.Context, repositoryID int64, stableKey string, parentClusterID *int64, name, kind, algorithm, settingsHash string, memberIDs []int64) (Cluster, error) {
	now := nowString()
	_, err := s.execRaw(ctx, `
		INSERT INTO watch_clusters(repository_id, stable_key, parent_cluster_id, name, kind, algorithm, settings_hash, member_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repository_id, stable_key) DO UPDATE SET
			parent_cluster_id = excluded.parent_cluster_id,
			name = excluded.name,
			kind = excluded.kind,
			algorithm = excluded.algorithm,
			settings_hash = excluded.settings_hash,
			member_count = excluded.member_count,
			updated_at = excluded.updated_at`,
		repositoryID, stableKey, parentClusterID, name, kind, algorithm, settingsHash, len(memberIDs), now, now)
	if err != nil {
		return Cluster{}, err
	}
	cluster, err := s.clusterByStableKey(ctx, repositoryID, stableKey)
	if err != nil {
		return Cluster{}, err
	}
	if _, err := s.execRaw(ctx, `DELETE FROM watch_cluster_members WHERE cluster_id = ?`, cluster.ID); err != nil {
		return Cluster{}, err
	}
	for _, memberID := range memberIDs {
		if _, err := s.execRaw(ctx, `
			INSERT INTO watch_cluster_members(cluster_id, owner_type, owner_id)
			VALUES (?, 'symbol', ?)`, cluster.ID, memberID); err != nil {
			return Cluster{}, err
		}
	}
	return cluster, nil
}

func (s *Store) Clusters(ctx context.Context, repositoryID int64) ([]Cluster, error) {
	rows, err := s.rowsRaw(ctx, `
		SELECT id, repository_id, stable_key, parent_cluster_id, name, kind, algorithm, settings_hash, member_count, created_at, updated_at
		FROM watch_clusters
		WHERE repository_id = ?
		ORDER BY stable_key`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Cluster
	for rows.Next() {
		cluster, err := scanCluster(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cluster)
	}
	return out, rows.Err()
}

func (s *Store) BeginRepresentationRun(ctx context.Context, repositoryID int64, rawGraphHash, settingsHash string, embeddingModelID *int64, representationHash string) (int64, error) {
	return s.insertReturningID(ctx, `
		INSERT INTO watch_representation_runs(repository_id, raw_graph_hash, filter_settings_hash, embedding_model_id, representation_hash, started_at, status)
		VALUES (?, ?, ?, ?, ?, ?, 'running')`,
		repositoryID, rawGraphHash, settingsHash, embeddingModelID, representationHash, nowString())
}

func (s *Store) FinishRepresentationRun(ctx context.Context, id int64, status string, result RepresentResult, runErr error) error {
	var errText any
	if runErr != nil {
		errText = runErr.Error()
	}
	_, err := s.execRaw(ctx, `
		UPDATE watch_representation_runs
		SET finished_at = ?, status = ?, elements_created = ?, elements_updated = ?, connectors_created = ?, connectors_updated = ?, views_created = ?, error = ?
		WHERE id = ?`,
		nowString(), status, result.ElementsCreated, result.ElementsUpdated, result.ConnectorsCreated, result.ConnectorsUpdated, result.ViewsCreated, errText, id)
	return err
}

func (s *Store) LatestCompletedRepresentationRun(ctx context.Context, repositoryID int64, rawGraphHash, settingsHash string, embeddingModelID *int64) (RepresentResult, bool, error) {
	query := `
		SELECT id, raw_graph_hash, filter_settings_hash, representation_hash,
		       elements_created, elements_updated, connectors_created, connectors_updated, views_created
		FROM watch_representation_runs
		WHERE repository_id = ? AND raw_graph_hash = ? AND filter_settings_hash = ? AND status = 'completed' AND embedding_model_id IS NULL
		ORDER BY id DESC
		LIMIT 1`
	args := []any{repositoryID, rawGraphHash, settingsHash}
	if embeddingModelID != nil {
		query = `
			SELECT id, raw_graph_hash, filter_settings_hash, representation_hash,
			       elements_created, elements_updated, connectors_created, connectors_updated, views_created
			FROM watch_representation_runs
			WHERE repository_id = ? AND raw_graph_hash = ? AND filter_settings_hash = ? AND status = 'completed' AND embedding_model_id = ?
			ORDER BY id DESC
			LIMIT 1`
		args = append(args, *embeddingModelID)
	}
	result := RepresentResult{RepositoryID: repositoryID}
	err := s.rowRaw(ctx, query, args...).Scan(
		&result.RepresentationRun,
		&result.RawGraphHash,
		&result.SettingsHash,
		&result.RepresentationHash,
		&result.ElementsCreated,
		&result.ElementsUpdated,
		&result.ConnectorsCreated,
		&result.ConnectorsUpdated,
		&result.ViewsCreated,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return RepresentResult{}, false, nil
	}
	return result, err == nil, err
}

func (s *Store) MappingResourceID(ctx context.Context, repositoryID int64, ownerType, ownerKey, resourceType string) (int64, bool, error) {
	var id int64
	err := s.rowRaw(ctx, `
		SELECT resource_id FROM watch_materialization
		WHERE repository_id = ? AND owner_type = ? AND owner_key = ? AND resource_type = ?`,
		repositoryID, ownerType, ownerKey, resourceType).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil, err
}

func (s *Store) EnsureRepository(ctx context.Context, input RepositoryInput) (Repository, error) {
	input.RemoteURL = strings.TrimSpace(input.RemoteURL)
	input.RepoRoot = strings.TrimSpace(input.RepoRoot)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.DisplayName == "" {
		input.DisplayName = input.RepoRoot
	}
	if input.IdentityStatus == "" {
		input.IdentityStatus = "known"
	}
	if input.RemoteURL == "" {
		input.IdentityStatus = "local_only"
	}
	now := nowString()

	var existing repositoryModel
	var err error
	if input.RemoteURL != "" {
		err = s.bun.NewSelect().Model(&existing).Column("id").Where("remote_url = ?", input.RemoteURL).Scan(ctx)
	} else {
		err = s.bun.NewSelect().Model(&existing).Column("id").Where("repo_root = ?", input.RepoRoot).Where("identity_status = 'local_only'").Scan(ctx)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Repository{}, err
	}
	if existing.ID > 0 {
		_, err = s.bun.NewUpdate().
			Model((*repositoryModel)(nil)).
			Set("repo_root = ?", input.RepoRoot).
			Set("display_name = ?", input.DisplayName).
			Set("branch = ?", nullString(input.Branch)).
			Set("head_commit = ?", nullString(input.HeadCommit)).
			Set("identity_status = ?", input.IdentityStatus).
			Set("settings_hash = ?", input.SettingsHash).
			Set("updated_at = ?", now).
			Where("id = ?", existing.ID).
			Exec(ctx)
		if err != nil {
			return Repository{}, err
		}
		return s.Repository(ctx, existing.ID)
	}

	row := &repositoryModel{
		RemoteURL:      stringPtrOrNil(input.RemoteURL),
		RepoRoot:       input.RepoRoot,
		DisplayName:    input.DisplayName,
		Branch:         stringPtrOrNil(input.Branch),
		HeadCommit:     stringPtrOrNil(input.HeadCommit),
		IdentityStatus: input.IdentityStatus,
		SettingsHash:   input.SettingsHash,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err = s.bun.NewInsert().Model(row).Exec(ctx)
	if err != nil {
		return Repository{}, err
	}
	return s.Repository(ctx, row.ID)
}

func (s *Store) Repository(ctx context.Context, id int64) (Repository, error) {
	var row repositoryModel
	err := s.bun.NewSelect().Model(&row).Where("id = ?", id).Scan(ctx)
	return repositoryFromModel(row), err
}

func (s *Store) Repositories(ctx context.Context) ([]Repository, error) {
	var rows []repositoryModel
	if err := s.bun.NewSelect().Model(&rows).Order("display_name").Order("id").Scan(ctx); err != nil {
		return nil, err
	}
	repos := make([]Repository, 0, len(rows))
	for _, row := range rows {
		repos = append(repos, repositoryFromModel(row))
	}
	return repos, nil
}

func (s *Store) ReassociateRepository(ctx context.Context, id int64, remoteURL string) (Repository, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return Repository{}, fmt.Errorf("remote_url is required")
	}
	_, err := s.execRaw(ctx, `
		UPDATE watch_repositories
		SET remote_url = ?, identity_status = 'known', updated_at = ?
		WHERE id = ?`, remoteURL, nowString(), id)
	if err != nil {
		return Repository{}, err
	}
	return s.Repository(ctx, id)
}

func (s *Store) BeginScanRun(ctx context.Context, repositoryID int64, mode string) (int64, error) {
	return s.insertReturningID(ctx, `
		INSERT INTO watch_scan_runs(repository_id, mode, started_at, status)
		VALUES (?, ?, ?, 'running')`, repositoryID, mode, nowString())
}

func (s *Store) FinishScanRun(ctx context.Context, id int64, status string, result ScanResult, runErr error) error {
	var errText any
	if runErr != nil {
		errText = runErr.Error()
	} else if result.Warning != "" {
		errText = result.Warning
	}
	_, err := s.execRaw(ctx, `
		UPDATE watch_scan_runs
		SET finished_at = ?, status = ?, files_seen = ?, files_parsed = ?, files_skipped = ?, symbols_seen = ?, references_seen = ?, error = ?
		WHERE id = ?`,
		nowString(),
		status,
		result.FilesSeen,
		result.FilesParsed,
		result.FilesSkipped,
		result.SymbolsSeen,
		result.ReferencesSeen,
		errText,
		id,
	)
	return err
}

func (s *Store) UpsertFile(ctx context.Context, repositoryID int64, path, language, gitBlobHash, worktreeHash string, sizeBytes, mtimeUnix int64, status string, scanErr error) (File, bool, error) {
	existing, found, err := s.fileByPath(ctx, repositoryID, path)
	if err != nil {
		return File{}, false, err
	}
	unchanged := found && existing.WorktreeHash == worktreeHash && existing.ScanStatus != "error"
	if unchanged {
		_, err := s.execRaw(ctx, `
			UPDATE watch_files
			SET git_blob_hash = ?, size_bytes = ?, mtime_unix = ?, scan_status = 'skipped', scan_error = NULL, updated_at = ?
			WHERE id = ?`, nullString(gitBlobHash), sizeBytes, mtimeUnix, nowString(), existing.ID)
		if err != nil {
			return File{}, false, err
		}
		file, err := s.file(ctx, existing.ID)
		return file, true, err
	}

	errText := ""
	if scanErr != nil {
		errText = scanErr.Error()
	}
	now := nowString()
	_, err = s.execRaw(ctx, `
		INSERT INTO watch_files(repository_id, path, language, git_blob_hash, worktree_hash, size_bytes, mtime_unix, scan_status, scan_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repository_id, path) DO UPDATE SET
			language = excluded.language,
			git_blob_hash = excluded.git_blob_hash,
			worktree_hash = excluded.worktree_hash,
			size_bytes = excluded.size_bytes,
			mtime_unix = excluded.mtime_unix,
			scan_status = excluded.scan_status,
			scan_error = excluded.scan_error,
			updated_at = excluded.updated_at`,
		repositoryID, path, language, nullString(gitBlobHash), worktreeHash, sizeBytes, mtimeUnix, status, nullString(errText), now, now)
	if err != nil {
		return File{}, false, err
	}
	file, err := s.fileByPathMust(ctx, repositoryID, path)
	return file, false, err
}

func (s *Store) DeleteMissingFiles(ctx context.Context, repositoryID int64, seen map[string]struct{}) error {
	rows, err := s.rowsRaw(ctx, `SELECT id, path FROM watch_files WHERE repository_id = ?`, repositoryID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			return err
		}
		if _, ok := seen[path]; !ok {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.execRaw(ctx, `DELETE FROM watch_files WHERE id = ?`, id); err != nil {
			return err
		}
	}
	if len(ids) > 0 {
		if _, err := s.execRaw(ctx, `DELETE FROM watch_symbol_identities WHERE repository_id = ? AND current_stable_key NOT IN (SELECT stable_key FROM watch_symbols WHERE repository_id = ?)`, repositoryID, repositoryID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteFilesByPath(ctx context.Context, repositoryID int64, paths []string) error {
	for _, path := range paths {
		path = strings.TrimSpace(filepathToSlash(path))
		if path == "" {
			continue
		}
		if _, err := s.execRaw(ctx, `DELETE FROM watch_files WHERE repository_id = ? AND path = ?`, repositoryID, path); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ReplaceFileSymbols(ctx context.Context, repositoryID, fileID int64, symbols []Symbol) error {
	missingIdentities, err := s.replacementMissingIdentityCandidates(ctx, repositoryID)
	if err != nil {
		return err
	}
	return s.ReplaceFileSymbolsWithMissingCandidates(ctx, repositoryID, fileID, symbols, missingIdentities)
}

func (s *Store) ReplaceFileSymbolsWithMissingCandidates(ctx context.Context, repositoryID, fileID int64, symbols []Symbol, missingIdentities []storedSymbolIdentity) error {
	existingIdentities, err := s.replacementIdentityCandidatesWithMissing(ctx, repositoryID, fileID, missingIdentities)
	if err != nil {
		return err
	}
	usedIdentities := map[string]struct{}{}
	keep := make(map[string]struct{}, len(symbols))
	for _, sym := range symbols {
		keep[sym.StableKey] = struct{}{}
		identityKey := s.matchSymbolIdentity(sym, existingIdentities, usedIdentities)
		usedIdentities[identityKey] = struct{}{}
		now := nowString()
		_, err := s.execRaw(ctx, `
			INSERT INTO watch_symbols(repository_id, file_id, stable_key, name, qualified_name, kind, start_line, end_line, signature_hash, content_hash, raw_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(repository_id, stable_key) DO UPDATE SET
				file_id = excluded.file_id,
				name = excluded.name,
				qualified_name = excluded.qualified_name,
				kind = excluded.kind,
				start_line = excluded.start_line,
				end_line = excluded.end_line,
				signature_hash = excluded.signature_hash,
				content_hash = excluded.content_hash,
				raw_json = excluded.raw_json,
				updated_at = excluded.updated_at`,
			repositoryID, fileID, sym.StableKey, sym.Name, sym.QualifiedName, sym.Kind, sym.StartLine, sym.EndLine, sym.SignatureHash, sym.ContentHash, sym.RawJSON, now, now)
		if err != nil {
			return err
		}
		if err := s.UpsertSymbolIdentity(ctx, repositoryID, identityKey, sym); err != nil {
			return err
		}
	}
	rows, err := s.rowsRaw(ctx, `SELECT id, stable_key FROM watch_symbols WHERE repository_id = ? AND file_id = ?`, repositoryID, fileID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	var deleteIDs []int64
	var deleteStableKeys []string
	for rows.Next() {
		var id int64
		var stableKey string
		if err := rows.Scan(&id, &stableKey); err != nil {
			return err
		}
		if _, ok := keep[stableKey]; !ok {
			deleteIDs = append(deleteIDs, id)
			deleteStableKeys = append(deleteStableKeys, stableKey)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range deleteIDs {
		if _, err := s.execRaw(ctx, `DELETE FROM watch_symbols WHERE id = ?`, id); err != nil {
			return err
		}
	}
	for _, key := range deleteStableKeys {
		if _, err := s.execRaw(ctx, `DELETE FROM watch_symbol_identities WHERE repository_id = ? AND current_stable_key = ?`, repositoryID, key); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CachedFileByPath(ctx context.Context, repositoryID int64, path string) (File, bool, error) {
	return s.fileByPath(ctx, repositoryID, path)
}

func (s *Store) CachedFilesByPath(ctx context.Context, repositoryID int64) (map[string]File, error) {
	rows, err := s.rowsRaw(ctx, `
		SELECT id, repository_id, path, language, git_blob_hash, worktree_hash, size_bytes, mtime_unix, scan_status, scan_error, created_at, updated_at
		FROM watch_files
		WHERE repository_id = ?`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	files := make(map[string]File)
	for rows.Next() {
		var file File
		if err := rows.Scan(&file.ID, &file.RepositoryID, &file.Path, &file.Language, &file.GitBlobHash, &file.WorktreeHash, &file.SizeBytes, &file.MtimeUnix, &file.ScanStatus, &file.ScanError, &file.CreatedAt, &file.UpdatedAt); err != nil {
			return nil, err
		}
		files[file.Path] = file
	}
	return files, rows.Err()
}

func (s *Store) CurrentEnrichmentVersionPaths(ctx context.Context, repositoryID int64, version string) (map[string]struct{}, error) {
	rows, err := s.rowsRaw(ctx, `
		SELECT file_path
		FROM watch_facts
		WHERE repository_id = ?
		  AND enricher = ?
		  AND type = ?
		  AND name = ?`, repositoryID, enrichmentVersionEnricher, enrichmentVersionType, version)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	paths := make(map[string]struct{})
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths[path] = struct{}{}
	}
	return paths, rows.Err()
}

func (s *Store) symbolIdentitiesForFile(ctx context.Context, repositoryID, fileID int64) ([]storedSymbolIdentity, error) {
	rows, err := s.rowsRaw(ctx, `
		SELECT COALESCE(i.identity_key, ws.stable_key), ws.stable_key, f.path, ws.kind, ws.name, ws.qualified_name, ws.start_line, ws.content_hash
		FROM watch_symbols ws
		JOIN watch_files f ON f.id = ws.file_id
		LEFT JOIN watch_symbol_identities i ON i.repository_id = ws.repository_id AND i.current_stable_key = ws.stable_key
		WHERE ws.repository_id = ? AND ws.file_id = ?`, repositoryID, fileID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []storedSymbolIdentity
	for rows.Next() {
		var identity storedSymbolIdentity
		if err := rows.Scan(&identity.IdentityKey, &identity.StableKey, &identity.FilePath, &identity.Kind, &identity.Name, &identity.QualifiedName, &identity.StartLine, &identity.ContentHash); err != nil {
			return nil, err
		}
		out = append(out, identity)
	}
	return out, rows.Err()
}

func (s *Store) symbolIdentitiesForRepository(ctx context.Context, repositoryID int64) ([]storedSymbolIdentity, error) {
	rows, err := s.rowsRaw(ctx, `
		SELECT COALESCE(i.identity_key, ws.stable_key), ws.stable_key, f.path, ws.kind, ws.name, ws.qualified_name, ws.start_line, ws.content_hash
		FROM watch_symbols ws
		JOIN watch_files f ON f.id = ws.file_id
		LEFT JOIN watch_symbol_identities i ON i.repository_id = ws.repository_id AND i.current_stable_key = ws.stable_key
		WHERE ws.repository_id = ?`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []storedSymbolIdentity
	for rows.Next() {
		var identity storedSymbolIdentity
		if err := rows.Scan(&identity.IdentityKey, &identity.StableKey, &identity.FilePath, &identity.Kind, &identity.Name, &identity.QualifiedName, &identity.StartLine, &identity.ContentHash); err != nil {
			return nil, err
		}
		out = append(out, identity)
	}
	return out, rows.Err()
}

func (s *Store) replacementIdentityCandidatesWithMissing(ctx context.Context, repositoryID, fileID int64, missingIdentities []storedSymbolIdentity) ([]storedSymbolIdentity, error) {
	currentFile, err := s.symbolIdentitiesForFile(ctx, repositoryID, fileID)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	out := make([]storedSymbolIdentity, 0, len(currentFile))
	for _, identity := range currentFile {
		seen[identity.IdentityKey] = struct{}{}
		out = append(out, identity)
	}
	for _, identity := range missingIdentities {
		if _, ok := seen[identity.IdentityKey]; ok {
			continue
		}
		out = append(out, identity)
		seen[identity.IdentityKey] = struct{}{}
	}
	return out, nil
}

func (s *Store) replacementMissingIdentityCandidates(ctx context.Context, repositoryID int64) ([]storedSymbolIdentity, error) {
	repo, err := s.Repository(ctx, repositoryID)
	if err != nil || strings.TrimSpace(repo.RepoRoot) == "" {
		return nil, err
	}
	all, err := s.symbolIdentitiesForRepository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	out := make([]storedSymbolIdentity, 0)
	for _, identity := range all {
		if _, ok := seen[identity.IdentityKey]; ok {
			continue
		}
		if identity.FilePath == "" || !sourcePathMissing(repo.RepoRoot, identity.FilePath) {
			continue
		}
		identity.MissingFile = true
		out = append(out, identity)
		seen[identity.IdentityKey] = struct{}{}
	}
	return out, nil
}

func (s *Store) materializedOwnerFilePath(ctx context.Context, repositoryID int64, ownerType, ownerKey string) (string, bool, error) {
	switch ownerType {
	case "file":
		path := strings.TrimPrefix(ownerKey, "file:")
		if strings.TrimSpace(path) == "" {
			return "", false, nil
		}
		return filepathToSlash(path), true, nil
	case "symbol":
		var path string
		err := s.rowRaw(ctx, `
			SELECT file_path
			FROM watch_symbol_identities
			WHERE repository_id = ? AND (identity_key = ? OR current_stable_key = ?)
			ORDER BY updated_at DESC
			LIMIT 1`, repositoryID, ownerKey, ownerKey).Scan(&path)
		if err == nil && strings.TrimSpace(path) != "" {
			return filepathToSlash(path), true, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", false, err
		}
		if err == nil {
			slog.WarnContext(ctx, "symbol identity has empty file_path in database, falling back to stable key",
				"repository_id", repositoryID,
				"owner_key", ownerKey,
				"db_file_path", path)
		}
		if extractedPath, ok := filePathFromStableKey(ownerKey); ok {
			return extractedPath, true, nil
		}
		slog.WarnContext(ctx, "materialized owner file path not found: no DB entry and stable key contains no path",
			"repository_id", repositoryID,
			"owner_key", ownerKey,
			"db_err", err)
		return "", false, nil
	default:
		return "", false, nil
	}
}

func (s *Store) materializedOwnerMissing(ctx context.Context, repositoryID int64, ownerType, ownerKey string) (bool, error) {
	repo, err := s.Repository(ctx, repositoryID)
	if err != nil {
		return false, err
	}
	path, ok, err := s.materializedOwnerFilePath(ctx, repositoryID, ownerType, ownerKey)
	if err != nil || !ok {
		return false, err
	}
	return sourcePathMissing(repo.RepoRoot, path), nil
}

func (s *Store) deletedMaterializedElementIDs(ctx context.Context, repositoryID int64) (map[int64]struct{}, error) {
	rows, err := s.rowsRaw(ctx, `
		SELECT resource_id, owner_type, owner_key
		FROM watch_materialization
		WHERE repository_id = ? AND resource_type = 'element' AND owner_type IN ('file', 'symbol')`, repositoryID)
	if err != nil {
		return nil, err
	}
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
			_ = rows.Close()
			return nil, err
		}
		owners = append(owners, elementOwner{id: id, ownerType: ownerType, ownerKey: ownerKey})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	out := map[int64]struct{}{}
	for _, owner := range owners {
		missing, err := s.materializedOwnerMissing(ctx, repositoryID, owner.ownerType, owner.ownerKey)
		if err != nil {
			return nil, err
		}
		if missing {
			out[owner.id] = struct{}{}
		}
	}
	return out, nil
}

func (s *Store) connectorTouchesElements(ctx context.Context, connectorID int64, elementIDs map[int64]struct{}) (bool, error) {
	if len(elementIDs) == 0 {
		return false, nil
	}
	var sourceID, targetID int64
	err := s.rowRaw(ctx, `SELECT source_element_id, target_element_id FROM connectors WHERE id = ?`, connectorID).Scan(&sourceID, &targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, sourceDeleted := elementIDs[sourceID]
	_, targetDeleted := elementIDs[targetID]
	return sourceDeleted || targetDeleted, nil
}

func (s *Store) materializationMappingTombstoned(ctx context.Context, repositoryID int64, mapping watchMaterializationMapping, deletedElementIDs map[int64]struct{}) (bool, error) {
	switch mapping.ResourceType {
	case "element", "view":
		return s.materializedOwnerMissing(ctx, repositoryID, mapping.OwnerType, mapping.OwnerKey)
	case "connector":
		return s.connectorTouchesElements(ctx, mapping.ResourceID, deletedElementIDs)
	default:
		return false, nil
	}
}

func (s *Store) matchSymbolIdentity(sym Symbol, existing []storedSymbolIdentity, used map[string]struct{}) string {
	for _, identity := range existing {
		if identity.StableKey == sym.StableKey {
			return identity.IdentityKey
		}
	}
	bestScore := 0.0
	bestKey := ""
	for _, identity := range existing {
		if _, ok := used[identity.IdentityKey]; ok {
			continue
		}
		if identity.FilePath != sym.FilePath || identity.Kind != sym.Kind {
			continue
		}
		lineDelta := absInt(identity.StartLine - sym.StartLine)
		if lineDelta > 3 {
			continue
		}
		score := 0.35
		if lineDelta == 0 {
			score += 0.35
		} else {
			score += 0.2
		}
		if identity.ContentHash == sym.ContentHash {
			score += 0.2
		}
		if sameQualifierParent(identity.QualifiedName, sym.QualifiedName) {
			score += 0.1
		}
		if score > bestScore {
			bestScore = score
			bestKey = identity.IdentityKey
		}
	}
	for _, identity := range existing {
		if _, ok := used[identity.IdentityKey]; ok {
			continue
		}
		if !identity.MissingFile || identity.Kind != sym.Kind || identity.ContentHash == "" || identity.ContentHash != sym.ContentHash {
			continue
		}
		score := 0.80
		if sameQualifierParent(identity.QualifiedName, sym.QualifiedName) {
			score += 0.10
		}
		if nameTokenSimilarity(identity.QualifiedName, sym.QualifiedName) >= 0.50 {
			score += 0.05
		}
		lineDelta := absInt(identity.StartLine - sym.StartLine)
		if lineDelta <= 5 {
			score += 0.05
		}
		if score > bestScore {
			bestScore = score
			bestKey = identity.IdentityKey
		}
	}
	if bestScore >= 0.70 && bestKey != "" {
		return bestKey
	}
	return sym.StableKey
}

func (s *Store) UpsertSymbolIdentity(ctx context.Context, repositoryID int64, identityKey string, sym Symbol) error {
	now := nowString()
	_, err := s.execRaw(ctx, `
		INSERT INTO watch_symbol_identities(repository_id, identity_key, current_stable_key, file_path, kind, name, qualified_name, start_line, content_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repository_id, identity_key) DO UPDATE SET
			current_stable_key = excluded.current_stable_key,
			file_path = excluded.file_path,
			kind = excluded.kind,
			name = excluded.name,
			qualified_name = excluded.qualified_name,
			start_line = excluded.start_line,
			content_hash = excluded.content_hash,
			updated_at = excluded.updated_at`,
		repositoryID, identityKey, sym.StableKey, sym.FilePath, sym.Kind, sym.Name, sym.QualifiedName, sym.StartLine, sym.ContentHash, now, now)
	return err
}

func (s *Store) SymbolIdentityKeys(ctx context.Context, repositoryID int64) (map[string]string, error) {
	rows, err := s.rowsRaw(ctx, `SELECT current_stable_key, identity_key FROM watch_symbol_identities WHERE repository_id = ?`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var stableKey, identityKey string
		if err := rows.Scan(&stableKey, &identityKey); err != nil {
			return nil, err
		}
		out[stableKey] = identityKey
	}
	return out, rows.Err()
}

func (s *Store) ReplaceReferencesForFiles(ctx context.Context, repositoryID int64, fileIDs []int64, refs []Reference) error {
	for _, fileID := range fileIDs {
		if _, err := s.execRaw(ctx, `DELETE FROM watch_references WHERE repository_id = ? AND source_file_id = ?`, repositoryID, fileID); err != nil {
			return err
		}
	}
	for _, ref := range refs {
		now := nowString()
		_, err := s.execRaw(ctx, `
			INSERT INTO watch_references(repository_id, source_symbol_id, target_symbol_id, source_file_id, kind, line, "column", evidence_hash, raw_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(repository_id, source_symbol_id, target_symbol_id, kind, evidence_hash) DO UPDATE SET
				source_file_id = excluded.source_file_id,
				line = excluded.line,
				"column" = excluded."column",
				raw_json = excluded.raw_json,
				updated_at = excluded.updated_at`,
			repositoryID, ref.SourceSymbolID, ref.TargetSymbolID, ref.SourceFileID, ref.Kind, ref.Line, ref.Column, ref.EvidenceHash, ref.RawJSON, now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ReplaceFactsForFile(ctx context.Context, repositoryID, fileID int64, facts []Fact) error {
	if _, err := s.execRaw(ctx, `DELETE FROM watch_facts WHERE repository_id = ? AND file_id = ?`, repositoryID, fileID); err != nil {
		return err
	}
	for _, fact := range facts {
		now := nowString()
		tags, _ := json.Marshal(fact.Tags)
		if fact.AttributesJSON == "" {
			fact.AttributesJSON = "{}"
		}
		if fact.VisibilityHintsJSON == "" {
			fact.VisibilityHintsJSON = "{}"
		}
		if fact.RawJSON == "" {
			fact.RawJSON = "{}"
		}
		_, err := s.execRaw(ctx, `
			INSERT INTO watch_facts(repository_id, file_id, stable_key, type, enricher, subject_kind, subject_stable_key, object_kind, object_stable_key, object_file_path, object_name, relationship, file_path, start_line, end_line, confidence, name, tags, attributes_json, visibility_hints_json, fact_hash, raw_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(repository_id, enricher, stable_key) DO UPDATE SET
				file_id = excluded.file_id,
				type = excluded.type,
				subject_kind = excluded.subject_kind,
				subject_stable_key = excluded.subject_stable_key,
				object_kind = excluded.object_kind,
				object_stable_key = excluded.object_stable_key,
				object_file_path = excluded.object_file_path,
				object_name = excluded.object_name,
				relationship = excluded.relationship,
				file_path = excluded.file_path,
				start_line = excluded.start_line,
				end_line = excluded.end_line,
				confidence = excluded.confidence,
				name = excluded.name,
				tags = excluded.tags,
				attributes_json = excluded.attributes_json,
				visibility_hints_json = excluded.visibility_hints_json,
				fact_hash = excluded.fact_hash,
				raw_json = excluded.raw_json,
				updated_at = excluded.updated_at`,
			repositoryID, fileID, fact.StableKey, fact.Type, fact.Enricher, fact.SubjectKind, fact.SubjectStableKey, fact.ObjectKind, fact.ObjectStableKey, fact.ObjectFilePath, fact.ObjectName, fact.Relationship, fact.FilePath, fact.StartLine, fact.EndLine, fact.Confidence, fact.Name, string(tags), fact.AttributesJSON, fact.VisibilityHintsJSON, fact.FactHash, fact.RawJSON, now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) FactVersionForFile(ctx context.Context, repositoryID, fileID int64, enricher, stableKey string) (string, error) {
	var version string
	err := s.rowRaw(ctx, `
		SELECT name
		FROM watch_facts
		WHERE repository_id = ? AND file_id = ? AND enricher = ? AND stable_key = ?
		LIMIT 1`, repositoryID, fileID, enricher, stableKey).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return version, nil
}

func (s *Store) FactsForRepository(ctx context.Context, repositoryID int64) ([]Fact, error) {
	rows, err := s.rowsRaw(ctx, `
		SELECT id, repository_id, file_id, file_path, stable_key, type, enricher, subject_kind, subject_stable_key, object_kind, object_stable_key, object_file_path, object_name, relationship, start_line, end_line, confidence, name, tags, attributes_json, visibility_hints_json, fact_hash, raw_json, created_at, updated_at
		FROM watch_facts
		WHERE repository_id = ?
		ORDER BY file_path, type, enricher, stable_key`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var facts []Fact
	for rows.Next() {
		fact, err := scanFact(rows)
		if err != nil {
			return nil, err
		}
		facts = append(facts, fact)
	}
	return facts, rows.Err()
}

func (s *Store) SymbolsForRepository(ctx context.Context, repositoryID int64) ([]Symbol, error) {
	return s.QuerySymbols(ctx, repositoryID, SymbolQuery{Limit: -1})
}

func (s *Store) querySymbolsWhere(ctx context.Context, repositoryID int64, whereClause string, args ...any) ([]Symbol, error) {
	query := `
		SELECT s.id, s.repository_id, s.file_id, f.path, s.stable_key, s.name, s.qualified_name, s.kind, s.start_line, s.end_line, s.signature_hash, s.content_hash, s.raw_json, s.created_at, s.updated_at
		FROM watch_symbols s
		JOIN watch_files f ON f.id = s.file_id
		WHERE s.repository_id = ? AND ` + whereClause
	fullArgs := append([]any{repositoryID}, args...)
	rows, err := s.rowsRaw(ctx, query, fullArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Symbol
	for rows.Next() {
		var sym Symbol
		var endLine sql.NullInt64
		if err := rows.Scan(&sym.ID, &sym.RepositoryID, &sym.FileID, &sym.FilePath, &sym.StableKey, &sym.Name, &sym.QualifiedName, &sym.Kind, &sym.StartLine, &endLine, &sym.SignatureHash, &sym.ContentHash, &sym.RawJSON, &sym.CreatedAt, &sym.UpdatedAt); err != nil {
			return nil, err
		}
		if endLine.Valid {
			value := int(endLine.Int64)
			sym.EndLine = &value
		}
		hydrateSymbolNodeType(&sym)
		out = append(out, sym)
	}
	return out, rows.Err()
}

// QuerySymbolsByFiles returns symbols in the repository whose files match any of the given relative paths.
func (s *Store) QuerySymbolsByFiles(ctx context.Context, repositoryID int64, filePaths []string) ([]Symbol, error) {
	if len(filePaths) == 0 {
		return nil, nil
	}
	var allSymbols []Symbol
	for i := 0; i < len(filePaths); i += maxInClauseIDs {
		end := min(i+maxInClauseIDs, len(filePaths))
		batch := filePaths[i:end]
		placeholders, inArgs := buildParameterListStrings(batch)
		syms, err := s.querySymbolsWhere(ctx, repositoryID, "f.path IN ("+placeholders+")", inArgs...)
		if err != nil {
			return nil, err
		}
		allSymbols = append(allSymbols, syms...)
	}
	return allSymbols, nil
}

// QuerySymbolsByIDs returns symbols in the repository whose IDs match any of the given IDs.
func (s *Store) QuerySymbolsByIDs(ctx context.Context, repositoryID int64, ids []int64) ([]Symbol, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var allSymbols []Symbol
	for i := 0; i < len(ids); i += maxInClauseIDs {
		end := min(i+maxInClauseIDs, len(ids))
		batch := ids[i:end]
		placeholders, inArgs := buildParameterList(batch)
		syms, err := s.querySymbolsWhere(ctx, repositoryID, "s.id IN ("+placeholders+")", inArgs...)
		if err != nil {
			return nil, err
		}
		allSymbols = append(allSymbols, syms...)
	}
	return allSymbols, nil
}

func (s *Store) QuerySymbols(ctx context.Context, repositoryID int64, q SymbolQuery) ([]Symbol, error) {
	query := `
		SELECT s.id, s.repository_id, s.file_id, f.path, s.stable_key, s.name, s.qualified_name, s.kind, s.start_line, s.end_line, s.signature_hash, s.content_hash, s.raw_json, s.created_at, s.updated_at
		FROM watch_symbols s
		JOIN watch_files f ON f.id = s.file_id
		WHERE s.repository_id = ?`
	args := []any{repositoryID}
	if q.Search != "" {
		query += ` AND (s.name LIKE ? OR s.qualified_name LIKE ?)`
		needle := "%" + q.Search + "%"
		args = append(args, needle, needle)
	}
	if q.File != "" {
		query += ` AND f.path = ?`
		args = append(args, q.File)
	}
	if q.Kind != "" {
		query += ` AND s.kind = ?`
		args = append(args, q.Kind)
	}
	query += ` ORDER BY f.path, s.start_line, s.name`
	if q.Limit >= 0 {
		if q.Limit == 0 {
			q.Limit = 100
		}
		if q.Limit > 0 {
			query += ` LIMIT ? OFFSET ?`
			args = append(args, q.Limit, q.Offset)
		}
	}
	rows, err := s.rowsRaw(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Symbol
	for rows.Next() {
		var sym Symbol
		var endLine sql.NullInt64
		if err := rows.Scan(&sym.ID, &sym.RepositoryID, &sym.FileID, &sym.FilePath, &sym.StableKey, &sym.Name, &sym.QualifiedName, &sym.Kind, &sym.StartLine, &endLine, &sym.SignatureHash, &sym.ContentHash, &sym.RawJSON, &sym.CreatedAt, &sym.UpdatedAt); err != nil {
			return nil, err
		}
		if endLine.Valid {
			value := int(endLine.Int64)
			sym.EndLine = &value
		}
		hydrateSymbolNodeType(&sym)
		out = append(out, sym)
	}
	return out, rows.Err()
}

func hydrateSymbolNodeType(sym *Symbol) {
	if sym == nil || strings.TrimSpace(sym.NodeType) != "" || strings.TrimSpace(sym.RawJSON) == "" {
		return
	}
	var raw struct {
		NodeType string `json:"node_type"`
	}
	if err := json.Unmarshal([]byte(sym.RawJSON), &raw); err != nil {
		return
	}
	sym.NodeType = strings.TrimSpace(raw.NodeType)
}

func (s *Store) QueryReferences(ctx context.Context, repositoryID int64, q ReferenceQuery) ([]Reference, error) {
	query := `
		SELECT id, repository_id, source_symbol_id, target_symbol_id, source_file_id, kind, line, "column", evidence_hash, raw_json, created_at, updated_at
		FROM watch_references
		WHERE repository_id = ?`
	args := []any{repositoryID}
	if q.SymbolID > 0 {
		query += ` AND (source_symbol_id = ? OR target_symbol_id = ?)`
		args = append(args, q.SymbolID, q.SymbolID)
	}
	query += ` ORDER BY source_file_id, line, "column"`
	if q.Limit == 0 {
		q.Limit = 100
	}
	query += ` LIMIT ? OFFSET ?`
	args = append(args, q.Limit, q.Offset)
	rows, err := s.rowsRaw(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Reference
	for rows.Next() {
		var ref Reference
		if err := rows.Scan(&ref.ID, &ref.RepositoryID, &ref.SourceSymbolID, &ref.TargetSymbolID, &ref.SourceFileID, &ref.Kind, &ref.Line, &ref.Column, &ref.EvidenceHash, &ref.RawJSON, &ref.CreatedAt, &ref.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

func (s *Store) queryReferencesWhere(ctx context.Context, repositoryID int64, whereClause string, whereArgs ...any) ([]Reference, error) {
	query := `SELECT id, repository_id, source_symbol_id, target_symbol_id, source_file_id, kind, line, "column", evidence_hash, raw_json, created_at, updated_at FROM watch_references WHERE repository_id = ? AND ` + whereClause + ` ORDER BY source_file_id, line, "column"`
	args := append([]any{repositoryID}, whereArgs...)
	rows, err := s.rowsRaw(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanReferences(rows)
}

// BuildDegreeMaps computes incoming and outgoing reference counts via SQL GROUP BY.
func (s *Store) BuildDegreeMaps(ctx context.Context, repositoryID int64) (incoming, outgoing map[int64]int, err error) {
	incoming = make(map[int64]int)
	outgoing = make(map[int64]int)

	rows, err := s.rowsRaw(ctx,
		"SELECT source_symbol_id, COUNT(*) FROM watch_references WHERE repository_id = ? GROUP BY source_symbol_id",
		repositoryID)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var cnt int
		if err := rows.Scan(&id, &cnt); err != nil {
			return nil, nil, err
		}
		outgoing[id] = cnt
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	rows2, err := s.rowsRaw(ctx,
		"SELECT target_symbol_id, COUNT(*) FROM watch_references WHERE repository_id = ? GROUP BY target_symbol_id",
		repositoryID)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows2.Close() }()
	for rows2.Next() {
		var id int64
		var cnt int
		if err := rows2.Scan(&id, &cnt); err != nil {
			return nil, nil, err
		}
		incoming[id] = cnt
	}
	if err := rows2.Err(); err != nil {
		return nil, nil, err
	}

	return incoming, outgoing, nil
}

// QueryReferencesBySourceIDs returns references where source_symbol_id matches any of the given IDs.
func (s *Store) QueryReferencesBySourceIDs(ctx context.Context, repositoryID int64, sourceIDs []int64) ([]Reference, error) {
	if len(sourceIDs) == 0 {
		return nil, nil
	}
	var allRefs []Reference
	for i := 0; i < len(sourceIDs); i += maxInClauseIDs {
		end := min(i+maxInClauseIDs, len(sourceIDs))
		batch := sourceIDs[i:end]
		placeholders, inArgs := buildParameterList(batch)
		refs, err := s.queryReferencesWhere(ctx, repositoryID, "source_symbol_id IN ("+placeholders+")", inArgs...)
		if err != nil {
			return nil, err
		}
		allRefs = append(allRefs, refs...)
	}
	return allRefs, nil
}

// QueryReferencesByTargetIDs returns references where target_symbol_id matches any of the given IDs.
func (s *Store) QueryReferencesByTargetIDs(ctx context.Context, repositoryID int64, targetIDs []int64) ([]Reference, error) {
	if len(targetIDs) == 0 {
		return nil, nil
	}
	var allRefs []Reference
	for i := 0; i < len(targetIDs); i += maxInClauseIDs {
		end := min(i+maxInClauseIDs, len(targetIDs))
		batch := targetIDs[i:end]
		placeholders, inArgs := buildParameterList(batch)
		refs, err := s.queryReferencesWhere(ctx, repositoryID, "target_symbol_id IN ("+placeholders+")", inArgs...)
		if err != nil {
			return nil, err
		}
		allRefs = append(allRefs, refs...)
	}
	return allRefs, nil
}

func (s *Store) WatchResourceHash(ctx context.Context, resourceType string, resourceID int64) (string, bool, error) {
	switch resourceType {
	case "element":
		return s.watchElementHash(ctx, resourceID)
	case "connector":
		return s.watchConnectorHash(ctx, resourceID)
	case "view":
		return s.watchViewHash(ctx, resourceID)
	default:
		return "", false, fmt.Errorf("unsupported watch resource type %q", resourceType)
	}
}

func (s *Store) watchElementHash(ctx context.Context, id int64) (string, bool, error) {
	var kind, description, technology, repo, branch, filePath, language sql.NullString
	var name, techLinks, tags string
	err := s.rowRaw(ctx, `
		SELECT name, kind, description, technology, technology_connectors, tags, repo, branch, file_path, language
		FROM elements WHERE id = ?`, id).Scan(&name, &kind, &description, &technology, &techLinks, &tags, &repo, &branch, &filePath, &language)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	payload := map[string]any{
		"name":                  name,
		"kind":                  nullableString(kind),
		"description":           nullableString(description),
		"technology":            nullableString(technology),
		"technology_connectors": normalizedJSONValue(techLinks),
		"tags":                  normalizedElementTagsForHash(tags),
		"repo":                  nullableString(repo),
		"branch":                nullableString(branch),
		"file_path":             nullableString(filePath),
		"language":              nullableString(language),
	}
	return hashCanonicalPayload(payload), true, nil
}

func (s *Store) watchConnectorHash(ctx context.Context, id int64) (string, bool, error) {
	var label, relationship sql.NullString
	var viewID, sourceID, targetID int64
	var direction, style string
	err := s.rowRaw(ctx, `
		SELECT view_id, source_element_id, target_element_id, label, relationship, direction, style
		FROM connectors WHERE id = ?`, id).Scan(&viewID, &sourceID, &targetID, &label, &relationship, &direction, &style)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	payload := map[string]any{
		"view_id":           viewID,
		"source_element_id": sourceID,
		"target_element_id": targetID,
		"label":             nullableString(label),
		"relationship":      nullableString(relationship),
		"direction":         direction,
		"style":             style,
	}
	return hashCanonicalPayload(payload), true, nil
}

func (s *Store) watchViewHash(ctx context.Context, id int64) (string, bool, error) {
	var ownerID sql.NullInt64
	var levelLabel sql.NullString
	var name string
	err := s.rowRaw(ctx, `
		SELECT owner_element_id, name, level_label
		FROM views WHERE id = ?`, id).Scan(&ownerID, &name, &levelLabel)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	var owner any
	if ownerID.Valid {
		owner = ownerID.Int64
	}
	payload := map[string]any{
		"owner_element_id": owner,
		"name":             name,
		"level_label":      nullableString(levelLabel),
	}
	return hashCanonicalPayload(payload), true, nil
}

func (s *Store) CreateWatchVersion(ctx context.Context, repositoryID int64, commitHash, commitMessage, parentCommitHash, branch, representationHash string, workspaceVersionID *int64, diffs []RepresentationDiff) (Version, error) {
	now := nowString()
	_, err := s.execRaw(ctx, `
		INSERT INTO watch_versions(repository_id, commit_hash, commit_message, parent_commit_hash, branch, representation_hash, workspace_version_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repository_id, commit_hash, representation_hash) DO NOTHING`,
		repositoryID, commitHash, nullString(commitMessage), nullString(parentCommitHash), nullString(branch), representationHash, workspaceVersionID, now)
	if err != nil {
		return Version{}, err
	}
	version, err := s.WatchVersion(ctx, repositoryID, commitHash, representationHash)
	if err != nil {
		return Version{}, err
	}
	for _, diff := range diffs {
		_, err := s.execRaw(ctx, `
			INSERT INTO watch_representation_diffs(version_id, owner_type, owner_key, change_type, before_hash, after_hash, resource_type, resource_id, summary, added_lines, removed_lines)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			version.ID, diff.OwnerType, diff.OwnerKey, diff.ChangeType, diff.BeforeHash, diff.AfterHash, diff.ResourceType, diff.ResourceID, diff.Summary, diff.AddedLines, diff.RemovedLines)
		if err != nil {
			return Version{}, err
		}
	}
	if err := s.SaveWatchVersionResources(ctx, version.ID, repositoryID); err != nil {
		return Version{}, err
	}
	if err := s.PruneWatchVersions(ctx, repositoryID, 5); err != nil {
		return Version{}, err
	}
	return version, nil
}

func (s *Store) PruneWatchVersions(ctx context.Context, repositoryID int64, keep int) error {
	if keep <= 0 {
		keep = 5
	}
	_, err := s.execRaw(ctx, `
		DELETE FROM watch_versions
		WHERE repository_id = ?
		  AND id NOT IN (
			SELECT id
			FROM watch_versions
			WHERE repository_id = ?
			ORDER BY id DESC
			LIMIT ?
		  )`, repositoryID, repositoryID, keep)
	return err
}

func (s *Store) WatchVersion(ctx context.Context, repositoryID int64, commitHash, representationHash string) (Version, error) {
	row := s.rowRaw(ctx, `
		SELECT id, repository_id, commit_hash, commit_message, parent_commit_hash, branch, representation_hash, workspace_version_id, created_at
		FROM watch_versions
		WHERE repository_id = ? AND commit_hash = ? AND representation_hash = ?`, repositoryID, commitHash, representationHash)
	return scanVersion(row)
}

func (s *Store) WatchVersions(ctx context.Context, repositoryID int64, limit int) ([]Version, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.rowsRaw(ctx, `
		SELECT id, repository_id, commit_hash, commit_message, parent_commit_hash, branch, representation_hash, workspace_version_id, created_at
		FROM watch_versions
		WHERE repository_id = ?
		ORDER BY id DESC
		LIMIT ?`, repositoryID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Version
	for rows.Next() {
		version, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, version)
	}
	return out, rows.Err()
}

func (s *Store) LatestWatchVersion(ctx context.Context, repositoryID int64) (Version, bool, error) {
	row := s.rowRaw(ctx, `
		SELECT id, repository_id, commit_hash, commit_message, parent_commit_hash, branch, representation_hash, workspace_version_id, created_at
		FROM watch_versions
		WHERE repository_id = ?
		ORDER BY id DESC
		LIMIT 1`, repositoryID)
	version, err := scanVersion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Version{}, false, nil
	}
	return version, err == nil, err
}

func (s *Store) WorkspaceResourceCounts(ctx context.Context) (views, elements, connectors int, err error) {
	for query, dest := range map[string]*int{
		`SELECT COUNT(*) FROM views`:      &views,
		`SELECT COUNT(*) FROM elements`:   &elements,
		`SELECT COUNT(*) FROM connectors`: &connectors,
	} {
		if scanErr := s.rowRaw(ctx, query).Scan(dest); scanErr != nil {
			return 0, 0, 0, scanErr
		}
	}
	return views, elements, connectors, nil
}

func (s *Store) CreateWorkspaceVersion(ctx context.Context, versionID, source string, parentID *int64, viewCount, elementCount, connectorCount int, description, workspaceHash *string) (int64, error) {
	return s.insertReturningID(ctx, `
		INSERT INTO workspace_versions(version_id, source, parent_version_id, view_count, element_count, connector_count, description, workspace_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		versionID, source, parentID, viewCount, elementCount, connectorCount, description, workspaceHash, nowString())
}

func (s *Store) WatchDiffs(ctx context.Context, versionID int64, ownerType, changeType, resourceType, language string, limit int) ([]RepresentationDiff, error) {
	if limit <= 0 {
		limit = 200
	}
	query := `
		SELECT id, version_id, owner_type, owner_key, change_type, before_hash, after_hash, resource_type, resource_id, summary, added_lines, removed_lines
		FROM watch_representation_diffs
		WHERE version_id = ?`
	args := []any{versionID}
	if ownerType != "" {
		query += ` AND owner_type = ?`
		args = append(args, ownerType)
	}
	if changeType != "" {
		query += ` AND change_type = ?`
		args = append(args, changeType)
	}
	if resourceType != "" {
		query += ` AND resource_type = ?`
		args = append(args, resourceType)
	}
	query += ` ORDER BY id LIMIT ?`
	args = append(args, limit)
	rows, err := s.rowsRaw(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []RepresentationDiff
	for rows.Next() {
		var diff RepresentationDiff
		var before, after, resourceType, summary sql.NullString
		var resourceID sql.NullInt64
		if err := rows.Scan(&diff.ID, &diff.VersionID, &diff.OwnerType, &diff.OwnerKey, &diff.ChangeType, &before, &after, &resourceType, &resourceID, &summary, &diff.AddedLines, &diff.RemovedLines); err != nil {
			return nil, err
		}
		diff.BeforeHash = nullStringPtr(before)
		diff.AfterHash = nullStringPtr(after)
		diff.ResourceType = nullStringPtr(resourceType)
		if resourceID.Valid {
			diff.ResourceID = &resourceID.Int64
		}
		if lang := diffLanguage(diff); lang != "" {
			diff.Language = &lang
		}
		diff.Summary = nullStringPtr(summary)
		if language == "" || (diff.Language != nil && *diff.Language == language) {
			out = append(out, diff)
		}
	}
	return out, rows.Err()
}
