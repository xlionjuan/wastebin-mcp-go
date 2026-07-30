# MCP Tools Reference

This document describes the MCP tool exposed by the Wastebin MCP server.

## create_paste

Create a text paste on the configured Wastebin instance. Supports two modes:
inline content or local file upload.

### Tool Description

The `create_paste` tool uploads content to a Wastebin pastebin server via its
REST API. It returns structured JSON with the paste ID and URLs that agents can
use to reconstruct full retrieval URLs.

**Important considerations:**

- Password-protected pastes: Retrieval requires providing the password via the
  `Wastebin-Password` header. There is no `get_paste`
  tool — agents must use `curl` directly.
- File mode applies sandbox pre-validation checks (traversal detection,
  sensitive component detection) before optional sandbox path translation,
  followed by the five-stage `validateFilePath` pipeline: traversal, component,
  allowlist, built-in blocklist, and user blocklist.
  See [Security Notes](#security-notes) for details.

### MCP Tool Annotations

The `create_paste` tool advertises the following MCP protocol-level Tool
Annotations, which affect how MCP hosts (client UIs, agent frameworks) present
and permit the tool:

| Annotation | Value | Meaning |
|---|---|---|
| `DestructiveHint` | `false` | The tool is additive (creation-only). It creates new pastes but never modifies or deletes existing data. |
| `OpenWorldHint` | `false` | The tool targets a configured Wastebin instance with scoped file access, not an open external domain. Agents should not expect general Web access. |

These annotations are set in `mcp.go` and are visible to MCP clients that
inspect tool metadata at runtime.

### Parameters

| Parameter | Type | Required | Description |
|---|---|---|---|
| `content` | string | conditional | The text content of the paste. Provide this OR `file_path`, not both. Required when file mode is disabled. |
| `file_path` | string | conditional | Path to a local file to read and upload as paste content. Provide this OR `content`, not both. Only present when file mode is enabled. |
| `extension` | string | no | File extension for syntax highlighting (e.g. `go`, `py`, `js`, `md`). When using `file_path`, detected from the file name if not provided. |
| `expires` | string | no | Expiration time: bare number (seconds) or number plus unit suffix (s, m, h, d, w, M=30d, y=365d). Examples: `3600`, `1h`, `7d`, `30M`. Defaults to the configured default. Configured via `WASTEBIN_MCP_DEFAULT_EXPIRES`. |
| `title` | string | no | Optional title for the paste. |
| `burn_after_reading` | boolean | no | If `true`, the paste is deleted automatically after being retrieved via any access method (raw, web, API) for the first time. The agent's own reads also count — creating a burn-after-reading paste and then reading it back will delete it. |
| `password` | string | no | Encrypt the paste with a password. Must be at least 1 character when provided. See [Password-Protected Pastes](#password-protected-pastes) for retrieval instructions. |
| `translate_sandbox_path` | boolean | no | Only present when sandbox mounts are configured and transparent mode is off. Set to `true` to translate a sandbox-internal `file_path` to the corresponding host path. |

### Schema Behavior

The tool schema is built **dynamically at startup** based on environment
configuration. The following rules determine which parameters appear:

#### `content` and `file_path`

- **File mode enabled** (default): Both `content` and `file_path` are present in
  the schema. Neither is `required` — the caller must provide exactly one.
  If both are provided, the server returns a clear error: *"Provide either
  'content' or 'file_path', not both."*
- **File mode disabled** (`WASTEBIN_MCP_FILE_READ_ENABLED=false`): Only
  `content` is present, and it becomes required. `file_path` is excluded from
  the schema entirely.

#### `translate_sandbox_path`

- **Not present** when no sandbox mounts are configured, or when
  `WASTEBIN_MCP_SANDBOX_TRANSPARENT=true` (automatic translation).
- **Present** when mounts are configured and transparent mode is off (default).

**Behavior in transparent mode**: When `WASTEBIN_MCP_SANDBOX_TRANSPARENT=true`,
the server translates the sandbox path automatically without requiring the
caller to set `translate_sandbox_path`. If the path does not match any
configured mount, the request is rejected with an error.

> **Summary:** Agents should always check the tool schema at runtime rather
> than hard-coding parameter names.

### Expiration Format

Accepts two formats:

1. **Bare number** (e.g. `3600`) — treated as seconds
2. **Number with unit suffix** — translated to seconds:

| Suffix | Unit | Example |
|---|---|---|
| `s` | seconds | `30s` |
| `m` | minutes | `5m` |
| `h` | hours | `2h` |
| `d` | days | `7d` |
| `w` | weeks | `2w` |
| `M` | months (30 days) | `6M` |
| `y` | years (365 days) | `1y` |

### Response Format

The tool returns a JSON object with the following fields:

**Paste with `.md` extension:**

```json
{
  "hostname": "https://bin-staging.xlion.tw",
  "id": "FTuutJssdSh",
  "url": "/FTuutJssdSh.md",
  "raw": "/raw/FTuutJssdSh.md",
  "markdown_rendered": "/md/FTuutJssdSh.md"
}
```

**Paste with unknown extension (or no extension):**

```json
{
  "hostname": "https://bin-staging.xlion.tw",
  "id": "AbCdEfGh123",
  "url": "/AbCdEfGh123",
  "raw": "/raw/AbCdEfGh123",
  "hint": "Extension not detected; syntax highlighting may not apply"
}
```

| Field | Type | Always Present | Description |
|---|---|---|---|
| `hostname` | string | ✅ | The configured Wastebin server URL |
| `id` | string | ✅ | The paste ID (base64-like encoded integer) |
| `url` | string | ✅ | Relative URL to the rendered paste page |
| `raw` | string | ✅ | Relative URL to the raw paste content |
| `markdown_rendered` | string | — | Present only when extension is `.md` or `.markdown` |
| `hint` | string | — | Present only when extension is unknown (fuzzy match hint) |
| `password_hint` | string | — | Present only when the paste is password-protected (retrieval instructions) |

**Reconstructing full URLs:**

Agents should reconstruct full URLs as `{hostname}{url}` or `{hostname}{raw}`.
For example:

```
https://bin-staging.xlion.tw/FTuutJssdSh.md
https://bin-staging.xlion.tw/raw/FTuutJssdSh.md
```

### Extension Detection (File Mode)

When using `file_path`, the extension is automatically detected using Go's
`filepath.Ext()`:

| File Name | Detected Extension |
|---|---|
| `script.py` | `.py` |
| `archive.tar.gz` | `.gz` |
| `Dockerfile` | (no extension) |
| `Makefile` | (no extension) |

For extensionless filenames, use the `extension` parameter explicitly.

### Output Scenarios

| Scenario | extension | markdown_rendered | hint | password_hint |
|---|---|---|---|---|
| Content mode + `.md` extension | from caller | ✅ | ❌ | when password set |
| Content mode + non-`.md` extension | from caller | ❌ | ❌ | when password set |
| Content mode + no extension | unset | ❌ | ✅ | when password set |
| File mode + `.md` extension | from path | ✅ | ❌ | when password set |
| File mode + non-`.md` extension | from path | ❌ | ❌ | when password set |
| File mode + no extension | unset | ❌ | ✅ | when password set |

### Example Usage

#### Basic inline paste

```json
{
  "content": "Hello, World!",
  "extension": "md"
}
```

**Response:**
```json
{
  "hostname": "https://bin-staging.xlion.tw",
  "id": "FTuutJssdSh",
  "url": "/FTuutJssdSh.md",
  "raw": "/raw/FTuutJssdSh.md",
  "markdown_rendered": "/md/FTuutJssdSh.md"
}
```

#### Paste from file

```json
{
  "file_path": "/home/user/documents/script.py",
  "title": "My Script",
  "expires": "30d"
}
```

**Response:**
```json
{
  "hostname": "https://bin-staging.xlion.tw",
  "id": "AbCdEfGh123",
  "url": "/AbCdEfGh123.py",
  "raw": "/raw/AbCdEfGh123.py"
}
```

#### Paste with password

```json
{
  "content": "secret content",
  "password": "my-password",
  "extension": "txt"
}
```

**Response:**
```json
{
  "hostname": "https://bin-staging.xlion.tw",
  "id": "XyZ789AbCdE",
  "url": "/XyZ789AbCdE.txt",
  "raw": "/raw/XyZ789AbCdE.txt",
  "password_hint": "This paste is password-protected. Retrieve raw content via the Wastebin-Password header:\n  curl -H 'Wastebin-Password: YOUR_PASSWORD' https://bin-staging.xlion.tw/raw/XyZ789AbCdE.txt\n(Replace YOUR_PASSWORD with the actual password.)"
}
```

#### Paste with sandbox path translation

```json
{
  "file_path": "/workspace/report.md",
  "translate_sandbox_path": true
}
```

---

### Error Handling

When a paste creation fails, the error is returned via `IsError: true` in the
MCP tool result with a plain text description.

Errors come from two distinct sources:

1. **Schema validation** — the MCP SDK rejects inputs that do not match the
   runtime JSON Schema before the handler runs.
2. **Handler errors** — the application logic returns `"Create paste error: ..."`
   text with `IsError: true`.

#### Schema Validation Errors

The tool's JSON Schema is built dynamically at startup (see
[Schema Behavior](#schema-behavior)). If an input does not match the schema,
the MCP SDK rejects it. These errors are **not** prefixed with
`Create paste error` and depend on the active schema configuration.

| Condition | When Reachable | Error Pattern |
|---|---|---|
| Unknown/unexpected property (e.g. `file_path` when file mode is disabled) | Property excluded from schema | `"Unknown error: additional properties not allowed: ..."` |
| Both `content` and `file_path` provided (file mode disabled) | `file_path` excluded from schema | `"Unknown error: additional properties not allowed: file_path"` |
| Neither `content` nor `file_path` provided (file mode disabled) | `content` is required in schema | `"Unknown error: required property 'content' not provided"` |
| `password` is empty string (`""`) | Always (`minLength: 1`) | `"Unknown error: ... minLength ..."` |
| Type mismatch (e.g. string for boolean field) | Always | `"Unknown error: ... expected ... got ..."` |

> **Note:** When file mode is **enabled**, `content` and `file_path` are both
> optional (neither is required) and both appear in the schema. The
> mutual-exclusivity validation is then handled at runtime (see handler errors
> below).

#### Handler-Generated Errors

These errors are returned from the `create_paste` handler and always follow the
`"Create paste error: <message>"` format.

**Always applicable (regardless of configuration):**

| Error Condition | Message |
|---|---|
| Both `content` and `file_path` provided (file mode enabled) | `"Create paste error: provide either 'content' or 'file_path', not both"` |
| Neither `content` nor `file_path` provided (file mode enabled) | `"Create paste error: provide either 'content' or 'file_path'"` |
| `content` is empty (content mode) | `"Create paste error: content cannot be empty"` |
| HTTP 403 from server | `"Create paste error: server rejected the request; content may contain disallowed data"` |
| HTTP 413 from server | `"Create paste error: content exceeds the server's maximum allowed size"` |
| Connection refused / timeout | `"Create paste error: cannot connect to Wastebin server; verify the server is running: <details>"` |
| DNS resolution failure | `"Create paste error: cannot resolve the server hostname: <details>"` |
| Content exceeds `WASTEBIN_MCP_MAX_CONTENT_SIZE` | `"Create paste error: content exceeds the maximum allowed size: <N> bytes exceeds limit of <N> bytes"` |
| Password over non-loopback HTTP without override | `"Create paste error: password-protected pastes are not allowed over non-loopback HTTP connections; use HTTPS or set WASTEBIN_MCP_ALLOW_INSECURE_PASSWORD=true for local development"` |
| `password` is empty string (defensive fallback, `minLength` should catch at schema first) | `"Create paste error: password must be non-empty when provided; set a non-empty password or omit the field entirely"` |
| Unknown HTTP error | `"Create paste error: unknown HTTP error: HTTP <CODE>"` |
| Invalid expiration format | `"Create paste error: invalid expiration: <reason>"` (reason: `expiration cannot be negative`, `unknown expiration unit`, `invalid expiration format`, `expiration overflow`, `expiration exceeds maximum supported value`) |
| Extension contains invalid path or query characters | `"Create paste error: extension contains invalid path or query characters: <ext>"` |
| Server returns response with empty paste path | `"Create paste error: invalid Wastebin response: empty path"` |
| Server returns response with non-relative paste path | `"Create paste error: invalid Wastebin response: path must be relative, got <path>"` |
| Server returns response without paste ID | `"Create paste error: invalid Wastebin response: path is missing paste ID"` |
| Server returns malformed JSON | `"Create paste error: failed to parse Wastebin response: <details>"` |
| Wastebin response exceeds maximum allowed size | `"Create paste error: wastebin response exceeds maximum allowed size"` |
| Server returns response with trailing non-whitespace content | `"Create paste error: invalid Wastebin response: unexpected content after JSON response"` |
| HTTP 422 from server (with body) | `"Create paste error: server rejected the request due to a validation error: <details>"` |
| HTTP 422 from server (empty body) | `"Create paste error: server rejected the request due to a validation error"` |
| Cross-host redirect blocked | `"Create paste error: HTTP request failed: Get {path}: redirect to different host blocked: <from> -> <to>"` |
| Redirect scheme downgrade from https to http | `"Create paste error: HTTP request failed: Get {path}: redirect scheme downgrade from https to http blocked: <host> (https -> http)"` |
| Too many redirects (>10) | `"Create paste error: HTTP request failed: Get {path}: stopped after 10 redirects"` |

**File mode errors (only when `WASTEBIN_MCP_FILE_READ_ENABLED=true`):**

| Error Condition | Message |
|---|---|
| File read disabled by configuration | `"Create paste error: file read is disabled by configuration"` |
| File path rejected by traversal detection | `"Create paste error: path traversal is not allowed"` |
| File path rejected by allowlist | `"Create paste error: file path is not under any allowed path"` |
| File path rejected by built-in blocklist (system prefix) | `"Create paste error: file path is in a blocked system directory (<path>)"` |
| File path rejected by built-in blocklist (sensitive component) | `"Create paste error: file path contains a blocked component (<name>)"` |
| File path rejected by user blocklist | `"Create paste error: file path is in a user-blocked directory"` |
| File is binary or non-UTF-8 | `"Create paste error: file is binary or not valid UTF-8 text"` |
| File does not exist | `"Create paste error: file path not found"` |
| Permission denied accessing file path | `"Create paste error: permission denied for file path"` |
| File cannot be read (symlink error, other I/O error) | `"Create paste error: file path cannot be used"` |

**Sandbox errors (only when sandbox mounts are configured):**

| Error Condition | Message |
|---|---|
| Sandbox translation requested but no mounts configured | `"Create paste error: sandbox path translation requested but no mounts configured"` |
| Sandbox path does not match any configured mount | `"Create paste error: sandbox path does not match any configured mount: <path>"` |

#### Error Response Format (as received by MCP client)

```json
{
  "content": [
    {
      "type": "text",
      "text": "Create paste error: provide either 'content' or 'file_path', not both"
    }
  ],
  "isError": true
}
```

---

### Security Notes

#### File Mode (Enabled by Default)

File read mode is **enabled by default** (`WASTEBIN_MCP_FILE_READ_ENABLED=true`).
When file mode is enabled, the `file_path` parameter allows reading local files.
This is a powerful feature that must be configured carefully.

When a sandbox path is supplied with translation enabled, the server runs
**sandbox pre-validation steps** before the **five-stage `validateFilePath`
pipeline** (Stages 1a–4):

**Sandbox pre-validation (before translation):**

1. **Path traversal detection** — rejects paths containing `..` or equivalents,
   checked on the raw sandbox path _before_ any translation occurs. This
   prevents `filepath.Join` normalization from silently removing `..` during
   translation and bypassing the check.
2. **Sensitive component detection** — checks the raw sandbox path for blocked
   components (`.ssh`, `.gnupg`, `.aws`, `.kube`, `.docker`, `.git`) before
   translation. If the sandbox path contains a blocked component, it is rejected
   immediately — before translation converts it to a host path where the
   component name may be symlinked away.
 3. **Sandbox path translation** — if sandbox mounts are configured and
   `translate_sandbox_path` is enabled, the sandbox path is translated to its
   corresponding host path. After translation, the result is verified to still
   be under the matched mount's host root.

**Five-stage `validateFilePath` pipeline (Stages 1a, 1b, 2, 3, 4):**

4. **Stage 1a — Path traversal detection** — runs on the (post-translation)
   path before any symlink resolution.
5. **Stage 1b — Sensitive component detection (raw input)** — checks the
   (post-translation) path for blocked components before symlink resolution.
   The same check is repeated on the resolved path in Stage 3b for defense
   in depth.
6. **Stage 2 — ALLOWED_PATHS (user allowlist)** — if configured, only paths
   under allowed directories are accepted. ALLOWED_PATHS bypasses the system
   directory prefix blocklist and the user blocklist, but **not** the
   sensitive component blocklist (Stage 3b).
7. **Stage 3 — Built-in blocklist** — two independent checks:
   - *System directory prefix* (Stage 3a): `/etc`, `/proc`, `/sys`, `/dev`
   - *Sensitive path component* (Stage 3b): `.ssh`, `.gnupg`, `.aws`, `.kube`,
     `.docker`, `.git`
   The component check on the resolved path is the defense-in-depth companion
   to Stage 1b. The prefix check is bypassed by ALLOWED_PATHS; the component
   check is not. Can be disabled entirely via
   `WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST=true`.
8. **Stage 4 — User blocklist** — configurable via `WASTEBIN_MCP_BLOCKED_PATHS`. Empty by default — the built-in blocklist handles system directories separately.

Without `WASTEBIN_MCP_ALLOWED_PATHS`, file reads **are not automatically
refused** — they fall through to the built-in blocklist, which blocks system
directories and sensitive credential paths by default. This provides a safe
out-of-the-box experience without requiring mandatory allowlist configuration.

**Recommendations:**

- **Configure `WASTEBIN_MCP_ALLOWED_PATHS`** for production deployments to
  tightly scope which directories are accessible.
- **Review the built-in blocklist defaults** — if your paths legitimately
  contain `.ssh` or similar components, you may need to adjust the component
  blocklist or disable the built-in blocklist entirely.
- **Symlink protection (three-layer)** — (1) Sensitive path components
  (`.ssh`, `.gnupg`, `.aws`, `.kube`, `.docker`, `.git`) are checked on the
  raw input **before** any resolution, catching symlinked blocked components
  before `EvalSymlinks` resolves them away. (2) All paths are then resolved
  via `EvalSymlinks` and `Clean` for the allowlist and blocklist checks, with
  the component check repeated on the resolved path for defense in depth.
  (3) The actual file open uses `openat(2)` with `O_NOFOLLOW`, walking every
  path component from a trusted root fd (`/`). This prevents TOCTOU
  symlink-swap attacks where a validated path is replaced with a symlink
  between validation and the file open.
- **Binary detection** — files are checked for valid UTF-8 and control
  character ratio; binary and non-UTF-8 files are rejected.

> **⚠️ Sandbox users:** When file read mode is enabled without path
> allowlist or blocklist restrictions, an agent inside a container/sandbox
> can read any file accessible from its perspective. This is effectively
> **sandbox escape**.

#### Sandbox Path Translation

When `WASTEBIN_MCP_SANDBOX_MOUNTS` is configured, the server validates at
startup that:

1. Each mount's host path is validated against `WASTEBIN_MCP_ALLOWED_PATHS`.
   When `ALLOWED_PATHS` is set and a mount's host path is not covered, the
   server logs a warning and skips that mount; startup continues normally.
   When `ALLOWED_PATHS` is empty (or not set), mounts self-authorize — no
   startup validation check is performed.
2. Each mount's sandbox path is an absolute POSIX path and does not contain
   `..` components before cleanup. Duplicate separators and trailing slashes
   are still normalized, but parent-directory traversal is rejected instead of
   being cleaned into a broader mount.
3. No two mounts share overlapping sandbox paths (one sandbox path being a
   prefix of another). Overlapping or duplicate sandbox paths are rejected at
   startup with a clear error, eliminating the ambiguity and security risk of
   first-match-wins resolution.

#### Password-Protected Pastes

Password-protected pastes cannot be retrieved via `/raw/{id}` with a simple GET
request. Provide the password via the `Wastebin-Password` header:

```bash
curl -H "Wastebin-Password: your-password" https://bin-staging.xlion.tw/raw/AbCdEfGh123
```

This returns the raw paste content directly. If the password is missing
or incorrect, Wastebin returns an HTML password form instead.

> **⚠️ Security warning:** Secrets in URL query parameters (e.g.
> `?password=...`) are commonly logged by proxies, retained in browser
> history, and visible in terminal scrollback. The `Wastebin-Password`
> header is the preferred mechanism. The query-parameter form is
> supported by the Wastebin server for legacy compatibility but should
> be avoided.

This is by design — there is no `get_paste` tool. Agents must use `curl`
(or equivalent) for paste retrieval.

When creating a password-protected paste via the `create_paste` tool, the
response includes a `password_hint` field with concrete curl examples for
reconstructing the retrieval commands.

> **⚠️ Security warning:** When the Wastebin server URL uses `http://`
> (not recommended), the password is sent in cleartext over the network.
> Password-protected pastes over non-loopback HTTP are **rejected** by
> default. Loopback addresses (localhost, 127.0.0.1, ::1) are allowed
> with a warning. To allow non-loopback HTTP for local development, set
> `WASTEBIN_MCP_ALLOW_INSECURE_PASSWORD=true`. Prefer `https://` for
> production use.

---

## Implementation Details

- **Transport**: Stdio (stdin/stdout)
- **Protocol**: MCP (Model Context Protocol)
- **Wastebin API**: REST (`POST /` with JSON body)
- **Server URL**: Configured via `WASTEBIN_SERVER_URL` (required)
- **Stdin validation**: First line of stdin must be a valid JSON-RPC 2.0
  `initialize` message (max 1 MB); non-MCP input causes immediate exit
- **SDK**: `github.com/modelcontextprotocol/go-sdk`
