# Wastebin MCP Server (wastebin-mcp-go)

A Model Context Protocol (MCP) server and CLI tool that creates pastes on a
Wastebin pastebin instance via its REST API.

## Language

### Core Types

**WastebinClient**: The HTTP client that holds the server base URL and an
`*http.Client` and communicates with a Wastebin instance to create pastes.

**Config**: The configuration parameters — server URL, default expiration, file
read mode settings, sandbox mount mappings, and translation mode.

**CreatePasteArgs**: All input parameters for creating a paste in MCP mode —
content or file_path, extension, expires, title, burn_after_reading, password,
and optional sandbox path translation flag.

**PasteResponse**: The structured result returned after creating a paste —
hostname, paste id, and URL variants (raw, rendered).

**Blocklist**: A default set of sensitive system directories (`/etc`, `/proc`,
`/sys`, `/dev`) that are denied unless explicitly allowed via ALLOWED_PATHS.
The allowlist takes precedence — a path under a blocked directory IS accessible
if the user explicitly added it to ALLOWED_PATHS.

### Wastebin API

The upstream Wastebin server (`github.com/matze/wastebin`) provides:

- `POST /` with JSON body `{"text": "...", "extension?": "...", "expires?": N,
  "burn_after_reading?": bool, "password?": "...", "title?": "..."}`
  → returns `{"path": "/FTuutJssdSh"}`
- `GET /raw/:id` — raw content
- `GET /md/:id` — rendered Markdown (only for `.md`/`.markdown` pastes)
- `GET /dl/:id` — download with Content-Disposition

Default max body size: 1 MB (`WASTEBIN_MAX_BODY_SIZE`).
Default paste expirations: `0=d,10m,1h,1d,1w,1M,1y` (`WASTEBIN_PASTE_EXPIRATIONS`).

### Wastebin ID format

IDs are base64-like encoded integers (`a-zA-Z0-9-+`). They can be 6 characters
(Id32) or 11 characters (Id64). The ID is randomly generated server-side. The
response `path` includes the extension if one was provided.

```
POST / → {"path": "/FTuutJssdSh.md"}
```

### create_paste Tool

Single tool with two usage modes — content passthrough or file path.

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | no\* | Paste content directly |
| `file_path` | string | no\* | Local file path to read content from |
| `extension` | string | no | Syntax highlighting extension (e.g. md, go, py) |
| `expires` | string | no | Expiration: bare number = seconds, unit suffix (s/m/h/d/w/M/y) |
| `title` | string | no | Optional paste title |
| `burn_after_reading` | boolean | no | Delete after first read |
| `password` | string | no | Encrypt with password |
| `translate_sandbox_path` | boolean | no | Translate sandbox path (only when mounts configured) |

\* At least one of `content` or `file_path` is required. If both are provided,
the server returns a clear error: "Provide either 'content' or 'file_path', not both."

**Schema is built dynamically at startup based on env config:**

- If file read mode is OFF: `content` becomes required; `file_path` and
  `translate_sandbox_path` are excluded from the schema.
- If sandbox mounts are not configured: `translate_sandbox_path` is excluded.
- If transparent mode is ON: `translate_sandbox_path` is excluded (translation
  happens automatically).

### Expiration Format

Accepts two formats:
- Bare number (e.g. `3600`) → treated as seconds
- Number with unit suffix → translated to seconds

Supported units: `s` (seconds), `m` (minutes), `h` (hours), `d` (days),
`w` (weeks), `M` (months ≈ 30d), `y` (years ≈ 365d).

Examples: `3600`, `1h`, `7d`, `30d`, `1y`.

### Output Format

```json
{
  "hostname": "https://bin-staging.xlion.tw",
  "id": "FTuutJssdSh",
  "url": "/FTuutJssdSh.md",
  "raw": "/raw/FTuutJssdSh.md"
}
```

