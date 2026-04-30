PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS watch_embedding_models (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  dimension INTEGER NOT NULL,
  config_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(provider, model, dimension, config_hash)
);

CREATE TABLE IF NOT EXISTS watch_embeddings (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  model_id INTEGER NOT NULL,
  owner_type TEXT NOT NULL,
  owner_key TEXT NOT NULL,
  input_hash TEXT NOT NULL,
  vector BLOB NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(model_id, owner_type, owner_key, input_hash),
  FOREIGN KEY (model_id) REFERENCES watch_embedding_models(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS watch_filter_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  repository_id INTEGER NOT NULL,
  settings_hash TEXT NOT NULL,
  raw_graph_hash TEXT NOT NULL,
  started_at TEXT NOT NULL,
  finished_at TEXT NULL,
  status TEXT NOT NULL,
  visible_symbols INTEGER NOT NULL DEFAULT 0,
  hidden_symbols INTEGER NOT NULL DEFAULT 0,
  visible_references INTEGER NOT NULL DEFAULT 0,
  hidden_references INTEGER NOT NULL DEFAULT 0,
  FOREIGN KEY (repository_id) REFERENCES watch_repositories(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_watch_filter_runs_repository_id
  ON watch_filter_runs(repository_id);

CREATE TABLE IF NOT EXISTS watch_filter_decisions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  filter_run_id INTEGER NOT NULL,
  owner_type TEXT NOT NULL,
  owner_id INTEGER NOT NULL,
  decision TEXT NOT NULL,
  reason TEXT NOT NULL,
  score REAL NULL,
  FOREIGN KEY (filter_run_id) REFERENCES watch_filter_runs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_watch_filter_decisions_filter_run_id
  ON watch_filter_decisions(filter_run_id);

CREATE INDEX IF NOT EXISTS idx_watch_filter_decisions_owner
  ON watch_filter_decisions(owner_type, owner_id);

CREATE TABLE IF NOT EXISTS watch_clusters (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  repository_id INTEGER NOT NULL,
  stable_key TEXT NOT NULL,
  parent_cluster_id INTEGER NULL,
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  algorithm TEXT NOT NULL,
  settings_hash TEXT NOT NULL,
  member_count INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(repository_id, stable_key),
  FOREIGN KEY (repository_id) REFERENCES watch_repositories(id) ON DELETE CASCADE,
  FOREIGN KEY (parent_cluster_id) REFERENCES watch_clusters(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_watch_clusters_repository_id
  ON watch_clusters(repository_id);

CREATE TABLE IF NOT EXISTS watch_cluster_members (
  cluster_id INTEGER NOT NULL,
  owner_type TEXT NOT NULL,
  owner_id INTEGER NOT NULL,
  PRIMARY KEY (cluster_id, owner_type, owner_id),
  FOREIGN KEY (cluster_id) REFERENCES watch_clusters(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS watch_materialization (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  repository_id INTEGER NOT NULL,
  owner_type TEXT NOT NULL,
  owner_key TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(repository_id, owner_type, owner_key, resource_type),
  FOREIGN KEY (repository_id) REFERENCES watch_repositories(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_watch_materialization_repository_id
  ON watch_materialization(repository_id);

CREATE TABLE IF NOT EXISTS watch_representation_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  repository_id INTEGER NOT NULL,
  raw_graph_hash TEXT NOT NULL,
  filter_settings_hash TEXT NOT NULL,
  embedding_model_id INTEGER NULL,
  representation_hash TEXT NOT NULL,
  started_at TEXT NOT NULL,
  finished_at TEXT NULL,
  status TEXT NOT NULL,
  elements_created INTEGER NOT NULL DEFAULT 0,
  elements_updated INTEGER NOT NULL DEFAULT 0,
  connectors_created INTEGER NOT NULL DEFAULT 0,
  connectors_updated INTEGER NOT NULL DEFAULT 0,
  views_created INTEGER NOT NULL DEFAULT 0,
  error TEXT NULL,
  FOREIGN KEY (repository_id) REFERENCES watch_repositories(id) ON DELETE CASCADE,
  FOREIGN KEY (embedding_model_id) REFERENCES watch_embedding_models(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_watch_representation_runs_repository_id
  ON watch_representation_runs(repository_id);
