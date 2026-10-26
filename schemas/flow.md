# Flow YAML

A flow is an ordered `steps` list. Every step still supports zero or more
`elements` and `connectors`, optional `title`, `description`, and string-valued
`metadata`. Existing flat v1 flows continue to work without changes.

The additional operations follow [IcePanel's flow concepts](https://docs.icepanel.io/visual-storytelling/flows).
They extend a step instead of introducing separate message, process, or note types:

| Operation | Representation |
| --- | --- |
| Introduction or conclusion | `overview: true` plus title/description, at the beginning or end |
| Message or response | A connector, optionally `reverse: true` |
| Process | An element with a description of its action |
| Information | Text and optional metadata, with no highlights |
| Alternate paths (OR) | `branches: { mode: alternate, paths: [...] }` |
| Parallel paths (AND) | `branches: { mode: parallel, paths: [...] }` |
| Go to another flow | `goToFlow: { name: Payment processing, view: Payments }` |

Use at most one of `overview: true`, `branches`, and `goToFlow` on a step.
Highlights and text can accompany any of them. Metadata remains descriptive;
it never evaluates conditions or executes commands.

See [the example](flow.example.yaml) and [schema](flow.schema.json).
The example requires the named diagram objects and a saved “Payment processing”
flow in a diagram named “Payments”. Import does not create dependencies.

## Playback

- Overview derives the union of all highlights in this flow, including nested
  paths. It does not expand linked flows. Use it for an introduction or conclusion.
- An ordinary empty step clears highlighting; it does not inherit previous steps.
- Alternate branches follow the selected path, defaulting to the first, then
  rejoin the containing sequence. The path selector lets viewers choose another
  scenario. These are presentation choices, not evaluated conditions.
- Parallel branches initially highlight the first step in each path together.
  Playback then follows the selected path (the first by default), as documented
  by IcePanel. Viewers can return to the branch and switch paths to explain each
  concurrent sequence; paths are not zipped into artificial synchronized frames.
- Autoplay pauses at linked-flow steps. Go opens the linked flow; Skip continues
  the caller. Return is available throughout the linked visit and at its end,
  restoring the exact caller step and path selections. Recursive visits are
  explicit and limited to 16 nested calls.

The editor and ZuiCanvas use the same playback model. The core editor navigates
to another diagram for cross-diagram links. Hosts embedding `FlowPanel` for one
diagram supply `onNavigateView(viewId)` and remount it with that diagram's data;
the React Query client carries the return session. Explorer hosts supply their
`diagrams` map. Saved flows retain their original diagram; visiting a linked flow
never copies or edits it. Drafts remain available on return.

## References and validation

Elements resolve by exact names in the target diagram. Connector `source` and
`target` are ordered element names; optional `label` selects among multiple
connections. `label: ""` selects an unlabeled connection. Omit `label` only when
exactly one connection matches the endpoints. Duplicate highlights are rejected.

`goToFlow.name` identifies a saved flow. Omit `view` for the current diagram;
otherwise use its exact name. Missing or ambiguous names fail import/export.
Files contain names, not database IDs. A linked target must be imported or
created first. The API also checks that link targets are accessible when saving;
deleted targets remain skippable during playback.

Each branch has 2–10 paths. A path requires `name` and ordered `steps`; `id` is
optional. Step and path IDs must be unique across the whole flow and are generated
when omitted. Reordering and duplication use the existing editor controls.
Limits: eight nested branch levels, 500 total steps across all paths, 500 elements
and 500 connections per step, 50 metadata entries per step, and 1 MiB per YAML
file. Existing text limits and strict YAML 1.2 parsing still apply.

SDK additions are optional arguments:

```ts
const catalog = { flows: savedFlows, views: [{ id: 42, name: 'Payments' }] }
importFlowYaml(source, targetViewId, diagram, bindings, catalog)
exportFlowYaml(flow, diagram, catalog)
```

`bindings` still resolves element names and step-ID/index connector references,
including references inside branches. Catalog entries are used only to resolve
links. Existing callers without linked flows need no changes.

The ConnectRPC contract adds optional fields to `FlowStep`; stored flat documents
need no migration. Regenerate locally with the parent `make dev-proto` and publish
and pin generated SDKs through the normal proto release workflow before shipping.