| Scenario | extension | markdown_rendered | hint |
|----------|-----------|-------------------|------|
| Content mode + ext=.md | from caller | ✅ | ❌ |
| Content mode + ext≠.md | from caller | ❌ | ❌ |
| Content mode + no ext | unset | ❌ | ✅ |
| File mode + .md extension | from path | ✅ | ❌ |
| File mode + non-.md ext | from path | ❌ | ❌ |
| File mode + no extension | unset | ❌ | ✅ |

- `id` is included for agents to construct custom URLs (e.g. `curl`).
- `markdown_rendered` only appears when extension is `.md` or `.markdown`.
- `hint` only appears when extension is unknown (fuzzy case).
- URLs are relative (`hostname` is separate). The tool description instructs
  agents to reconstruct full URLs as `{hostname}{url}`.

### Extension Detection (File Mode)

Uses Go's `filepath.Ext()` which returns the trailing extension only:
- `script.py` → `.py`, `archive.tar.gz` → `.gz`, `Dockerfile` → `""`
- For extensionless filenames (Dockerfile, Makefile), the `extension` parameter
  can be used explicitly.

Password-protected pastes: If you create a paste with a password, retrieval
requires providing the password via the `Wastebin-Password` header or the
`password` query parameter. Since there is no
`get_paste` tool, agents must use `curl -H "Wastebin-Password: ..." {hostname}/raw/{id}`
to retrieve it. This is a known limitation by design.

### Operation Modes

**MCP Mode**: The program starts a stdio JSON-RPC server implementing the Model
Context Protocol, listening for incoming `create_paste` tool-call requests and
returning structured JSON responses. Activated when no CLI subcommand is given.

**CLI Mode**: Activated by the `create` subcommand. Accepts paste parameters as
flags, executes a one-shot paste creation, and prints the result to stdout, then
exits. Output format: JSON (same as MCP mode response).

**Debug Mode**: Activated by `--debug` or `DEBUG=1` — logs HTTP request/response
details to stderr.

### Error Handling

When a paste creation fails, the error message is constructed as follows:

**Known errors translated to clear messages:**

| HTTP Error | Translated Message |
|------------|-------------------|
| 403 | "Server rejected the request; content may contain disallowed data" |
| 413 | "Content exceeds the server's maximum allowed size" |
| Connection refused / timeout | "Cannot connect to Wastebin server; verify the server is running" |
| DNS resolution failure | "Cannot resolve the server hostname" |

**Unknown/ambiguous errors**: Returned as-is with the HTTP status code and any
additional upstream error message:

`"Unknown error: HTTP {CODE} - {additional server error message}"`

**Format**: Errors are always reported via `IsError: true` in the MCP tool result
with a plain text description.

### File Read Mode

File read mode allows the `create_paste` tool to read file contents from the
local filesystem. It is **enabled by default** (`WASTEBIN_MCP_FILE_READ_ENABLED`).

**⚠️ Security warning for sandbox users**: When file read mode is enabled
without any path restrictions (no ALLOWED_PATHS, no blocklist), an agent running
inside a container/sandbox can read any file accessible from its perspective.
This is effectively **sandbox escape**. Always configure ALLOWED_PATHS and
review the BLOCKED_PATHS defaults.

**Path Allowlist** (`WASTEBIN_MCP_ALLOWED_PATHS`): A comma-separated list of
absolute directory paths. Any file read is validated against this list — the
resolved path must be within one of the allowed directories. Has no default; if
file read mode is enabled and ALLOWED_PATHS is empty, the server skips the
allowlist check and falls through to the blocklist pipeline instead.

**Path Blocklist** (`WASTEBIN_MCP_BLOCKED_PATHS`): A comma-separated list of
absolute directory paths that are denied by default. Default value:
`/etc,/proc,/sys,/dev`. The allowlist takes precedence over the blocklist — if
a file is under a blocked directory but explicitly inside an ALLOWED_PATHS entry,
it IS accessible. This allows e.g. ALLOWED_PATHS=/etc/nginx to work despite `/etc`
being blocked.

**Path resolution**: Before any check, the path is resolved via
`filepath.EvalSymlinks` and `filepath.Clean`. This prevents symlink-based
allowlist bypass.

