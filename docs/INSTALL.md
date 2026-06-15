# Installation and Configuration Guide

## Building from Source

### Prerequisites

- Go 1.22 or later
- `git` (to clone the repository)

### Build

```bash
git clone https://github.com/xlionjuan/wastebin-mcp-go.git
cd wastebin-mcp-go

# Build all packages
go build ./...

# Build a single binary
go build -o wastebin-mcp-go .
```

The resulting binary is a standalone executable with no runtime dependencies.

### Install with Version

```bash
go build -ldflags="-X main.version=$(git describe --tags --always)" -o wastebin-mcp-go .
```

### Verify

```bash
./wastebin-mcp-go --version
```

---

## Environment Variables Reference

| Variable | Required | Default | Description |
|---|---|---|---|
| `WASTEBIN_SERVER_URL` | ✅ | — | Wastebin server URL (e.g. `https://bin-staging.xlion.tw`) |
| `WASTEBIN_MCP_FILE_READ_ENABLED` | | `true` | Enable file-reading mode; set to `false` to restrict to inline content only |
| `WASTEBIN_MCP_DEFAULT_EXPIRES` | | `31536000` | Default paste expiration in seconds when no `expires` parameter is given |
| `WASTEBIN_MCP_ALLOWED_PATHS` | | — | Comma-separated absolute directory paths allowed for file reads. When set, only paths under these directories are accepted. When empty, skips allowlist and falls through to blocklist checks |
| `WASTEBIN_MCP_BLOCKED_PATHS` | | — | Comma-separated absolute directory paths for **user-defined** blocklist entries (e.g. `/home/user/secret`). Applied after the built-in blocklist. Empty by default |
| `WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST` | | `false` | Set to `true` to disable the built-in blocklist (system directory prefixes + sensitive path components). Use with caution |
| `WASTEBIN_MCP_MAX_CONTENT_SIZE` | | `1048576` | Maximum paste content size in bytes (client-side guard) |
| `WASTEBIN_MCP_SANDBOX_MOUNTS` | | — | Docker-style mount mappings (`host_path:sandbox_path,...`) for sandbox path translation |
| `WASTEBIN_MCP_SANDBOX_TRANSPARENT` | | `false` | When set, sandbox path translation happens automatically |
| `DEBUG` | | — | Set to `1` or `true` to enable debug logging (HTTP request/response details on stderr) |

### Invalid Environment Variable Values

When an environment variable cannot be parsed (e.g. invalid boolean, negative
number), the server prints a clear error message to stderr and exits with a
non-zero status. This applies to both MCP mode and CLI mode.

---

## Running in MCP Mode

MCP mode is activated when no subcommand is given. The server reads
configuration from environment variables, then starts a stdio JSON-RPC server
implementing the Model Context Protocol.

### Stdio Transport

The server communicates via stdin/stdout. MCP clients (AI agent frameworks) are
responsible for launching the process and providing the MCP `initialize` message
on stdin before tool calls become available.

**Stdin validation:** Before starting the server, the binary reads the first
line of stdin and verifies it is a valid JSON-RPC 2.0 `initialize` message
(maximum 1 MB). If the input is not a valid MCP initialize message, the server
prints an error to stderr and exits immediately — this prevents the process
from hanging when piped non-MCP input.

**Logging:** All server-side logging (info, warnings, errors) goes to stderr.
Tool results (JSON) are written to stdout.

### MCP Client Configuration Example

```json
{
  "mcpServers": {
    "wastebin": {
      "command": "/path/to/wastebin-mcp-go",
      "env": {
        "WASTEBIN_SERVER_URL": "https://bin-staging.xlion.tw",
        "WASTEBIN_MCP_FILE_READ_ENABLED": "true",
        "WASTEBIN_MCP_ALLOWED_PATHS": "/home/user/documents",
        "WASTEBIN_MCP_BLOCKED_PATHS": "/etc,/proc,/sys,/dev",
        "WASTEBIN_MCP_DEFAULT_EXPIRES": "31536000",
        "WASTEBIN_MCP_MAX_CONTENT_SIZE": "1048576"
      }
    }
  }
}
```

### Direct Invocation (Testing)

```bash
# Start the server directly
export WASTEBIN_SERVER_URL=https://bin-staging.xlion.tw
wastebin-mcp-go
```

> **Note:** When started without a proper MCP client, the server will reject
> the input (since stdin is not a valid `initialize` message) and exit.
> Use a proper MCP client or the CLI mode for one-shot pastes.

---

## Running in CLI Mode

