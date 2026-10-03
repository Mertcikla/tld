-- Incremental watch support: shared watcher control, per-source extraction
-- caches, and snapshot membership so the CLI and server can observe and
-- cooperatively stop a watcher while an incremental publish writes only the
-- entities that changed.

-- Watch control: extend the shared watch state row so the CLI and the server
-- can both observe and cooperatively stop a repository's watcher.
ALTER TABLE codeindex_watch_state ADD COLUMN owner_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_watch_state ADD COLUMN owner_pid INTEGER NOT NULL DEFAULT 0;
ALTER TABLE codeindex_watch_state ADD COLUMN owner_id TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_watch_state ADD COLUMN state TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_watch_state ADD COLUMN stage TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_watch_state ADD COLUMN repo_root TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_watch_state ADD COLUMN snapshot_id TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_watch_state ADD COLUMN content_fingerprint TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_watch_state ADD COLUMN changed_files INTEGER NOT NULL DEFAULT 0;
ALTER TABLE codeindex_watch_state ADD COLUMN pending_files INTEGER NOT NULL DEFAULT 0;
ALTER TABLE codeindex_watch_state ADD COLUMN started_unix BIGINT NOT NULL DEFAULT 0;
ALTER TABLE codeindex_watch_state ADD COLUMN last_scan_unix BIGINT NOT NULL DEFAULT 0;
ALTER TABLE codeindex_watch_state ADD COLUMN last_scan_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE codeindex_watch_state ADD COLUMN stop_requested BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE codeindex_watch_state ADD COLUMN poll_interval_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE codeindex_watch_state ADD COLUMN debounce_ms INTEGER NOT NULL DEFAULT 0;

-- Whole-file fact/chunk cache keyed by content hash so unchanged files are not
-- re-chunked on every incremental build.
ALTER TABLE codeindex_sources ADD COLUMN file_cache TEXT NOT NULL DEFAULT '';

-- Snapshot membership decouples immutable entities (facts, chunks, edges) from
-- the snapshots that contain them.
CREATE TABLE IF NOT EXISTS codeindex_snapshot_facts (
  snapshot_id TEXT NOT NULL,
  fact_id TEXT NOT NULL,
  PRIMARY KEY (snapshot_id, fact_id)
);
CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_facts_fact ON codeindex_snapshot_facts(fact_id);

CREATE TABLE IF NOT EXISTS codeindex_snapshot_chunks (
  snapshot_id TEXT NOT NULL,
  chunk_id TEXT NOT NULL,
  PRIMARY KEY (snapshot_id, chunk_id)
);
CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_chunks_chunk ON codeindex_snapshot_chunks(chunk_id);

CREATE TABLE IF NOT EXISTS codeindex_snapshot_edges (
  snapshot_id TEXT NOT NULL,
  edge_id TEXT NOT NULL,
  PRIMARY KEY (snapshot_id, edge_id)
);
CREATE INDEX IF NOT EXISTS idx_codeindex_snapshot_edges_edge ON codeindex_snapshot_edges(edge_id);

-- Backfill membership from the legacy snapshot-scoped rows.
INSERT INTO codeindex_snapshot_facts (snapshot_id, fact_id)
  SELECT snapshot_id, id FROM codeindex_facts ON CONFLICT DO NOTHING;
INSERT INTO codeindex_snapshot_chunks (snapshot_id, chunk_id)
  SELECT snapshot_id, id FROM codeindex_chunks ON CONFLICT DO NOTHING;
INSERT INTO codeindex_snapshot_edges (snapshot_id, edge_id)
  SELECT snapshot_id, id FROM codeindex_edges ON CONFLICT DO NOTHING;
