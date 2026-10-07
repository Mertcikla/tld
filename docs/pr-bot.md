# GitHub PR change diagrams

The `.github/actions/pr-diagram/action.yml` is a reusable composite action with two modes:

| Mode | Purpose |
| --- | --- |
| `generate` | Compare a PR's merge base to HEAD, restore/save snapshots, upload a result artifact |
| `publish` | Verify the originating run and update one bot-owned PR comment |

All bot failures are advisory. Oversized diagrams, empty comparisons, setup
errors, and partial indexing replace any previous diagram with a short status.
The comment leads with change-risk statistics derived from the comparison and
the PR checkout's git history: the change's size rank among recent commits, its
lines, files, directories, and symbol churn, and whether its files have prior
bug fixes. The diagram and run link follow. Existing test and lint checks are
independent of this bot.

## Change-risk stats

The generator computes the stats once and stores them in the artifact, so the
publisher stays read-only and never runs PR code or git. The compare report
supplies the change's shape (files, lines, directories, symbol deltas); the
checkout supplies two git walks anchored at the PR's merge base:

- **Size rank** — total churn ranked against up to 200 recent commits, banded
  `Below typical` / `Typical` / `Elevated`. With fewer than eight sampled
  commits (a shallow clone) it falls back to fixed bands and says so.
- **Prior bug fixes** — recency-weighted counts over the files the change
  touches. A fix from a year before the change counts a half. A fix-history
  percentile is deliberately deferred; only counts ship.
- **Symbols** — added / modified / removed non-file facts from the index.

The stats are adapted from Repowise's `repowise risk` change-risk layer; the
0-10 diff-shape model and per-file fix table are intentionally out of scope.
History walks are bounded (200 baseline commits, 2000 fix-history commits) and
never fail the bot: a broken or shallow history degrades to "unavailable".

## Installing in another repository

Use an Ubuntu runner (x86_64 or ARM64). Choose a commit or release containing this action and
replace `PINNED_REF` in both examples with that same immutable reference.
Use a reference containing the subdirectory action. Older releases, including
`v2.4.0`, keep the root action and use `Mertcikla/tld@PINNED_REF` instead;
this repository's workflows remain pinned to that release until the next upgrade.
To upgrade, change `tldVersion` in `scripts/pr-bot/main.mjs`;
the download URL and binary/database cache keys update together.
It sets up Go 1.26.2 and Node 24 for indexing and installs
`scip-go@v0.2.7` and `@sourcegraph/scip-typescript@0.4.0`.

The producer and publisher must have different permissions. The publisher
workflow must exist on the default branch before it can receive `workflow_run`
events. Do not execute PR code in the publisher.

Create `.github/workflows/pr-diagram.yml`:

```yaml
name: PR diagram
on:
  pull_request:
    types: [opened, synchronize, reopened, edited]
permissions:
  contents: read
concurrency:
  group: pr-diagram-${{ github.event.pull_request.number }}
  cancel-in-progress: true
jobs:
  diagram:
    runs-on: ubuntu-latest
    timeout-minutes: 45
    continue-on-error: true
    steps:
      - uses: actions/checkout@v6
        with:
          ref: ${{ github.event.pull_request.head.sha }}
          fetch-depth: 0
          persist-credentials: false
      - uses: Mertcikla/tld/.github/actions/pr-diagram@PINNED_REF
        env:
          TLD_PR_MAX_ELEMENTS: '80'
          TLD_PR_MAX_CONNECTORS: '160'
        with:
          mode: generate
          indexers: go typescript
          # Customize for your repo; this runs separately inside each revision.
          prepare-command: npm --prefix frontend ci --ignore-scripts
```

Create `.github/workflows/pr-diagram-publish.yml`:

