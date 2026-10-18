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

# Adapt context and presentation for a PR comment
tld git compare main feature --auto --markdown --report-json comparison.json

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
| `--auto` | off | Adapt context for PR review; Markdown can contain up to two diagrams or a changed-file summary |
| `--url` | off | Emit a self-contained link for the local web viewer; maximum 10,000 characters |
| `--url-base URL` | local server | Override the application base URL for `--url`, such as `https://tldiagram.com/app` |
| `--url-local` | off | Emit a self-contained `tld://compare` link for the desktop app; maximum 10,000 characters |
| `--plain` | off | Mermaid: keep change/dependency evidence and flatten view wrappers. JSON/canvas: retain the existing Plain selection |
| `--theme PATH` | embedded theme | Merge a JSON theme override into the default for `--mermaid` or `--markdown` |
| `--no-theme` | off | Omit all Mermaid class and link styling; keep textual status labels |
| `-v`, `--verbose` | off | Report base/head indexing progress on stderr |
| `--max-nodes N` | `400` | Node budget; output is narrowed when exceeded (`0` disables) |
| `--max-bytes N` | `2097152` | Output byte budget; output is narrowed when exceeded (`0` disables) |
| `--blast-radius N` | `0` | Dependency hops (`0`–`2`); Mermaid also bounds authored context by declared connector hops |
| `--max-elements N` | `0` | Skip output if the requested diagram has more than N elements (`0` disables) |
| `--max-connectors N` | `0` | Skip output if the requested diagram has more than N connectors (`0` disables) |
| `--view REF` | unset | Restrict the scene to one authored view (id or name substring) |
| `--debug-provenance` | off | Include structured matching and dependency audit evidence; write audit decisions to stderr |
| `--report-json PATH` | unset | Write command status and canonical comparison metadata, file ledger, summary and statistics |
| `--prepare-command COMMAND` | unset | Run a Bash command in each uncached revision's temporary checkout before indexing |
| `--data-dir PATH` | global | Override the data directory |

## Mermaid themes

Open [the theme studio](mermaid-theme-editor.html) in a browser to tune every
style and label option against a live mock diagram, import partial overrides,
and download a compatible `theme.json`. The single HTML file embeds the default
theme and loads Mermaid from a pinned CDN URL.

CLI and UI Mermaid output use the embedded [theme.json](../internal/mermaid/theme.json).
The default uses Mermaid's customizable `base` theme with explicit frontmatter
`themeVariables` and the `classic` look. Nodes share the canvas's default slate
fill (`#2d3748`) and light text; amber (`#ecc94b`), green (`#48bb78`), and red
(`#fc8181`) borders and edges indicate modified, added, and removed resources.
Diagram, subgraph, and edge-label backgrounds stay transparent so GitHub's
page background shows through. HTML labels color added line counts yellow and removed counts red. A scoped
`themeCSS` rule keeps Mermaid's HTML edge-label backgrounds transparent. Muted container and edge labels remain visible on light
and dark pages. Nodes, including compact isolated groups, use rounded Mermaid
syntax (`id("Label")`) in UI and CLI output. Mermaid controls layout and does
not reproduce canvas positions, badges, or user-specific appearance settings.
Removed nodes also have dashed borders. Labels retain known line counts without
repeating `modified` or `(context)` in every node. Zero churn counts are omitted
(`+42 −0` becomes `+42`, and `+0 −31` becomes `−31`); counts with both sides zero
produce no churn label. Compact isolated-node groups
are separated by change type so they cannot misrepresent a mixed group.

Use a partial JSON override to customize the presentation:

```json
{
  "version": 1,
  "nodes": {
    "modified": { "fill": "#eaf2ff", "stroke": "#0969da", "color": "#0550ae" },
    "context": { "font-family": "monospace" }
  },
  "edges": { "modified": { "stroke": "#0969da" } },
  "labels": { "churn": false }
}
```

```bash
tld git compare main feature --auto --markdown --theme ./review-theme.json
tld git compare main feature --auto --markdown --no-theme
```

The format has six top-level fields:

