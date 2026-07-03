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

func TestIsLikelyTextFile_UTF8CrossingSniffBoundary(t *testing.T) {
	t.Parallel()

	// 3-byte UTF-8 character (中 = 0xE4 0xB8 0xAD) straddling the
	// 8192-byte sniff boundary. 8190 bytes of ASCII + 3-byte rune
	// starting at byte 8190 means the last byte of the rune is at
	// index 8192. Since the file is 8193 bytes (≤ readSize), the
	// entire file is read (EOF) and no trimming is needed — the
	// full rune is present in the buffer.
	dir := t.TempDir()
	path := filepath.Join(dir, "utf8_boundary.txt")

	data := make([]byte, 8193)
	for i := range 8190 {
		data[i] = 'A'
	}

	data[8190] = 0xE4
	data[8191] = 0xB8
	data[8192] = 0xAD

	err := os.WriteFile(path, data, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := IsLikelyTextFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ok {
		t.Error("expected UTF-8 file with rune straddling sniff boundary to be likely text")
	}
}

func TestIsLikelyTextFile_EmojiCrossingSniffBoundary(t *testing.T) {
	t.Parallel()

	// 4-byte emoji (🔥 = 0xF0 0x9F 0x94 0xA5) straddling the 8192-byte
	// sniff boundary. 8190 bytes of ASCII + 4-byte emoji starting at
	// byte 8190. Since the file is 8194 bytes (≤ readSize), the entire
	// file is read (EOF) — the full emoji is present and no trimming
	// is needed.
	dir := t.TempDir()
	path := filepath.Join(dir, "emoji_boundary.txt")

	data := make([]byte, 8194)
	for i := range 8190 {
		data[i] = 'A'
	}

	// 🔥 = F0 9F 94 A5
	data[8190] = 0xF0
	data[8191] = 0x9F
	data[8192] = 0x94
	data[8193] = 0xA5

	err := os.WriteFile(path, data, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := IsLikelyTextFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ok {
		t.Error("expected file with emoji straddling sniff boundary to be likely text")
	}
}

func TestIsLikelyTextFile_ExactSniffSizeWithTruncation(t *testing.T) {
	t.Parallel()

	// File of exactly 8192 bytes where the last byte is 0x80 (a lone
	// continuation byte — invalid UTF-8 on its own). With readSize =
	// sniffSize + utf8.UTFMax, the entire 8192-byte file is read (EOF),
	// so no trimming is applied. 0x80 at the end makes the buffer
	// invalid UTF-8, and the file is correctly classified as non-text.
	dir := t.TempDir()
	path := filepath.Join(dir, "exact_sniff.txt")

	data := make([]byte, 8192)
	for i := range 8191 {
		data[i] = 'A'
	}

	data[8191] = 0x80

	err := os.WriteFile(path, data, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := IsLikelyTextFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ok {
		t.Error("expected file with lone continuation byte at EOF boundary to NOT be likely text")
	}
}

func TestIsLikelyTextFile_ValidUTF8AtExactSniffSize(t *testing.T) {
	t.Parallel()

	// File of exactly 8192 bytes with valid UTF-8 (all ASCII).
	// No trimming should occur, and it should be identified as text.
	dir := t.TempDir()
	path := filepath.Join(dir, "exact_valid.txt")

	data := make([]byte, 8192)
	for i := range data {
		data[i] = 'A'
	}

	err := os.WriteFile(path, data, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := IsLikelyTextFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ok {
		t.Error("expected exactly 8192 bytes of ASCII to be likely text")
	}
}

func TestIsLikelyTextFile_LoneContinuationByteNotCompletable(t *testing.T) {
	t.Parallel()

	// File larger than readSize (sniffSize + utf8.UTFMax). First
	// sniffSize-1 bytes are 'A', byte sniffSize-1 is a lone 0x80
	// (continuation byte), and everything from sniffSize onward is
	// ASCII. The 0x80 cannot be completed by the extra bytes into
	// valid UTF-8 — it's genuinely invalid, not boundary-truncated.
	dir := t.TempDir()
	path := filepath.Join(dir, "lone_cont.txt")

	data := make([]byte, readSize+100)
	for i := range data {
		data[i] = 'A'
	}

	data[sniffSize-1] = 0x80

	err := os.WriteFile(path, data, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := IsLikelyTextFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ok {
		t.Error("expected file with lone continuation byte not completable by extra bytes to NOT be likely text")
	}
}
