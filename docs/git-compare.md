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
| `--data-dir PATH` | global | Override the data directory |

## Output

- **Default:** protojson encoding of the **impact scene** — the same payload the
  web canvas loads from `GetImpactScene`. It is portable: the emitted document
  is all a viewer needs, with no repository, index, or snapshot on the other
  end. Each placement carries a change overlay with the file's line counts and
  structured symbol changes (name, kind, anchor, body fingerprint), and the
  scene names what it compared (`schemaVersion`, `repositoryId`,
  `comparisonKey`, `fromGitRevision`, `toGitRevision`, `maxRadius`).
- **`--mermaid` / `--markdown`:** the Mermaid change diagram. It is drawn from
  the comparison's file-level dependency graph rather than the scene, because a
  scene carries workspace connectors, not the dependency weights between changed
  files.

Progress is written to stderr only with `--verbose`; budget warnings always go
to stderr. If the payload exceeds a budget, a warning tells you to raise
`--radius` or `--max-nodes`.

## Tips

- Pass `--depth` alone to widen both the computed neighbourhood and the output.
- Pass `--radius` to narrow the display only.
- `--depth 0` gives a fast, direct-changes-only view.
