package wastebin //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// ──────────────────────────────────────────────
// normalizePath tests
// ──────────────────────────────────────────────

func TestNormalizePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "regular unix path", input: "/tmp/foo", expected: "/tmp/foo"},
		{name: "windows backslashes", input: `C:\Users\foo`, expected: "C:/Users/foo"},
		{name: "mixed slashes", input: `foo\bar/baz`, expected: "foo/bar/baz"},
		{name: "empty string", input: "", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := normalizePath(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// ──────────────────────────────────────────────
// hasPathTraversal tests
// ──────────────────────────────────────────────

func TestHasPathTraversal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "parent dir prefix", input: "../foo", want: true},
		{name: "parent dir mid path", input: "foo/../bar", want: true},
		{name: "no traversal", input: "foo/bar", want: false},
		{name: "absolute path", input: "/etc/passwd", want: false},
		{name: "multiple parent dir", input: "../../etc", want: true},
		{name: "windows backslash traversal", input: `..\\foo`, want: true},
		{name: "empty string", input: "", want: false},
		{name: "dot", input: ".", want: false},
		{name: "just dot-dot", input: "..", want: true},
		{name: "unicode path", input: "/tmp/文件", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := hasPathTraversal(tt.input)
			if result != tt.want {
				t.Errorf("hasPathTraversal(%q) = %v, want %v", tt.input, result, tt.want)
			}
		})
	}
}

// ──────────────────────────────────────────────
// isAllowedPath tests
// ──────────────────────────────────────────────

func TestIsAllowedPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		allowed []string
		want    bool
	}{
		{name: "exact match", path: "/tmp/foo", allowed: []string{"/tmp"}, want: true},
		{name: "subdirectory", path: "/tmp/foo/bar", allowed: []string{"/tmp"}, want: true},
		{name: "not in allowed", path: "/var", allowed: []string{"/tmp"}, want: false},
		{name: "prefix not partial", path: "/tmp2", allowed: []string{"/tmp"}, want: false},
		{name: "no allowed paths", path: "/tmp/foo", allowed: nil, want: false},
		{name: "empty allowed paths", path: "/tmp/foo", allowed: []string{}, want: false},
		{name: "deep nested", path: "/a/b/c/d/e/f", allowed: []string{"/a/b"}, want: true},
		{name: "multiple allowed", path: "/opt/data/file.txt", allowed: []string{"/tmp", "/opt/data", "/var"}, want: true},
		{name: "allowed equals path", path: "/tmp", allowed: []string{"/tmp"}, want: true},
		{name: "..vault descendant", path: "/tmp/allowed/..vault/file", allowed: []string{"/tmp/allowed"}, want: true},
		{name: "..vault dir itself", path: "/tmp/allowed/..vault", allowed: []string{"/tmp/allowed"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := isAllowedPath(tt.path, tt.allowed)
			if result != tt.want {
				t.Errorf("isAllowedPath(%q, %v) = %v, want %v", tt.path, tt.allowed, result, tt.want)
			}
		})
	}
}

// ──────────────────────────────────────────────
// isBuiltinBlocked tests
// ──────────────────────────────────────────────

func TestIsBuiltinBlocked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		path        string
		wantBlocked bool
		wantReason  string
	}{
		{name: "/etc prefix", path: "/etc/passwd", wantBlocked: true, wantReason: "/etc"},
		{name: "/proc prefix", path: "/proc/cpuinfo", wantBlocked: true, wantReason: "/proc"},
		{name: ".ssh component", path: "/home/user/.ssh/id_rsa", wantBlocked: true, wantReason: ".ssh"},
		{name: ".gnupg component", path: "/home/user/.gnupg/pubring.kbx", wantBlocked: true, wantReason: ".gnupg"},
		{name: "/tmp not blocked", path: "/tmp/foo", wantBlocked: false, wantReason: ""},
		{name: "home documents not blocked", path: "/home/user/documents", wantBlocked: false, wantReason: ""},
		{name: "exact /etc prefix", path: "/etc", wantBlocked: true, wantReason: "/etc"},
		{name: "case sensitive", path: "/ETC", wantBlocked: false, wantReason: ""},
		{name: "multiple levels", path: "/sys/devices/virtual", wantBlocked: true, wantReason: "/sys"},
		{name: "/dev prefix", path: "/dev/null", wantBlocked: true, wantReason: "/dev"},
		{name: ".ssh on blocked prefix", path: "/etc/.ssh", wantBlocked: true, wantReason: "/etc"},
		{name: ".ssh at root", path: "/.ssh/authorized_keys", wantBlocked: true, wantReason: ".ssh"},
		{name: ".ssh at end", path: "/home/user/.ssh", wantBlocked: true, wantReason: ".ssh"},
		{name: ".gnupg mid-path", path: "/data/backup/.gnupg/keys/pubring.kbx", wantBlocked: true, wantReason: ".gnupg"},
		{name: ".aws component", path: "/home/user/.aws/credentials", wantBlocked: true, wantReason: ".aws"},
		{name: ".kube component", path: "/home/user/.kube/config", wantBlocked: true, wantReason: ".kube"},
		{name: ".docker component", path: "/home/user/.docker/config.json", wantBlocked: true, wantReason: ".docker"},
		{name: ".git component", path: "/home/user/project/.git/config", wantBlocked: true, wantReason: ".git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reason, blocked := isBuiltinBlocked(tt.path)
			if blocked != tt.wantBlocked {
				t.Errorf("isBuiltinBlocked(%q) blocked=%v, want blocked=%v", tt.path, blocked, tt.wantBlocked)
			}

			if tt.wantBlocked && reason != tt.wantReason {
				t.Errorf("isBuiltinBlocked(%q) reason=%q, want reason=%q", tt.path, reason, tt.wantReason)
			}
		})
	}
}

