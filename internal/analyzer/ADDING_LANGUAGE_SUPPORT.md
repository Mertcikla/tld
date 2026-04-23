# Adding Language Support To `tld analyze`

This document traces `tld analyze` from the Cobra command down to the YAML writers and turns the current implementation into a reusable language-agnostic process.

The goal is not to copy the current Go-specific parser behavior into every new language. The goal is to understand the shared pipeline and feed it the normalized data it already expects.

## Why this guide exists

`tld analyze` does two distinct jobs:

1. Parse source files into a normalized set of declarations and references.
2. Translate that normalized data into workspace elements, connectors, and views.

When adding a new language, most of the work should stay in step 1. The command, deduping, connector planning, and YAML persistence layers are already shared.

## End-to-end flow

The entry point is `cmd/analyze.go` in `newAnalyzeCmd`.

### 1. Resolve the analysis scope

The command starts by:

- Resolving the input path to an absolute path.
- Loading the workspace with `workspace.Load`.
- Expanding the path into one or more repository scopes with `ResolveAnalyzeRepoScopes` from `cmd/repo_scope.go`.

Important behavior:

- In a configured multi-repo workspace, analyzing the workspace root fans out into the configured repositories.
- An input path outside configured repositories is rejected.
- In a single-repo workspace, the command anchors identities to the detected git repository root.

This matters for new languages because repo scope determines the stable `repo`, `branch`, and repo-relative `file_path` values later written to `elements.yaml`.

### 2. Build the scan plan

For each repo scope, the command:

- Merges workspace-level and repository-level exclusions with `Workspace.IgnoreRulesForRepository`.
- Counts matching entries to drive the progress bar.
- Optionally prepares incremental filtering with `--changed-since`.

This step is language-neutral. A new language should respect the same ignore model and should not try to implement its own exclusion logic in the parser.

### 3. Extract normalized analyzer results

The command calls `analyzeService.ExtractPath`, which currently resolves to `internal/analyzer.DefaultService()`.

That service path is:

- `internal/analyzer/service.go`: shared service interface.
- `internal/analyzer/tree_sitter_service.go`: default implementation.
- `internal/analyzer/parser_registry.go`: language-to-parser registration.

Each parser only needs to return:

- `[]analyzer.Symbol`
- `[]analyzer.Ref`

The shared pipeline does not care how the parser found them.

### 4. Apply coarse filtering before graph creation

After extraction, the command performs a first cleanup pass:

- `filterSymbols` removes ignored symbol names.
- `filterSymbolsByFiles` and `filterRefsByFiles` apply `--changed-since`.
- In `--deep` mode, only extra references are merged in from the wider repo scan. The symbol set still comes from the original scan target.

This is an important dedupe boundary: deep mode is allowed to discover more cross-references, but it must not duplicate the local declaration graph.

### 5. Materialize the element hierarchy

The command then turns parser output into workspace elements in four layers:

1. Repository element
2. Folder elements
3. File elements
4. Symbol elements

The helpers in `cmd/analyze.go` do most of this work:

- `analyzeElementRoot`
- `analyzeRelativeFilePath`
- `uniqueFilePaths`
- `uniqueFolderPaths`
- `ensureAnalyzeElement`

The parser is not responsible for creating file or folder nodes. It only needs to emit correct symbol locations. The command derives the rest.

### 6. Resolve references into connectors

Once symbol elements exist, the command resolves `analyzer.Ref` records into connectors with `buildAnalyzeConnectorsForRef` in `cmd/analyze_connectors.go`.

This stage creates:

- Symbol-to-symbol call connectors.
- File-to-file dependency connectors.
- File-to-folder dependency connectors for import-style references.
- Folder-to-folder aggregate dependency connectors.

The connector builder is language-agnostic except for one thing: import-like references need a way to translate a raw import target into a repo-relative directory. The current Go path does that with `analyzeModulePath` and `analyzeRepoRelativeImportDir`, but conceptually that is just one instance of a broader rule:

- If the language exposes package, module, namespace, include, or import targets, normalize them into repo-relative dependency targets before connector planning.

### 7. Deduplicate connectors and persist YAML

Before writing anything, the command runs `uniqueAnalyzeConnectors`.

Then persistence is split across two workspace writers:

- `workspace.Save(ws)` writes the full element map.
- `workspace.AppendConnectors(...)` appends the connector batch and saves again.

The YAML writer layer in `workspace/writer.go` handles formatting, stable output, metadata persistence, and legacy-file cleanup.

## The parser contract

The stable extension seam is the `fileParser` interface in `internal/analyzer/parser_registry.go`:

```go
type fileParser interface {
    ParseFile(ctx context.Context, path string, source []byte) (*Result, error)
}
```

