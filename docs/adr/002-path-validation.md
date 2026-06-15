# ADR 002: Path Validation Architecture

**Status:** Active  
**Last updated:** 2026-06-10  
**Supersedes:** Earlier draft with single-layer blocklist

## Context

The `file_path` parameter in `create_paste` accepts a local filesystem path.
Without proper validation, a malicious or buggy agent could read arbitrary files
via path traversal (`../`) or by requesting sensitive system paths.

The original implementation had a single `isPathAllowed` function that checked
both a blocklist and an optional allowlist. This design had several issues:

1. No path-traversal detection — `../` in the raw input was resolved by
   `filepath.Clean` before the blocklist check, meaning traversals that
   landed outside blocked paths would succeed silently.
2. No concept of "sensitive path components" (e.g. `.ssh`, `.gnupg`).
   Only absolute-path prefixes like `/etc` were blocked.
3. The blocklist was a single list with no distinction between built-in
   defaults and user additions, making it impossible to have separate error
   messages or disable one without the other.
4. No Windows path separator normalisation.

## Decision

We separate path validation into four **independent, composable** stages, each
implemented as its own function. The stages are evaluated in strict order:

```
file_path (raw user input)
    │
    ├── (1) PATH TRAVERSAL DETECTION
    │       Rejects `..` / `../` patterns in the raw input.
    │       Independent from all path resolution.
    │       Error: "path traversal is not allowed"
    │
    ├── (2) ALLOWED_PATHS (user whitelist) — highest priority
    │       If configured, the resolved path MUST be under one of these.
    │       Path is resolved via filepath.EvalSymlinks before the check.
    │       Only absolute paths accepted.
    │       Error: "file path is not under any allowed path"
    │
    ├── (3) BUILT-IN BLOCKLIST (hardcoded defaults)
    │       Two independent sub-checks, both evaluated:
    │         (3a) Absolute path prefix match
    │              Checks resolved absolute path against:
    │              /etc, /proc, /sys, /dev
    │              Error: "file path is in a blocked system directory (...)"
    │         (3b) Path component match
    │              Checks each path component (directory name) in the
    │              resolved path against sensitive patterns like:
    │              .ssh, .gnupg
    │              Error: "file path contains a blocked component (...)"
    │       Disabled by setting WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST=true
    │       Both sub-checks share the same disable flag.
    │
    ├── (4) USER BLOCKLIST (WASTEBIN_MCP_BLOCKED_PATHS env var)
    │       User-defined list of absolute path prefixes.
    │       Resolved via filepath.Abs + Clean before matching.
    │       Error: "file path is in a user-blocked directory (...)"
    │
    └── All passed → file is allowed
```

### Path types by mode

| Mode | file_path input | Base directory |
|------|----------------|----------------|
| **MCP mode** | Absolute path (e.g. `/home/user/doc.txt`) | N/A — paths are always absolute |
| **CLI mode** | Absolute or relative path | Relative paths are resolved against `$PWD` at invocation time; absolute paths are used as-is |

Both modes apply the **same four-stage validation pipeline** after the path is
resolved to an absolute form. CLI mode is not exempt from path validation.

### Key design principles

1. **ALLOWED_PATHS bypasses all blocklists.** If the resolved path is under
   an allowed path, it is accepted immediately regardless of blocklist
   matches. This prevents blocklists from interfering with legitimate
   file reads in configured sandbox directories.

2. **Path traversal is caught BEFORE resolution.** The raw input is checked
   for `..` components before any path resolution or symlink evaluation.
   This prevents `..` from being used to reach sensitive paths even when
   the final resolved path would pass the blocklist checks.

3. **Built-in blocklist vs user blocklist are separate concepts** with
   separate error messages, so the user knows exactly which rule rejected
   their path.

4. **Windows support:** All file paths are normalised at the earliest
   stage: `\` is replaced with `/` before any processing. This ensures
   that path component matching, traversal detection, and path resolution
   work uniformly regardless of OS.

### Rejected alternatives

- **Single blocklist with mixed entries:** Rejected because it conflates
  "sensitive system directory" (absolute prefix) with "sensitive file
  name" (path component), making error messages vague and the disable
  semantics ambiguous.
- **Path traversal only via filepath.Clean/EvalSymlinks:** Rejected
  because Clean resolves `..` before the blocklist sees the path, making
  traversal detection dependent on path resolution rather than being an
  independent guard.
- **No Windows support:** Rejected because the MCP server may run on
  Windows hosts in future.

### Windows strategy

The Go `path/filepath` package is platform-aware. By normalising `\` to
`/` at the input stage and using `filepath.FromSlash()` when interacting
with the OS, we get cross-platform behaviour without platform-specific
code paths.

Key points:
- `\` → `/` normalisation happens once, before any validation.
- `filepath.Clean`, `filepath.Abs`, `filepath.EvalSymlinks` use the
  OS-native separator internally — no need to change their behaviour.
- Path component matching splits on `/` (after normalisation), so it
  works transparently on all platforms.

## Consequences

**Positive:**
- Clear separation of concerns — each stage is independently testable.
- ALLOWED_PATHS cannot be accidentally circumvented by blocklist entries.
- Path traversal is caught early, before resolution.
- Cross-platform support without platform-specific code.

**Negative:**
- More validation code than the original single-function approach.
- Path component matching adds a new class of blocklist entries that
  could surprise users who expect only absolute-path matching.
- EvalSymlinks in the ALLOWED_PATHS stage requires the file to exist;
  non-existent files under an allowed path will be rejected.

## Implementation notes

- All four stages are pure functions operating on strings/paths —
  no external dependencies.
- The `Config` struct gains: `DisableBuiltinBlocklist bool` field.
- The `WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST` env var is parsed as bool.
- Built-in blocked path prefixes and blocked path components are defined
  as package-level slices:
  ```go
  var builtinBlockedPrefixes = []string{
      "/etc", "/proc", "/sys", "/dev",
  }
  var builtinBlockedComponents = []string{
      ".ssh", ".gnupg",
  }
  ```
- Error sentinel values for each stage, enabling tests to assert the
  exact rejection reason.

## Related documents

- `CONTEXT.md` — project domain context
- `AGENTS.md` — agent behaviour constraints
