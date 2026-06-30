# Wastebin MCP Server

A Model Context Protocol (MCP) server and CLI tool for creating pastes on a
[Wastebin](https://github.com/matze/wastebin) instance.

## Quick Start

```bash
# Build
go build ./...

# MCP mode (stdio server for AI agents)
export WASTEBIN_SERVER_URL=https://bin-staging.xlion.tw
wastebin-mcp-go

# CLI mode
wastebin-mcp-go create --content "hello world" --extension md
wastebin-mcp-go create --file-path /tmp/doc.md
```

**Flags**: `--help` and `--version` are available as top-level flags. The `create` subcommand also accepts `--help`.

**Exit codes**: 0 = success, 1 = CLI error, 2 = MCP server error.

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
| `WASTEBIN_SERVER_URL` | ✅ | — | Wastebin server URL (e.g. `https://bin-staging.xlion.tw`) |
| `WASTEBIN_MCP_DEFAULT_EXPIRES` | | 31536000 | Default expiration in seconds |
| `WASTEBIN_MCP_FILE_READ_ENABLED` | | true | Enable file reading mode |
| `WASTEBIN_MCP_ALLOWED_PATHS` | | — | Comma-separated allowed directory paths |
| `WASTEBIN_MCP_BLOCKED_PATHS` | | `/etc,/proc,/sys,/dev` | Comma-separated blocked directory paths |
| `WASTEBIN_MCP_MAX_CONTENT_SIZE` | | 1048576 | Max content size in bytes |
| `WASTEBIN_MCP_SANDBOX_MOUNTS` | | — | Docker mount mappings (`host:sandbox,...`) |
| `WASTEBIN_MCP_SANDBOX_TRANSPARENT` | | false | Transparent sandbox translation |
| `WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST` | | false | Disable built-in blocklist (system + sensitive paths) |
| `DEBUG` | | — | Set to `1` or `true` to enable debug logging |

## Response Format

The response contains a fixed set of fields (`hostname`, `id`, `url`, `raw`) plus
conditional fields that appear only in specific scenarios.

#### Paste with `.md` extension

```json
{
  "hostname": "https://bin-staging.xlion.tw",
  "id": "FTuutJssdSh",
  "url": "/FTuutJssdSh.md",
  "raw": "/raw/FTuutJssdSh.md",
  "markdown_rendered": "/md/FTuutJssdSh.md"
}
```

#### Paste with unknown extension (or no extension)

```json
{
  "hostname": "https://bin-staging.xlion.tw",
  "id": "AbCdEfGh123",
  "url": "/AbCdEfGh123",
  "raw": "/raw/AbCdEfGh123",
  "hint": "Extension not detected; syntax highlighting may not apply"
}
```

#### Paste with password protection

```json
{
  "hostname": "https://bin-staging.xlion.tw",
  "id": "XyZ789AbCdE",
  "url": "/XyZ789AbCdE.txt",
  "raw": "/raw/XyZ789AbCdE.txt",
  "password_hint": "curl -H 'Wastebin-Password: <password>' https://bin-staging.xlion.tw/raw/XyZ789AbCdE.txt"
}
```


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
