package wastebin

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"unicode/utf8"
)

const (
	sniffSize       = 8192
	binaryThreshold = 0.05 // max ratio of control chars to still consider text
)

// isBinaryMagic checks the first bytes of data for known binary format signatures.
// Returns true if data starts with a known binary magic byte sequence.
func isBinaryMagic(data []byte) bool {
	// PDF: %PDF
	if bytes.HasPrefix(data, []byte("%PDF")) {
		return true
	}
	// PNG: \x89PNG\r\n\x1a\n
	if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		return true
	}
	// ZIP and derived formats (docx, jar, etc.): PK
	if bytes.HasPrefix(data, []byte("PK")) {
		return true
	}
	// GZip: \x1f\x8b
	if bytes.HasPrefix(data, []byte("\x1f\x8b")) {
		return true
	}
	// ELF: \x7fELF
	if bytes.HasPrefix(data, []byte("\x7fELF")) {
		return true
	}

	return false
}

// IsLikelyText checks if content looks like text.
// Checks:
// 0. Known binary magic bytes (PDF, PNG, ZIP, GZip, ELF) → immediate rejection
// 1. Complete data is valid UTF-8
// 2. First 8KB has no null bytes
// 3. First 8KB control character ratio (excluding \n\r	) < 5%.
func IsLikelyText(data []byte) bool {
	if len(data) == 0 {
		return true
	}

	// Check magic bytes for known binary formats.
	if isBinaryMagic(data) {
		return false
	}

	if !utf8.Valid(data) {
		return false
	}

	size := min(len(data), sniffSize)
	buf := data[:size]

	var ctrlCount int

	for _, b := range buf {
		if b == 0 {
			return false
		}

		if b <= 0x1F && b != '\n' && b != '\r' && b != '\t' {
			ctrlCount++
		}
	}

	return float64(ctrlCount)/float64(len(buf)) < binaryThreshold
}

// IsLikelyTextFile reads first 8KB from path and calls IsLikelyText.
func IsLikelyTextFile(path string) (bool, error) {
	f, err := os.Open(path) //nolint:gosec // Path validated through validateFilePath pipeline
	if err != nil {
		return false, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // Close failure is non-critical; file was already read successfully

	buf := make([]byte, sniffSize+utf8.UTFMax)

	n, err := io.ReadFull(f, buf)
	if err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			// File is smaller than the buffer; n may be 0.
			return IsLikelyText(buf[:n]), nil
		}

		return false, fmt.Errorf("failed to read file: %w", err)
	}

	return IsLikelyText(buf[:n]), nil
}
