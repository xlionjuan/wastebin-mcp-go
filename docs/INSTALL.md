# Installation and Configuration Guide

## Building from Source

### Prerequisites

- Go 1.26.4 or later
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
| `WASTEBIN_SERVER_URL` | ✅ | — | HTTP(S) Wastebin server URL, optionally with a base path (e.g. `https://bin-staging.xlion.tw/wastebin`). Credentials, query strings, and fragments are rejected. When using `http://` with a password, the password is sent in cleartext — prefer `https://` for production use. Password-protected pastes over non-loopback HTTP are rejected unless `WASTEBIN_MCP_ALLOW_INSECURE_PASSWORD=true` is set. |
| `WASTEBIN_MCP_FILE_READ_ENABLED` | | `true` | Enable file-reading mode; set to `false` to restrict to inline content only |
| `WASTEBIN_MCP_DEFAULT_EXPIRES` | | `31536000` | Default paste expiration in seconds when no `expires` parameter is given |
| `WASTEBIN_MCP_ALLOWED_PATHS` | | — | Comma-separated absolute directory paths allowed for file reads. Relative entries are rejected at startup. When set, only paths under these directories are accepted. When empty, skips allowlist and falls through to blocklist checks |
| `WASTEBIN_MCP_BLOCKED_PATHS` | | — | Comma-separated absolute directory paths to block for file reads (e.g. `/home/user/secret`). Relative entries are rejected at startup. Each entry stores both a lexical form (as configured) and a resolved form (after `EvalSymlinks`, with fallback to `Clean` for non-existent paths). A pre-resolution lexical check on the absolute request path catches late-created or retargeted symlinks, including relative `file_path` values in CLI mode. A post-resolution resolved check catches access through the canonical target. Empty by default — the built-in blocklist handles system directories (`/etc`, `/proc`, `/sys`, `/dev`) separately |
| `WASTEBIN_MCP_ALLOW_INSECURE_PASSWORD` | | `false` | Allow password-protected pastes over non-loopback HTTP connections. By default, password-protected pastes are rejected when using `http://` with a non-loopback host. Loopback addresses (localhost, 127.0.0.1, ::1) are always allowed with a warning |
| `WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST` | | `false` | Set to `true` to disable the built-in blocklist (system directory prefixes + sensitive path components). When enabled, only the user-configured blocklist (`WASTEBIN_MCP_BLOCKED_PATHS`) and allowlist (`WASTEBIN_MCP_ALLOWED_PATHS`) apply. Since `WASTEBIN_MCP_BLOCKED_PATHS` is empty by default, this flag alone (without explicit blocklist or allowlist) allows all paths. Use with caution |
| `WASTEBIN_MCP_MAX_CONTENT_SIZE` | | `1048576` | Maximum paste content size in bytes (client-side guard). Also determines the stdio first-message transport bound (this value plus a 64 KiB JSON envelope/escaping allowance) — see [Stdio Transport](#stdio-transport). Values above `268435456` (256 MiB) are rejected |
| `WASTEBIN_MCP_SANDBOX_MOUNTS` | | — | Docker-style mount mappings (`host_path:sandbox_path,...`) for sandbox path translation. Sandbox paths must be absolute POSIX paths and must not contain `..` components. **MCP mode only** — the CLI rejects this variable (sandbox path translation is not supported in CLI mode) |
| `WASTEBIN_MCP_SANDBOX_TRANSPARENT` | | `false` | When set, sandbox path translation happens automatically. **MCP mode only** — the CLI rejects this variable |
| `DEBUG` | | — | Set to `1` or `true` to enable debug logging (sparse `slog.Debug` entries on stderr — paste submission info, sandbox translation, error details; not an HTTP wire dump) |

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
responsible for launching the process and providing the first MCP request — the
session starter — on stdin before tool calls become available.

**Stdin validation:** Before starting the server, the binary reads the first
line of stdin and verifies it is a valid JSON-RPC 2.0 MCP session starter —
the legacy `initialize` handshake, the stateless `server/discover` RPC, or any
request carrying the per-request stateless protocol metadata
(`params._meta.io.modelcontextprotocol/protocolVersion`, protocol version
`2026-07-28`, SEP-2575). The stateless protocol has no handshake, so a first
request such as `tools/list` or `tools/call` is accepted and its metadata is
validated by the MCP SDK. If the input is not a valid MCP session starter,
the server prints an error to stderr and exits immediately — this prevents the
process from hanging when piped non-MCP input.

The first-line bound is a **transport limit**, not a content validator. It is
derived from the configured paste content limit: `WASTEBIN_MCP_MAX_CONTENT_SIZE`
plus a fixed 64 KiB allowance for the JSON-RPC envelope and JSON escaping
(`mcpFirstMessageEnvelopeAllowance`). The configured content size itself is
capped at 256 MiB (`MaxContentSizeLimit`); larger values are rejected at
startup, so the derived bound and its integer arithmetic can never overflow
(including on 32-bit targets). The gate reads the first line incrementally with
a small fixed buffer and accumulates at most `limit + 1` bytes (bounded memory)
before rejecting an oversized first line, so the configured maximum is never
allocated up front.

This keeps the gate consistent with the valid payload contract: a first-request
`tools/call` carrying content up to the configured limit (typical escaping)
passes, instead of failing only because it is the first message. Content that
requires heavy JSON escaping (many quotes, control characters, or HTML
metacharacters) expands the wire representation and lowers the effective
first-call content ceiling accordingly — the gate enforces a wire-size bound,
so heavily escaped content can be rejected as a first request even when its
decoded size is well below the configured limit. Full content and metadata
validation is left to the MCP SDK and the `create_paste` handler.

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
        "WASTEBIN_MCP_BLOCKED_PATHS": "/home/user/secret",
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
> the input (since stdin is not a valid MCP first message) and exit.
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

> **Note:** Sandbox path translation is an MCP-mode-only feature. The CLI
> (`wastebin-mcp-go create`) does not support it: setting
> `WASTEBIN_MCP_SANDBOX_MOUNTS` or `WASTEBIN_MCP_SANDBOX_TRANSPARENT` while
> running the CLI is rejected with a configuration error.

### CLI Flags

| Flag | Description |
|---|---|
| `--content TEXT` | Paste content (provide this or `--file-path`, not both) |
| `--file-path PATH` | Read content from local file |
| `--extension EXT` | Syntax highlighting extension (e.g. `md`, `go`, `py`). Normalization strips leading dots and lowercases; passing `.`, `...`, or empty input silently results in no extension |
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
falls through to the blocklist pipeline. Relative entries are rejected at
startup instead of being resolved against the process working directory.

**ALLOWED_PATHS bypasses the system directory prefix blocklist and the user
blocklist, but not the sensitive component blocklist.** If the resolved path
is under an allowed directory, it is accepted immediately — but sensitive path
components (`.ssh`, `.gnupg`, `.aws`, `.kube`, `.docker`, `.git`) are still
checked and rejected if found. This prevents blocklists from interfering with
legitimate file reads in configured directories while maintaining credential
protection.

### Path Blocklist (Built-in + User-defined)

The system uses a **two-tier blocklist**:

1. **Built-in blocklist** (hardcoded defaults, disabled via
   `WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST=true`):
   - *System directory prefixes*: `/etc`, `/proc`, `/sys`, `/dev`
   - *Sensitive path components*: `.ssh`, `.gnupg`, `.aws`, `.kube`, `.docker`, `.git`
2. **User-defined blocklist** (`WASTEBIN_MCP_BLOCKED_PATHS`, comma-separated
   absolute directory paths). Relative entries are rejected at startup.
   Each entry stores both the **lexical form** (as configured, `filepath.Clean`)
   and the **resolved form** (after `filepath.EvalSymlinks`, or falls back to
   `filepath.Clean` for non-existent paths). The lexical check (Stage 4a) runs
   **before** `EvalSymlinks` and the built-in blocklist; the resolved check
   (Stage 4b) runs **after** both. Empty by default — the built-in blocklist
   handles system directories separately.
   - **Lexical check (pre-resolution):** Before symlink resolution, the raw
     request path is converted to absolute via `filepath.Abs` and compared
     against the lexical entries. This catches access through a user-blocked
     directory that is created or retargeted as a symlink after process startup,
     including relative `file_path` values in CLI mode.
   - **Resolved check (post-resolution):** After `EvalSymlinks`, the canonical
     resolved path is compared against the resolved entries. This catches access
     through the symlink's target even when the symlink alias itself was not
     the configured path.

Each tier produces a distinct error message so the user knows exactly which
rule rejected their path.

### Path Traversal Protection

Paths containing `..` or path traversal equivalents are rejected **before**
any path resolution occurs. This prevents `../` from being used to reach
sensitive paths even when the final resolved path would pass blocklist checks.

Sensitive path components (`.ssh`, `.gnupg`, `.aws`, `.kube`, `.docker`,
`.git`) are also checked on the raw input **before** symlink resolution. This
catches symlinked blocked components (e.g. `.ssh` → `realssh`) at the earliest
possible stage, before `EvalSymlinks` resolves the symlink and the evidence
disappears. The same component check runs a second time on the resolved path
for defense in depth.

After the pre-resolution checks, all paths are resolved via
`filepath.EvalSymlinks` and `filepath.Clean` before validation, preventing
symlink-based bypass of the allowlist or blocklist. For defence in depth, the
actual file open uses `openat(2)` with `O_NOFOLLOW` to walk every path
component from a trusted root fd (`/`), preventing TOCTOU symlink-swap attacks
between validation and file open.

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

> **Note:** Sandbox path translation is an MCP-mode-only feature. The opt-in
> `translate_sandbox_path` parameter is only exposed in the MCP tool schema,
> and transparent mode applies translation only in MCP mode. CLI mode
> (`wastebin-mcp-go create`) does not support sandbox path translation:
> setting `WASTEBIN_MCP_SANDBOX_MOUNTS` or `WASTEBIN_MCP_SANDBOX_TRANSPARENT`
> while running the CLI is rejected with a configuration error.

---

## Related Documentation

- [README.md](../README.md) — Quick start and usage overview
- [MCP Tools Reference](MCP_TOOLS.md) — Full tool parameters and response format
- [Architecture Decision Records](adr/) — Key design decisions
