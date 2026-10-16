-- Compact the SQLite no-org sentinel in rows, composite keys and defaults.
-- Real tenant IDs are preserved. PostgreSQL keeps native UUID columns.
-- The SQLite migrator commits this table rebuild and its history atomically.

DROP VIEW codeindex_snapshot_facts;
DROP VIEW codeindex_snapshot_edges;

CREATE TEMP TABLE saved_codeindex_analysis_runs_org AS SELECT * FROM codeindex_analysis_runs;
CREATE TEMP TABLE saved_codeindex_artifacts_org AS SELECT * FROM codeindex_artifacts;
CREATE TEMP TABLE saved_codeindex_blobs_org AS SELECT * FROM codeindex_blobs;
CREATE TEMP TABLE saved_codeindex_completed_maps_org AS SELECT * FROM codeindex_completed_maps;
CREATE TEMP TABLE saved_codeindex_edges_org AS SELECT * FROM codeindex_edges;
CREATE TEMP TABLE saved_codeindex_elements_org AS SELECT * FROM codeindex_elements;
CREATE TEMP TABLE saved_codeindex_facts_org AS SELECT * FROM codeindex_facts;
CREATE TEMP TABLE saved_codeindex_group_members_org AS SELECT * FROM codeindex_group_members;
CREATE TEMP TABLE saved_codeindex_groups_org AS SELECT * FROM codeindex_groups;
CREATE TEMP TABLE saved_codeindex_impacts_org AS SELECT * FROM codeindex_impacts;
CREATE TEMP TABLE saved_codeindex_leases_org AS SELECT * FROM codeindex_leases;
CREATE TEMP TABLE saved_codeindex_project_artifacts_org AS SELECT * FROM codeindex_project_artifacts;
CREATE TEMP TABLE saved_codeindex_repositories_org AS SELECT * FROM codeindex_repositories;
CREATE TEMP TABLE saved_codeindex_snapshot_edges_org AS SELECT * FROM codeindex_snapshot_edges_direct;
CREATE TEMP TABLE saved_codeindex_snapshot_facts_org AS SELECT * FROM codeindex_snapshot_facts_direct;
CREATE TEMP TABLE saved_codeindex_snapshots_org AS SELECT * FROM codeindex_snapshots;
CREATE TEMP TABLE saved_codeindex_sources_org AS SELECT * FROM codeindex_sources;
CREATE TEMP TABLE saved_codeindex_watch_state_org AS SELECT * FROM codeindex_watch_state;
CREATE TEMP TABLE saved_tags_org AS SELECT * FROM tags;
CREATE TEMP TABLE saved_codeindex_active_maps_org AS SELECT * FROM codeindex_active_maps;
CREATE TEMP TABLE saved_codeindex_repository_settings_org AS SELECT * FROM codeindex_repository_settings;

DROP TABLE codeindex_repository_settings;
DROP TABLE codeindex_active_maps;
DROP TABLE tags;
DROP TABLE codeindex_watch_state;
DROP TABLE codeindex_sources;
DROP TABLE codeindex_snapshots;
DROP TABLE codeindex_snapshot_facts_direct;
DROP TABLE codeindex_snapshot_edges_direct;
DROP TABLE codeindex_repositories;
DROP TABLE codeindex_project_artifacts;
DROP TABLE codeindex_leases;
DROP TABLE codeindex_impacts;
DROP TABLE codeindex_groups;
DROP TABLE codeindex_group_members;
DROP TABLE codeindex_facts;
DROP TABLE codeindex_elements;
DROP TABLE codeindex_edges;
DROP TABLE codeindex_completed_maps;
DROP TABLE codeindex_blobs;
DROP TABLE codeindex_artifacts;
DROP TABLE codeindex_analysis_runs;

