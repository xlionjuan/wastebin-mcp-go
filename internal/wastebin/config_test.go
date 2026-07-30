package wastebin //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		t.Errorf("expected 4 BlockedPaths, got %d", len(cfg.BlockedPaths))
	}

	// Default blocked paths should have lexical == resolved.
	for i, entry := range cfg.BlockedPaths {
		if entry.Lexical != entry.Resolved {
			t.Errorf("BlockedPaths[%d]: lexical %q != resolved %q", i, entry.Lexical, entry.Resolved)
		}
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
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")

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

	if len(cfg.BlockedPaths) != 0 {
		t.Errorf("expected 0 BlockedPaths, got %d: %v", len(cfg.BlockedPaths), cfg.BlockedPaths)
	}

	if cfg.MaxContentSize != 1048576 {
		t.Errorf("got %d", cfg.MaxContentSize)
	}

	if cfg.SandboxTransparent {
		t.Error("SandboxTransparent should be false")
	}
}

func TestConfigFromEnv_AllSet(t *testing.T) {
	// Create a temp dir with a symlink to test path resolution.
	tmpDir := t.TempDir()
	realDir := filepath.Join(tmpDir, "real")

	err := os.Mkdir(realDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	linkDir := filepath.Join(tmpDir, "link")

	err = os.Symlink(realDir, linkDir)
	if err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_DEFAULT_EXPIRES", "3600")
	t.Setenv("WASTEBIN_MCP_FILE_READ_ENABLED", "false")
	t.Setenv("WASTEBIN_MCP_ALLOWED_PATHS", "/tmp")
	t.Setenv("WASTEBIN_MCP_BLOCKED_PATHS", linkDir)
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

	wantBlocked, err := filepath.EvalSymlinks(linkDir)
	if err != nil {
		t.Fatalf("failed to resolve symlink: %v", err)
	}

	wantBlocked = filepath.Clean(wantBlocked)
	if cfg.BlockedPaths[0].Resolved != wantBlocked {
		t.Errorf("got resolved %q, want %q", cfg.BlockedPaths[0].Resolved, wantBlocked)
	}

	if cfg.BlockedPaths[0].Lexical != filepath.Clean(linkDir) {
		t.Errorf("got lexical %q, want %q", cfg.BlockedPaths[0].Lexical, filepath.Clean(linkDir))
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
	t.Setenv("WASTEBIN_SERVER_URL", "")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for missing ServerURL")
	}

	if !errors.Is(err, errServerURLRequired) {
		t.Errorf("got %v, want %v", err, errServerURLRequired)
	}
}

func TestConfigFromEnv_InvalidDefaultExpires(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_DEFAULT_EXPIRES", "not-a-number")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid default expires")
	}
}

func TestConfigFromEnv_NegativeDefaultExpires(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_DEFAULT_EXPIRES", "-1")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for negative default expires")
	}
}

func TestConfigFromEnv_InvalidFileReadEnabled(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_FILE_READ_ENABLED", "maybe")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid bool")
	}
}

func TestConfigFromEnv_InvalidMaxContentSize(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_MAX_CONTENT_SIZE", "large")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid max content size")
	}
}

func TestConfigFromEnv_ZeroMaxContentSize(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_MAX_CONTENT_SIZE", "0")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for zero max content size")
	}
}

func TestConfigFromEnv_BlockedPathsWhitespace(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_BLOCKED_PATHS", "/etc, /proc, /sys")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.BlockedPaths) != 3 {
		t.Fatalf("expected 3 BlockedPaths, got %d: %v", len(cfg.BlockedPaths), cfg.BlockedPaths)
	}

	if cfg.BlockedPaths[0].Lexical != "/etc" {
		t.Errorf("expected lexical /etc, got %q", cfg.BlockedPaths[0].Lexical)
	}
}

func TestConfigFromEnv_AllowedPathsRejectsRelative(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_ALLOWED_PATHS", "workspace")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for relative allowed path")
	}

	if !errors.Is(err, errConfiguredPathNotAbsolute) {
		t.Errorf("expected %v, got: %v", errConfiguredPathNotAbsolute, err)
	}

	if !strings.Contains(err.Error(), "WASTEBIN_MCP_ALLOWED_PATHS") {
		t.Errorf("expected error to mention WASTEBIN_MCP_ALLOWED_PATHS, got: %v", err)
	}
}

