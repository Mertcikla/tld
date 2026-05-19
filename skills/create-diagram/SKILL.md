---
name: create-diagram
description: Create architecture diagrams from a local codebase or system description. Use when the user asks to diagram a repo, map system architecture, show component relationships, create drill-down views, or explain a system visually with tld.
allowed-tools: Read, Glob, Grep, Write, Bash(tld *)
---

# Create Diagram

Build a tld workspace that helps someone understand a system by moving from broad architecture to useful detail.

## When to Use

- The user asks for a diagram, architecture map, component map, dependency map, or data-flow view.
- The target is a local codebase, service, package, multi-service app, or described system.
- The useful output is a navigable tld model rather than prose alone.

## When NOT to Use

- The user only wants code explanation and did not ask for a visual model.
- The task is to change application behavior.
- The diagram is unrelated to software/system structure and does not benefit from tld's hierarchy.


## tld Mental Model

tld models architecture as a hierarchy of **elements** and **connectors**.

- An **element** is the node in the knowledge graph. It can represent a system, subsystem, service, module, class, database, external system, user, or any other entity relevant to the architecture.
- A **view** is the canvas inside an element. Add children with `--parent <ref>` to create drill-downs. There is no separate "create view" command. View is used to group related elements, or to explain a subsystem in more detail.
- A **connector** is the edges of the graph, describes a relationship between elements. The view is inferred from the elements' shared parent.
- A **kind** is a broad role such as `system`, `container`, `component`, `database`, `external system`, or `person` omit if it doesn't fit any clear category. 
- A **technology** is metadata. Prefer catalog names suggested by `tld tech suggest`.

The goal is not to mirror folders. Build a navigable map of how control, data, ownership, and dependencies move through the system.

## CLI Basics

Use the flat CLI shape:

```bash
tld add "<name>" --ref <ref> --kind <kind> --parent <parent-ref>
tld connect --from <source-ref> --to <target-ref> --label "<specific interaction>"
tld validate
```

For root-level elements, omit `--parent`.

If a technology label is uncertain, check it first:

```bash
tld tech suggest "<technology name>"
```

If the same real element appears in another view, reuse the same `--ref` with a different `--parent`. This creates another placement, not a duplicate element, and highly desired to truly show interaction. Add connectors with different labels in each view to show different relationships using `--view <optional-view-ref>`.

## Working File

Record all diagram commands in `./.tld/diagram.sh`:

```bash
#!/bin/bash
set -e
```

Group commands by view or subsystem with short comments. Run each new block after adding it so mistakes stay small and the script remains an execution log.

Do not manually edit `elements.yaml` or `connectors.yaml`. Use `tld remove element <ref>` or `tld remove connector ...` for corrections, then append the correction to `diagram.sh`.

## Workflow

Before exploring, check for `.tld` folder. If it exists we are working in an active workspace and should inspect the existing model and ask the user how they want to update it. If not, run `tld init` in the codebase root then ask the user the following to set expectations and guide the level of detail:

> 1. **How detailed should the diagram be?** (overview, medium, detailed)
Use their answers to calibrate the rest of the work. If they don't know, suggest options based on the codebase size once you've done a quick directory scan.

**Overview** ~5–10 views, 1–2 levels deep Services and their direct dependencies. Entry points, major layers, external systems. Nothing low level.

**Medium** ~10–30 views, 2–3 levels deep Modules and packages decomposed. Key classes identified and placed. Major data and logic flows wired. Inheritance shown where architecturally significant.

**Detailed** ~50–200+ views, 4–6 levels deep. Every significant class and function has its own linked view. A reader should be able to navigate from the root view down to understanding a specific method's behavior without opening a file. Connected end-to-end from producers to consumers. 

1. Inspect the codebase enough to identify entry points, major subsystems, storage, external systems, and runtime boundaries.
2. Write a compact inventory before adding elements:

```text
Subsystem inventory:
- API: handles HTTP requests | calls: services, auth, database | called by: frontend
- Worker: processes queued jobs | calls: redis, external API | called by: queue
```

3. Create a root view with a few high-level elements that match the system, not a fixed template.
4. For each view, add children first, then immediately add labeled connectors for that same view.
5. Drill down on sub-systems if asked for: service internals, module responsibilities, key classes, important data paths, or complex methods. Children **reuses the parent element ref when connecting, unless children exists as indiviual elements**


```text
API
Subsystem inventory:
-  Parent Element: Users 
    - endpoints: GET /users, POST /users
    - services: UserService
    - handlers: UserHandler
    - calls: UserService, GetUserData
```

6. Reuse shared elements in every relevant view and reconnect them, optionally with labels specific to that context.
7. Run `tld validate`, read the output carefully, fix the model, and repeat until validation passes or only intentional exceptions remain.
8. Run `tld plan` to see a summary of the workspace, if the resource counts do not match the initial goals set with the user, iterate. Use validation errors as a guide, look for overly-simplified views, or missing relationships.


## Modeling Guidance

Keep each view readable. Aim for about 8-12 elements; split or cluster before a view becomes crowded. If a parent would contain many items, introduce a new element with a role-based name such as "Data Layer", "Event Pipeline", or "Authentication".

An isolated element is usually a modeling bug. Either connect it, move it to a better view, or remove it.

Depth should match the user's goal, before handoff, validate that the diagram meets their needs. If they want an overview, don't add too much detail. If they want a detailed map, make sure to drill down enough.

## Handoff

Ask the user to run to apply pending changes: 

```bash
tld apply 
```

Then to view the diagram:

```bash
tld serve --open
```

Use their feedback to if they want to add/remove some detail on sub-systems. Make adjustments to `diagram.sh`, run it, and validate again.
