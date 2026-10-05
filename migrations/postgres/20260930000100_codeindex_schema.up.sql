-- codeindex: immutable, snapshot-scoped code graph replacing the watch_* tables.
-- Complete schema for a fresh install. org_id is part of every primary key and
-- the nil UUID is the self-hosted single-tenant sentinel, so organisations can
-- hold identically-derived entity ids without collisions.

CREATE TABLE IF NOT EXISTS codeindex_repositories (
  id TEXT NOT NULL,
  root TEXT NOT NULL,
  remote_url TEXT NOT NULL DEFAULT '',
  remote_key TEXT NOT NULL DEFAULT '',
  managed BOOLEAN NOT NULL DEFAULT FALSE,
  latest_snapshot_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_repositories_remote_key
  ON codeindex_repositories(remote_key);
CREATE INDEX IF NOT EXISTS idx_codeindex_repositories_org_id
  ON codeindex_repositories(org_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_codeindex_repositories_org_remote_key_unique
  ON codeindex_repositories(org_id, remote_key) WHERE remote_key <> '';

CREATE TABLE IF NOT EXISTS codeindex_snapshots (
  id TEXT NOT NULL,
  repository_id TEXT NOT NULL,
  created_unix BIGINT NOT NULL DEFAULT 0,
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
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_snapshots_repository
  ON codeindex_snapshots(repository_id, created_unix);
CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_revision
  ON codeindex_snapshots(repository_id, git_revision, provenance, config_hash);
CREATE INDEX IF NOT EXISTS idx_codeindex_snapshots_org_id
  ON codeindex_snapshots(org_id);

CREATE TABLE IF NOT EXISTS codeindex_sources (
  snapshot_id TEXT NOT NULL,
  path TEXT NOT NULL,
  hash TEXT NOT NULL,
  size BIGINT NOT NULL DEFAULT 0,
  content BYTEA,
  language TEXT NOT NULL DEFAULT '',
  input_blob TEXT NOT NULL DEFAULT '',
  dirty BOOLEAN NOT NULL DEFAULT FALSE,
  syntax_cache TEXT NOT NULL DEFAULT '',
  file_cache TEXT NOT NULL DEFAULT '',
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, snapshot_id, path)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_sources_hash
  ON codeindex_sources(hash);
CREATE INDEX IF NOT EXISTS idx_codeindex_sources_org_id
  ON codeindex_sources(org_id);

CREATE TABLE IF NOT EXISTS codeindex_facts (
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
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_facts_snapshot ON codeindex_facts(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_facts_logical ON codeindex_facts(snapshot_id, logical_key);
CREATE INDEX IF NOT EXISTS idx_codeindex_facts_symbol ON codeindex_facts(snapshot_id, symbol_key);
CREATE INDEX IF NOT EXISTS idx_codeindex_facts_path ON codeindex_facts(snapshot_id, path);
CREATE INDEX IF NOT EXISTS idx_codeindex_facts_org_id ON codeindex_facts(org_id);

CREATE TABLE IF NOT EXISTS codeindex_chunks (
  id TEXT NOT NULL,
  fact_id TEXT NOT NULL DEFAULT '',
  snapshot_id TEXT NOT NULL,
  anchor_json TEXT NOT NULL DEFAULT 'null',
  text TEXT NOT NULL DEFAULT '',
  context TEXT NOT NULL DEFAULT '',
  idx INTEGER NOT NULL DEFAULT 0,
  total INTEGER NOT NULL DEFAULT 0,
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_chunks_snapshot ON codeindex_chunks(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_chunks_fact ON codeindex_chunks(fact_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_chunks_org_id ON codeindex_chunks(org_id);

CREATE TABLE IF NOT EXISTS codeindex_edges (
  id TEXT NOT NULL,
  repository_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  kind INTEGER NOT NULL DEFAULT 0,
  from_fact_id TEXT NOT NULL DEFAULT '',
  to_fact_id TEXT NOT NULL DEFAULT '',
  target_symbol_key TEXT NOT NULL DEFAULT '',
  logical_key TEXT NOT NULL DEFAULT '',
  weight DOUBLE PRECISION NOT NULL DEFAULT 0,
  anchor_json TEXT NOT NULL DEFAULT 'null',
  evidence_json TEXT NOT NULL DEFAULT '[]',
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_edges_snapshot ON codeindex_edges(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_edges_logical ON codeindex_edges(snapshot_id, logical_key);
CREATE INDEX IF NOT EXISTS idx_codeindex_edges_from ON codeindex_edges(from_fact_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_edges_to ON codeindex_edges(to_fact_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_edges_org_id ON codeindex_edges(org_id);

CREATE TABLE IF NOT EXISTS codeindex_analysis_runs (
  id TEXT NOT NULL,
  repository_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  algorithm TEXT NOT NULL DEFAULT '',
  params_json TEXT NOT NULL DEFAULT '{}',
  created_unix BIGINT NOT NULL DEFAULT 0,
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_analysis_runs_org_id ON codeindex_analysis_runs(org_id);

CREATE TABLE IF NOT EXISTS codeindex_groups (
  id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  kind INTEGER NOT NULL DEFAULT 0,
  size INTEGER NOT NULL DEFAULT 0,
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_groups_org_id ON codeindex_groups(org_id);

CREATE TABLE IF NOT EXISTS codeindex_group_members (
  group_id TEXT NOT NULL,
  fact_id TEXT NOT NULL,
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, group_id, fact_id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_group_members_org_id ON codeindex_group_members(org_id);

-- Canonical codeindex logical keys mapped to materialized workspace resources.
-- logical_key is the fact or edge logical key; resource_type is 'element' or
-- 'connector'; resource_id points at the corresponding workspace row.
CREATE TABLE IF NOT EXISTS codeindex_elements (
  logical_key TEXT NOT NULL,
  resource_type TEXT NOT NULL DEFAULT 'element',
  resource_id BIGINT NOT NULL DEFAULT 0,
  repository_id TEXT NOT NULL DEFAULT '',
  snapshot_id TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, logical_key)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_elements_snapshot ON codeindex_elements(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_elements_resource ON codeindex_elements(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_elements_org_id ON codeindex_elements(org_id);

CREATE TABLE IF NOT EXISTS codeindex_completed_maps (
  run_id TEXT NOT NULL,
  repository_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  config_hash TEXT NOT NULL,
  completed_unix BIGINT NOT NULL,
  result_json TEXT NOT NULL,
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, run_id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_completed_maps_repository
  ON codeindex_completed_maps(repository_id, completed_unix);
CREATE INDEX IF NOT EXISTS idx_codeindex_completed_maps_org_id ON codeindex_completed_maps(org_id);

CREATE TABLE IF NOT EXISTS codeindex_impacts (
  repository_id TEXT NOT NULL,
  comparison_key TEXT NOT NULL,
  result_json TEXT NOT NULL,
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, repository_id, comparison_key)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_impacts_org_id ON codeindex_impacts(org_id);

CREATE TABLE IF NOT EXISTS codeindex_leases (
  repository_id TEXT NOT NULL,
  owner TEXT NOT NULL,
  expires_unix BIGINT NOT NULL,
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, repository_id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_leases_org_id ON codeindex_leases(org_id);

-- Watch control lets the CLI and the server observe and cooperatively stop a
-- repository's watcher while an incremental publish writes only changed rows.
CREATE TABLE IF NOT EXISTS codeindex_watch_state (
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
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, repository_id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_watch_state_org_id ON codeindex_watch_state(org_id);

CREATE TABLE IF NOT EXISTS codeindex_project_artifacts (
  snapshot_id TEXT NOT NULL,
  project_key TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  data BYTEA NOT NULL,
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, snapshot_id, project_key)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_project_artifacts_org_id ON codeindex_project_artifacts(org_id);

-- Snapshot membership decouples immutable entities (facts, chunks, edges) from
-- the snapshots that contain them. Reused entities keep a stable id across
-- snapshots, so an incremental publish writes only changed rows and records
-- membership for the rest.
CREATE TABLE IF NOT EXISTS codeindex_snapshot_facts (
  snapshot_id TEXT NOT NULL,
  fact_id TEXT NOT NULL,
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, snapshot_id, fact_id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_facts_fact ON codeindex_snapshot_facts(fact_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_facts_org_id ON codeindex_snapshot_facts(org_id);

CREATE TABLE IF NOT EXISTS codeindex_snapshot_chunks (
  snapshot_id TEXT NOT NULL,
  chunk_id TEXT NOT NULL,
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, snapshot_id, chunk_id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_chunks_chunk ON codeindex_snapshot_chunks(chunk_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_chunks_org_id ON codeindex_snapshot_chunks(org_id);

CREATE TABLE IF NOT EXISTS codeindex_snapshot_edges (
  snapshot_id TEXT NOT NULL,
  edge_id TEXT NOT NULL,
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, snapshot_id, edge_id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_edges_edge ON codeindex_snapshot_edges(edge_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_edges_org_id ON codeindex_snapshot_edges(org_id);

CREATE TABLE IF NOT EXISTS codeindex_repository_settings (
  repository_id TEXT NOT NULL,
  map_overrides TEXT NOT NULL DEFAULT '{}',
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, repository_id),
  FOREIGN KEY (org_id, repository_id) REFERENCES codeindex_repositories(org_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_codeindex_repository_settings_org_id ON codeindex_repository_settings(org_id);

CREATE TABLE IF NOT EXISTS codeindex_active_maps (
  repository_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, repository_id),
  FOREIGN KEY (org_id, run_id) REFERENCES codeindex_completed_maps(org_id, run_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_codeindex_active_maps_org_id ON codeindex_active_maps(org_id);

-- Remove the legacy watch pipeline tables, replaced by the codeindex schema.
DROP TABLE IF EXISTS watch_version_resources;
DROP TABLE IF EXISTS watch_representation_diffs;
DROP TABLE IF EXISTS watch_versions;
DROP TABLE IF EXISTS watch_context_expansions;
DROP TABLE IF EXISTS watch_context_policies;
DROP TABLE IF EXISTS watch_apply_locks;
DROP TABLE IF EXISTS watch_locks;
DROP TABLE IF EXISTS watch_representation_runs;
DROP TABLE IF EXISTS watch_architecture_links;
DROP TABLE IF EXISTS watch_materialization;
DROP TABLE IF EXISTS watch_cluster_members;
DROP TABLE IF EXISTS watch_clusters;
DROP TABLE IF EXISTS watch_filter_decisions;
DROP TABLE IF EXISTS watch_filter_runs;
DROP TABLE IF EXISTS watch_embeddings;
DROP TABLE IF EXISTS watch_embedding_models;
DROP TABLE IF EXISTS watch_scan_runs;
DROP TABLE IF EXISTS watch_symbol_identities;
DROP TABLE IF EXISTS watch_facts;
DROP TABLE IF EXISTS watch_references;
DROP TABLE IF EXISTS watch_symbols;
DROP TABLE IF EXISTS watch_files;
DROP TABLE IF EXISTS watch_repositories;

-- Link workspace elements to indexed codeindex repositories.
ALTER TABLE elements ADD COLUMN repository_id TEXT NULL;
CREATE INDEX IF NOT EXISTS idx_elements_repository ON elements(repository_id);