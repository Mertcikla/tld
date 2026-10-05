-- Tenant identity belongs in every codeindex key, including immutable entities.
-- Normalize legacy NULL organisations to the single-tenant sentinel so retries
-- conflict within that tenant on both dialects. Preserve all existing rows.

-- Stage referencing children before rebuilding parents so foreign-key cascades
-- cannot delete settings or active maps during the table replacement.

CREATE TEMP TABLE codeindex_repository_settings_backup AS SELECT * FROM codeindex_repository_settings;

DROP TABLE codeindex_repository_settings;

CREATE TEMP TABLE codeindex_active_maps_backup AS SELECT * FROM codeindex_active_maps;

DROP TABLE codeindex_active_maps;

CREATE TABLE codeindex_repositories_scoped (
  id TEXT NOT NULL,
  root TEXT NOT NULL,
  remote_url TEXT NOT NULL DEFAULT '',
  remote_key TEXT NOT NULL DEFAULT '',
  managed BOOLEAN NOT NULL DEFAULT FALSE,
  latest_snapshot_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

INSERT INTO codeindex_repositories_scoped (id, root, remote_url, remote_key, managed, latest_snapshot_id, created_at, updated_at, org_id)
  SELECT id, root, remote_url, remote_key, managed, latest_snapshot_id, created_at, updated_at, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_repositories;

DROP TABLE codeindex_repositories;

ALTER TABLE codeindex_repositories_scoped RENAME TO codeindex_repositories;

CREATE TABLE codeindex_snapshots_scoped (
  id TEXT NOT NULL,
  repository_id TEXT NOT NULL,
  created_unix INTEGER NOT NULL DEFAULT 0,
  git_revision TEXT NOT NULL DEFAULT '',
  git_branch TEXT NOT NULL DEFAULT '',
  ingestion_status TEXT NOT NULL DEFAULT '',
  config_hash TEXT NOT NULL DEFAULT '',
  projects_json TEXT NOT NULL DEFAULT '[]',
  warnings_json TEXT NOT NULL DEFAULT '[]',
  tool_versions_json TEXT NOT NULL DEFAULT '{}',
  provenance TEXT NOT NULL DEFAULT '',
  content_fingerprint TEXT NOT NULL DEFAULT '',
  capture_order BIGINT NOT NULL DEFAULT 0,
  commit_message TEXT NOT NULL DEFAULT '',
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

INSERT INTO codeindex_snapshots_scoped (id, repository_id, created_unix, git_revision, git_branch, ingestion_status, config_hash, projects_json, warnings_json, tool_versions_json, provenance, content_fingerprint, capture_order, commit_message, org_id)
  SELECT id, repository_id, created_unix, git_revision, git_branch, ingestion_status, config_hash, projects_json, warnings_json, tool_versions_json, provenance, content_fingerprint, capture_order, commit_message, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_snapshots;

DROP TABLE codeindex_snapshots;

ALTER TABLE codeindex_snapshots_scoped RENAME TO codeindex_snapshots;

CREATE TABLE codeindex_sources_scoped (
  snapshot_id TEXT NOT NULL,
  path TEXT NOT NULL,
  hash TEXT NOT NULL,
  size INTEGER NOT NULL DEFAULT 0,
  content BLOB,
  language TEXT NOT NULL DEFAULT '',
  input_blob TEXT NOT NULL DEFAULT '',
  dirty BOOLEAN NOT NULL DEFAULT FALSE,
  syntax_cache TEXT NOT NULL DEFAULT '',
  file_cache TEXT NOT NULL DEFAULT '',
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, snapshot_id, path)
);

INSERT INTO codeindex_sources_scoped (snapshot_id, path, hash, size, content, language, input_blob, dirty, syntax_cache, file_cache, org_id)
  SELECT snapshot_id, path, hash, size, content, language, input_blob, dirty, syntax_cache, file_cache, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_sources;

DROP TABLE codeindex_sources;

ALTER TABLE codeindex_sources_scoped RENAME TO codeindex_sources;

CREATE TABLE codeindex_facts_scoped (
  id TEXT NOT NULL,
  repository_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  language TEXT NOT NULL DEFAULT '',
  kind INTEGER NOT NULL DEFAULT 0,
  name TEXT NOT NULL DEFAULT '',
  qualified_name TEXT NOT NULL DEFAULT '',
  symbol_key TEXT NOT NULL DEFAULT '',
  signature TEXT NOT NULL DEFAULT '',
  documentation TEXT NOT NULL DEFAULT '',
  code TEXT NOT NULL DEFAULT '',
  parent_fact_id TEXT NOT NULL DEFAULT '',
  logical_key TEXT NOT NULL DEFAULT '',
  path TEXT NOT NULL DEFAULT '',
  anchor_json TEXT NOT NULL DEFAULT 'null',
  evidence_json TEXT NOT NULL DEFAULT '[]',
  imports_json TEXT NOT NULL DEFAULT '[]',
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

INSERT INTO codeindex_facts_scoped (id, repository_id, snapshot_id, language, kind, name, qualified_name, symbol_key, signature, documentation, code, parent_fact_id, logical_key, path, anchor_json, evidence_json, imports_json, org_id)
  SELECT id, repository_id, snapshot_id, language, kind, name, qualified_name, symbol_key, signature, documentation, code, parent_fact_id, logical_key, path, anchor_json, evidence_json, imports_json, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_facts;

DROP TABLE codeindex_facts;

ALTER TABLE codeindex_facts_scoped RENAME TO codeindex_facts;

CREATE TABLE codeindex_chunks_scoped (
  id TEXT NOT NULL,
  fact_id TEXT NOT NULL DEFAULT '',
  snapshot_id TEXT NOT NULL,
  anchor_json TEXT NOT NULL DEFAULT 'null',
  text TEXT NOT NULL DEFAULT '',
  context TEXT NOT NULL DEFAULT '',
  idx INTEGER NOT NULL DEFAULT 0,
  total INTEGER NOT NULL DEFAULT 0,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

INSERT INTO codeindex_chunks_scoped (id, fact_id, snapshot_id, anchor_json, text, context, idx, total, org_id)
  SELECT id, fact_id, snapshot_id, anchor_json, text, context, idx, total, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_chunks;

DROP TABLE codeindex_chunks;

ALTER TABLE codeindex_chunks_scoped RENAME TO codeindex_chunks;

CREATE TABLE codeindex_edges_scoped (
  id TEXT NOT NULL,
  repository_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  kind INTEGER NOT NULL DEFAULT 0,
  from_fact_id TEXT NOT NULL DEFAULT '',
  to_fact_id TEXT NOT NULL DEFAULT '',
  target_symbol_key TEXT NOT NULL DEFAULT '',
  logical_key TEXT NOT NULL DEFAULT '',
  weight REAL NOT NULL DEFAULT 0,
  anchor_json TEXT NOT NULL DEFAULT 'null',
  evidence_json TEXT NOT NULL DEFAULT '[]',
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

INSERT INTO codeindex_edges_scoped (id, repository_id, snapshot_id, kind, from_fact_id, to_fact_id, target_symbol_key, logical_key, weight, anchor_json, evidence_json, org_id)
  SELECT id, repository_id, snapshot_id, kind, from_fact_id, to_fact_id, target_symbol_key, logical_key, weight, anchor_json, evidence_json, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_edges;

DROP TABLE codeindex_edges;

ALTER TABLE codeindex_edges_scoped RENAME TO codeindex_edges;

CREATE TABLE codeindex_analysis_runs_scoped (
  id TEXT NOT NULL,
  repository_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  algorithm TEXT NOT NULL DEFAULT '',
  params_json TEXT NOT NULL DEFAULT '{}',
  created_unix INTEGER NOT NULL DEFAULT 0,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

INSERT INTO codeindex_analysis_runs_scoped (id, repository_id, snapshot_id, algorithm, params_json, created_unix, org_id)
  SELECT id, repository_id, snapshot_id, algorithm, params_json, created_unix, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_analysis_runs;

DROP TABLE codeindex_analysis_runs;

ALTER TABLE codeindex_analysis_runs_scoped RENAME TO codeindex_analysis_runs;

CREATE TABLE codeindex_groups_scoped (
  id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  kind INTEGER NOT NULL DEFAULT 0,
  size INTEGER NOT NULL DEFAULT 0,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

INSERT INTO codeindex_groups_scoped (id, run_id, snapshot_id, label, kind, size, org_id)
  SELECT id, run_id, snapshot_id, label, kind, size, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_groups;

DROP TABLE codeindex_groups;

ALTER TABLE codeindex_groups_scoped RENAME TO codeindex_groups;

CREATE TABLE codeindex_group_members_scoped (
  group_id TEXT NOT NULL,
  fact_id TEXT NOT NULL,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, group_id, fact_id)
);

INSERT INTO codeindex_group_members_scoped (group_id, fact_id, org_id)
  SELECT group_id, fact_id, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_group_members;

DROP TABLE codeindex_group_members;

ALTER TABLE codeindex_group_members_scoped RENAME TO codeindex_group_members;

CREATE TABLE codeindex_elements_scoped (
  logical_key TEXT NOT NULL,
  resource_type TEXT NOT NULL DEFAULT 'element',
  resource_id INTEGER NOT NULL DEFAULT 0,
  repository_id TEXT NOT NULL DEFAULT '',
  snapshot_id TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, logical_key)
);

INSERT INTO codeindex_elements_scoped (logical_key, resource_type, resource_id, repository_id, snapshot_id, updated_at, org_id)
  SELECT logical_key, resource_type, resource_id, repository_id, snapshot_id, updated_at, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_elements;

DROP TABLE codeindex_elements;

ALTER TABLE codeindex_elements_scoped RENAME TO codeindex_elements;

CREATE TABLE codeindex_completed_maps_scoped (
  run_id TEXT NOT NULL,
  repository_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  config_hash TEXT NOT NULL,
  completed_unix BIGINT NOT NULL,
  result_json TEXT NOT NULL,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, run_id)
);

INSERT INTO codeindex_completed_maps_scoped (run_id, repository_id, snapshot_id, config_hash, completed_unix, result_json, org_id)
  SELECT run_id, repository_id, snapshot_id, config_hash, completed_unix, result_json, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_completed_maps;

DROP TABLE codeindex_completed_maps;

ALTER TABLE codeindex_completed_maps_scoped RENAME TO codeindex_completed_maps;

CREATE TABLE codeindex_impacts_scoped (
  repository_id TEXT NOT NULL,
  comparison_key TEXT NOT NULL,
  result_json TEXT NOT NULL,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, repository_id, comparison_key)
);

INSERT INTO codeindex_impacts_scoped (repository_id, comparison_key, result_json, org_id)
  SELECT repository_id, comparison_key, result_json, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_impacts;

DROP TABLE codeindex_impacts;

ALTER TABLE codeindex_impacts_scoped RENAME TO codeindex_impacts;

CREATE TABLE codeindex_leases_scoped (
  repository_id TEXT NOT NULL,
  owner TEXT NOT NULL,
  expires_unix BIGINT NOT NULL,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, repository_id)
);

INSERT INTO codeindex_leases_scoped (repository_id, owner, expires_unix, org_id)
  SELECT repository_id, owner, expires_unix, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_leases;

DROP TABLE codeindex_leases;

ALTER TABLE codeindex_leases_scoped RENAME TO codeindex_leases;

CREATE TABLE codeindex_watch_state_scoped (
  repository_id TEXT NOT NULL,
  heartbeat_unix BIGINT NOT NULL DEFAULT 0,
  error TEXT NOT NULL DEFAULT '',
  git_branch TEXT NOT NULL DEFAULT '',
  git_revision TEXT NOT NULL DEFAULT '',
  owner_kind TEXT NOT NULL DEFAULT '',
  owner_pid INTEGER NOT NULL DEFAULT 0,
  owner_id TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT '',
  stage TEXT NOT NULL DEFAULT '',
  repo_root TEXT NOT NULL DEFAULT '',
  snapshot_id TEXT NOT NULL DEFAULT '',
  content_fingerprint TEXT NOT NULL DEFAULT '',
  changed_files INTEGER NOT NULL DEFAULT 0,
  pending_files INTEGER NOT NULL DEFAULT 0,
  started_unix BIGINT NOT NULL DEFAULT 0,
  last_scan_unix BIGINT NOT NULL DEFAULT 0,
  last_scan_ms INTEGER NOT NULL DEFAULT 0,
  stop_requested BOOLEAN NOT NULL DEFAULT FALSE,
  poll_interval_ms INTEGER NOT NULL DEFAULT 0,
  debounce_ms INTEGER NOT NULL DEFAULT 0,
  stop_requested_unix BIGINT NOT NULL DEFAULT 0,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, repository_id)
);

