-- Watch stop deadline: records when a stop was requested so controllers can
-- escalate if a watcher does not honor it. Kept as its own migration because
-- databases that applied the earlier watch-control migration do not have it.
ALTER TABLE codeindex_watch_state ADD COLUMN stop_requested_unix BIGINT NOT NULL DEFAULT 0;
