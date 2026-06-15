package wastebin //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()

	if cfg.ServerURL != "" {
		t.Errorf("expected empty ServerURL, got %q", cfg.ServerURL)
	}

	if cfg.DefaultExpires != 31536000 {
		t.Errorf("expected DefaultExpires 31536000, got %d", cfg.DefaultExpires)
	}

	if !cfg.FileReadEnabled {
		t.Error("expected FileReadEnabled to be true")
	}

	if cfg.AllowedPaths != nil {
		t.Errorf("expected nil AllowedPaths, got %v", cfg.AllowedPaths)
	}

	if len(cfg.BlockedPaths) != 4 {
		t.Errorf("expected 4 BlockedPaths, got %d: %v", len(cfg.BlockedPaths), cfg.BlockedPaths)
	}

	if cfg.MaxContentSize != 1048576 {
		t.Errorf("expected MaxContentSize 1048576, got %d", cfg.MaxContentSize)
	}

	if cfg.SandboxMounts != nil {
		t.Errorf("expected nil SandboxMounts, got %v", cfg.SandboxMounts)
	}

	if cfg.SandboxTransparent {
		t.Error("expected SandboxTransparent to be false")
	}

	if cfg.Debug {
		t.Error("expected Debug to be false")
	}
}

func TestConfigFromEnv_Defaults(t *testing.T) {
	t.Setenv("WASTEBIN_MCP_SERVER_URL", "https://bin.example.com")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ServerURL != "https://bin.example.com" {
		t.Errorf("got %q", cfg.ServerURL)
	}

	if cfg.DefaultExpires != 31536000 {
		t.Errorf("got %d", cfg.DefaultExpires)
	}

	if !cfg.FileReadEnabled {
		t.Error("FileReadEnabled should be true")
	}

	if len(cfg.AllowedPaths) != 0 {
		t.Errorf("AllowedPaths should be empty, got %v", cfg.AllowedPaths)
	}

	if len(cfg.BlockedPaths) != 4 {
		t.Errorf("expected 4 BlockedPaths, got %d", len(cfg.BlockedPaths))
	}

	if cfg.MaxContentSize != 1048576 {
		t.Errorf("got %d", cfg.MaxContentSize)
	}

	if cfg.SandboxTransparent {
		t.Error("SandboxTransparent should be false")
	}
}

func TestConfigFromEnv_AllSet(t *testing.T) {
	t.Setenv("WASTEBIN_MCP_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_DEFAULT_EXPIRES", "3600")
	t.Setenv("WASTEBIN_MCP_FILE_READ_ENABLED", "false")
	t.Setenv("WASTEBIN_MCP_ALLOWED_PATHS", "/tmp")
	t.Setenv("WASTEBIN_MCP_BLOCKED_PATHS", "/home")
	t.Setenv("WASTEBIN_MCP_MAX_CONTENT_SIZE", "512000")
	t.Setenv("WASTEBIN_MCP_SANDBOX_TRANSPARENT", "true")
	t.Setenv("DEBUG", "true")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ServerURL != "https://bin.example.com" {
		t.Errorf("got %q", cfg.ServerURL)
	}

	if cfg.DefaultExpires != 3600 {
		t.Errorf("got %d", cfg.DefaultExpires)
	}

	if cfg.FileReadEnabled {
		t.Error("FileReadEnabled should be false")
	}

	if len(cfg.AllowedPaths) != 1 {
		t.Fatalf("expected 1 AllowedPaths, got %d", len(cfg.AllowedPaths))
	}
	// /tmp should resolve to something ending with /tmp.
	if !filepath.IsAbs(cfg.AllowedPaths[0]) {
		t.Errorf("AllowedPath should be absolute, got %q", cfg.AllowedPaths[0])
	}

	if len(cfg.BlockedPaths) != 1 {
		t.Fatalf("expected 1 BlockedPath, got %d", len(cfg.BlockedPaths))
	}

	if cfg.BlockedPaths[0] != "/home" {
		t.Errorf("got %q, want %q", cfg.BlockedPaths[0], "/home")
	}

	if cfg.MaxContentSize != 512000 {
		t.Errorf("got %d", cfg.MaxContentSize)
	}

	if !cfg.SandboxTransparent {
		t.Error("SandboxTransparent should be true")
	}

	if !cfg.Debug {
		t.Error("Debug should be true")
	}
}

func TestConfigFromEnv_MissingServerURL(t *testing.T) {
	// Ensure no env var is set.
	t.Setenv("WASTEBIN_MCP_SERVER_URL", "")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for missing ServerURL")
	}

	if !errors.Is(err, errServerURLRequired) {
		t.Errorf("got %v, want %v", err, errServerURLRequired)
	}
}