INSERT INTO codeindex_watch_state_scoped (repository_id, heartbeat_unix, error, git_branch, git_revision, owner_kind, owner_pid, owner_id, state, stage, repo_root, snapshot_id, content_fingerprint, changed_files, pending_files, started_unix, last_scan_unix, last_scan_ms, stop_requested, poll_interval_ms, debounce_ms, stop_requested_unix, org_id)
  SELECT repository_id, heartbeat_unix, error, git_branch, git_revision, owner_kind, owner_pid, owner_id, state, stage, repo_root, snapshot_id, content_fingerprint, changed_files, pending_files, started_unix, last_scan_unix, last_scan_ms, stop_requested, poll_interval_ms, debounce_ms, stop_requested_unix, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_watch_state;

DROP TABLE codeindex_watch_state;

ALTER TABLE codeindex_watch_state_scoped RENAME TO codeindex_watch_state;

CREATE TABLE codeindex_project_artifacts_scoped (
  snapshot_id TEXT NOT NULL,
  project_key TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  data BLOB NOT NULL,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, snapshot_id, project_key)
);

INSERT INTO codeindex_project_artifacts_scoped (snapshot_id, project_key, fingerprint, data, org_id)
  SELECT snapshot_id, project_key, fingerprint, data, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_project_artifacts;

