---
name: create-diagram
description: Create, update, or maintain tld architecture diagrams from a local codebase or system description. Use for architecture maps, component relationships, drill-down views, and requests to revise an existing tld diagram. Do not use for prose-only explanations or unrelated application changes.
allowed-tools: Read, Glob, Grep, Write, Bash(tld *)
---

# Create and Maintain Diagrams

Create a navigable architecture map that answers the user's questions. Keep useful existing content and stable refs when revising it.

## Scope and evidence

Read the relevant source and inventory responsibilities, execution/data paths, callers, and dependencies. Connectors must have source evidence or support from the user's system description. Put source anchors in element links (`file_path: path#symbol` or a folder) and useful evidence in connector descriptions. Distinguish inferred relationships from confirmed behavior.

Choose overview and drill-down boundaries by responsibility. Keep each canvas readable, usually around 8–12 components. Avoid mirroring directories or expanding every class unless the requested questions need it. Do not add relationships just to eliminate warnings.

## Select the destination

Reuse the current workspace or run `tld init <project-dir>` to create `<project-dir>/.tld`. Use `tld context --format json` to confirm the workspace and destination. Pin `--workspace`, `--target`, and any `--data-dir` consistently on previews, writes, and verification; use `--target local` for a local diagram.

For existing diagrams, inspect `tld list views --tree`, relevant elements/connectors, and `tld inspect <ref> --format json` before changing them. Use `--source live` for current target state. Prefer CLI mutations over editing cached YAML; apply deliberate YAML edits with `tld sync`.

## Author by intent

Use command help for accepted fields and examples, `tld import --schema` for the exact offline document shape, and JSON output for returned refs/IDs.

- Create a canvas for an existing element: `tld view create runtime --name "Runtime"`. Create its owner with `tld add` first; adding children also creates the owner's canvas automatically.
- Add or update a component: `tld add "API" --ref api --parent runtime --kind service --link path#symbol`.
- Connect components: `tld connect --view runtime --from api --to db --label "reads and writes"`. Both endpoints become visible on that canvas automatically; existing placements elsewhere are retained.
- Show an unconnected component: `tld view place runtime api`. `--from <old-view>` moves its placement. Sharing may copy existing relationships; inspect live connectors afterward before adding or removing any.
- Change metadata or a relationship: `tld update element api description "Handles requests"` or `tld update connector <listed-key-or-id> label "queries"`.

For batches, choose the operation that matches the intended change:

| Intent | Operation |
| --- | --- |
| Add or patch resources across the workspace | `tld import diagram.yaml` |
| Remove one placement, keeping its element elsewhere | `tld view unplace runtime api` |
| Delete a reusable element everywhere | `tld remove element api` |

Imports preserve omitted resources and placements; they do not replace a canvas. Remove unwanted connectors explicitly and use `view unplace` for unwanted components. A canvas's owner appears in its containing view; use its children as endpoints inside its own canvas.

An import document is an elements map plus a connectors list. Element definitions need names; connector views are explicit, and missing endpoint placements are added automatically. Preview with `--dry-run`:

```yaml
elements:
  api:
    name: API
  db:
    name: Database
    kind: database
connectors:
  - view: runtime
    source: api
    target: db
    label: reads
```

Use actual existing refs and evidenced relationships. Retain returned keys/IDs when updating connectors; changing a label/view/endpoint changes the key. Use `update connector` to change identity or set tags. Use `update element` to clear fields or rename a display name/ref, and `view rename` to change a canvas name.

Read error hints before retrying. If a write reports `error.applied: true`, refresh the cache with `tld pull` against the same destination before continuing. Review `pull --dry-run` before replacing local edits, and stop repeated retries when the same issue persists.

## Verify and hand off

Run `tld validate --source live --format json` and address structural/source-link failures. Review architectural warnings, explain intentional exceptions, and use `--strict` when warnings should fail. Check that a populated diagram does not report `extra.empty`.

Render each affected canvas with `tld render <view> --source live --format json`; check emitted/omitted connector diagnostics. Compare the map against the requested questions and evidenced paths, since counts and validation do not prove completeness. Inspect layout in the UI when available and disclose any visual verification limits.

For local diagrams, open the UI with `tld serve --open`. Summarize the result, verification, and remaining coverage gaps.
