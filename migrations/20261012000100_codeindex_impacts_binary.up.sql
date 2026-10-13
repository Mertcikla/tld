-- Comparison payloads are stored as binary protobuf documents. Earlier rows
-- held protojson text, which the current store cannot read; impacts are
-- regenerable from immutable snapshots, so drop the cache instead of migrating
-- each document. SQLite keeps the TEXT-affinity column, which stores BLOBs
-- unchanged.
DELETE FROM codeindex_impacts;
