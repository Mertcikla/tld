# tlDiagram (`tld`)

[![Go Version](https://img.shields.io/github/go-mod/go-version/mertcikla/tld)](https://go.dev/) [![License](https://img.shields.io/github/license/mertcikla/tld)](./LICENSE) [![Build Status](https://img.shields.io/github/actions/workflow/status/mertcikla/tld/test.yml?branch=main)](https://github.com/mertcikla/tld/actions) [![Go Report Card](https://goreportcard.com/badge/github.com/mertcikla/tld)](https://goreportcard.com/report/github.com/mertcikla/tld)

`tld` provides a complete software architecture management platform that bundles a high-performance Go backend with an interactive React frontend into a single, standalone binary. Includes a CLI to enable managing diagrams from the shell or in CI.

Designed for local-first development and private self-hosting, `tld` allows teams to visualize, document, and manage their system architecture using a combination of a rich web UI and "Diagrams as Code" workflows.

---

## Key Features

- **Full-Featured Web UI**: A React frontend designed, polished and optimized to handle complex architectures while attempting to intelligently show and hide details.
- **Bi-directional Sync**: Seamlessly sync changes between your local YAML files, the self-hosted web UI, and the cloud version at tlDiagram.com.
- **Standalone Distribution**: A single, dependency-free binary containing both the server and the web application.
- **CLI built that speaks agent**: Use the [agent skill](./skills/create-diagram/SKILL.md) and teach your agent how to create a diagram of your codebase with the exact detail level you need. You can prompt your agent to add/remove details as needed. 
Here are some examples that were generated using the agent skill.

  - [Kubernetes](https://tldiagram.com/app/explore/shared/827bc17d-7d9b-411f-9d03-179fab99bcbd)

  - [pytorch](https://tldiagram.com/app/explore/shared/abc56a26-3e20-4235-90fa-e045b2b2ac74)
 
  - [kafka](https://tldiagram.com/app/explore/shared/9d415d7f-b91f-47c0-9dc7-756de2860695)

  - [.NET eShop reference](https://tldiagram.com/app/explore/shared/ba6cbf2a-e0ff-468a-87e5-f720d35f448d)

- **Diagrams as Code**: A Git-like workflow (`plan`/`apply`) to manage architectural evolution alongside your source code.
- **Automated Codebase Analysis**: (Preview) Built-in tree-sitter integration to automatically discover architecture components in Go, Java, Python, C++, and TypeScript (more soon™ (hopefully)).

---

## Table of Contents

1. [Quick Start](#quick-start)
2. [Deployment & Self-Hosting](#deployment--self-hosting)
3. [The tlDiagram Workflow](#the-tldiagram-workflow)
4. [Tech Stack](#tech-stack)
5. [Development Setup](#development-setup)
6. [Commands Reference](#commands-reference)
7. [Workspace Structure](#workspace-structure)
8. [Environment Variables](#environment-variables)
9. [Troubleshooting](#troubleshooting)

---

## Quick Start
### Single line install and start
```bash
curl -LsSf https://tldiagram.com/install.sh | sh -s serve --open
```

OR

### 1. Install the binary
```bash
curl -LsSf https://tldiagram.com/install.sh | sh
```

### 2. Launch the Web UI
Initialize a workspace and start the local server:
```bash
tld serve
```
Open **`http://localhost:8060`** to start visually mapping your architecture.

---

## Deployment & Self-Hosting

The `tld` binary is designed to be run as a persistent service in your infrastructure or as a local development tool.

### Local Development
Run `tld serve` in any directory to start a local instance that uses your current folder for storage. 

### Server Deployment
1. Provide a persistent volume for the `.tld/` directory (where YAMLs and the SQLite cache are stored).
2. Set `TLD_ADDR=0.0.0.0` and `PORT=8060`.

---

## The tlDiagram Workflow

`tld` bridges the gap between manual diagramming and automated documentation.

1. **Visualize**: Use `tld serve` to open the interactive UI. Drag, drop, and connect components.
2. **Automate**: Run `tld analyze` to scan your repository. It will suggest new elements and connectors based on your actual source code.
3. **Commit**: Save your changes. All UI edits are persisted to `elements.yaml` and `connectors.yaml`. Commit these to Git to version your architecture.

---

## Tech Stack

- **Backend**: Go 1.26+ 
  - *CLI*: Cobra
  - *API*: Connect RPC (gRPC compatible)
  - *Analysis*: Tree-sitter
  - *Database*: Embedded SQLite (`modernc.org/sqlite`)
- **Frontend**: React 18 & TypeScript
  - *Visualization*: ReactFlow, ElkJS (auto-layout), D3-force
  - *UI Components*: Chakra UI
- **Build System**: GoReleaser (for cross-platform standalone binaries)

---

## Development Setup

If you want to contribute to `tld` or build it from source:

  1. **Clone the Repo**:
   ```bash
   git clone https://github.com/Mertcikla/tld.git
   cd tld
   ```

2. **Install Frontend Dependencies**:
   ```bash
   make frontend-deps
   ```

3. **Development Mode (Hot Reloading)**:
   This starts the Vite dev server for the frontend and the Air reloader for the Go backend.
   ```bash
   make dev
   ```

4. **Production Build**:
   ```bash
   make build
   ```

---

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
  -w, --workspace string   workspace directory (default "tld")

Use "tld [command] --help" for more information about a command

```
---

## Workspace Structure

- `.tld.yaml`: Project settings and exclusions.
- `elements.yaml`: Definitions for all components and their placements.
- `connectors.yaml`: Connection and relationship definitions.
- `.tld.lock`: Tracks sync state and versioning.

---

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `TLD_ADDR` | Host address to bind the server. | `127.0.0.1` |
| `PORT` | Port for the web UI and API. | `8060` |
| `TLD_API_KEY` | API key for cloud synchronization. | - |

---

## Troubleshooting

- **"Server already running"**: Run `tld stop` to clear the PID file and shut down the background process.
- **UI not reflecting YAML changes**: Restart the server or ensure `tld serve` is running in the correct directory.
- **Language support**: If a language isn't detected, ensure the parser is registered in `internal/analyzer`.
