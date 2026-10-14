-- Additive and retryable; no existing snapshots or payloads are rewritten.
ALTER TABLE codeindex_facts ADD COLUMN IF NOT EXISTS content_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_edges ADD COLUMN IF NOT EXISTS content_hash TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_codeindex_facts_content ON codeindex_facts(org_id, repository_id, content_hash);
CREATE INDEX IF NOT EXISTS idx_codeindex_edges_content ON codeindex_edges(org_id, repository_id, content_hash);
