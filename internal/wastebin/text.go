package wastebin

import (
	"errors"
	"fmt"
	"io"
	"os"
	"unicode/utf8"
)

const (
	sniffSize       = 8192
	binaryThreshold = 0.05 // max ratio of control chars to still consider text
	minSigLen       = 2    // minimum data length to check for any binary signature
)

// hasBinarySignature checks whether data starts with a known binary format
// magic-byte signature. Only 4-byte ZIP signatures are matched (PK\x03\x04,
// PK\x05\x06, PK\x07\x08) — plain "PK" alone is not a binary signal because
// it appears in ordinary text (e.g. "PK is a common initialism...").
func hasBinarySignature(data []byte) bool {
	if len(data) < minSigLen {
		return false
	}

	// PDF: %PDF
	if len(data) >= 4 && string(data[:4]) == "%PDF" {
		return true
	}

	// PNG: \x89PNG
	if len(data) >= 4 && data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
		return true
	}

	// ZIP: PK\x03\x04, PK\x05\x06, PK\x07\x08 (NOT plain "PK")
	if data[0] == 'P' && data[1] == 'K' && len(data) >= 4 {
		if (data[2] == 0x03 && data[3] == 0x04) ||
			(data[2] == 0x05 && data[3] == 0x06) ||
			(data[2] == 0x07 && data[3] == 0x08) {
			return true
		}
	}

	// GZip: \x1f\x8b
	if data[0] == 0x1f && data[1] == 0x8b {
		return true
	}

	// ELF: \x7fELF
	if len(data) >= 4 && data[0] == 0x7f && data[1] == 'E' && data[2] == 'L' && data[3] == 'F' {
		return true
	}

	return false
}

// IsLikelyText checks if content looks like text.
// Checks:
// 0. Magic-byte signatures for common binary formats (PDF, PNG, ZIP, GZip, ELF)
// 1. Complete data is valid UTF-8
// 2. First 8KB has no null bytes
// 3. First 8KB control character ratio (excluding \n\r	) < 5%.
func IsLikelyText(data []byte) bool {
	if len(data) == 0 {
		return true
	}

	// Guard against binary formats that happen to pass UTF-8 validation
	// (e.g. minimal ASCII PDFs starting with %PDF).
	if hasBinarySignature(data) {
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

	buf := make([]byte, sniffSize)

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
