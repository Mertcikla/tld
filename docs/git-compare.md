# `tld git compare` — Quick Start

Compare two Git revisions and emit the repository impact diagram. Missing
snapshots are indexed on demand. Runs against the local data dir; no server
needed.

## Syntax

```bash
tld git compare [repository] <base> <head>
```

`repository` is optional and defaults to the current checkout. It accepts a
repository id, a local path, or a remote URL (`github.com/owner/repo`,
`owner/repo`, or a Git URL).

## Examples

```bash
# Current checkout, last commit vs previous
tld git compare HEAD~1 HEAD

# A range of commits
tld git compare main..feature

# Mermaid diagram for a PR description
tld git compare HEAD~1 HEAD --mermaid

# Mermaid inside a Markdown code fence
tld git compare HEAD~1 HEAD --markdown

# Compare a remote repo (cloned + indexed on demand)
tld git compare github.com/owner/repo v1.2.0 v1.3.0

# Compare a known repository by id
tld git compare 3f9c1a... HEAD~5 HEAD
```

## Flags

| Flag | Default | Purpose |
|---|---|---|
| `--mermaid` | off | Emit the Mermaid change diagram instead of the scene |
| `--markdown` | off | Wrap the Mermaid diagram in a Markdown code fence |
| `--plain` | off | Hide unrelated authored context in the selected scene; retain changes, dependency neighbours and required containers |
| `-v`, `--verbose` | off | Report base/head indexing progress on stderr |
| `--max-nodes N` | `400` | Node budget; output is narrowed when exceeded (`0` disables) |
| `--max-bytes N` | `2097152` | Output byte budget; output is narrowed when exceeded (`0` disables) |
| `--blast-radius N` | `0` | Dependency hops of unchanged neighbours and edges to include around each change (`0`–`2`) |
| `--max-elements N` | `0` | Skip output if the requested diagram has more than N elements (`0` disables) |
| `--max-connectors N` | `0` | Skip output if the requested diagram has more than N connectors (`0` disables) |
| `--view REF` | unset | Restrict the grounded bundle to one authored view (id or name substring) |
| `--all-edges` | off | Disable fan-out roll-up in the grounded summary |
| `--debug-provenance` | off | Write pipeline stage counts and decisions to stderr |
| `--provenance-json PATH` | unset | Write structured per-primitive provenance, coverage and detailed decisions to a JSON sidecar |
| `--report-json PATH` | unset | Write status, requested counts, change stats, resolved BASE/HEAD SHAs, index warnings, grounded overlay, and skip reason to JSON |
| `--prepare-command COMMAND` | unset | Run a Bash command in each uncached revision's temporary checkout before indexing |
| `--data-dir PATH` | global | Override the data directory |

## Output

- **Default:** a portable scene and a grounded summary derived from it. Standard
  mode keeps the authored skeleton of selected or relevant views, including
  unlinked conceptual elements. Direct changes use authored representations
  first, compatible mapped representations second, then transient file
  primitives. Symbol/line anchors cover only their matched scope; remaining
  file edits receive a fallback. The result includes a coverage entry for every
  changed file, including binaries and files excluded from indexing.
- **`--blast-radius 1` / `2`:** add neighbours from bounded dependency traversal
  in BASE and HEAD separately. Each neighbour keeps its distance and witness
  path. Neighbours without authored or mapped representations receive raw
  file primitives. Traversal uses both directions; rendered relationships keep
  their observed direction.
- **`--mermaid` / `--markdown`:** render the same selected scene as the canvas,
  with nested views, change annotations, provenance glyphs and directed edges.
  A synthesized connector is marked added only when the source dependency
  was added. Multiple observations between the same representatives share a
  connector and retain their evidence in provenance.

Comparisons freeze the scene and provenance. Subsequent workspace edits produce
another comparison key; opening an older result does not reread the workspace.
The source-analysis cache depends on snapshots and radius; the scene cache also
includes the captured workspace, mappings, repository identities and view filter.
Old artifacts remain readable through the version 1 compatibility path.