func TestConfigFromEnv_BlockedPathsRejectsRelative(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_BLOCKED_PATHS", "secret")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for relative blocked path")
	}

	if !errors.Is(err, errConfiguredPathNotAbsolute) {
		t.Errorf("expected %v, got: %v", errConfiguredPathNotAbsolute, err)
	}

	if !strings.Contains(err.Error(), "WASTEBIN_MCP_BLOCKED_PATHS") {
		t.Errorf("expected error to mention WASTEBIN_MCP_BLOCKED_PATHS, got: %v", err)
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

	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
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

	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
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

func TestConfigFromEnv_SandboxMountSymlink(t *testing.T) {
	tmpDir := t.TempDir()

	realDir := filepath.Join(tmpDir, "real")

	err := os.Mkdir(realDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	linkDir := filepath.Join(tmpDir, "link")

	err = os.Symlink(realDir, linkDir)
	if err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_ALLOWED_PATHS", realDir)
	t.Setenv("WASTEBIN_MCP_SANDBOX_MOUNTS", linkDir+":/workspace")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.SandboxMounts) != 1 {
		t.Fatalf("expected 1 SandboxMount, got %d", len(cfg.SandboxMounts))
	}

	// HostPath should be canonicalised to the real dir.
	if cfg.SandboxMounts[0].HostPath != filepath.Clean(realDir) {
		t.Errorf("expected host path %q, got %q", filepath.Clean(realDir), cfg.SandboxMounts[0].HostPath)
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

	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_ALLOWED_PATHS", allowedDir)
	t.Setenv("WASTEBIN_MCP_SANDBOX_MOUNTS", otherDir+":/workspace")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.SandboxMounts) != 0 {
		t.Fatalf("expected disallowed sandbox mount to be removed, got %v", cfg.SandboxMounts)
	}
}

func TestConfigFromEnv_SandboxMountsFilterPreservesOrder(t *testing.T) {
	tmpDir := t.TempDir()
	allowedDir := filepath.Join(tmpDir, "allowed")
	firstAllowedDir := filepath.Join(allowedDir, "first")
	secondAllowedDir := filepath.Join(allowedDir, "second")
	disallowedDir := filepath.Join(tmpDir, "disallowed")

	for _, dir := range []string{firstAllowedDir, secondAllowedDir, disallowedDir} {
		err := os.MkdirAll(dir, 0o750)
		if err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_ALLOWED_PATHS", allowedDir)
	t.Setenv(
		"WASTEBIN_MCP_SANDBOX_MOUNTS",
		firstAllowedDir+":/workspace/first,"+
			disallowedDir+":/workspace/skipped,"+
			secondAllowedDir+":/workspace/second",
	)

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.SandboxMounts) != 2 {
		t.Fatalf("expected 2 authorized sandbox mounts, got %v", cfg.SandboxMounts)
	}

	want := []SandboxMount{
		{HostPath: firstAllowedDir, SandboxPath: "/workspace/first"},
		{HostPath: secondAllowedDir, SandboxPath: "/workspace/second"},
	}
	for i := range want {
		if cfg.SandboxMounts[i] != want[i] {
			t.Errorf("SandboxMounts[%d] = %#v, want %#v", i, cfg.SandboxMounts[i], want[i])
		}
	}
}

func TestConfigFromEnv_InvalidSandboxTransparent(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_SANDBOX_TRANSPARENT", "nope")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid bool")
	}
}

func TestConfigFromEnv_InvalidDebug(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("DEBUG", "maybe")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid debug bool")
	}
}

func TestConfigFromEnv_SandboxMountsSelfAuthorize(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_SANDBOX_MOUNTS", "/tmp:/workspace")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.SandboxMounts) != 1 {
		t.Fatalf("expected 1 SandboxMount, got %d", len(cfg.SandboxMounts))
	}

	if cfg.SandboxMounts[0].HostPath != "/tmp" {
		t.Errorf("got %q, want %q", cfg.SandboxMounts[0].HostPath, "/tmp")
	}

	if cfg.SandboxMounts[0].SandboxPath != "/workspace" {
		t.Errorf("got %q, want %q", cfg.SandboxMounts[0].SandboxPath, "/workspace")
	}
}

func TestConfigFromEnv_SandboxMountNonExistentHostPath(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_ALLOWED_PATHS", tmpDir)
	t.Setenv("WASTEBIN_MCP_SANDBOX_MOUNTS", "/nonexistent/path/that/does/not/exist/12345:/workspace")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for nonexistent sandbox mount host path")
	}

	if !strings.Contains(err.Error(), "failed to resolve sandbox mount host path") {
		t.Errorf("expected 'failed to resolve sandbox mount host path', got: %v", err)
	}
}

func TestConfigFromEnv_InvalidSandboxMounts(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_SANDBOX_MOUNTS", "invalid-format")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid sandbox mounts")
	}
}

func TestConfigFromEnv_FileReadEnabledTrue(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_FILE_READ_ENABLED", "true")
	t.Setenv("WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST", "true")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.FileReadEnabled {
		t.Error("expected FileReadEnabled to be true")
	}

	if !cfg.DisableBuiltinBlocklist {
		t.Error("expected DisableBuiltinBlocklist to be true")
	}
}

