# Wastebin MCP Server (wastebin-mcp-go)

A Model Context Protocol (MCP) server and CLI tool that creates pastes on a
Wastebin pastebin instance via its REST API.

## Language

### Core Types

**WastebinClient**: The HTTP client that holds the server base URL and an
`*http.Client` and communicates with a Wastebin instance to create pastes. The
underlying HTTP transport uses hardcoded defaults that are not currently
configurable via environment variables:

| Setting | Value |
|---------|-------|
| Request timeout | 30s |
| TCP dial timeout | 10s |
| TLS handshake timeout | 10s |
| Response header timeout | 30s |
| Idle connection timeout | 90s |
| Max idle connections | 100 |
| Max idle connections per host | 10 |
| Max redirects | 10 |

**Config**: The configuration parameters — server URL, default expiration, file
read mode settings, sandbox mount mappings, and translation mode.

**CreatePasteArgs**: All input parameters for creating a paste in MCP mode —
content or file_path, extension, expires, title, burn_after_reading, password,
and optional sandbox path translation flag.

**PasteResponse**: The structured result returned after creating a paste —
hostname, paste id, and URL variants (raw, rendered).

**Blocklist**: A default set of sensitive system directories (`/etc`, `/proc`,
`/sys`, `/dev`) and sensitive path components (`.ssh`, `.gnupg`, `.aws`,
`.kube`, `.docker`, `.git`) that are denied. The allowlist bypasses system
directory prefixes (so `ALLOWED_PATHS=/etc/nginx` works) but the sensitive
**component** blocklist is enforced regardless of ALLOWED_PATHS — a path
under an allowed directory that contains `.ssh` or `.git` is still denied.

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
| `translate_sandbox_path` | boolean | no | Translate sandbox path (MCP mode only; only when mounts configured) |

