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
// Checks:
// 1. Complete data is valid UTF-8
// 2. First 8KB has no null bytes
// 3. First 8KB control character ratio (excluding \n\r\t) < 5%.
func IsLikelyText(data []byte) bool {
	if len(data) == 0 {
		return true
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

	data := buf[:n]

	// Trim trailing incomplete UTF-8 bytes. When a file is larger than
	// sniffSize, a multi-byte rune may straddle the boundary, leaving
	// the buffer with a truncated trailing sequence that would cause
	// utf8.Valid to fail. Walk backward from the end, dropping
	// continuation bytes and incomplete start bytes until the buffer
	// is valid (or we hit the trim limit).
	for i := 0; i < utf8.UTFMax && len(data) > 0; i++ {
		if utf8.Valid(data) {
			break
		}

		lastByte := data[len(data)-1]

		// ASCII byte at end: the invalidity is elsewhere and
		// cannot be fixed by trailing trim.
		if lastByte < 0x80 { //nolint:mnd // ASCII upper bound
			break
		}

		// Continuation bytes (10xxxxxx) are always trailing fragments.
		if lastByte&0xC0 == 0x80 { //nolint:mnd // UTF-8 continuation byte bitmask
			data = data[:len(data)-1]

			continue
		}

		// Start byte (11xxxxxx). DecodeLastRune distinguishes complete
		// multi-byte runes from truncated ones.
		r, _ := utf8.DecodeLastRune(data)
		if r == utf8.RuneError {
			data = data[:len(data)-1]
		}
	}

	return IsLikelyText(data), nil
}
