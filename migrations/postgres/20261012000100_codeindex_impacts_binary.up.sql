-- Comparison payloads are stored as binary protobuf documents. Earlier rows
-- held protojson text, which the current store cannot read; impacts are
-- regenerable from immutable snapshots, so drop the cache instead of migrating
-- each document.
DELETE FROM codeindex_impacts;
ALTER TABLE codeindex_impacts ALTER COLUMN result_json TYPE BYTEA USING convert_to(result_json, 'UTF8');
