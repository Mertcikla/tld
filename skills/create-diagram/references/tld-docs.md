# tld CLI Reference (Diagram Creation)

`tld` is a CLI for tlDiagram. Define your architecture in YAML, validate it, and sync it to a tlDiagram server.


## The model: elements, views, and connectors

In tld, everything is an **element**. Every element you add automatically gets its own **view** a navigable canvas that can hold child elements. The hierarchy is defined purely by `--parent` flags: place an element inside another element's view, and the parent becomes navigable. There are no standalone "diagrams" to create separately.

**Connectors** link two elements within the same view. The view is inferred from the elements' shared parent, so you never specify it manually.

**Roles** (service, database, external system, person, etc.) are expressed as tags on an element not as distinct types.

### Workspace file layout

```
~/.config/tldiagram/tld.yaml    # Global config: API key, org
.tld/
  ├── .tld.yaml                 # Workspace config: project metadata
  ├── elements.yaml             # Elements + placements
  ├── connectors.yaml           # Connectors between elements
  └── .tld.lock                 # Sync state at last apply
```


## Commands

### `tld add <name> [flags]`
Create or update an element in `elements.yaml`. **Idempotent:** if an element with the same ref already exists, running `add` again merges the update or adds a new placement.

Every element automatically gets a view (`has_view: true`), so any element can be drilled into.

**Flags:**
- `--ref <slug>` - Override the auto-generated ref (default: slugified name, e.g. `"My API"` → `my-api`)
- `--parent <ref>` - Place this element in the named element's view (default: `root`)
- `--kind <kind>` - Element kind (default: `element`)
- `--description "..."` - What this element does
- `--technology "..."` - Primary technology (e.g. `Go`, `React`, `PostgreSQL`)
- `--url "..."` - Link to docs, repo, or dashboard
- `--diagram-label "..."` - Optional label for this element's view (e.g. `System`, `Container`, `Component`)
- `--position-x <float>` - Horizontal canvas position
- `--position-y <float>` - Vertical canvas position

```bash
# Root-level elements (shown in the top-level canvas)
tld add "Frontend" --technology "React"
tld add "API Gateway" --technology "Go"
tld add "Database" --technology "PostgreSQL"

# Child elements (shown inside the parent's view when you drill down)
tld add "Auth Middleware" --parent api-gateway --technology "Go"
tld add "User Handler" --parent api-gateway --technology "Go"
```

---

### `tld connect [flags]`
Add a connector between two elements. The view is **automatically inferred** from the elements' shared parent you never need to specify it.

**Flags:**
- `--from <ref>` **(required)** - Source element ref
- `--to <ref>` **(required)** - Target element ref
- `--label "..."` - Short label shown on the connector (e.g. `"calls"`, `"reads"`)
- `--description "..."` - Longer explanation shown in the connector panel
- `--relationship "..."` - Semantic type (e.g. `uses`, `depends_on`, `integrates`)
- `--direction` - `forward` | `backward` | `both` | `none` (default: `forward`)
- `--style` - `bezier` | `straight` | `step` | `smoothstep` (default: `bezier`)
- `--url "..."` - Link to relevant docs or API spec

```bash
# Connectors between root-level elements
tld connect --from frontend --to api-gateway --label "HTTP"
tld connect --from api-gateway --to database --label "SQL"

# Connectors within a drill-down view (view inferred from shared parent)
tld connect --from auth-middleware --to user-handler --label "forwards request"
tld connect --from user-handler --to database --label "SQL"
```

---

### `tld remove element <ref>`
Remove an element (and all its placements) from `elements.yaml`.

```bash
tld remove element old-service
```

---

### `tld remove connector [flags]`
Remove a specific connector from `connectors.yaml`.

---

### `tld validate`
Validate workspace YAML for referential integrity, required fields, and cycles.

```bash
tld validate
```

---

## Slugify rules

Names are automatically converted to refs: lowercase, non-alphanumeric characters replaced with hyphens, leading/trailing hyphens trimmed.

Examples: `"My API"` → `my-api`, `"Auth Controller"` → `auth-controller`

---

## Full walkthrough example

```bash
# 1. Init workspace
mkdir my-arch && cd my-arch
tld init .

# 2. Add root-level elements (appear in the top-level "root" canvas)
tld add "Customer" --ref customer
tld add "Web App" --ref web-app --technology "React"
tld add "Payment Gateway" --ref payment-gateway --technology "Stripe"

# 3. Connect them (view inferred automatically as "root")
tld connect --from customer --to web-app --label "browses and buys"
tld connect --from web-app --to payment-gateway --label "processes payments"

# 4. Drill down into Web App (add children view inferred as "web-app")
tld add "React Frontend" --parent web-app --technology "React"
tld add "GraphQL API" --parent web-app --technology "Go"
tld add "Postgres DB" --parent web-app --technology "PostgreSQL"

# 5. Connect inside the Web App view
tld connect --from react-frontend --to graphql-api --label "queries"
tld connect --from graphql-api --to postgres-db --label "SQL"

# 6. Validate and apply
tld validate
tld plan
tld apply
```
