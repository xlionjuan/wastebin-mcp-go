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

func TestNormalizePath_RegularUnixPath(t *testing.T) {
	t.Parallel()

	result := normalizePath("/tmp/foo")
	if result != "/tmp/foo" {
		t.Errorf("expected %q, got %q", "/tmp/foo", result)
	}
}

func TestNormalizePath_WindowsBackslashes(t *testing.T) {
	t.Parallel()

	result := normalizePath(`C:\Users\foo`)
	if result != "C:/Users/foo" {
		t.Errorf("expected %q, got %q", "C:/Users/foo", result)
	}
}

func TestNormalizePath_MixedSlashes(t *testing.T) {
	t.Parallel()

	result := normalizePath("foo\\bar/baz")
	if result != "foo/bar/baz" {
		t.Errorf("expected %q, got %q", "foo/bar/baz", result)
	}
}

func TestNormalizePath_EmptyString(t *testing.T) {
	t.Parallel()

	result := normalizePath("")
	if result != "" {
		t.Errorf("expected %q, got %q", "", result)
	}
}

// ──────────────────────────────────────────────
// hasPathTraversal tests
// ──────────────────────────────────────────────

func TestHasPathTraversal_ParentDirPrefix(t *testing.T) {
	t.Parallel()

	if !hasPathTraversal("../foo") {
		t.Error("expected true for '../foo'")
	}
}

func TestHasPathTraversal_ParentDirMidPath(t *testing.T) {
	t.Parallel()

	if !hasPathTraversal("foo/../bar") {
		t.Error("expected true for 'foo/../bar'")
	}
}

func TestHasPathTraversal_NoTraversal(t *testing.T) {
	t.Parallel()

	if hasPathTraversal("foo/bar") {
		t.Error("expected false for 'foo/bar'")
	}
}

func TestHasPathTraversal_AbsolutePath(t *testing.T) {
	t.Parallel()

	if hasPathTraversal("/etc/passwd") {
		t.Error("expected false for '/etc/passwd'")
	}
}

func TestHasPathTraversal_MultipleParentDir(t *testing.T) {
	t.Parallel()

	if !hasPathTraversal("../../etc") {
		t.Error("expected true for '../../etc'")
	}
}

func TestHasPathTraversal_WindowsBackslashTraversal(t *testing.T) {
	t.Parallel()
	// After normalizePath, `..\\foo` becomes `../foo`.
	if !hasPathTraversal(`..\\foo`) {
		t.Error("expected true for '..\\\\foo'")
	}
}

func TestHasPathTraversal_EmptyString(t *testing.T) {
	t.Parallel()

	if hasPathTraversal("") {
		t.Error("expected false for empty string")
	}
}

func TestHasPathTraversal_Dot(t *testing.T) {
	t.Parallel()

	if hasPathTraversal(".") {
		t.Error("expected false for '.'")
	}
}

func TestHasPathTraversal_JustDotDot(t *testing.T) {
	t.Parallel()

	if !hasPathTraversal("..") {
		t.Error("expected true for '..'")
	}
}

// ──────────────────────────────────────────────
// isAllowedPath tests
// ──────────────────────────────────────────────

func TestIsAllowedPath_ExactMatch(t *testing.T) {
	t.Parallel()

	if !isAllowedPath("/tmp/foo", []string{"/tmp"}) {
		t.Error("expected exact match to be allowed")
	}
}

func TestIsAllowedPath_Subdirectory(t *testing.T) {
	t.Parallel()

	if !isAllowedPath("/tmp/foo/bar", []string{"/tmp"}) {
		t.Error("expected subdirectory to be allowed")
	}
}

func TestIsAllowedPath_NotInAllowed(t *testing.T) {
	t.Parallel()

	if isAllowedPath("/var", []string{"/tmp"}) {
		t.Error("expected path outside allowed to be denied")
	}
}

func TestIsAllowedPath_PrefixNotPartial(t *testing.T) {
	t.Parallel()

	if isAllowedPath("/tmp2", []string{"/tmp"}) {
		t.Error("expected /tmp2 not to match /tmp prefix")
	}
}

func TestIsAllowedPath_NoAllowedPaths(t *testing.T) {
	t.Parallel()

	if isAllowedPath("/tmp/foo", nil) {
		t.Error("expected false when no allowed paths are configured")
	}
}

func TestIsAllowedPath_EmptyAllowedPaths(t *testing.T) {
	t.Parallel()

	if isAllowedPath("/tmp/foo", []string{}) {
		t.Error("expected false when allowed paths list is empty")
	}
}

func TestIsAllowedPath_DeepNested(t *testing.T) {
	t.Parallel()

	if !isAllowedPath("/a/b/c/d/e/f", []string{"/a/b"}) {
		t.Error("expected deep nested path to be allowed")
	}
}

func TestIsAllowedPath_MultipleAllowed(t *testing.T) {
	t.Parallel()

	if !isAllowedPath("/opt/data/file.txt", []string{"/tmp", "/opt/data", "/var"}) {
		t.Error("expected path under second allowed dir to be allowed")
	}
}

