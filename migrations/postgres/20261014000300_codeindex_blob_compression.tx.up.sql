DO $$
BEGIN
  PERFORM pg_advisory_xact_lock(20261014000300);
  -- Retry safely if DDL committed before its migration history was recorded.
  IF EXISTS (SELECT 1 FROM information_schema.columns
    WHERE table_schema=current_schema() AND table_name='codeindex_blobs' AND column_name='compression') THEN
    RETURN;
  END IF;
  ALTER TABLE codeindex_blobs
    ALTER COLUMN syntax_cache DROP DEFAULT,
    ALTER COLUMN syntax_cache TYPE BYTEA USING convert_to(syntax_cache, 'UTF8'),
    ALTER COLUMN syntax_cache SET DEFAULT ''::bytea,
    ALTER COLUMN file_cache DROP DEFAULT,
    ALTER COLUMN file_cache TYPE BYTEA USING convert_to(file_cache, 'UTF8'),
    ALTER COLUMN file_cache SET DEFAULT ''::bytea,
    ADD COLUMN compression INTEGER NOT NULL DEFAULT 0;
END $$;
