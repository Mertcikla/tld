PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS watch_locks (
  id INTEGER PRIMARY KEY,
  repository_id INTEGER NOT NULL,
  pid INTEGER NOT NULL,
  token TEXT NOT NULL,
  started_at TEXT NOT NULL,
  heartbeat_at TEXT NOT NULL,
  status TEXT NOT NULL,
  FOREIGN KEY (repository_id) REFERENCES watch_repositories(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_watch_locks_repository_active
  ON watch_locks(repository_id)
  WHERE status IN ('active', 'stopping');

CREATE TABLE IF NOT EXISTS watch_versions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  repository_id INTEGER NOT NULL,
  commit_hash TEXT NOT NULL,
  parent_commit_hash TEXT NULL,
  branch TEXT NULL,
  representation_hash TEXT NOT NULL,
  workspace_version_id INTEGER NULL,
  created_at TEXT NOT NULL,
  UNIQUE(repository_id, commit_hash, representation_hash),
  FOREIGN KEY (repository_id) REFERENCES watch_repositories(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_watch_versions_repository_id
  ON watch_versions(repository_id);

CREATE TABLE IF NOT EXISTS watch_representation_diffs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  version_id INTEGER NOT NULL,
  owner_type TEXT NOT NULL,
  owner_key TEXT NOT NULL,
  change_type TEXT NOT NULL,
  before_hash TEXT NULL,
  after_hash TEXT NULL,
  resource_type TEXT NULL,
  resource_id INTEGER NULL,
  summary TEXT NULL,
  FOREIGN KEY (version_id) REFERENCES watch_versions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_watch_representation_diffs_version_id
  ON watch_representation_diffs(version_id);

CREATE TABLE IF NOT EXISTS workspace_versions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  version_id TEXT NOT NULL UNIQUE,
  source TEXT NOT NULL,
  parent_version_id INTEGER NULL,
  view_count INTEGER NOT NULL DEFAULT 0,
  element_count INTEGER NOT NULL DEFAULT 0,
  connector_count INTEGER NOT NULL DEFAULT 0,
  description TEXT NULL,
  workspace_hash TEXT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY (parent_version_id) REFERENCES workspace_versions(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS workspace_version_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  cli_versioning_enabled INTEGER NOT NULL DEFAULT 1
);

INSERT INTO workspace_version_settings(id, cli_versioning_enabled)
VALUES (1, 1)
ON CONFLICT(id) DO NOTHING;
