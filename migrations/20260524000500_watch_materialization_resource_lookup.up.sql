CREATE INDEX IF NOT EXISTS idx_watch_materialization_resource_lookup
  ON watch_materialization(repository_id, resource_type, resource_id);
