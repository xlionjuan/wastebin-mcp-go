package wastebin

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Sentinel errors for expiration parsing.
var (
	errNegativeExpiration    = errors.New("expiration cannot be negative")
	errUnknownExpirationUnit = errors.New("unknown expiration unit")
	errInvalidExpirationFmt  = errors.New("invalid expiration format")
)

// ParseExpiration parses an expiration string to seconds.
// Bare number -> seconds.
// Number + unit suffix -> translated.
// Units: s, m, h, d, w, M (30d), y (365d).
func ParseExpiration(s string, defaultSeconds int) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultSeconds, nil
	}

	// If it starts with a digit or minus sign, try parsing.
	if s[0] >= '0' && s[0] <= '9' || s[0] == '-' {
		return parseNumberWithUnit(s)
	}

	return 0, fmt.Errorf("%w: %q", errInvalidExpirationFmt, s)
}

// parseNumberWithUnit extracts the numeric value and optional unit suffix from
// an expiration string like "3600", "1h", "7d", and returns the value in seconds.
func parseNumberWithUnit(s string) (int, error) {
	// Find the boundary between the number and optional unit suffix.
	numEnd := 0

	for i, c := range s {
		if c >= '0' && c <= '9' || i == 0 && c == '-' {
			numEnd = i + 1
		} else {
			break
		}
	}

	numStr := s[:numEnd]
	unitStr := s[numEnd:]

	n, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("invalid expiration number: %w", err)
	}

	if n < 0 {
		return 0, errNegativeExpiration
	}

	if unitStr == "" {
		// Bare number.
		return n, nil
	}

	// Number with unit suffix.
	multiplier, ok := unitMultiplier(unitStr)
	if !ok {
		return 0, fmt.Errorf("%w: %q", errUnknownExpirationUnit, unitStr)
	}

	return n * multiplier, nil
}

// Time multipliers in seconds.
const (
	secondsPerMinute = 60
	secondsPerHour   = 3600
	secondsPerDay    = 86400
	secondsPerWeek   = 604800
	secondsPerMonth  = 2592000  // 30 days
	secondsPerYear   = 31536000 // 365 days
)

// unitMultiplier returns the multiplier in seconds for the given unit string.
func unitMultiplier(unit string) (int, bool) {
	switch unit {
	case "s":
		return 1, true
	case "m":
		return secondsPerMinute, true
	case "h":
		return secondsPerHour, true
	case "d":
		return secondsPerDay, true
	case "w":
		return secondsPerWeek, true
	case "M":
		return secondsPerMonth, true // 30 days
	case "y":
		return secondsPerYear, true // 365 days
	default:
		return 0, false
	}
}
