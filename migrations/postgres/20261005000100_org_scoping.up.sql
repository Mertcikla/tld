-- Organisation scoping for the workspace tables that lacked it.
--
-- views.id, elements.id and connectors.id are global surrogate keys, so org_id
-- is a filter column rather than part of the primary key. Existing rows
-- inherit the organisation of the view they belong to so tenant filters match
-- immediately after the upgrade.

ALTER TABLE placements ADD COLUMN IF NOT EXISTS org_id UUID NULL;
ALTER TABLE view_layers ADD COLUMN IF NOT EXISTS org_id UUID NULL;
ALTER TABLE view_visibility_overrides ADD COLUMN IF NOT EXISTS org_id UUID NULL;

UPDATE placements SET org_id = v.org_id FROM views v WHERE v.id = placements.view_id;
UPDATE view_layers SET org_id = v.org_id FROM views v WHERE v.id = view_layers.view_id;
UPDATE view_visibility_overrides SET org_id = v.org_id FROM views v WHERE v.id = view_visibility_overrides.view_id;

CREATE INDEX IF NOT EXISTS idx_placements_org_id ON placements(org_id);
CREATE INDEX IF NOT EXISTS idx_view_layers_org_id ON view_layers(org_id);
CREATE INDEX IF NOT EXISTS idx_view_visibility_overrides_org_id ON view_visibility_overrides(org_id);

-- tags and view_markdown_documents stored org_id as TEXT while elements, views,
-- connectors and workspace_versions used UUID. Unify on UUID so tenant filters
-- and comparisons work across tables.
UPDATE tags SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL OR org_id = '';
ALTER TABLE tags ALTER COLUMN org_id TYPE UUID USING NULLIF(org_id, '')::uuid;
ALTER TABLE view_markdown_documents
  ALTER COLUMN org_id TYPE UUID USING NULLIF(org_id, '')::uuid;

-- Tag names were globally unique because name was the sole primary key, which
-- made two organisations unable to reuse the same tag name. Re-key on
-- (org_id, name). org_id becomes NOT NULL with the nil-uuid sentinel because
-- NULLs are distinct in Postgres UNIQUE constraints, which would leave the
-- uniqueness unenforced for the self-hosted single-tenant mode.
ALTER TABLE tags ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE tags ALTER COLUMN org_id SET NOT NULL;
ALTER TABLE tags DROP CONSTRAINT IF EXISTS tags_pkey;
ALTER TABLE tags ADD PRIMARY KEY (org_id, name);

-- SQLite declares these as NOT NULL without a default; match it so an insert
-- that omits a position fails on both dialects instead of silently landing at
-- the origin.
ALTER TABLE placements ALTER COLUMN position_x DROP DEFAULT;
ALTER TABLE placements ALTER COLUMN position_y DROP DEFAULT;

-- Workspace versioning is superseded by the codeindex snapshot history.
DROP TABLE IF EXISTS workspace_version_settings;
DROP TABLE IF EXISTS workspace_versions;

-- Lookup indexes that were only ever created for SQLite. Both dialects now
-- converge on the same access paths.
CREATE INDEX IF NOT EXISTS idx_views_owner_element_id ON views(owner_element_id);
CREATE INDEX IF NOT EXISTS idx_placements_element_id_view_id ON placements(element_id, view_id);
CREATE INDEX IF NOT EXISTS idx_placements_view_id_id ON placements(view_id, id);
CREATE INDEX IF NOT EXISTS idx_connectors_view_id_id ON connectors(view_id, id);
CREATE INDEX IF NOT EXISTS idx_connectors_source_element_id ON connectors(source_element_id, id);
CREATE INDEX IF NOT EXISTS idx_connectors_target_element_id ON connectors(target_element_id, id);
CREATE INDEX IF NOT EXISTS idx_view_layers_view_id ON view_layers(view_id, id);
CREATE INDEX IF NOT EXISTS idx_elements_updated_at_id ON elements(updated_at DESC, id DESC);