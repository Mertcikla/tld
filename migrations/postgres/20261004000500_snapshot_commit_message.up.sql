-- Snapshot commit message: records the Git subject at the captured revision so
-- the UI and CLI can label saved snapshots without a separate history lookup.
ALTER TABLE codeindex_snapshots ADD COLUMN commit_message TEXT NOT NULL DEFAULT '';