```yaml
name: Publish PR diagram
on:
  workflow_run:
    workflows: [PR diagram]
    types: [completed]
permissions:
  contents: read
  actions: read
  pull-requests: write
concurrency:
  group: pr-diagram-publish-${{ github.event.workflow_run.head_repository.id }}-${{ github.event.workflow_run.head_branch }}
  cancel-in-progress: false
jobs:
  comment:
    if: github.event.workflow_run.event == 'pull_request'
    runs-on: ubuntu-latest
    timeout-minutes: 5
    continue-on-error: true
    steps:
      - uses: Mertcikla/tld/.github/actions/pr-diagram@PINNED_REF
        with:
          mode: publish
          source-workflow: pr-diagram.yml
```

An empty preparation command is appropriate for projects that need no setup.
For this repo, the generation workflow installs frontend dependencies and builds the
embedded frontend inside each uncached revision. Its producer checks out trusted
default-branch bot code separately from the PR checkout so PR source changes
do not change the bot code or cache namespace on every synchronize event.

## Configuration

| Setting | Default | Meaning |
| --- | --- | --- |
| `TLD_PR_MAX_ELEMENTS` | `80` | Maximum diagram nodes, including context if enabled |
| `TLD_PR_MAX_CONNECTORS` | `160` | Maximum diagram dependency edges |
| `repository-path` | current workspace | Checkout to index |
| `prepare-command` | empty | Bash setup inside each uncached commit worktree |
| `indexers` | `go typescript` | Enabled indexers; currently Go and TypeScript are supported |
| `context-depth` | `0` | Dependency hops of unchanged context, from 0 to 100 |
| `artifact-name` | `tld-pr-diagram` | Must match between producer and publisher |
| `run-id` | triggering run | Must match the `workflow_run` event |
| `source-workflow` | `pr-diagram.yml` | Expected generator workflow filename |
| `github-token` | workflow token | Used only by publication |

Both limits must be positive integers. Zero, negatives, fractions, and malformed
values produce an advisory configuration error. Limits are checked before
Mermaid rendering and automatic budget shrinking. They count the requested PR
diagram, rather than every symbol or relation in the full repository indexes.
Equality is permitted; exceeding either limit skips the whole diagram.

This repository's producer reads the limits from GitHub repository variables
with the same names, falling back to 80 and 160. Other consumers set them in the
action step's `env` block.

Artifact size is capped at 256 KiB and comment size at 60 KiB. Oversized output
becomes a short skip status. Indexer warnings omit the diagram rather than
presenting incomplete dependency results. Failures and reasons are logged in
the job summary; detailed preparation/indexer output stays in the run logs.

## Cache behavior and fork PRs

The action caches the release binary and indexers separately from the isolated
tld database. Snapshot keys include repository identity, action source digest,
the tld release, runtime and indexer versions, enabled indexers, and preparation configuration.
Binary cache keys include the tld release, runtime/indexer versions, and runner
OS/architecture; action source changes do not redownload the same release.
Restore reuses the PR's previous database, preferring the same BASE revision.
The database is saved after the compare process closes, including SQLite
sidecars when present. Snapshots are retained even when a diagram is skipped.
Temporary worktrees, config files, checkouts, and credentials are excluded.

The first run indexes BASE and HEAD. Subsequent runs reuse compatible snapshots
from that PR's cache. An evicted entry, a different action version, setup changes,
or indexer changes may require indexing BASE again. First-time fork workflows may require the
repository's normal GitHub Actions approval before generation can run.

Publication verifies the run's repository, event, workflow path, and PR
association through GitHub APIs. Missing fork associations fall back to verified
commit/PR associations. It rejects mismatched artifact identities, checks the
current HEAD and target BASE, and ignores closed/stale PRs. Artifacts are data
only. A stable marker and `github-actions[bot]` ownership identify the comment;
run metadata prevents older publications from replacing a newer one.

After merging, verify one same-repository PR and one fork PR.
Confirm a synchronize reuses compatible snapshots and updates the existing comment, and
temporarily lower a limit to check that the skip status replaces the diagram
while CI remains green.

## Local validation

```bash
node --test scripts/pr-bot/*.test.mjs
actionlint .github/workflows/pr-diagram*.yml
make lint-be
rtk go test ./...
```