CREATE TABLE codeindex_analysis_runs (
  id INTEGER NOT NULL,
  repository_id INTEGER NOT NULL,
  snapshot_id INTEGER NOT NULL,
  algorithm TEXT NOT NULL DEFAULT '',
  params_json TEXT NOT NULL DEFAULT '{}',
  created_unix INTEGER NOT NULL DEFAULT 0,
  org_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, id)
);
INSERT INTO codeindex_analysis_runs (id, repository_id, snapshot_id, algorithm, params_json, created_unix, org_id)
  SELECT id, repository_id, snapshot_id, algorithm, params_json, created_unix, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END
  FROM saved_codeindex_analysis_runs_org;
DROP TABLE saved_codeindex_analysis_runs_org;
CREATE INDEX idx_codeindex_analysis_runs_org_id ON codeindex_analysis_runs(org_id);

CREATE TABLE codeindex_artifacts (
  org_id TEXT NOT NULL DEFAULT '',
  project_key TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  data BLOB NOT NULL,
  PRIMARY KEY (org_id, project_key, fingerprint)
);
INSERT INTO codeindex_artifacts (org_id, project_key, fingerprint, data)
  SELECT CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END, project_key, fingerprint, data
  FROM saved_codeindex_artifacts_org;
DROP TABLE saved_codeindex_artifacts_org;

CREATE TABLE "codeindex_blobs" (
  org_id TEXT NOT NULL DEFAULT '',
  hash TEXT NOT NULL,
  language TEXT NOT NULL DEFAULT '',
  content BLOB,
  syntax_cache BLOB NOT NULL DEFAULT X'',
  file_cache BLOB NOT NULL DEFAULT X'',
  compression INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (org_id, hash, language)
);
INSERT INTO codeindex_blobs (org_id, hash, language, content, syntax_cache, file_cache, compression)
  SELECT CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END, hash, language, content, syntax_cache, file_cache, compression
  FROM saved_codeindex_blobs_org;
DROP TABLE saved_codeindex_blobs_org;

CREATE TABLE codeindex_completed_maps (
  run_id INTEGER NOT NULL,
  repository_id INTEGER NOT NULL,
  snapshot_id INTEGER NOT NULL,
  config_hash TEXT NOT NULL,
  completed_unix BIGINT NOT NULL,
  result_json TEXT NOT NULL,
  org_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, run_id)
);
INSERT INTO codeindex_completed_maps (run_id, repository_id, snapshot_id, config_hash, completed_unix, result_json, org_id)
  SELECT run_id, repository_id, snapshot_id, config_hash, completed_unix, result_json, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END
  FROM saved_codeindex_completed_maps_org;
DROP TABLE saved_codeindex_completed_maps_org;
CREATE INDEX idx_codeindex_completed_maps_org_id ON codeindex_completed_maps(org_id);
CREATE INDEX idx_codeindex_completed_maps_repository
  ON codeindex_completed_maps(repository_id, completed_unix);

CREATE TABLE codeindex_edges (
  id INTEGER NOT NULL,
  repository_id INTEGER NOT NULL,
  snapshot_id INTEGER NOT NULL,
  kind INTEGER NOT NULL DEFAULT 0,
  from_fact_id INTEGER NOT NULL DEFAULT 0,
  to_fact_id INTEGER NOT NULL DEFAULT 0,
  target_symbol_key TEXT NOT NULL DEFAULT '',
  logical_key BLOB NOT NULL CHECK (typeof(logical_key) = 'blob' AND length(logical_key) = 32),
  weight REAL NOT NULL DEFAULT 0,
  org_id TEXT NOT NULL DEFAULT '',
  content_hash TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, id)
);
INSERT INTO codeindex_edges (id, repository_id, snapshot_id, kind, from_fact_id, to_fact_id, target_symbol_key, logical_key, weight, org_id, content_hash)
  SELECT id, repository_id, snapshot_id, kind, from_fact_id, to_fact_id, target_symbol_key, logical_key, weight, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END, content_hash
  FROM saved_codeindex_edges_org;
