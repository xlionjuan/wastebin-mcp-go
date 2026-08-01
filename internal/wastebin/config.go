package wastebin

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Sentinel errors are defined in errors.go.

const (
	defaultExpirySeconds  = 31536000 // 1 year
	defaultMaxContentSize = 1048576  // 1 MB

	// MaxContentSizeLimit is the largest accepted value of
	// WASTEBIN_MCP_MAX_CONTENT_SIZE (256 MiB). It bounds the MCP first-message
	// transport gate that is derived from the content size, keeping the gate's
	// int arithmetic (conversion plus the 64 KiB envelope allowance plus the
	// trailing-newline byte) safe on 32-bit targets and preventing a
	// configuration mistake from triggering a large startup allocation.
	MaxContentSizeLimit int64 = 256 << 20
)

// DefaultConfig returns Config with safe defaults.
func DefaultConfig() *Config {
	return &Config{
		ServerURL:             "",
		DefaultExpires:        defaultExpirySeconds, // 1 year
		FileReadEnabled:       true,
		AllowedPaths:          nil,
		BlockedPaths:          nil,
		MaxContentSize:        defaultMaxContentSize, // 1 MB
		SandboxMounts:         nil,
		SandboxTransparent:    false,
		AllowInsecurePassword: false,
		Debug:                 false,
	}
}

// ConfigFromEnv reads and validates config from environment variables.
// Returns validated Config or error.
func ConfigFromEnv() (*Config, error) {
	cfg := DefaultConfig()

	// Server URL (required).
	cfg.ServerURL = os.Getenv("WASTEBIN_SERVER_URL")
	if cfg.ServerURL == "" {
		return nil, errServerURLRequired
	}

	// Default expires.
	if v := os.Getenv("WASTEBIN_MCP_DEFAULT_EXPIRES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WASTEBIN_MCP_DEFAULT_EXPIRES: %w", err)
		}

		if n < 0 {
			return nil, errNegativeDefaultExpires
		}

		cfg.DefaultExpires = n
	}

	// File read enabled.
	if v := os.Getenv("WASTEBIN_MCP_FILE_READ_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WASTEBIN_MCP_FILE_READ_ENABLED: %w", err)
		}

		cfg.FileReadEnabled = b
	}

	// Allowed paths (comma-separated absolute paths, resolved with EvalSymlinks + Clean).
	if v := os.Getenv("WASTEBIN_MCP_ALLOWED_PATHS"); v != "" {
		parts := strings.SplitSeq(v, ",")
		for p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}

			resolved, err := resolveConfiguredPath("WASTEBIN_MCP_ALLOWED_PATHS", p, false)
			if err != nil {
				return nil, err
			}

			cfg.AllowedPaths = append(cfg.AllowedPaths, resolved)
		}
	}

	// Blocked paths (comma-separated absolute paths; empty by default — the
	// built-in blocklist handles system directories separately).
	if v := os.Getenv("WASTEBIN_MCP_BLOCKED_PATHS"); v != "" {
		cfg.BlockedPaths = nil

		parts := strings.SplitSeq(v, ",")
		for p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}

			cfg.BlockedPaths = append(cfg.BlockedPaths, p)
		}
	}
	// Resolve all blocked paths — first try EvalSymlinks (matching
	// validateFilePath's resolution), then fall back to Clean for
	// non-existent absolute paths that may exist later.
	var resolvedBlocked []string

	for _, p := range cfg.BlockedPaths {
		resolved, err := resolveConfiguredPath("WASTEBIN_MCP_BLOCKED_PATHS", p, true)
		if err != nil {
			return nil, err
		}

		resolvedBlocked = append(resolvedBlocked, resolved)
	}

	cfg.BlockedPaths = resolvedBlocked

	// Max content size.
	if v := os.Getenv("WASTEBIN_MCP_MAX_CONTENT_SIZE"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid WASTEBIN_MCP_MAX_CONTENT_SIZE: %w", err)
		}

		if n < 1 {
			return nil, errMaxContentSizeTooSmall
		}

		if n > MaxContentSizeLimit {
			return nil, fmt.Errorf(
				"%w (%d exceeds the maximum of %d)",
				errMaxContentSizeTooLarge, n, MaxContentSizeLimit,
			)
		}

		cfg.MaxContentSize = n
	}

	// Sandbox mounts.
	if v := os.Getenv("WASTEBIN_MCP_SANDBOX_MOUNTS"); v != "" {
		mounts, err := ParseSandboxMounts(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WASTEBIN_MCP_SANDBOX_MOUNTS: %w", err)
		}

		cfg.SandboxMounts, err = resolveAndValidateMounts(mounts, cfg.AllowedPaths)
		if err != nil {
			return nil, err
		}
	}

	// Sandbox transparent.
	if v := os.Getenv("WASTEBIN_MCP_SANDBOX_TRANSPARENT"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WASTEBIN_MCP_SANDBOX_TRANSPARENT: %w", err)
		}

		cfg.SandboxTransparent = b
	}

	// Disable built-in blocklist.
	if v := os.Getenv("WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", errInvalidDisableBuiltinBlocklist, v)
		}

		cfg.DisableBuiltinBlocklist = b
	}

	// Allow insecure password over HTTP.
	if v := os.Getenv("WASTEBIN_MCP_ALLOW_INSECURE_PASSWORD"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WASTEBIN_MCP_ALLOW_INSECURE_PASSWORD: %w", err)
		}

		cfg.AllowInsecurePassword = b
	}

	// Debug.
	if v := os.Getenv("DEBUG"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("invalid DEBUG: %w", err)
		}

		cfg.Debug = b
	}

	return cfg, nil
}

func resolveConfiguredPath(envName, p string, allowMissing bool) (string, error) {
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("%w: %s entry %q", errConfiguredPathNotAbsolute, envName, p)
	}

	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		if allowMissing {
			return filepath.Clean(p), nil
		}

		return "", fmt.Errorf("failed to resolve %s path %q: %w", envName, p, err)
	}

	return filepath.Clean(resolved), nil
}

// resolveAndValidateMounts resolves mount host paths with EvalSymlinks and
// validates them against allowed paths.
func resolveAndValidateMounts(
	mounts []SandboxMount,
	allowedPaths []string,
) ([]SandboxMount, error) {
	for i := range mounts {
		resolved, err := filepath.EvalSymlinks(mounts[i].HostPath)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to resolve sandbox mount host path %q: %w",
				mounts[i].HostPath, err,
			)
		}

		mounts[i].HostPath = filepath.Clean(resolved)
	}

	validated := make([]SandboxMount, 0, len(mounts))
	for _, m := range mounts {
		if len(allowedPaths) > 0 && !isAllowedPath(m.HostPath, allowedPaths) {
			slog.Warn("sandbox mount host_path not under any allowed path; skipping mount",
				"host_path", m.HostPath,
				"sandbox_path", m.SandboxPath,
			)

			continue
		}

		validated = append(validated, m)
	}

	return validated, nil
}
