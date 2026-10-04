-- Embedding vectors are no longer produced or consumed: repository maps use the
-- dependency graph and semantic populate/search are removed.
DROP TABLE IF EXISTS codeindex_embedding_vec;
DROP TABLE IF EXISTS _vec_codeindex_embedding_vec;
DROP TABLE IF EXISTS codeindex_embedding_cache;
DROP TABLE IF EXISTS codeindex_fact_embeddings;
DROP TABLE IF EXISTS codeindex_embeddings;
-- The unused embedding_status column on codeindex_snapshots is retained:
-- SQLite has no idempotent DROP COLUMN, and these migrations must be re-runnable
-- for databases whose history predates migration tracking.