- `version`: currently `1`; optional in a partial override.
- `baseTheme`: Mermaid's built-in theme, defaulting to `base`. Choose
  `default`, `neutral`, `dark`, `forest`, `base`, `redux-color`, `redux-dark-color`,
  `redux`, `redux-dark`, `neo`, or `neo-dark`. The theme studio saves this selection.
  Themed CLI and UI exports prepend YAML frontmatter (`config.theme`) before
  `flowchart`. Keep `base` for Mermaid's supported custom theme behavior.
- `themeVariables`: partial overrides for Mermaid's frontmatter theme variables.
  Supported values are `darkMode` (boolean), `fontFamily` (a font stack),
  `fontSize` (a CSS size), and hex colors for `primaryColor`, `primaryTextColor`,
  `primaryBorderColor`, the corresponding `secondary*` and `tertiary*` colors,
  `mainBkg`, `nodeBorder`, `nodeTextColor`, `clusterBorder`, `lineColor`,
  `defaultLinkColor`, `textColor`, and `titleColor`. `background`, `clusterBkg`,
  and `edgeLabelBackground` additionally accept `transparent` (the default).
  Missing variables inherit the embedded defaults. Per-state node/edge styles
  take precedence over these diagram-wide defaults.
- `nodes` and `edges`: each contains `context`, `modified`, `added`, and `removed`
  style objects. Missing states and properties inherit the embedded default.
  An empty string removes a style property; an empty object inherits everything.
- `labels`: `change`, `context`, and `churn` booleans control status words,
  the `(context)` suffix, and known line counts. Defaults are `false`, `false`,
  and `true`. `addedColor` and `removedColor` set the line-count text colors
  (hex colors, default yellow `#ecc94b` and red `#fc8181`); an empty string
  disables that count color. Missing counts are never presented as zero.

Supported style properties and values:

| Property | Values |
|---|---|
| `fill`, `stroke` | `#RGB`, `#RRGGBB`, `transparent`, or `none` |
| `color` | `#RGB` or `#RRGGBB` |
| `stroke-width` | Nonnegative number, optionally followed by `px` |
| `stroke-dasharray` | Space-separated nonnegative numbers, or `none` |
| `font-size` | Nonnegative number followed by `px`, `em`, `rem`, or `%` |
| `font-weight` | `normal`, `bold`, or `100` through `900` in steps of 100 |
| `font-style` | `normal` or `italic` |
| `font-family` | One family name containing letters, digits, spaces or hyphens; starts with a letter |

Unknown keys, invalid values, nulls, and unsupported versions fail before
indexing. The theme accepts presentation values rather than arbitrary CSS or
Mermaid directives. Fonts must be available on the reader's system; the theme
does not load external fonts. Set text and fill colors together for contrast.

`--no-theme` emits no theme frontmatter, `classDef`, `class`, or `linkStyle` statements. It restores
textual change/context labels and line counts, adds textual status labels to
changed connectors, and preserves change arrow shapes. It does not change scene
selection. `--plain` can be combined with `--no-theme`; plain Mermaid omits view
subgraphs, while its canvas behavior remains unchanged.

Both flags require `--mermaid` or `--markdown`; `--theme` and `--no-theme` are
mutually exclusive. Themes apply to every auto panel, and styling counts toward
`--max-bytes`. Auto file summaries keep their textual status labels. Themes do
not change JSON scenes, URLs, or the web canvas. The PR bot passes through the
styled Markdown without needing a new output format.

Source builds can edit `internal/mermaid/theme.json` and rebuild to change the
default. Go consumers can use `mermaid.DefaultTheme()` or
`mermaid.ParseTheme(jsonBytes)` with `SceneExportOptions.Theme`; existing callers
that omit `Theme` retain their legacy presentation.

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
- **`--mermaid` / `--markdown`:** render a bounded projection of the frozen scene,
  with nested views, themed change markers and directed edges. Authored context
  must be within the selected number of declared connector hops; containment
  and navigation do not pull unrelated authored nodes into Mermaid. Changed
  placements, changed-connector endpoints, and changed-file representatives
  remain protected. Plain output removes hierarchy wrappers. The UI applies
  the same policy, including offline shared comparisons. Canvas scenes and
  their data retain the canonical hierarchy and membership.
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

