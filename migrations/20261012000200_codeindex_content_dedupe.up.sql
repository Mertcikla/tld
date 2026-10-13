-- Content-addressed storage for captured sources and project artifacts.
-- Unchanged file text, syntax caches, and SCIP indexes are shared across
-- snapshots instead of being rewritten per revision; snapshot tables keep only
-- per-revision metadata and reference the shared rows by hash or fingerprint.
CREATE TABLE IF NOT EXISTS codeindex_blobs (
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  hash TEXT NOT NULL,
  language TEXT NOT NULL DEFAULT '',
  content BLOB,
  syntax_cache TEXT NOT NULL DEFAULT '',
  file_cache TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, hash, language)
);

INSERT INTO codeindex_blobs (org_id, hash, language, content, syntax_cache, file_cache)
  SELECT org_id, hash, language, content, syntax_cache, file_cache
  FROM codeindex_sources WHERE hash <> ''
  ON CONFLICT (org_id, hash, language) DO NOTHING;

ALTER TABLE codeindex_sources DROP COLUMN content;
ALTER TABLE codeindex_sources DROP COLUMN syntax_cache;
ALTER TABLE codeindex_sources DROP COLUMN file_cache;

CREATE TABLE IF NOT EXISTS codeindex_artifacts (
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  project_key TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  data BLOB NOT NULL,
  PRIMARY KEY (org_id, project_key, fingerprint)
);

INSERT INTO codeindex_artifacts (org_id, project_key, fingerprint, data)
  SELECT org_id, project_key, fingerprint, data
  FROM codeindex_project_artifacts
  WHERE project_key <> ''
  ON CONFLICT (org_id, project_key, fingerprint) DO NOTHING;

ALTER TABLE codeindex_project_artifacts DROP COLUMN data;

CREATE INDEX IF NOT EXISTS idx_codeindex_project_artifacts_key
  ON codeindex_project_artifacts(org_id, project_key, fingerprint);
