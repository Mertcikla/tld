PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS watch_context_policies (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  repository_id INTEGER NOT NULL,
  owner_type TEXT NOT NULL,
  owner_key TEXT NOT NULL,
  action TEXT NOT NULL,
  scope TEXT NOT NULL,
  active INTEGER NOT NULL DEFAULT 1,
  reason TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (repository_id) REFERENCES watch_repositories(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_watch_context_policies_repository_active
  ON watch_context_policies(repository_id, active);

CREATE INDEX IF NOT EXISTS idx_watch_context_policies_owner
  ON watch_context_policies(repository_id, owner_type, owner_key);
