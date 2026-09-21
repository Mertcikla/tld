CREATE TABLE IF NOT EXISTS watch_repository_settings (
  repository_id INTEGER PRIMARY KEY,
  settings_json TEXT NOT NULL,
  embedding_json TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  FOREIGN KEY (repository_id) REFERENCES watch_repositories(id) ON DELETE CASCADE
);
