# Wastebin MCP Server

A Model Context Protocol (MCP) server and CLI tool for creating pastes on a
[Wastebin](https://github.com/matze/wastebin) instance.

## Quick Start

```bash
# Build
go build ./...

# MCP mode (stdio server for AI agents)
export WASTEBIN_MCP_SERVER_URL=https://bin-staging.xlion.tw
wastebin-mcp-go

# CLI mode
wastebin-mcp-go create --content "hello world" --extension md
wastebin-mcp-go create --file-path /tmp/doc.md
```

## Features

- **Content mode**: paste text directly
- **File mode** (default: on): paste from file path
- **Path safety**: allowlist + blocklist + symlink resolution
- **Sandbox translation** (optional, ENV-gated): translate container paths to host paths
- **Text validation**: rejects binary and non-UTF-8 files
- **CLI mode**: one-shot paste creation (`create` subcommand)
- **MCP mode**: stdio server for AI agents

## Configuration

| Environment Variable | Required | Default | Description |
|----------------------|----------|---------|-------------|
| `WASTEBIN_MCP_SERVER_URL` | ✅ | — | Wastebin server URL (e.g. `https://bin-staging.xlion.tw`) |
| `WASTEBIN_MCP_DEFAULT_EXPIRES` | | 31536000 | Default expiration in seconds |
| `WASTEBIN_MCP_FILE_READ_ENABLED` | | true | Enable file reading mode |
| `WASTEBIN_MCP_ALLOWED_PATHS` | | — | Comma-separated allowed directory paths |
| `WASTEBIN_MCP_BLOCKED_PATHS` | | `/etc,/proc,/sys,/dev` | Comma-separated blocked directory paths |
| `WASTEBIN_MCP_MAX_CONTENT_SIZE` | | 1048576 | Max content size in bytes |
| `WASTEBIN_MCP_SANDBOX_MOUNTS` | | — | Docker mount mappings (`host:sandbox,...`) |
| `WASTEBIN_MCP_SANDBOX_TRANSPARENT` | | false | Transparent sandbox translation |

## Response Format

```json
{
  "hostname": "https://bin-staging.xlion.tw",
  "id": "FTuutJssdSh",
  "url": "/FTuutJssdSh.md",
  "raw": "/raw/FTuutJssdSh.md",
  "markdown_rendered": "/md/FTuutJssdSh.md"
}
```

`markdown_rendered` is only present when extension is `.md`.

## Security

- File reads are gated by an **allowlist** (ALLOWED_PATHS) and a **blocklist**
  (BLOCKED_PATHS, defaults to `/etc,/proc,/sys,/dev`).
- All paths are resolved via `filepath.EvalSymlinks` before checking,
  preventing symlink-based bypass.
- Binary and non-UTF-8 files are rejected at read time.
- Sandbox translation is opt-in and ENV-gated.
- Content size is pre-checked against a configurable limit.
- See `CONTEXT.md` for the full security model.

## License

AGPL-3.0