\* At least one of `content` or `file_path` is required. If both are provided,
the server returns a clear error: "provide either 'content' or 'file_path', not both; pick exactly one input mode".

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
  "raw": "/raw/FTuutJssdSh.md",
  "markdown_rendered": "/md/FTuutJssdSh.md",
  "password_hint": "This paste is password-protected. Retrieve raw content via the Wastebin-Password header:\n  curl -H 'Wastebin-Password: YOUR_PASSWORD' https://bin-staging.xlion.tw/raw/FTuutJssdSh.md\n(Replace YOUR_PASSWORD with the actual password.)"
}
```

| Scenario | extension | markdown_rendered | hint | password_hint |
|----------|-----------|-------------------|------|---------------|
| Content mode + ext=.md | from caller | ✅ | ❌ | when password set |
| Content mode + ext≠.md | from caller | ❌ | ❌ | when password set |
| Content mode + no ext | unset | ❌ | ✅ | when password set |
| File mode + .md extension | from path | ✅ | ❌ | when password set |
| File mode + non-.md ext | from path | ❌ | ❌ | when password set |
| File mode + no extension | unset | ❌ | ✅ | when password set |

- `id` is included for agents to construct custom URLs (e.g. `curl`).
- `markdown_rendered` only appears when extension is `.md` or `.markdown`.
- `hint` only appears when extension is unknown (fuzzy case).
- `password_hint` only appears when the paste is password-protected.
- URLs are relative (`hostname` is separate). The tool description instructs
  agents to reconstruct full URLs as `{hostname}{url}`.

### Extension Detection (File Mode)

Uses Go's `filepath.Ext()` which returns the trailing extension only:
- `script.py` → `.py`, `archive.tar.gz` → `.gz`, `Dockerfile` → `""`
- For extensionless filenames (Dockerfile, Makefile), the `extension` parameter
  can be used explicitly.

Password-protected pastes: If you create a paste with a password, retrieval
requires providing the password via the `Wastebin-Password` header. Since there
is no `get_paste` tool, agents must use
`curl -H "Wastebin-Password: ..." {hostname}/raw/{id}` to retrieve it.
This is a known limitation by design.

> **Security**: The `password` query parameter is deprecated and should not be
> used. URL query parameters are commonly logged by proxies, load balancers,
> and web servers, which can leak secrets.

### Operation Modes

**MCP Mode**: The program starts a stdio JSON-RPC server implementing the Model
Context Protocol, listening for incoming `create_paste` tool-call requests and
returning structured JSON responses. Activated when no CLI subcommand is given.

**CLI Mode**: Activated by the `create` subcommand. Accepts paste parameters as
flags, executes a one-shot paste creation, and prints the result to stdout, then
exits. Output format: JSON (same as MCP mode response). CLI mode does not
support sandbox path translation — it rejects the sandbox translation
environment variables (`WASTEBIN_MCP_SANDBOX_MOUNTS`,
`WASTEBIN_MCP_SANDBOX_TRANSPARENT`) with a configuration error, since sandbox
translation is an MCP-mode-only feature.

**Debug Mode**: Activated by `DEBUG=1` env var in MCP mode, or `--debug` flag on
the `create` subcommand — enables sparse `slog.Debug` logging to stderr for
selected operations (paste submission details, sandbox path translation, error
responses, response-body close failures). This is not an HTTP wire dump;
individual request and response bodies are not logged.

### Error Handling

When a paste creation fails, the error message is constructed as follows:

**Unified wording**: Every paste-creation (handler) error message follows the
`"<problem>; <next-step guidance>"` convention, with dynamic values appended as
`: <value>`. Errors that carry structured, programmatically-relevant data are
typed errors whose `Error()` method produces the static wording centrally (see
[docs/adr/005-error-model.md](docs/adr/005-error-model.md) and
`internal/wastebin/errors.go`); purely diagnostic values are appended with
`: <value>` suffixes at the wrapping site. Pure condition checks remain
sentinel errors.

**Known errors translated to clear messages:**

| Error Condition | Message |
|-----------------|---------|
| HTTP 403 | "server rejected the request; content may contain disallowed data; ask the user to check the content or the server logs" |
| HTTP 413 | "content exceeds the server's maximum allowed size; split the content into smaller parts and upload each separately" |
| Connection refused / dial timeout | "cannot connect to Wastebin server; verify the server is running: <wrapped err>" |
| DNS resolution failure | "cannot resolve the server hostname; verify WASTEBIN_SERVER_URL points to a resolvable host: <wrapped err>" |
| Sandbox translation requested, no mounts | "sandbox path translation was requested but no sandbox mounts are configured; ask the user to check WASTEBIN_MCP_SANDBOX_MOUNTS if translation should be enabled" |
| Sandbox path matches no mount | "sandbox path does not match any configured mount; ask the user to check the sandbox mount configuration: <path>" |

**Unknown/ambiguous errors**: Returned with the HTTP status code:

`"unknown HTTP error; ask the user to check the server status or the request: HTTP <CODE>"`

**Format**: Errors are always reported via `IsError: true` in the MCP tool result
with a plain text description. The complete error tables live in
[docs/MCP_TOOLS.md](docs/MCP_TOOLS.md).

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
allowlist check and falls through to the blocklist pipeline instead. Relative
entries are rejected at startup.

**Built-in Blocklist**: Two independent sub-checks:
- **System directory prefixes** (`/etc`, `/proc`, `/sys`, `/dev`): Bypassed by
  ALLOWED_PATHS. This allows e.g. `ALLOWED_PATHS=/etc/nginx` to work despite
  `/etc` being in the prefix blocklist.
- **Sensitive path components** (`.ssh`, `.gnupg`, `.aws`, `.kube`, `.docker`,
  `.git`): **Not bypassed by ALLOWED_PATHS.** Even if a file is under an
  explicitly allowed directory, it is denied if any path component matches
  a sensitive pattern.

**User Blocklist** (`WASTEBIN_MCP_BLOCKED_PATHS`): A comma-separated list of
absolute directory paths to block. Empty by default — the built-in blocklist
handles system directories (`/etc`, `/proc`, `/sys`, `/dev`) separately. Each
entry stores both the **lexical form** (as configured, `filepath.Clean`) and
the **resolved form** (after `filepath.EvalSymlinks`; falls back to
`filepath.Clean` for non-existent paths). The lexical form enables a
pre-resolution check (Stage 4a) that catches late-created or retargeted
symlinks, including relative `file_path` values in CLI mode. The resolved form
(Stage 4b) matches what `EvalSymlinks` produces at request time. Relative
entries are rejected at startup. The allowlist takes precedence over the user
blocklist.

**Path resolution**: Sensitive path components (`.ssh`, `.gnupg`, `.aws`,
`.kube`, `.docker`, `.git`) are checked on the raw input **before** symlink
resolution, catching symlinked blocked components (e.g. `.ssh` → `realssh`)
before `EvalSymlinks` resolves the symlink and the component name disappears.
After this pre-resolution check, the path is then resolved via
`filepath.EvalSymlinks` and `filepath.Clean`. This multi-layer approach
prevents symlink-based bypass of the component blocklist. After validation
passes, the file is opened using `openat(2)` with `O_NOFOLLOW`, walking every
path component from a trusted root fd (`/`). This three-layer approach prevents
TOCTOU symlink-swap attacks where a validated path is replaced with a symlink
between validation and the actual file open.

**Validation flow:**

```
User-supplied file_path
  → Sandbox path traversal detection (before sandbox translation)
  → Sandbox component blocklist check (before sandbox translation)
  → Sandbox translation (if enabled)
  → Mount host root verification (after translation)
  → Stage 1a — Path traversal detection (raw input)
  → Stage 1b — Sensitive component detection (raw input, before symlink resolution)
  → Stage 4a — User blocklist lexical (pre-resolution, before EvalSymlinks)
  → Resolve (EvalSymlinks + Clean + Abs)
     → Stage 2 — ALLOWED_PATHS check
      ├─ ALLOWED_PATHS configured
      │  ├─ Path under allowed path → Stage 3b (sensitive component check)
      │  │                          ├─ Blocked component found → ❌  denied
      │  │                          └─ No blocked component → ✅  IsLikelyText
      │  └─ Path not under any allowed path → ❌  denied (not authorized)
      └─ ALLOWED_PATHS not configured
         → Stage 3a — Built-in prefix blocklist
         │  ├─ Blocked → ❌  denied
         │  └─ OK → Stage 3b — Built-in component blocklist
         │          ├─ Blocked → ❌  denied
         │          └─ OK → Stage 4b — User blocklist resolved (post-resolution)
         │                  ├─ Blocked → ❌  denied
         │                  └─ OK → ✅  IsLikelyText