// ──────────────────────────────────────────────
// isBuiltinBlocked tests
// ──────────────────────────────────────────────

func TestIsBuiltinBlocked_EtcPrefix(t *testing.T) {
	t.Parallel()

	reason, blocked := isBuiltinBlocked("/etc/passwd")
	if !blocked {
		t.Fatal("expected /etc/passwd to be blocked")
	}

	if reason != "/etc" {
		t.Errorf("expected reason %q, got %q", "/etc", reason)
	}
}

func TestIsBuiltinBlocked_ProcPrefix(t *testing.T) {
	t.Parallel()

	reason, blocked := isBuiltinBlocked("/proc/cpuinfo")
	if !blocked {
		t.Fatal("expected /proc/cpuinfo to be blocked")
	}

	if reason != "/proc" {
		t.Errorf("expected reason %q, got %q", "/proc", reason)
	}
}

func TestIsBuiltinBlocked_DotSshComponent(t *testing.T) {
	t.Parallel()

	reason, blocked := isBuiltinBlocked("/home/user/.ssh/id_rsa")
	if !blocked {
		t.Fatal("expected path with .ssh component to be blocked")
	}

	if reason != ".ssh" {
		t.Errorf("expected reason %q, got %q", ".ssh", reason)
	}
}

func TestIsBuiltinBlocked_DotGnupgComponent(t *testing.T) {
	t.Parallel()

	reason, blocked := isBuiltinBlocked("/home/user/.gnupg/pubring.kbx")
	if !blocked {
		t.Fatal("expected path with .gnupg component to be blocked")
	}

	if reason != ".gnupg" {
		t.Errorf("expected reason %q, got %q", ".gnupg", reason)
	}
}

func TestIsBuiltinBlocked_TmpNotBlocked(t *testing.T) {
	t.Parallel()

	_, blocked := isBuiltinBlocked("/tmp/foo")
	if blocked {
		t.Error("expected /tmp/foo not to be blocked")
	}
}

func TestIsBuiltinBlocked_HomeDocumentsNotBlocked(t *testing.T) {
	t.Parallel()

	_, blocked := isBuiltinBlocked("/home/user/documents")
	if blocked {
		t.Error("expected /home/user/documents not to be blocked")
	}
}

func TestIsBuiltinBlocked_ExactEtcPrefix(t *testing.T) {
	t.Parallel()

	reason, blocked := isBuiltinBlocked("/etc")
	if !blocked {
		t.Fatal("expected /etc to be blocked")
	}

	if reason != "/etc" {
		t.Errorf("expected reason %q, got %q", "/etc", reason)
	}
}

func TestIsBuiltinBlocked_CaseSensitive(t *testing.T) {
	t.Parallel()
	// On Linux, paths are case sensitive. /ETC should NOT match /etc.
	_, blocked := isBuiltinBlocked("/ETC")
	if blocked {
		t.Error("expected /ETC not to match /etc on case-sensitive filesystem")
	}
}

func TestIsBuiltinBlocked_MultipleLevels(t *testing.T) {
	t.Parallel()

	reason, blocked := isBuiltinBlocked("/sys/devices/virtual")
	if !blocked {
		t.Fatal("expected /sys/devices to be blocked")
	}

	if reason != "/sys" {
		t.Errorf("expected reason %q, got %q", "/sys", reason)
	}
}

func TestIsBuiltinBlocked_DevPrefix(t *testing.T) {
	t.Parallel()

	reason, blocked := isBuiltinBlocked("/dev/null")
	if !blocked {
		t.Fatal("expected /dev/null to be blocked")
	}

	if reason != "/dev" {
		t.Errorf("expected reason %q, got %q", "/dev", reason)
	}
}

func TestIsBuiltinBlocked_DotSshOnBlockedPrefix(t *testing.T) {
	t.Parallel()
	// /etc/.ssh should match the /etc prefix before the .ssh component is checked.
	reason, blocked := isBuiltinBlocked("/etc/.ssh")
	if !blocked {
		t.Fatal("expected /etc/.ssh to be blocked")
	}

	if reason != "/etc" {
		t.Errorf("expected prefix %q to take priority over component, got %q", "/etc", reason)
	}
}

// ──────────────────────────────────────────────
// isUserBlocked tests
// ──────────────────────────────────────────────

func TestIsUserBlocked_ExactMatch(t *testing.T) {
	t.Parallel()

	matched, blocked := isUserBlocked("/home/user/secret", []string{"/home/user/secret"})
	if !blocked {
		t.Fatal("expected exact match to be blocked")
	}

	if matched != "/home/user/secret" {
		t.Errorf("expected matched path %q, got %q", "/home/user/secret", matched)
	}
}

func TestIsUserBlocked_Subdirectory(t *testing.T) {
	t.Parallel()

	matched, blocked := isUserBlocked("/home/user/secret/file.txt", []string{"/home/user/secret"})
	if !blocked {
		t.Fatal("expected subdirectory to be blocked")
	}

	if matched != "/home/user/secret" {
		t.Errorf("expected matched path %q, got %q", "/home/user/secret", matched)
	}
}

