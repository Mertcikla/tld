-- Links workspace elements to indexed codeindex repositories. Nullable: a NULL
-- repository_id means the element purely relies on the legacy repo/branch/
-- file_path string fields.
ALTER TABLE elements ADD COLUMN repository_id TEXT NULL;

CREATE INDEX IF NOT EXISTS idx_elements_repository ON elements(repository_id);
