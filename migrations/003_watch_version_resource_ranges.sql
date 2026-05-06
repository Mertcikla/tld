ALTER TABLE watch_version_resources ADD COLUMN file_path TEXT NULL;
ALTER TABLE watch_version_resources ADD COLUMN start_line INTEGER NOT NULL DEFAULT 0;
ALTER TABLE watch_version_resources ADD COLUMN end_line INTEGER NOT NULL DEFAULT 0;