DROP TABLE saved_codeindex_edges_org;
CREATE INDEX idx_codeindex_edges_from ON codeindex_edges(from_fact_id);
CREATE INDEX idx_codeindex_edges_logical ON codeindex_edges(org_id, logical_key, snapshot_id);
CREATE INDEX idx_codeindex_edges_org_id ON codeindex_edges(org_id);
CREATE INDEX idx_codeindex_edges_snapshot ON codeindex_edges(snapshot_id);
CREATE INDEX idx_codeindex_edges_to ON codeindex_edges(to_fact_id);
CREATE INDEX idx_codeindex_edges_content ON codeindex_edges(org_id, repository_id, content_hash);

CREATE TABLE codeindex_elements (
  logical_key TEXT NOT NULL,
  resource_type TEXT NOT NULL DEFAULT 'element',
  resource_id INTEGER NOT NULL DEFAULT 0,
  repository_id INTEGER NOT NULL DEFAULT 0,
  snapshot_id INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL DEFAULT '',
  org_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, logical_key)
);
INSERT INTO codeindex_elements (logical_key, resource_type, resource_id, repository_id, snapshot_id, updated_at, org_id)
  SELECT logical_key, resource_type, resource_id, repository_id, snapshot_id, updated_at, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END
  FROM saved_codeindex_elements_org;
DROP TABLE saved_codeindex_elements_org;
CREATE INDEX idx_codeindex_elements_org_id ON codeindex_elements(org_id);
CREATE INDEX idx_codeindex_elements_resource ON codeindex_elements(resource_type, resource_id);
CREATE INDEX idx_codeindex_elements_snapshot ON codeindex_elements(snapshot_id);

CREATE TABLE codeindex_facts (
  id INTEGER NOT NULL,
  repository_id INTEGER NOT NULL,
  snapshot_id INTEGER NOT NULL,
  language TEXT NOT NULL DEFAULT '',
  kind INTEGER NOT NULL DEFAULT 0,
  name TEXT NOT NULL DEFAULT '',
  body_hash TEXT NOT NULL DEFAULT '',
  qualified_name TEXT NOT NULL DEFAULT '',
  symbol_key TEXT NOT NULL DEFAULT '',
  signature TEXT NOT NULL DEFAULT '',
  documentation TEXT NOT NULL DEFAULT '',
  parent_fact_id INTEGER NOT NULL DEFAULT 0,
  logical_key TEXT NOT NULL DEFAULT '',
  path TEXT NOT NULL DEFAULT '',
  anchor_json TEXT NOT NULL DEFAULT 'null',
  evidence_json TEXT NOT NULL DEFAULT '[]',
  imports_json TEXT NOT NULL DEFAULT '[]',
  org_id TEXT NOT NULL DEFAULT '',
  content_hash TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, id)
);
INSERT INTO codeindex_facts (id, repository_id, snapshot_id, language, kind, name, body_hash, qualified_name, symbol_key, signature, documentation, parent_fact_id, logical_key, path, anchor_json, evidence_json, imports_json, org_id, content_hash)
  SELECT id, repository_id, snapshot_id, language, kind, name, body_hash, qualified_name, symbol_key, signature, documentation, parent_fact_id, logical_key, path, anchor_json, evidence_json, imports_json, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END, content_hash
  FROM saved_codeindex_facts_org;
DROP TABLE saved_codeindex_facts_org;
CREATE INDEX idx_codeindex_facts_logical ON codeindex_facts(snapshot_id, logical_key);
CREATE INDEX idx_codeindex_facts_org_id ON codeindex_facts(org_id);
CREATE INDEX idx_codeindex_facts_path ON codeindex_facts(snapshot_id, path);
CREATE INDEX idx_codeindex_facts_snapshot ON codeindex_facts(snapshot_id);
CREATE INDEX idx_codeindex_facts_symbol ON codeindex_facts(snapshot_id, symbol_key);
CREATE INDEX idx_codeindex_facts_content ON codeindex_facts(org_id, repository_id, content_hash);

CREATE TABLE codeindex_group_members (
  group_id INTEGER NOT NULL,
  fact_id INTEGER NOT NULL,
  org_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, group_id, fact_id)
);
INSERT INTO codeindex_group_members (group_id, fact_id, org_id)
  SELECT group_id, fact_id, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END
  FROM saved_codeindex_group_members_org;