Everything that follows depends on the quality of the returned `Result`.

### `Symbol` fields and why they matter

`internal/analyzer/types.go` defines the normalized declaration shape.

`Name`

- Use the simple declaration name.
- Do not pre-qualify it with package or module names unless the language truly requires that to represent the declaration.
- The command generates more qualified display names later when collisions appear.

`Kind`

- Use stable, cross-language categories when possible: `function`, `method`, `class`, `interface`, `enum`, `record`, `constructor`, and similar workspace-friendly names.
- Avoid leaking parser-node names directly into YAML.

`FilePath`

- Return the real file path given to the parser.
- The command later converts it into a repo-relative `file_path` for stable identities.

`Line` and `EndLine`

- `Line` anchors the declaration start.
- `EndLine` should cover the declaration body or full declaration span.
- This is required by `symbolByFileAndLine`, which maps a reference site back to the enclosing symbol element.

`Parent`

- Use the immediate lexical owner name for nested declarations when the language has them.
- This lets the command place methods under classes, members under enclosing types, and so on.

### `Ref` fields and why they matter

`Name`

- Use the simple referenced name that should resolve to a declaration element.
- For chained calls such as member access, prefer the terminal callable or constructible name.

`Kind`

- Use `call` for call-like symbol references.
- Use `import` for import-like dependencies that should generate file/folder dependency edges.
- If omitted, the current code treats most parser-produced refs like regular call references.

`TargetPath`

- Populate this for import-like refs when the language exposes a package, namespace, module, include, or import target.
- This field is what lets the command build aggregate dependency edges even when no exact declaration target is available.

`FilePath`, `Line`, and `Column`

- These locate the reference site.
- `Column` should point at the start of the referenced identifier when possible.
- Accurate columns improve definition lookup via LSP and reduce ambiguous fallback matching.

## Deduping strategies already built into the command

New language support should reuse these strategies instead of replacing them.

### 1. Repository-scope dedupe

- A configured workspace root expands into each configured repository once.
- Deep mode may add more reference observations, but it does not create a second declaration graph.

### 2. Identity-based element reuse

`ensureAnalyzeElement` does not reuse elements by display name alone.

The identity key is effectively:

- repo
- branch
- cleaned file path
- symbol name
- kind

This prevents different files from collapsing into a single symbol element just because they share the same name.

### 3. Stable ref generation

When a new element ref must be created, `uniqueAnalyzeRef`:

- Starts from a slug of the element name.
- Falls back to a file-name-prefixed slug.
- Adds numeric suffixes when needed.

This keeps refs deterministic enough for repeated analysis passes without depending on display-name uniqueness.

### 4. Global display-name dedupe

Element names must be globally unique in the workspace.

`uniqueAnalyzeElementName` tries progressively more specific candidates:

- Raw symbol or file name.
- Parent-qualified symbol name.
- File-qualified symbol name.
- Owner-qualified path-based name.
- Numeric suffix as a last resort.

This is important for new languages with common names like `main`, `index`, `Service`, `run`, or `build`.

### 5. File and folder dedupe

The command derives unique file and folder sets from the filtered symbol list using cleaned repo-relative paths.

Implication:

- If a file produces no kept symbols, it does not get its own file element.
- References from that file also cannot produce symbol-level call edges because there is no enclosing symbol to anchor them.

That is a current pipeline constraint new languages should know about.

### 6. Connector dedupe and merge

`uniqueAnalyzeConnectors` performs three cleanup passes:

- Exact duplicate connectors collapse by connector key.
- Reverse connectors with the same label collapse into a single connector with direction `both`.

This keeps dependency views compact even when multiple call sites or multiple import statements point at the same target.

## How cross-references are found

Cross-reference resolution is handled after elements exist, not during parsing.

### Step 1: find the source symbol

`refByFileAndLine` uses `symbolByFileAndLine` to find the most specific symbol whose span contains the reference line.

That is why declaration spans matter. A parser that omits `EndLine` makes source-symbol attribution less precise.

### Step 2: resolve the target symbol

`resolveAnalyzeTargetRef` uses this order:

1. Try language-server definition lookup through `analyzeLSPResolver`.
2. Map the returned location to a symbol by file path and line span.
3. Fall back to name-only matching if and only if the referenced name has exactly one candidate symbol in the analyzed batch.
4. Drop the reference if the fallback is ambiguous.

This is a deliberate cleanup rule: the command prefers missing edges over guessed edges.

### Step 3: generate connectors at multiple levels

For a resolved call-like reference, the command may emit:

- A symbol-level `calls` connector.
- A file-level `depends_on:reference` connector if source and target are in different files.
- A folder-level `depends_on:reference` connector if source and target are in different folders.

