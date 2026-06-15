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
)

// IsLikelyText checks if content looks like text.
// Reads first 8KB of data and checks:
// 1. Valid UTF-8
// 2. No null bytes
// 3. Control character ratio (excluding \n\r\t) < 5%.
func IsLikelyText(data []byte) bool {
	if len(data) == 0 {
		return true
	}

	size := min(len(data), sniffSize)

	buf := data[:size]

	if !utf8.Valid(buf) {
		return false
	}

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