DROP TABLE saved_codeindex_group_members_org;
CREATE INDEX idx_codeindex_group_members_org_id ON codeindex_group_members(org_id);

CREATE TABLE codeindex_groups (
  id INTEGER NOT NULL,
  run_id INTEGER NOT NULL,
  snapshot_id INTEGER NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  kind INTEGER NOT NULL DEFAULT 0,
  size INTEGER NOT NULL DEFAULT 0,
  org_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, id)
);
INSERT INTO codeindex_groups (id, run_id, snapshot_id, label, kind, size, org_id)
  SELECT id, run_id, snapshot_id, label, kind, size, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END
  FROM saved_codeindex_groups_org;
DROP TABLE saved_codeindex_groups_org;
CREATE INDEX idx_codeindex_groups_org_id ON codeindex_groups(org_id);

CREATE TABLE codeindex_impacts (
  repository_id INTEGER NOT NULL,
  comparison_key TEXT NOT NULL,
  result_json BLOB NOT NULL,
  org_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, repository_id, comparison_key)
);
INSERT INTO codeindex_impacts (repository_id, comparison_key, result_json, org_id)
  SELECT repository_id, comparison_key, result_json, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END
  FROM saved_codeindex_impacts_org;
DROP TABLE saved_codeindex_impacts_org;
CREATE INDEX idx_codeindex_impacts_org_id ON codeindex_impacts(org_id);

CREATE TABLE codeindex_leases (
  repository_id TEXT NOT NULL,
  owner TEXT NOT NULL,
  expires_unix BIGINT NOT NULL,
  org_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, repository_id)
);
INSERT INTO codeindex_leases (repository_id, owner, expires_unix, org_id)
  SELECT repository_id, owner, expires_unix, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END
  FROM saved_codeindex_leases_org;
DROP TABLE saved_codeindex_leases_org;
CREATE INDEX idx_codeindex_leases_org_id ON codeindex_leases(org_id);

CREATE TABLE codeindex_project_artifacts (
  snapshot_id INTEGER NOT NULL,
  project_key TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  org_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, snapshot_id, project_key)
);
INSERT INTO codeindex_project_artifacts (snapshot_id, project_key, fingerprint, org_id)
  SELECT snapshot_id, project_key, fingerprint, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END
  FROM saved_codeindex_project_artifacts_org;
DROP TABLE saved_codeindex_project_artifacts_org;
CREATE INDEX idx_codeindex_project_artifacts_key ON codeindex_project_artifacts(org_id, project_key, fingerprint);
CREATE INDEX idx_codeindex_project_artifacts_org_id ON codeindex_project_artifacts(org_id);

CREATE TABLE codeindex_repositories (
  id INTEGER NOT NULL,
  mapping_namespace TEXT NOT NULL DEFAULT '',
  root TEXT NOT NULL,
  remote_url TEXT NOT NULL DEFAULT '',
  remote_key TEXT NOT NULL DEFAULT '',
  managed BOOLEAN NOT NULL DEFAULT FALSE,
  latest_snapshot_id INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  org_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, id)
);
INSERT INTO codeindex_repositories (id, mapping_namespace, root, remote_url, remote_key, managed, latest_snapshot_id, created_at, updated_at, org_id)
  SELECT id, mapping_namespace, root, remote_url, remote_key, managed, latest_snapshot_id, created_at, updated_at, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END
  FROM saved_codeindex_repositories_org;
DROP TABLE saved_codeindex_repositories_org;
CREATE INDEX idx_codeindex_repositories_org_id
  ON codeindex_repositories(org_id);
CREATE UNIQUE INDEX idx_codeindex_repositories_org_remote_key_unique
  ON codeindex_repositories(org_id, remote_key) WHERE remote_key <> '';
CREATE INDEX idx_codeindex_repositories_remote_key
  ON codeindex_repositories(remote_key);

CREATE TABLE codeindex_snapshot_edges_direct (
  snapshot_id INTEGER NOT NULL,
  edge_id INTEGER NOT NULL,
  org_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, snapshot_id, edge_id)
);
INSERT INTO codeindex_snapshot_edges_direct (snapshot_id, edge_id, org_id)
  SELECT snapshot_id, edge_id, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END
  FROM saved_codeindex_snapshot_edges_org;
