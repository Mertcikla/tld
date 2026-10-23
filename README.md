<p align="center">
  <a href="https://tldiagram.com">
    <img src="./frontend/logo/tld.svg" alt="Logo" width="200">
  </a>
</p>

<p align="center">
<img width="900" height="900" alt="explore" src="https://github.com/user-attachments/assets/4a85bdda-b51f-4c4f-946f-30ba5dacd26a" />
</p>


`tld` is an opinionated, flexible diagramming tool with rich featureset to help you visualize, understand, and maintain your software architecture. Inspired by C4 model, designed with multiple opt-in features to answer evolving needs of software teams. 

<p align="center">
  <a href="https://go.dev/"><img src="https://img.shields.io/github/go-mod/go-version/mertcikla/tld" alt="Go Version"></a>
  <a href="./LICENSE"><img src="https://img.shields.io/github/license/mertcikla/tld" alt="License"></a>
  <a href="https://github.com/mertcikla/tld/actions"><img src="https://img.shields.io/github/actions/workflow/status/mertcikla/tld/test.yml?branch=main" alt="Build Status"></a>
  <a href="https://deepwiki.com/Mertcikla/tld"><img src="https://deepwiki.com/badge.svg" alt="Ask DeepWiki"></a>
</p>


## Highlights

- **Markdown Notes Support**: Add notes and documentation for your diagram or link an existing one, preview and edit diagrams and markdown side-by-side. 
- **Mermaid Integration**: Easily sync your diagrams with mermaid blocks in a markdown file for portable viewing in any markdown viewer [docs](https://tldiagram.com/docs/ui/mermaid/).
  
<p align="center">
<img width="900" height="506" alt="export-1792291239238" src="https://github.com/user-attachments/assets/bce6a3b5-fec5-44d7-a685-0710edf704dc" />
</p>


- **Standalone Distribution**: A single, dependency-free binary containing both the server and the web application. Available as CLI + WebUI or Native app(windows and macOS).
- **CLI that speaks agent**: Use the [agent skill](./skills/create-diagram/SKILL.md) and use your agent to create a diagram of your codebase with the exact detail level you need. Prompt the agent to add/remove details you see fit. 
Here are some examples that were generated using the agent skill.

  - [Kubernetes](https://tldiagram.com/app/explore/shared/827bc17d-7d9b-411f-9d03-179fab99bcbd)

  - [pytorch](https://tldiagram.com/app/explore/shared/abc56a26-3e20-4235-90fa-e045b2b2ac74)
 
  - [kafka](https://tldiagram.com/app/explore/shared/9d415d7f-b91f-47c0-9dc7-756de2860695)

  - [.NET eShop reference](https://tldiagram.com/app/explore/shared/ba6cbf2a-e0ff-468a-87e5-f720d35f448d)

- **Editor and Github Integration**: Jump to the code in your editor or Github from diagrams, or open the code symbol in diagram from your editor to visualize the code using the [VSCode extension](https://marketplace.visualstudio.com/items?itemName=tlDiagram-com.tldiagram). 

- **Bi-directional Sync**: Seamlessly sync changes between your local YAML files, the self-hosted web UI, and the cloud version at tlDiagram.com.
- **Git diff visualization**: Sync and visualize the changes you or your agent are making live in diagram form. Inspect the dependencies and intervene when necessary.
- **Codebase Mapping**: Generate source grounded elements to jump-start your diagramming process. 
- **Visualize Code Changes**: Visualize the changes between commits or a PR [docs](./docs/git-compare.md).
- **PR Diagram Bot**: Automatically generate a diagram for your PR and visualize the changes in your codebase [docs](https://github.com/Mertcikla/tld-pr-bot).
- **Diagrams as Code**: Manage architectural evolution alongside your source code.


- **UI**: A frontend optimized to handle complex architectures, with intuitive design and polished with tools to manage contextual views.

<p align="center">
<img width="1280" height="882" alt="editor" src="https://github.com/user-attachments/assets/4ee283b0-ee45-4a04-86e4-f477c26e9977" />
</p>

## Quick Start


macOS and Linux
```bash
curl -LsSf https://tldiagram.com/install.sh | sh -s serve --open
```

Windows
```powershell
powershell -ExecutionPolicy ByPass -c "irm https://tldiagram.com/install.ps1 | iex; tld serve --open"
```

By default, the web UI runs at [http://127.0.0.1:8060](http://127.0.0.1:8060). `tld serve` starts the server in the background; use `tld status` to inspect it and `tld stop` to shut it down. Run `tld serve --open` to start it again.

### Desktop App

Native desktop builds for macOS and Windows are available on the [releases page](https://github.com/Mertcikla/tld/releases). Look for assets named `tld-desktop-*`. CLI archives for macOS, Linux, and Windows are available there too.


## Deployment & Self-Hosting

Run `tld serve` for a local SQLite-backed instance. Configuration lives in `~/.config/tldiagram/tld.global.yaml` by default; `TLD_CONFIG_DIR` overrides the configuration directory, and `TLD_DATA_DIR` overrides the directory for local data and logs.

To use PostgreSQL, configure `database.driver` and `database.url` in the global config, or set environment variables.

```bash
export TLD_DB_DRIVER=postgres
export TLD_DATABASE_URL='postgres://user:pass@postgres:5432/tld?sslmode=require'
export TLD_PUBLIC_URL='https://example.com'
export TLD_HOST=127.0.0.1
export PORT=8060

tld serve
```

`TLD_HOST` controls the bind address, and `TLD_PUBLIC_URL` sets the public URL used behind a reverse proxy. The example binds to loopback for a proxy on the same host. Use `tld serve --foreground` when a service manager needs to supervise the process.

### Docker

From a checkout of this repository, start one of the supplied Compose stacks:

```bash
# SQLite
docker compose up -d

# Or PostgreSQL
docker compose -f docker-compose.postgres.yml up -d
```

Both stacks expose the web UI at [http://127.0.0.1:8060](http://127.0.0.1:8060) by default and persist data in named volumes. The PostgreSQL stack uses the standard `postgres:16` image, with no extensions required. See [the SQLite Compose file](./docker-compose.yml) or [the PostgreSQL Compose file](./docker-compose.postgres.yml) for image, port, and storage settings.

## Mobile
There are Mobile apps available in both [App Store](https://apps.apple.com/us/app/tldiagram/id6760236883) and [Play Store](https://play.google.com/store/apps/details?id=com.mertcikla.tldiagram) they are mostly cloud-oriented and free but require [tldiagram.com](https://tldiagram.com) account. They are good for quickly checking stuff on-the-go, small screens are hard for full featured diagram authoring but they still received some development effort and attention to make authoring possible on mobile. 

## CLI Reference

The CLI is built to enable LLM integration and use in CI/CD pipelines. It is a secondary interface to the web UI, and can be used to create, update, and inspect diagrams. 

Run `tld --help` for the authoring commands and `tld --help-all` for the full command catalog. Use `tld <command> --help` for fields, flags, and examples.

```bash
tld context                       # Show the selected workspace and target
tld list elements                 # Element refs, names, and database IDs
tld list connectors               # Keys for inspecting and updating connectors
tld list views --tree             # View owners and drill-down hierarchy
tld update element --help         # Editable fields, name changes, and YAML ref changes
tld update connector --help       # Editable fields, connector keys, and direction values
tld view --help                   # Create and manage an element's owned view
tld render root                   # Export the root view as Mermaid
tld doctor                        # Check source-indexing prerequisites
tld index --help                  # Index a repository, generate a map of your codebase, or watch changes live
tld git compare --help            # Compare Git revisions in a repository and visualize the changes on your diagrams
tld mcp --help                    # Expose diagram tools to MCP clients over stdio
```
[User guide and CLI reference](https://tldiagram.com/docs/tld/workspace/) for more information.

## Building & Development

Requires Go 1.26.2 or newer, Node.js 24, npm, and Make.

```bash
git clone https://github.com/Mertcikla/tld.git
cd tld
make build
"$(go env GOPATH)/bin/tld" serve --open
```

`make build` installs frontend dependencies, builds the web UI, and embeds it in
the binary at `$(go env GOPATH)/bin/tld`. Build the frontend before compiling Go
directly: the embedded assets come from `frontend/dist`.


For live development, install [Air](https://github.com/air-verse/air)

```bash
go install github.com/air-verse/air@latest
make frontend-build
make dev
```
Development data is stored in
`data/dev`, with configuration in `data/dev/config`.

Run checks from the repository root:

```bash
make lint-be   # Go lint
make lint-fe   # TypeScript lint
make test      # Go and frontend unit tests
make test-e2e  # Playwright end-to-end tests; requires installed browser binaries
```

The Go module is `github.com/mertcikla/tld/v2`, and the reusable frontend is
published as `@tldiagram/core-ui`.

## License

[Apache License 2.0](./LICENSE).
