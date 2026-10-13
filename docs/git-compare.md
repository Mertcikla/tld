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
tld git compare main feature

# Mermaid diagram for a PR description
tld git compare HEAD~1 HEAD --mermaid

# Mermaid inside a Markdown code fence
tld git compare HEAD~1 HEAD --markdown

# Self-contained link for the local web viewer (no upload)
tld git compare HEAD~1 HEAD --url

# Link using the hosted viewer for sharing with other people
tld git compare HEAD~1 HEAD --url --url-base https://tldiagram.com/app

# Open the same comparison in the installed desktop app
tld git compare HEAD~1 HEAD --url-local

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
| `--url` | off | Emit a self-contained link for the local web viewer; maximum 10,000 characters |
| `--url-base URL` | local server | Override the application base URL for `--url`, such as `https://tldiagram.com/app` |
| `--url-local` | off | Emit a self-contained `tld://compare` link for the desktop app; maximum 10,000 characters |
| `--plain` | off | Hide unrelated authored context in the selected scene; retain changes, dependency neighbours and required containers |
| `-v`, `--verbose` | off | Report base/head indexing progress on stderr |
| `--max-nodes N` | `400` | Node budget; output is narrowed when exceeded (`0` disables) |
| `--max-bytes N` | `2097152` | Output byte budget; output is narrowed when exceeded (`0` disables) |
| `--blast-radius N` | `0` | Dependency hops of unchanged neighbours and edges to include around each change (`0`–`2`) |
| `--max-elements N` | `0` | Skip output if the requested diagram has more than N elements (`0` disables) |
| `--max-connectors N` | `0` | Skip output if the requested diagram has more than N connectors (`0` disables) |
| `--view REF` | unset | Restrict the scene to one authored view (id or name substring) |
| `--debug-provenance` | off | Include structured matching and dependency audit evidence; write audit decisions to stderr |
| `--report-json PATH` | unset | Write command status and canonical comparison metadata, file ledger, summary and statistics |
| `--prepare-command COMMAND` | unset | Run a Bash command in each uncached revision's temporary checkout before indexing |
| `--data-dir PATH` | global | Override the data directory |

## Output

URL exports gzip and base64url-encode the existing selected JSON scene, including
optional debug provenance, in a `#v1=` fragment. No data is uploaded. The standalone
viewer renders the frozen canvas and Mermaid diagram without login, a repository,
or a cached comparison. Web links offer **Open in TLD**; local links require an
installed desktop app registered for the `tld` protocol (macOS application or
Windows NSIS installation).

`--url` uses `serve.public_url` when explicitly configured; otherwise it uses a
running local server registered for the selected data directory, or the configured
`tld serve` host/port (default `http://127.0.0.1:8060`). It does not use the cloud
`server_url` setting or start a server. Run `tld serve` to open a local link. Use
`--url --url-base https://tldiagram.com/app` for a hosted share link, or pass another
HTTP(S) application base URL, including its `/app` prefix when applicable.

The complete URL, excluding its trailing newline, must be at most **10,000
characters**. Longer exports fail with a nonzero exit code and no stdout; indexed
snapshots and cached comparisons remain available. An optional JSON report records
`skipped` and the failure reason. Existing node/byte budgets apply before encoding;
the command does not narrow the scene further to meet the URL limit. Both export
and import limit the decoded document to 16 MiB. URL output flags cannot be combined
with each other or with `--mermaid`/`--markdown`. The encoded scene is complete and
is not encrypted; anyone with the link can read its contents.

- **Default:** the canonical `ImpactScene` directly, with `comparison`, `files`,
  `symbols`, `summary`, `tree`, `views` and `navigations`. Standard
  mode keeps the authored skeleton of selected or relevant views, including
  unlinked conceptual elements. Direct changes use authored representations
  first, compatible mapped representations second, then transient file
  primitives. Symbol/line anchors cover only their matched scope; remaining
  file edits receive a fallback. The result includes a compact file-ledger entry for every
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
Schema/pipeline version 5 invalidates unreleased comparison artifacts, including
stale live results. Regenerate those comparisons; indexed snapshots are retained.
Historical scene reads never rebuild against the current workspace.

