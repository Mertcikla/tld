---
name: create-diagram
description: Create, update, or maintain tld architecture diagrams from a local codebase or system description. Use for architecture maps, component relationships, drill-down views, and requests to revise an existing tld diagram. Do not use for prose-only explanations or unrelated application changes.
allowed-tools: Read, Glob, Grep, Write, Bash(tld *)
---

# Create and Maintain Diagrams

Build a navigable map that answers the user's architectural questions. Preserve existing refs, useful layout, and unrelated diagram content when making updates.

## Model and command discovery

- **Elements** are reusable entities. Their workspace refs remain stable when their display names change. Custom refs require `tld init`; database-only refs are derived from names. Numeric IDs identify persisted resources.
- **Views** are canvases owned by elements. `root` is the top-level view. Adding a child with `--parent` or using `tld view place` creates the owner's view automatically.
- **Placements** put an element in a view. An element can appear in several views without being duplicated. Unplacing it preserves the element and its other placements.
- **Connectors** belong to one view. Use `--view` explicitly when authoring or updating multiple views. The same endpoints can have different labeled relationships.
- **Kind** is optional role metadata. Technology is metadata too; use `tld tech suggest` when a catalog name is uncertain.

Use `tld <command> --help` for accepted fields and examples, and `tld --help-all` for advanced commands. Prefer `--format json` for inspection and mutation results. Successful mutations return selectors for subsequent commands; connector keys change when their view, endpoints, or label change, so retain the returned key or ID.

## Establish scope and inspect

1. Reuse the current workspace, or select one with `--workspace <dir>`. For a new diagram, run `tld init` in the project root. Check the configured target with `tld config get apply.target`; `auto` selects cloud when credentials and an organization are configured, otherwise local. Use `--target local` on mutations when the requested destination is local.
2. For existing diagrams, inspect `tld list views --tree`, relevant elements/connectors, and `tld inspect <ref> --type element --format json` before editing. Use `tld pull` when the task requires refreshing from the target; resolve reported conflicts before proceeding.
3. Use the user's requested scope. Ask about detail or destination only when it is missing and materially affects the result. An overview should explain major runtime boundaries and dependencies; drill down where the requested questions require it. View counts and hierarchy depth are not completion targets.
4. Read the relevant source and write a compact inventory of responsibilities, callers, dependencies, and evidence. For updates, identify the affected views and relationships before making changes.

## Create or update in batches

For substantial creation or related edits, prefer `tld import`: it validates and applies a batch atomically on the target. Reapplying the same refs and connector keys updates existing resources. It does not delete omitted resources; perform intended deletions explicitly.

An import document is a ref-keyed `elements` map and a `connectors` list. For example, after verifying the actual relationship:

```yaml
elements:
  api:
    name: API
    kind: service
    placements:
      - parent: root
  db:
    name: Database
    kind: database
    placements:
      - parent: root
connectors:
  - view: root
    source: api
    target: db
    label: reads
```

```bash
tld import diagram.yaml --dry-run --format json
tld import diagram.yaml --format json
```

Use actual source links in `file_path` when available (`path`, `path#symbol`, or `folder/`), or `--link` with `tld add`. Use `tld link` for advanced anchors and external resources. See `tld import --help` for schema references.

For small edits, use the existing commands:

```bash
tld add "API" --ref api --kind service --parent platform
tld connect --from api --to db --view platform --label "reads and writes"
tld update element api description "Handles public requests"
tld update connector 'platform/api~db/reads' label "reads and writes"
tld view place root api
tld view place platform api --from root
tld remove connector 'platform/api~db/reads and writes'
tld view unplace platform api
```

These are individual recipes, not a sequence to run blindly. Parents/endpoints must exist. Remove or update connectors in a view before unplacing or moving their endpoints. `remove element` deletes the reusable resource, so use `view unplace` when only one view should change.

`add` preserves omitted fields and unions tags; supplied nonempty fields update metadata. Use `update element` to clear a field, change its kind, or rename its display name. Use its `ref` field only to deliberately rename a workspace alias. For placement-only changes, prefer `view place` so metadata need not be repeated.

Mutations apply immediately and refresh workspace YAML. Prefer CLI mutations to editing cache files; deliberate YAML edits require `tld sync`. Read error hints before retrying. If an error says the target changed but the cache failed, refresh with `tld pull` before continuing. Stop repeated retries when the same issue persists and report the unresolved cause.

## Evidence and readable views

Connectors must represent relationships supported by source or by the user's system description. Record useful source evidence in element links and connector descriptions. Distinguish inferred or proposed relationships from confirmed behavior; do not add edges merely to eliminate isolation warnings.

Reuse existing elements across views. Add children and their labeled relationships together. When summarizing internal interactions at a higher level, connect the actual containing elements and describe the aggregate interaction; do not substitute parent refs for distinct children within a detailed view.

Keep views focused, usually around 8–12 elements. Split by responsibility when that improves understanding. An intentional isolated element can remain if its role is explained. Avoid mirroring directories or expanding every class/function unless the user's questions require that detail.

## Verify and hand off

Run `tld validate --format json` and address structural/source-link failures. Review architectural warnings with `tld validate --verbose` or a specific rule code. Use `--strict` when warnings should fail the check; explain intentional exceptions rather than inventing relationships or suppressing warnings to obtain a pass.

Inspect the changed resources and render each affected view with `tld render <view>`. If browser or visual inspection is available, inspect layout and labels in the UI too. Validation and resource counts do not prove completeness: compare the result against the requested questions and evidenced execution/data paths.

For a local diagram, use `tld serve --open` to open the UI. For an existing remote diagram, hand off its known destination. Summarize what changed, what was verified, and any evidence or coverage gaps. Do not claim visual verification if only CLI checks were possible.
