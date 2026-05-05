ALTER TABLE watch_materialization ADD COLUMN last_watch_hash TEXT NULL;
ALTER TABLE watch_materialization ADD COLUMN dirty INTEGER NOT NULL DEFAULT 0;
ALTER TABLE watch_materialization ADD COLUMN dirty_detected_at TEXT NULL;

CREATE TABLE IF NOT EXISTS watch_apply_locks (
  id INTEGER PRIMARY KEY,
  repository_id INTEGER NOT NULL,
  pid INTEGER NOT NULL,
  token TEXT NOT NULL,
  started_at TEXT NOT NULL,
  heartbeat_at TEXT NOT NULL,
  status TEXT NOT NULL,
  FOREIGN KEY (repository_id) REFERENCES watch_repositories(id) ON DELETE CASCADE
);
