# ADR 002: Path Validation Architecture

**Status:** Active  
**Last updated:** 2026-06-16  
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
    │       When a path matches, continues to (3b) component check.
    │       Error: "file path is not under any allowed path"
    │
    ├── (3) BUILT-IN BLOCKLIST (hardcoded defaults)
    │       Two independent sub-checks:
    │         (3a) Absolute path prefix match
    │              Checks resolved absolute path against:
    │              /etc, /proc, /sys, /dev
    │              Error: "file path is in a blocked system directory (...)"
    │              Bypassed by ALLOWED_PATHS (prefixes are location, not
    │              sensitivity).
    │         (3b) Path component match
    │              Checks each path component (directory name) in the
    │              resolved path against sensitive patterns like:
    │              .ssh, .gnupg, .aws, .kube, .docker, .git
    │              Error: "file path contains a blocked component (...)"
    │              Enforced even inside ALLOWED_PATHS (per B2 exception).
    │       Disabled by setting WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST=true
    │       Both sub-checks share the same disable flag.
    │
    ├── (4) USER BLOCKLIST (WASTEBIN_MCP_BLOCKED_PATHS env var)
    │       User-defined list of absolute path prefixes.
    │       Resolved via filepath.Abs + Clean before matching.
    │       Error: "file path is in a user-blocked directory (...)"
    │       Bypassed by ALLOWED_PATHS.
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

1. **ALLOWED_PATHS bypasses the prefix blocklist and user blocklist, but not
   the sensitive component blocklist (B2 exception).** If the resolved path
   is under an allowed path, the sensitive component blocklist is still
   checked before accepting the path. This prevents explicit ALLOWED_PATHS
   entries from accidentally exposing credential directories (`.ssh`,
   `.gnupg`, `.aws`, `.kube`, `.docker`, `.git`) while still allowing
   system-directory prefixes like `/etc/nginx` to be used. See
   [Breaking change B2](#breaking-change-b2-component-blocklist-inside-allowed_paths)
   for details.

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
- ALLOWED_PATHS cannot be accidentally circumvented by blocklist entries
  (prefix and user), while the component blocklist provides defence in depth
  against credential exposure even inside allowed directories.
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
      ".ssh", ".gnupg", ".aws", ".kube", ".docker", ".git",
  }
  ```
- Error sentinel values for each stage, enabling tests to assert the
  exact rejection reason.

## Breaking change B2: Component blocklist inside ALLOWED_PATHS

**Date:** 2026-06-16  
**ID:** B2

### Change

Previously, ALLOWED_PATHS bypassed all blocklist stages entirely. After B2,
the sensitive component blocklist (Stage 3b) is checked even when a path
falls under an allowed directory. Only the system directory prefix blocklist
(Stage 3a) and the user blocklist (Stage 4) remain bypassed.

### Rationale

The sensitive component blocklist protects credential files (`.ssh`,
`.gnupg`, `.aws`, `.kube`, `.docker`, `.git`) from accidental exposure. A
user who configures `ALLOWED_PATHS=/home/user` probably does not intend to
make `~/.ssh/id_rsa` readable. Enforcing the component blocklist inside
ALLOWED_PATHS provides defense in depth without breaking legitimate use
cases (e.g., `ALLOWED_PATHS=/etc/nginx` for reading nginx configs).

### Migration

Existing configurations that deliberately place sensitive directories under
ALLOWED_PATHS must either:
1. Move the sensitive data outside the sensitive directory name, or
2. Disable the built-in blocklist entirely with
   `WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST=true`.

### Affected stages

| Stage | Bypassed by ALLOWED_PATHS? |
|-------|---------------------------|
| 3a — Prefix blocklist | ✅ Yes (unchanged) |
| 3b — Component blocklist | ❌ No (changed) |
| 4 — User blocklist | ✅ Yes (unchanged) |

## Post-validation TOCTOU protection (openat+O_NOFOLLOW)

While ADR 002 describes the validation pipeline (Stages 1-4), the validation
only determines *whether* a path is allowed. The actual file open is a separate
step with its own security properties.

### Implementation

After validation passes, the file is opened via the `openFileResolved` function
in `internal/wastebin/open.go`:

1. **Trusted root**: `/` is opened with `os.O_RDONLY|unix.O_NOFOLLOW` as a
   pinned directory fd. (The root directory cannot be a symlink on Linux, but
   `O_NOFOLLOW` is applied defensively.)

2. **Component walk**: Each path component is opened relative to the previous
   one using `unix.Openat(parentFd, component, O_RDONLY|O_NOFOLLOW|O_CLOEXEC,
   0)`. Intermediate components are verified to be directories via `Fstat`.

3. **Result**: The final component is returned as an `*os.File`. If any
   component is (or becomes) a symlink, `openat` returns `ELOOP` instead of
   following it.

### Security model

- **Pre-validation (EvalSymlinks)**: Eliminates symlink-based evasion of the
  allowlist/blocklist at validation time.
- **Post-validation (openat+O_NOFOLLOW)**: Eliminates TOCTOU symlink-swap
  attacks where an attacker replaces a validated directory component with a
  symlink between validation and the file open.

Without the post-validation step, a concurrent attacker could:
- Create a legitimate path `/home/user/allowed/file.txt`
- Wait for validation to pass
- Replace `/home/user/allowed` with a symlink to `/etc/passwd`
- The file open would follow the symlink and read the blocked path

With `openat+O_NOFOLLOW`, Step 3 causes `ELOOP` — the open fails rather than
following the swapped symlink.

### Related documents

- `internal/wastebin/open.go` — full implementation
- `CONTEXT.md` — project domain context with file-read pipeline description
- `docs/MCP_TOOLS.md` — MCP tool documentation with security notes
- `AGENTS.md` — agent behaviour constraints
