-- codeindex: immutable, snapshot-scoped code graph replacing the watch_* tables.
-- Consolidated migration covering the codeindex baseline, snapshot provenance,
-- completed maps, watch state, snapshot membership, and repository settings.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS codeindex_repositories (
  id TEXT PRIMARY KEY,
  root TEXT NOT NULL,
  remote_url TEXT NOT NULL DEFAULT '',
  managed BOOLEAN NOT NULL DEFAULT FALSE,
  latest_snapshot_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS codeindex_snapshots (
  id TEXT PRIMARY KEY,
  repository_id TEXT NOT NULL,
  created_unix BIGINT NOT NULL DEFAULT 0,
  git_revision TEXT NOT NULL DEFAULT '',
  git_branch TEXT NOT NULL DEFAULT '',
  ingestion_status TEXT NOT NULL DEFAULT '',
  config_hash TEXT NOT NULL DEFAULT '',
  projects_json TEXT NOT NULL DEFAULT '[]',
  warnings_json TEXT NOT NULL DEFAULT '[]',
  tool_versions_json TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_codeindex_snapshots_repository
  ON codeindex_snapshots(repository_id, created_unix);

CREATE TABLE IF NOT EXISTS codeindex_sources (
  snapshot_id TEXT NOT NULL,
  path TEXT NOT NULL,
  hash TEXT NOT NULL,
  size BIGINT NOT NULL DEFAULT 0,
  content BYTEA,
  PRIMARY KEY (snapshot_id, path)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_sources_hash
  ON codeindex_sources(hash);

CREATE TABLE IF NOT EXISTS codeindex_facts (
  id TEXT PRIMARY KEY,
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
  imports_json TEXT NOT NULL DEFAULT '[]'
);

CREATE INDEX IF NOT EXISTS idx_codeindex_facts_snapshot ON codeindex_facts(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_facts_logical ON codeindex_facts(snapshot_id, logical_key);
CREATE INDEX IF NOT EXISTS idx_codeindex_facts_symbol ON codeindex_facts(snapshot_id, symbol_key);
CREATE INDEX IF NOT EXISTS idx_codeindex_facts_path ON codeindex_facts(snapshot_id, path);

CREATE TABLE IF NOT EXISTS codeindex_chunks (
  id TEXT PRIMARY KEY,
  fact_id TEXT NOT NULL DEFAULT '',
  snapshot_id TEXT NOT NULL,
  anchor_json TEXT NOT NULL DEFAULT 'null',
  text TEXT NOT NULL DEFAULT '',
  context TEXT NOT NULL DEFAULT '',
  idx INTEGER NOT NULL DEFAULT 0,
  total INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_codeindex_chunks_snapshot ON codeindex_chunks(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_chunks_fact ON codeindex_chunks(fact_id);

CREATE TABLE IF NOT EXISTS codeindex_edges (
  id TEXT PRIMARY KEY,
  repository_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  kind INTEGER NOT NULL DEFAULT 0,
  from_fact_id TEXT NOT NULL DEFAULT '',
  to_fact_id TEXT NOT NULL DEFAULT '',
  target_symbol_key TEXT NOT NULL DEFAULT '',
  logical_key TEXT NOT NULL DEFAULT '',
  weight DOUBLE PRECISION NOT NULL DEFAULT 0,
  anchor_json TEXT NOT NULL DEFAULT 'null',
  evidence_json TEXT NOT NULL DEFAULT '[]'
);

CREATE INDEX IF NOT EXISTS idx_codeindex_edges_snapshot ON codeindex_edges(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_edges_logical ON codeindex_edges(snapshot_id, logical_key);
CREATE INDEX IF NOT EXISTS idx_codeindex_edges_from ON codeindex_edges(from_fact_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_edges_to ON codeindex_edges(to_fact_id);

CREATE TABLE IF NOT EXISTS codeindex_analysis_runs (
  id TEXT PRIMARY KEY,
  repository_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  algorithm TEXT NOT NULL DEFAULT '',
  params_json TEXT NOT NULL DEFAULT '{}',
  created_unix BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS codeindex_groups (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  kind INTEGER NOT NULL DEFAULT 0,
  size INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS codeindex_group_members (
  group_id TEXT NOT NULL,
  fact_id TEXT NOT NULL,
  PRIMARY KEY (group_id, fact_id)
);

-- Canonical codeindex logical keys mapped to materialized workspace resources.
CREATE TABLE IF NOT EXISTS codeindex_elements (
  logical_key TEXT PRIMARY KEY,
  resource_type TEXT NOT NULL DEFAULT 'element',
  resource_id BIGINT NOT NULL DEFAULT 0,
  repository_id TEXT NOT NULL DEFAULT '',
  snapshot_id TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_codeindex_elements_snapshot ON codeindex_elements(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_elements_resource ON codeindex_elements(resource_type, resource_id);

-- Snapshot provenance, completed maps, and source extraction caches.

ALTER TABLE codeindex_snapshots ADD COLUMN provenance TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_snapshots ADD COLUMN content_fingerprint TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_snapshots ADD COLUMN capture_order BIGINT NOT NULL DEFAULT 0;
CREATE INDEX idx_codeindex_snapshot_revision ON codeindex_snapshots(repository_id, git_revision, provenance, config_hash);
CREATE TABLE codeindex_completed_maps (
  run_id TEXT PRIMARY KEY,
  repository_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  config_hash TEXT NOT NULL,
  completed_unix BIGINT NOT NULL,
  result_json TEXT NOT NULL
);
CREATE INDEX idx_codeindex_completed_maps_repository ON codeindex_completed_maps(repository_id, completed_unix);

ALTER TABLE codeindex_sources ADD COLUMN language TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_sources ADD COLUMN input_blob TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_sources ADD COLUMN dirty BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE codeindex_sources ADD COLUMN syntax_cache TEXT NOT NULL DEFAULT '';
CREATE TABLE codeindex_impacts (
 repository_id TEXT NOT NULL,
 comparison_key TEXT NOT NULL,
 result_json TEXT NOT NULL,
 PRIMARY KEY (repository_id, comparison_key)
);
CREATE TABLE codeindex_leases (
 repository_id TEXT PRIMARY KEY,
 owner TEXT NOT NULL,
 expires_unix BIGINT NOT NULL
);
CREATE TABLE codeindex_watch_state (
 repository_id TEXT PRIMARY KEY,
 heartbeat_unix BIGINT NOT NULL DEFAULT 0,
 error TEXT NOT NULL DEFAULT '',
 git_branch TEXT NOT NULL DEFAULT '',
 git_revision TEXT NOT NULL DEFAULT ''
);
CREATE TABLE codeindex_project_artifacts (
 snapshot_id TEXT NOT NULL,
 project_key TEXT NOT NULL,
 fingerprint TEXT NOT NULL,
 data BYTEA NOT NULL,
 PRIMARY KEY (snapshot_id, project_key)
);

-- Incremental watch support: shared watcher control, per-source extraction
-- caches, and snapshot membership so the CLI and server can observe and
-- cooperatively stop a watcher while an incremental publish writes only the
-- entities that changed.

-- Watch control: extend the shared watch state row so the CLI and the server
-- can both observe and cooperatively stop a repository's watcher.
ALTER TABLE codeindex_watch_state ADD COLUMN owner_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_watch_state ADD COLUMN owner_pid INTEGER NOT NULL DEFAULT 0;
ALTER TABLE codeindex_watch_state ADD COLUMN owner_id TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_watch_state ADD COLUMN state TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_watch_state ADD COLUMN stage TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_watch_state ADD COLUMN repo_root TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_watch_state ADD COLUMN snapshot_id TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_watch_state ADD COLUMN content_fingerprint TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_watch_state ADD COLUMN changed_files INTEGER NOT NULL DEFAULT 0;
ALTER TABLE codeindex_watch_state ADD COLUMN pending_files INTEGER NOT NULL DEFAULT 0;
ALTER TABLE codeindex_watch_state ADD COLUMN started_unix BIGINT NOT NULL DEFAULT 0;
ALTER TABLE codeindex_watch_state ADD COLUMN last_scan_unix BIGINT NOT NULL DEFAULT 0;
ALTER TABLE codeindex_watch_state ADD COLUMN last_scan_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE codeindex_watch_state ADD COLUMN stop_requested BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE codeindex_watch_state ADD COLUMN poll_interval_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE codeindex_watch_state ADD COLUMN debounce_ms INTEGER NOT NULL DEFAULT 0;

-- Whole-file fact/chunk cache keyed by content hash so unchanged files are not
-- re-chunked on every incremental build.
ALTER TABLE codeindex_sources ADD COLUMN file_cache TEXT NOT NULL DEFAULT '';

-- Snapshot membership decouples immutable entities (facts, chunks, edges) from
-- the snapshots that contain them. Reused entities keep a stable id across
-- snapshots, so an incremental publish writes only changed rows and records
-- membership for the rest.
CREATE TABLE IF NOT EXISTS codeindex_snapshot_facts (
  snapshot_id TEXT NOT NULL,
  fact_id TEXT NOT NULL,
  PRIMARY KEY (snapshot_id, fact_id)
);
CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_facts_fact ON codeindex_snapshot_facts(fact_id);

CREATE TABLE IF NOT EXISTS codeindex_snapshot_chunks (
  snapshot_id TEXT NOT NULL,
  chunk_id TEXT NOT NULL,
  PRIMARY KEY (snapshot_id, chunk_id)
);
CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_chunks_chunk ON codeindex_snapshot_chunks(chunk_id);

CREATE TABLE IF NOT EXISTS codeindex_snapshot_edges (
  snapshot_id TEXT NOT NULL,
  edge_id TEXT NOT NULL,
  PRIMARY KEY (snapshot_id, edge_id)
);
CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_edges_edge ON codeindex_snapshot_edges(edge_id);

-- Watch stop deadline: records when a stop was requested so controllers can
-- escalate if a watcher does not honor it.
ALTER TABLE codeindex_watch_state ADD COLUMN stop_requested_unix BIGINT NOT NULL DEFAULT 0;

-- Snapshot commit message: records the Git subject at the captured revision so
-- the UI and CLI can label saved snapshots without a separate history lookup.
ALTER TABLE codeindex_snapshots ADD COLUMN commit_message TEXT NOT NULL DEFAULT '';

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

CREATE TABLE codeindex_repository_settings (
    repository_id TEXT PRIMARY KEY REFERENCES codeindex_repositories(id) ON DELETE CASCADE,
    map_overrides TEXT NOT NULL DEFAULT '{}'
);

-- Link workspace elements to indexed codeindex repositories.
ALTER TABLE elements ADD COLUMN repository_id TEXT NULL;
CREATE INDEX IF NOT EXISTS idx_elements_repository ON elements(repository_id);
