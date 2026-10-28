---
name: create-flow
description: Create and edit tlDiagram saved flows with the tld CLI or portable tld-flow YAML, including highlighted steps, alternate or parallel branches, linked flows, and complete diagram bundles. Use for narrated journeys over TLD diagrams, not generic workflow automation or application code changes.
---

# Create TLD Flows

Turn the requested scenario into an ordered story over diagram elements and connectors. Base interactions on the user's description or source evidence; distinguish assumptions. Reuse existing diagram resources when the user wants a flow over an existing view. State the selected scenario and material omissions in the flow description (for example, success path with unrecovered failures, excluding redirects). Preserve source anchors in step descriptions or a companion note. Check conditional behavior against the inspected revision: a failure may recover, and an early return must not appear to continue through later steps.

## Establish the target

Check `tld flow --help` and `tld flow import --schema` to confirm the installed CLI supports flows. An older binary can print general help without supporting the command. If necessary in a TLD source checkout, build a temporary CLI from `./cmd/tld` using the repository's build prerequisites; do not replace the installed binary implicitly.

Flows always use the configured **live target**. `--workspace`, `--yaml`, cached workspace files, and `tld sync` do not select or persist flows. Inspect `tld context --format json` and keep `--target` and any `--data-dir` consistent across discovery, dry runs, writes, and verification. Pin `--target local` for local work; honor an explicitly requested cloud destination.

Inspect live views, elements, and connectors with `tld list`/`tld inspect` and `--source live`, then `tld flow list --format json`. A flow belongs to one view. CLI `--view` accepts `root`, a numeric view ID, or a uniquely resolved live owner ref/name. Select existing flows by UUID or disambiguate names with `--view`. Save returned flow UUIDs, revisions, and step/path IDs.

## Choose an authoring route

- For incremental edits, use `flow create`, `flow step`, and `flow branch` commands below.
- For an entire story, nested branches, or a shareable file, read [portable-flow-yaml.md](references/portable-flow-yaml.md), write portable YAML, then use `flow import`.
- A YAML-only request needs a file, not an unsolicited live import. State if contextual validation could not run because the destination resources are unavailable.
- To create a fresh diagram and flow together, use a complete YAML bundle with `--new-view`. To add a flow to an existing diagram, use `--view`; that mode ignores embedded resource definitions.

## Incremental CLI authoring

These examples assume API and Database already exist in Root. Append the chosen target flags to every command. Use `--format json` when capturing IDs, and `--dry-run` to preview mutations before applying them.

```bash
tld flow create "Checkout" --view root --description "A purchase journey" --dry-run
tld flow create "Checkout" --view root --description "A purchase journey"
tld flow step add Checkout --view root --id request --title "Receive request" --element API
tld flow step add Checkout --view root --id payment --title "Payment result" --after request
tld flow branch create Checkout payment --view root --mode alternate --path Approved --path Declined
tld flow inspect Checkout --view root --format json
```

The first two commands show the preview/apply pair; preview subsequent step and branch mutations with `--dry-run` too. Use IDs from applied results when authoring dependent commands; preview-generated IDs are not saved.

Read the returned path IDs, then add children using `flow step add <flow> --path <path-id>`. `--path` takes a stable ID, not a display name. Use `--connector <key-or-id>` to highlight a connector discovered from the live diagram. Elements must be placed in the flow's view and connectors must belong to its diagram content.

- `step update <flow> <step-id>` preserves omitted fields. Repeated `--element` or `--connector` flags replace that entire highlight list; use `--clear-elements`/`--clear-connectors` to empty it.
- `step move <flow> <step-id> --path <path-id> --before <sibling-id>` moves a step and descendants. Omit `--path` for the top-level sequence. `--before` and `--after` are mutually exclusive and refer to siblings in the destination sequence.
- Branch modes are `alternate` and `parallel`, with 2–26 named paths. `branch path` supports add, update, move, and delete. Preserve stable IDs when editing or reordering.
- Link a step to another saved flow with `--go-to <UUID-or-name>` and optionally `--go-to-view <view>`. A step can have branches or a linked flow, never both. Use `--clear-go-to` to remove the link.
- Deleting a step or branch removes its descendants. Preview the scope. Deleting a flow retains its diagram resources.

For edits to existing flows, use the inspected `--expected-revision` when guarding against concurrent changes. On conflict, inspect and reconcile before retrying; do not blindly retry with a new revision.

## Verify and deliver

Run `tld flow inspect <UUID> --format json` against the same target. Check order, branch children, highlights, linked flows, and warnings against the requested story. Separately compare the narrative with its source anchors: import validation verifies structure and references, not execution semantics or coverage. Export with `tld flow export <UUID> -o <file>.flow.yaml` when a portable artifact is requested; add `--complete` to include highlighted resources and connector endpoints.

Validate a generated export with `flow import <file> --view <view> --dry-run`. This checks contextual references without creating another flow. Inspect playback in the UI if available; CLI inspection alone does not prove visual presentation quality.

A complete import uses separate resource and flow writes. If the result is interrupted, uncertain, or reports `partial_import`, inspect the returned IDs and live state before retrying; repeated imports can duplicate diagrams and flows. Stop blind retries and report unresolved recovery needs.

Report the saved flow/view, YAML file if created, validation performed, and any unresolved links or visual verification limits.
