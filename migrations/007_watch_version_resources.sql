PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS watch_version_resources (
  version_id INTEGER NOT NULL,
  owner_type TEXT NOT NULL,
  owner_key TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id INTEGER NULL,
  language TEXT NULL,
  resource_hash TEXT NOT NULL,
  summary TEXT NULL,
  PRIMARY KEY(version_id, owner_type, owner_key, resource_type),
  FOREIGN KEY (version_id) REFERENCES watch_versions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_watch_version_resources_version_id
  ON watch_version_resources(version_id);

