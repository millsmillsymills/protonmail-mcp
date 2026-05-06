# protonmail-mcp — Claude Code instructions

## Canonical MCP standards

Authoritative source: `~/Desktop/Projects/consistency-check/docs/standards/`. This repo is graded against `mcp.md`, `go.md`, and `mcp-protocol.md`.

Run the audit:

```bash
cd ~/Desktop/Projects/consistency-check
uv run consistency-check audit --repo protonmail-mcp
```

## Repo-specific notes

Project layout follows standard Go conventions: `cmd/protonmail-mcp/` for the entrypoint, `internal/` for protocol handlers, raw API client, error mapping, keychain, and session management.