DROP TABLE codeindex_project_artifacts;

ALTER TABLE codeindex_project_artifacts_scoped RENAME TO codeindex_project_artifacts;

CREATE TABLE codeindex_snapshot_facts_scoped (
  snapshot_id TEXT NOT NULL,
  fact_id TEXT NOT NULL,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, snapshot_id, fact_id)
);

INSERT INTO codeindex_snapshot_facts_scoped (snapshot_id, fact_id, org_id)
  SELECT snapshot_id, fact_id, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_snapshot_facts;

DROP TABLE codeindex_snapshot_facts;

ALTER TABLE codeindex_snapshot_facts_scoped RENAME TO codeindex_snapshot_facts;

CREATE TABLE codeindex_snapshot_chunks_scoped (
  snapshot_id TEXT NOT NULL,
  chunk_id TEXT NOT NULL,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, snapshot_id, chunk_id)
);

INSERT INTO codeindex_snapshot_chunks_scoped (snapshot_id, chunk_id, org_id)
  SELECT snapshot_id, chunk_id, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_snapshot_chunks;

DROP TABLE codeindex_snapshot_chunks;

ALTER TABLE codeindex_snapshot_chunks_scoped RENAME TO codeindex_snapshot_chunks;

CREATE TABLE codeindex_snapshot_edges_scoped (
  snapshot_id TEXT NOT NULL,
  edge_id TEXT NOT NULL,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, snapshot_id, edge_id)
);

