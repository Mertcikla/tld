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
  commit_message TEXT NULL,
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
  added_lines INTEGER NOT NULL DEFAULT 0,
  removed_lines INTEGER NOT NULL DEFAULT 0,
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

CREATE INDEX IF NOT EXISTS idx_views_owner_element_id
  ON views(owner_element_id);

CREATE INDEX IF NOT EXISTS idx_placements_element_id_view_id
  ON placements(element_id, view_id);

CREATE INDEX IF NOT EXISTS idx_placements_view_id_id
  ON placements(view_id, id);

CREATE INDEX IF NOT EXISTS idx_connectors_view_id_id
  ON connectors(view_id, id);

CREATE INDEX IF NOT EXISTS idx_elements_updated_at_id
  ON elements(updated_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS watch_version_resources (
  version_id INTEGER NOT NULL,
  owner_type TEXT NOT NULL,
  owner_key TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id INTEGER NULL,
  language TEXT NULL,
  resource_hash TEXT NOT NULL,
  summary TEXT NULL,
  line_count INTEGER NOT NULL DEFAULT 0,
  file_path TEXT NULL,
  start_line INTEGER NOT NULL DEFAULT 0,
  end_line INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(version_id, owner_type, owner_key, resource_type),
  FOREIGN KEY (version_id) REFERENCES watch_versions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_watch_version_resources_version_id
  ON watch_version_resources(version_id);