func TestConfigFromEnv_InvalidDisableBuiltinBlocklist(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST", "not-bool")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid bool")
	}

	if !strings.Contains(err.Error(), "WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST") {
		t.Errorf("expected error about DISABLE_BUILTIN_BLOCKLIST, got: %v", err)
	}
}

func TestConfigFromEnv_AllowedPathsEmptyParts(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_ALLOWED_PATHS", "/tmp,,"+tmpDir)

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have 2 paths (empty part skipped).
	if len(cfg.AllowedPaths) != 2 {
		t.Errorf("expected 2 allowed paths (skipping empty), got %d: %v", len(cfg.AllowedPaths), cfg.AllowedPaths)
	}
}

func TestConfigFromEnv_BlockedPathsEmptyParts(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_BLOCKED_PATHS", "/etc,,/proc")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.BlockedPaths) != 2 {
		t.Errorf("expected 2 blocked paths (skipping empty), got %d: %v", len(cfg.BlockedPaths), cfg.BlockedPaths)
	}

	// Both should resolve to absolute paths.
	if !strings.HasSuffix(cfg.BlockedPaths[0].Lexical, "/etc") {
		t.Errorf("expected first blocked lexical to end with /etc, got %q", cfg.BlockedPaths[0].Lexical)
	}

	if !strings.HasSuffix(cfg.BlockedPaths[1].Lexical, "/proc") {
		t.Errorf("expected second blocked lexical to end with /proc, got %q", cfg.BlockedPaths[1].Lexical)
	}
}

func TestConfigFromEnv_AllowedPathsNonExistent(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_ALLOWED_PATHS", "/nonexistent/path/that/does/not/exist/12345")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for nonexistent allowed path")
	}

	if !strings.Contains(err.Error(), "failed to resolve WASTEBIN_MCP_ALLOWED_PATHS path") {
		t.Errorf("expected 'failed to resolve WASTEBIN_MCP_ALLOWED_PATHS path', got: %v", err)
	}
}

// The isAllowedPath function is tested in path_test.go.

func envExamplePath(t *testing.T) string {
	t.Helper()
	// .env.example sits at the repository root. go test changes the working
	// directory to the package directory (internal/wastebin/), so we need
	// to go up two levels.
	return filepath.Join("..", "..", ".env.example")
}

type envExampleEntry struct {
	defaultDesc string // text of the "# Default:" line
	exampleVal  string // value after VAR= (uncommented or commented-out)
}

func parseEnvExample(content string) map[string]envExampleEntry {
	doc := make(map[string]envExampleEntry)

	var lastDefault string

	for line := range strings.SplitSeq(content, "\n") {
		const defaultPrefix = "# Default: "
		if after, ok := strings.CutPrefix(line, defaultPrefix); ok {
			lastDefault = after

			continue
		}

		eqIdx := strings.Index(line, "=")
		if eqIdx <= 0 {
			continue
		}

		name := strings.TrimSpace(line[:eqIdx])
		name = strings.TrimPrefix(name, "# ")

		val := line[eqIdx+1:]

		if !isUpperSnake(name) {
			continue
		}

		doc[name] = envExampleEntry{
			defaultDesc: lastDefault,
			exampleVal:  val,
		}

		lastDefault = ""
	}

	return doc
}

func isUpperSnake(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}

	return true
}