INSERT INTO codeindex_snapshot_edges_scoped (snapshot_id, edge_id, org_id)
  SELECT snapshot_id, edge_id, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_snapshot_edges;

DROP TABLE codeindex_snapshot_edges;

ALTER TABLE codeindex_snapshot_edges_scoped RENAME TO codeindex_snapshot_edges;

CREATE TABLE codeindex_repository_settings_scoped (
  repository_id TEXT NOT NULL,
  map_overrides TEXT NOT NULL DEFAULT '{}',
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, repository_id),
  FOREIGN KEY (org_id, repository_id) REFERENCES codeindex_repositories(org_id, id) ON DELETE CASCADE
);

INSERT INTO codeindex_repository_settings_scoped (repository_id, map_overrides, org_id)
  SELECT repository_id, map_overrides, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_repository_settings_backup;

DROP TABLE codeindex_repository_settings_backup;

ALTER TABLE codeindex_repository_settings_scoped RENAME TO codeindex_repository_settings;

CREATE TABLE codeindex_active_maps_scoped (
  repository_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, repository_id),
  FOREIGN KEY (org_id, run_id) REFERENCES codeindex_completed_maps(org_id, run_id) ON DELETE CASCADE
);

INSERT INTO codeindex_active_maps_scoped (repository_id, run_id, org_id)
  SELECT repository_id, run_id, COALESCE(org_id, '00000000-0000-0000-0000-000000000000') FROM codeindex_active_maps_backup;

