<p align="center">
  <a href="https://tldiagram.com">
    <img src="./frontend/logo/tld.svg" alt="Logo" width="200">
  </a>
</p>


<p align="center">
  <a href="https://go.dev/"><img src="https://img.shields.io/github/go-mod/go-version/mertcikla/tld" alt="Go Version"></a>
  <a href="./LICENSE"><img src="https://img.shields.io/github/license/mertcikla/tld" alt="License"></a>
  <a href="https://github.com/mertcikla/tld/actions"><img src="https://img.shields.io/github/actions/workflow/status/mertcikla/tld/test.yml?branch=main" alt="Build Status"></a>
  <a href="https://goreportcard.com/report/github.com/mertcikla/tld"><img src="https://goreportcard.com/badge/github.com/mertcikla/tld" alt="Go Report Card"></a>
  <a href="https://deepwiki.com/Mertcikla/tld"><img src="https://deepwiki.com/badge.svg" alt="Ask DeepWiki"></a>
</p>

<p align="center">
    <img src="./assets/tld-editor-ss.png" alt="editor-ss">
</p>

---

`tld` is an opinionated, flexible diagramming tool to help you visualize, understand, and maintain your software architecture. Inspired by C4 model, designed with multiple opt-in features to answer evolving needs of software teams. 

## Highlights

- **UI**: A frontend optimized to handle complex architectures while attempting to intelligently show and hide details.
- **Bi-directional Sync**: Seamlessly sync changes between your local YAML files, the self-hosted web UI, and the cloud version at tlDiagram.com.
- **Standalone Distribution**: A single, dependency-free binary containing both the server and the web application.
- **Git diff visualization**: Sync and visualize the changes you or your agent are making live in diagram form. Inspect the dependencies and intervene when necessary.
- **CLI that speaks agent**: Use the [agent skill](./skills/create-diagram/SKILL.md) and use your agent to create a diagram of your codebase with the exact detail level you need. Prompt the agent to add/remove details you see fit. 
Here are some examples that were generated using the agent skill.

  - [Kubernetes](https://tldiagram.com/app/explore/shared/827bc17d-7d9b-411f-9d03-179fab99bcbd)

  - [pytorch](https://tldiagram.com/app/explore/shared/abc56a26-3e20-4235-90fa-e045b2b2ac74)
 
  - [kafka](https://tldiagram.com/app/explore/shared/9d415d7f-b91f-47c0-9dc7-756de2860695)

  - [.NET eShop reference](https://tldiagram.com/app/explore/shared/ba6cbf2a-e0ff-468a-87e5-f720d35f448d)

- **Diagrams as Code**: A Git-like workflow (`plan`/`apply`) to manage architectural evolution alongside your source code.
- **Automated Codebase Analysis**: (Preview) Built-in tree-sitter integration to automatically discover architecture components in Go, Java, Python, C++, and TypeScript (more soon™ (hopefully)).

## Quick Start

macOS and Linux
```bash
curl -LsSf https://tldiagram.com/install.sh | sh -s serve --open
```

Windows
```powershell
powershell -ExecutionPolicy ByPass -c "irm https://tldiagram.com/install.ps1 | iex; tld serve --open"
```

## Deployment & Self-Hosting

The `tld` binary is designed to be run as a persistent service in your infrastructure or as a local development tool.

### Local Development
Run `tld serve` in any directory to start a local instance that uses your current folder for storage. 

### Server Deployment
1. Provide a persistent volume for the `.tld/` directory (where YAMLs and the SQLite cache are stored).
2. Set `TLD_ADDR=0.0.0.0` and `PORT=8060`.

### Configuration
Various configuration options are available in `~/.config/tldiagram/tld.yaml`


## The workflow

1. **Visualize**: Use `tld serve` to open the interactive UI. Drag, drop, and connect components.
2. **Automate**: Run `tld analyze` to scan your repository. It will suggest new elements and connectors based on your actual source code.
3. **Commit**: Save your changes. All UI edits are persisted to `elements.yaml` and `connectors.yaml`. Commit these to Git to version your architecture.


## Commands Reference 
`tld --help`

```text
Usage:
  tld [command]

CRUD actions on resources:
  add         Add or update an element in elements.yaml
  connect     Add a connector between two elements
  remove      Remove workspace resources
  rename      Rename an element in elements.yaml
  update      Update a resource field with a value

Secondary actions:
  analyze     Extract symbols from source files and upsert them as workspace elements
  apply       Apply plan to the tldiagram.com
  check       Check workspace health and diagram freshness
  completion  Generate the autocompletion script for the specified shell
  diff        Show differences between local workspace and server
  export      Export all diagrams from an organization to the local workspace
  help        Help about any command
  init        Initialize a new tld workspace
  login       Authenticate the CLI with a tlDiagram server
  plan        Show what would be applied
  pull        Pull the current server state into local YAML files
  serve       Start the local tlDiagram web server
  status      Show sync status between local YAML and the server
  stop        Stop the local tlDiagram web server
  validate    Validate the workspace YAML files
  version     Print the version number of tld
  views       Show derived view structure for the workspace

Flags:
      --compact            compact JSON output (no whitespace)
      --format string      output format: text or json (default "text")
  -h, --help               help for tld
  -v, --version            version for tld
  -w, --workspace string   workspace directory (prefers .tld, then tld; empty when neither exists)

Use "tld [command] --help" for more information about a command

```

## Workspace Structure

- `.tld.yaml`: Project settings and exclusions.
- `elements.yaml`: Definitions for all components and their placements.
- `connectors.yaml`: Connection and relationship definitions.
- `.tld.lock`: Tracks sync state and versioning.



## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `TLD_ADDR` | Host address to bind the server. | `127.0.0.1` |
| `PORT` | Port for the web UI and API. | `8060` |
| `TLD_API_KEY` | API key for cloud synchronization. | - |

see `tld config list` for the full list of configuration options.

`tld config path` shows the path to the current configuration file.