**Validation flow:**

```
User-supplied file_path
  → Resolve (EvalSymlinks + Clean)
  → ALLOWED_PATHS check
     ├─ Under an allowed path  → ✅  proceed to IsLikelyText
     └─ Not under any allowed path
        └─ BLOCKED_PATHS check
           ├─ Under a blocked path → ❌  denied
           └─ Not under any blocked path → ❌  denied (not authorized)
```

**File validation (text detection: IsLikelyText)**:

Reads the first 8 KB of the file and applies a content-based heuristic:

1. Must be valid UTF-8 (`utf8.Valid`).
2. No null bytes (`b == 0`).
3. Control character ratio (characters `0x00-0x1F` excluding `\n`, `\r`, `\t`)
   must be below 5%.

This rejects binary files and non-UTF-8 encodings (Big5, Shift-JIS, Latin-1),
but accepts any text-like file regardless of file extension — Makefile,
Dockerfile, `.gitignore`, scripts without extension, etc.

**Known limitation**: Non-UTF-8 text files (Big5, Shift-JIS, Latin-1) are
rejected. Workaround: convert to UTF-8 before uploading.

**Content size pre-check**: A configurable maximum content size
(`WASTEBIN_MCP_MAX_CONTENT_SIZE`, default 1 MB) is checked before sending the
HTTP request to Wastebin. This saves wasted uploads for oversized files.

**Design rationale**: The Wastebin server itself enforces `WASTEBIN_MAX_BODY_SIZE`
(default 1 MB) and uses zstd compression. The client-side checks are a safety net
to prevent accidental binary uploads and to fail fast on oversized content.

### Sandbox Path Translation

A gated feature (`WASTEBIN_MCP_SANDBOX_MOUNTS`) for translating
container/sandbox-internal paths to host paths before file reading.

**Mount Mapping Format**: Docker mount style `host_path:sandbox_path` pairs,
comma-separated. Example:
`/home/user/.hermes/profiles/neko/sandboxes/default/workspace:/workspace`

**Translation Modes**:
- **opt-in** (default): Tool schema includes a `translate_sandbox_path`
  boolean parameter. The caller must explicitly set it to `true`.
- **transparent** (`WASTEBIN_MCP_SANDBOX_TRANSPARENT=true`): Translation is
  automatic. The `translate_sandbox_path` parameter is removed from the
  schema, and the server always attempts sandbox-to-host translation when
  mounts are configured.

**Behavior when a transparent-mode path matches no mount**: If the path does
not match any configured mount, it is used as-is (no translation, no error).
The path must still pass the allowlist + blocklist checks after translation
(or as-is when no mount matches).

In both modes, the translated path must still pass the allowlist + blocklist
checks.

**Startup validation**: If `WASTEBIN_MCP_SANDBOX_MOUNTS` is configured, the
server validates at startup that each mount's `host_path` component is covered
by at least one entry in `WASTEBIN_MCP_ALLOWED_PATHS`. If not, the server prints
a clear error and exits. This prevents opaque "path not allowed" failures that
an agent cannot debug.

### Gating Summary

| Feature | ENV | Default |
|---------|-----|---------|
| File read mode | `WASTEBIN_MCP_FILE_READ_ENABLED` | true |
| Path allowlist | `WASTEBIN_MCP_ALLOWED_PATHS` | — (required for file reads) |
| Path blocklist | `WASTEBIN_MCP_BLOCKED_PATHS` | `/etc,/proc,/sys,/dev` |
| Max content size | `WASTEBIN_MCP_MAX_CONTENT_SIZE` | 1 MB |
| Sandbox mounts | `WASTEBIN_MCP_SANDBOX_MOUNTS` | — |
| Transparent mode | `WASTEBIN_MCP_SANDBOX_TRANSPARENT` | false |
| Disable built-in blocklist | `WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST` | false |
| Server URL | `WASTEBIN_SERVER_URL` | — (required) |
| Default expires | `WASTEBIN_MCP_DEFAULT_EXPIRES` | 31536000 |
