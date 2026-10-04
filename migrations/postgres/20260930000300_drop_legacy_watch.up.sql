-- Remove the legacy watch pipeline tables, replaced by the codeindex schema.

DROP TABLE IF EXISTS watch_version_resources;
DROP TABLE IF EXISTS watch_representation_diffs;
DROP TABLE IF EXISTS watch_versions;
DROP TABLE IF EXISTS watch_context_expansions;
DROP TABLE IF EXISTS watch_context_policies;
DROP TABLE IF EXISTS watch_apply_locks;
DROP TABLE IF EXISTS watch_locks;
DROP TABLE IF EXISTS watch_representation_runs;
DROP TABLE IF EXISTS watch_architecture_links;
DROP TABLE IF EXISTS watch_materialization;
DROP TABLE IF EXISTS watch_cluster_members;
DROP TABLE IF EXISTS watch_clusters;
DROP TABLE IF EXISTS watch_filter_decisions;
DROP TABLE IF EXISTS watch_filter_runs;
DROP TABLE IF EXISTS watch_embeddings;
DROP TABLE IF EXISTS watch_embedding_models;
DROP TABLE IF EXISTS watch_scan_runs;
DROP TABLE IF EXISTS watch_symbol_identities;
DROP TABLE IF EXISTS watch_facts;
DROP TABLE IF EXISTS watch_references;
DROP TABLE IF EXISTS watch_symbols;
DROP TABLE IF EXISTS watch_files;
DROP TABLE IF EXISTS watch_repositories;
