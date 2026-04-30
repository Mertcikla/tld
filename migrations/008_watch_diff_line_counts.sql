ALTER TABLE watch_representation_diffs
  ADD COLUMN added_lines INTEGER NOT NULL DEFAULT 0;

ALTER TABLE watch_representation_diffs
  ADD COLUMN removed_lines INTEGER NOT NULL DEFAULT 0;

ALTER TABLE watch_version_resources
  ADD COLUMN line_count INTEGER NOT NULL DEFAULT 0;
