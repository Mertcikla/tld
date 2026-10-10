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
| `-v`, `--verbose` | off | Report base/head indexing progress on stderr |
| `--depth N` | `3` | Dependency hops of unchanged context (`0` = direct changes only) |
| `--radius N` | `--depth` | Blast radius to display; narrows the view without shrinking the computed neighbourhood |
| `--max-nodes N` | `400` | Node budget; blast radius is narrowed when exceeded (`0` disables) |
| `--max-bytes N` | `2097152` | Output byte budget; blast radius is narrowed when exceeded (`0` disables) |
| `--max-elements N` | `0` | Skip output if the requested diagram has more than N elements (`0` disables) |
| `--max-connectors N` | `0` | Skip output if the requested diagram has more than N connectors (`0` disables) |
| `--view REF` | unset | Restrict the grounded bundle to one authored view (id or name substring) |
| `--scope SCOPE` | `grounded` | Diagram scope for `--mermaid`: grounded, authored, or mapped (mirrors the canvas menu) |
| `--all-edges` | off | Disable fan-out roll-up in the grounded summary |
| `--report-json PATH` | unset | Write status, mode, requested counts, change stats, resolved BASE/HEAD SHAs, index warnings, grounded overlay, and skip reason to JSON |
| `--prepare-command COMMAND` | unset | Run a Bash command in each uncached revision's temporary checkout before indexing |
| `--data-dir PATH` | global | Override the data directory |

## Output

- **Default:** protojson encoding of the **grounded bundle** — the portable impact
  scene plus the authored overlay (`mode: "grounded"`, `grounded: {affected,
  context, ungrounded, views, elements, fanout, uncovered}`). The user-authored diagram
  stays canonical while each linked element is verified against the pinned
  snapshots; the generated map contributes collapsed context only. High fan-out
  nodes roll up by target prefix (7 groups + "+N more"). When no authored view
  intersects the change, the bundle notes the fallback and carries raw impact
  context. The legacy bare scene remains behind the hidden `--raw-impact` flag.
- **`--mermaid` / `--markdown`:** the change scene rendered as Mermaid — the same
  payload the canvas draws, node for node: a `%% tld-scene` header, one subgraph
  per retained view, change badges (`modified +8 −0`, `(context)`) on nodes,
  provenance glyphs (`◇` graph-augmented, `▦` map-generated) on non-authored
  nodes, and connector arrows with `linkStyle` colors for added/removed/modified
  edges (`--scope` selects grounded/authored/mapped, like the canvas menu; the
  CLI-only `%% grounded` stats lines precede the diagram). The legacy
  file-level dependency graph remains behind the hidden `--raw-impact` flag
  (there, new dependencies render as thick `==>` arrows, removed ones as
  `A--x|"-N"|B`, modified ones keep `-->` with a `~N` label).

Progress is written to stderr only with `--verbose`; budget warnings always go
to stderr. If the payload exceeds a budget, a warning tells you to raise
`--radius` or `--max-nodes`.

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

- Pass `--depth` alone to widen both the computed neighbourhood and the output.
- Pass `--radius` to narrow the display only.
- `--depth 0` gives a fast, direct-changes-only view.
