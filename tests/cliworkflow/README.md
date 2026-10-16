# CLI workspace lifecycle tests

This suite runs behind the `cliworkflow` build tag, locally and in the CI
**CLI Workflow Tests** job for pull requests to `main`, manual runs, and nightly
runs. Ordinary `go test ./...` excludes the suite. Rapid is a test-only
development dependency; it is not imported by production code. Go has no separate
development-dependency manifest, so its pinned version remains in `go.mod`.

The suite builds `./cmd/tld` once and starts a fresh process for each command.
Each history owns its workspace, configuration, and database. Child processes
discard ambient `TLD_*` settings and use explicit destinations. No personal
workspace, staging server, or staging database is used.

```sh
go test -tags cliworkflow ./tests/cliworkflow -count=1
```

When explicitly enabled, SQLite and remote RPC histories run by default.
PostgreSQL histories and scenarios require a test URL whose user can create
databases:

```sh
TLD_TEST_POSTGRES_URL='postgres://tld:tld@localhost:55432/tld?sslmode=disable' go test -tags cliworkflow ./tests/cliworkflow -count=1
```

Each PostgreSQL attempt creates and drops a uniquely named database. It never
resets the supplied database's schema. CI supplies an isolated PostgreSQL service
and runs all three backends. These tests run separately from
`scripts/test-postgres-e2e.sh`. Remote mode uses the production
WorkspaceService/ImportService over loopback HTTP, a real SQLite store, a dummy
bearer credential, and the production workspace context helper.

## Coverage

| Area | Exercised behavior |
| --- | --- |
| Elements | CRUD, all 15 advertised update fields, clearing fields, tag append/remove, aliases, name/ID selectors, implicit owned views |
| Connectors | CRUD, all 13 update fields, endpoint/view/key changes, direction/style/handles/tags, off-view relationships |
| Views | Create/delete/rename/set-level, place/unplace, move with `--from`, sharing through add |
| Import | YAML files and stdin, JSON stdin, multi-resource graph creation, partial patches, omitted-field preservation, retries, dry runs |
| Read commands | Element/connector/view inventory with content and ID assertions, inspect by type/ID, render to stdout/file, validate and strict refusal |
| Input/output | Compact/pretty JSON, text reads and mutation, ref/name/ID selectors, ambiguous names, invalid refs/fields/endpoints, malformed import and workspace YAML |
| Synchronization | Manual YAML edits, external target edits, sync/pull/dry-run/force, reference renames, stale caches, repeated convergence |
| Modes | Workspace-backed and database-only clients; SQLite, PostgreSQL, remote RPC |
| Recovery | Rejected RPC writes, committed sync followed by failed cache export, read-only cache write failure, transactional rollback, canceled imports after real uncommitted inserts |
| Concurrency | Independent CLI processes editing the same target without a shared YAML cache, followed by pull and integrity checks |

Visibility deltas are workspace metadata preserved by pull, including `--force`,
until sync applies them. Language is
checked in the target; it is intentionally absent from workspace YAML. Technology
icons can be derived by the API when the database has no explicit logo URL.

The reference model is separate from production import/export conversion code.
It tracks expected YAML, committed data, current IDs, and historical sync IDs.
After every action, direct YAML decoding and SQL reads compare modeled contents,
references, duplicates, resource counts, and identities. SQLite also runs
`integrity_check` and `foreign_key_check`. The sync hash check uses the production
hash helper as a separate consistency assertion. Reads, previews, and rejected
commands preserve workspace bytes and domain rows (including timestamps).

Fault injection lives entirely in the test server, filesystem permissions, and
database triggers. The interrupted-import barrier is inside the real database
transaction after its rows are written; the test kills the CLI, waits for server
cancellation/rollback, checks no rows leaked, and retries successfully.

## Generated histories and replay

Each enabled backend defaults to 10 Rapid histories with an average of 20
actions, plus setup and final reconciliation. `-short` reduces that workload.
Increase either dimension with CLI flags or `RAPID_CHECKS`/`RAPID_STEPS`:

```sh
go test -tags cliworkflow ./tests/cliworkflow -run '^TestWorkspaceStateMachine$' -count=1 -timeout=30m -rapid.checks=100 -rapid.steps=100
```

Omit the final `$` in the test selector to include remote and PostgreSQL history
tests. PostgreSQL still requires its test URL. Every history ends with sync and
two pulls. Generated actions include imports, field updates, view lifecycle and
placement changes, inventories, rendering, manual/external edits, and sync/pull.
Preconditions skip actions that cannot be valid in the current model.

Rapid shrinks failures and prints `-rapid.failfile=<path>` and/or a seed. Use
the same source revision and `rapid.steps` setting to replay a seed. Local
`testdata/rapid` failures replay automatically, but that directory is ignored by
Git: promote useful failures into authored regression scenarios.

```sh
TLD_WORKFLOW_ARTIFACT_DIR=/tmp/tld-workflow-failures go test -tags cliworkflow ./tests/cliworkflow -count=1 -v
```

Retained failures include YAML, SQLite data, and command traces. Shrinking can
retain several attempts. PostgreSQL test databases are always dropped; SQL/model
differences and CLI output remain in the test log.

CI explicitly runs 10 histories with an average of 20 actions per backend and a
20-minute test timeout. The `cli-workflow-results` artifact contains JUnit
results, any Rapid replay files, and retained failure workspaces and traces.

This is lifecycle coverage, not exhaustive statement/flag coverage. Interactive
login, server lifecycle, indexing/Git analysis, MCP, shell completion, and every
possible concurrent shared-YAML schedule remain separate from this suite. The
read-only-cache permission fault skips when running as root.