// TestEnvExampleSyncWithConfigFromEnv cross-checks .env.example documentation
// against ConfigFromEnv's actual env vars and DefaultConfig's defaults.
// This is a maintenance checklist — adding a new env var to ConfigFromEnv
// requires an entry in both .env.example and this test's want table.
func TestEnvExampleSyncWithConfigFromEnv(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(envExamplePath(t))
	if err != nil {
		t.Fatalf("reading .env.example: %v", err)
	}

	doc := parseEnvExample(string(data))

	type wantEntry struct {
		name              string
		documentedDefault string // expected text of "# Default:"
		documentedExample string // expected value after VAR=
		required          bool   // WASTEBIN_SERVER_URL has no default
	}

	cfg := DefaultConfig()

	want := []wantEntry{
		{
			name:              "WASTEBIN_SERVER_URL",
			documentedExample: "https://bin-staging.xlion.tw",
			required:          true,
		},
		{
			name:              "WASTEBIN_MCP_DEFAULT_EXPIRES",
			documentedDefault: fmt.Sprintf("%d (1 year)", cfg.DefaultExpires),
			documentedExample: "31536000",
		},
		{
			name:              "WASTEBIN_MCP_FILE_READ_ENABLED",
			documentedDefault: "true",
			documentedExample: "true",
		},
		{
			name:              "WASTEBIN_MCP_ALLOWED_PATHS",
			documentedDefault: "(empty — only built-in and user blocklists apply)",
			documentedExample: "/home/user/documents,/tmp",
		},
		{
			name:              "WASTEBIN_MCP_BLOCKED_PATHS",
			documentedDefault: "(empty — built-in blocklist handles system paths)",
			documentedExample: "/home/user/secret",
		},
		{
			name:              "WASTEBIN_MCP_MAX_CONTENT_SIZE",
			documentedDefault: fmt.Sprintf("%d (1 MB)", cfg.MaxContentSize),
			documentedExample: "1048576",
		},
		{
			name:              "WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST",
			documentedDefault: "false",
			documentedExample: "true",
		},
		{
			name:              "WASTEBIN_MCP_ALLOW_INSECURE_PASSWORD",
			documentedDefault: "false",
			documentedExample: "false",
		},
		{
			name:              "WASTEBIN_MCP_SANDBOX_MOUNTS",
			documentedDefault: "(empty — sandbox translation disabled)",
			documentedExample: "",
		},
		{
			name:              "WASTEBIN_MCP_SANDBOX_TRANSPARENT",
			documentedDefault: "false",
			documentedExample: "false",
		},
		{
			name:              "DEBUG",
			documentedDefault: "(not set — debug logging disabled)",
			documentedExample: "1",
		},
	}

	for _, w := range want {
		got, ok := doc[w.name]
		if !ok {
			t.Errorf(
				"env var %q is missing from .env.example (it must be documented "+
					"with a commented-out line and # Default: comment)",
				w.name,
			)

			continue
		}

		if !w.required && got.defaultDesc != w.documentedDefault {
			t.Errorf(
				"%q documented default: got %q, want %q",
				w.name, got.defaultDesc, w.documentedDefault,
			)
		}

		if got.exampleVal != w.documentedExample {
			t.Errorf(
				"%q example value: got %q, want %q",
				w.name, got.exampleVal, w.documentedExample,
			)
		}
	}

	wantByKey := make(map[string]wantEntry, len(want))
	for _, w := range want {
		wantByKey[w.name] = w
	}

	for name := range doc {
		if _, ok := wantByKey[name]; !ok {
			t.Errorf(
				"env var %q is documented in .env.example but missing from the want "+
					"table — add an entry if it should be tested, or remove it from .env.example",
				name,
			)
		}
	}
}

func TestConfigFromEnv_BlockedPathsSymlink(t *testing.T) {
	tmpDir := t.TempDir()

	realDir := filepath.Join(tmpDir, "real")

	err := os.Mkdir(realDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	linkDir := filepath.Join(tmpDir, "link")

	err = os.Symlink(realDir, linkDir)
	if err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_BLOCKED_PATHS", linkDir)

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.BlockedPaths) != 1 {
		t.Fatalf("expected 1 BlockedPath, got %d", len(cfg.BlockedPaths))
	}

	// The resolved path should be the real path (EvalSymlinks), not the symlink.
	if cfg.BlockedPaths[0].Resolved != filepath.Clean(realDir) {
		t.Errorf("expected resolved %q, got %q", filepath.Clean(realDir), cfg.BlockedPaths[0].Resolved)
	}

	if cfg.BlockedPaths[0].Lexical != filepath.Clean(linkDir) {
		t.Errorf("expected lexical %q, got %q", filepath.Clean(linkDir), cfg.BlockedPaths[0].Lexical)
	}
}

func TestConfigFromEnv_BlockedPathsNonExistent(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_BLOCKED_PATHS", "/nonexistent/path/that/does/not/exist/blocked")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.BlockedPaths) != 1 {
		t.Fatalf("expected 1 BlockedPath, got %d", len(cfg.BlockedPaths))
	}

	// Non-existent absolute paths should fall back to Clean (lexical == resolved).
	if !filepath.IsAbs(cfg.BlockedPaths[0].Lexical) {
		t.Errorf("expected absolute lexical path for non-existent blocked path, got %q", cfg.BlockedPaths[0].Lexical)
	}

	if !strings.HasSuffix(cfg.BlockedPaths[0].Lexical, "blocked") {
		t.Errorf("expected lexical path to end with 'blocked', got %q", cfg.BlockedPaths[0].Lexical)
	}

	if cfg.BlockedPaths[0].Lexical != cfg.BlockedPaths[0].Resolved {
		t.Errorf(
			"expected lexical %q to equal resolved %q for non-existent path",
			cfg.BlockedPaths[0].Lexical,
			cfg.BlockedPaths[0].Resolved,
		)
	}
}
