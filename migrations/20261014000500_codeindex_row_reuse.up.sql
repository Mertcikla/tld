-- Preserve existing payloads and membership tables. Empty fingerprints on old
-- rows leave them readable; new publications share exactly matching new rows.
ALTER TABLE codeindex_facts ADD COLUMN content_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_edges ADD COLUMN content_hash TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_codeindex_facts_content ON codeindex_facts(org_id, repository_id, content_hash);
CREATE INDEX idx_codeindex_edges_content ON codeindex_edges(org_id, repository_id, content_hash);
