CREATE TABLE codeindex_repository_settings (
    repository_id TEXT PRIMARY KEY REFERENCES codeindex_repositories(id) ON DELETE CASCADE,
    map_overrides TEXT NOT NULL DEFAULT '{}'
);
