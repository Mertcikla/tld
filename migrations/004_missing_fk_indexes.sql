CREATE INDEX IF NOT EXISTS idx_view_layers_view_id
  ON view_layers(view_id, id);

CREATE INDEX IF NOT EXISTS idx_connectors_source_element_id
  ON connectors(source_element_id, id);

CREATE INDEX IF NOT EXISTS idx_connectors_target_element_id
  ON connectors(target_element_id, id);
