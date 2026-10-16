
| Package |  What it is | codeindex module dep |
| --- |  --- | --- |
| `store` |  **The DB implementation** — bun/SQLite/Postgres persistence for snapshots, facts, edges, mappings, impacts, maps, watch leases | `graph`, `symbols` (assert vs `codeindex/store`) |
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

Edges store only relationship identity, endpoints, unresolved target symbols,
and weights. Extractors use observation sites internally for ownership and
deduplication, but emit no edge anchors or evidence. Source locations and
provenance remain on facts; source blobs, SCIP import caches and syntax caches
retain the data used for extraction. RPC message shapes are unchanged, with
edge anchor and evidence fields unset. Fresh databases have no `details` column.

SQLite reuses freed pages but does not automatically shrink its database file.
After rebuilding, stop database users and run a separate maintenance `VACUUM`
to reclaim disk space. Startup migrations never run `VACUUM`.

Publication shares fact and edge rows within a repository. A SHA-256
fingerprint covers stored payload fields, including fact locations,
evidence and parent IDs. For facts, the defining package's SCIP version is
excluded from symbol metadata before hashing; each snapshot keeps its package
versions in `symbol_versions_json` and restores them on reads. Dependency and
ambiguous package versions remain identifying. An equal row is reused through the
existing `codeindex_snapshot_facts` or `codeindex_snapshot_edges` membership
table. Repeated equal observations remain separate rows to preserve counts.
The additive migrations preserve existing rows and add fingerprint columns,
indexes and the snapshot version map. Old rows remain readable and acquire no
fingerprints retroactively.

SCIP parsing, normalization and formatting live in `codeindex/symbols`. The
database store persists the opaque version map and calls the module's fact
helpers; it does not import the SCIP library directly.

Sources and extraction artifacts keep their existing whole-payload sharing.
Memberships and inventories are still stored per snapshot, and changes to
positions require new fact rows. Storage growth therefore
includes these costs rather than being strictly proportional to edited bytes.

## Conservative map upsert

Mapping updates source links and adds generated content, but never deletes
existing elements, views, connectors, layers or placements. Existing element
presentation, connector endpoints/routes, view ownership and layer membership
are preserved. Stable ownership keys let a later snapshot reuse retained resources.

`codeindex:stale` is a reserved status tag on retained elements, views and
connectors absent from the last applied map. It is cleared when they reappear.
It means absent from that map, which can also result from grouping or budget
changes; it is not proof that source was deleted. Retained placements and layers
have no independent status, and a current group can contain older placements.
Snapshots and completed-map records are history, not isolated copies of views.
An interrupted run can leave additive changes; retry reuses committed resources.

This release has one policy: conservative upsert. Reconciliation is centralized
in `materialize/upsert.go`; manual/sync policies and workspace/view preference
persistence are deferred. There are no new RPCs or schema changes. Cleanup is an
explicit user action, and map size can grow across snapshots.
