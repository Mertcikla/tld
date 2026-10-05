PRAGMA foreign_keys = ON;

-- Organisation scoping for the workspace tables that lacked it.
--
-- views.id, elements.id and connectors.id are global surrogate keys, so org_id
-- is a filter column rather than part of the primary key. Existing rows
-- inherit the organisation of the view they belong to so tenant filters match
-- immediately after the upgrade.

ALTER TABLE placements ADD COLUMN org_id TEXT NULL;
ALTER TABLE view_layers ADD COLUMN org_id TEXT NULL;
ALTER TABLE view_visibility_overrides ADD COLUMN org_id TEXT NULL;

UPDATE placements SET org_id = (SELECT v.org_id FROM views v WHERE v.id = placements.view_id);
UPDATE view_layers SET org_id = (SELECT v.org_id FROM views v WHERE v.id = view_layers.view_id);
UPDATE view_visibility_overrides SET org_id = (SELECT v.org_id FROM views v WHERE v.id = view_visibility_overrides.view_id);

CREATE INDEX IF NOT EXISTS idx_placements_org_id ON placements(org_id);
CREATE INDEX IF NOT EXISTS idx_view_layers_org_id ON view_layers(org_id);
CREATE INDEX IF NOT EXISTS idx_view_visibility_overrides_org_id ON view_visibility_overrides(org_id);

-- Tag names were globally unique because name was the sole primary key, which
-- made two organisations unable to reuse the same tag name. Rebuild the table
-- so the key is (org_id, name). org_id becomes NOT NULL with the nil-uuid
-- sentinel because SQLite treats NULLs as distinct in both PRIMARY KEY and
-- UNIQUE constraints, which would leave the uniqueness unenforced for the
-- self-hosted single-tenant mode that stores a nil organisation.

UPDATE tags SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL OR org_id = '';

CREATE TABLE tags_org_scoped (
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  name TEXT NOT NULL,
  color TEXT NOT NULL,
  description TEXT NULL,
  PRIMARY KEY (org_id, name)
);

INSERT OR IGNORE INTO tags_org_scoped (org_id, name, color, description)
  SELECT org_id, name, color, description FROM tags ORDER BY org_id, name;

DROP TABLE tags;
ALTER TABLE tags_org_scoped RENAME TO tags;

-- Workspace versioning is superseded by the codeindex snapshot history.
DROP TABLE IF EXISTS workspace_version_settings;
DROP TABLE IF EXISTS workspace_versions;

CREATE INDEX IF NOT EXISTS idx_views_owner_element_id ON views(owner_element_id);
CREATE INDEX IF NOT EXISTS idx_placements_element_id_view_id ON placements(element_id, view_id);
CREATE INDEX IF NOT EXISTS idx_placements_view_id_id ON placements(view_id, id);
CREATE INDEX IF NOT EXISTS idx_connectors_view_id_id ON connectors(view_id, id);
CREATE INDEX IF NOT EXISTS idx_connectors_source_element_id ON connectors(source_element_id, id);
CREATE INDEX IF NOT EXISTS idx_connectors_target_element_id ON connectors(target_element_id, id);
CREATE INDEX IF NOT EXISTS idx_view_layers_view_id ON view_layers(view_id, id);
CREATE INDEX IF NOT EXISTS idx_elements_updated_at_id ON elements(updated_at DESC, id DESC);