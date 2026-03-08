# mongork

A lightweight MongoDB migration engine, CLI, and MCP-ready toolset that lets you keep migrations, change streams, and scripted operations in sync with your clusters. The binary is exposed as `mongo` (aliases `mmo`, `mt`, and `mmt`) so you can run your status, up/down, and oplog commands with a single executable.

## Highlights
- **Stateful Migration Engine:** Distributed locking, checksum validation, and smooth `Up`/`Down` flows keep the schema registry in sync across teams.
- **Mongo CLI:** `mongo` wraps the migration engine plus schema, oplog, and MCP helpers. The `oplog --follow --resume-file` flag saves resume tokens to disk so you never lose events during network hiccups.
- **Bubble Tea console:** `mongo ui` gives you an interactive dashboard for migration timeline, live stream events, MCP activity, and playbook checkpoints.
- **MCP Bridge:** The CLI can run as an MCP server (`mongo mcp --with-examples`) so AI agents can query migration status or apply work for you.
- **Zero-downtime playbooks:** `examples/practical/...` ships multi-stage expand/contract scenarios (add column, rename field, split collection, catalog backfill) complete with batching, checkpoints, and README guidance.

## Installation
1. **Homebrew (macOS/Linux)** – `brew tap drewjocham/mongork` then `brew install mongo`.
2. **Docker** – `docker pull ghcr.io/drewjocham/mongork:latest` and run `docker run --rm -v "$(pwd)":/workspace ghcr.io/drewjocham/mongork:latest --help`.
3. **Go tooling (development)** – `go install github.com/drewjocham/mongork/cmd@latest`.

Need more setup help? See [install.md](install.md).

## Quick Start
1. Copy `.env.example` to `.env` and configure `MONGO_URL`, credentials, and overrides for `MONGO_DATABASE`/`MIGRATIONS_COLLECTION`.
2. Run `mongo status`, `mongo up`, or `mongo down --target <version>` to inspect and evolve your schema.
3. Tail migrations with `mongo oplog --follow --resume-file /tmp/oplog.token`. The CLI keeps the last seen resume token on disk so reconnects never skip an oplog gap.
4. Start the MCP endpoint with `mongo mcp` (add `--with-examples` to seed the sample migrations).
5. Launch the Bubble Tea interface with `mongo ui` (aliases: `mongo tui`, `mongo bubbletea`).

## How to Use
### 1) Run migrations safely
- Check current state:
  - `mongo status`
- Preview changes before execution:
  - `mongo up --dry-run`
  - `mongo down --dry-run --target <version>`
- Apply or roll back:
  - `mongo up`
  - `mongo down --target <version>`

### 2) Tail live changes without losing position
- Start live stream with persisted resume token:
  - `mongo oplog --follow --resume-file /tmp/oplog.token`
- Restarting the same command resumes from the saved token.

### 3) Use the interactive Bubble Tea dashboard
- Launch:
  - `mongo ui --resume-file .mongork.resume`
- Global controls:
  - `TAB` / `SHIFT+TAB` switch tabs
  - `q` quits
- Migrations tab:
  - `↑/↓` select a migration
  - `r` starts rollback confirmation for selected applied migration
  - `y` confirms rollback, `n`/`esc` cancels
- Live Stream tab:
  - `↑/↓` select events
  - `p` pause/resume stream updates
  - `i`/`u`/`d` toggle insert/update/delete filters
  - `enter` toggles JSON inspector modal
- Playbook tab:
  - `K` sets the stop signal in `migration_control`

### 4) Start MCP mode for AI tooling
- Start MCP server:
  - `mongo mcp`
- Seed examples for experimentation:
  - `mongo mcp --with-examples`

## CLI Overview
| Command | Purpose |
| --- | --- |
| `mongo status` | Show migration state and timestamps. |
| `mongo up` | Apply pending migrations (use `--dry-run` to preview). |
| `mongo down` | Roll back migrations (`--target` limits how far). |
| `mongo create <name>` | Scaffold a new migration stub. |
| `mongo oplog` | Query and tail change stream events (use `--resume-file` to persist tokens). |
| `mongo ui` | Open the interactive Bubble Tea dashboard for migrations, stream activity, and playbook state. |
| `mongo schema indexes` | Print the schema indexes registered in Go. |
| `mongo schema diff` | Compare registered indexes/validators against live MongoDB. |
| `mongo mcp` | Start the Model Context Protocol server. |

## Architectural Toolbox
- **The Engine** manages distributed locks, applies migrations via registered `migration.Migration` implementations, and tracks versions in Mongo's migrations collection.
- **The Processor** in `cmd/examples` and `internal/mcp` shows how to batch scripted work such as `ReassignAssets`.
- **The CLI** exposes those capabilities, resumes oplog tails with disk-backed tokens, and serves an MCP endpoint for AI tooling.

## Documents
- **Top-level doc** – [`doc.go`](doc.go) contains the narrative that feeds the Documents section on [pkg.go.dev](https://pkg.go.dev/github.com/drewjocham/mongork). Keep it updated whenever you add concepts so the generated doc section stays accurate.
- **Library Guide** – [`library.md`](library.md) shows how to embed the migration engine in another Go project.
- **MCP Guide** – [`mcp.md`](mcp.md) explains how to wire the MCP server and register AI tools.
- **MCP Architecture** – [`docs/mcp-architecture.md`](docs/mcp-architecture.md) documents the MCP lifecycle, lock management, and failure recovery diagram.
- **Contributing & tests** – [`contributing.md`](contributing.md) documents workflows, release steps, and how to run `make lint`, `make test`, and `make integration-test`.
- **Installation** – [`install.md`](install.md) details platform-specific installs, Docker compose setups, and every supported entry point.

## Support & Community
- Issues/feature requests: [github.com/drewjocham/mongork/issues](https://github.com/drewjocham/mongork/issues)
- RFCs & discussions: [github.com/drewjocham/mongork/discussions](https://github.com/drewjocham/mongork/discussions)
- Example migrations: `internal/migrations/`

## License
MIT. See [LICENSE](LICENSE).
