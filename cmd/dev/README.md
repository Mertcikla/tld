# `tld dev` Golden Fixture Tools

`tld dev` contains developer-only commands for building and checking the watch golden fixture corpus. These commands are intentionally separate from `tld watch` because they are for maintaining tld itself, not for normal user workflows.

## Commands

### `tld dev fixture <repo-path>`

Runs the watch pipeline against a small repository, prints a preview, and can write a fixture directory containing:

- `repo/`: a copy of the source repository, excluding heavy/generated directories such as `.git`, `node_modules`, `.next`, `dist`, and `build`
- `fixture.json`: review metadata, taxonomy, status, notes, and snapshot paths
- `golden/snapshot.json`: stable golden output for facts, materialized elements, connectors, views, and visibility decisions

Typical usage:

```sh
tld dev fixture ./scratch/go-nethttp \
  --corpus-dir ../tld-fixtures \
  --name go-nethttp-basic-route \
  --fixture-language go \
  --fixture-domain http \
  --fixture-framework nethttp \
  --fixture-type basic_route
```

Non-interactive approval:

```sh
tld dev fixture ./scratch/go-nethttp \
  --corpus-dir ../tld-fixtures \
  --name go-nethttp-basic-route \
  --fixture-language go \
  --fixture-domain http \
  --fixture-framework nethttp \
  --fixture-type basic_route \
  --approve \
  --note "covers http.HandleFunc route extraction"
```

Preview without writing files:

```sh
tld dev fixture ./scratch/go-nethttp --json
```

Useful flags:

- `--corpus-dir`: root of the fixture corpus repository
- `--fixture-language`: taxonomy language, for example `go` or `typescript`
- `--fixture-domain`: capability area, for example `http`, `frontend`, `orm`, or `dependency`
- `--fixture-framework`: framework/library, for example `nethttp`, `chi`, `gin`, `express`, `nextjs`, `react_router`, or `prisma`
- `--fixture-type`: behavior shape, for example `basic_route`, `dependency_manifest`, or `basic_query`
- `--approve`, `--reject`: skip the interactive prompt
- `--note`: attach review notes; repeatable
- `--language`: limit watch scanning languages
- threshold flags such as `--max-elements-per-view` when a fixture intentionally exercises representation limits

When all taxonomy flags are provided, output is written to:

```text
<corpus-dir>/<language>/<domain>/<framework>/<type>/
```

If taxonomy flags are omitted, output falls back to:

```text
<corpus-dir>/<name>/
```

## `tld dev conformance`

Discovers `fixture.json` files recursively, runs each fixture `repo/` through the watch pipeline with embeddings disabled, compares the generated snapshot to `golden/snapshot.json`, and opens the interactive fixture reviewer when stdout is a terminal.

```sh
tld dev conformance --fixtures ../tld-fixtures --mode warn
```

For CI or report-only usage, pass `--report` or pipe stdout:

```sh
tld dev conformance --fixtures ../tld-fixtures --mode warn --report
```

Modes:

- `warn`: always exits zero after printing the report unless the runner itself cannot execute
- `strict`: exits non-zero when any fixture drifts or errors
- `threshold`: currently behaves like strict; keep using `warn` until category thresholds are implemented

The report is grouped by:

- language
- domain
- framework/library
- type

Each group shows pass/drift/error counts plus fact and element deltas. Fixture details include missing/extra/changed facts, elements, decisions, views, and connectors.

The reviewer autosaves these manifest fields as you move:

- `review_status`: `pending`, `reviewed`, or `skipped`
- `accuracy`: `accurate`, `partially_accurate`, `inaccurate`, or `unsure`
- `review_comments`
- `reviewed_at`

Use `j`/`k` to move, `r` to mark reviewed, `s` to skip, `1`-`4` to set accuracy, `/` to filter, `tab` to edit comments, and `o` to open the selected fixture’s golden snapshot in the local read-only frontend reviewer.

CI uses the warn-only flow by cloning the external corpus and running:

```sh
go run ./cmd/tld dev conformance --fixtures ./tld-fixtures --mode warn
```

## Fixture Repository Layout

Use small synthetic repositories first:

```text
tld-fixtures/
  go/
    http/
      nethttp/
        basic_route/
          fixture.json
          golden/snapshot.json
          repo/main.go
      chi/
        basic_route/
      gin/
        basic_route/
    dependency/
      gomod/
        dependency_manifest/
  typescript/
    http/
      express/
        basic_route/
    frontend/
      nextjs/
        app_route/
      react_router/
        basic_route/
    orm/
      prisma/
        basic_query/
    dependency/
      npm/
        dependency_manifest/
```

The `repo/` directory does not need to contain `.git`. The conformance runner copies it to a temporary directory and initializes git before scanning.

## What Makes a Good Fixture

A good fixture is small, readable, and targeted.

Prefer:

- one capability per fixture
- one or two source files unless the behavior requires cross-file references
- minimal dependencies declared in `go.mod` or `package.json`
- literal route paths and query calls that humans can inspect quickly
- names that describe the behavior, such as `basic_route`, `nested_route`, `dependency_manifest`, or `basic_query`
- notes that explain why the fixture exists and what regression it should catch

Avoid:

- large real repositories as first-pass goldens
- generated code, vendored code, compiled output, lockfile-only behavior, or fixture data copied from production apps
- broad fixtures that test Go parsing, route extraction, dependency inventory, filtering, and materialization all at once
- relying on local machine paths, timestamps, network access, installed packages, or external services
- fixtures whose expected behavior is unclear without reading many files

## Review Checklist

Before approving a fixture, inspect the preview and check:

- the expected facts are present with the right type, enricher, stable key, file path, name, and tags
- there are no surprising extra facts from accidental imports or dependencies
- visible/hidden decisions make sense for the fixture’s purpose
- materialized elements and connectors are stable and relevant
- the taxonomy path matches the behavior under test
- the note explains the intended regression signal

If the preview is noisy, simplify the repo before approving. A noisy golden is expensive because every future drift report becomes harder to trust.

## Common Pitfalls

**Testing too much at once**

Split fixtures by capability. For example, keep `go/http/nethttp/basic_route` separate from `go/dependency/gomod/dependency_manifest`.

**Using framework names inconsistently**

Pick one normalized taxonomy value and keep it stable. Use `nethttp`, `react_router`, and `nextjs` rather than alternating between aliases.

**Forgetting activation signals**

Some enrichers activate from imports or dependency manifests. If a route/query fact is missing, make sure the fixture contains the import or dependency that activates the enricher.

**Adding realistic dependencies casually**

Every dependency can produce dependency facts and alter visibility decisions. Add only the dependencies required for the behavior under test.

**Approving accidental output**

The builder records what the current pipeline emits. Approval means “this is the intended compass,” not just “the command ran.”

**Assuming conformance is red/green**

The default CI mode is `warn`. The goal is a categorized compass that shows quality and drift by language/framework/type while the corpus is still growing.

## Expanding the Corpus

Start with one fixture per existing enricher:

- Go `net/http` route
- Go `chi` route
- Go `gin` route
- Go `go.mod` dependency declaration
- TypeScript Express route
- Next.js route file
- React Router route
- Prisma query
- `package.json` dependency declaration

After the small synthetic set is stable, add variants for nesting, multiple files, import aliases, grouped routes, and negative cases. Real-world sampled fixtures should live under an explicit taxonomy branch such as `go/realworld/...` and should only be added after the synthetic suite is easy to interpret.
