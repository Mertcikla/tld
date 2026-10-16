---
name: create-diagram
description: Create, update, or maintain tld architecture diagrams from a local codebase or system description. Use for architecture maps, component relationships, drill-down views, and requests to revise an existing tld diagram. Do not use for prose-only explanations or unrelated application changes.
allowed-tools: Read, Glob, Grep, Write, Bash(tld *)
---

# Create and Maintain Diagrams

Create a navigable architecture map that answers the user's questions. Keep useful existing content and stable refs when revising it.

## Scope and evidence

Read the relevant source and inventory responsibilities, execution/data paths, callers, and dependencies. Connectors must have source evidence or support from the user's system description. Put source anchors in element links (`file_path: path#symbol` or a folder) and useful evidence in connector descriptions. Distinguish inferred relationships from confirmed behavior.

Choose overview and drill-down boundaries by responsibility. Keep each view readable, usually around 8–12 components. Avoid mirroring directories or expanding every class unless the requested questions need it. Do not add relationships just to eliminate warnings.

## Select the destination

Reuse the current workspace or run `tld init <project-dir>` to create `<project-dir>/.tld`. Use `tld context --format json` to confirm the workspace and destination. Pin `--workspace`, `--target`, and any `--data-dir` consistently on previews, writes, and verification; use `--target local` for a local diagram.

For existing diagrams, inspect `tld list views --tree`, relevant elements/connectors, and `tld inspect <ref> --format json` before changing them. Use `--source live` for current target state. Prefer CLI mutations over editing cached YAML; apply deliberate YAML edits with `tld sync`.

## Author by intent

Use command help for accepted fields and examples, `tld import --schema` for the exact offline document shape, and JSON output for returned refs/IDs.

- Create a view for an existing element: `tld view create runtime --name "Runtime"`. Create its owner with `tld add` first; adding children also creates the owner's view automatically.
- Add or update a component: `tld add "API" --ref api --parent runtime --kind service --link path#symbol`.
- Connect components: `tld connect --view runtime --from api --to db --label "reads and writes"`. This creates the relationship without changing endpoint placements.
- Show an unconnected component: `tld view place runtime api`. `--from <old-view>` moves its placement. Sharing may copy existing relationships; inspect live connectors afterward before adding or removing any.
- Change metadata or a relationship: `tld update element api description "Handles requests"` or `tld update connector <listed-key-or-id> label "queries"`.

For batches, choose the operation that matches the intended change:

| Intent | Operation |
| --- | --- |
| Add or patch resources across the workspace | `tld import diagram.yaml` |
| Remove one placement, keeping its element elsewhere | `tld view unplace runtime api` |
| Delete a reusable element everywhere | `tld remove element api` |

Use off-view connectors to express dependencies across architectural boundaries without duplicating components in every view. Choose the relationship's owning `--view`, keeping external endpoints in their existing views. The UI resolves these through ancestor/context nodes and can aggregate them into `+N` badges; visible hierarchy, off-view filters, and connector budget affect display. `inspect` and `list connectors` report off-view endpoints and their placement views. Use `view place` only when the component should appear directly in that view.

Imports preserve omitted resources and placements; they do not replace a view. Remove unwanted connectors explicitly and use `view unplace` for unwanted components. A view's owner remains in its containing view and can be an off-view endpoint in its own view.

An import document is an elements map plus a connectors list. Element definitions need names; connector views are explicit, and endpoints retain their placements. Preview with `--dry-run`:

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

Use actual existing refs and evidenced relationships. Retain returned keys/IDs when updating connectors; changing a label/view/endpoint changes the key. Use `update connector` to change identity or set tags. Use `update element` to clear fields or rename a display name/ref, and `view rename` to change a view name.

Read error hints before retrying. If a write reports `error.applied: true`, refresh the cache with `tld pull` against the same destination before continuing. Review `pull --dry-run` before replacing local edits, and stop repeated retries when the same issue persists.

## Verify and hand off

Run `tld validate --source live --format json` and address structural/source-link failures. Review architectural warnings, explain intentional exceptions, and use `--strict` when warnings should fail. Check that a populated diagram does not report `extra.empty`.

Render each affected view with `tld render <view> --source live --format json`. Mermaid exports only directly placed endpoints; off-view omissions do not mean the relationship was lost or invalid. Check off-view details and unresolved endpoints separately, and inspect cross-view context in the UI. Compare the map against the requested questions and evidenced paths, since counts and validation do not prove completeness. Disclose any visual verification limits.

For local diagrams, open the UI with `tld serve --open`. Summarize the result, verification, and remaining coverage gaps.
