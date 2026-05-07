PRAGMA foreign_keys = ON;

ALTER TABLE views ADD COLUMN density_level INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS view_visibility_overrides (
  view_id INTEGER NOT NULL,
  resource_type TEXT NOT NULL CHECK(resource_type IN ('element', 'connector')),
  resource_id INTEGER NOT NULL,
  level_delta INTEGER NOT NULL DEFAULT 0 CHECK(level_delta BETWEEN -4 AND 4),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(view_id, resource_type, resource_id),
  FOREIGN KEY(view_id) REFERENCES views(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_view_visibility_overrides_resource
  ON view_visibility_overrides(resource_type, resource_id);