For an import-like reference, the command may emit:

- A file-to-folder `depends_on:import` connector.
- A folder-to-folder `depends_on:import` connector.

This multi-level projection is what makes the same parser output useful for both detailed and high-level architecture views.

### Step 4: place the connector in the best view

`analyzeCommonConnectorView` chooses the closest shared ancestor view of the source and target elements.

That keeps edges attached to the most local view that can show both endpoints, instead of forcing everything into the workspace root.

## Cleanup before saving to YAML

Before the writers touch `elements.yaml` and `connectors.yaml`, the command and workspace layer clean up the data in several ways.

### Command-layer cleanup

- Ignore excluded symbols.
- Ignore excluded files and directories during traversal.
- Limit incremental runs to changed source files.
- Normalize absolute file paths into repo-relative paths.
- Skip unresolved references.
- Skip ambiguous name-only target matches.
- Skip self-references where source and target are the same symbol.
- Skip duplicate aggregate file/folder edges.

### Reuse and merge cleanup

When an analyzed element already exists:

- Its ref is reused when identity matches.
- Existing `description`, `technology`, and `url` values are preserved if analyze does not provide replacements.
- Existing placements are merged rather than replaced wholesale.

This is important because `analyze` is allowed to refresh structural data without wiping user-authored annotations.

### YAML-writer cleanup

`workspace/writer.go` adds another normalization layer:

- Mapping nodes are written in a stable format.
- Flow-style YAML is removed.
- Element fields are reordered so `name` and `kind` stay at the top.
- Connector maps are saved through deterministic keys.
- Metadata is persisted to the lock file when possible.
- Legacy workspace files such as `diagrams.yaml`, `objects.yaml`, `edges.yaml`, and `links.yaml` are removed.

The parser should not try to solve any of these persistence concerns.

## What to implement for a new language

### Minimum implementation

1. Add the file extensions to `internal/analyzer/language.go`.
2. Implement a `fileParser` in `internal/analyzer`.
3. Register it in `newDefaultParserRegistry`.
4. Emit normalized `Symbol` and `Ref` records with accurate paths and positions.
5. Add analyzer service tests for declarations and references.
6. Add end-to-end `cmd/analyze` tests for element and connector output.

### Recommended parser behavior

- Emit declaration spans, not just start lines.
- Emit parent names for nested declarations.
- Emit identifier-start columns for references.
- Emit import-like refs separately from call-like refs.
- Normalize member-call names to the terminal callable name.
- Prefer normalized semantic kinds over raw AST node names.

### Language-specific hook to think about early

Most popular languages have some notion of external or cross-file dependency target:

- imports
- package references
- namespace references
- includes
- module paths
- use statements

If the parser can recover that target, put it in `Ref.TargetPath` and mark the ref as `import`.

The current command only has one concrete implementation of import-path-to-directory translation, but the architectural requirement is broader: new languages should supply enough information for repo-relative dependency aggregation.

## Testing checklist for new language support

At the analyzer layer, add cases for:

- top-level declarations
- nested declarations
- call references
- constructor or instantiation references, if the language has them
- import or include references, if the language has them
- unsupported-file detection

At the command layer, add cases for:

- element creation
- folder hierarchy creation
- reuse of existing elements
- cross-file connectors
- cross-folder aggregate connectors
- duplicate-name handling
- ambiguous-reference dropping

## Current limitations worth keeping in mind

These are not blockers, but they matter when designing new parser output.

### Parent matching is still name-based

Nested declarations are attached to parents by parent name, and only when the parent name resolves uniquely in the current symbol batch.

If a language commonly allows repeated enclosing type names across files, that may flatten some nesting until we introduce a more explicit parent identity.

### Name-only fallback is intentionally conservative

If LSP definition lookup is unavailable and a reference name matches multiple candidate symbols, the connector is dropped.

For languages with heavy overloading or very common method names, accurate `Column` values and working LSP support are much more important.

### Files without kept symbols do not become first-class nodes

The current graph model is declaration-driven. Files that only contribute imports or free-floating references but no retained symbols do not become standalone file elements.

### Import aggregation needs a language-aware translation step

The shared connector planner expects repo-relative dependency targets. New languages may need a small adapter that translates their import model into that form.

## The core rule

When adding language support, do not reimplement the analyze command.

Instead:

- Parse the language into the shared `Symbol` and `Ref` model.
- Preserve enough positional information for later resolution.
- Let the existing command own dedupe, graph projection, connector planning, and YAML persistence.

If a new language needs extra behavior, add it as a narrow normalization hook near the parser or reference-resolution boundary, not as a separate command path.