DROP TABLE codeindex_active_maps_backup;

ALTER TABLE codeindex_active_maps_scoped RENAME TO codeindex_active_maps;

CREATE INDEX IF NOT EXISTS idx_codeindex_repositories_remote_key
  ON codeindex_repositories(remote_key);

CREATE INDEX IF NOT EXISTS idx_codeindex_snapshots_repository
  ON codeindex_snapshots(repository_id, created_unix);

CREATE INDEX IF NOT EXISTS idx_codeindex_sources_hash
  ON codeindex_sources(hash);

CREATE INDEX IF NOT EXISTS idx_codeindex_facts_snapshot ON codeindex_facts(snapshot_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_facts_logical ON codeindex_facts(snapshot_id, logical_key);

CREATE INDEX IF NOT EXISTS idx_codeindex_facts_symbol ON codeindex_facts(snapshot_id, symbol_key);

CREATE INDEX IF NOT EXISTS idx_codeindex_facts_path ON codeindex_facts(snapshot_id, path);

CREATE INDEX IF NOT EXISTS idx_codeindex_chunks_snapshot ON codeindex_chunks(snapshot_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_chunks_fact ON codeindex_chunks(fact_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_edges_snapshot ON codeindex_edges(snapshot_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_edges_logical ON codeindex_edges(snapshot_id, logical_key);

CREATE INDEX IF NOT EXISTS idx_codeindex_edges_from ON codeindex_edges(from_fact_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_edges_to ON codeindex_edges(to_fact_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_elements_snapshot ON codeindex_elements(snapshot_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_elements_resource ON codeindex_elements(resource_type, resource_id);

CREATE INDEX idx_codeindex_snapshot_revision ON codeindex_snapshots(repository_id, git_revision, provenance, config_hash);

CREATE INDEX idx_codeindex_completed_maps_repository ON codeindex_completed_maps(repository_id, completed_unix);

CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_facts_fact ON codeindex_snapshot_facts(fact_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_chunks_chunk ON codeindex_snapshot_chunks(chunk_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_edges_edge ON codeindex_snapshot_edges(edge_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_repositories_org_id ON codeindex_repositories(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_snapshots_org_id ON codeindex_snapshots(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_sources_org_id ON codeindex_sources(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_facts_org_id ON codeindex_facts(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_chunks_org_id ON codeindex_chunks(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_edges_org_id ON codeindex_edges(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_analysis_runs_org_id ON codeindex_analysis_runs(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_groups_org_id ON codeindex_groups(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_group_members_org_id ON codeindex_group_members(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_elements_org_id ON codeindex_elements(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_completed_maps_org_id ON codeindex_completed_maps(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_impacts_org_id ON codeindex_impacts(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_leases_org_id ON codeindex_leases(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_watch_state_org_id ON codeindex_watch_state(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_project_artifacts_org_id ON codeindex_project_artifacts(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_facts_org_id ON codeindex_snapshot_facts(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_chunks_org_id ON codeindex_snapshot_chunks(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_edges_org_id ON codeindex_snapshot_edges(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_repository_settings_org_id ON codeindex_repository_settings(org_id);

CREATE INDEX IF NOT EXISTS idx_codeindex_active_maps_org_id ON codeindex_active_maps(org_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_codeindex_repositories_org_remote_key_unique
  ON codeindex_repositories(COALESCE(org_id, ''), remote_key)
  WHERE remote_key <> '';
