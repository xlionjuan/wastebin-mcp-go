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

func TestIsLikelyText_TruncatedMultiByte(t *testing.T) {
	t.Parallel()

	data := make([]byte, 8194)
	for i := range 8191 {
		data[i] = 'A'
	}

	data[8191] = 0xE4
	data[8192] = 0xB8
	data[8193] = 0xAD

	if !IsLikelyText(data) {
		t.Error("expected UTF-8 data split at the sniff boundary to be likely text")
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

func TestIsLikelyTextFile_StatError(t *testing.T) {
	t.Parallel()

	// Path in a non-existent directory should cause os.Stat error.
	result, err := IsLikelyTextFile("/nonexistent/path/that/does/not/exist/12345")

	if result {
		t.Error("expected false when file doesn't exist")
	}

	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestIsLikelyTextFile_ReadError(t *testing.T) {
	t.Parallel()

	// Create a directory (can't be read as file for sniffing).
	tmpDir := t.TempDir()
	result, err := IsLikelyTextFile(tmpDir)

	if result {
		t.Error("expected false when path is a directory")
	}

	if err == nil {
		t.Error("expected error when reading directory as file")
	}
}

func TestIsLikelyTextFile_LargeFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "large.txt")

	// Create a file > sniffSize (sniffSize is 8192 bytes).
	data := make([]byte, 8193)
	for i := range data {
		data[i] = 'a'
	}

	err := os.WriteFile(filePath, data, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	result, err := IsLikelyTextFile(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result {
		t.Error("expected true for large text file")
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

// --- Magic-byte signature tests ---

func TestIsLikelyText_PDFMagicBytes(t *testing.T) {
	t.Parallel()
	// Minimal ASCII PDF — valid UTF-8 but must be caught by magic-byte check.
	data := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\n")
	if IsLikelyText(data) {
		t.Error("expected PDF magic bytes to be detected as non-text")
	}
}

func TestIsLikelyText_PNGMagicBytes(t *testing.T) {
	t.Parallel()
	// PNG header.
	data := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	if IsLikelyText(data) {
		t.Error("expected PNG magic bytes to be detected as non-text")
	}
}

func TestIsLikelyText_ZIPLocalFileMagicBytes(t *testing.T) {
	t.Parallel()
	// ZIP local file header: PK\x03\x04
	data := []byte{'P', 'K', 0x03, 0x04, 'h', 'e', 'l', 'l', 'o'}
	if IsLikelyText(data) {
		t.Error("expected ZIP PK\\x03\\x04 magic bytes to be detected as non-text")
	}
}

func TestIsLikelyText_ZIPCentralDirMagicBytes(t *testing.T) {
	t.Parallel()
	// ZIP central directory: PK\x05\x06
	data := []byte{'P', 'K', 0x05, 0x06, 'h', 'e', 'l', 'l', 'o'}
	if IsLikelyText(data) {
		t.Error("expected ZIP PK\\x05\\x06 magic bytes to be detected as non-text")
	}
}

func TestIsLikelyText_ZIP64MagicBytes(t *testing.T) {
	t.Parallel()
	// ZIP64 end of central dir: PK\x07\x08
	data := []byte{'P', 'K', 0x07, 0x08, 'h', 'e', 'l', 'l', 'o'}
	if IsLikelyText(data) {
		t.Error("expected ZIP PK\\x07\\x08 magic bytes to be detected as non-text")
	}
}

func TestIsLikelyText_GZipMagicBytes(t *testing.T) {
	t.Parallel()
	// GZip header.
	data := []byte{0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00}
	if IsLikelyText(data) {
		t.Error("expected GZip magic bytes to be detected as non-text")
	}
}

func TestIsLikelyText_ELFMagicBytes(t *testing.T) {
	t.Parallel()
	// ELF header; 0x7f and ASCII are valid UTF-8, so this specifically tests
	// the magic-byte guard (would otherwise pass UTF-8 validation).
	data := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
	if IsLikelyText(data) {
		t.Error("expected ELF magic bytes to be detected as non-text")
	}
}

// --- Edge cases: text that starts like a signature but is not binary ---

func TestIsLikelyText_TextStartingWithP(t *testing.T) {
	t.Parallel()
	// "P" + not ZIP signature → still text.
	data := []byte("Parking instructions for the building")
	if !IsLikelyText(data) {
		t.Error("expected text starting with 'P' (no ZIP sig) to be likely text")
	}
}

func TestIsLikelyText_TextStartingWithPercent(t *testing.T) {
	t.Parallel()
	// "%" + not PDF signature → still text.
	data := []byte("% discount applied to all items")
	if !IsLikelyText(data) {
		t.Error("expected text starting with '%' (no PDF sig) to be likely text")
	}
}

func TestIsLikelyText_TextStartingWithPK(t *testing.T) {
	t.Parallel()
	// "PK" without \x03\x04, \x05\x06, or \x07\x08 is fine.
	data := []byte("PK is a common initialism in many contexts")
	if !IsLikelyText(data) {
		t.Error("expected text starting with 'PK' (without ZIP sig) to be likely text")
	}
}

func TestIsLikelyText_ShortInputNoFalsePositive(t *testing.T) {
	t.Parallel()
	// Data too short for any 4-byte signature should not trigger.
	if !IsLikelyText([]byte("PK")) {
		t.Error("expected 2-byte 'PK' to be likely text")
	}

	if !IsLikelyText([]byte("%")) {
		t.Error("expected 1-byte '%' to be likely text")
	}

	if !IsLikelyText([]byte("P")) {
		t.Error("expected 1-byte 'P' to be likely text")
	}
}
