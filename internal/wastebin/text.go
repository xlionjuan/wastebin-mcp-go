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

// readSize is the number of bytes to read from a file for text detection.
// It is sniffSize + utf8.UTFMax: the extra bytes confirm whether the file
// extends beyond the sniff boundary, so trailing incomplete runes can be
// trimmed safely only when we know more data follows.
const readSize = sniffSize + utf8.UTFMax

// IsLikelyTextFile reads readSize bytes from path and calls IsLikelyText.
func IsLikelyTextFile(path string) (bool, error) {
	f, err := os.Open(path) //nolint:gosec // Path validated through validateFilePath pipeline
	if err != nil {
		return false, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // Close failure is non-critical; file was already read successfully

	buf := make([]byte, readSize)

	n, err := io.ReadFull(f, buf)
	if err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			// File ≤ readSize — entire content was read. The extra
			// bytes (if any) ensure no mid-rune truncation at the
			// sniff boundary, so no trimming is needed.
			return IsLikelyText(buf[:n]), nil
		}

		return false, fmt.Errorf("failed to read file: %w", err)
	}

	// File is larger than readSize. A multi-byte rune may straddle
	// the sniffSize boundary, so trim trailing incomplete UTF-8
	// from the first sniffSize bytes before checking.
	data := trimTrailingIncompleteUTF8(buf[:sniffSize])

	return IsLikelyText(data), nil
}

// trimTrailingIncompleteUTF8 removes up to utf8.UTFMax trailing bytes
// from the end of data when they form an incomplete UTF-8 sequence.
// It stops early when data is valid or the invalidity is not fixable
// by trailing trim (e.g., an ASCII byte at the end with invalid bytes
// elsewhere).
func trimTrailingIncompleteUTF8(data []byte) []byte {
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

	return data
}
