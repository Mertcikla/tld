-- Index format 2: discard derived graphs; retain repository configuration and
-- diagram ownership. Old mapping keys/tags keep their original namespace.
CREATE TEMP TABLE codeindex_repository_ids AS
  SELECT org_id, id AS old_id, ROW_NUMBER() OVER (ORDER BY org_id, id) AS new_id
  FROM codeindex_repositories;
CREATE TEMP TABLE codeindex_saved_repositories AS
  SELECT r.*, m.new_id FROM codeindex_repositories r
  JOIN codeindex_repository_ids m ON m.org_id=r.org_id AND m.old_id=r.id;
CREATE TEMP TABLE codeindex_saved_settings AS
  SELECT s.map_overrides, s.org_id, m.new_id FROM codeindex_repository_settings s
  JOIN codeindex_repository_ids m ON m.org_id=s.org_id AND m.old_id=s.repository_id;
CREATE TEMP TABLE codeindex_saved_mappings AS
  SELECT e.*, COALESCE(m.new_id, 0) AS new_id FROM codeindex_elements e
  LEFT JOIN codeindex_repository_ids m ON m.org_id=e.org_id AND m.old_id=e.repository_id;
UPDATE elements SET repository_id = (
  SELECT CAST(m.new_id AS TEXT) FROM codeindex_repository_ids m
  WHERE m.org_id=COALESCE(elements.org_id,'00000000-0000-0000-0000-000000000000') AND m.old_id=elements.repository_id)
WHERE EXISTS (SELECT 1 FROM codeindex_repository_ids m
  WHERE m.org_id=COALESCE(elements.org_id,'00000000-0000-0000-0000-000000000000') AND m.old_id=elements.repository_id);

DROP TABLE codeindex_active_maps;
DROP TABLE codeindex_group_members;
DROP TABLE codeindex_groups;
DROP TABLE codeindex_analysis_runs;
DROP TABLE codeindex_completed_maps;
DROP TABLE codeindex_snapshot_facts;
DROP TABLE codeindex_snapshot_edges;
DROP TABLE codeindex_project_artifacts;
DROP TABLE codeindex_sources;
DROP TABLE codeindex_edges;
DROP TABLE codeindex_facts;
DROP TABLE codeindex_snapshots;
DROP TABLE codeindex_impacts;
DROP TABLE codeindex_repository_settings;
DROP TABLE codeindex_elements;
DROP TABLE codeindex_repositories;

DELETE FROM codeindex_blobs;
DELETE FROM codeindex_artifacts;
DELETE FROM codeindex_watch_state;
DELETE FROM codeindex_leases;

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
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_repositories_remote_key
  ON codeindex_repositories(remote_key);
