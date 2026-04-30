package watch

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/viant/sqlite-vec/vector"
)

const LockHeartbeatTimeout = 30 * time.Second

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

type RepositoryInput struct {
	RemoteURL      string
	RepoRoot       string
	DisplayName    string
	Branch         string
	HeadCommit     string
	IdentityStatus string
	SettingsHash   string
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

	var existingID int64
	var err error
	if input.RemoteURL != "" {
		err = s.db.QueryRowContext(ctx, `SELECT id FROM watch_repositories WHERE remote_url = ?`, input.RemoteURL).Scan(&existingID)
	} else {
		err = s.db.QueryRowContext(ctx, `SELECT id FROM watch_repositories WHERE repo_root = ? AND identity_status = 'local_only'`, input.RepoRoot).Scan(&existingID)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Repository{}, err
	}
	if existingID > 0 {
		_, err = s.db.ExecContext(ctx, `
			UPDATE watch_repositories
			SET repo_root = ?, display_name = ?, branch = ?, head_commit = ?, identity_status = ?, settings_hash = ?, updated_at = ?
			WHERE id = ?`,
			input.RepoRoot,
			input.DisplayName,
			nullString(input.Branch),
			nullString(input.HeadCommit),
			input.IdentityStatus,
			input.SettingsHash,
			now,
			existingID,
		)
		if err != nil {
			return Repository{}, err
		}
		return s.Repository(ctx, existingID)
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO watch_repositories(remote_url, repo_root, display_name, branch, head_commit, identity_status, settings_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nullString(input.RemoteURL),
		input.RepoRoot,
		input.DisplayName,
		nullString(input.Branch),
		nullString(input.HeadCommit),
		input.IdentityStatus,
		input.SettingsHash,
		now,
		now,
	)
	if err != nil {
		return Repository{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Repository{}, err
	}
	return s.Repository(ctx, id)
}

func (s *Store) Repository(ctx context.Context, id int64) (Repository, error) {
	var repo Repository
	err := s.db.QueryRowContext(ctx, `
		SELECT id, remote_url, repo_root, display_name, branch, head_commit, identity_status, settings_hash, created_at, updated_at
		FROM watch_repositories
		WHERE id = ?`, id).Scan(
		&repo.ID,
		&repo.RemoteURL,
		&repo.RepoRoot,
		&repo.DisplayName,
		&repo.Branch,
		&repo.HeadCommit,
		&repo.IdentityStatus,
		&repo.SettingsHash,
		&repo.CreatedAt,
		&repo.UpdatedAt,
	)
	return repo, err
}

func (s *Store) Repositories(ctx context.Context) ([]Repository, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, remote_url, repo_root, display_name, branch, head_commit, identity_status, settings_hash, created_at, updated_at
		FROM watch_repositories
		ORDER BY display_name, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var repos []Repository
	for rows.Next() {
		var repo Repository
		if err := rows.Scan(&repo.ID, &repo.RemoteURL, &repo.RepoRoot, &repo.DisplayName, &repo.Branch, &repo.HeadCommit, &repo.IdentityStatus, &repo.SettingsHash, &repo.CreatedAt, &repo.UpdatedAt); err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}
	return repos, rows.Err()
}

func (s *Store) ReassociateRepository(ctx context.Context, id int64, remoteURL string) (Repository, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return Repository{}, fmt.Errorf("remote_url is required")
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE watch_repositories
		SET remote_url = ?, identity_status = 'known', updated_at = ?
		WHERE id = ?`, remoteURL, nowString(), id)
	if err != nil {
		return Repository{}, err
	}
	return s.Repository(ctx, id)
}

func (s *Store) BeginScanRun(ctx context.Context, repositoryID int64, mode string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO watch_scan_runs(repository_id, mode, started_at, status)
		VALUES (?, ?, ?, 'running')`, repositoryID, mode, nowString())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) FinishScanRun(ctx context.Context, id int64, status string, result ScanResult, runErr error) error {
	var errText any
	if runErr != nil {
		errText = runErr.Error()
	} else if result.Warning != "" {
		errText = result.Warning
	}
	_, err := s.db.ExecContext(ctx, `
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
		_, err := s.db.ExecContext(ctx, `
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
	_, err = s.db.ExecContext(ctx, `
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
	rows, err := s.db.QueryContext(ctx, `SELECT id, path FROM watch_files WHERE repository_id = ?`, repositoryID)
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
		if _, err := s.db.ExecContext(ctx, `DELETE FROM watch_files WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ReplaceFileSymbols(ctx context.Context, repositoryID, fileID int64, symbols []Symbol) error {
	existingIdentities, err := s.replacementIdentityCandidates(ctx, repositoryID, fileID)
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
		_, err := s.db.ExecContext(ctx, `
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
	rows, err := s.db.QueryContext(ctx, `SELECT id, stable_key FROM watch_symbols WHERE repository_id = ? AND file_id = ?`, repositoryID, fileID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	var deleteIDs []int64
	for rows.Next() {
		var id int64
		var stableKey string
		if err := rows.Scan(&id, &stableKey); err != nil {
			return err
		}
		if _, ok := keep[stableKey]; !ok {
			deleteIDs = append(deleteIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range deleteIDs {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM watch_symbols WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CachedFileByPath(ctx context.Context, repositoryID int64, path string) (File, bool, error) {
	return s.fileByPath(ctx, repositoryID, path)
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

func (s *Store) symbolIdentitiesForFile(ctx context.Context, repositoryID, fileID int64) ([]storedSymbolIdentity, error) {
	rows, err := s.db.QueryContext(ctx, `
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
	rows, err := s.db.QueryContext(ctx, `
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

func (s *Store) replacementIdentityCandidates(ctx context.Context, repositoryID, fileID int64) ([]storedSymbolIdentity, error) {
	currentFile, err := s.symbolIdentitiesForFile(ctx, repositoryID, fileID)
	if err != nil {
		return nil, err
	}
	repo, err := s.Repository(ctx, repositoryID)
	if err != nil || strings.TrimSpace(repo.RepoRoot) == "" {
		return currentFile, err
	}
	all, err := s.symbolIdentitiesForRepository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	out := make([]storedSymbolIdentity, 0, len(currentFile))
	for _, identity := range currentFile {
		seen[identity.IdentityKey] = struct{}{}
		out = append(out, identity)
	}
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

func sourcePathMissing(repoRoot, relPath string) bool {
	cleanRel := filepath.Clean(filepath.FromSlash(relPath))
	if filepath.IsAbs(cleanRel) || cleanRel == "." || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return false
	}
	_, err := os.Stat(filepath.Join(repoRoot, cleanRel))
	return errors.Is(err, os.ErrNotExist)
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
	_, err := s.db.ExecContext(ctx, `
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
	rows, err := s.db.QueryContext(ctx, `SELECT current_stable_key, identity_key FROM watch_symbol_identities WHERE repository_id = ?`, repositoryID)
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
		if _, err := s.db.ExecContext(ctx, `DELETE FROM watch_references WHERE repository_id = ? AND source_file_id = ?`, repositoryID, fileID); err != nil {
			return err
		}
	}
	for _, ref := range refs {
		now := nowString()
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO watch_references(repository_id, source_symbol_id, target_symbol_id, source_file_id, kind, line, column, evidence_hash, raw_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(repository_id, source_symbol_id, target_symbol_id, kind, evidence_hash) DO UPDATE SET
				source_file_id = excluded.source_file_id,
				line = excluded.line,
				column = excluded.column,
				raw_json = excluded.raw_json,
				updated_at = excluded.updated_at`,
			repositoryID, ref.SourceSymbolID, ref.TargetSymbolID, ref.SourceFileID, ref.Kind, ref.Line, ref.Column, ref.EvidenceHash, ref.RawJSON, now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SymbolsForRepository(ctx context.Context, repositoryID int64) ([]Symbol, error) {
	return s.QuerySymbols(ctx, repositoryID, SymbolQuery{Limit: -1})
}

type SymbolQuery struct {
	Search string
	File   string
	Kind   string
	Limit  int
	Offset int
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
		query += ` LIMIT ? OFFSET ?`
		args = append(args, q.Limit, q.Offset)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
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
		out = append(out, sym)
	}
	return out, rows.Err()
}

type ReferenceQuery struct {
	SymbolID int64
	Limit    int
	Offset   int
}

func (s *Store) QueryReferences(ctx context.Context, repositoryID int64, q ReferenceQuery) ([]Reference, error) {
	query := `
		SELECT id, repository_id, source_symbol_id, target_symbol_id, source_file_id, kind, line, column, evidence_hash, raw_json, created_at, updated_at
		FROM watch_references
		WHERE repository_id = ?`
	args := []any{repositoryID}
	if q.SymbolID > 0 {
		query += ` AND (source_symbol_id = ? OR target_symbol_id = ?)`
		args = append(args, q.SymbolID, q.SymbolID)
	}
	query += ` ORDER BY source_file_id, line, column`
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
	return s.similarEmbeddingsFallback(ctx, modelID, query, limit)
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
	return nil
}

func (s *Store) BeginFilterRun(ctx context.Context, repositoryID int64, settingsHash, rawGraphHash string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO watch_filter_runs(repository_id, settings_hash, raw_graph_hash, started_at, status)
		VALUES (?, ?, ?, ?, 'running')`, repositoryID, settingsHash, rawGraphHash, nowString())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) SaveFilterDecision(ctx context.Context, filterRunID int64, ownerType string, ownerID int64, decision, reason string, score *float64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO watch_filter_decisions(filter_run_id, owner_type, owner_id, decision, reason, score)
		VALUES (?, ?, ?, ?, ?, ?)`, filterRunID, ownerType, ownerID, decision, reason, score)
	return err
}

func (s *Store) FinishFilterRun(ctx context.Context, id int64, status string, visibleSymbols, hiddenSymbols, visibleReferences, hiddenReferences int) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE watch_filter_runs
		SET finished_at = ?, status = ?, visible_symbols = ?, hidden_symbols = ?, visible_references = ?, hidden_references = ?
		WHERE id = ?`,
		nowString(), status, visibleSymbols, hiddenSymbols, visibleReferences, hiddenReferences, id)
	return err
}

func (s *Store) UpsertCluster(ctx context.Context, repositoryID int64, stableKey string, parentClusterID *int64, name, kind, algorithm, settingsHash string, memberIDs []int64) (Cluster, error) {
	now := nowString()
	_, err := s.db.ExecContext(ctx, `
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
	if _, err := s.db.ExecContext(ctx, `DELETE FROM watch_cluster_members WHERE cluster_id = ?`, cluster.ID); err != nil {
		return Cluster{}, err
	}
	for _, memberID := range memberIDs {
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO watch_cluster_members(cluster_id, owner_type, owner_id)
			VALUES (?, 'symbol', ?)`, cluster.ID, memberID); err != nil {
			return Cluster{}, err
		}
	}
	return cluster, nil
}

func (s *Store) Clusters(ctx context.Context, repositoryID int64) ([]Cluster, error) {
	rows, err := s.db.QueryContext(ctx, `
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
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO watch_representation_runs(repository_id, raw_graph_hash, filter_settings_hash, embedding_model_id, representation_hash, started_at, status)
		VALUES (?, ?, ?, ?, ?, ?, 'running')`,
		repositoryID, rawGraphHash, settingsHash, embeddingModelID, representationHash, nowString())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) FinishRepresentationRun(ctx context.Context, id int64, status string, result RepresentResult, runErr error) error {
	var errText any
	if runErr != nil {
		errText = runErr.Error()
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE watch_representation_runs
		SET finished_at = ?, status = ?, elements_created = ?, elements_updated = ?, connectors_created = ?, connectors_updated = ?, views_created = ?, error = ?
		WHERE id = ?`,
		nowString(), status, result.ElementsCreated, result.ElementsUpdated, result.ConnectorsCreated, result.ConnectorsUpdated, result.ViewsCreated, errText, id)
	return err
}

func (s *Store) MappingResourceID(ctx context.Context, repositoryID int64, ownerType, ownerKey, resourceType string) (int64, bool, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `
		SELECT resource_id FROM watch_materialization
		WHERE repository_id = ? AND owner_type = ? AND owner_key = ? AND resource_type = ?`,
		repositoryID, ownerType, ownerKey, resourceType).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil, err
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

func (s *Store) Materialization(ctx context.Context, repositoryID int64) ([]MaterializationMapping, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, repository_id, owner_type, owner_key, resource_type, resource_id, created_at, updated_at
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
		if err := rows.Scan(&item.ID, &item.RepositoryID, &item.OwnerType, &item.OwnerKey, &item.ResourceType, &item.ResourceID, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) RepositoryMaterializationCount(ctx context.Context, repositoryID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM watch_materialization WHERE repository_id = ?`, repositoryID).Scan(&count)
	return count, err
}

type FilterDecisionQuery struct {
	OwnerType string
	Decision  string
	Limit     int
	Offset    int
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
		SELECT id, filter_run_id, owner_type, owner_id, decision, reason, score
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
		if err := rows.Scan(&item.ID, &item.FilterRunID, &item.OwnerType, &item.OwnerID, &item.Decision, &item.Reason, &score); err != nil {
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
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *Store) AcquireLock(ctx context.Context, repositoryID int64, pid int, token string, staleAfter time.Duration) (Lock, error) {
	if staleAfter <= 0 {
		staleAfter = LockHeartbeatTimeout
	}
	now := nowString()
	cutoff := time.Now().UTC().Add(-staleAfter).Format(time.RFC3339)
	_, _ = s.db.ExecContext(ctx, `UPDATE watch_locks SET status = 'stale' WHERE status IN ('active', 'paused', 'stopping') AND heartbeat_at < ?`, cutoff)
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

func (s *Store) ActiveLock(ctx context.Context) (Lock, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, repository_id, pid, token, started_at, heartbeat_at, status
		FROM watch_locks
		WHERE status IN ('active', 'paused', 'stopping')
		ORDER BY id
		LIMIT 1`)
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
	if err != nil || time.Since(heartbeat) > staleAfter || lock.Status == "stale" || lock.Status == "released" {
		return lock, false, nil
	}
	return lock, true, nil
}

func (s *Store) HeartbeatLock(ctx context.Context, repositoryID int64, token string) (Lock, error) {
	_, err := s.db.ExecContext(ctx, `
		UPDATE watch_locks
		SET heartbeat_at = ?
		WHERE repository_id = ? AND token = ? AND status IN ('active', 'paused')`,
		nowString(), repositoryID, token)
	if err != nil {
		return Lock{}, err
	}
	return s.ActiveLock(ctx)
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

func (s *Store) EnsureGitTags(ctx context.Context) error {
	tags := map[string]string{
		"git:staged":    "Git staged change",
		"git:unstaged":  "Git unstaged change",
		"git:untracked": "Git untracked file",
		"watch:new":     "Introduced since the previous representation version",
		"watch:updated": "Updated since the previous representation version",
		"watch:deleted": "Backing symbol disappeared during watch",
	}
	for name, description := range tags {
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO tags(name, color, description) VALUES (?, '#0f766e', ?)
			ON CONFLICT(name) DO UPDATE SET description = excluded.description`, name, description); err != nil {
			return err
		}
	}
	return nil
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
		SELECT resource_id, owner_key
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
	for rows.Next() {
		var id int64
		var ownerKey string
		if err := rows.Scan(&id, &ownerKey); err != nil {
			return GitTagUpdateResult{}, err
		}
		allElementIDs = append(allElementIDs, id)
		file := strings.TrimPrefix(ownerKey, "file:")
		if strings.HasPrefix(ownerKey, "go:") {
			parts := strings.Split(ownerKey, ":")
			if len(parts) >= 2 {
				file = parts[1]
			}
		}
		if tags := files[file]; len(tags) > 0 {
			updates = append(updates, update{id: id, tags: tags})
		}
	}
	if err := rows.Err(); err != nil {
		return GitTagUpdateResult{}, err
	}
	var result GitTagUpdateResult
	for _, id := range allElementIDs {
		removed, err := s.removeElementTags(ctx, id, managedGitTags())
		if err != nil {
			return GitTagUpdateResult{}, err
		}
		result.TagsRemoved += removed
	}
	for _, item := range updates {
		added, err := s.addElementTags(ctx, item.id, item.tags)
		if err != nil {
			return GitTagUpdateResult{}, err
		}
		result.TagsAdded += added
	}
	return result, nil
}

func (s *Store) CreateWatchVersion(ctx context.Context, repositoryID int64, commitHash, parentCommitHash, branch, representationHash string, workspaceVersionID *int64, diffs []RepresentationDiff) (Version, error) {
	now := nowString()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO watch_versions(repository_id, commit_hash, parent_commit_hash, branch, representation_hash, workspace_version_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repository_id, commit_hash, representation_hash) DO NOTHING`,
		repositoryID, commitHash, nullString(parentCommitHash), nullString(branch), representationHash, workspaceVersionID, now)
	if err != nil {
		return Version{}, err
	}
	version, err := s.WatchVersion(ctx, repositoryID, commitHash, representationHash)
	if err != nil {
		return Version{}, err
	}
	for _, diff := range diffs {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO watch_representation_diffs(version_id, owner_type, owner_key, change_type, before_hash, after_hash, resource_type, resource_id, summary)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			version.ID, diff.OwnerType, diff.OwnerKey, diff.ChangeType, diff.BeforeHash, diff.AfterHash, diff.ResourceType, diff.ResourceID, diff.Summary)
		if err != nil {
			return Version{}, err
		}
	}
	if err := s.SaveWatchVersionResources(ctx, version.ID, repositoryID); err != nil {
		return Version{}, err
	}
	return version, nil
}

func (s *Store) WatchVersion(ctx context.Context, repositoryID int64, commitHash, representationHash string) (Version, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, repository_id, commit_hash, parent_commit_hash, branch, representation_hash, workspace_version_id, created_at
		FROM watch_versions
		WHERE repository_id = ? AND commit_hash = ? AND representation_hash = ?`, repositoryID, commitHash, representationHash)
	return scanVersion(row)
}

func (s *Store) WatchVersions(ctx context.Context, repositoryID int64, limit int) ([]Version, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, repository_id, commit_hash, parent_commit_hash, branch, representation_hash, workspace_version_id, created_at
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
	row := s.db.QueryRowContext(ctx, `
		SELECT id, repository_id, commit_hash, parent_commit_hash, branch, representation_hash, workspace_version_id, created_at
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
		if scanErr := s.db.QueryRowContext(ctx, query).Scan(dest); scanErr != nil {
			return 0, 0, 0, scanErr
		}
	}
	return views, elements, connectors, nil
}

func (s *Store) CreateWorkspaceVersion(ctx context.Context, versionID, source string, parentID *int64, viewCount, elementCount, connectorCount int, description, workspaceHash *string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO workspace_versions(version_id, source, parent_version_id, view_count, element_count, connector_count, description, workspace_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		versionID, source, parentID, viewCount, elementCount, connectorCount, description, workspaceHash, nowString())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) WatchDiffs(ctx context.Context, versionID int64, ownerType, changeType, resourceType, language string, limit int) ([]RepresentationDiff, error) {
	if limit <= 0 {
		limit = 200
	}
	query := `
		SELECT id, version_id, owner_type, owner_key, change_type, before_hash, after_hash, resource_type, resource_id, summary
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
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []RepresentationDiff
	for rows.Next() {
		var diff RepresentationDiff
		var before, after, resourceType, summary sql.NullString
		var resourceID sql.NullInt64
		if err := rows.Scan(&diff.ID, &diff.VersionID, &diff.OwnerType, &diff.OwnerKey, &diff.ChangeType, &before, &after, &resourceType, &resourceID, &summary); err != nil {
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

type watchResourceSnapshot struct {
	OwnerType    string
	OwnerKey     string
	ResourceType string
	ResourceID   *int64
	Language     string
	Hash         string
	Summary      string
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
	var diffs []RepresentationDiff
	repoKey := fmt.Sprintf("%d", repositoryID)
	repoSummary := "Representation added"
	change := "added"
	if found {
		change = "updated"
		repoSummary = "Representation updated"
	}
	diffs = append(diffs, RepresentationDiff{OwnerType: "repository", OwnerKey: repoKey, ChangeType: change, BeforeHash: stringPtrIf(found, latest.RepresentationHash), AfterHash: &representationHash, Summary: &repoSummary})
	for key, next := range current {
		prev, ok := previous[key]
		if !ok {
			diffs = append(diffs, snapshotDiff(next, "added", nil, &next.Hash))
			continue
		}
		if prev.Hash != next.Hash || ptrInt64Value(prev.ResourceID) != ptrInt64Value(next.ResourceID) {
			before, after := prev.Hash, next.Hash
			diffs = append(diffs, snapshotDiff(next, "updated", &before, &after))
		}
		delete(previous, key)
	}
	for _, prev := range previous {
		before := prev.Hash
		diffs = append(diffs, snapshotDiff(prev, "deleted", &before, nil))
	}
	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].OwnerType == diffs[j].OwnerType {
			return diffs[i].OwnerKey < diffs[j].OwnerKey
		}
		return diffs[i].OwnerType < diffs[j].OwnerType
	})
	return diffs, nil
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
		SELECT s.id, COALESCE(i.identity_key, s.stable_key), s.stable_key, s.content_hash, s.signature_hash, s.qualified_name
		FROM watch_symbols s
		LEFT JOIN watch_symbol_identities i ON i.repository_id = s.repository_id AND i.current_stable_key = s.stable_key
		WHERE s.repository_id = ?`, repositoryID)
	if err != nil {
		return nil, err
	}
	for symRows.Next() {
		var id int64
		var key, stableKey, contentHash, signatureHash, name string
		if err := symRows.Scan(&id, &key, &stableKey, &contentHash, &signatureHash, &name); err != nil {
			_ = symRows.Close()
			return nil, err
		}
		hash := hashString(contentHash + ":" + signatureHash)
		out[resourceSnapshotKey("symbol", key, "symbol")] = watchResourceSnapshot{OwnerType: "symbol", OwnerKey: key, ResourceType: "symbol", ResourceID: &id, Language: languageFromStableKey(stableKey), Hash: hash, Summary: name}
	}
	if err := symRows.Close(); err != nil {
		return nil, err
	}
	mapRows, err := s.db.QueryContext(ctx, `
		SELECT owner_type, owner_key, resource_type, resource_id
		FROM watch_materialization
		WHERE repository_id = ?`, repositoryID)
	if err != nil {
		return nil, err
	}
	type materializedMapping struct {
		OwnerType    string
		OwnerKey     string
		ResourceType string
		ResourceID   int64
	}
	var mappings []materializedMapping
	for mapRows.Next() {
		var mapping materializedMapping
		if err := mapRows.Scan(&mapping.OwnerType, &mapping.OwnerKey, &mapping.ResourceType, &mapping.ResourceID); err != nil {
			_ = mapRows.Close()
			return nil, err
		}
		mappings = append(mappings, mapping)
	}
	if err := mapRows.Close(); err != nil {
		return nil, err
	}
	for _, mapping := range mappings {
		hash, summary, language, err := s.materializedResourceHash(ctx, mapping.ResourceType, mapping.ResourceID)
		if err != nil {
			continue
		}
		id := mapping.ResourceID
		out[resourceSnapshotKey(mapping.OwnerType, mapping.OwnerKey, mapping.ResourceType)] = watchResourceSnapshot{OwnerType: mapping.OwnerType, OwnerKey: mapping.OwnerKey, ResourceType: mapping.ResourceType, ResourceID: &id, Language: language, Hash: hash, Summary: summary}
	}
	return out, nil
}

func (s *Store) materializedResourceHash(ctx context.Context, resourceType string, resourceID int64) (string, string, string, error) {
	switch resourceType {
	case "element":
		var name, kind, description, repo, branch, filePath, language sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT name, kind, description, repo, branch, file_path, language FROM elements WHERE id = ?`, resourceID).Scan(&name, &kind, &description, &repo, &branch, &filePath, &language)
		if err != nil {
			return "", "", "", err
		}
		raw := strings.Join([]string{name.String, kind.String, description.String, repo.String, branch.String, filePath.String, language.String}, "\n")
		return hashString(raw), name.String, language.String, nil
	case "view":
		var name, label sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT name, level_label FROM views WHERE id = ?`, resourceID).Scan(&name, &label)
		if err != nil {
			return "", "", "", err
		}
		return hashString(name.String + "\n" + label.String), name.String, "", nil
	case "connector":
		var viewID, sourceID, targetID int64
		var label, relationship sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT view_id, source_element_id, target_element_id, label, relationship FROM connectors WHERE id = ?`, resourceID).Scan(&viewID, &sourceID, &targetID, &label, &relationship)
		if err != nil {
			return "", "", "", err
		}
		raw := fmt.Sprintf("%d:%d:%d:%s:%s", viewID, sourceID, targetID, label.String, relationship.String)
		return hashString(raw), label.String, "", nil
	default:
		return "", "", "", fmt.Errorf("unsupported resource type %q", resourceType)
	}
}

func (s *Store) WatchVersionResourceSnapshots(ctx context.Context, versionID int64) (map[string]watchResourceSnapshot, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT owner_type, owner_key, resource_type, resource_id, language, resource_hash, summary
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
		var language, summary sql.NullString
		if err := rows.Scan(&item.OwnerType, &item.OwnerKey, &item.ResourceType, &resourceID, &language, &item.Hash, &summary); err != nil {
			return nil, err
		}
		if resourceID.Valid {
			item.ResourceID = &resourceID.Int64
		}
		item.Language = language.String
		item.Summary = summary.String
		out[resourceSnapshotKey(item.OwnerType, item.OwnerKey, item.ResourceType)] = item
	}
	return out, rows.Err()
}

