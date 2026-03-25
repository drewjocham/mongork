# mongork Desktop

A desktop application for managing MongoDB migrations via [mongork](https://github.com/drewjocham/mongork), built with [Wails](https://wails.io) (Go backend + Vue 3 frontend).

## Features

- **Connection** — connect to any MongoDB instance with optional credentials
- **Migrations** — view status, run Up/Down with dry-run preview, create new migration files
- **Opslog** — search and filter applied migration history by text, version, regex, and date range
- **Oplog** — inspect raw MongoDB oplog entries
- **Schema Diff** — compare registered indexes/validators against the live database
- **Health** — view database role, oplog window, connection counts, and replication lag
- **MCP** — start/stop the MCP server for AI-assisted operations (Claude, Ollama, Goose)

## Prerequisites

- [Go 1.25+](https://go.dev)
- [Node.js 18+](https://nodejs.org)
- [Wails CLI v2](https://wails.io/docs/gettingstarted/installation): `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

## Development

```bash
wails dev
```

This starts a Vite dev server with hot-reload. The app is also reachable in a browser at `http://localhost:34115` for devtools access to Go methods.

## Build

```bash
wails build
```

Produces a native binary at `build/bin/mongork-desktop`.

## Configuration

The app connects to MongoDB using the URL, database name, and optional credentials entered in the **Connection** tab. No config file is required — all settings are managed at runtime.