CREATE INDEX IF NOT EXISTS idx_codeindex_repositories_org_id
  ON codeindex_repositories(org_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_codeindex_repositories_org_remote_key_unique
  ON codeindex_repositories(org_id, remote_key) WHERE remote_key <> '';

CREATE TABLE codeindex_repository_settings (
  repository_id INTEGER NOT NULL,
  map_overrides TEXT NOT NULL DEFAULT '{}',
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, repository_id),
  FOREIGN KEY (org_id, repository_id) REFERENCES codeindex_repositories(org_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_codeindex_repository_settings_org_id ON codeindex_repository_settings(org_id);

CREATE TABLE codeindex_elements (
  logical_key TEXT NOT NULL,
  resource_type TEXT NOT NULL DEFAULT 'element',
  resource_id INTEGER NOT NULL DEFAULT 0,
  repository_id INTEGER NOT NULL DEFAULT 0,
  snapshot_id INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL DEFAULT '',
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, logical_key)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_elements_snapshot ON codeindex_elements(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_elements_resource ON codeindex_elements(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_elements_org_id ON codeindex_elements(org_id);

CREATE TABLE codeindex_impacts (
  repository_id INTEGER NOT NULL,
  comparison_key TEXT NOT NULL,
  result_json BLOB NOT NULL,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, repository_id, comparison_key)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_impacts_org_id ON codeindex_impacts(org_id);

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
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_snapshots_repository
  ON codeindex_snapshots(repository_id, created_unix);
CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_revision
  ON codeindex_snapshots(repository_id, git_revision, provenance, config_hash);
CREATE INDEX IF NOT EXISTS idx_codeindex_snapshots_org_id
  ON codeindex_snapshots(org_id);

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
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_facts_snapshot ON codeindex_facts(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_facts_logical ON codeindex_facts(snapshot_id, logical_key);
CREATE INDEX IF NOT EXISTS idx_codeindex_facts_symbol ON codeindex_facts(snapshot_id, symbol_key);
CREATE INDEX IF NOT EXISTS idx_codeindex_facts_path ON codeindex_facts(snapshot_id, path);
CREATE INDEX IF NOT EXISTS idx_codeindex_facts_org_id ON codeindex_facts(org_id);

CREATE TABLE codeindex_edges (
  id INTEGER NOT NULL,
  repository_id INTEGER NOT NULL,
  snapshot_id INTEGER NOT NULL,
  kind INTEGER NOT NULL DEFAULT 0,
  from_fact_id INTEGER NOT NULL DEFAULT 0,
  to_fact_id INTEGER NOT NULL DEFAULT 0,
  target_symbol_key TEXT NOT NULL DEFAULT '',
  logical_key TEXT NOT NULL DEFAULT '',
  weight REAL NOT NULL DEFAULT 0,
  anchor_json TEXT NOT NULL DEFAULT 'null',
  evidence_json TEXT NOT NULL DEFAULT '[]',
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_edges_snapshot ON codeindex_edges(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_edges_logical ON codeindex_edges(snapshot_id, logical_key);
CREATE INDEX IF NOT EXISTS idx_codeindex_edges_from ON codeindex_edges(from_fact_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_edges_to ON codeindex_edges(to_fact_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_edges_org_id ON codeindex_edges(org_id);

CREATE TABLE codeindex_sources (
  snapshot_id INTEGER NOT NULL,
  path TEXT NOT NULL,
  hash TEXT NOT NULL,
  size INTEGER NOT NULL DEFAULT 0,
  language TEXT NOT NULL DEFAULT '',
  input_blob TEXT NOT NULL DEFAULT '',
  dirty BOOLEAN NOT NULL DEFAULT FALSE,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, snapshot_id, path)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_sources_hash
  ON codeindex_sources(hash);
CREATE INDEX IF NOT EXISTS idx_codeindex_sources_org_id
  ON codeindex_sources(org_id);

CREATE TABLE codeindex_project_artifacts (
  snapshot_id INTEGER NOT NULL,
  project_key TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, snapshot_id, project_key)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_project_artifacts_org_id ON codeindex_project_artifacts(org_id);
CREATE INDEX idx_codeindex_project_artifacts_key ON codeindex_project_artifacts(org_id, project_key, fingerprint);

CREATE TABLE codeindex_snapshot_edges (
  snapshot_id INTEGER NOT NULL,
  edge_id INTEGER NOT NULL,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, snapshot_id, edge_id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_edges_edge ON codeindex_snapshot_edges(edge_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_edges_org_id ON codeindex_snapshot_edges(org_id);

CREATE TABLE codeindex_snapshot_facts (
  snapshot_id INTEGER NOT NULL,
  fact_id INTEGER NOT NULL,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, snapshot_id, fact_id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_facts_fact ON codeindex_snapshot_facts(fact_id);
CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_facts_org_id ON codeindex_snapshot_facts(org_id);

CREATE TABLE codeindex_completed_maps (
  run_id INTEGER NOT NULL,
  repository_id INTEGER NOT NULL,
  snapshot_id INTEGER NOT NULL,
  config_hash TEXT NOT NULL,
  completed_unix BIGINT NOT NULL,
  result_json TEXT NOT NULL,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, run_id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_completed_maps_repository
  ON codeindex_completed_maps(repository_id, completed_unix);
CREATE INDEX IF NOT EXISTS idx_codeindex_completed_maps_org_id ON codeindex_completed_maps(org_id);

CREATE TABLE codeindex_analysis_runs (
  id INTEGER NOT NULL,
  repository_id INTEGER NOT NULL,
  snapshot_id INTEGER NOT NULL,
  algorithm TEXT NOT NULL DEFAULT '',
  params_json TEXT NOT NULL DEFAULT '{}',
  created_unix INTEGER NOT NULL DEFAULT 0,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_analysis_runs_org_id ON codeindex_analysis_runs(org_id);

CREATE TABLE codeindex_groups (
  id INTEGER NOT NULL,
  run_id INTEGER NOT NULL,
  snapshot_id INTEGER NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  kind INTEGER NOT NULL DEFAULT 0,
  size INTEGER NOT NULL DEFAULT 0,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_groups_org_id ON codeindex_groups(org_id);

CREATE TABLE codeindex_group_members (
  group_id INTEGER NOT NULL,
  fact_id INTEGER NOT NULL,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, group_id, fact_id)
);

CREATE INDEX IF NOT EXISTS idx_codeindex_group_members_org_id ON codeindex_group_members(org_id);

CREATE TABLE codeindex_active_maps (
  repository_id INTEGER NOT NULL,
  run_id INTEGER NOT NULL,
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, repository_id),
  FOREIGN KEY (org_id, run_id) REFERENCES codeindex_completed_maps(org_id, run_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_codeindex_active_maps_org_id ON codeindex_active_maps(org_id);

INSERT INTO codeindex_repositories
  (id, mapping_namespace, root, remote_url, remote_key, managed, created_at, updated_at, org_id)
  SELECT new_id, id, root, remote_url, remote_key, managed, created_at, updated_at, org_id
  FROM codeindex_saved_repositories;
INSERT INTO codeindex_repository_settings (repository_id, map_overrides, org_id)
  SELECT new_id, map_overrides, org_id FROM codeindex_saved_settings;
INSERT INTO codeindex_elements
  (logical_key, resource_type, resource_id, repository_id, snapshot_id, updated_at, org_id)
  SELECT logical_key, resource_type, resource_id, new_id, 0, updated_at, org_id FROM codeindex_saved_mappings;
CREATE TABLE codeindex_id_sequence (
  singleton INTEGER PRIMARY KEY CHECK (singleton=1),
  value INTEGER NOT NULL
);
INSERT INTO codeindex_id_sequence SELECT 1, COALESCE(MAX(new_id),0) FROM codeindex_repository_ids;
DROP TABLE codeindex_saved_repositories;
DROP TABLE codeindex_saved_settings;
DROP TABLE codeindex_saved_mappings;
DROP TABLE codeindex_repository_ids;
