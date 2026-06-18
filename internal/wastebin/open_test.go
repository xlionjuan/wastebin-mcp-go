package wastebin //nolint:testpackage // white-box tests need access to unexported functions

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenRelNoFollow_EmptyPath(t *testing.T) {
	t.Parallel()

	parent, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	//nolint:errcheck // Read-only dir; close error non-critical
	defer parent.Close()

	_, err = openRelNoFollow(parent, "")
	if !errors.Is(err, errOpenEmptyPath) {
		t.Errorf("expected errOpenEmptyPath, got: %v", err)
	}

	_, err = openRelNoFollow(parent, ".")
	if !errors.Is(err, errOpenEmptyPath) {
		t.Errorf("expected errOpenEmptyPath for '.', got: %v", err)
	}
}

func TestOpenRelNoFollow_IntermediateNonDirectory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	subDir := filepath.Join(tmpDir, "subdir")

	err := os.Mkdir(subDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	notADir := filepath.Join(subDir, "notadir")

	err = os.WriteFile(notADir, []byte("i am a file, not a directory"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Two-level intermediate path: the first level "subdir" is a real
	// directory (opened from the root fd), the second level "notadir" is
	// a file (opened from subdir's fd, where parentFd != firstFd).
	// This exercises both the mode check AND the parentFd != firstFd
	// cleanup branch in the error path.
	//nolint:gosec // Test helper opens known temp dir
	parent, err := os.Open(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	//nolint:errcheck // Read-only dir; close error non-critical
	defer parent.Close()

	_, err = openRelNoFollow(parent, "subdir/notadir/somechild.txt")
	if err == nil {
		t.Fatal("expected error for intermediate non-directory component")
	}

	if !errors.Is(err, errFilePathCannotBeUsed) {
		t.Errorf("expected errFilePathCannotBeUsed, got: %v", err)
	}
}

func TestOpenFileFromRoot_RootOpenFails(t *testing.T) {
	t.Parallel()

	// Use a path that IS contained in the allowed root (so findAllowedRoot
	// succeeds), but the root path itself does not exist on disk (so
	// os.OpenFile fails).
	_, err := openFileFromRoot("/nonexistent-root/subdir/file", []string{"/nonexistent-root"})
	if err == nil {
		t.Fatal("expected error when root directory does not exist")
	}
}

func TestOpenFileFromRoot_PathNotUnderAllowedRoot(t *testing.T) {
	t.Parallel()

	// When findAllowedRoot fails (path not under any allowed root),
	// openFileFromRoot returns errPathNotAllowed.
	_, err := openFileFromRoot("/some/other/path/file.txt", []string{"/tmp/allowed"})
	if !errors.Is(err, errPathNotAllowed) {
		t.Errorf("expected errPathNotAllowed, got: %v", err)
	}
}

func TestFindAllowedRoot_NoMatch(t *testing.T) {
	t.Parallel()

	root, rel, ok := findAllowedRoot("/tmp/some/path", []string{"/nonexistent"})
	if ok {
		t.Error("expected ok=false")
	}

	if root != "" {
		t.Errorf("expected empty root, got %q", root)
	}

	if rel != "" {
		t.Errorf("expected empty rel, got %q", rel)
	}
}
