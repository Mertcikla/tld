-- Tenant identity belongs in every codeindex key, including immutable entities.
-- Normalize legacy NULL organisations to the single-tenant sentinel so retries
-- conflict within that tenant on both dialects. Preserve all existing rows.

ALTER TABLE codeindex_repository_settings DROP CONSTRAINT codeindex_repository_settings_repository_id_fkey;

ALTER TABLE codeindex_active_maps DROP CONSTRAINT codeindex_active_maps_run_id_fkey;

UPDATE codeindex_repositories SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_repositories ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_repositories ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_repositories DROP CONSTRAINT codeindex_repositories_pkey;

ALTER TABLE codeindex_repositories ADD PRIMARY KEY (org_id, id);

UPDATE codeindex_snapshots SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_snapshots ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_snapshots ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_snapshots DROP CONSTRAINT codeindex_snapshots_pkey;

ALTER TABLE codeindex_snapshots ADD PRIMARY KEY (org_id, id);

UPDATE codeindex_sources SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_sources ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_sources ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_sources DROP CONSTRAINT codeindex_sources_pkey;

ALTER TABLE codeindex_sources ADD PRIMARY KEY (org_id, snapshot_id, path);

UPDATE codeindex_facts SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_facts ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_facts ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_facts DROP CONSTRAINT codeindex_facts_pkey;

ALTER TABLE codeindex_facts ADD PRIMARY KEY (org_id, id);

UPDATE codeindex_chunks SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_chunks ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_chunks ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_chunks DROP CONSTRAINT codeindex_chunks_pkey;

ALTER TABLE codeindex_chunks ADD PRIMARY KEY (org_id, id);

UPDATE codeindex_edges SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_edges ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_edges ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_edges DROP CONSTRAINT codeindex_edges_pkey;

ALTER TABLE codeindex_edges ADD PRIMARY KEY (org_id, id);

UPDATE codeindex_analysis_runs SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_analysis_runs ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_analysis_runs ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_analysis_runs DROP CONSTRAINT codeindex_analysis_runs_pkey;

ALTER TABLE codeindex_analysis_runs ADD PRIMARY KEY (org_id, id);

UPDATE codeindex_groups SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_groups ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_groups ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_groups DROP CONSTRAINT codeindex_groups_pkey;

ALTER TABLE codeindex_groups ADD PRIMARY KEY (org_id, id);

UPDATE codeindex_group_members SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_group_members ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_group_members ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_group_members DROP CONSTRAINT codeindex_group_members_pkey;

ALTER TABLE codeindex_group_members ADD PRIMARY KEY (org_id, group_id, fact_id);

UPDATE codeindex_elements SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_elements ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_elements ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_elements DROP CONSTRAINT codeindex_elements_pkey;

ALTER TABLE codeindex_elements ADD PRIMARY KEY (org_id, logical_key);

UPDATE codeindex_completed_maps SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_completed_maps ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_completed_maps ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_completed_maps DROP CONSTRAINT codeindex_completed_maps_pkey;

ALTER TABLE codeindex_completed_maps ADD PRIMARY KEY (org_id, run_id);

UPDATE codeindex_impacts SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_impacts ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_impacts ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_impacts DROP CONSTRAINT codeindex_impacts_pkey;

ALTER TABLE codeindex_impacts ADD PRIMARY KEY (org_id, repository_id, comparison_key);

UPDATE codeindex_leases SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_leases ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_leases ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_leases DROP CONSTRAINT codeindex_leases_pkey;

ALTER TABLE codeindex_leases ADD PRIMARY KEY (org_id, repository_id);

UPDATE codeindex_watch_state SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_watch_state ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_watch_state ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_watch_state DROP CONSTRAINT codeindex_watch_state_pkey;

ALTER TABLE codeindex_watch_state ADD PRIMARY KEY (org_id, repository_id);

UPDATE codeindex_project_artifacts SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_project_artifacts ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_project_artifacts ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_project_artifacts DROP CONSTRAINT codeindex_project_artifacts_pkey;

ALTER TABLE codeindex_project_artifacts ADD PRIMARY KEY (org_id, snapshot_id, project_key);

UPDATE codeindex_snapshot_facts SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_snapshot_facts ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_snapshot_facts ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_snapshot_facts DROP CONSTRAINT codeindex_snapshot_facts_pkey;

ALTER TABLE codeindex_snapshot_facts ADD PRIMARY KEY (org_id, snapshot_id, fact_id);

UPDATE codeindex_snapshot_chunks SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_snapshot_chunks ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_snapshot_chunks ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_snapshot_chunks DROP CONSTRAINT codeindex_snapshot_chunks_pkey;

ALTER TABLE codeindex_snapshot_chunks ADD PRIMARY KEY (org_id, snapshot_id, chunk_id);

UPDATE codeindex_snapshot_edges SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_snapshot_edges ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_snapshot_edges ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_snapshot_edges DROP CONSTRAINT codeindex_snapshot_edges_pkey;

ALTER TABLE codeindex_snapshot_edges ADD PRIMARY KEY (org_id, snapshot_id, edge_id);

UPDATE codeindex_repository_settings SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_repository_settings ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_repository_settings ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_repository_settings DROP CONSTRAINT codeindex_repository_settings_pkey;

ALTER TABLE codeindex_repository_settings ADD PRIMARY KEY (org_id, repository_id);

UPDATE codeindex_active_maps SET org_id = '00000000-0000-0000-0000-000000000000' WHERE org_id IS NULL;

ALTER TABLE codeindex_active_maps ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE codeindex_active_maps ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE codeindex_active_maps DROP CONSTRAINT codeindex_active_maps_pkey;

ALTER TABLE codeindex_active_maps ADD PRIMARY KEY (org_id, repository_id);

ALTER TABLE codeindex_repository_settings ADD FOREIGN KEY (org_id, repository_id) REFERENCES codeindex_repositories(org_id, id) ON DELETE CASCADE;

ALTER TABLE codeindex_active_maps ADD FOREIGN KEY (org_id, run_id) REFERENCES codeindex_completed_maps(org_id, run_id) ON DELETE CASCADE;
