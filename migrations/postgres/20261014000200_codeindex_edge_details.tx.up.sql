DO $$
BEGIN
  PERFORM pg_advisory_xact_lock(20261014000200);
  -- Make retry safe if migration-history recording failed after committing DDL.
  IF EXISTS (SELECT 1 FROM information_schema.columns
    WHERE table_schema=current_schema() AND table_name='codeindex_edges' AND column_name='logical_key' AND data_type='bytea') THEN
    RETURN;
  END IF;

  -- Index format 3: rebuild derived data, retaining configuration and ownership.
  TRUNCATE codeindex_active_maps, codeindex_group_members, codeindex_groups,
    codeindex_analysis_runs, codeindex_completed_maps, codeindex_snapshot_facts,
    codeindex_snapshot_edges, codeindex_project_artifacts, codeindex_sources,
    codeindex_edges, codeindex_facts, codeindex_snapshots, codeindex_impacts,
    codeindex_blobs, codeindex_artifacts, codeindex_watch_state, codeindex_leases;
  UPDATE codeindex_repositories SET latest_snapshot_id = 0;
  UPDATE codeindex_elements SET snapshot_id = 0;

  DROP INDEX idx_codeindex_edges_logical;
  ALTER TABLE codeindex_edges
    ALTER COLUMN logical_key DROP DEFAULT,
    ALTER COLUMN logical_key TYPE BYTEA USING decode(logical_key, 'hex'),
    DROP COLUMN anchor_json,
    DROP COLUMN evidence_json,
    ADD CONSTRAINT codeindex_edges_logical_key_length CHECK (octet_length(logical_key) = 32);
  CREATE INDEX idx_codeindex_edges_logical ON codeindex_edges(org_id, logical_key, snapshot_id);
END $$;
