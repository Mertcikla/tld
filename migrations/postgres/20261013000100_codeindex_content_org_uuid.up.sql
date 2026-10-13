-- Shared content must use the same tenant key type as its snapshot references.
-- Convert existing rows as well as fresh installations so orphan collection
-- can compare tenant keys without a text/UUID operator error.
ALTER TABLE codeindex_blobs
  ALTER COLUMN org_id DROP DEFAULT,
  ALTER COLUMN org_id TYPE UUID USING org_id::uuid,
  ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE codeindex_artifacts
  ALTER COLUMN org_id DROP DEFAULT,
  ALTER COLUMN org_id TYPE UUID USING org_id::uuid,
  ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000';
