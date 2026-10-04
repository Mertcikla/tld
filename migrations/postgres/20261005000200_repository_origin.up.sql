-- Tracks where an indexed repository's checkout came from. remote_url is the
-- canonical browser URL of the repository's remote (empty when there is none).
-- managed marks checkouts cloned by tld into its own data directory, which may
-- safely be removed when the repository is deleted.
ALTER TABLE codeindex_repositories ADD COLUMN remote_url TEXT NOT NULL DEFAULT '';
ALTER TABLE codeindex_repositories ADD COLUMN managed BOOLEAN NOT NULL DEFAULT FALSE;
