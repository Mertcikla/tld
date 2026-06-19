CREATE TABLE IF NOT EXISTS view_markdown_documents (
  view_id INTEGER PRIMARY KEY,
  org_id TEXT NULL,
  path TEXT NOT NULL,
  is_managed INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (view_id) REFERENCES views(id) ON DELETE CASCADE
);

ALTER TABLE view_markdown_documents ADD COLUMN source_kind TEXT NOT NULL DEFAULT '';

UPDATE view_markdown_documents
SET source_kind = CASE WHEN is_managed THEN 'PRIVATE_APP' ELSE 'ATTACHED' END
WHERE source_kind = '';
