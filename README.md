<p align="center">
  <a href="https://tldiagram.com">
    <img width="200" alt="Logo" src="./frontend/logo/tld.svg" />
  </a>
</p>

`tld` is an opinionated, flexible diagramming tool with rich featureset to help you visualize, understand, and maintain your software architecture. Inspired by C4 model, designed with multiple opt-in features to answer evolving needs of software teams.

<p align="center">
  <a href="https://go.dev/">
    ![Go Version](https://img.shields.io/github/go-mod/go-version/mertcikla/tld)
  </a>

  <a href="./LICENSE">
    ![License](https://img.shields.io/github/license/mertcikla/tld)
  </a>

  <a href="https://github.com/mertcikla/tld/actions">
    ![Build Status](https://img.shields.io/github/actions/workflow/status/mertcikla/tld/test.yml?branch=main)
  </a>

  <a href="https://goreportcard.com/report/github.com/mertcikla/tld">
    ![Go Report Card](https://goreportcard.com/badge/github.com/mertcikla/tld)
  </a>

  <a href="https://deepwiki.com/Mertcikla/tld">
    ![Ask DeepWiki](https://deepwiki.com/badge.svg)
  </a>
</p>

<p align="center">
  <img height="882" width="1280" alt="editor" src="https://github.com/user-attachments/assets/4ee283b0-ee45-4a04-86e4-f477c26e9977" />
</p>

## Highlights

* **UI**: A frontend optimized to handle complex architectures, with intuitive design and polished with tools to manage contextual views.
* **Standalone Distribution**: A single, dependency-free binary containing both the server and the web application. Available as CLI + WebUI or Native app(windows and macOS).
* **CLI that speaks agent**: Use the [agent skill](./skills/create-diagram/SKILL.md) and use your agent to create a diagram of your codebase with the exact detail level you need. Prompt the agent to add/remove details you see fit.
  Here are some examples that were generated using the agent skill.
  * [Kubernetes](https://tldiagram.com/app/explore/shared/827bc17d-7d9b-411f-9d03-179fab99bcbd)
  * [pytorch](https://tldiagram.com/app/explore/shared/abc56a26-3e20-4235-90fa-e045b2b2ac74)
  * [kafka](https://tldiagram.com/app/explore/shared/9d415d7f-b91f-47c0-9dc7-756de2860695)
  * [.NET eShop reference](https://tldiagram.com/app/explore/shared/ba6cbf2a-e0ff-468a-87e5-f720d35f448d)
* **Editor and Github Integration**: Jump to the code in your editor or Github from diagrams, or open the code symbol in diagram from your editor to visualize the code using the [VSCode extension](https://marketplace.visualstudio.com/items?itemName=tlDiagram-com.tldiagram).
* **Mermaid Integration**: Paste your mermaid diagrams into canvas to import them or export as mermaid for quick sharing.
* **Markdown Notes Support**: Add notes and documentation for your diagram or link an existing one, preview and edit diagrams and markdown side-by-side.
* **Bi-directional Sync**: (Experimental) Seamlessly sync changes between your local YAML files, the self-hosted web UI, and the cloud version at tlDiagram.com.
* **Git diff visualization**: (Experimental) Sync and visualize the changes you or your agent are making live in diagram form. Inspect the dependencies and intervene when necessary.
* **Diagrams as Code**: (Experimental) A git/terraform like workflow (`plan`/`apply`) to manage architectural evolution alongside your source code.
* **Automated Codebase Analysis**: (Experimental) Built-in tree-sitter integration to automatically discover architecture components in Go, Java, Python, C++, and TypeScript (more soon™ (hopefully)).

<p align="center">
  <img height="720" width="1280" alt="explore" src="https://github.com/user-attachments/assets/0b194071-d411-449a-87bd-2d4883e9c354" />
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

## Documentation

Visit [docs](https://tldiagram.com/docs) for more info.

## Deployment & Self-Hosting

tld has native-desktop builds for macOS and windows. Look for tld-desktop binaries in releases.

`tld` designed to be run fully offline, behind a reverse-proxy or in your infrastructure or as a local development tool.

Run `tld serve` to start a local SQLite-backed instance, or configure PostgreSQL in `~/.config/tldiagram/tld.yaml` or via env vars

```bash
export TLD_DB_DRIVER=postgres
export TLD_DATABASE_URL='postgres://user:pass@postgres:5432/tld?sslmode=require'
export TLD_PUBLIC_URL='https://example.com'
export TLD_HOST=127.0.0.1
export PORT=8060

tld serve
```

The PostgreSQL database must have `pgvector` support.

## Mobile

There are Mobile apps available in both [App Store](https://apps.apple.com/us/app/tldiagram/id6760236883) and [Play Store](https://play.google.com/store/apps/details?id=com.mertcikla.tldiagram) they are mostly cloud-oriented and free but require [tldiagram.com](https://tldiagram.com) account. They are good for quickly checking stuff on-the-go, small screens are hard for full featured diagram authoring but they still received some development effort and attention to make authoring possible on mobile.

## Commands Reference

`tld --help`

or

Use `tld [command] --help` for more information about a command

```mermaid
flowchart LR
%% tld/v1 view=6
  node_125["API Gateway"]
%% tld-element ref=node_125 x=869.25 y=237.25 kind=service desc=REST API gateway tech=Go logo=/icons/go.svg tags=backend bypass=1 hasView=1 viewLabel=Component
  node_126["Auth Service"]
%% tld-element ref=node_126 x=524.25 y=117.25 kind=service desc=Handles authentication & sessions tech=Go logo=/icons/go.svg tags=backend bypass=1 hasView=1
  node_127["CDN"]
%% tld-element ref=node_127 x=524.25 y=517.25 kind=service desc=Content delivery network tech=Cloudflare logo=/icons/cloudflare.svg tags=external,infrastructure bypass=1
  node_128["User"]
%% tld-element ref=node_128 x=224.25 y=317.25 kind=person desc=End user of the system tech=User logo=/icons/user.svg tags=external bypass=1 hasView=1
  node_129["Web App"]
%% tld-element ref=node_129 x=524.25 y=317.25 kind=service desc=React single-page application tech=React logo=/icons/react.svg tags=frontend bypass=1 hasView=1 viewLabel=Container

  node_125 --> node_126
%% tld-connector ref=142 source=node_125 target=node_126 dir=both sourceHandle=top targetHandle=right
  node_129 -- "API calls" --> node_125
%% tld-connector ref=143 source=node_129 target=node_125 label=API calls
  node_129 -- "Auth" --> node_126
%% tld-connector ref=144 source=node_129 target=node_126 label=Auth sourceHandle=top targetHandle=bottom
  node_129 -- "Serves via" --> node_127
%% tld-connector ref=145 source=node_129 target=node_127 label=Serves via dir=backward sourceHandle=bottom targetHandle=top
  node_128 -- "Uses" --> node_129
%% tld-connector ref=146 source=node_128 target=node_129 label=Uses
```
