
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
