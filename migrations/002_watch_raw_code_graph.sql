PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS watch_repositories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  remote_url TEXT NULL,
  repo_root TEXT NOT NULL,
  display_name TEXT NOT NULL,
  branch TEXT NULL,
  head_commit TEXT NULL,
  identity_status TEXT NOT NULL DEFAULT 'known',
  settings_hash TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_watch_repositories_remote_url
  ON watch_repositories(remote_url)
  WHERE remote_url IS NOT NULL AND remote_url <> '';

CREATE INDEX IF NOT EXISTS idx_watch_repositories_repo_root
  ON watch_repositories(repo_root);

CREATE TABLE IF NOT EXISTS watch_files (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  repository_id INTEGER NOT NULL,
  path TEXT NOT NULL,
  language TEXT NOT NULL,
  git_blob_hash TEXT NULL,
  worktree_hash TEXT NOT NULL,
  size_bytes INTEGER NOT NULL DEFAULT 0,
  mtime_unix INTEGER NOT NULL DEFAULT 0,
  scan_status TEXT NOT NULL,
  scan_error TEXT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(repository_id, path),
  FOREIGN KEY (repository_id) REFERENCES watch_repositories(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_watch_files_repository_id
  ON watch_files(repository_id);

CREATE TABLE IF NOT EXISTS watch_symbols (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  repository_id INTEGER NOT NULL,
  file_id INTEGER NOT NULL,
  stable_key TEXT NOT NULL,
  name TEXT NOT NULL,
  qualified_name TEXT NOT NULL,
  kind TEXT NOT NULL,
  start_line INTEGER NOT NULL,
  end_line INTEGER NULL,
  signature_hash TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  raw_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(repository_id, stable_key),
  FOREIGN KEY (repository_id) REFERENCES watch_repositories(id) ON DELETE CASCADE,
  FOREIGN KEY (file_id) REFERENCES watch_files(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_watch_symbols_repository_id
  ON watch_symbols(repository_id);

CREATE INDEX IF NOT EXISTS idx_watch_symbols_file_id
  ON watch_symbols(file_id);

CREATE INDEX IF NOT EXISTS idx_watch_symbols_search
  ON watch_symbols(repository_id, name, qualified_name, kind);

CREATE TABLE IF NOT EXISTS watch_references (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  repository_id INTEGER NOT NULL,
  source_symbol_id INTEGER NOT NULL,
  target_symbol_id INTEGER NOT NULL,
  source_file_id INTEGER NOT NULL,
  kind TEXT NOT NULL,
  line INTEGER NOT NULL,
  column INTEGER NOT NULL DEFAULT 0,
  evidence_hash TEXT NOT NULL,
  raw_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(repository_id, source_symbol_id, target_symbol_id, kind, evidence_hash),
  FOREIGN KEY (repository_id) REFERENCES watch_repositories(id) ON DELETE CASCADE,
  FOREIGN KEY (source_symbol_id) REFERENCES watch_symbols(id) ON DELETE CASCADE,
  FOREIGN KEY (target_symbol_id) REFERENCES watch_symbols(id) ON DELETE CASCADE,
  FOREIGN KEY (source_file_id) REFERENCES watch_files(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_watch_references_repository_id
  ON watch_references(repository_id);

CREATE INDEX IF NOT EXISTS idx_watch_references_source_symbol_id
  ON watch_references(source_symbol_id);

CREATE INDEX IF NOT EXISTS idx_watch_references_target_symbol_id
  ON watch_references(target_symbol_id);

CREATE TABLE IF NOT EXISTS watch_scan_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  repository_id INTEGER NOT NULL,
  mode TEXT NOT NULL,
  started_at TEXT NOT NULL,
  finished_at TEXT NULL,
  status TEXT NOT NULL,
  files_seen INTEGER NOT NULL DEFAULT 0,
  files_parsed INTEGER NOT NULL DEFAULT 0,
  files_skipped INTEGER NOT NULL DEFAULT 0,
  symbols_seen INTEGER NOT NULL DEFAULT 0,
  references_seen INTEGER NOT NULL DEFAULT 0,
  error TEXT NULL,
  FOREIGN KEY (repository_id) REFERENCES watch_repositories(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_watch_scan_runs_repository_id
  ON watch_scan_runs(repository_id);