func TestConfigFromEnv_InvalidDefaultExpires(t *testing.T) {
	t.Setenv("WASTEBIN_MCP_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_DEFAULT_EXPIRES", "not-a-number")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid default expires")
	}
}

func TestConfigFromEnv_NegativeDefaultExpires(t *testing.T) {
	t.Setenv("WASTEBIN_MCP_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_DEFAULT_EXPIRES", "-1")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for negative default expires")
	}
}

func TestConfigFromEnv_InvalidFileReadEnabled(t *testing.T) {
	t.Setenv("WASTEBIN_MCP_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_FILE_READ_ENABLED", "maybe")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid bool")
	}
}

func TestConfigFromEnv_InvalidMaxContentSize(t *testing.T) {
	t.Setenv("WASTEBIN_MCP_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_MAX_CONTENT_SIZE", "large")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid max content size")
	}
}

func TestConfigFromEnv_ZeroMaxContentSize(t *testing.T) {
	t.Setenv("WASTEBIN_MCP_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_MAX_CONTENT_SIZE", "0")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for zero max content size")
	}
}

func TestConfigFromEnv_BlockedPathsWhitespace(t *testing.T) {
	t.Setenv("WASTEBIN_MCP_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_BLOCKED_PATHS", "/etc, /proc, /sys")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.BlockedPaths) != 3 {
		t.Fatalf("expected 3 BlockedPaths, got %d: %v", len(cfg.BlockedPaths), cfg.BlockedPaths)
	}
}

func TestConfigFromEnv_AllowedPathsSymlink(t *testing.T) {
	// Use a real directory that exists.
	tmpDir := t.TempDir()

	subDir := filepath.Join(tmpDir, "sub")

	err := os.Mkdir(subDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}
	// Create a symlink to /tmp.
	linkDir := filepath.Join(tmpDir, "link")

	err = os.Symlink(subDir, linkDir)
	if err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	t.Setenv("WASTEBIN_MCP_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_ALLOWED_PATHS", linkDir)

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.AllowedPaths) != 1 {
		t.Fatalf("expected 1 AllowedPath, got %d", len(cfg.AllowedPaths))
	}
	// The resolved path should be the real path, not the symlink.
	if cfg.AllowedPaths[0] != filepath.Clean(subDir) {
		t.Errorf("expected %q, got %q", filepath.Clean(subDir), cfg.AllowedPaths[0])
	}
}

func TestConfigFromEnv_SandboxMountValidation(t *testing.T) {
	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("WASTEBIN_MCP_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_ALLOWED_PATHS", allowedDir)
	t.Setenv("WASTEBIN_MCP_SANDBOX_MOUNTS", allowedDir+":/workspace")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.SandboxMounts) != 1 {
		t.Fatalf("expected 1 SandboxMount, got %d", len(cfg.SandboxMounts))
	}

	if cfg.SandboxMounts[0].HostPath != allowedDir {
		t.Errorf("got %q, want %q", cfg.SandboxMounts[0].HostPath, allowedDir)
	}

	if cfg.SandboxMounts[0].SandboxPath != "/workspace" {
		t.Errorf("got %q, want %q", cfg.SandboxMounts[0].SandboxPath, "/workspace")
	}
}

func TestConfigFromEnv_SandboxMountNotInAllowed(t *testing.T) {
	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	otherDir := filepath.Join(tmpDir, "other")

	err = os.Mkdir(otherDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("WASTEBIN_MCP_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_ALLOWED_PATHS", allowedDir)
	t.Setenv("WASTEBIN_MCP_SANDBOX_MOUNTS", otherDir+":/workspace")

	_, err = ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for mount not in allowed paths")
	}
}

func TestConfigFromEnv_InvalidSandboxTransparent(t *testing.T) {
	t.Setenv("WASTEBIN_MCP_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_SANDBOX_TRANSPARENT", "nope")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid bool")
	}
}

func TestConfigFromEnv_InvalidDebug(t *testing.T) {
	t.Setenv("WASTEBIN_MCP_SERVER_URL", "https://bin.example.com")
	t.Setenv("DEBUG", "maybe")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid debug bool")
	}
}

func TestConfigFromEnv_InvalidSandboxMounts(t *testing.T) {
	t.Setenv("WASTEBIN_MCP_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_SANDBOX_MOUNTS", "invalid-format")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid sandbox mounts")
	}
}

// The isAllowedPath function is tested in path_test.go.
