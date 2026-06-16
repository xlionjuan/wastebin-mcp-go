package wastebin //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"testing"
)

func TestParseSandboxMounts_Empty(t *testing.T) {
	t.Parallel()

	mounts, err := ParseSandboxMounts("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mounts) != 0 {
		t.Errorf("expected 0 mounts, got %d", len(mounts))
	}
}

func TestParseSandboxMounts_Single(t *testing.T) {
	t.Parallel()

	mounts, err := ParseSandboxMounts("/host/path:/sandbox/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}

	if mounts[0].HostPath != "/host/path" {
		t.Errorf("expected host path /host/path, got %q", mounts[0].HostPath)
	}

	if mounts[0].SandboxPath != "/sandbox/path" {
		t.Errorf("expected sandbox path /sandbox/path, got %q", mounts[0].SandboxPath)
	}
}

func TestParseSandboxMounts_Multiple(t *testing.T) {
	t.Parallel()

	mounts, err := ParseSandboxMounts("/a:/x,/b:/y")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(mounts))
	}

	if mounts[0].HostPath != "/a" || mounts[0].SandboxPath != "/x" {
		t.Errorf("first mount: got %+v", mounts[0])
	}

	if mounts[1].HostPath != "/b" || mounts[1].SandboxPath != "/y" {
		t.Errorf("second mount: got %+v", mounts[1])
	}
}

func TestParseSandboxMounts_EmptyPair(t *testing.T) {
	t.Parallel()

	mounts, err := ParseSandboxMounts("/a:/x,,/b:/y")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mounts) != 2 {
		t.Errorf("expected 2 mounts (skipping empty), got %d: %v", len(mounts), mounts)
	}

	if mounts[0].HostPath != "/a" {
		t.Errorf("expected first host path /a, got %q", mounts[0].HostPath)
	}

	if mounts[1].HostPath != "/b" {
		t.Errorf("expected second host path /b, got %q", mounts[1].HostPath)
	}
}

func TestParseSandboxMounts_InvalidFormat(t *testing.T) {
	t.Parallel()

	_, err := ParseSandboxMounts("invalid")
	if err == nil {
		t.Fatal("expected error for invalid format (no colon)")
	}

	_, err = ParseSandboxMounts(":/sandbox")
	if err == nil {
		t.Fatal("expected error for empty host path")
	}

	_, err = ParseSandboxMounts("/host:")
	if err == nil {
		t.Fatal("expected error for empty sandbox path")
	}
}

func TestParseSandboxMounts_Whitespace(t *testing.T) {
	t.Parallel()

	mounts, err := ParseSandboxMounts("  /a:/x  ,  /b:/y  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(mounts))
	}

	if mounts[0].HostPath != "/a" || mounts[0].SandboxPath != "/x" {
		t.Errorf("first mount: got %+v", mounts[0])
	}

	if mounts[1].HostPath != "/b" || mounts[1].SandboxPath != "/y" {
		t.Errorf("second mount: got %+v", mounts[1])
	}
}

func TestTranslator_NoMounts(t *testing.T) {
	t.Parallel()

	tr := NewTranslator(nil)

	_, ok := tr.Translate("/any/path")
	if ok {
		t.Error("expected no match with empty mounts")
	}
}

func TestTranslator_ExactMatch(t *testing.T) {
	t.Parallel()

	mounts := []SandboxMount{
		{HostPath: "/host/workspace", SandboxPath: "/workspace"},
	}
	tr := NewTranslator(mounts)

	host, ok := tr.Translate("/workspace")
	if !ok {
		t.Fatal("expected match")
	}

	if host != "/host/workspace" {
		t.Errorf("got %q, want %q", host, "/host/workspace")
	}
}

func TestTranslator_PrefixMatch(t *testing.T) {
	t.Parallel()

	mounts := []SandboxMount{
		{HostPath: "/host/workspace", SandboxPath: "/workspace"},
	}
	tr := NewTranslator(mounts)

	host, ok := tr.Translate("/workspace/subdir/file.go")
	if !ok {
		t.Fatal("expected match")
	}

	if host != "/host/workspace/subdir/file.go" {
		t.Errorf("got %q, want %q", host, "/host/workspace/subdir/file.go")
	}
}

func TestTranslator_NoMatch(t *testing.T) {
	t.Parallel()

	mounts := []SandboxMount{
		{HostPath: "/host/workspace", SandboxPath: "/workspace"},
	}
	tr := NewTranslator(mounts)

	_, ok := tr.Translate("/other/path")
	if ok {
		t.Error("expected no match for unmounted path")
	}
}

