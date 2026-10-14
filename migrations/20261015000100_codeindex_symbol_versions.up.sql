-- Preserve the literal symbols on existing fact rows; new snapshots retain
-- their package versions separately so fingerprints can ignore revisions.
ALTER TABLE codeindex_snapshots ADD COLUMN symbol_versions_json TEXT NOT NULL DEFAULT '{}';