```

**File validation (text detection: IsLikelyText)**:

Reads the first 8 KB of the file and applies a content-based heuristic:

0. **Magic-byte signature detection** (`hasBinarySignature`) — rejects files
   starting with known binary format signatures: PDF (`%PDF`), PNG (`\x89PNG`),
   ZIP (`PK\x03\x04` / `PK\x05\x06` / `PK\x07\x08`), GZip (`\x1f\x8b`), and
   ELF (`\x7fELF`). This runs first, before UTF-8 validation, to catch binary
   formats that happen to pass the other checks.
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

**MCP mode only**: Sandbox path translation is only available in MCP mode. The
CLI (`wastebin-mcp-go create`) does not support it and rejects the sandbox
translation environment variables with a configuration error, so transparent
mode never applies translation in CLI mode.

**Mount Mapping Format**: Docker mount style `host_path:sandbox_path` pairs,
comma-separated. Example:
`/home/user/.hermes/profiles/neko/sandboxes/default/workspace:/workspace`

Sandbox mount paths must be unique and non-overlapping — one mount's sandbox
path cannot be a prefix of another's. Overlapping or duplicate paths are
rejected at startup with a clear error.

**Translation Modes**:
- **opt-in** (default): Tool schema includes a `translate_sandbox_path`
  boolean parameter. The caller must explicitly set it to `true`.
- **transparent** (`WASTEBIN_MCP_SANDBOX_TRANSPARENT=true`): Translation is
  automatic. The `translate_sandbox_path` parameter is removed from the
  schema, and the server always attempts sandbox-to-host translation when
  mounts are configured.

**Behavior when a transparent-mode path matches no mount**: If the path does
not match any configured mount, the request is rejected with an error.

In both modes, the translated path must still pass the allowlist + blocklist
checks.

**Startup validation**: If `WASTEBIN_MCP_SANDBOX_MOUNTS` is configured, the
server validates at startup that each mount's `host_path` is covered by at least
one entry in `WASTEBIN_MCP_ALLOWED_PATHS`. When `ALLOWED_PATHS` is empty (or not
set), mounts self-authorize — no startup validation check is performed. When
`ALLOWED_PATHS` is set and a mount's host path is not covered, the server logs
a warning and skips that mount; startup continues normally.

**Security**: Path traversal (`..`) is detected on the original sandbox path
_before_ any translation occurs. After translation, the result is verified to
still be under the matched mount's host root. This prevents an attacker from
using `filepath.Join` normalization to bypass the traversal check.

### Gating Summary

| Feature | ENV | Default |
|---------|-----|---------|
| File read mode | `WASTEBIN_MCP_FILE_READ_ENABLED` | true |
| Path allowlist | `WASTEBIN_MCP_ALLOWED_PATHS` | — (optional — when empty, falls through to blocklist pipeline) |
| Path blocklist | `WASTEBIN_MCP_BLOCKED_PATHS` | — (empty; built-in blocklist handles `/etc,/proc,/sys,/dev`) |
| Max content size | `WASTEBIN_MCP_MAX_CONTENT_SIZE` | 1 MB |
| Sandbox mounts | `WASTEBIN_MCP_SANDBOX_MOUNTS` | — (MCP mode only — rejected in CLI mode) |
| Transparent mode | `WASTEBIN_MCP_SANDBOX_TRANSPARENT` | false (MCP mode only — rejected in CLI mode) |
| Disable built-in blocklist | `WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST` | false |
| Server URL | `WASTEBIN_SERVER_URL` | — (required) |
| Default expires | `WASTEBIN_MCP_DEFAULT_EXPIRES` | 31536000 |
| Insecure password (HTTP) | `WASTEBIN_MCP_ALLOW_INSECURE_PASSWORD` | false |
| Debug logging | `DEBUG` | — |
