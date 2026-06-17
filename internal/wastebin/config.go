package wastebin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Sentinel errors for configuration validation.
var (
	errServerURLRequired              = errors.New("WASTEBIN_SERVER_URL is required and must not be empty")
	errNegativeDefaultExpires         = errors.New("WASTEBIN_MCP_DEFAULT_EXPIRES cannot be negative")
	errMaxContentSizeTooSmall         = errors.New("WASTEBIN_MCP_MAX_CONTENT_SIZE must be at least 1")
	errSandboxMountNotAllowed         = errors.New("sandbox mount host_path is not under any allowed path")
	errInvalidDisableBuiltinBlocklist = errors.New("invalid WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST")
)

const (
	defaultExpirySeconds  = 31536000 // 1 year
	defaultMaxContentSize = 1048576  // 1 MB
)

// DefaultConfig returns Config with safe defaults.
func DefaultConfig() *Config {
	return &Config{
		ServerURL:          "",
		DefaultExpires:     defaultExpirySeconds, // 1 year
		FileReadEnabled:    true,
		AllowedPaths:       nil,
		BlockedPaths:       []string{"/etc", "/proc", "/sys", "/dev"},
		MaxContentSize:     defaultMaxContentSize, // 1 MB
		SandboxMounts:      nil,
		SandboxTransparent: false,
		Debug:              false,
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

	// Allowed paths (comma-separated, resolved with EvalSymlinks + Clean).
	if v := os.Getenv("WASTEBIN_MCP_ALLOWED_PATHS"); v != "" {
		parts := strings.SplitSeq(v, ",")
		for p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}

			resolved, err := filepath.EvalSymlinks(p)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve allowed path %q: %w", p, err)
			}

			cfg.AllowedPaths = append(cfg.AllowedPaths, filepath.Clean(resolved))
		}
	}

	// Blocked paths (comma-separated; defaults to /etc,/proc,/sys,/dev).
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
	// Resolve all blocked paths to absolute, cleaned paths.
	var resolvedBlocked []string

	for _, p := range cfg.BlockedPaths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve blocked path %q: %w", p, err)
		}

		resolvedBlocked = append(resolvedBlocked, filepath.Clean(abs))
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

		cfg.MaxContentSize = n
	}

	// Sandbox mounts.
	if v := os.Getenv("WASTEBIN_MCP_SANDBOX_MOUNTS"); v != "" {
		mounts, err := ParseSandboxMounts(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WASTEBIN_MCP_SANDBOX_MOUNTS: %w", err)
		}

		cfg.SandboxMounts = mounts

		err = resolveAndValidateMounts(cfg.SandboxMounts, cfg.AllowedPaths)
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

// resolveAndValidateMounts resolves mount host paths with EvalSymlinks and
// validates them against allowed paths.
func resolveAndValidateMounts(mounts []SandboxMount, allowedPaths []string) error {
	for i := range mounts {
		resolved, err := filepath.EvalSymlinks(mounts[i].HostPath)
		if err != nil {
			return fmt.Errorf(
				"failed to resolve sandbox mount host path %q: %w",
				mounts[i].HostPath, err,
			)
		}

		mounts[i].HostPath = filepath.Clean(resolved)
	}

	if len(allowedPaths) > 0 {
		for _, m := range mounts {
			if !isAllowedPath(m.HostPath, allowedPaths) {
				return fmt.Errorf(
					"%w: host_path %q is not under any allowed path; "+
						"each mount host_path must be covered by WASTEBIN_MCP_ALLOWED_PATHS",
					errSandboxMountNotAllowed, m.HostPath,
				)
			}
		}
	}

	return nil
}