Canvas membership, grounding, coverage and Plain/radius selection decisions
belong to canonical assembly and selection. Mermaid applies its own bounded
presentation to a copy; exporters perform no independent workspace matching
and never mutate canvas data. Mermaid placement counts describe its projection,
while the changed-file ledger remains complete.

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

## Automatic PR presentation

`--auto` is opt-in. It uses the frozen comparison and dependency evidence; it
does not infer intent from commit messages or claim exhaustive runtime impact.
It applies only to `--mermaid` and `--markdown`; JSON and canvas URL exports
retain their existing selection and limits.

Auto starts at two dependency hops and retries at one, then zero, whenever
readability or explicit node, element, connector, or byte budgets are exceeded.
An explicit `--blast-radius N` sets the starting ceiling; `--blast-radius 0`
starts with direct changes. After radius zero, auto makes one final attempt
using flat Plain output at radius zero before reporting failure. If `--plain`
is already set, every radius uses Plain. The first complete selection within
budget wins.
Each attempt assembles the scene at that exact radius from the same pinned
snapshots, reusing the comparison cache where available. This prevents authored
views discovered at a larger radius from leaking into smaller attempts. Git
refs are not resolved again, and indexing/preparation is not repeated.

Auto removes unchanged relationships between dependency neighbours at the same
distance and isolated unchanged context. To keep fan-out diagrams readable, it
also trims optional dependency leaves toward six neighbours for one/two-file
changes, or twelve otherwise. Nearer and reciprocally connected leaves take
priority, with stable name/key ordering for ties. It preserves bridges, changed
connector endpoints, navigation, repeated placements, and changed-file
representatives even when those prevent meeting the neighbour target.
Changed relationships retain their original direction and styling.

For `--auto --markdown`:

- Scenes with more than twelve placements and at least two connected groups of
  three or more placements can become two diagrams. Whole disconnected groups
  are balanced between them; no relationship or changed-file representation is
  cut across diagrams. Connectivity includes cross-view relationships and
  navigation. The groups are disconnected **in the selected scope**, which does
  not establish independence in the full application.
- In larger scenes, isolated raw changed files move to a visible list below the
  diagrams, with full paths, change kinds, and available line counts. Every
  changed file remains represented. Authored representations stay in diagrams.
- If the selected scene has no relationships, output is a file summary instead
  of a collection of disconnected boxes. This describes available evidence,
  not the absence of dependencies. Unknown line counts stay unknown.
- A note discloses trimmed unchanged context, and subgraphs use an explicit
  left-to-right layout to avoid horizontal fan-out squeezing labels.

Raw Mermaid stays a single diagram, keeping isolated changed-file placements.
JSON and URL exports do not use auto reduction or its Plain fallback. `--view`
still restricts the authored view selection.

Auto limits each diagram to 24 placements and 40 connectors. It declines
comparisons with more than 50 changed paths or 2,000 known added-plus-removed
lines. Renames use the existing removal/addition ledger, so a broad move can
exceed these limits even if Git's rename-aware summary is small. These are
conservative review limits, not accuracy guarantees. Omit `--auto` to use manual
controls for a larger change.

Explicit `--max-nodes`, `--max-elements`, and `--max-connectors` apply to the
**whole selected scene**, including files listed below diagrams. Unlike manual
mode, auto checks them after simplification and can fall back to a smaller
radius. `--max-bytes` includes all Markdown text and fences; debug provenance
retains its existing byte-budget exemption. Zero disables the corresponding
explicit limit, not auto's readability limits. If no complete selection fits,
stdout is empty, stderr explains why, and the report has `status: "skipped"`;
the command exits successfully so a bot can use its own fallback. URL encoding
failures retain the existing nonzero exit behavior.

The optional report adds an `auto` object with `mode` (`diagram`, `split`,
`summary`, or `skipped`), `diagrams`, `listedFiles`, `prunedContext`,
`prunedConnectors`, `attemptedRadii` (in trial order), `plainFallback` (when
attempted), and `reason`. Existing status/count/statistics fields remain
available. Counts cover the entire selection; each Markdown panel's metadata
describes that panel's files. The canonical report retains the complete file
ledger even when rendering is skipped. `--debug-provenance` explains reductions.

See [the evaluation corpus](git-compare-auto-evaluation.md) for pinned public
commits, observations, reproduction commands, and limitations.

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
