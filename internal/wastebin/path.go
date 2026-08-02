package wastebin

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Sentinel errors are defined in errors.go.

// builtinBlockedPrefixes are absolute path prefixes blocked by default.
var builtinBlockedPrefixes = []string{"/etc", "/proc", "/sys", "/dev"}

// isContainedPath reports whether target is under or equal to base.
// Both paths should be cleaned before passing to this function.
func isContainedPath(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}

	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// builtinBlockedComponents are directory/file names blocked by default
// regardless of their location in the directory tree.
var builtinBlockedComponents = []string{".ssh", ".gnupg", ".aws", ".kube", ".docker", ".git"}

// normalizePath normalizes Windows backslashes to forward slashes.
func normalizePath(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}

// hasPathTraversal checks for '..' in the normalized raw input.
// This is performed before any path resolution or sandbox translation.
func hasPathTraversal(path string) bool {
	normalized := normalizePath(path)

	for part := range strings.SplitSeq(normalized, "/") {
		if part == ".." {
			return true
		}
	}

	return false
}

// hasComponentBlockedIn reports whether any component of path matches the
// given blocked components. Path is split by "/". This is the consolidated
// component matching function that replaces the separate raw/resolved matchers.
func hasComponentBlockedIn(path string, components []string) (string, bool) {
	for part := range strings.SplitSeq(path, "/") {
		if slices.Contains(components, part) {
			return part, true
		}
	}

	return "", false
}

// hasComponentBlocked checks the normalized raw path for blocked components.
// This is performed before path resolution (EvalSymlinks), so it catches
// cases where a blocked component (e.g. .ssh) is a symlink to an unaffected
// directory name.
func hasComponentBlocked(path string) (string, bool) {
	return hasComponentBlockedIn(normalizePath(path), builtinBlockedComponents)
}

// isAllowedPath checks if a resolved (cleaned, absolute) path falls under
// one of the allowed paths. Returns true if the path is allowed.
// This is Stage 2 of the validation pipeline.
func isAllowedPath(resolvedPath string, allowedPaths []string) bool {
	cleaned := filepath.Clean(resolvedPath)
	for _, allowed := range allowedPaths {
		allowed = filepath.Clean(allowed)
		if isContainedPath(allowed, cleaned) {
			return true
		}
	}

	return false
}

// isBuiltinBlocked checks the resolved path against the built-in blocklist.
// It checks both absolute path prefixes (Stage 3a) and path components (Stage 3b).
// Returns (reason, true) if blocked, ("", false) if not blocked.
func isBuiltinBlocked(resolvedPath string) (string, bool) {
	cleaned := filepath.Clean(resolvedPath)

	// Stage 3a: Absolute path prefix match.
	for _, prefix := range builtinBlockedPrefixes {
		prefix = filepath.Clean(prefix)
		if cleaned == prefix || strings.HasPrefix(cleaned, prefix+string(filepath.Separator)) {
			return prefix, true
		}
	}

	// Stage 3b: Path component match.
	return hasComponentBlockedIn(cleaned, builtinBlockedComponents)
}

// isUserBlockedLexical checks the absolute, cleaned request path against
// the operator's original lexical entries before symlink resolution.
// This catches a path that is under a user-blocked directory that was
// created as a symlink AFTER process startup.
func isUserBlockedLexical(absRawPath string, userBlockedPaths []blockedPathEntry) (string, bool) {
	if len(userBlockedPaths) == 0 {
		return "", false
	}

	cleaned := filepath.Clean(absRawPath)
	for _, entry := range userBlockedPaths {
		if isContainedPath(entry.Lexical, cleaned) {
			return entry.Lexical, true
		}
	}

	return "", false
}

