PRAGMA foreign_keys = ON;

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