New snapshots persist complete Git file manifests independently of extraction.
Legacy commit snapshots are enriched when their pinned Git revision is available.
Legacy/offline working-tree snapshots without manifests explicitly report
incomplete coverage. Commit comparisons capture pinned BASE→HEAD Git statistics and HEAD-not-BASE
commit metadata once. File totals are **net tree churn**, never a sum of commit
churn; renames remain removal plus addition. Git line counts remain available
for files excluded from extraction. Binary counts and missing counts remain
absent, with separate binary totals and explicit aggregate completeness.
Working-tree/offline results use only captured evidence and explicitly mark
unavailable Git statistics/history. A representative does not imply complete
analysis: each file records base/head analysis availability independently.

```bash
tld git compare HEAD~1 HEAD --blast-radius 2 \
  --debug-provenance > /tmp/impact-debug.json
```

Ordinary rendering reads placement origin, role and grounding directly; it does
not require debug provenance. Changed symbols live once in a compact table and
file entries/overlays reference them by deterministic short keys. Symbol `path`, `startLine` and `endLine`
keep zero-based index locations; labels and editor links convert to one-based lines. Repository and
comparison identity live in `comparison`; internal workspace, snapshot, fact
and map hashes are omitted.

`--debug-provenance` adds accepted/rejected matching candidates, original authored
links and normalized scopes, partial symbol/line coverage, stale-map fallbacks,
required-container reasons, selection omissions, snapshot-specific directed
dependency observations/witnesses, and explanations for indexed symbols excluded
from display. It excludes raw patches, routine pipeline history and expanded
folder memberships. Debug bytes do not affect node/byte-budget selection; the
normal payload determines membership, with a separate notice for debug overhead.

All canvas membership, grounding, coverage and Plain/radius selection decisions
belong to canonical assembly and selection. CLI, RPC, Mermaid and the frontend
only project that model; exporters perform no independent workspace matching.
Mermaid annotations and report statistics use the same canonical summary.

The repositories page exposes Hops 0/1/2 before Compare and shows the Mermaid
preview by default. Developers can open `?debugImpact=1` for the file-first Impact
inspector: changed files → match evidence → workspace representatives. Selected
representatives expose observed dependencies, witnesses and rejected candidates.
**Impact debug** toggles the audit evidence; Standard/Plain selection happens in
the backend. The inspector also reports changed files hidden by tag filters.

Progress is written to stderr only with `--verbose`; budget warnings always go
to stderr. If the payload exceeds a budget, a warning tells you to raise
`--max-nodes`.

Hard element and connector limits are checked **before** the shrinking budgets
and rendering. Either limit being exceeded suppresses stdout and exits
successfully; both indexed snapshots remain in the database. Counts equal to a
limit are allowed. The optional JSON report uses `ready`, `skipped`, or `empty`
status and includes the canonical `comparison`, `files` and `summary` fields.
Successful reports describe the emitted selection; skipped reports describe the
requested scene. Its `stats` object is derived from that same model — `files`, `directories`, `subsystems`, `linesAdded`,
`linesRemoved`, `lineCountsComplete`, `binaryFiles`, `indexedSymbolsAdded`, `indexedSymbolsModified`, `indexedSymbolsRemoved`,
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
only). RPC callers can set `context_depth` to `0`–`2`, matching `--blast-radius`.
Comparisons share a cache keyed by snapshot pair and radius; the live result
also keeps a mutable `live` alias. Scene and Mermaid exports preserve the
cached comparison’s radius.

## Reviewing a comparison

Use the same compare command as the canvas, with the audit flag enabled:

```bash
tld git compare BASE HEAD --debug-provenance > /tmp/impact.json
git log --topo-order --stat --no-renames BASE..HEAD
git diff --no-renames --numstat BASE HEAD
```

Start at `files`: verify changed paths and net churn independently of analysis.
Follow each entry's `representatives` to `views[viewId].placements`, then inspect
`provenance.matches` for the authored source link, normalized scope, snapshot
side and partial coverage. Rejected candidates identify their chosen replacements;
omitted targets resolve names through `provenance.references`. Compare displayed
symbol references with `excludedSymbols`, and validate each directed dependency
and witness within its named snapshot. `declaredConnectors` are workspace claims,
separate from observed source dependencies. Unknown counts remain absent. File counts (`linesAdded`/`linesRemoved`) and
`baseAnalysis`/`headAnalysis` live directly on each file. `excludedSymbols` and
`references` are maps from canonical short references to explanations or names.

The public proto contains only portable API data. Analysis nodes, assembly
bookkeeping and cache state are private Go types; routine stage counters and
operation history are not retained. Snapshot diffs have no scene placement data.
