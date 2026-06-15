package wastebin //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsLikelyText_Empty(t *testing.T) {
	t.Parallel()

	if !IsLikelyText(nil) {
		t.Error("expected nil to be likely text")
	}

	if !IsLikelyText([]byte{}) {
		t.Error("expected empty slice to be likely text")
	}
}

func TestIsLikelyText_ValidUTF8(t *testing.T) {
	t.Parallel()

	data := []byte("Hello, 世界! This is a normal text file with valid UTF-8 content.\n")
	if !IsLikelyText(data) {
		t.Error("expected valid UTF-8 text to be likely text")
	}
}

func TestIsLikelyText_InvalidUTF8(t *testing.T) {
	t.Parallel()
	// Invalid UTF-8 sequence: 0xFF is not valid UTF-8.
	data := []byte("Hello\xFFWorld")
	if IsLikelyText(data) {
		t.Error("expected invalid UTF-8 to not be likely text")
	}
}

func TestIsLikelyText_BinaryWithNulls(t *testing.T) {
	t.Parallel()
	// Binary data with null bytes.
	data := []byte{0x00, 0x01, 0x02, 0x48, 0x65, 0x6C}
	if IsLikelyText(data) {
		t.Error("expected data with null bytes to not be likely text")
	}
}

func TestIsLikelyText_HighControlCharRatio(t *testing.T) {
	t.Parallel()
	// More than 5% control characters (excluding \n\r	).
	// 10 control chars + 90 printable = 10%, above 5%.
	data := make([]byte, 100)
	for i := range 10 {
		data[i] = 0x01 // SOH control char
	}

	for i := 10; i < 100; i++ {
		data[i] = 'A'
	}

	if IsLikelyText(data) {
		t.Error("expected data with >5% control chars to not be likely text")
	}
}

func TestIsLikelyText_JustBelowThreshold(t *testing.T) {
	t.Parallel()
	// Control chars (excluding \n\r	) below 5%.
	// 3 control chars + 97 printable = ~3%, below 5%.
	data := make([]byte, 100)
	for i := range 3 {
		data[i] = 0x01 // SOH control char
	}

	for i := 3; i < 100; i++ {
		data[i] = 'A'
	}

	if !IsLikelyText(data) {
		t.Error("expected data with <5% control chars to be likely text")
	}
}

func TestIsLikelyText_NewlinesAndTabsAllowed(t *testing.T) {
	t.Parallel()
	// \n, \r, 	 should not count as control chars.
	data := []byte("line1\nline2\rline3\tline4\n")
	if !IsLikelyText(data) {
		t.Error("expected data with only \\n\\r\\t control chars to be likely text")
	}
}

func TestIsLikelyText_OnlyFirst8KB(t *testing.T) {
	t.Parallel()
	// Only first 8KB is checked; null bytes after that should pass.
	data := make([]byte, 16384)
	for i := range 8192 {
		data[i] = 'A'
	}
	// Null bytes after 8KB.
	for i := 8192; i < 16384; i++ {
		data[i] = 0x00
	}

	if !IsLikelyText(data) {
		t.Error("expected data with nulls only after 8KB to be likely text")
	}
}

func TestIsLikelyTextFile_NotFound(t *testing.T) {
	t.Parallel()

	_, err := IsLikelyTextFile("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestIsLikelyTextFile_TextFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	path := filepath.Join(dir, "test.txt")

	err := os.WriteFile(path, []byte("Hello, world!\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := IsLikelyTextFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ok {
		t.Error("expected text file to be likely text")
	}
}

func TestIsLikelyTextFile_BinaryFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.bin")
	// Write binary data with null byte.
	err := os.WriteFile(path, []byte{0x00, 0xFF, 0xFE}, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := IsLikelyTextFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ok {
		t.Error("expected binary file to not be likely text")
	}
}
