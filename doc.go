// Package main exposes the mongork CLI binary as `mongo` (aliases: `mmo`, `mt`,
// and `mmt`) for MongoDB migration management and operational workflows.
//
// The CLI combines a stateful migration engine (distributed locking, checksum
// validation, up/down planning), oplog tooling with disk-backed resume tokens,
// an interactive Bubble Tea dashboard (`mongo ui`), and an MCP server endpoint
// (`mongo mcp`) for AI-assisted migration operations.
//
// See readme.md, install.md, library.md, mcp.md, and docs/mcp-architecture.md
// for usage guides and architectural details.
package main
