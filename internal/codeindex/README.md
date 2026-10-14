
| Package |  What it is | codeindex module dep |
| --- |  --- | --- |
| `store` |  **The DB implementation** — bun/SQLite/Postgres persistence for snapshots, facts, edges, mappings, impacts, maps, watch leases | `graph` (assert vs `codeindex/store`) |
| `materialize` |  Projects the fact graph into workspace views/elements/connectors | `graph`, `community` |
| `impact` |  Change-impact diagrams (fact-delta scenes, placement, hierarchy) | `graph`, `community`, `config`, `indexer`, `ingest` |
| `mapconfig` | Map grouping options + per-repository overrides | `graph`, `community`, `materialize` |
| `maprun` | Orchestrates group → materialize → persist for `tld index --map` | `graph`, `community`, `metrics` |
| `configbridge` |  Bridges tld's workspace config → `codeindex/config` | `config` |
| `symbolcheck` |  Answers "does this file declare symbol X" (server/validator) | none (direct gotreesitter) |
| `mappingcheck` | DB-backed "is this logical key mapped" check | none |
| `linkcheck` |  Reads the codeindex file inventory from the local DB | `graph` |

Index format 3 retains global 64-bit integer IDs allocated at publication;
protobuf references remain decimal strings. Edge logical keys are SHA-256
digests of length-delimited structural identities, exposed as 64 lowercase hex
characters and stored as 32-byte SQLite BLOBs or PostgreSQL BYTEA. Fact logical
keys retain their readable format. Public-export qualifiers participate in the
edge digest; observation sites and entity IDs do not.

The format-3 migration discards derived snapshots, analysis results, impacts,
and index caches. Reindex normally to repopulate them. Repository IDs, remotes,
settings, workspace links, diagram resources and ownership records survive;
latest-snapshot pointers and stale mapping snapshot references are cleared.
`mapping_namespace` retains existing mapping keys and group tags so remapping
updates the same resources. Legacy edge keys are not accepted.

This change requires the matching codeindex module changes. Local development
uses the sibling module through the workspace. Before publishing tld, release
codeindex and update the pinned `github.com/mertcikla/codeindex` dependency;
the previous v0.0.2 module does not expose the new key decoder. No local-path
replacement is added to the portable core module.

Edge anchors and evidence now share one versioned binary `details` payload,
using the existing protobuf messages. Evidence anchors equal to the edge anchor
are stored as references and reconstructed on reads. Distinct/absent anchors,
nil entries, empty lists, order, and unknown protobuf fields are preserved.
Corrupt or unsupported payloads fail reads instead of silently losing provenance.
Facts still use their existing JSON encoding. RPC message shapes are unchanged.

SQLite reuses freed pages but does not automatically shrink its database file.
After rebuilding, stop database users and run a separate maintenance `VACUUM`
to reclaim disk space. Startup migrations never run `VACUUM`.

To repeat the space measurement, create an isolated backup of a format-2
database and run:

```sh
TLD_EDGE_STORAGE_BENCHMARK_DB=/tmp/backup.db go test ./internal/codeindex/store -run TestEdgeStorageSize -v
go test ./internal/codeindex/store -run '^$' -bench BenchmarkEdgeDetails -benchmem
```

The measurement checks every edge for lossless round trips and reports payload,
table, and index sizes. The supplied database's 41,011 edges reduced raw payload
from 36,922,718 to 12,246,879 bytes (66.8%). Edge table plus indexes reduced from
64,290,816 to 25,587,712 bytes (60.2%); unrelated database tables are excluded.
Keeping all unrelated data and vacuuming both whole-database copies reduced the
database from 137,809,920 to 101,490,688 bytes (26.4%). This isolates edge-format
savings from reclaimed freelist space and from discarded derived snapshots.
