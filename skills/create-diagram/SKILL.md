---
name: create-diagram
description: Create architecture diagrams from a codebase using the tld CLI. Use this skill whenever the user asks to diagram, map, or document their codebase or system architecture even if they don't mention "tld" or "diagram" explicitly. Trigger on phrases like "map my services", "document my architecture", "create a system diagram", "diagram this repo", "show how my code is structured", or any request to visually represent how a system's components fit together.
allowed-tools: Bash(tld *), Write
---

> **Reference:** When you need exact command syntax, flags, or a full walkthrough example, read `references/tld-docs.md`.

## How tld thinks about architecture

In tld, everything is an **element** a service, a database, a person, a class, a method, a module. Any element can have a **view**: a navigable canvas that holds child elements. Navigation works by drilling into any element whose view contains something interesting. There are no standalone "diagrams" to create separately views emerge from the element hierarchy.

**Connectors** express relationships between elements in the same view. The view is inferred automatically from the elements' shared parent.

**Roles** (service, database, external system, person, etc.) are expressed as tags not as distinct types. All elements share a single kind.

This is not traditional architecture diagramming. It is a **navigatable atlas of the entire system** like a hyperlinked wiki where you can zoom into anything. At the top you see services talking to each other. Drill into a service and you see its modules. Drill into a module and you see its classes. Drill into a class and you see its methods, parameters, what each method calls, its parent classes, its subclasses.

**The goal at full detail: a reader can start at the root view and follow drill-downs until they understand any piece of the system without ever opening a file.**

---

## The goal: a view that earns its existence

A view is only useful if it tells you something you couldn't immediately see from the file structure. The failure mode to avoid is a **shallow view** a list of boxes with no connectors, or connectors with no labels, or elements that exist in isolation without showing who talks to whom, who owns what data, or where failures propagate. Boxes are cheap. The connectors are the insight.

**Before moving to the next step, ask: would a new engineer reading this view understand how data or control actually flows? If not, add more connectors.**

---

## Working surface: diagram.sh

All tld commands go into a single bash script, `diagram.sh`, in the workspace directory. The script is your notes and your execution log comments explain what you found, commands record what you built.

**Create the script once at the beginning of Step 3:**

```bash
#!/bin/bash
# Architecture diagram build script
set -e
```

**For every batch:**
1. Append a labeled block of commands (with a `# ===` comment header) to `diagram.sh`
2. Run only that new block
3. Do a **batch checkpoint** before continuing to the next batch

The script is the complete, replayable record of every diagram decision. Never run a tld command outside it.

---

## Batch checkpoint

Run this check after every batch of `tld add` or `tld connect` commands:

- **New elements:** Do any just-created elements also appear in other views (i.e., have a second placement)? Add them now.
- **New views:** Do any just-created elements' views connect to existing views via drill-down? No separate link commands needed drill-down happens automatically when a child is placed inside an element's view.
- **Cross-batch connectors:** Are there interactions between elements from this batch and elements from a previous batch that aren't wired yet?

Append any missing connectors to `diagram.sh` and run them before moving on.

---

## View density: the 10-element rule

Keep each view focused. **Aim for ~10 elements per view; never exceed 20.** A crowded view isn't more informative it becomes a wall of boxes where the relationships disappear. If you're placing more than 10–15 elements in a single view, that's a signal to cluster before you continue.

### When and how to cluster

After mapping your inventory, count the elements you're about to place. If the count is heading above 10–15:

1. **List all the elements** write them out so you can see the whole set at once.
2. **Find the largest cohesive group** look for elements that share a role, layer, or domain: all the auth-related pieces, all the data-layer components, all the event processors, etc.
3. **Label the cluster by its role** give it a name that reflects what those elements *do* at that level of abstraction (e.g., "Auth Services", "Data Layer", "Event Pipeline").
4. **Create a container element for the cluster** `tld add "Auth Services" --parent root`. This element's view will hold the cluster.
5. **Place the clustered elements inside it** `tld add "Token Validator" --parent auth-services`.
6. **Replace the cluster on the parent view with the container element** the parent now has one element for the whole cluster; drill in to see the details.
7. **Repeat** until every view sits at or below 15 elements.

The goal is not to hide complexity it's to present the right level of detail at each zoom level.

