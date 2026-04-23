# pkg/importer

Parses external diagram formats and converts them to the common `ParsedWorkspace` structure used by the import endpoint (`POST /api/views/import`).

## Supported formats

| Format | Dialect | Parser |
|--------|---------|--------|
| Structurizr DSL | Full C4 model DSL (workspaces, nested elements, hierarchical IDs) | `ParseStructurizr` |
| Mermaid | `flowchart` and `graph TD / flowchart` | `ParseMermaid` |

Format is auto-detected by `Parse()` based on keywords (`workspace`/`model` → Structurizr, `flowchart` → Mermaid, otherwise Mermaid).

---

## Output types

```go
type ParsedElement struct {
    ID          string  // element identifier (may be dot-qualified in hierarchical mode)
    Name        string  // display name
    Kind        string  // see kind mapping below
    Shape       string  // hint for renderer (Mermaid only; empty for Structurizr)
    Description string  // element description
    Technology  string  // technology/stack (containers and components)
}

type ParsedConnector struct {
    SourceID   string  // resolved source element ID
    TargetID   string  // resolved destination element ID
    Label      string  // relationship label/description
    Technology string  // relationship protocol or technology
}

type ParsedWorkspace struct {
    Elements   []ParsedElement
    Connectors []ParsedConnector
    Warnings   []string  // lines/constructs that could not be parsed
}
```

---

## Structurizr DSL

### What is parsed

**Elements** (from `model` block only):

| DSL keyword | `ParsedElement.Kind` |
|---|---|
| `person` | `person` |
| `softwareSystem` / `softwaresystem` | `system` |
| `container` | `container` |
| `component` | `component` |
| `element` | `element` |
| Unknown keyword (archetype instance) | `element` + warning |

Positional string arguments per element type:

| Element | pos 0 | pos 1 | pos 2 |
|---|---|---|---|
| `person` / `softwareSystem` | Name | Description | *(tags, ignored)* |
| `container` / `component` / `element` | Name | Description | Technology |

Inline property keywords inside `{ }` blocks (`description "..."`, `technology "..."`) override the positional values.

**Relationships:**

```
src -> dst "label" "technology" "tags..."
```

| Position | Field |
|---|---|
| pos 0 | `ParsedConnector.Label` |
| pos 1 | `ParsedConnector.Technology` |
| pos 2+ | Tags (ignored) |

Supported relationship forms:
- Explicit: `src -> dst "label"`
- Implicit source (inside element body): `-> dst "label"` source resolves to the enclosing element
- Archetype arrow: `src --https-> dst` normalized to a plain arrow, archetype name discarded
- `this` as destination: resolved to the enclosing element's ID

**Identifier modes:**

`!identifiers flat` (default): each element uses its declared ID as-is (`wa`, `db`).

`!identifiers hierarchical`: IDs are dot-qualified by their parent path (`ss.wa`, `ss.db`, `ss.apiApplication.signinController`). References inside a scope resolve local names first (inner scope shadows outer). The directive can appear at workspace level or inside the model block.

**Groups:**

`group "name" { ... }` is parsed transparently elements inside are captured with their own IDs, the group wrapper itself is not added as an element.

### What is skipped

The following are consumed and discarded:

| Construct | Why skipped |
|---|---|
| `views { ... }` | Layout and rendering concerns |
| `deploymentEnvironment`, `deploymentNode`, `infrastructureNode`, `containerInstance`, `softwareSystemInstance` | Deployment topology, not logical model |
| `archetypes { ... }` | Type definitions; instances are parsed as kind `element` with a warning |
| `styles`, `themes`, `theme` | Visual styling |
| `properties { ... }`, `perspectives { ... }`, `configuration { ... }` | Metadata blocks |
| `!docs`, `!adrs`, `!script`, `!plugin`, `!include`, `!const`, `!var`, `!elements`, `!relationships` | Directives |
| `workspace extends <file>` | External workspace extension (the current file is still parsed) |
| `tags`, `tag`, `url` inside element body | Tag and URL metadata |

---

## Mermaid

### What is parsed

**`flowchart`:**
- `service id(kind)[Name]` and `group id(kind)[Name]` → elements with `Shape` set to the kind token
- `id1:R -- L:id2` connectors (direction tokens discarded)
- All elements get `Kind: "system"`

**`graph TD` / `flowchart`:**
- `id[Label]`, `id(Label)`, `id{Label}` → elements with `Kind: "system"`
- `id1 --> id2` and `id1 -->|label| id2` connectors

### What is skipped

- `style`, `classDef` lines
- `%%` comments
- Diagram type declaration lines (`graph TD`, `flowchart LR`, `architecture-beta`)

---

## Import behaviour summary

When you import a workspace, you get back a flat list of elements and connectors. The following information is **preserved**:

- Element name, kind, description, technology
- Relationship label and technology
- Hierarchical identity (as dot-qualified IDs, not as a parent/child DB relationship)

The following information is **not available after import** and must be set manually:

- Visual positions (x/y coordinates)
- Styling, colors, shapes (Structurizr), border styles
- Tag values
- URLs attached to elements
- Deployment topology
- View definitions (which elements appear in which view)
- View-level metadata (title, description, autoLayout settings)
- Nested parent/child hierarchy all elements are returned at the same flat level; the dot-qualified ID is the only indicator of original nesting