// isUserBlocked checks the resolved path against the user-defined blocklist
// (WASTEBIN_MCP_BLOCKED_PATHS). It compares against the resolved (canonical)
// form of each entry, which matches what EvalSymlinks produces at request time.
// Returns (matchedPath, true) if blocked, ("", false) if not blocked.
func isUserBlocked(resolvedPath string, userBlockedPaths []blockedPathEntry) (string, bool) {
	if len(userBlockedPaths) == 0 {
		return "", false
	}

	cleaned := filepath.Clean(resolvedPath)
	for _, entry := range userBlockedPaths {
		if isContainedPath(entry.Resolved, cleaned) {
			return entry.Resolved, true
		}
	}

	return "", false
}

// ──────────────────────────────────────────────
// Blocklist pipeline types
// ──────────────────────────────────────────────

// Stage is a composable path validation or transformation step.
type Stage func(path string) (string, error)

// BlocklistStages holds the optional builtin blocklist validation stages.
// When the builtin blocklist is disabled, all fields are nil and the
// corresponding checks are skipped without branching in the validation code.
type BlocklistStages struct {
	rawComponent      Stage // Stage 1b: pre-resolution component check
	resolvedComponent Stage // Stage 2/3b: post-resolution component-only check
	builtinFull       Stage // Stage 3: post-resolution prefix + component check
}

// newBlocklistStages creates blocklist stages from the disable flag.
// This is the only place DisableBuiltinBlocklist is consumed.
func newBlocklistStages(disabled bool) BlocklistStages {
	if disabled {
		return BlocklistStages{}
	}

	return BlocklistStages{
		rawComponent:      stageRawComponentBlocked,
		resolvedComponent: stageResolvedComponentBlocked,
		builtinFull:       stageBuiltinBlocked,
	}
}

// stageRawComponentBlocked checks the normalized raw path for blocked
// components. Used as Stage 1b (pre-resolution).
func stageRawComponentBlocked(path string) (string, error) {
	if reason, blocked := hasComponentBlockedIn(normalizePath(path), builtinBlockedComponents); blocked {
		return "", NewBlockedComponentError(reason)
	}

	return path, nil
}

// stageResolvedComponentBlocked checks the cleaned resolved path for blocked
// components. Used as the component check inside ALLOWED_PATHS (Stage 2).
func stageResolvedComponentBlocked(path string) (string, error) {
	if reason, blocked := hasComponentBlockedIn(filepath.Clean(path), builtinBlockedComponents); blocked {
		return "", NewBlockedComponentError(reason)
	}

	return path, nil
}

// stageBuiltinBlocked checks the resolved path against the full builtin
// blocklist (prefixes + components). Used as Stage 3.
func stageBuiltinBlocked(path string) (string, error) {
	reason, blocked := isBuiltinBlocked(path)
	if !blocked {
		return path, nil
	}

	for _, prefix := range builtinBlockedPrefixes {
		if reason == filepath.Clean(prefix) {
			return "", NewBlockedPrefixError(reason)
		}
	}

	return "", NewBlockedComponentError(reason)
}

// ──────────────────────────────────────────────
// validateFilePath
// ──────────────────────────────────────────────

