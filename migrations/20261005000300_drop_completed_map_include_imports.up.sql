-- External imports are always materialized in full maps; the per-map
-- include_imports flag is obsolete.
ALTER TABLE codeindex_completed_maps DROP COLUMN include_imports;
