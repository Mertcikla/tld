-- The format marker makes retry safe if migration history recording fails.
DO $$
BEGIN
  PERFORM pg_advisory_xact_lock(20261014000100);
  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema()
    AND table_name='codeindex_repositories' AND column_name='mapping_namespace') THEN
    RETURN;
  END IF;
-- Index format 2. Retain configuration and ownership; discard derived graphs.
CREATE TEMP TABLE codeindex_repository_ids ON COMMIT DROP AS
  SELECT org_id, id AS old_id, ROW_NUMBER() OVER (ORDER BY org_id, id) AS new_id
  FROM codeindex_repositories;
ALTER TABLE codeindex_active_maps DROP CONSTRAINT codeindex_active_maps_org_id_run_id_fkey;
ALTER TABLE codeindex_repository_settings DROP CONSTRAINT codeindex_repository_settings_org_id_repository_id_fkey;
ALTER TABLE codeindex_repositories ADD COLUMN mapping_namespace TEXT NOT NULL DEFAULT '';
UPDATE codeindex_repositories SET mapping_namespace=id, latest_snapshot_id='0';
UPDATE codeindex_repositories r SET id='format2:' || m.new_id::text FROM codeindex_repository_ids m
  WHERE m.org_id=r.org_id AND m.old_id=r.id;
UPDATE codeindex_repository_settings s SET repository_id='format2:' || m.new_id::text FROM codeindex_repository_ids m
  WHERE m.org_id=s.org_id AND m.old_id=s.repository_id;
UPDATE codeindex_elements e SET repository_id=COALESCE((SELECT m.new_id::text
  FROM codeindex_repository_ids m WHERE m.org_id=e.org_id AND m.old_id=e.repository_id),'0'), snapshot_id='0';
UPDATE elements e SET repository_id=m.new_id::text FROM codeindex_repository_ids m
  WHERE m.org_id=COALESCE(e.org_id,'00000000-0000-0000-0000-000000000000'::uuid) AND m.old_id=e.repository_id;
TRUNCATE codeindex_active_maps, codeindex_group_members, codeindex_groups, codeindex_analysis_runs, codeindex_completed_maps, codeindex_snapshot_facts, codeindex_snapshot_edges, codeindex_project_artifacts, codeindex_sources, codeindex_edges, codeindex_facts, codeindex_snapshots, codeindex_impacts, codeindex_blobs, codeindex_artifacts, codeindex_watch_state, codeindex_leases;
ALTER TABLE codeindex_repositories
  ALTER COLUMN id TYPE BIGINT USING replace(id,'format2:','')::bigint,
  ALTER COLUMN latest_snapshot_id DROP DEFAULT,
  ALTER COLUMN latest_snapshot_id TYPE BIGINT USING latest_snapshot_id::bigint,
  ALTER COLUMN latest_snapshot_id SET DEFAULT 0;
ALTER TABLE codeindex_repository_settings
  ALTER COLUMN repository_id TYPE BIGINT USING replace(repository_id,'format2:','')::bigint;
ALTER TABLE codeindex_elements
  ALTER COLUMN repository_id DROP DEFAULT,
  ALTER COLUMN repository_id TYPE BIGINT USING repository_id::bigint,
  ALTER COLUMN repository_id SET DEFAULT 0,
  ALTER COLUMN snapshot_id DROP DEFAULT,
  ALTER COLUMN snapshot_id TYPE BIGINT USING snapshot_id::bigint,
  ALTER COLUMN snapshot_id SET DEFAULT 0;
ALTER TABLE codeindex_active_maps
  ALTER COLUMN repository_id TYPE BIGINT USING repository_id::bigint,
  ALTER COLUMN run_id TYPE BIGINT USING run_id::bigint;
ALTER TABLE codeindex_group_members
  ALTER COLUMN group_id TYPE BIGINT USING group_id::bigint,
  ALTER COLUMN fact_id TYPE BIGINT USING fact_id::bigint;
