
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

Community maps use conservative upsert. Reconciliation is centralized
in `materialize/upsert.go`; manual/sync policies and workspace/view preference
persistence are deferred. There are no new RPCs or schema changes. Community-map cleanup is an
explicit user action, and retained library resources can grow across snapshots.

### Complete source hierarchy

`tld index . --full-map` preserves the complete directory/file hierarchy and
stops at files. Files use short names in their containing folder; source links
retain the full path. Only directories get child views. Functions, methods,
classes, configuration facts, and external symbols remain in the index and are
not materialized as workspace elements.

Symbol relationships are rolled up into directed file-to-file connectors.
When files in separate folder branches are connected, the map also creates one
unlabeled directed connector between their containing folders. This rule repeats
up the hierarchy until the branches meet, so dependencies remain visible when
folders are collapsed. For unequal folder depths, the deeper branch is lifted
first. Dependencies within one branch do not create self-connectors or links
between a folder and its own descendants. File connectors retain symbol anchors;
folder summaries use folder borders and merge all underlying relationship kinds.
Calls and references share one unlabeled connector per ordered file pair.
Repeated observations are merged; opposite directions remain separate. The
merged connector retains every participating symbol's declaration line on both
endpoints. Code previews can show all connected symbols without creating symbol
nodes. Source handles use one-based `right-line-12,27` / `left-line-5,18` lists;
existing single-line handles remain supported.
Implementations and type definitions retain their distinct relationship labels.
Upgrading reuses an existing calls/references connector where possible, removes
its generated label, and removes duplicate generated calls/references connectors.
Intra-file relationships and unresolved/external targets are omitted from the map. Connector budgets and community grouping do not apply.

Use `--watch --full-map` to refresh after scans, and add `--detach` to run in the
background. `--full-map` and `--map` are mutually exclusive. Full maps manage
placements inside their generated hierarchy. Upgrading an older symbol map
removes its generated symbol connectors and placements in generated views, and
removes empty child views with their default presentation. Library elements and additional placements
in authored views are retained; superseded resources receive a stale tag.
Indexed facts and relationships remain unchanged. The full-map cache version
forces an upgrade even when the snapshot is unchanged.
