ALTER TABLE codeindex_sources ADD COLUMN language TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_sources ADD COLUMN input_blob TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_sources ADD COLUMN dirty BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE codeindex_sources ADD COLUMN syntax_cache TEXT NOT NULL DEFAULT '';
CREATE TABLE codeindex_impacts (
 repository_id TEXT NOT NULL,
 comparison_key TEXT NOT NULL,
 result_json TEXT NOT NULL,
 PRIMARY KEY (repository_id, comparison_key)
);
CREATE TABLE codeindex_leases (
 repository_id TEXT PRIMARY KEY,
 owner TEXT NOT NULL,
 expires_unix BIGINT NOT NULL
);
CREATE TABLE codeindex_watch_state (
 repository_id TEXT PRIMARY KEY,
 heartbeat_unix BIGINT NOT NULL DEFAULT 0,
 error TEXT NOT NULL DEFAULT '',
 git_branch TEXT NOT NULL DEFAULT '',
 git_revision TEXT NOT NULL DEFAULT ''
);
CREATE TABLE codeindex_project_artifacts (
 snapshot_id TEXT NOT NULL,
 project_key TEXT NOT NULL,
 fingerprint TEXT NOT NULL,
 data BYTEA NOT NULL,
 PRIMARY KEY (snapshot_id, project_key)
);
