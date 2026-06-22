

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
  <a href="https://goreportcard.com/report/github.com/mertcikla/tld"><img src="https://goreportcard.com/badge/github.com/mertcikla/tld" alt="Go Report Card"></a>
  <a href="https://deepwiki.com/Mertcikla/tld"><img src="https://deepwiki.com/badge.svg" alt="Ask DeepWiki"></a>
</p>


## Highlights

- **UI**: A frontend optimized to handle complex architectures, with intuitive design and polished with tools to manage contextual views.
- **Standalone Distribution**: A single, dependency-free binary containing both the server and the web application. Available as CLI + WebUI or Native app(windows and macOS).
- **CLI that speaks agent**: Use the [agent skill](./skills/create-diagram/SKILL.md) and use your agent to create a diagram of your codebase with the exact detail level you need. Prompt the agent to add/remove details you see fit. 
Here are some examples that were generated using the agent skill.

  - [Kubernetes](https://tldiagram.com/app/explore/shared/827bc17d-7d9b-411f-9d03-179fab99bcbd)

  - [pytorch](https://tldiagram.com/app/explore/shared/abc56a26-3e20-4235-90fa-e045b2b2ac74)
 
  - [kafka](https://tldiagram.com/app/explore/shared/9d415d7f-b91f-47c0-9dc7-756de2860695)

  - [.NET eShop reference](https://tldiagram.com/app/explore/shared/ba6cbf2a-e0ff-468a-87e5-f720d35f448d)

- **Editor and Github Integration**: Jump to the code in your editor or Github from diagrams, or open the code symbol in diagram from your editor to visualize the code using the [VSCode extension](https://marketplace.visualstudio.com/items?itemName=tlDiagram-com.tldiagram). 
- **Mermaid Integration**: Paste your mermaid diagrams into canvas to import them or export as mermaid for quick sharing.
- **Markdown Notes Support**: Add notes and documentation for your diagram or link an existing one, preview and edit diagrams and markdown side-by-side. 
- **Mermaid Integration**: Easily sync your diagrams with mermaid blocks in a markdown file for portable viewing in any markdown viewer. [Link to docs](https://tldiagram.com/docs/ui/mermaid/)
- **Bi-directional Sync**: (Experimental) Seamlessly sync changes between your local YAML files, the self-hosted web UI, and the cloud version at tlDiagram.com.
- **Git diff visualization**: (Experimental) Sync and visualize the changes you or your agent are making live in diagram form. Inspect the dependencies and intervene when necessary.
- **Diagrams as Code**: (Experimental) A git/terraform like workflow (`plan`/`apply`) to manage architectural evolution alongside your source code.
- **Automated Codebase Analysis**: (Experimental) Built-in tree-sitter integration to automatically discover architecture components in Go, Java, Python, C++, and TypeScript (more soon™ (hopefully)).

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