func (s *Store) SaveWatchVersionResources(ctx context.Context, versionID, repositoryID int64) error {
	snapshots, err := s.CurrentWatchResourceSnapshots(ctx, repositoryID)
	if err != nil {
		return err
	}
	for _, item := range snapshots {
		_, err := s.db.ExecContext(ctx, `
			INSERT OR REPLACE INTO watch_version_resources(version_id, owner_type, owner_key, resource_type, resource_id, language, resource_hash, summary)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			versionID, item.OwnerType, item.OwnerKey, item.ResourceType, item.ResourceID, nullString(item.Language), item.Hash, nullString(item.Summary))
		if err != nil {
			return err
		}
	}
	return nil
}

func snapshotDiff(snapshot watchResourceSnapshot, changeType string, beforeHash, afterHash *string) RepresentationDiff {
	resourceType := snapshot.ResourceType
	summary := snapshot.Summary
	language := snapshot.Language
	return RepresentationDiff{OwnerType: snapshot.OwnerType, OwnerKey: snapshot.OwnerKey, ChangeType: changeType, BeforeHash: beforeHash, AfterHash: afterHash, ResourceType: &resourceType, ResourceID: snapshot.ResourceID, Language: &language, Summary: &summary}
}

func resourceSnapshotKey(ownerType, ownerKey, resourceType string) string {
	return ownerType + "\x00" + ownerKey + "\x00" + resourceType
}

func ptrInt64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func stringPtrIf(ok bool, value string) *string {
	if !ok {
		return nil
	}
	return &value
}

func diffLanguage(diff RepresentationDiff) string {
	if diff.Language != nil {
		return *diff.Language
	}
	if diff.OwnerType == "symbol" || diff.ResourceType != nil && *diff.ResourceType == "symbol" {
		return languageFromStableKey(diff.OwnerKey)
	}
	return ""
}

func (s *Store) fileByPath(ctx context.Context, repositoryID int64, path string) (File, bool, error) {
	file, err := s.fileByPathMust(ctx, repositoryID, path)
	if errors.Is(err, sql.ErrNoRows) {
		return File{}, false, nil
	}
	return file, err == nil, err
}

func scanLock(row rowScanner) (Lock, error) {
	var lock Lock
	if err := row.Scan(&lock.ID, &lock.RepositoryID, &lock.PID, &lock.Token, &lock.StartedAt, &lock.HeartbeatAt, &lock.Status); err != nil {
		return Lock{}, err
	}
	return lock, nil
}

func scanVersion(row rowScanner) (Version, error) {
	var version Version
	var parent sql.NullString
	var branch sql.NullString
	var workspaceVersionID sql.NullInt64
	if err := row.Scan(&version.ID, &version.RepositoryID, &version.CommitHash, &parent, &branch, &version.RepresentationHash, &workspaceVersionID, &version.CreatedAt); err != nil {
		return Version{}, err
	}
	if parent.Valid {
		version.ParentCommitHash = parent.String
	}
	if branch.Valid {
		version.Branch = branch.String
	}
	if workspaceVersionID.Valid {
		version.WorkspaceVersionID = &workspaceVersionID.Int64
	}
	return version, nil
}

func (s *Store) addElementTags(ctx context.Context, elementID int64, add []string) (int, error) {
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT tags FROM elements WHERE id = ?`, elementID).Scan(&raw); err != nil {
		return 0, err
	}
	var tags []string
	_ = json.Unmarshal([]byte(raw), &tags)
	seen := make(map[string]struct{}, len(tags)+len(add))
	next := make([]string, 0, len(tags)+len(add))
	added := 0
	for _, tag := range tags {
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		next = append(next, tag)
	}
	for _, tag := range add {
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		next = append(next, tag)
		added++
	}
	if added == 0 {
		return 0, nil
	}
	data, _ := json.Marshal(next)
	_, err := s.db.ExecContext(ctx, `UPDATE elements SET tags = ?, updated_at = ? WHERE id = ?`, string(data), nowString(), elementID)
	return added, err
}

func (s *Store) removeElementTags(ctx context.Context, elementID int64, remove []string) (int, error) {
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT tags FROM elements WHERE id = ?`, elementID).Scan(&raw); err != nil {
		return 0, err
	}
	var tags []string
	_ = json.Unmarshal([]byte(raw), &tags)
	removeSet := make(map[string]struct{}, len(remove))
	for _, tag := range remove {
		removeSet[tag] = struct{}{}
	}
	next := make([]string, 0, len(tags))
	removed := 0
	for _, tag := range tags {
		if _, ok := removeSet[tag]; ok {
			removed++
			continue
		}
		next = append(next, tag)
	}
	if removed == 0 {
		return 0, nil
	}
	data, _ := json.Marshal(next)
	_, err := s.db.ExecContext(ctx, `UPDATE elements SET tags = ?, updated_at = ? WHERE id = ?`, string(data), nowString(), elementID)
	return removed, err
}

func managedGitTags() []string {
	return []string{"git:staged", "git:unstaged", "git:untracked", "watch:deleted"}
}

func filepathToSlash(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func sameQualifierParent(left, right string) bool {
	leftParent := qualifierParent(left)
	rightParent := qualifierParent(right)
	return leftParent != "" && leftParent == rightParent
}

func qualifierParent(value string) string {
	if idx := strings.LastIndex(value, "."); idx > 0 {
		return value[:idx]
	}
	return ""
}

func nameTokenSimilarity(left, right string) float64 {
	leftTokens := splitIdentifierToken(pathBaseQualifier(left))
	rightTokens := splitIdentifierToken(pathBaseQualifier(right))
	if len(leftTokens) == 0 || len(rightTokens) == 0 {
		return 0
	}
	leftSet := make(map[string]struct{}, len(leftTokens))
	for _, token := range leftTokens {
		leftSet[token] = struct{}{}
	}
	intersection := 0
	union := len(leftSet)
	for _, token := range rightTokens {
		if _, ok := leftSet[token]; ok {
			intersection++
			continue
		}
		union++
	}
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func pathBaseQualifier(value string) string {
	if idx := strings.LastIndex(value, "."); idx >= 0 && idx+1 < len(value) {
		return value[idx+1:]
	}
	return value
}

func embeddingDataset(modelID int64) string {
	return fmt.Sprintf("model:%d", modelID)
}

func bytesToVector(data []byte) Vector {
	if len(data)%4 != 0 {
		return nil
	}
	vector := make(Vector, len(data)/4)
	for i := range vector {
		vector[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return vector
}

func (s *Store) clusterByStableKey(ctx context.Context, repositoryID int64, stableKey string) (Cluster, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, repository_id, stable_key, parent_cluster_id, name, kind, algorithm, settings_hash, member_count, created_at, updated_at
		FROM watch_clusters
		WHERE repository_id = ? AND stable_key = ?`, repositoryID, stableKey)
	return scanCluster(row)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCluster(row rowScanner) (Cluster, error) {
	var cluster Cluster
	var parent sql.NullInt64
	if err := row.Scan(&cluster.ID, &cluster.RepositoryID, &cluster.StableKey, &parent, &cluster.Name, &cluster.Kind, &cluster.Algorithm, &cluster.SettingsHash, &cluster.MemberCount, &cluster.CreatedAt, &cluster.UpdatedAt); err != nil {
		return Cluster{}, err
	}
	if parent.Valid {
		cluster.ParentClusterID = &parent.Int64
	}
	return cluster, nil
}

func (s *Store) latestFilterRunID(ctx context.Context, repositoryID int64) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM watch_filter_runs
		WHERE repository_id = ?
		ORDER BY id DESC
		LIMIT 1`, repositoryID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (s *Store) fileByPathMust(ctx context.Context, repositoryID int64, path string) (File, error) {
	var file File
	err := s.db.QueryRowContext(ctx, `
		SELECT id, repository_id, path, language, git_blob_hash, worktree_hash, size_bytes, mtime_unix, scan_status, scan_error, created_at, updated_at
		FROM watch_files
		WHERE repository_id = ? AND path = ?`, repositoryID, path).Scan(&file.ID, &file.RepositoryID, &file.Path, &file.Language, &file.GitBlobHash, &file.WorktreeHash, &file.SizeBytes, &file.MtimeUnix, &file.ScanStatus, &file.ScanError, &file.CreatedAt, &file.UpdatedAt)
	return file, err
}

func (s *Store) file(ctx context.Context, id int64) (File, error) {
	var file File
	err := s.db.QueryRowContext(ctx, `
		SELECT id, repository_id, path, language, git_blob_hash, worktree_hash, size_bytes, mtime_unix, scan_status, scan_error, created_at, updated_at
		FROM watch_files
		WHERE id = ?`, id).Scan(&file.ID, &file.RepositoryID, &file.Path, &file.Language, &file.GitBlobHash, &file.WorktreeHash, &file.SizeBytes, &file.MtimeUnix, &file.ScanStatus, &file.ScanError, &file.CreatedAt, &file.UpdatedAt)
	return file, err
}

func nullString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339)
}
