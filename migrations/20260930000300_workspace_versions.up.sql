-- Workspace versioning and shared indexes previously created by the watch
-- migration; preserved after that migration was removed.

CREATE TABLE IF NOT EXISTS workspace_versions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id TEXT NULL,
  version_id TEXT NOT NULL UNIQUE,
  source TEXT NOT NULL,
  parent_version_id INTEGER NULL,
  view_count INTEGER NOT NULL DEFAULT 0,
  element_count INTEGER NOT NULL DEFAULT 0,
  connector_count INTEGER NOT NULL DEFAULT 0,
  description TEXT NULL,
  workspace_hash TEXT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY (parent_version_id) REFERENCES workspace_versions(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS workspace_version_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  cli_versioning_enabled INTEGER NOT NULL DEFAULT 1
);

INSERT INTO workspace_version_settings(id, cli_versioning_enabled)
VALUES (1, 1)
ON CONFLICT(id) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_views_owner_element_id
  ON views(owner_element_id);

CREATE INDEX IF NOT EXISTS idx_placements_element_id_view_id
  ON placements(element_id, view_id);

CREATE INDEX IF NOT EXISTS idx_placements_view_id_id
  ON placements(view_id, id);

CREATE INDEX IF NOT EXISTS idx_connectors_view_id_id
  ON connectors(view_id, id);

CREATE INDEX IF NOT EXISTS idx_elements_updated_at_id
  ON elements(updated_at DESC, id DESC);