// validateFilePath runs the six-stage path validation pipeline:
//
//	Stage 1a: Path traversal detection on the raw input (before resolution).
//	Stage 1b: Sensitive component detection on the raw input (before resolution).
//	Stage 4a: USER BLOCKLIST (lexical) — pre-resolution check against the
//	          operator's original configured paths (before EvalSymlinks).
//	          Converts relative paths to absolute via filepath.Abs.
//	          Skipped when ALLOWED_PATHS is configured (Stage 2 takes precedence).
//	          EvalSymlinks resolves the path.
//	Stage 2: ALLOWED_PATHS check — if configured and path is under one,
//	         the prefix blocklist and user blocklist are bypassed, but the
//	         sensitive component blocklist (Stage 3b) is still enforced.
//	Stage 3: BUILT-IN BLOCKLIST check (prefix + component).
//	Stage 4b: USER BLOCKLIST (resolved) — post-resolution check against the
//	          canonical resolved form of each entry.
//
// Stages 1a and 1b run on the path as received. For sandbox paths, the caller
// (readFileContent) applies traversal and component checks on the original
// sandbox path before translation, then runs validateFilePath on the
// translated path for defense in depth.
//
//nolint:nonamedreturns // Named returns improve godoc clarity
func validateFilePath(rawPath string, cfg *Config) (resolvedPath string, err error) {
	// Build blocklist stages from config. DisableBuiltinBlocklist is consumed
	// here — the stages are either present or absent.
	blk := newBlocklistStages(cfg.DisableBuiltinBlocklist)

	// Stage 1a: Path traversal detection on the raw input.
	if hasPathTraversal(rawPath) {
		return "", errPathTraversal
	}

	// Stage 1b: Sensitive component detection on the raw input, before
	// symlink resolution. This catches symlinked blocked components
	// (e.g. .ssh -> realssh) that would disappear after resolution.
	if blk.rawComponent != nil {
		_, err = blk.rawComponent(rawPath)
		if err != nil {
			return "", err
		}
	}

	// Normalize the path for resolution.
	normalized := normalizePath(rawPath)

	// Stage 4a: USER BLOCKLIST (lexical) — pre-resolution check.
	// This catches paths that are in a user-blocked directory that was
	// created as a symlink AFTER process startup, so that EvalSymlinks
	// would resolve through the symlink and lose the blocked prefix.
	// The normalized path is converted to absolute here (without resolving
	// symlinks) so that relative file_path values in CLI mode are checked
	// against the lexical blocklist before EvalSymlinks can remove the
	// blocked alias.
	// When ALLOWED_PATHS is configured, it takes precedence over the
	// user blocklist (both lexical and resolved forms), matching the
	// existing bypass behavior in Stage 2.
	if len(cfg.BlockedPaths) > 0 && len(cfg.AllowedPaths) == 0 {
		absRaw, absErr := filepath.Abs(normalized)
		if absErr != nil {
			return "", errFilePathCannotBeUsed
		}

		if _, blocked := isUserBlockedLexical(filepath.Clean(absRaw), cfg.BlockedPaths); blocked {
			return "", errUserBlockedPath
		}
	}

	// Resolve the path via EvalSymlinks.
	resolved, err := filepath.EvalSymlinks(normalized)
	if err != nil {
		// When ALLOWED_PATHS is configured, don't leak whether a
		// path outside the sandbox exists or is accessible — the
		// caller might probe arbitrary paths.
		if len(cfg.AllowedPaths) > 0 {
			return "", errFilePathCannotBeUsed
		}

		if os.IsNotExist(err) {
			return "", NewPathNotFoundError(err)
		}

		if os.IsPermission(err) {
			return "", NewPathPermissionError(err)
		}

		return "", errFilePathCannotBeUsed
	}

	resolvedPath = filepath.Clean(resolved)

	// Convert to absolute path for consistent comparison with allowlist/blocklist.
	resolvedPath, err = filepath.Abs(resolvedPath)
	if err != nil {
		return "", errFilePathCannotBeUsed
	}

	// Stage 2: ALLOWED_PATHS check.
	if len(cfg.AllowedPaths) > 0 {
		if !isAllowedPath(resolvedPath, cfg.AllowedPaths) {
			return "", errPathNotAllowed
		}

		// ALLOWED_PATHS bypasses the prefix blocklist and user blocklist,
		// but not the sensitive component blocklist.
		if blk.resolvedComponent != nil {
			_, err = blk.resolvedComponent(resolvedPath)
			if err != nil {
				return "", err
			}
		}

		return resolvedPath, nil
	}

	// Stage 3: BUILT-IN BLOCKLIST.
	if blk.builtinFull != nil {
		_, err = blk.builtinFull(resolvedPath)
		if err != nil {
			return "", err
		}
	}

	// Stage 4b: USER BLOCKLIST (resolved, post-resolution).
	if _, blocked := isUserBlocked(resolvedPath, cfg.BlockedPaths); blocked {
		return "", errUserBlockedPath
	}

	return resolvedPath, nil
}
