-- Keep legacy payloads verbatim (compression = 0). Binary cache columns allow
-- new blobs to store gzip bytes without base64 overhead.
CREATE TABLE codeindex_blobs_compressed (
  org_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  hash TEXT NOT NULL,
  language TEXT NOT NULL DEFAULT '',
  content BLOB,
  syntax_cache BLOB NOT NULL DEFAULT X'',
  file_cache BLOB NOT NULL DEFAULT X'',
  compression INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (org_id, hash, language)
);
INSERT INTO codeindex_blobs_compressed (org_id, hash, language, content, syntax_cache, file_cache)
  SELECT org_id, hash, language, content, CAST(syntax_cache AS BLOB), CAST(file_cache AS BLOB)
  FROM codeindex_blobs;
DROP TABLE codeindex_blobs;
ALTER TABLE codeindex_blobs_compressed RENAME TO codeindex_blobs;
