package wastebin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Stage-specific sentinel errors for the path validation pipeline.
var (
	errPathTraversal           = errors.New("path traversal is not allowed")
	errPathNotAllowed          = errors.New("file path is not under any allowed path")
	errBuiltinBlockedPrefix    = errors.New("file path is in a blocked system directory")
	errBuiltinBlockedComponent = errors.New("file path contains a blocked component")
	errUserBlockedPath         = errors.New("file path is in a user-blocked directory")
	errFilePathCannotBeUsed    = errors.New("file path cannot be used")
	errPathNotFound            = errors.New("file path not found")
	errPathPermissionDenied    = errors.New("permission denied for file path")
)

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

// isUserBlocked checks the resolved path against the user-defined blocklist
// (WASTEBIN_MCP_BLOCKED_PATHS). Returns (matchedPath, true) if blocked,
// ("", false) if not blocked.
func isUserBlocked(resolvedPath string, userBlockedPaths []string) (string, bool) {
	if len(userBlockedPaths) == 0 {
		return "", false
	}

	cleaned := filepath.Clean(resolvedPath)
	for _, blocked := range userBlockedPaths {
		blocked = filepath.Clean(blocked)
		if isContainedPath(blocked, cleaned) {
			return blocked, true
		}
	}

	return "", false
}

// ──────────────────────────────────────────────
// Blocklist pipeline types
// ──────────────────────────────────────────────

// Stage is a composable path validation or transformation step.
// It receives the current path and returns the (possibly modified) path,
// or an error if validation fails.
type Stage func(path string) (string, error)

// Pipeline runs a sequence of validation stages in order.
// On the first error, execution stops immediately.
type Pipeline struct {
	stages []Stage
}

// Run executes all stages in order.
func (p *Pipeline) Run(path string) (string, error) {
	for _, stage := range p.stages {
		var sErr error

		path, sErr = stage(path)
		if sErr != nil {
			return "", sErr
		}
	}

	return path, nil
}

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
		return "", fmt.Errorf("%w (%s)", errBuiltinBlockedComponent, reason)
	}

	return path, nil
}

// stageResolvedComponentBlocked checks the cleaned resolved path for blocked
// components. Used as the component check inside ALLOWED_PATHS (Stage 2).
func stageResolvedComponentBlocked(path string) (string, error) {
	if reason, blocked := hasComponentBlockedIn(filepath.Clean(path), builtinBlockedComponents); blocked {
		return "", fmt.Errorf("%w (%s)", errBuiltinBlockedComponent, reason)
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
			return "", fmt.Errorf("%w (%s)", errBuiltinBlockedPrefix, reason)
		}
	}

	return "", fmt.Errorf("%w (%s)", errBuiltinBlockedComponent, reason)
}

// ──────────────────────────────────────────────
// validateFilePath
// ──────────────────────────────────────────────

// validateFilePath runs the five-stage path validation pipeline:
//
//	Stage 1a: Path traversal detection on the raw input (before resolution).
//	Stage 1b: Sensitive component detection on the raw input (before resolution).
//	Stage 2: ALLOWED_PATHS check — if configured and path is under one,
//	         the prefix blocklist and user blocklist are bypassed, but the
//	         sensitive component blocklist (Stage 3b) is still enforced.
//	Stage 3: BUILT-IN BLOCKLIST check (prefix + component).
//	Stage 4: USER BLOCKLIST check (WASTEBIN_MCP_BLOCKED_PATHS).
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

	// Resolve the path via EvalSymlinks.
	normalized := normalizePath(rawPath)

	resolved, err := filepath.EvalSymlinks(normalized)
	if err != nil {
		// When ALLOWED_PATHS is configured, don't leak whether a
		// path outside the sandbox exists or is accessible — the
		// caller might probe arbitrary paths.
		if len(cfg.AllowedPaths) > 0 {
			return "", errFilePathCannotBeUsed
		}

		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %w", errPathNotFound, err)
		}

		if os.IsPermission(err) {
			return "", fmt.Errorf("%w: %w", errPathPermissionDenied, err)
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

	// Stage 4: USER BLOCKLIST.
	if _, blocked := isUserBlocked(resolvedPath, cfg.BlockedPaths); blocked {
		return "", errUserBlockedPath
	}

	return resolvedPath, nil
}