New snapshots persist complete Git file manifests independently of extraction.
Legacy commit snapshots are enriched when their pinned Git revision is available.
Legacy/offline working-tree snapshots without manifests explicitly report
incomplete coverage. Files without captured sources keep unknown line counts and
unknown dependency impact; a file primitive does not imply complete facts.

```bash
tld git compare HEAD~1 HEAD --blast-radius 2 \
  --debug-provenance --provenance-json /tmp/impact-provenance.json
```

The JSON sidecar contains compact provenance and the full decision ledger,
without duplicating scene geometry. Debug output goes to stderr so stdout stays
machine-readable. Normal scene responses include compact origin, grounding,
role, stage, paths and witnesses; detailed stage decisions require debug opt-in.

The proto contract reuses `SourceFile` for manifest entries (with an optional Git
mode value) and `SourceChange` for coverage (with selected placement keys).
Stage counts and decisions belong directly to `ImpactScene`; dependency witness
seeds are the first path entry. Provenance, dependency witnesses, decisions and
stage counters retain distinct types because they represent different records.
Scene schema 3 prevents older cached layouts from bypassing this contract.

All canvas membership, grounding, coverage and Plain/radius selection decisions
belong to canonical scene assembly and selection. The frontend, Mermaid export
and grounded summary only project that scene; they do not rebuild a fact graph
or apply independent workspace matching. Older comparison artifacts without a
canonical scene are rebuilt through the current pipeline when their indexed
snapshots remain available.

The repositories page exposes Hops 0/1/2 before Compare and shows the Mermaid
preview by default. Developers can open the page with `?debugImpact=1` to replace
that panel with the Impact inspector. Select an element, connector or view on
the canvas, or search the inspector, to see its origin, grounding, inclusion
stage, source paths and dependency witnesses. **Impact debug** toggles detailed
stage counts and suppression decisions within the inspector. Copy Markdown is
available in both panels. Standard/Plain selection happens in the backend;
the inspector reports when frontend tag filters hide changed-file coverage.

Progress is written to stderr only with `--verbose`; budget warnings always go
to stderr. If the payload exceeds a budget, a warning tells you to raise
`--max-nodes`.

Hard element and connector limits are checked **before** the shrinking budgets
and rendering. Either limit being exceeded suppresses stdout and exits
successfully; both indexed snapshots remain in the database. Counts equal to a
limit are allowed. The optional JSON report uses `ready`, `skipped`, or `empty`
status and reports the requested scope, even if the shrinking budgets later
narrow the rendered output. It also carries a `stats` object derived from the
snapshot diff — `files`, `directories`, `subsystems`, `linesAdded`,
`linesRemoved`, `symbolsAdded`, `symbolsModified`, `symbolsRemoved`,
`edgesAdded`, `edgesRemoved`, `edgesModified`, and the
changed `paths` (capped at 500, with `pathsTruncated` set beyond that). Stats
describe the change itself, so they are present even when the diagram is
skipped or empty. Command failures still return a nonzero exit code.

`--prepare-command` runs only when a snapshot needs indexing. Its contents are
part of the snapshot configuration hash, so changing setup invalidates cached
snapshots. Setup stdout and stderr go to stderr. Each revision has its own
temporary checkout: install dependencies and generate required build assets
there, rather than only in the original checkout. A setup failure stops the
comparison. Preparation should leave tracked indexing inputs unchanged.

See [the tld PR diagram bot](https://github.com/Mertcikla/tld-pr-bot) for cached GitHub PR comments and fork support.

## Tips

- Only directly changed sections are emitted by default; `--blast-radius 1`
  or `2` widens the bundle to neighbours and edges that many hops out.
  Node and byte budgets narrow oversized output automatically.

CLI, RPC, and live/watch impact default to blast radius `0` (direct changes
only). RPC callers can set `blast_radius` to `0`–`2`, matching `--blast-radius`.
Comparisons share a cache keyed by snapshot pair and radius; the live result
also keeps a mutable `live` alias. Scene and Mermaid exports preserve the
cached comparison’s radius.
