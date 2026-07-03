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

	// File is larger than readSize. Check whether trailing bytes at
	// the sniffSize boundary are an incomplete UTF-8 sequence that can
	// be completed by the extra bytes — only trim when confirmed.
	data := trimTrailingIncompleteUTF8(buf[:sniffSize], buf[sniffSize:readSize])

	return IsLikelyText(data), nil
}

// trimTrailingIncompleteUTF8 removes trailing bytes from data when they
// form an incomplete UTF-8 sequence truncated at a boundary.
//
// When extra is non-empty, trailing bytes are only trimmed if extra can
// complete them into valid UTF-8 (confirming boundary truncation). Without
// this check, a genuinely invalid byte at the boundary — e.g. a lone
// continuation byte followed by ASCII in the actual file — would be
// incorrectly trimmed away.
//
// When extra is empty, trailing non-ASCII bytes are trimmed optimistically
// (backward-compatible behavior for callers without boundary context).
func trimTrailingIncompleteUTF8(data, extra []byte) []byte {
	if utf8.Valid(data) {
		return data
	}

	// Walk backward to find the start of the trailing non-ASCII sequence.
	split := len(data)
	for split > 0 && data[split-1] >= 0x80 {
		split--
	}

	if split == len(data) {
		// No trailing non-ASCII bytes; invalidity is elsewhere.
		return data
	}

	trail := data[split:]

	// If trail alone is valid UTF-8, it's a complete rune at the
	// boundary — don't trim. The invalidity is earlier in data.
	if utf8.Valid(trail) {
		return data
	}

	if len(extra) == 0 {
		// No extra bytes to verify — optimistic trim.
		return data[:split]
	}

	// Combine trail with extra bytes. If any prefix is valid UTF-8,
	// the trail was boundary-truncated → safe to trim.
	combined := make([]byte, len(trail)+len(extra))
	copy(combined, trail)
	copy(combined[len(trail):], extra)

	for k := len(trail) + 1; k <= len(combined); k++ {
		if utf8.Valid(combined[:k]) {
			return data[:split]
		}
	}

	// Trail cannot be completed by extra — genuinely invalid → keep.
	return data
}
