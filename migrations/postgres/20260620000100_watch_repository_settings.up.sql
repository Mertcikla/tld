CREATE TABLE IF NOT EXISTS watch_repository_settings (
  repository_id BIGINT PRIMARY KEY REFERENCES watch_repositories(id) ON DELETE CASCADE,
  settings_json TEXT NOT NULL,
  embedding_json TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);