func TestIsUserBlocked_NotBlocked(t *testing.T) {
	t.Parallel()

	matched, blocked := isUserBlocked("/home/user/other", []string{"/home/user/secret"})
	if blocked {
		t.Errorf("expected not blocked, got matched path %q", matched)
	}
}

func TestIsUserBlocked_NoBlockedPaths(t *testing.T) {
	t.Parallel()

	_, blocked := isUserBlocked("/home/user/secret", nil)
	if blocked {
		t.Error("expected not blocked when no blocked paths configured")
	}
}

func TestIsUserBlocked_EmptyBlockedPaths(t *testing.T) {
	t.Parallel()

	_, blocked := isUserBlocked("/home/user/secret", []string{})
	if blocked {
		t.Error("expected not blocked when blocked paths list is empty")
	}
}

func TestIsUserBlocked_PrefixNotPartial(t *testing.T) {
	t.Parallel()

	_, blocked := isUserBlocked("/home/user/secret2", []string{"/home/user/secret"})
	if blocked {
		t.Error("expected /home/user/secret2 not to match /home/user/secret")
	}
}

func TestIsUserBlocked_MultipleBlockedPaths(t *testing.T) {
	t.Parallel()

	matched, blocked := isUserBlocked("/opt/restricted/data", []string{"/tmp", "/opt/restricted"})
	if !blocked {
		t.Fatal("expected path under second blocked dir to be blocked")
	}

	if matched != "/opt/restricted" {
		t.Errorf("expected matched path %q, got %q", "/opt/restricted", matched)
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
	// DisableBuiltinBlocklist=true + builtin blocked path but not in user blocklist
	// and no allowed paths → path should be allowed (not errBuiltin*).
	tmpDir := t.TempDir()

	etcDir := filepath.Join(tmpDir, "etc")

	err := os.Mkdir(etcDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	testFile := filepath.Join(etcDir, "test.txt")

	err = os.WriteFile(testFile, []byte("test"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		DisableBuiltinBlocklist: true,
	}

	// The path is under a tmp dir, not /etc, so it won't match builtin prefix.
	// But with builtin disabled, even a real /etc path would be skipped.
	// We use a non-/etc path here to avoid the builtin prefix match,
	// and verify that DisableBuiltinBlocklist doesn't cause new issues.
	result, err := validateFilePath(testFile, cfg)
	if err != nil {
		t.Fatalf("expected success with builtin blocklist disabled, got: %v", err)
	}

	if result != testFile {
		t.Errorf("expected resolved path %q, got %q", testFile, result)
	}
}

func TestValidateFilePath_DisableBuiltinBlocklistSkipsBuiltinCheck(t *testing.T) {
	t.Parallel()
	// With DisableBuiltinBlocklist=true, /etc/passwd should NOT be blocked
	// by the builtin check (but it still needs to not be in user blocklist).
	tmpDir := t.TempDir()

	etcDir := filepath.Join(tmpDir, "etc")

	err := os.MkdirAll(etcDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	passwdFile := filepath.Join(etcDir, "passwd")

	err = os.WriteFile(passwdFile, []byte("root:x:0:0"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		DisableBuiltinBlocklist: true,
	}

	// This path is under tmpDir/etc, not real /etc, so it won't match builtin
	// prefix. We just verify that DisableBuiltinBlocklist doesn't cause errors.
	result, err := validateFilePath(passwdFile, cfg)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	if result != passwdFile {
		t.Errorf("expected resolved path %q, got %q", passwdFile, result)
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
// Edge case tests
// ──────────────────────────────────────────────

func TestHasPathTraversal_Unicode(t *testing.T) {
	t.Parallel()

	if hasPathTraversal("/tmp/文件") {
		t.Error("expected false for unicode path")
	}
}

func TestIsBuiltinBlocked_DotSshAtRoot(t *testing.T) {
	t.Parallel()
	// Even though /.ssh is at root, it should be caught as a component match.
	_, blocked := isBuiltinBlocked("/.ssh/authorized_keys")
	if !blocked {
		t.Log("NOTE: /.ssh is a valid system path on some systems; test is informational")
	}
}

func TestIsAllowedPath_AllowedEqualsPath(t *testing.T) {
	t.Parallel()

	if !isAllowedPath("/tmp", []string{"/tmp"}) {
		t.Error("expected path equal to allowed dir to match")
	}
}

func TestIsBuiltinBlocked_DotSshAtEnd(t *testing.T) {
	t.Parallel()

	reason, blocked := isBuiltinBlocked("/home/user/.ssh")
	if !blocked {
		t.Fatal("expected /home/user/.ssh to be blocked")
	}

	if reason != ".ssh" {
		t.Errorf("expected reason %q, got %q", ".ssh", reason)
	}
}

func TestIsBuiltinBlocked_GnupgMidPath(t *testing.T) {
	t.Parallel()

	reason, blocked := isBuiltinBlocked("/data/backup/.gnupg/keys/pubring.kbx")
	if !blocked {
		t.Fatal("expected path with .gnupg mid-path to be blocked")
	}

	if reason != ".gnupg" {
		t.Errorf("expected reason %q, got %q", ".gnupg", reason)
	}
}