// ──────────────────────────────────────────────
// isUserBlocked tests
// ──────────────────────────────────────────────

func TestIsUserBlocked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		path        string
		blocked     []string
		wantBlocked bool
		wantMatch   string
	}{
		{
			name:        "exact match",
			path:        "/home/user/secret",
			blocked:     []string{"/home/user/secret"},
			wantBlocked: true,
			wantMatch:   "/home/user/secret",
		},
		{
			name:        "subdirectory",
			path:        "/home/user/secret/file.txt",
			blocked:     []string{"/home/user/secret"},
			wantBlocked: true,
			wantMatch:   "/home/user/secret",
		},
		{name: "not blocked", path: "/home/user/other", blocked: []string{"/home/user/secret"}, wantBlocked: false},
		{name: "no blocked paths", path: "/home/user/secret", blocked: nil, wantBlocked: false},
		{name: "empty blocked paths", path: "/home/user/secret", blocked: []string{}, wantBlocked: false},
		{name: "prefix not partial", path: "/home/user/secret2", blocked: []string{"/home/user/secret"}, wantBlocked: false},
		{
			name:        "multiple blocked paths",
			path:        "/opt/restricted/data",
			blocked:     []string{"/tmp", "/opt/restricted"},
			wantBlocked: true,
			wantMatch:   "/opt/restricted",
		},
		{
			name:        "..vault descendant",
			path:        "/tmp/blocked/..vault/file.txt",
			blocked:     []string{"/tmp/blocked"},
			wantBlocked: true,
			wantMatch:   "/tmp/blocked",
		},
		{
			name:        "..vault not traversal",
			path:        "/home/user/..vault/project",
			blocked:     []string{"/tmp/blocked"},
			wantBlocked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			matched, blocked := isUserBlocked(tt.path, tt.blocked)
			if blocked != tt.wantBlocked {
				t.Errorf("isUserBlocked(%q, %v) blocked=%v, want blocked=%v", tt.path, tt.blocked, blocked, tt.wantBlocked)
			}

			if tt.wantBlocked && matched != tt.wantMatch {
				t.Errorf("isUserBlocked(%q, %v) matched=%q, want match=%q", tt.path, tt.blocked, matched, tt.wantMatch)
			}
		})
	}
}

// ──────────────────────────────────────────────
// validateFilePath full pipeline tests
// ──────────────────────────────────────────────

func TestValidateFilePath_PathTraversal(t *testing.T) {
	t.Parallel()

	cfg := &Config{}

	_, err := validateFilePath("../etc/passwd", cfg)
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}

	if !errors.Is(err, errPathTraversal) {
		t.Errorf("expected errPathTraversal, got: %v", err)
	}
}