func TestTranslator_PrefixNotPartial(t *testing.T) {
	t.Parallel()
	// /workspace2 should NOT match mount /workspace.
	mounts := []SandboxMount{
		{HostPath: "/host/workspace", SandboxPath: "/workspace"},
	}
	tr := NewTranslator(mounts)

	_, ok := tr.Translate("/workspace2")
	if ok {
		t.Error("expected no match for /workspace2 against mount /workspace")
	}
}

func TestTranslator_FirstMatchWins(t *testing.T) {
	t.Parallel()

	mounts := []SandboxMount{
		{HostPath: "/host/first", SandboxPath: "/mnt"},
		{HostPath: "/host/second", SandboxPath: "/mnt/sub"},
	}
	tr := NewTranslator(mounts)

	host, ok := tr.Translate("/mnt/sub/file.txt")
	if !ok {
		t.Fatal("expected match")
	}
	// First mount matches (/mnt is prefix of /mnt/sub/file.txt).
	if host != "/host/first/sub/file.txt" {
		t.Errorf("got %q, want %q", host, "/host/first/sub/file.txt")
	}
}

func TestTranslator_MultipleMounts(t *testing.T) {
	t.Parallel()

	mounts := []SandboxMount{
		{HostPath: "/host/data", SandboxPath: "/data"},
		{HostPath: "/host/config", SandboxPath: "/config"},
	}
	tr := NewTranslator(mounts)

	host, ok := tr.Translate("/config/app.yaml")
	if !ok {
		t.Fatal("expected match")
	}

	if host != "/host/config/app.yaml" {
		t.Errorf("got %q, want %q", host, "/host/config/app.yaml")
	}
}

func TestTranslator_PathTraversal_Rejected(t *testing.T) {
	t.Parallel()

	mounts := []SandboxMount{
		{HostPath: "/host/workspace", SandboxPath: "/workspace"},
	}
	tr := NewTranslator(mounts)

	// filepath.Join("/host/workspace", "../secret.txt") → "/host/secret.txt"
	// The normalization removes ".." — the mount check after translation
	// must prevent the escape.
	host, ok := tr.Translate("/workspace/../secret.txt")
	if !ok {
		t.Fatal("expected match (translation happens before validation)")
	}

	if host == "/host/workspace/secret.txt" {
		t.Error("expected translated path to escape workspace due to ..")
	}

	// Verify isUnderMountHost catches the escape.
	if isUnderMountHost(host, mounts) {
		t.Errorf("expected translated path %q to NOT be under mount host root /host/workspace", host)
	}
}

func TestTranslator_DoubleDotDot_Rejected(t *testing.T) {
	t.Parallel()

	mounts := []SandboxMount{
		{HostPath: "/host/workspace", SandboxPath: "/workspace"},
	}
	tr := NewTranslator(mounts)

	host, ok := tr.Translate("/workspace/../../etc/passwd")
	if !ok {
		t.Fatal("expected match")
	}

	if isUnderMountHost(host, mounts) {
		t.Errorf("expected translated path %q to NOT be under mount host root", host)
	}
}

func TestIsUnderMountHost_ExactMatch(t *testing.T) {
	t.Parallel()

	mounts := []SandboxMount{
		{HostPath: "/host/workspace", SandboxPath: "/workspace"},
	}

	if !isUnderMountHost("/host/workspace", mounts) {
		t.Error("expected exact match to be under mount")
	}
}

func TestIsUnderMountHost_Subdirectory(t *testing.T) {
	t.Parallel()

	mounts := []SandboxMount{
		{HostPath: "/host/workspace", SandboxPath: "/workspace"},
	}

	if !isUnderMountHost("/host/workspace/subdir/file.go", mounts) {
		t.Error("expected subdirectory to be under mount")
	}
}

func TestIsUnderMountHost_Escaped(t *testing.T) {
	t.Parallel()

	mounts := []SandboxMount{
		{HostPath: "/host/workspace", SandboxPath: "/workspace"},
	}

	if isUnderMountHost("/host/secret.txt", mounts) {
		t.Error("expected /host/secret.txt to NOT be under /host/workspace")
	}

	if isUnderMountHost("/etc/passwd", mounts) {
		t.Error("expected /etc/passwd to NOT be under /host/workspace")
	}
}

func TestIsUnderMountHost_MultipleMounts(t *testing.T) {
	t.Parallel()

	mounts := []SandboxMount{
		{HostPath: "/host/data", SandboxPath: "/data"},
		{HostPath: "/host/config", SandboxPath: "/config"},
	}

	if !isUnderMountHost("/host/config/app.yaml", mounts) {
		t.Error("expected /host/config/app.yaml to match /host/config mount")
	}

	if isUnderMountHost("/host/other/secret.txt", mounts) {
		t.Error("expected /host/other/secret.txt to NOT match any mount")
	}
}
