# ADR 005: Typed Error Model with Unified Wording

**Status:** Accepted
**Last updated:** 2026-08-02
**Supersedes:** Issue #208 (docs drift) — absorbed into this ADR.

## Context

`docs/MCP_TOOLS.md` documented error messages that drifted from the actual
runtime output. Root cause: error messages were assembled piecemeal.
`internal/wastebin/errors.go` provided sentinel constants, while five wrapping
sites (`checkContentSize`, `readFileContent`, `validateFilePath`,
`normalizeExtension`, `translateHTTPError`) appended dynamic suffixes with
ad-hoc `fmt.Errorf` calls. Documentation had to hand-copy every combination, so
drift was inevitable.

The sibling project `searxng-mcp-go` solves this with typed errors
(`ValidationError`, `SearXNGError`, `HTMLResponseError`): the output format is
produced centrally by each type's `Error()` method, and contract tests pin the
exact strings.

## Decision

### Typed errors for data-carrying errors

Errors that carry dynamic data are represented by typed errors. Each typed
error wraps the legacy sentinel it replaces and implements `Unwrap()` and
`Is()`, so existing `errors.Is(err, errSentinel)` assertions keep working
through the chain. `PathNotFoundError` and `PathPermissionError` additionally
join the underlying OS error into `Unwrap()` (`errors.Join(sentinel,
underlying)`), preserving `errors.Is(err, os.ErrNotExist)` /
`errors.Is(err, os.ErrPermission)` matching that callers relied on before the
refactor. Errors that carry no data remain sentinel errors used with
`errors.Is`.

| Type | Fields | Replaces |
|---|---|---|
| `ContentTooLargeError` | `Size`, `Limit` | `errContentTooLarge` + wrapping sites |
| `InvalidExtensionError` | `Ext` | `errInvalidExtension` + `normalizeExtension` |
| `PathNotFoundError` | `Underlying` | `errPathNotFound` + `validateFilePath` |
| `PathPermissionError` | `Underlying` | `errPathPermissionDenied` + `validateFilePath` |
| `SandboxNoMatchError` | `Path` | `errSandboxTranslationNoMatch` + `readFileContent` |
| `BlockedComponentError` | `Reason` | `errBuiltinBlockedComponent` + wrapping sites |
| `BlockedPrefixError` | `Reason` | `errBuiltinBlockedPrefix` + `stageBuiltinBlocked` |
| `HTTPError` | `StatusCode`, `Body` | `translateHTTPError` branches (403/413/422/unknown) |

### Unified wording — BREAKING

Every error message follows the `"<problem>; <next-step guidance>"` convention,
with dynamic values appended as `: <value>`. This change added guidance to every
family that previously carried none: HTTP 403, HTTP 422, unknown HTTP status,
redirects, response-too-large, the path and file-mode sentinels
(`errPathTraversal`, `errBuiltinBlockedPrefix`, `errBuiltinBlockedComponent`,
`errUserBlockedPath`, `errFileNotText`, `errFileReadDisabled`), response
validation (`errInvalidWastebinResponse` and its parse failures), expiration
errors, and hostname resolution. Consumers that match on exact error strings
must update.

### No error codes

Error codes are intentionally out of scope for this change. They can be added
additively later by giving each typed error a `Code` field without changing the
wording convention.

### One-shot migration

The migration happens in a single change. No intermediate mixed-wording state
exists: every wrapping site produces either a typed error or a sentinel that
already follows the unified format.

### Contract tests

Each typed error's `Error()` output is pinned to the exact string in
`internal/wastebin/errors_test.go` (mirroring searxng's
`TestMCPErrors_InvalidInputs` approach). Documentation describes the message
shape with `<placeholder>` markers instead of claiming exact strings.

## Consequences

**Positive:**

- Error output is produced centrally by each type's `Error()` method; there is
  no per-call-site string assembly to drift.
- `errors.Is` assertions against the legacy sentinels keep working through the
  typed errors' `Unwrap()`/`Is()`.
- Errors that previously gave no next-step guidance now tell the agent what to
  do.
- Docs describe a convention plus placeholders, so a wording tweak no longer
  requires a documentation rewrite.

**Negative:**

- Breaking change for consumers matching on exact error strings (HTTP unknown
  status, content-size wording, and messages that gained guidance).
- Two sources of truth for wording — the `Error()` methods and the docs tables —
  still exist, but contract tests pin the code side and the docs tables use
  placeholders.

## Related documents

- `internal/wastebin/errors.go` — sentinels and typed errors
- `internal/wastebin/errors_test.go` — contract tests pinning `Error()` output
- `docs/MCP_TOOLS.md` — error section rewritten as convention + placeholders
- `docs/adr/002-path-validation.md` — updated stage error wording
- `CONTEXT.md` — error handling summary