---

## Task Progress

Copy this checklist and check off items as you complete them:

- [ ] Step 1: Explore the codebase
- [ ] Step 2: Produce a subsystem inventory
- [ ] Step 3: Create diagram.sh and root elements batch checkpoint
- [ ] Step 4: Add child elements per subsystem batch checkpoint after each view
- [ ] Step 4a: Connect elements within each view
- [ ] Step 4b: 10-element rule
- [ ] Step 5: Audit shared elements trace every view they appear in
- [ ] Step 6: Drill into every major subsystem (3–5 levels) batch checkpoint after each view
- [ ] Step 7: Run `tld validate`
- [ ] Step 8: Ask user to run `tld plan` and give feedback; iterate if needed

---

## Step 1: Explore the codebase

Before exploring, ask the user two questions:

> 1. **How many views do you expect?** (rough target is fine e.g. "around 20", "as many as needed", "keep it under 10")
> 2. **How deeply nested do you want the drill-downs?** (e.g. "just top-level", "2–3 levels", "go as deep as possible")

Use their answers to calibrate the rest of the work. If they don't know, suggest options based on the codebase size once you've done a quick directory scan.

**Shallow** ~5–10 views, 1–2 levels deep Services and their direct dependencies. Entry points, major layers, external systems. Nothing low level.

**Medium** ~10–30 views, 2–3 levels deep Modules and packages decomposed. Key classes identified and placed. Major data and logic flows wired. Inheritance shown where architecturally significant.

**Detailed** ~50–200+ views, 4–6 levels deep Every significant class has its own linked view. Each class view shows: `__init__` parameters and their types, all public methods with connectors showing which methods call which, all private methods, the full inheritance chain. Every significant function shows its parameters, return type, and external calls. A reader should be able to navigate from the root view down to understanding a specific method's behavior without opening a file.

### Exploration strategy

Start broad, then narrow:
1. Get the lay of the land directory tree, top-level file structure
2. Find entry points `main.go`, `app.tsx`, `index.ts`, `server.py`, etc.
3. Identify major layers API, services, repositories, workers, frontends
4. Follow imports/dependencies to map how components connect
5. Check config files for external dependencies (DBs, queues, auth providers)

**The output of this step is not a list of files it's a map of connections.** For each component you identify, note: what does it call? what calls it? what data does it read or write?

## Step 2: Produce a subsystem inventory

Before creating a single element, write out an explicit inventory of every major subsystem you found:

```
Subsystem inventory:
- [name]: [one-line description] | calls: [...] | called by: [...] | shared deps: [...]
- [name]: ...
```

Do not proceed to Step 3 until every top-level directory or package is accounted for. A diagram that covers 60% of the codebase is worse than no diagram, because it creates false confidence.

---

## Step 3: Create diagram.sh and root elements

Create `diagram.sh`, then append and run the root elements as the first batch. Root elements appear on the top-level canvas:

```bash
# === Root elements ===
tld add "Domain & Business Logic" --ref domain --diagram-label "System"
tld add "Data & Persistence" --ref data --diagram-label "System"
tld add "Interfaces & Integrations" --ref interfaces --diagram-label "System"
tld add "Platform & Infrastructure" --ref deployment --diagram-label "System"
```

Append and run the next level as a second batch:

```bash
# === Level 2: major subsystems ===
tld add "Backend" --ref backend --parent domain
tld add "Frontend" --ref frontend --parent interfaces
tld add "Storage" --ref storage --parent data
```

**Batch checkpoint:** Are there connectors between root elements suggested by your inventory? Add them now:

```bash
tld connect --from domain --to data --label "reads/writes"
tld connect --from interfaces --to domain --label "calls"
```

---

## Step 4: Add elements and wire them together

Work one view at a time. For each view (parent element), append its children as a batch, run it, then immediately append and run its connectors before moving to the next.

```bash
# === Backend children ===
tld add "REST API" --parent backend --technology "Go" --ref api
tld add "Stripe API" --parent backend --technology "Stripe" --ref stripe
tld add "Job Worker" --parent backend --technology "Go" --ref worker
```

**Batch checkpoint after elements:** Do any of these elements also belong in other views? Add those placements now.

### Step 4a: Connect everything

