-- Existing snapshots retain direct memberships. New snapshots share immutable
-- per-file membership groups, with no parent-snapshot dependency.
ALTER TABLE codeindex_snapshot_facts RENAME TO codeindex_snapshot_facts_direct;
ALTER TABLE codeindex_snapshot_edges RENAME TO codeindex_snapshot_edges_direct;
CREATE TABLE codeindex_membership_groups (
 group_key TEXT NOT NULL, repository_id BIGINT NOT NULL, org_id UUID NOT NULL,
 PRIMARY KEY (org_id, group_key)
);
CREATE TABLE codeindex_membership_facts (
 group_key TEXT NOT NULL, fact_id BIGINT NOT NULL, org_id UUID NOT NULL,
 PRIMARY KEY (org_id, group_key, fact_id)
);
CREATE INDEX idx_codeindex_membership_fact ON codeindex_membership_facts(org_id, fact_id);
CREATE TABLE codeindex_membership_edges (
 group_key TEXT NOT NULL, edge_id BIGINT NOT NULL, org_id UUID NOT NULL,
 PRIMARY KEY (org_id, group_key, edge_id)
);
CREATE INDEX idx_codeindex_membership_edge ON codeindex_membership_edges(org_id, edge_id);
CREATE TABLE codeindex_snapshot_membership (
 snapshot_id BIGINT NOT NULL, group_key TEXT NOT NULL, org_id UUID NOT NULL,
 PRIMARY KEY (org_id, snapshot_id, group_key)
);
CREATE INDEX idx_codeindex_snapshot_membership_group ON codeindex_snapshot_membership(org_id, group_key);
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

-- Counts are immutable snapshot metadata. NULL keeps legacy snapshots readable.
ALTER TABLE codeindex_snapshots ADD COLUMN fact_count BIGINT;
ALTER TABLE codeindex_snapshots ADD COLUMN edge_count BIGINT;
ALTER TABLE codeindex_snapshots ADD COLUMN source_count BIGINT;
CREATE INDEX idx_codeindex_snapshot_facts_direct_snapshot ON codeindex_snapshot_facts_direct(snapshot_id);
CREATE INDEX idx_codeindex_snapshot_edges_direct_snapshot ON codeindex_snapshot_edges_direct(snapshot_id);
CREATE INDEX idx_codeindex_snapshot_membership_snapshot ON codeindex_snapshot_membership(snapshot_id);
