CREATE TABLE IF NOT EXISTS view_markdown_documents (
  view_id INTEGER PRIMARY KEY REFERENCES views(id) ON DELETE CASCADE,
  org_id TEXT NULL,
  path TEXT NOT NULL,
  is_managed BOOLEAN NOT NULL DEFAULT FALSE,
  source_kind TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

ALTER TABLE view_markdown_documents ADD COLUMN IF NOT EXISTS source_kind TEXT NOT NULL DEFAULT '';

UPDATE view_markdown_documents
SET source_kind = CASE WHEN is_managed THEN 'PRIVATE_APP' ELSE 'ATTACHED' END
WHERE source_kind = '';
