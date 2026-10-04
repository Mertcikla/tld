ALTER TABLE codeindex_snapshots ADD COLUMN provenance TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_snapshots ADD COLUMN content_fingerprint TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_snapshots ADD COLUMN capture_order BIGINT NOT NULL DEFAULT 0;
CREATE INDEX idx_codeindex_snapshot_revision ON codeindex_snapshots(repository_id, git_revision, provenance, config_hash);
CREATE TABLE codeindex_completed_maps (
  run_id TEXT PRIMARY KEY,
  repository_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  include_imports BOOLEAN NOT NULL DEFAULT FALSE,
  config_hash TEXT NOT NULL,
  completed_unix BIGINT NOT NULL,
  result_json TEXT NOT NULL
);
CREATE INDEX idx_codeindex_completed_maps_repository ON codeindex_completed_maps(repository_id, completed_unix);
