CREATE TABLE IF NOT EXISTS elements (
  id BIGSERIAL PRIMARY KEY,
  org_id UUID NULL,
  name TEXT NOT NULL,
  kind TEXT NULL,
  description TEXT NULL,
  technology TEXT NULL,
  url TEXT NULL,
  logo_url TEXT NULL,
  technology_connectors TEXT NOT NULL DEFAULT '[]',
  tags TEXT NOT NULL DEFAULT '[]',
  repo TEXT NULL,
  branch TEXT NULL,
  file_path TEXT NULL,
  language TEXT NULL,
  bypass_noise_gate BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS views (
  id BIGSERIAL PRIMARY KEY,
  org_id UUID NULL,
  owner_element_id BIGINT NULL REFERENCES elements(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NULL,
  level_label TEXT NULL,
  tags TEXT NOT NULL DEFAULT '[]',
  level INTEGER NOT NULL DEFAULT 1,
  density_level INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS placements (
  id BIGSERIAL PRIMARY KEY,
  view_id BIGINT NOT NULL REFERENCES views(id) ON DELETE CASCADE,
  element_id BIGINT NOT NULL REFERENCES elements(id) ON DELETE CASCADE,
  position_x DOUBLE PRECISION NOT NULL DEFAULT 0,
  position_y DOUBLE PRECISION NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(view_id, element_id)
);

CREATE TABLE IF NOT EXISTS connectors (
  id BIGSERIAL PRIMARY KEY,
  org_id UUID NULL,
  view_id BIGINT NOT NULL REFERENCES views(id) ON DELETE CASCADE,
  source_element_id BIGINT NOT NULL REFERENCES elements(id) ON DELETE CASCADE,
  target_element_id BIGINT NOT NULL REFERENCES elements(id) ON DELETE CASCADE,
  label TEXT NULL,
  description TEXT NULL,
  relationship TEXT NULL,
  direction TEXT NOT NULL DEFAULT 'forward',
  style TEXT NOT NULL DEFAULT 'bezier',
  url TEXT NULL,
  source_handle TEXT NULL,
  target_handle TEXT NULL,
  tags TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS view_layers (
  id BIGSERIAL PRIMARY KEY,
  view_id BIGINT NOT NULL REFERENCES views(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  tags TEXT NOT NULL DEFAULT '[]',
  color TEXT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tags (
  org_id TEXT NULL,
  name TEXT PRIMARY KEY,
  color TEXT NOT NULL,
  description TEXT NULL
);

CREATE TABLE IF NOT EXISTS view_visibility_overrides (
  view_id BIGINT NOT NULL REFERENCES views(id) ON DELETE CASCADE,
  resource_type TEXT NOT NULL CHECK (resource_type IN ('element', 'connector')),
  resource_id BIGINT NOT NULL,
  level_delta INTEGER NOT NULL DEFAULT 0 CHECK (level_delta BETWEEN -4 AND 4),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(view_id, resource_type, resource_id)
);

CREATE INDEX IF NOT EXISTS idx_view_visibility_overrides_resource
  ON view_visibility_overrides(resource_type, resource_id);

CREATE TABLE IF NOT EXISTS watch_repositories (
  id BIGSERIAL PRIMARY KEY,
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
  id BIGSERIAL PRIMARY KEY,
  repository_id BIGINT NOT NULL REFERENCES watch_repositories(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  language TEXT NOT NULL,
  git_blob_hash TEXT NULL,
  worktree_hash TEXT NOT NULL,
  size_bytes BIGINT NOT NULL DEFAULT 0,
  mtime_unix BIGINT NOT NULL DEFAULT 0,
  scan_status TEXT NOT NULL,
  scan_error TEXT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(repository_id, path)
);

CREATE INDEX IF NOT EXISTS idx_watch_files_repository_id
  ON watch_files(repository_id);

CREATE TABLE IF NOT EXISTS watch_symbols (
  id BIGSERIAL PRIMARY KEY,
  repository_id BIGINT NOT NULL REFERENCES watch_repositories(id) ON DELETE CASCADE,
  file_id BIGINT NOT NULL REFERENCES watch_files(id) ON DELETE CASCADE,
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
  UNIQUE(repository_id, stable_key)
);

CREATE INDEX IF NOT EXISTS idx_watch_symbols_repository_id ON watch_symbols(repository_id);
CREATE INDEX IF NOT EXISTS idx_watch_symbols_file_id ON watch_symbols(file_id);
CREATE INDEX IF NOT EXISTS idx_watch_symbols_search ON watch_symbols(repository_id, name, qualified_name, kind);

CREATE TABLE IF NOT EXISTS watch_references (
  id BIGSERIAL PRIMARY KEY,
  repository_id BIGINT NOT NULL REFERENCES watch_repositories(id) ON DELETE CASCADE,
  source_symbol_id BIGINT NOT NULL REFERENCES watch_symbols(id) ON DELETE CASCADE,
  target_symbol_id BIGINT NOT NULL REFERENCES watch_symbols(id) ON DELETE CASCADE,
  source_file_id BIGINT NOT NULL REFERENCES watch_files(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  line INTEGER NOT NULL,
  "column" INTEGER NOT NULL DEFAULT 0,
  evidence_hash TEXT NOT NULL,
  raw_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(repository_id, source_symbol_id, target_symbol_id, kind, evidence_hash)
);

CREATE INDEX IF NOT EXISTS idx_watch_references_repository_id ON watch_references(repository_id);
CREATE INDEX IF NOT EXISTS idx_watch_references_source_symbol_id ON watch_references(source_symbol_id);
CREATE INDEX IF NOT EXISTS idx_watch_references_target_symbol_id ON watch_references(target_symbol_id);

CREATE TABLE IF NOT EXISTS watch_facts (
  id BIGSERIAL PRIMARY KEY,
  repository_id BIGINT NOT NULL REFERENCES watch_repositories(id) ON DELETE CASCADE,
  file_id BIGINT NOT NULL REFERENCES watch_files(id) ON DELETE CASCADE,
  stable_key TEXT NOT NULL,
  type TEXT NOT NULL,
  enricher TEXT NOT NULL,
  subject_kind TEXT NOT NULL,
  subject_stable_key TEXT NOT NULL,
  object_kind TEXT NOT NULL DEFAULT '',
  object_stable_key TEXT NOT NULL DEFAULT '',
  object_file_path TEXT NOT NULL DEFAULT '',
  object_name TEXT NOT NULL DEFAULT '',
  relationship TEXT NOT NULL DEFAULT '',
  file_path TEXT NOT NULL,
  start_line INTEGER NOT NULL DEFAULT 0,
  end_line INTEGER NULL,
  confidence DOUBLE PRECISION NOT NULL DEFAULT 1.0,
  name TEXT NOT NULL DEFAULT '',
  tags TEXT NOT NULL DEFAULT '[]',
  attributes_json TEXT NOT NULL DEFAULT '{}',
  visibility_hints_json TEXT NOT NULL DEFAULT '{}',
  fact_hash TEXT NOT NULL,
  raw_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(repository_id, enricher, stable_key)
);

CREATE INDEX IF NOT EXISTS idx_watch_facts_repository_id ON watch_facts(repository_id);
CREATE INDEX IF NOT EXISTS idx_watch_facts_file_id ON watch_facts(file_id);
CREATE INDEX IF NOT EXISTS idx_watch_facts_subject ON watch_facts(repository_id, subject_kind, subject_stable_key);
CREATE INDEX IF NOT EXISTS idx_watch_facts_object ON watch_facts(repository_id, object_kind, object_stable_key);
CREATE INDEX IF NOT EXISTS idx_watch_facts_type ON watch_facts(repository_id, type);

CREATE TABLE IF NOT EXISTS watch_scan_runs (
  id BIGSERIAL PRIMARY KEY,
  repository_id BIGINT NOT NULL REFERENCES watch_repositories(id) ON DELETE CASCADE,
  mode TEXT NOT NULL,
  started_at TEXT NOT NULL,
  finished_at TEXT NULL,
  status TEXT NOT NULL,
  files_seen INTEGER NOT NULL DEFAULT 0,
  files_parsed INTEGER NOT NULL DEFAULT 0,
  files_skipped INTEGER NOT NULL DEFAULT 0,
  symbols_seen INTEGER NOT NULL DEFAULT 0,
  references_seen INTEGER NOT NULL DEFAULT 0,
  error TEXT NULL
);

CREATE INDEX IF NOT EXISTS idx_watch_scan_runs_repository_id ON watch_scan_runs(repository_id);

CREATE TABLE IF NOT EXISTS watch_embedding_models (
  id BIGSERIAL PRIMARY KEY,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  dimension INTEGER NOT NULL,
  config_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(provider, model, dimension, config_hash)
);

CREATE TABLE IF NOT EXISTS watch_embeddings (
  id BIGSERIAL PRIMARY KEY,
  model_id BIGINT NOT NULL REFERENCES watch_embedding_models(id) ON DELETE CASCADE,
  owner_type TEXT NOT NULL,
  owner_key TEXT NOT NULL,
  input_hash TEXT NOT NULL,
  vector BYTEA NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(model_id, owner_type, owner_key, input_hash)
);

CREATE INDEX IF NOT EXISTS idx_watch_embeddings_model ON watch_embeddings(model_id);

CREATE TABLE IF NOT EXISTS watch_filter_runs (
  id BIGSERIAL PRIMARY KEY,
  repository_id BIGINT NOT NULL REFERENCES watch_repositories(id) ON DELETE CASCADE,
  settings_hash TEXT NOT NULL,
  raw_graph_hash TEXT NOT NULL,
  started_at TEXT NOT NULL,
  finished_at TEXT NULL,
  status TEXT NOT NULL,
  visible_symbols INTEGER NOT NULL DEFAULT 0,
  hidden_symbols INTEGER NOT NULL DEFAULT 0,
  visible_references INTEGER NOT NULL DEFAULT 0,
  hidden_references INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_watch_filter_runs_repository_id ON watch_filter_runs(repository_id);

CREATE TABLE IF NOT EXISTS watch_filter_decisions (
  id BIGSERIAL PRIMARY KEY,
  filter_run_id BIGINT NOT NULL REFERENCES watch_filter_runs(id) ON DELETE CASCADE,
  owner_type TEXT NOT NULL,
  owner_id BIGINT NOT NULL,
  owner_key TEXT NOT NULL DEFAULT '',
  decision TEXT NOT NULL,
  reason TEXT NOT NULL,
  score DOUBLE PRECISION NULL,
  tier INTEGER NOT NULL DEFAULT 0,
  signals_json TEXT NOT NULL DEFAULT '[]'
);

CREATE INDEX IF NOT EXISTS idx_watch_filter_decisions_filter_run_id ON watch_filter_decisions(filter_run_id);
CREATE INDEX IF NOT EXISTS idx_watch_filter_decisions_owner ON watch_filter_decisions(owner_type, owner_id);
CREATE INDEX IF NOT EXISTS idx_watch_filter_decisions_owner_key ON watch_filter_decisions(owner_type, owner_key);

CREATE TABLE IF NOT EXISTS watch_clusters (
  id BIGSERIAL PRIMARY KEY,
  repository_id BIGINT NOT NULL REFERENCES watch_repositories(id) ON DELETE CASCADE,
  stable_key TEXT NOT NULL,
  parent_cluster_id BIGINT NULL REFERENCES watch_clusters(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  algorithm TEXT NOT NULL,
  settings_hash TEXT NOT NULL,
  member_count INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(repository_id, stable_key)
);

CREATE INDEX IF NOT EXISTS idx_watch_clusters_repository_id ON watch_clusters(repository_id);

CREATE TABLE IF NOT EXISTS watch_cluster_members (
  cluster_id BIGINT NOT NULL REFERENCES watch_clusters(id) ON DELETE CASCADE,
  owner_type TEXT NOT NULL,
  owner_id BIGINT NOT NULL,
  PRIMARY KEY (cluster_id, owner_type, owner_id)
);

CREATE TABLE IF NOT EXISTS watch_materialization (
  id BIGSERIAL PRIMARY KEY,
  repository_id BIGINT NOT NULL REFERENCES watch_repositories(id) ON DELETE CASCADE,
  owner_type TEXT NOT NULL,
  owner_key TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id BIGINT NOT NULL,
  last_watch_hash TEXT NULL,
  dirty INTEGER NOT NULL DEFAULT 0,
  dirty_detected_at TEXT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(repository_id, owner_type, owner_key, resource_type)
);

CREATE INDEX IF NOT EXISTS idx_watch_materialization_repository_id ON watch_materialization(repository_id);
CREATE INDEX IF NOT EXISTS idx_watch_materialization_resource_lookup ON watch_materialization(repository_id, resource_type, resource_id);

CREATE TABLE IF NOT EXISTS watch_architecture_links (
  id BIGSERIAL PRIMARY KEY,
  repository_id BIGINT NOT NULL REFERENCES watch_repositories(id) ON DELETE CASCADE,
  component_key TEXT NOT NULL,
  target_repository_id BIGINT NULL REFERENCES watch_repositories(id) ON DELETE CASCADE,
  target_owner_type TEXT NOT NULL,
  target_owner_key TEXT NOT NULL,
  target_resource_type TEXT NOT NULL,
  target_resource_id BIGINT NOT NULL,
  role TEXT NOT NULL,
  confidence DOUBLE PRECISION NOT NULL DEFAULT 1.0,
  evidence_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(repository_id, component_key, target_repository_id, target_owner_type, target_owner_key, target_resource_type, role)
);

CREATE INDEX IF NOT EXISTS idx_watch_architecture_links_repository_id
  ON watch_architecture_links(repository_id);
CREATE INDEX IF NOT EXISTS idx_watch_architecture_links_target
  ON watch_architecture_links(target_repository_id, target_owner_type, target_owner_key);

CREATE TABLE IF NOT EXISTS watch_apply_locks (
  id BIGINT PRIMARY KEY,
  repository_id BIGINT NOT NULL,
  pid BIGINT NOT NULL,
  token TEXT NOT NULL,
  started_at TEXT NOT NULL,
  heartbeat_at TEXT NOT NULL,
  status TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS watch_context_policies (
  id BIGSERIAL PRIMARY KEY,
  repository_id BIGINT NOT NULL REFERENCES watch_repositories(id) ON DELETE CASCADE,
  owner_type TEXT NOT NULL,
  owner_key TEXT NOT NULL,
  action TEXT NOT NULL,
  scope TEXT NOT NULL,
  active INTEGER NOT NULL DEFAULT 1,
  reason TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_watch_context_policies_repository_active ON watch_context_policies(repository_id, active);
CREATE INDEX IF NOT EXISTS idx_watch_context_policies_owner ON watch_context_policies(repository_id, owner_type, owner_key);

CREATE TABLE IF NOT EXISTS watch_context_expansions (
  id BIGSERIAL PRIMARY KEY,
  repository_id BIGINT NOT NULL REFERENCES watch_repositories(id) ON DELETE CASCADE,
  scope_resource_type TEXT NOT NULL,
  scope_resource_id BIGINT NOT NULL,
  scope_owner_type TEXT NOT NULL,
  scope_owner_key TEXT NOT NULL,
  tier INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(repository_id, scope_resource_type, scope_resource_id, scope_owner_type, scope_owner_key)
);

CREATE INDEX IF NOT EXISTS idx_watch_context_expansions_repository ON watch_context_expansions(repository_id);
CREATE INDEX IF NOT EXISTS idx_watch_context_expansions_scope ON watch_context_expansions(repository_id, scope_resource_type, scope_resource_id);
CREATE INDEX IF NOT EXISTS idx_watch_context_expansions_owner ON watch_context_expansions(repository_id, scope_owner_type, scope_owner_key);

CREATE TABLE IF NOT EXISTS watch_representation_runs (
  id BIGSERIAL PRIMARY KEY,
  repository_id BIGINT NOT NULL REFERENCES watch_repositories(id) ON DELETE CASCADE,
  filter_run_id BIGINT NULL REFERENCES watch_filter_runs(id) ON DELETE SET NULL,
  embedding_model_id BIGINT NULL REFERENCES watch_embedding_models(id) ON DELETE SET NULL,
  raw_graph_hash TEXT NOT NULL,
  filter_settings_hash TEXT NOT NULL,
  representation_hash TEXT NOT NULL,
  status TEXT NOT NULL,
  started_at TEXT NOT NULL,
  finished_at TEXT NULL,
  elements_created INTEGER NOT NULL DEFAULT 0,
  elements_updated INTEGER NOT NULL DEFAULT 0,
  connectors_created INTEGER NOT NULL DEFAULT 0,
  connectors_updated INTEGER NOT NULL DEFAULT 0,
  views_created INTEGER NOT NULL DEFAULT 0,
  error TEXT NULL
);

CREATE TABLE IF NOT EXISTS watch_locks (
  id BIGINT PRIMARY KEY,
  repository_id BIGINT NOT NULL,
  pid BIGINT NOT NULL,
  token TEXT NOT NULL,
  started_at TEXT NOT NULL,
  heartbeat_at TEXT NOT NULL,
  status TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS watch_versions (
  id BIGSERIAL PRIMARY KEY,
  repository_id BIGINT NOT NULL REFERENCES watch_repositories(id) ON DELETE CASCADE,
  commit_hash TEXT NOT NULL,
  parent_commit_hash TEXT NULL,
  branch TEXT NULL,
  representation_hash TEXT NOT NULL,
  workspace_version_id BIGINT NULL,
  created_at TEXT NOT NULL,
  commit_message TEXT NULL,
  UNIQUE(repository_id, commit_hash, representation_hash)
);

CREATE INDEX IF NOT EXISTS idx_watch_versions_repository_id
  ON watch_versions(repository_id);

CREATE TABLE IF NOT EXISTS watch_representation_diffs (
  id BIGSERIAL PRIMARY KEY,
  version_id BIGINT NOT NULL REFERENCES watch_versions(id) ON DELETE CASCADE,
  owner_type TEXT NOT NULL,
  owner_key TEXT NOT NULL,
  change_type TEXT NOT NULL,
  before_hash TEXT NULL,
  after_hash TEXT NULL,
  resource_type TEXT NULL,
  resource_id BIGINT NULL,
  summary TEXT NULL,
  added_lines INTEGER NOT NULL DEFAULT 0,
  removed_lines INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_watch_representation_diffs_version_id
  ON watch_representation_diffs(version_id);

CREATE TABLE IF NOT EXISTS workspace_versions (
  id BIGSERIAL PRIMARY KEY,
  org_id UUID NULL,
  version_id TEXT NOT NULL UNIQUE,
  source TEXT NOT NULL,
  parent_version_id BIGINT NULL REFERENCES workspace_versions(id) ON DELETE SET NULL,
  view_count BIGINT NOT NULL DEFAULT 0,
  element_count BIGINT NOT NULL DEFAULT 0,
  connector_count BIGINT NOT NULL DEFAULT 0,
  description TEXT NULL,
  workspace_hash TEXT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS workspace_version_settings (
  id BIGINT PRIMARY KEY,
  cli_versioning_enabled INTEGER NOT NULL DEFAULT 1
);

INSERT INTO workspace_version_settings(id, cli_versioning_enabled)
VALUES (1, 1)
ON CONFLICT(id) DO NOTHING;

CREATE TABLE IF NOT EXISTS watch_symbol_identities (
  id BIGSERIAL PRIMARY KEY,
  repository_id BIGINT NOT NULL REFERENCES watch_repositories(id) ON DELETE CASCADE,
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
  UNIQUE(repository_id, current_stable_key)
);

CREATE INDEX IF NOT EXISTS idx_watch_symbol_identities_current_key
  ON watch_symbol_identities(repository_id, current_stable_key);

CREATE TABLE IF NOT EXISTS watch_version_resources (
  version_id BIGINT NOT NULL REFERENCES watch_versions(id) ON DELETE CASCADE,
  owner_type TEXT NOT NULL,
  owner_key TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id BIGINT NULL,
  language TEXT NULL,
  resource_hash TEXT NOT NULL,
  summary TEXT NULL,
  line_count INTEGER NOT NULL DEFAULT 0,
  file_path TEXT NULL,
  start_line INTEGER NOT NULL DEFAULT 0,
  end_line INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(version_id, owner_type, owner_key, resource_type)
);

CREATE INDEX IF NOT EXISTS idx_watch_version_resources_version_id
  ON watch_version_resources(version_id);
