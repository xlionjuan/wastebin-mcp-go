# ADR-001: Use Go SDK for MCP and Follow searxng-mcp-go Patterns

## Status

Accepted

## Context

The wastebin-mcp-go project is an MCP server that exposes paste-creation
functionality via the Model Context Protocol. Several architectural decisions
needed to be made at the outset:

1. Which MCP SDK to use for implementing the server
2. What project structure and patterns to follow
3. Whether to include a paste retrieval tool
4. Whether file-mode reading should be opt-in or enabled by default

The existing [searxng-mcp-go](https://github.com/xlionjuan/searxng-mcp-go)
project (also by the same author) provides an established template for MCP
servers in Go, with proven patterns for SDK usage, CLI structure, and
documentation.

## Decisions

### 1. Use `github.com/modelcontextprotocol/go-sdk`

**Decision:** Use the official Go SDK for the Model Context Protocol
(`github.com/modelcontextprotocol/go-sdk`) to implement the MCP server.

**Reasoning:**

- Official SDK maintained by the MCP specification authors — follows spec
  correctly and stays up to date with protocol changes.
- Same SDK used by searxng-mcp-go, providing a known, working codebase to
  reference.
- Provides `mcp.NewServer`, `mcp.AddTool`, and `mcp.CallToolResult` types that
  handle JSON-RPC framing and transport details.
- The SDK supports both tool registration (for MCP mode) and direct invocation
  patterns (for CLI mode) — both are needed by this project.
- Avoids maintaining a custom JSON-RPC implementation.

**Alternatives considered:**
- **Custom JSON-RPC implementation** — rejected; would require implementing
  protocol negotiation, error handling, and transport management from scratch.
- **`go-mcp` community SDK** — rejected; the official SDK has broader adoption
  and is guaranteed to follow the spec.

### 2. Follow searxng-mcp-go Project Patterns

**Decision:** Structure wastebin-mcp-go following the same patterns as
searxng-mcp-go.

**Reasoning:**

- Same author and same deployment context (MCP server for a self-hosted service
  behind an OpenCode agent runtime).
- Proven project structure:
  - Root `main.go` for entry point and CLI dispatch
  - `cli.go` for CLI subcommand implementation (create subcommand)
  - `mcp.go` for MCP server initialization and handler registration
  - `internal/{service}/` for core business logic, types, and HTTP client
  - Root `*_test.go` for integration-style tests
  - `docs/` for user-facing documentation
  - `docs/adr/` for architecture decision records
- Same CI patterns (Go build, test, golangci-lint, goreleaser).
- Same SDK import style and MCP server setup pattern.

**Specific patterns adopted:**
- Config loaded from environment variables in `internal/wastebin/config.go`
  via `ConfigFromEnv()`.
- CLI flags parsed with Go's `flag` package in root-level files.
- MCP server initialized with `mcp.NewServer`, tools added via `mcp.AddTool`,
  then `server.Run(ctx, &mcp.IOTransport{...})`.
- Error messages returned as `IsError: true` MCP tool results with plain text
  descriptions.
- Stdin validation (checking for valid MCP `initialize` message) to prevent
  hanging when piped non-MCP input.

### 3. Do NOT Add a `get_paste` Tool

**Decision:** Do not implement a `get_paste` MCP tool for retrieving existing
pastes.

**Reasoning:**

- Paste retrieval is trivially done with `curl {hostname}/raw/{id}` — no
  special tool is needed.
- The Wastebin server supports raw content retrieval via simple HTTP GET,
  which is well-suited to `curl`.
- Adding a `get_paste` tool would require an HTTP client that supports password
  headers (`Wastebin-Password`), increasing the scope and surface area of the project.
- Keeping the tool set focused on a single creation tool reduces complexity,
  testing burden, and documentation surface.
- Agents already have `curl` or HTTP client capabilities — telling them to
  use `{hostname}{raw}` gives them everything they need.

**Limitation acknowledged:**
- Password-protected paste retrieval requires `curl -H "Wastebin-Password: ..."`,
  which is a manual step for agents but well within standard tooling.
- The tool description explicitly instructs agents on how to handle this.

### 4. File Mode On by Default with Security Guards

**Decision:** Enable file-reading mode by default
(`WASTEBIN_MCP_FILE_READ_ENABLED=true`), but with strict security guards.

**Reasoning:**

- **On by default makes the tool immediately useful** — the most common use case
  for wastebin-mcp-go is uploading file contents from a workspace. Requiring
  explicit opt-in for every deployment adds friction.
- **Security guards prevent abuse in default configuration:**
  - Path allowlist (`WASTEBIN_MCP_ALLOWED_PATHS`): No file reads succeed
    without explicit path configuration. When file mode is enabled and
    ALLOWED_PATHS is empty, all file reads are refused.
  - Path blocklist (`WASTEBIN_MCP_BLOCKED_PATHS`, default
    `/etc,/proc,/sys,/dev`): Sensitive system directories are blocked.
  - Symlink resolution (`filepath.EvalSymlinks`): Prevents symlink-based
    allowlist bypass.
  - Binary detection: First 8 KB are checked for UTF-8 validity and control
    character ratio before upload.
  - Content size limit: Configurable maximum size prevents oversized uploads.
- **Explicit security warning in documentation** for sandbox/container
  deployments where unrestricted file read could enable sandbox escape.

**Alternative considered:**
- **File mode off by default** — rejected; would require every user to discover
  and enable the feature, reducing usability for the primary use case.
- **File mode opt-in with a startup flag** — rejected; environment variable
  already provides the toggle mechanism without adding CLI complexity.

## Consequences

- The project has a single `create_paste` tool with dynamic schema based on
  configuration (file mode, sandbox mounts, transparent mode).
- Agents must use `curl` for paste retrieval, but the response always includes
  the `raw` URL and `hostname` for easy reconstruction.
- Security documentation must clearly warn about the implications of file mode
  in sandbox/container environments.
- The project structure is immediately familiar to anyone who has worked with
  searxng-mcp-go.
- Upgrading the MCP SDK follows the same dependency update pattern as
  searxng-mcp-go.

## Effective Date

2026-06-09
