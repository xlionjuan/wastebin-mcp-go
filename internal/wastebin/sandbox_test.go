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

func TestParseSandboxMounts_RelativeHostPath(t *testing.T) {
	t.Parallel()

	_, err := ParseSandboxMounts("./workspace:/mnt")
	if err == nil {
		t.Fatal("expected error for relative host path")
	}

	_, err = ParseSandboxMounts("relative/path:/sandbox")
	if err == nil {
		t.Fatal("expected error for relative host path")
	}
}

func TestParseSandboxMounts_SandboxPathCleaning(t *testing.T) {
	t.Parallel()

	mounts, err := ParseSandboxMounts("/host://sandbox//path///")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}

	if mounts[0].HostPath != "/host" {
		t.Errorf("expected host path /host, got %q", mounts[0].HostPath)
	}

	if mounts[0].SandboxPath != "/sandbox/path" {
		t.Errorf("expected sandbox path /sandbox/path, got %q", mounts[0].SandboxPath)
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

func TestParseSandboxMounts_Overlapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "prefix overlap",
			input: "/host/workspace:/workspace,/host/workspace-sub:/workspace/sub",
		},
		{
			name:  "same path (duplicate)",
			input: "/host/a:/workspace,/host/b:/workspace",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseSandboxMounts(tt.input)
			if err == nil {
				t.Fatal("expected error for overlapping sandbox mounts")
			}
		})
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

	// filepath.Rel normalizes ".." before computing the relative path,
	// so Translate does not match paths with parent directory references.
	// Path traversal is caught upstream by hasPathTraversal.
	_, ok := tr.Translate("/workspace/../secret.txt")
	if ok {
		t.Fatal("expected no match — filepath.Rel normalizes .. so this is not under /workspace")
	}
}

func TestTranslator_DoubleDotDot_Rejected(t *testing.T) {
	t.Parallel()

	mounts := []SandboxMount{
		{HostPath: "/host/workspace", SandboxPath: "/workspace"},
	}
	tr := NewTranslator(mounts)

	// filepath.Rel normalizes "..", so this path is not under /workspace.
	_, ok := tr.Translate("/workspace/../../etc/passwd")
	if ok {
		t.Fatal("expected no match — filepath.Rel normalizes .. so this is not under /workspace")
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