Append and run connectors for the same view immediately after its elements. View is inferred automatically:

```bash
# === Backend connectors ===
tld connect --from api --to stripe --label "billing"
tld connect --from api --to db --label "reads/writes"
tld connect --from worker --to queue --label "consumes jobs"
```

Labels should describe *what* the interaction does "validates JWT", "publishes events" not just "calls".

An unconnected element is a bug either it's missing connectors, or it doesn't belong in this view.

**Batch checkpoint after connectors:** Any elements from other views that interact with elements you just wired? Append cross-view connectors now.

### Step 4b: 10-element rule

Count elements in each view. More than ~10? Apply the **clustering strategy** from the View density section before continuing.

---

## Step 5: Audit shared elements

Identify every element appearing in more than one view (databases, caches, queues, external APIs). For each one, verify:
- Is it placed in every view where it's relevant?
- Does each placement have labeled connectors showing exactly how that view uses it?

No connectors on a shared element = missing information. Append any missing placements and connectors to `diagram.sh` and run them.

---

## Step 6: Drill into subsystems

**Depth target: match the user's requested level.** At full detail, every significant class and module has its own view and the depth can reach 5–6 levels. Stop only when there is nothing meaningful left to decompose.

Example depth for a typical backend service at full detail:
```
Root (L1)
  └── Backend Service (L2)
        └── Auth Module (L3)
              └── AuthService class (L4)
                    └── AuthService: __init__ params, methods, base classes, subclasses (L5)
                          └── validate_token() : params, JWT lib call, DB call, return type (L6)
```

### What to capture in a code-level view

**Class view** (L4–L5): one element per method (public and significant private), one element per `__init__` parameter that is a dependency (not primitives), connectors showing which methods call which, connectors to parent class element and each known subclass element.

**Inheritance view**: a dedicated view for a polymorphic hierarchy base class at top, all concrete subclasses below, connectors labelled with what each override changes.

**Function/method view** (L5–L6): parameters as elements (with types), return type as an element, connectors to external calls it makes. Only create this level if the function is complex enough to warrant it more than ~5 distinct operations.

**Module/package view** (L3–L4): exported symbols as elements, internal dependencies between them wired, external imports shown as elements.

For every subsystem, append one batch per view and run it before starting the next:

### 6a. Create the subsystem element

```bash
# === API (L3) ===
tld add "API" --ref api-internals --parent backend --diagram-label "Container"
```

No separate link command needed drilling into `api-internals` from the backend view happens automatically.

### 6b. Populate with internal elements

Go back to the code don't guess. Apply the 10-element rule here too; cluster if needed.

```bash
tld add "Auth Middleware" --parent api-internals --technology "Go" --ref auth-mw
tld add "User Handler" --parent api-internals --technology "Go" --ref user-handler
tld add "Database" --parent api-internals --technology "PostgreSQL" --ref db
```

> If `db` already exists from a parent view, `tld add` with the same ref but a new `--parent` adds a new *placement* rather than a duplicate. Reused elements don't inherit connectors add them explicitly for this context.

### 6c. Connect internal elements

```bash
tld connect --from auth-mw --to user-handler --label "forwards request"
tld connect --from user-handler --to db --label "SQL"
```

Before moving to the next subsystem, check every element: at least one incoming connector, at least one outgoing connector, labels specific enough to tell a reader what the interaction does. Missing connectors = go back to the code.

**Batch checkpoint:** Do any elements in this view appear in sibling views? Are there connectors that cross view boundaries? Append and run them now.

Repeat 6a–6c for each remaining subsystem, going 3–5 levels deep per topic.

---

## Step 7: Validate

Append to `diagram.sh` and run:

```bash
# === Validation ===
tld validate
```
Carefully observe its output and its instructions to improve the quality of the diagrams. Don't use scripting to automate this and just to bypass the validation. It needs careful attention and deliberation to improve the diagrams. Work on each issue one by one.

Fix any broken refs or missing fields, append the fixes to `diagram.sh`, and re-run.

---

## Step 8: Hand off to user

Ask the user to run:

```bash
tld plan
```

Walk them through the output. If they want changes more elements, deeper drill-downs, better connector labels append the changes to `diagram.sh`, run the new block, and iterate.
