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
  `Wastebin-Password` header or as a `password` query parameter. There is no
  `get_paste`
  tool — agents must use `curl` directly.
- File mode applies a three-tier path validation pipeline: traversal detection,
  allowlist (`WASTEBIN_MCP_ALLOWED_PATHS`), and blocklist (built-in + user).
  See [Security Notes](#security-notes) for details.

### Parameters

| Parameter | Type | Required | Description |
|---|---|---|---|
| `content` | string | conditional | The text content of the paste. Provide this OR `file_path`, not both. Required when file mode is disabled. |
| `file_path` | string | conditional | Path to a local file to read and upload as paste content. Provide this OR `content`, not both. Only present when file mode is enabled. |
| `extension` | string | no | File extension for syntax highlighting (e.g. `go`, `py`, `js`, `md`). When using `file_path`, detected from the file name if not provided. |
| `expires` | string | no | Expiration time: bare number (seconds) or number with unit suffix. Examples: `3600`, `1h`, `7d`, `30M`. Defaults to `WASTEBIN_MCP_DEFAULT_EXPIRES`. |
| `title` | string | no | Optional title for the paste. |
| `burn_after_reading` | boolean | no | If `true`, the paste is deleted after being read once. |
| `password` | string | no | Encrypt the paste with a password. See [Password-Protected Pastes](#password-protected-pastes) for retrieval instructions. |
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

```json
{
  "hostname": "https://bin-staging.xlion.tw",
  "id": "FTuutJssdSh",
  "url": "/FTuutJssdSh.md",
  "raw": "/raw/FTuutJssdSh.md",
  "markdown_rendered": "/md/FTuutJssdSh.md",
  "hint": "optional hint"
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

| Scenario | extension | markdown_rendered | hint |
|---|---|---|---|
| Content mode + `.md` extension | from caller | ✅ | ❌ |
| Content mode + non-`.md` extension | from caller | ❌ | ❌ |
| Content mode + no extension | unset | ❌ | ✅ |
| File mode + `.md` extension | from path | ✅ | ❌ |
| File mode + non-`.md` extension | from path | ❌ | ❌ |
| File mode + no extension | unset | ❌ | ✅ |

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
  "password_hint": "This paste is password-protected. Retrieve raw content via the Wastebin-Password header:\n  curl -H 'Wastebin-Password: YOUR_PASSWORD' https://bin-staging.xlion.tw/raw/XyZ789AbCdE.txt\nOr as a query parameter:\n  curl 'https://bin-staging.xlion.tw/raw/XyZ789AbCdE.txt?password=YOUR_PASSWORD'\n(Replace YOUR_PASSWORD with the actual password.)"
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

#### Known Error Messages

| Error Condition | Message |
|---|---|
| Both `content` and `file_path` provided | `"Create paste error: provide either 'content' or 'file_path', not both"` |
| Neither `content` nor `file_path` provided | `"Create paste error: provide either 'content' or 'file_path'"` |
| `content` is empty (content mode) | `"Create paste error: content cannot be empty"` |
| HTTP 403 from server | `"Create paste error: server rejected the request; content may contain disallowed data"` |
| HTTP 413 from server | `"Create paste error: content exceeds the server's maximum allowed size"` |
| Connection refused / timeout | `"Create paste error: cannot connect to Wastebin server; verify the server is running: <details>"` |
| DNS resolution failure | `"Create paste error: cannot resolve the server hostname: <details>"` |
| Content exceeds `WASTEBIN_MCP_MAX_CONTENT_SIZE` | `"Create paste error: content exceeds the maximum allowed size: <N> bytes exceeds limit of <N> bytes"` |
| File path rejected by traversal detection | `"Create paste error: path traversal is not allowed"` |
| File path rejected by allowlist | `"Create paste error: file path is not under any allowed path"` |
| File path rejected by built-in blocklist (system prefix) | `"Create paste error: file path is in a blocked system directory (<path>)"` |
| File path rejected by built-in blocklist (sensitive component) | `"Create paste error: file path contains a blocked component (<name>)"` |
| File path rejected by user blocklist | `"Create paste error: file path is in a user-blocked directory (<path>)"` |
| File is binary or non-UTF-8 | `"Create paste error: file is binary or not valid UTF-8 text"` |
| File cannot be read (not found, permissions, symlink error) | `"Create paste error: file path cannot be used"` |
| Sandbox translation requested but no mounts configured | `"Create paste error: sandbox path translation requested but no mounts configured"` |
| Sandbox path does not match any configured mount | `"Create paste error: sandbox path does not match any configured mount: <path>"` |
| Unknown HTTP error | `"Create paste error: unknown HTTP error: HTTP <CODE>"` |
| Invalid expiration format | `"Create paste error: invalid expiration: <reason>"` (reason: `expiration cannot be negative`, `unknown expiration unit`, `invalid expiration format`, `expiration overflow`) |
| Server returns malformed JSON | `"Create paste error: failed to parse Wastebin response: <details>"` |
| Cross-host redirect blocked | `"Create paste error: redirect to different host blocked: <from> -> <to>"` |
| Redirect scheme downgrade from https to http | `"Create paste error: redirect scheme downgrade from https to http blocked: <host> (https -> http)"` |
| Too many redirects (>10) | `"Create paste error: stopped after 10 redirects"` |
| `args` is nil | `"Create paste error: args is required"` |
| File read disabled by configuration | `"Create paste error: file read is disabled by configuration"` |

#### Error Response Format (as received by MCP client)

```json
{
  "content": [
    {
      "type": "text",
      "text": "Create paste error: Provide either 'content' or 'file_path', not both."
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

The server applies a **five-stage path validation pipeline** (in order):

1. **Path traversal detection (before sandbox translation)** — rejects paths
   containing `..` or equivalents, checked on the raw input _before_ any
   sandbox path translation occurs. This prevents `filepath.Join` normalization
   from silently removing `..` during translation and bypassing the check.
2. **Sandbox path translation** — if sandbox mounts are configured and
   `translate_sandbox_path` is enabled, the sandbox path is translated to its
   corresponding host path. After translation, the result is verified to still
   be under the matched mount's host root.
3. **ALLOWED_PATHS (user allowlist)** — if configured, only paths under allowed
   directories are accepted. ALLOWED_PATHS bypasses the system directory prefix
   blocklist and the user blocklist, but **not** the sensitive component
   blocklist (Stage 4b).
4. **Built-in blocklist** — two independent checks:
   - *System directory prefix*: `/etc`, `/proc`, `/sys`, `/dev`
   - *Sensitive path component*: `.ssh`, `.gnupg`, `.aws`, `.kube`, `.docker`,
     `.git`
   The prefix check is bypassed by ALLOWED_PATHS; the component check is not.
   Can be disabled entirely via `WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST=true`.
5. **User blocklist** — configurable via `WASTEBIN_MCP_BLOCKED_PATHS`.

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
- **Symlink protection** — all paths are resolved via `EvalSymlinks` and
  `Clean` before validation.
- **Binary detection** — files are checked for valid UTF-8 and control
  character ratio; binary and non-UTF-8 files are rejected.

> **⚠️ Sandbox users:** When file read mode is enabled without path
> allowlist or blocklist restrictions, an agent inside a container/sandbox
> can read any file accessible from its perspective. This is effectively
> **sandbox escape**.

#### Sandbox Path Translation

When `WASTEBIN_MCP_SANDBOX_MOUNTS` is configured, the server validates at
startup that:

1. Each mount's host path is covered by `WASTEBIN_MCP_ALLOWED_PATHS`. If not,
   the server prints a clear error and exits — preventing opaque "path not
   allowed" failures that agents cannot debug.
2. No two mounts share overlapping sandbox paths (one sandbox path being a
   prefix of another). Overlapping or duplicate sandbox paths are rejected at
   startup with a clear error, eliminating the ambiguity and security risk of
   first-match-wins resolution.

#### Password-Protected Pastes

Password-protected pastes cannot be retrieved via `/raw/{id}` with a simple GET
request. The Wastebin server accepts the password through two mechanisms:

1. **HTTP header** — pass the `Wastebin-Password` header:
   ```bash
   curl -H "Wastebin-Password: your-password" https://bin-staging.xlion.tw/raw/AbCdEfGh123
   ```

2. **Query parameter** — append `?password=...` to the raw URL:
   ```bash
   curl "https://bin-staging.xlion.tw/raw/AbCdEfGh123?password=your-password"
   ```

Both methods return the raw paste content directly. If the password is missing
or incorrect, Wastebin returns an HTML password form instead.

This is by design — there is no `get_paste` tool. Agents must use `curl`
(or equivalent) for paste retrieval.

When creating a password-protected paste via the `create_paste` tool, the
response includes a `password_hint` field with concrete curl examples for
reconstructing the retrieval commands.

---

## Implementation Details

- **Transport**: Stdio (stdin/stdout)
- **Protocol**: MCP (Model Context Protocol)
- **Wastebin API**: REST (`POST /` with JSON body)
- **Server URL**: Configured via `WASTEBIN_SERVER_URL` (required)
- **Stdin validation**: First line of stdin must be a valid JSON-RPC 2.0
  `initialize` message (max 1 MB); non-MCP input causes immediate exit
- **SDK**: `github.com/modelcontextprotocol/go-sdk`
