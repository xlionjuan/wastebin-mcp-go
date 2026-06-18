# Verification

- Code changes must be verified with the narrowest meaningful build/test
  commands before committing. Broaden verification for shared behavior, public
  interfaces, test infrastructure, or CI changes.
- PR agents must run local verification before opening or updating a PR or
  reporting the task complete. Choose the gate by the affected surface.
- Do not run the Go completion gate for changes that cannot affect Go code,
  dependencies, build, test, lint, or release behavior. For example, pure
  documentation changes should use narrow validation such as diff checks or
  YAML parsing.
- If `golangci-lint` is unavailable, run `go vet ./...` as the fallback static
  check and state in the PR body that the linter itself could not be run.
- If a subagent made code changes, the coordinating agent must review and verify
  those changes before committing.
- Never trust your own knowledge of version numbers, release dates, or
  specification statuses. Time-sensitive external facts must be verified against
  a current authoritative source before being stated as fact; if verification is
  unavailable, avoid the claim or state the verification gap.

## Build and Test Commands

| Command | Scope |
|---------|-------|
| `go mod verify && go mod download` | Verify and download dependencies |
| `go mod tidy && git diff --exit-code go.mod go.sum` | Check module files are tidy |
| `go build ./...` | Build all packages |
| `go test -race -shuffle=on -coverprofile=coverage.out ./...` | CI-style test run with race detector and coverage |
| `go tool govulncheck ./...` | Known vulnerability scan |
| `golangci-lint run ./...` | Lint; CI uses v2.12.2 |
| `golangci-lint fmt --diff` | Check formatting (gofumpt / gofmt / gci) |
| `go vet ./...` | Static analysis fallback |

## Completion Gate for AI Agents

Before any AI agent opens a PR, updates a PR, or reports a code-changing task
complete, it must run the gate that matches the touched surface.

For Go code, Go tests, Go dependencies, Go-related scripts, or workflow changes
that alter Go setup, Go commands, test/lint commands, or required environment
for Go execution:

- `go mod verify`
- `go mod download`
- `go mod tidy` + `git diff --exit-code go.mod go.sum`
- `go build ./...`
- `go test -race -shuffle=on -coverprofile=coverage.out ./...`
- `go tool govulncheck ./...`
- `golangci-lint run ./...`
- `golangci-lint fmt --diff`
- `go vet ./...`

For MCP tool schema changes, also verify with a CLI mode test or manual MCP
inspection after the build gate.

For workflow-only changes that do not affect Go execution, use targeted
workflow validation instead of the Go completion gate. At minimum, inspect the
diff and run `git diff --check`; also run `actionlint` or a YAML parser when
available.

Pure documentation changes (`.md` files only) do not require the build, test,
lint, or vet gates.

If a required tool is unavailable, use the documented fallback when one exists
and state the exact limitation in the PR body or final response. If the missing
tool means the completion gate could not be run, do not claim the task is fully
verified.
