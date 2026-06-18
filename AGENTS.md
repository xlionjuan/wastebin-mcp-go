# Wastebin MCP Server

A Model Context Protocol (MCP) server providing paste creation via a Wastebin
instance. Agents call this server to upload content (text, code, documents) to
a configured Wastebin pastebin.

## Orientation

- Entry points: `main.go`, `cli.go`, `mcp.go`
- Core package: `internal/wastebin/`
- Unit tests: root `*_test.go` and `internal/wastebin/*_test.go`
- CI and release workflows: `.github/workflows/`
- Domain context and terminology: `CONTEXT.md`

## Documentation Index

| Topic | Document |
|-------|----------|
| Build, install, configuration | [docs/INSTALL.md](docs/INSTALL.md) |
| MCP tool parameters | [docs/MCP_TOOLS.md](docs/MCP_TOOLS.md) |
| Issue tracker workflow | [docs/agents/issue-tracker.md](docs/agents/issue-tracker.md) |
| Triage labels | [docs/agents/triage-labels.md](docs/agents/triage-labels.md) |
| Domain and ADR workflow | [docs/agents/domain.md](docs/agents/domain.md) |
| Architecture decisions | [docs/adr/](docs/adr/) |

## Agent skills

### Issue tracker

Issues are tracked as GitHub issues. See `docs/agents/issue-tracker.md`.

### Triage labels

Labels are managed via `gh label list`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context. See `docs/agents/domain.md`.

## Project Rules

### Build and Test

```bash
# Verify and download dependencies
go mod verify
go mod download

# Check module files are tidy
go mod tidy
git diff --exit-code go.mod go.sum

# Build all packages
go build ./...

# CI-style test run with race detector and coverage
go test -race -shuffle=on -coverprofile=coverage.out ./...

# Known vulnerability scan
go tool govulncheck ./...

# Lint
golangci-lint run ./...

# Check formatting (gofumpt / gofmt / gci)
golangci-lint fmt --diff

# Static analysis fallback (when golangci-lint is unavailable)
go vet ./...

# E2E tests (requires Wastebin server)
go test -tags=e2e ./...
```

### Editing

- Use patch-style edits for existing files (`patch` tool); avoid `sed -i`.
- For new files, use `write_file`.
- Follow the searxng-mcp-go patterns for SDK usage (`github.com/modelcontextprotocol/go-sdk`).

### Code Cleanliness

- Remove junk files before committing: `.bak`, `*.test`, `*~`, `.swp`, `.swo`.
- `.gitignore` already covers `*.bak`, `*.test`, `*.swp`, `*.swo`, `*.out`,
  `REPORT.md`, `.env`.
- **The only correct build output binary name is `wastebin-mcp-go`.**
  Do **not** create, commit, or leave behind binaries with other names
  (e.g. `wastebin-mcp`, `wastebin`, `server`) in the project root.
  Use `go build -o wastebin-mcp-go .` — never a different `-o` value.
- **NEVER create a binary with a name different from the project's canonical
  binary name.** The canonical binary is `wastebin-mcp-go` (the module name).
  Use `go build .` (without `-o`) to let Go use the directory name, which IS
  the canonical name. If you use `go build -o <name>`, the name MUST be
  `wastebin-mcp-go`. Any other name is a self-inflicted mess that must be
  cleaned up immediately and NEVER added to `.gitignore` — that would hide the
  mistake instead of facing it.
- `.gitignore` covers `wastebin-mcp-go`, `*.bak`, `*.test`, `*.swp`, `*.swo`,
  `*.out`, `REPORT.md`, `.env`, and common toolchain extensions.

### Do Not Change

- Do not modify `.gitignore` unless explicitly asked.

### Documentation

- All docs (`docs/*.md`, README, CONTEXT, AGENTS) must be in English.
- Pull requests must update related documentation.

### Go Skill Corpus

- When the optional `cc-skills-golang` series is available,
  every AI agent working on Go code must locate and read the relevant skill
  files before editing, reviewing, or claiming completion.
- For coding tasks, first identify the implementation domains involved, such as
  error handling, concurrency, context propagation, CLI behavior, testing,
  security, dependencies, or performance. Then read the matching
  `cc-skills-golang` skill files. Do not limit coding work to the
  review-focused skill subset.
- Repository rules and local project context override generic skill advice.

### Go Toolchain

Never manually install Go. Run `which go` before any Go command.
If `which go` fails, report and stop — do not install, download, or work around it.

### Verification

- Code changes must be verified with the narrowest meaningful build/test
  command before committing.
- Before opening or updating a PR, or reporting a code-changing task complete,
  all AI agents must select and run the verification gate described below for
  the affected surface.
- Build must pass before commit.
- Run `go vet ./...` as a regular static check alongside the linter.
- After adding or changing MCP tool schemas, verify with a manual MCP test or
  CLI mode test.
- Pure documentation changes (`.md` files only) do not require the build, test,
  or lint gates above.

## GitHub and PR Work

- Use `gh` CLI for all GitHub operations (not browser tools).
- PR title and body must be in English.
- PR agents must create requested PRs with `gh pr create`; do not stop after
  pushing a branch.

## Git Configuration

- Treat all Git configuration as read-only. Agents may inspect Git config, but
  must not write, unset, or override any Git config at repo, worktree, global,
  system, file, submodule, or per-command (`git -c`) scope unless the current
  human explicitly names the config key to change. If Git fails because config
  or identity is missing, unsafe, or wrong, stop and report; do not repair,
  guess, or work around it.

## Git Identity

- Use the existing git identity as-is. Inspect with
  `git config --get user.name` / `git config --get user.email`. Do not set,
  override, or hard-code author/committer identity.
- A commit with known wrong author metadata is tainted: do not make it the tip
  of any branch.

## Key Constraints

- **No `get_paste` tool** — agents should use `curl {hostname}/raw/{id}` instead.
- **File read mode is ON by default** — configure ALLOWED_PATHS and review
  BLOCKED_PATHS defaults for sandbox deployments.
- **Schema is dynamic** — `file_path` and `translate_sandbox_path` are excluded
  from the MCP tool schema when their respective features are disabled.
- **Upstream**: `github.com/xlionjuan/wastebin-mcp-go` (private)
- **SDK**: `github.com/modelcontextprotocol/go-sdk` (same as searxng-mcp-go).
- **Error messages**: English.
- **All docs**: English.
- **No `# main` on SHA-pinned actions** — do NOT append `# main` or any branch
  name comment after `uses:` lines that use a commit SHA. The upstream repo
  `xlionjuan/opencode-github-actions` has no version tags and never will; a
  `# main` comment after its SHA pin would break Renovate automation. Renovate
  interprets `# main` as a version and will try to update the SHA based on the
  `main` branch, which is wrong for repos without tags. This rule applies to
  all SHA-pinned actions from repos without version tags.