func TestValidateFilePath_AllowedPathBypassesBlocklists(t *testing.T) {
	t.Parallel()
	// The built-in blocklist blocks /etc, but ALLOWED_PATHS should bypass it.
	tmpDir := t.TempDir()

	nginxDir := filepath.Join(tmpDir, "etc", "nginx", "conf.d")

	err := os.MkdirAll(nginxDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	confFile := filepath.Join(nginxDir, "default.conf")

	err = os.WriteFile(confFile, []byte("server { }"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		AllowedPaths: []string{filepath.Join(tmpDir, "etc", "nginx")},
	}

	result, err := validateFilePath(confFile, cfg)
	if err != nil {
		t.Fatalf("expected success with ALLOWED_PATHS bypass, got: %v", err)
	}

	if result != confFile {
		t.Errorf("expected resolved path %q, got %q", confFile, result)
	}
}

func TestValidateFilePath_BuiltinBlockedPrefixNoAllowed(t *testing.T) {
	t.Parallel()
	// /etc/passwd exists on any Linux system.
	cfg := &Config{}

	_, err := validateFilePath("/etc/passwd", cfg)
	if err == nil {
		t.Fatal("expected error for /etc/passwd, got nil")
	}

	if !errors.Is(err, errBuiltinBlockedPrefix) {
		t.Errorf("expected errBuiltinBlockedPrefix, got: %v", err)
	}
}

func TestValidateFilePath_BuiltinBlockedComponentNoAllowed(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	sshDir := filepath.Join(tmpDir, ".ssh")

	err := os.Mkdir(sshDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	keyFile := filepath.Join(sshDir, "id_rsa")

	err = os.WriteFile(keyFile, []byte("ssh-key"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{}

	_, err = validateFilePath(keyFile, cfg)
	if err == nil {
		t.Fatal("expected error for path with .ssh component, got nil")
	}

	if !errors.Is(err, errBuiltinBlockedComponent) {
		t.Errorf("expected errBuiltinBlockedComponent, got: %v", err)
	}
}

func TestValidateFilePath_UserBlockedPath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	secretDir := filepath.Join(tmpDir, "secret")

	err := os.Mkdir(secretDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	secretFile := filepath.Join(secretDir, "data.txt")

	err = os.WriteFile(secretFile, []byte("s3kr1t"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		BlockedPaths: []string{secretDir},
	}

	_, err = validateFilePath(secretFile, cfg)
	if err == nil {
		t.Fatal("expected error for user-blocked path, got nil")
	}

	if !errors.Is(err, errUserBlockedPath) {
		t.Errorf("expected errUserBlockedPath, got: %v", err)
	}
}

func TestValidateFilePath_UserBlockedSymlink(t *testing.T) {
	t.Parallel()

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

	secretFile := filepath.Join(realDir, "data.txt")

	err = os.WriteFile(secretFile, []byte("s3kr1t"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate ConfigFromEnv's symlink resolution for blocked paths.
	resolvedBlocked, err := filepath.EvalSymlinks(linkDir)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		BlockedPaths: []string{filepath.Clean(resolvedBlocked)},
	}

	_, err = validateFilePath(secretFile, cfg)
	if err == nil {
		t.Fatal("expected error for user-blocked path via symlink, got nil")
	}

	if !errors.Is(err, errUserBlockedPath) {
		t.Errorf("expected errUserBlockedPath, got: %v", err)
	}
}

func TestValidateFilePath_UserBlockedViaSymlinkAlias(t *testing.T) {
	t.Parallel()

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

	secretFile := filepath.Join(linkDir, "data.txt")

	err = os.WriteFile(secretFile, []byte("s3kr1t"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Block the real dir (the resolved target).
	cfg := &Config{
		BlockedPaths: []string{filepath.Clean(realDir)},
	}

	// Request the file via the symlink path.
	_, err = validateFilePath(secretFile, cfg)
	if err == nil {
		t.Fatal("expected error for user-blocked path accessed via symlink, got nil")
	}

	if !errors.Is(err, errUserBlockedPath) {
		t.Errorf("expected errUserBlockedPath, got: %v", err)
	}
}

func TestValidateFilePath_NotInAllowedNotInBlocklist(t *testing.T) {
	t.Parallel()
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

	otherFile := filepath.Join(otherDir, "test.txt")

	err = os.WriteFile(otherFile, []byte("hello"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		AllowedPaths: []string{allowedDir},
	}

	_, err = validateFilePath(otherFile, cfg)
	if err == nil {
		t.Fatal("expected error for path not under allowed, got nil")
	}

	if !errors.Is(err, errPathNotAllowed) {
		t.Errorf("expected errPathNotAllowed, got: %v", err)
	}
}

func TestValidateFilePath_DisableBuiltinBlocklist(t *testing.T) {
	t.Parallel()
	// DisableBuiltinBlocklist=true with a real builtin prefix path (/etc/passwd)
	// should skip the builtin prefix check and allow the path through
	// (assuming no user blocklist or allowed path restrictions).
	// /etc/passwd exists on all Linux systems.
	cfg := &Config{
		DisableBuiltinBlocklist: true,
	}

	result, err := validateFilePath("/etc/passwd", cfg)
	if err != nil {
		t.Fatalf("expected success with builtin blocklist disabled for /etc/passwd, got: %v", err)
	}

	if result != "/etc/passwd" {
		t.Errorf("expected resolved path %q, got %q", "/etc/passwd", result)
	}
}

func TestValidateFilePath_AllowedWithAllowedPath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(allowedDir, "doc.txt")

	err = os.WriteFile(file, []byte("content"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		AllowedPaths: []string{allowedDir},
	}

	result, err := validateFilePath(file, cfg)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	if result != file {
		t.Errorf("expected resolved path %q, got %q", file, result)
	}
}

func TestValidateFilePath_AllowedPathStillBlocksSensitiveComponent(t *testing.T) {
	t.Parallel()
	// ALLOWED_PATHS should NOT bypass the sensitive component blocklist.
	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "workspace")
	sshDir := filepath.Join(allowedDir, ".ssh")

	err := os.MkdirAll(sshDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	keyFile := filepath.Join(sshDir, "id_rsa")

	err = os.WriteFile(keyFile, []byte("ssh-key"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		AllowedPaths: []string{allowedDir},
	}

	_, err = validateFilePath(keyFile, cfg)
	if err == nil {
		t.Fatal("expected error for .ssh component inside ALLOWED_PATHS, got nil")
	}

	if !errors.Is(err, errBuiltinBlockedComponent) {
		t.Errorf("expected errBuiltinBlockedComponent, got: %v", err)
	}
}

func TestValidateFilePath_AllowedPathBlockedByUserBlocklist(t *testing.T) {
	t.Parallel()
	// ALLOWED_PATHS should bypass even the user blocklist.
	tmpDir := t.TempDir()

	dir := filepath.Join(tmpDir, "mydir")

	err := os.Mkdir(dir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(dir, "file.txt")

	err = os.WriteFile(file, []byte("data"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		AllowedPaths: []string{dir},
		BlockedPaths: []string{dir}, // same path in blocklist
	}

	result, err := validateFilePath(file, cfg)
	if err != nil {
		t.Fatalf("expected ALLOWED_PATHS to bypass blocklist, got: %v", err)
	}

	if result != file {
		t.Errorf("expected resolved path %q, got %q", file, result)
	}
}

// ──────────────────────────────────────────────
// isContainedPath tests
// ──────────────────────────────────────────────

func TestIsContainedPath_DotVaultDescendant(t *testing.T) {
	t.Parallel()

	// ..vault starts with ".." but is a legitimate name, not traversal.
	if !isContainedPath("/tmp/blocked", "/tmp/blocked/..vault/file.txt") {
		t.Error("expected ..vault descendant to be contained")
	}
}

func TestIsContainedPath_ActualTraversal(t *testing.T) {
	t.Parallel()

	if isContainedPath("/tmp/blocked", "/tmp/blocked/../etc") {
		t.Error("expected actual traversal to NOT be contained")
	}
}

func TestIsContainedPath_ExactMatch(t *testing.T) {
	t.Parallel()

	if !isContainedPath("/tmp/foo", "/tmp/foo") {
		t.Error("expected exact match to be contained")
	}
}

func TestIsContainedPath_ParentDirOnly(t *testing.T) {
	t.Parallel()

	if isContainedPath("/tmp/foo", "/tmp") {
		t.Error("expected parent dir to NOT be contained")
	}
}

func TestIsContainedPath_Unrelated(t *testing.T) {
	t.Parallel()

	if isContainedPath("/tmp/foo", "/var/log") {
		t.Error("expected unrelated path to NOT be contained")
	}
}

func TestIsContainedPath_DotVaultDirItself(t *testing.T) {
	t.Parallel()

	if !isContainedPath("/tmp", "/tmp/..vault") {
		t.Error("expected ..vault dir itself to be contained")
	}
}

// ──────────────────────────────────────────────
// Symlink escape from allowed directory
// ──────────────────────────────────────────────

func TestValidateFilePath_SymlinkEscapeFromAllowed(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	// Create target file outside allowed dir
	outsideDir := filepath.Join(tmpDir, "outside")

	err = os.Mkdir(outsideDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	outsideFile := filepath.Join(outsideDir, "secret.txt")

	err = os.WriteFile(outsideFile, []byte("secret"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Create symlink inside allowed dir pointing outside
	symlinkPath := filepath.Join(allowedDir, "escape_link")

	err = os.Symlink(outsideFile, symlinkPath)
	if err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	cfg := &Config{
		AllowedPaths: []string{allowedDir},
		BlockedPaths: DefaultConfig().BlockedPaths,
	}

	_, err = validateFilePath(symlinkPath, cfg)
	if err == nil {
		t.Error("expected error for symlink escape, got nil")
	}

	if !errors.Is(err, errPathNotAllowed) {
		t.Errorf("expected errPathNotAllowed, got: %v", err)
	}
}

// ──────────────────────────────────────────────
// hasComponentBlocked tests
// ──────────────────────────────────────────────

func TestHasComponentBlocked_DotSsh(t *testing.T) {
	t.Parallel()

	reason, blocked := hasComponentBlocked("/home/user/.ssh/id_rsa")
	if !blocked {
		t.Fatal("expected .ssh to be detected in raw path")
	}

	if reason != ".ssh" {
		t.Errorf("expected reason %q, got %q", ".ssh", reason)
	}
}

func TestHasComponentBlocked_DotGit(t *testing.T) {
	t.Parallel()

	reason, blocked := hasComponentBlocked("/home/user/project/.git/config")
	if !blocked {
		t.Fatal("expected .git to be detected in raw path")
	}

	if reason != ".git" {
		t.Errorf("expected reason %q, got %q", ".git", reason)
	}
}

func TestHasComponentBlocked_DotAws(t *testing.T) {
	t.Parallel()

	reason, blocked := hasComponentBlocked("/home/user/.aws/credentials")
	if !blocked {
		t.Fatal("expected .aws to be detected in raw path")
	}

	if reason != ".aws" {
		t.Errorf("expected reason %q, got %q", ".aws", reason)
	}
}

func TestHasComponentBlocked_DotKube(t *testing.T) {
	t.Parallel()

	reason, blocked := hasComponentBlocked("/home/user/.kube/config")
	if !blocked {
		t.Fatal("expected .kube to be detected in raw path")
	}

	if reason != ".kube" {
		t.Errorf("expected reason %q, got %q", ".kube", reason)
	}
}

func TestHasComponentBlocked_DotDocker(t *testing.T) {
	t.Parallel()

	reason, blocked := hasComponentBlocked("/home/user/.docker/config.json")
	if !blocked {
		t.Fatal("expected .docker to be detected in raw path")
	}

	if reason != ".docker" {
		t.Errorf("expected reason %q, got %q", ".docker", reason)
	}
}

func TestHasComponentBlocked_DotGnupg(t *testing.T) {
	t.Parallel()

	reason, blocked := hasComponentBlocked("/home/user/.gnupg/pubring.kbx")
	if !blocked {
		t.Fatal("expected .gnupg to be detected in raw path")
	}

	if reason != ".gnupg" {
		t.Errorf("expected reason %q, got %q", ".gnupg", reason)
	}
}

func TestHasComponentBlocked_CleanPath(t *testing.T) {
	t.Parallel()

	_, blocked := hasComponentBlocked("/home/user/documents/report.txt")
	if blocked {
		t.Error("expected clean path not to be blocked")
	}
}

func TestHasComponentBlocked_EmptyString(t *testing.T) {
	t.Parallel()

	_, blocked := hasComponentBlocked("")
	if blocked {
		t.Error("expected empty path not to be blocked")
	}
}

func TestHasComponentBlocked_DisableBuiltinBlocklist(t *testing.T) {
	t.Parallel()

	// hasComponentBlocked doesn't check the config, so it always flags.
	// The config check happens in the caller (validateFilePath/readFileContent).
	reason, blocked := hasComponentBlocked("/home/user/.ssh/id_rsa")
	if !blocked {
		t.Fatal("expected hasComponentBlocked to detect .ssh regardless of config")
	}

	if reason != ".ssh" {
		t.Errorf("expected reason %q, got %q", ".ssh", reason)
	}
}

func TestHasComponentBlocked_WindowsBackslashes(t *testing.T) {
	t.Parallel()

	reason, blocked := hasComponentBlocked(`C:\Users\foo\.ssh\id_rsa`)
	if !blocked {
		t.Fatal("expected .ssh to be detected in Windows path")
	}

	if reason != ".ssh" {
		t.Errorf("expected reason %q, got %q", ".ssh", reason)
	}
}

func TestHasComponentBlocked_MixedSlashes(t *testing.T) {
	t.Parallel()

	reason, blocked := hasComponentBlocked("foo\\bar/.ssh/key")
	if !blocked {
		t.Fatal("expected .ssh to be detected in mixed-slash path")
	}

	if reason != ".ssh" {
		t.Errorf("expected reason %q, got %q", ".ssh", reason)
	}
}

// ──────────────────────────────────────────────
// Stage function tests
// ──────────────────────────────────────────────

func TestStageRawComponentBlocked_CleanPath(t *testing.T) {
	t.Parallel()

	result, err := stageRawComponentBlocked("/home/user/documents/report.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "/home/user/documents/report.txt" {
		t.Errorf("expected path unchanged, got %q", result)
	}
}

func TestStageResolvedComponentBlocked_CleanPath(t *testing.T) {
	t.Parallel()

	result, err := stageResolvedComponentBlocked("/home/user/documents/report.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "/home/user/documents/report.txt" {
		t.Errorf("expected path unchanged, got %q", result)
	}
}

func TestStageResolvedComponentBlocked_BlockedPath(t *testing.T) {
	t.Parallel()

	_, err := stageResolvedComponentBlocked("/home/user/.ssh/id_rsa")
	if err == nil {
		t.Fatal("expected error for .ssh component, got nil")
	}

	if !errors.Is(err, errBuiltinBlockedComponent) {
		t.Errorf("expected errBuiltinBlockedComponent, got: %v", err)
	}
}

func TestStageBuiltinBlocked_PrefixMatch(t *testing.T) {
	t.Parallel()

	_, err := stageBuiltinBlocked("/etc/passwd")
	if err == nil {
		t.Fatal("expected error for /etc prefix, got nil")
	}

	if !errors.Is(err, errBuiltinBlockedPrefix) {
		t.Errorf("expected errBuiltinBlockedPrefix, got: %v", err)
	}
}

func TestStageBuiltinBlocked_ComponentOnlyMatch(t *testing.T) {
	t.Parallel()

	// /.ssh/authorized_keys is not under any blocked prefix
	// (/etc, /proc, /sys, /dev), but .ssh is a blocked component.
	_, err := stageBuiltinBlocked("/.ssh/authorized_keys")
	if err == nil {
		t.Fatal("expected error for .ssh component without prefix match, got nil")
	}

	if !errors.Is(err, errBuiltinBlockedComponent) {
		t.Errorf("expected errBuiltinBlockedComponent, got: %v", err)
	}
}

func TestStageBuiltinBlocked_CleanPath(t *testing.T) {
	t.Parallel()

	result, err := stageBuiltinBlocked("/home/user/documents/report.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "/home/user/documents/report.txt" {
		t.Errorf("expected path unchanged, got %q", result)
	}
}

func TestStageRawComponentBlocked_BlockedPath(t *testing.T) {
	t.Parallel()

	_, err := stageRawComponentBlocked("/home/user/.ssh/id_rsa")
	if err == nil {
		t.Fatal("expected error for .ssh component, got nil")
	}

	if !errors.Is(err, errBuiltinBlockedComponent) {
		t.Errorf("expected errBuiltinBlockedComponent, got: %v", err)
	}
}

func TestStageRawComponentBlocked_WindowsPath(t *testing.T) {
	t.Parallel()

	_, err := stageRawComponentBlocked(`C:\Users\foo\.ssh\id_rsa`)
	if err == nil {
		t.Fatal("expected error for .ssh component in Windows path, got nil")
	}

	if !errors.Is(err, errBuiltinBlockedComponent) {
		t.Errorf("expected errBuiltinBlockedComponent, got: %v", err)
	}
}

func TestNewBlocklistStages_Disabled(t *testing.T) {
	t.Parallel()

	blk := newBlocklistStages(true)
	if blk.rawComponent != nil {
		t.Error("expected rawComponent to be nil when disabled")
	}

	if blk.resolvedComponent != nil {
		t.Error("expected resolvedComponent to be nil when disabled")
	}

	if blk.builtinFull != nil {
		t.Error("expected builtinFull to be nil when disabled")
	}
}

func TestNewBlocklistStages_Enabled(t *testing.T) {
	t.Parallel()

	blk := newBlocklistStages(false)
	if blk.rawComponent == nil {
		t.Error("expected rawComponent to be non-nil when enabled")
	}

	if blk.resolvedComponent == nil {
		t.Error("expected resolvedComponent to be non-nil when enabled")
	}

	if blk.builtinFull == nil {
		t.Error("expected builtinFull to be non-nil when enabled")
	}
}

// ──────────────────────────────────────────────
// Symlinked sensitive component tests
// ──────────────────────────────────────────────

// symlinkSensitiveComponent creates a real directory and a symlink with the
// given sensitive name pointing to it, then returns the path of a test file
// via the symlink.
func symlinkSensitiveComponent(t *testing.T, sensitiveName string) string {
	t.Helper()

	tmpDir := t.TempDir()
	realDir := filepath.Join(tmpDir, "real_"+sensitiveName[1:])

	err := os.Mkdir(realDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	testFile := filepath.Join(realDir, "test.txt")

	err = os.WriteFile(testFile, []byte("content"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(tmpDir, sensitiveName)

	err = os.Symlink(realDir, symlinkPath)
	if err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	return filepath.Join(symlinkPath, "test.txt")
}

func TestValidateFilePath_SymlinkedDotSsh_NoAllowed(t *testing.T) {
	t.Parallel()

	path := symlinkSensitiveComponent(t, ".ssh")

	cfg := &Config{
		BlockedPaths: DefaultConfig().BlockedPaths,
	}

	_, err := validateFilePath(path, cfg)
	if err == nil {
		t.Fatal("expected error for symlinked .ssh, got nil")
	}

	if !errors.Is(err, errBuiltinBlockedComponent) {
		t.Errorf("expected errBuiltinBlockedComponent, got: %v", err)
	}
}

func TestValidateFilePath_SymlinkedDotGit_NoAllowed(t *testing.T) {
	t.Parallel()

	path := symlinkSensitiveComponent(t, ".git")

	cfg := &Config{
		BlockedPaths: DefaultConfig().BlockedPaths,
	}

	_, err := validateFilePath(path, cfg)
	if err == nil {
		t.Fatal("expected error for symlinked .git, got nil")
	}

	if !errors.Is(err, errBuiltinBlockedComponent) {
		t.Errorf("expected errBuiltinBlockedComponent, got: %v", err)
	}
}

func TestValidateFilePath_SymlinkedDotAws_NoAllowed(t *testing.T) {
	t.Parallel()

	path := symlinkSensitiveComponent(t, ".aws")

	cfg := &Config{
		BlockedPaths: DefaultConfig().BlockedPaths,
	}

	_, err := validateFilePath(path, cfg)
	if err == nil {
		t.Fatal("expected error for symlinked .aws, got nil")
	}

	if !errors.Is(err, errBuiltinBlockedComponent) {
		t.Errorf("expected errBuiltinBlockedComponent, got: %v", err)
	}
}

func TestValidateFilePath_SymlinkedDotKube_NoAllowed(t *testing.T) {
	t.Parallel()

	path := symlinkSensitiveComponent(t, ".kube")

	cfg := &Config{
		BlockedPaths: DefaultConfig().BlockedPaths,
	}

	_, err := validateFilePath(path, cfg)
	if err == nil {
		t.Fatal("expected error for symlinked .kube, got nil")
	}

	if !errors.Is(err, errBuiltinBlockedComponent) {
		t.Errorf("expected errBuiltinBlockedComponent, got: %v", err)
	}
}

func TestValidateFilePath_SymlinkedDotDocker_NoAllowed(t *testing.T) {
	t.Parallel()

	path := symlinkSensitiveComponent(t, ".docker")

	cfg := &Config{
		BlockedPaths: DefaultConfig().BlockedPaths,
	}

	_, err := validateFilePath(path, cfg)
	if err == nil {
		t.Fatal("expected error for symlinked .docker, got nil")
	}

	if !errors.Is(err, errBuiltinBlockedComponent) {
		t.Errorf("expected errBuiltinBlockedComponent, got: %v", err)
	}
}

func TestValidateFilePath_SymlinkedDotGnupg_NoAllowed(t *testing.T) {
	t.Parallel()

	path := symlinkSensitiveComponent(t, ".gnupg")

	cfg := &Config{
		BlockedPaths: DefaultConfig().BlockedPaths,
	}

	_, err := validateFilePath(path, cfg)
	if err == nil {
		t.Fatal("expected error for symlinked .gnupg, got nil")
	}

	if !errors.Is(err, errBuiltinBlockedComponent) {
		t.Errorf("expected errBuiltinBlockedComponent, got: %v", err)
	}
}

func TestValidateFilePath_SymlinkedDotSsh_WithAllowed(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	realDir := filepath.Join(tmpDir, "realdir")
	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(realDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	// Create symlink .ssh -> realdir inside allowed dir
	symlinkPath := filepath.Join(allowedDir, ".ssh")

	err = os.Symlink(realDir, symlinkPath)
	if err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	keyFile := filepath.Join(symlinkPath, "id_rsa")

	err = os.WriteFile(filepath.Join(realDir, "id_rsa"), []byte("ssh-key"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		AllowedPaths: []string{allowedDir},
	}

	_, err = validateFilePath(keyFile, cfg)
	if err == nil {
		t.Fatal("expected error for symlinked .ssh inside ALLOWED_PATHS, got nil")
	}

	if !errors.Is(err, errBuiltinBlockedComponent) {
		t.Errorf("expected errBuiltinBlockedComponent, got: %v", err)
	}
}

func TestValidateFilePath_NonBlockedSymlink_NoAllowed(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	realDir := filepath.Join(tmpDir, "realdir")

	err := os.Mkdir(realDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	testFile := filepath.Join(realDir, "test.txt")

	err = os.WriteFile(testFile, []byte("content"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Create a non-sensitive symlink (not in the blocked list)
	symlinkPath := filepath.Join(tmpDir, "mylink")

	err = os.Symlink(realDir, symlinkPath)
	if err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	cfg := &Config{
		BlockedPaths: DefaultConfig().BlockedPaths,
	}

	result, err := validateFilePath(filepath.Join(symlinkPath, "test.txt"), cfg)
	if err != nil {
		t.Fatalf("expected non-blocked symlink to be allowed, got: %v", err)
	}

	if result != filepath.Clean(testFile) {
		t.Errorf("expected resolved path %q, got %q", filepath.Clean(testFile), result)
	}
}

func TestValidateFilePath_NonBlockedSymlink_WithAllowed(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	allowedDir := filepath.Join(tmpDir, "allowed")
	realDir := filepath.Join(tmpDir, "realdir")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Mkdir(realDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	testFile := filepath.Join(realDir, "test.txt")

	err = os.WriteFile(testFile, []byte("content"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Create a non-sensitive symlink inside allowed dir
	symlinkPath := filepath.Join(allowedDir, "mylink")

	err = os.Symlink(realDir, symlinkPath)
	if err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	cfg := &Config{
		AllowedPaths: []string{allowedDir},
	}

	// Symlink points outside allowed directory, so it should be blocked
	_, err = validateFilePath(filepath.Join(symlinkPath, "test.txt"), cfg)
	if err == nil {
		t.Error("expected error for symlink escape from allowed, got nil")
	}

	if !errors.Is(err, errPathNotAllowed) {
		t.Errorf("expected errPathNotAllowed, got: %v", err)
	}
}

func TestValidateFilePath_SymlinkedDotSsh_DisabledBuiltinBlocklist(t *testing.T) {
	t.Parallel()

	path := symlinkSensitiveComponent(t, ".ssh")

	cfg := &Config{
		DisableBuiltinBlocklist: true,
		BlockedPaths:            DefaultConfig().BlockedPaths,
	}

	result, err := validateFilePath(path, cfg)
	if err != nil {
		t.Fatalf("expected symlinked .ssh to be allowed when builtin blocklist disabled, got: %v", err)
	}

	if result == "" {
		t.Error("expected non-empty resolved path")
	}
}

func TestValidateFilePath_DisableBuiltinBlocklistWithBlockedComponent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a .ssh directory with a test file ('.ssh' IS a builtin blocked component)
	sshDir := filepath.Join(tmpDir, ".ssh")

	err := os.Mkdir(sshDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	testFile := filepath.Join(sshDir, "authorized_keys")

	err = os.WriteFile(testFile, []byte("ssh-rsa AAA..."), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("blocked when builtin blocklist enabled", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			DisableBuiltinBlocklist: false,
			BlockedPaths:            DefaultConfig().BlockedPaths,
		}

		_, err := validateFilePath(testFile, cfg)
		if err == nil {
			t.Fatal("expected error for path with .ssh component, got nil")
		}

		if !errors.Is(err, errBuiltinBlockedComponent) {
			t.Errorf("expected errBuiltinBlockedComponent, got: %v", err)
		}
	})

	t.Run("allowed when builtin blocklist disabled", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			DisableBuiltinBlocklist: true,
			BlockedPaths:            DefaultConfig().BlockedPaths,
		}

		resolved, err := validateFilePath(testFile, cfg)
		if err != nil {
			t.Fatalf("expected no error with builtin blocklist disabled, got: %v", err)
		}

		if resolved != filepath.Clean(testFile) {
			t.Errorf("expected %q, got %q", filepath.Clean(testFile), resolved)
		}
	})
}

// ──────────────────────────────────────────────
// Permission / not-found error tests
// ──────────────────────────────────────────────

func TestValidateFilePath_NotFound(t *testing.T) {
	t.Parallel()

	cfg := &Config{}

	_, err := validateFilePath(filepath.Join(t.TempDir(), "missing"), cfg)
	if err == nil {
		t.Fatal("expected error for nonexistent path, got nil")
	}

	if !errors.Is(err, errPathNotFound) {
		t.Errorf("expected errPathNotFound, got: %v", err)
	}
}

func TestValidateFilePath_PermissionDenied(t *testing.T) {
	t.Parallel()

	if os.Getuid() == 0 {
		t.Skip("skipping permission-denied test when running as root")
	}

	tmpDir := t.TempDir()

	// Create a subdirectory and remove execute permission so that
	// EvalSymlinks cannot lstat files inside it, triggering a permission error.
	noXDir := filepath.Join(tmpDir, "noexec")

	err := os.Mkdir(noXDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	testFile := filepath.Join(noXDir, "test.txt")

	err = os.WriteFile(testFile, []byte("test"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Remove all permissions from the directory so EvalSymlinks
	// cannot traverse into it.
	err = os.Chmod(noXDir, 0o000)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		// Restore permissions so TempDir cleanup succeeds.
		//nolint:gosec // Restore permissions so TempDir cleanup succeeds
		restoreErr := os.Chmod(noXDir, 0o750)
		if restoreErr != nil {
			t.Logf("failed to restore directory permissions: %v", restoreErr)
		}
	})

	cfg := &Config{}

	_, err = validateFilePath(testFile, cfg)
	if err == nil {
		t.Fatal("expected error for permission-denied path, got nil")
	}

	if !errors.Is(err, errPathPermissionDenied) {
		t.Errorf("expected errPathPermissionDenied, got: %v", err)
	}
}
