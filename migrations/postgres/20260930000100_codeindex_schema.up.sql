-- codeindex: immutable, snapshot-scoped code graph replacing the watch_* tables.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS codeindex_repositories (
  id TEXT PRIMARY KEY,
  root TEXT NOT NULL,
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
  embedding_status TEXT NOT NULL DEFAULT '',
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

CREATE TABLE IF NOT EXISTS codeindex_embeddings (
  id TEXT PRIMARY KEY,
  chunk_id TEXT NOT NULL DEFAULT '',
  fact_id TEXT NOT NULL DEFAULT '',
  snapshot_id TEXT NOT NULL,
  profile TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  dimensions INTEGER NOT NULL DEFAULT 0,
  input_hash TEXT NOT NULL DEFAULT '',
  vector BYTEA
);

CREATE INDEX IF NOT EXISTS idx_codeindex_embeddings_snapshot ON codeindex_embeddings(snapshot_id, profile);

CREATE TABLE IF NOT EXISTS codeindex_fact_embeddings (
  id TEXT PRIMARY KEY,
  fact_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  profile TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  dimensions INTEGER NOT NULL DEFAULT 0,
  input_hash TEXT NOT NULL DEFAULT '',
  vector BYTEA,
  embedding vector
);

CREATE INDEX IF NOT EXISTS idx_codeindex_fact_embeddings_snapshot
  ON codeindex_fact_embeddings(snapshot_id, profile);

CREATE TABLE IF NOT EXISTS codeindex_embedding_cache (
  key TEXT PRIMARY KEY,
  profile TEXT NOT NULL DEFAULT '',
  input_hash TEXT NOT NULL DEFAULT '',
  dimensions INTEGER NOT NULL DEFAULT 0,
  vector BYTEA
);

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
  profile TEXT NOT NULL DEFAULT '',
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