DROP TABLE saved_codeindex_snapshot_edges_org;
CREATE INDEX idx_codeindex_snapshot_edges_edge ON codeindex_snapshot_edges_direct(edge_id);
CREATE INDEX idx_codeindex_snapshot_edges_org_id ON codeindex_snapshot_edges_direct(org_id);

CREATE TABLE codeindex_snapshot_facts_direct (
  snapshot_id INTEGER NOT NULL,
  fact_id INTEGER NOT NULL,
  org_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, snapshot_id, fact_id)
);
INSERT INTO codeindex_snapshot_facts_direct (snapshot_id, fact_id, org_id)
  SELECT snapshot_id, fact_id, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END
  FROM saved_codeindex_snapshot_facts_org;
DROP TABLE saved_codeindex_snapshot_facts_org;
CREATE INDEX idx_codeindex_snapshot_facts_fact ON codeindex_snapshot_facts_direct(fact_id);
CREATE INDEX idx_codeindex_snapshot_facts_org_id ON codeindex_snapshot_facts_direct(org_id);

CREATE TABLE codeindex_snapshots (
  id INTEGER NOT NULL,
  repository_id INTEGER NOT NULL,
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
  manifest_json TEXT NOT NULL DEFAULT '{}',
  org_id TEXT NOT NULL DEFAULT '',
  symbol_versions_json TEXT NOT NULL DEFAULT '{}',
  fact_count INTEGER,
  edge_count INTEGER,
  source_count INTEGER,
  PRIMARY KEY (org_id, id)
);
INSERT INTO codeindex_snapshots (id, repository_id, created_unix, git_revision, git_branch, ingestion_status, config_hash, projects_json, warnings_json, tool_versions_json, provenance, content_fingerprint, capture_order, commit_message, manifest_json, org_id, symbol_versions_json, fact_count, edge_count, source_count)
  SELECT id, repository_id, created_unix, git_revision, git_branch, ingestion_status, config_hash, projects_json, warnings_json, tool_versions_json, provenance, content_fingerprint, capture_order, commit_message, manifest_json, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END, symbol_versions_json, fact_count, edge_count, source_count
  FROM saved_codeindex_snapshots_org;
DROP TABLE saved_codeindex_snapshots_org;
CREATE INDEX idx_codeindex_snapshot_revision
  ON codeindex_snapshots(repository_id, git_revision, provenance, config_hash);
CREATE INDEX idx_codeindex_snapshots_org_id
  ON codeindex_snapshots(org_id);
CREATE INDEX idx_codeindex_snapshots_repository
  ON codeindex_snapshots(repository_id, created_unix);

CREATE TABLE codeindex_sources (
  snapshot_id INTEGER NOT NULL,
  path TEXT NOT NULL,
  hash TEXT NOT NULL,
  size INTEGER NOT NULL DEFAULT 0,
  language TEXT NOT NULL DEFAULT '',
  input_blob TEXT NOT NULL DEFAULT '',
  dirty BOOLEAN NOT NULL DEFAULT FALSE,
  org_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, snapshot_id, path)
);
INSERT INTO codeindex_sources (snapshot_id, path, hash, size, language, input_blob, dirty, org_id)
  SELECT snapshot_id, path, hash, size, language, input_blob, dirty, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END
  FROM saved_codeindex_sources_org;
DROP TABLE saved_codeindex_sources_org;
CREATE INDEX idx_codeindex_sources_hash
  ON codeindex_sources(hash);
CREATE INDEX idx_codeindex_sources_org_id
  ON codeindex_sources(org_id);

