# Portable flow YAML

Use `tld flow import --schema` for the installed version's exact fields. The source definitions are `schemas/flow.schema.json` and `internal/flowdoc/` in the TLD repository. This format is separate from workspace `elements.yaml`/`connectors.yaml` and the ordinary `tld import` document.

## Existing diagram

```yaml
format: tld-flow
version: 1
name: Checkout
description: A purchase journey
steps:
  - id: request
    title: Receive request
    elements: [API]
    connectors:
      - source: API
        target: Database
        label: writes
  - id: payment
    title: Payment result
    branches:
      mode: alternate
      paths:
        - id: approved
          name: Approved
          steps:
            - id: persist
              title: Persist order
              elements: [Database]
        - id: declined
          name: Declined
          steps: []
```

Portable highlights use exact **display names**, not CLI refs or numeric IDs. Each name must uniquely identify a placed element in the destination view. Connector identity uses ordered source/target names and label. Omit `label` only when the endpoints identify exactly one connector; `label: ""` explicitly selects an unlabeled connector.

Steps accept `title`, `description`, `elements`, `connectors`, and either `branches` or `goToFlow`. For a linked flow:

```yaml
  - id: fulfillment
    title: Continue to fulfillment
    goToFlow:
      name: Fulfillment
      view: Order processing
```

`goToFlow.view` is an exact unique view **name**, not a CLI view selector. Without it, resolution is scoped to the importing flow's view. Linked flows must already exist; exports do not bundle them. For `--new-view`, give links explicit existing view names.

Use stable descriptive step/path IDs. IDs may be omitted and generated, but all supplied step and path IDs share one uniqueness namespace across the entire tree. Runtime limits include 1 MiB UTF-8 input, 1,000 total steps, eight levels of branch nesting, and 2–26 paths per branch. Unknown fields are rejected. Schema validation alone does not validate live references or all runtime constraints.

```bash
tld flow import checkout.flow.yaml --view root --dry-run
tld flow import checkout.flow.yaml --view root
```

Keep destination flags consistent. Import creates a **new flow** by default. To revise an existing flow, export it first, preserve the intended content, then use:

```bash
tld flow import checkout.flow.yaml --view root --replace <flow-uuid> --expected-revision <revision> --dry-run
```

Apply the same command without `--dry-run` once reviewed. `--replace` replaces the complete flow content; it is not a partial merge.

## Complete bundle

Append resources to the example above to create a fresh diagram:

```yaml
resources:
  elements:
    - name: API
      kind: service
      position_x: 120
      position_y: 200
    - name: Database
      kind: database
      position_x: 480
      position_y: 200
  connectors:
    - source: API
      target: Database
      label: writes
      direction: forward
      style: bezier
```

Both resource lists are required, even if empty. Resource names must be unique; include all highlighted elements and every connector endpoint. Supply both position coordinates or neither. Consult the schema for optional metadata and technology icons.

```bash
tld flow import checkout.flow.yaml --new-view "Checkout demo" --dry-run
tld flow import checkout.flow.yaml --new-view "Checkout demo"
```

This creates fresh elements/connectors and a new element-owned view under Root, then saves the flow. `--new-view` cannot combine with `--view`, `--replace`, or `--expected-revision`. Importing with `--view` ignores bundled resources and resolves against the existing diagram instead.

For an existing saved flow, prefer generating the bundle with `tld flow export <flow-uuid> --complete -o checkout.flow.yaml`. Complete export contains highlighted resources and connector endpoints, not necessarily the entire diagram.
