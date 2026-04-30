PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS watch_symbol_identities (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  repository_id INTEGER NOT NULL,
  identity_key TEXT NOT NULL,
  current_stable_key TEXT NOT NULL,
  file_path TEXT NOT NULL,
  kind TEXT NOT NULL,
  name TEXT NOT NULL,
  qualified_name TEXT NOT NULL,
  start_line INTEGER NOT NULL,
  content_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(repository_id, identity_key),
  UNIQUE(repository_id, current_stable_key),
  FOREIGN KEY (repository_id) REFERENCES watch_repositories(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_watch_symbol_identities_current_key
  ON watch_symbol_identities(repository_id, current_stable_key);

CREATE TABLE IF NOT EXISTS _vec_watch_embedding_vec (
  dataset_id TEXT NOT NULL,
  id TEXT NOT NULL,
  content TEXT,
  meta TEXT,
  embedding BLOB,
  PRIMARY KEY(dataset_id, id)
);