CREATE TABLE codeindex_watch_state (
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
  org_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, repository_id)
);
INSERT INTO codeindex_watch_state (repository_id, heartbeat_unix, error, git_branch, git_revision, owner_kind, owner_pid, owner_id, state, stage, repo_root, snapshot_id, content_fingerprint, changed_files, pending_files, started_unix, last_scan_unix, last_scan_ms, stop_requested, poll_interval_ms, debounce_ms, stop_requested_unix, org_id)
  SELECT repository_id, heartbeat_unix, error, git_branch, git_revision, owner_kind, owner_pid, owner_id, state, stage, repo_root, snapshot_id, content_fingerprint, changed_files, pending_files, started_unix, last_scan_unix, last_scan_ms, stop_requested, poll_interval_ms, debounce_ms, stop_requested_unix, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END
  FROM saved_codeindex_watch_state_org;
DROP TABLE saved_codeindex_watch_state_org;
CREATE INDEX idx_codeindex_watch_state_org_id ON codeindex_watch_state(org_id);

CREATE TABLE "tags" (
  org_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  color TEXT NOT NULL,
  description TEXT NULL,
  PRIMARY KEY (org_id, name)
);
INSERT INTO tags (org_id, name, color, description)
  SELECT CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END, name, color, description
  FROM saved_tags_org;
DROP TABLE saved_tags_org;

CREATE TABLE codeindex_active_maps (
  repository_id INTEGER NOT NULL,
  run_id INTEGER NOT NULL,
  org_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, repository_id),
  FOREIGN KEY (org_id, run_id) REFERENCES codeindex_completed_maps(org_id, run_id) ON DELETE CASCADE
);
INSERT INTO codeindex_active_maps (repository_id, run_id, org_id)
  SELECT repository_id, run_id, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END
  FROM saved_codeindex_active_maps_org;
DROP TABLE saved_codeindex_active_maps_org;
CREATE INDEX idx_codeindex_active_maps_org_id ON codeindex_active_maps(org_id);

CREATE TABLE codeindex_repository_settings (
  repository_id INTEGER NOT NULL,
  map_overrides TEXT NOT NULL DEFAULT '{}',
  org_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, repository_id),
  FOREIGN KEY (org_id, repository_id) REFERENCES codeindex_repositories(org_id, id) ON DELETE CASCADE
);
INSERT INTO codeindex_repository_settings (repository_id, map_overrides, org_id)
  SELECT repository_id, map_overrides, CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN '' ELSE org_id END
  FROM saved_codeindex_repository_settings_org;
DROP TABLE saved_codeindex_repository_settings_org;
CREATE INDEX idx_codeindex_repository_settings_org_id ON codeindex_repository_settings(org_id);

-- These tables have no org-id defaults to rebuild. Normalize their keys in place.
UPDATE codeindex_membership_groups SET org_id = '' WHERE org_id = '00000000-0000-0000-0000-000000000000';
UPDATE codeindex_membership_facts SET org_id = '' WHERE org_id = '00000000-0000-0000-0000-000000000000';
UPDATE codeindex_membership_edges SET org_id = '' WHERE org_id = '00000000-0000-0000-0000-000000000000';
UPDATE codeindex_snapshot_membership SET org_id = '' WHERE org_id = '00000000-0000-0000-0000-000000000000';

CREATE INDEX idx_codeindex_snapshot_facts_direct_snapshot ON codeindex_snapshot_facts_direct(snapshot_id);
CREATE INDEX idx_codeindex_snapshot_edges_direct_snapshot ON codeindex_snapshot_edges_direct(snapshot_id);

CREATE VIEW codeindex_snapshot_facts AS
  SELECT snapshot_id, fact_id, org_id FROM codeindex_snapshot_facts_direct
  UNION ALL
  SELECT s.snapshot_id, f.fact_id, s.org_id FROM codeindex_snapshot_membership s
  JOIN codeindex_membership_facts f ON f.org_id=s.org_id AND f.group_key=s.group_key;
CREATE VIEW codeindex_snapshot_edges AS
  SELECT snapshot_id, edge_id, org_id FROM codeindex_snapshot_edges_direct
  UNION ALL
  SELECT s.snapshot_id, e.edge_id, s.org_id FROM codeindex_snapshot_membership s
  JOIN codeindex_membership_edges e ON e.org_id=s.org_id AND e.group_key=s.group_key;
