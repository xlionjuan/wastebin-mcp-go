package wastebin

import (
	"errors"
	"fmt"
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
)

// builtinBlockedPrefixes are absolute path prefixes blocked by default.
var builtinBlockedPrefixes = []string{"/etc", "/proc", "/sys", "/dev"}

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

// isAllowedPath checks if a resolved (cleaned, absolute) path falls under
// one of the allowed paths. Returns true if the path is allowed.
// This is Stage 2 of the validation pipeline.
func isAllowedPath(resolvedPath string, allowedPaths []string) bool {
	cleaned := filepath.Clean(resolvedPath)
	for _, allowed := range allowedPaths {
		allowed = filepath.Clean(allowed)
		if cleaned == allowed || strings.HasPrefix(cleaned, allowed+string(filepath.Separator)) {
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
	for part := range strings.SplitSeq(cleaned, string(filepath.Separator)) {
		if slices.Contains(builtinBlockedComponents, part) {
			return part, true
		}
	}

	return "", false
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
		if cleaned == blocked || strings.HasPrefix(cleaned, blocked+string(filepath.Separator)) {
			return blocked, true
		}
	}

	return "", false
}

// validateFilePath runs the four-stage path validation pipeline:
//
//	Stage 1: Path traversal detection on the raw input (before resolution).
//	Stage 2: ALLOWED_PATHS check — if configured and path is under one,
//	         blocklists are bypassed entirely.
//	Stage 3: BUILT-IN BLOCKLIST check (prefix + component) — unless
//	         cfg.DisableBuiltinBlocklist is true.
//	Stage 4: USER BLOCKLIST check (WASTEBIN_MCP_BLOCKED_PATHS).
//
// Path traversal detection (Stage 1) runs on the path as received — callers
// must apply sandbox path translation AFTER the traversal check has already
// been performed on the original sandbox path (see readFileContent).
//
//nolint:nonamedreturns // Named returns improve godoc clarity
func validateFilePath(rawPath string, cfg *Config) (resolvedPath string, err error) {
	// Stage 1: Path traversal detection on the raw input.
	if hasPathTraversal(rawPath) {
		return "", errPathTraversal
	}

	// Resolve the path via EvalSymlinks.
	normalized := normalizePath(rawPath)

	resolved, err := filepath.EvalSymlinks(normalized)
	if err != nil {
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
		if isAllowedPath(resolvedPath, cfg.AllowedPaths) {
			// ALLOWED_PATHS bypasses all blocklists.
			return resolvedPath, nil
		}

		return "", errPathNotAllowed
	}

	// Stage 3: BUILT-IN BLOCKLIST.
	if !cfg.DisableBuiltinBlocklist {
		if reason, blocked := isBuiltinBlocked(resolvedPath); blocked {
			// Determine whether it was a prefix or component match.
			for _, prefix := range builtinBlockedPrefixes {
				if reason == filepath.Clean(prefix) || reason == prefix {
					return "", fmt.Errorf("%w (%s)", errBuiltinBlockedPrefix, reason)
				}
			}

			// If not matched by prefix, it's a component match.
			return "", fmt.Errorf("%w (%s)", errBuiltinBlockedComponent, reason)
		}
	}

	// Stage 4: USER BLOCKLIST.
	if matched, blocked := isUserBlocked(resolvedPath, cfg.BlockedPaths); blocked {
		return "", fmt.Errorf("%w (%s)", errUserBlockedPath, matched)
	}

	return resolvedPath, nil
}
