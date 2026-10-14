-- Index format 3: compact edge identities and relationship-only rows.
-- Derived indexes are rebuilt; repository configuration and diagram ownership
-- keep their existing IDs and namespaces.
DELETE FROM codeindex_active_maps;
DELETE FROM codeindex_group_members;
DELETE FROM codeindex_groups;
DELETE FROM codeindex_analysis_runs;
DELETE FROM codeindex_completed_maps;
DELETE FROM codeindex_snapshot_facts;
DELETE FROM codeindex_snapshot_edges;
DELETE FROM codeindex_project_artifacts;
DELETE FROM codeindex_sources;
DELETE FROM codeindex_facts;
DELETE FROM codeindex_snapshots;
DELETE FROM codeindex_impacts;
DELETE FROM codeindex_blobs;
DELETE FROM codeindex_artifacts;
DELETE FROM codeindex_watch_state;
DELETE FROM codeindex_leases;
UPDATE codeindex_repositories SET latest_snapshot_id = 0;
UPDATE codeindex_elements SET snapshot_id = 0;

DROP TABLE codeindex_edges;
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
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  PRIMARY KEY (org_id, id)
);
CREATE INDEX idx_codeindex_edges_snapshot ON codeindex_edges(snapshot_id);
CREATE INDEX idx_codeindex_edges_logical ON codeindex_edges(org_id, logical_key, snapshot_id);
CREATE INDEX idx_codeindex_edges_from ON codeindex_edges(from_fact_id);
CREATE INDEX idx_codeindex_edges_to ON codeindex_edges(to_fact_id);
CREATE INDEX idx_codeindex_edges_org_id ON codeindex_edges(org_id);
