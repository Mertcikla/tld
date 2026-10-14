-- Retryable and additive; existing facts and snapshots remain intact.
ALTER TABLE codeindex_snapshots ADD COLUMN IF NOT EXISTS symbol_versions_json TEXT NOT NULL DEFAULT '{}';
