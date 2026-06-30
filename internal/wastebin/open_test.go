package wastebin //nolint:testpackage // white-box tests need access to unexported functions

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
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

func TestOpenRelNoFollow_FIFORejected(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	fifoPath := filepath.Join(tmpDir, "test.fifo")

	mkfifoErr := unix.Mkfifo(fifoPath, 0o600)
	if mkfifoErr != nil {
		t.Fatal(mkfifoErr)
	}

	//nolint:gosec // Test helper opens known temp dir
	parent, err := os.Open(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	//nolint:errcheck // Read-only dir; close error non-critical
	defer parent.Close()

	// Use a goroutine with timeout to detect blocking: a FIFO opened
	// without O_NONBLOCK hangs forever on O_RDONLY when no writer exists.
	done := make(chan struct{})

	var openErr error

	go func() {
		_, openErr = openRelNoFollow(parent, "test.fifo")

		close(done)
	}()

	select {
	case <-done:
		if !errors.Is(openErr, errFilePathCannotBeUsed) {
			t.Errorf("expected errFilePathCannotBeUsed for FIFO, got: %v", openErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("openRelNoFollow blocked on FIFO (test timeout)")
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