CLI mode is activated by the `create` subcommand. It reads configuration from
the environment (same env vars as MCP mode), executes a one-shot paste creation,
and prints the JSON result to stdout.

```bash
# Inline content
wastebin-mcp-go create --content "hello world" --extension md

# File content
wastebin-mcp-go create --file-path /tmp/doc.md

# With all options
wastebin-mcp-go create \
  --content "my paste content" \
  --extension py \
  --title "My Snippet" \
  --expires 7d \
  --burn-after-reading \
  --password "secret123"

# Debug mode
wastebin-mcp-go create --content "test" --debug
```

### CLI Flags

| Flag | Description |
|---|---|
| `--content TEXT` | Paste content (provide this or `--file-path`, not both) |
| `--file-path PATH` | Read content from local file |
| `--extension EXT` | Syntax highlighting extension (e.g. `md`, `go`, `py`) |
| `--expires DURATION` | Expiration: bare number = seconds, or with unit suffix (`s`, `m`, `h`, `d`, `w`, `M`, `y`) |
| `--title TEXT` | Optional paste title |
| `--burn-after-reading` | Delete paste after first read |
| `--password TEXT` | Encrypt paste with password |
| `--debug` | Enable debug logging |
| `--help` | Show help message |
| `--version` | Show version information |

### Exit Codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | CLI error / invalid arguments |
| `2` | MCP server error |

---

## Security Configuration Notes

### File Read Mode (Enabled by Default)

File read mode allows the `create_paste` tool to read file contents from the
local filesystem. **This is enabled by default** — configure path restrictions
before using in any deployment where agents have access to the tool.

> **⚠️ Sandbox users:** When file read mode is enabled without path
> restrictions, an agent inside a container/sandbox can read any file accessible
> from its perspective. This is effectively **sandbox escape**. Always configure
> `ALLOWED_PATHS` and review the `BLOCKED_PATHS` defaults.

### Path Allowlist

Set `WASTEBIN_MCP_ALLOWED_PATHS` to a comma-separated list of absolute
directory paths. Every file read is validated against this list — the resolved
path must be within one of the allowed directories. When file read mode is
enabled and `ALLOWED_PATHS` is empty, the server skips the allowlist check and
falls through to the blocklist pipeline.

**ALLOWED_PATHS bypasses all blocklists.** If the resolved path is under an
allowed directory, it is accepted immediately — neither the built-in nor the
user-defined blocklist is consulted. This prevents blocklists from interfering
with legitimate file reads in configured directories.

### Path Blocklist (Built-in + User-defined)

The system uses a **two-tier blocklist**:

1. **Built-in blocklist** (hardcoded defaults, disabled via
   `WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST=true`):
   - *System directory prefixes*: `/etc`, `/proc`, `/sys`, `/dev`
   - *Sensitive path components*: `.ssh`, `.gnupg`, etc.
2. **User-defined blocklist** (`WASTEBIN_MCP_BLOCKED_PATHS`, comma-separated
   absolute directory paths). Applied after the built-in blocklist.

Each tier produces a distinct error message so the user knows exactly which
rule rejected their path.

### Path Traversal Protection

Paths containing `..` or path traversal equivalents are rejected **before**
any path resolution occurs. This prevents `../` from being used to reach
sensitive paths even when the final resolved path would pass blocklist checks.

All paths are resolved via `filepath.EvalSymlinks` and `filepath.Clean` before
validation, preventing symlink-based bypass of the allowlist or blocklist.

### Binary Detection

The server reads the first 8 KB of any file and applies content-based
heuristics to reject binary and non-UTF-8 files before uploading.

### Content Size Limit

A configurable maximum content size (`WASTEBIN_MCP_MAX_CONTENT_SIZE`, default
1 MB) is checked client-side before sending the HTTP request, preventing wasted
uploads for oversized content.

### Sandbox Path Translation

When using container/sandbox deployments with mount mappings
(`WASTEBIN_MCP_SANDBOX_MOUNTS`), internal paths can be translated to host paths
before file reading. Two modes:

- **Opt-in (default):** The `translate_sandbox_path` parameter appears in the
  tool schema. The caller must explicitly set it to `true`.
- **Transparent:** Set `WASTEBIN_MCP_SANDBOX_TRANSPARENT=true` to make
  translation automatic and remove the parameter from the schema.

Translated paths still pass through the allowlist and blocklist checks. Mount
host paths are validated against `ALLOWED_PATHS` at startup.

---

## Related Documentation

- [README.md](../README.md) — Quick start and usage overview
- [MCP Tools Reference](MCP_TOOLS.md) — Full tool parameters and response format
- [Architecture Decision Records](adr/) — Key design decisions