ALTER TABLE codeindex_groups
  ALTER COLUMN id TYPE BIGINT USING id::bigint,
  ALTER COLUMN run_id TYPE BIGINT USING run_id::bigint,
  ALTER COLUMN snapshot_id TYPE BIGINT USING snapshot_id::bigint;
ALTER TABLE codeindex_analysis_runs
  ALTER COLUMN id TYPE BIGINT USING id::bigint,
  ALTER COLUMN repository_id TYPE BIGINT USING repository_id::bigint,
  ALTER COLUMN snapshot_id TYPE BIGINT USING snapshot_id::bigint;
ALTER TABLE codeindex_completed_maps
  ALTER COLUMN run_id TYPE BIGINT USING run_id::bigint,
  ALTER COLUMN repository_id TYPE BIGINT USING repository_id::bigint,
  ALTER COLUMN snapshot_id TYPE BIGINT USING snapshot_id::bigint;
ALTER TABLE codeindex_snapshot_facts
  ALTER COLUMN snapshot_id TYPE BIGINT USING snapshot_id::bigint,
  ALTER COLUMN fact_id TYPE BIGINT USING fact_id::bigint;
ALTER TABLE codeindex_snapshot_edges
  ALTER COLUMN snapshot_id TYPE BIGINT USING snapshot_id::bigint,
  ALTER COLUMN edge_id TYPE BIGINT USING edge_id::bigint;
ALTER TABLE codeindex_project_artifacts
  ALTER COLUMN snapshot_id TYPE BIGINT USING snapshot_id::bigint;
ALTER TABLE codeindex_sources
  ALTER COLUMN snapshot_id TYPE BIGINT USING snapshot_id::bigint;
ALTER TABLE codeindex_edges
  ALTER COLUMN id TYPE BIGINT USING id::bigint,
  ALTER COLUMN repository_id TYPE BIGINT USING repository_id::bigint,
  ALTER COLUMN snapshot_id TYPE BIGINT USING snapshot_id::bigint,
  ALTER COLUMN from_fact_id DROP DEFAULT,
  ALTER COLUMN from_fact_id TYPE BIGINT USING from_fact_id::bigint,
  ALTER COLUMN from_fact_id SET DEFAULT 0,
  ALTER COLUMN to_fact_id DROP DEFAULT,
  ALTER COLUMN to_fact_id TYPE BIGINT USING to_fact_id::bigint,
  ALTER COLUMN to_fact_id SET DEFAULT 0;
ALTER TABLE codeindex_facts
  ALTER COLUMN id TYPE BIGINT USING id::bigint,
  ALTER COLUMN repository_id TYPE BIGINT USING repository_id::bigint,
  ALTER COLUMN snapshot_id TYPE BIGINT USING snapshot_id::bigint,
  ALTER COLUMN parent_fact_id DROP DEFAULT,
  ALTER COLUMN parent_fact_id TYPE BIGINT USING parent_fact_id::bigint,
  ALTER COLUMN parent_fact_id SET DEFAULT 0;
ALTER TABLE codeindex_snapshots
  ALTER COLUMN id TYPE BIGINT USING id::bigint,
  ALTER COLUMN repository_id TYPE BIGINT USING repository_id::bigint;
ALTER TABLE codeindex_impacts
  ALTER COLUMN repository_id TYPE BIGINT USING repository_id::bigint;
ALTER TABLE codeindex_repository_settings ADD FOREIGN KEY (org_id, repository_id)
  REFERENCES codeindex_repositories(org_id,id) ON DELETE CASCADE;
ALTER TABLE codeindex_active_maps ADD FOREIGN KEY (org_id,run_id)
  REFERENCES codeindex_completed_maps(org_id,run_id) ON DELETE CASCADE;
CREATE TABLE codeindex_id_sequence (
  singleton INTEGER PRIMARY KEY CHECK (singleton=1),
  value BIGINT NOT NULL
);
INSERT INTO codeindex_id_sequence SELECT 1, COALESCE(MAX(new_id),0) FROM codeindex_repository_ids;

END $$;
