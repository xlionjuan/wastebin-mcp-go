package wastebin

import (
	"fmt"
	"strconv"
	"strings"
)

// Sentinel errors are defined in errors.go.

// ParseExpiration parses an expiration string to seconds.
// Bare number -> seconds.
// Number + unit suffix -> translated.
// Units: s, m, h, d, w, M (30d), y (365d).
func ParseExpiration(input string, defaultSeconds int) (int, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultSeconds, nil
	}

	// If it starts with a digit or minus sign, try parsing.
	if input[0] >= '0' && input[0] <= '9' || input[0] == '-' {
		return parseNumberWithUnit(input)
	}

	return 0, fmt.Errorf("%w: %q", errInvalidExpirationFmt, input)
}

// parseNumberWithUnit extracts the numeric value and optional unit suffix from
// an expiration string like "3600", "1h", "7d", and returns the value in seconds.
func parseNumberWithUnit(input string) (int, error) {
	// Find the boundary between the number and optional unit suffix.
	numEnd := 0

	for i, c := range input {
		if c >= '0' && c <= '9' || i == 0 && c == '-' {
			numEnd = i + 1
		} else {
			break
		}
	}

	numStr := input[:numEnd]
	unitStr := input[numEnd:]

	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("invalid expiration number; use a valid numeric expiration value: %w", err)
	}

	if num < 0 {
		return 0, errNegativeExpiration
	}

	if unitStr == "" {
		// Bare number.
		return num, nil
	}

	// Number with unit suffix.
	multiplier, ok := unitMultiplier(unitStr)
	if !ok {
		return 0, fmt.Errorf("%w: %q", errUnknownExpirationUnit, unitStr)
	}

	result := num * multiplier
	if multiplier != 0 && result/multiplier != num {
		return 0, errExpirationOverflow
	}

	return result, nil
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

// maxExpirationSeconds is the maximum supported expiration value in seconds
// based on Wastebin's API constraints. 0 means no expiration.
const maxExpirationSeconds = 315360000 // 10 years

// ValidateExpiration checks if the expiration value is within supported bounds.
func ValidateExpiration(seconds int) error {
	if seconds < 0 {
		return errNegativeExpiration
	}

	if seconds == 0 {
		return nil
	}

	if seconds > maxExpirationSeconds {
		return fmt.Errorf(
			"%w: %d seconds exceeds maximum of %d seconds",
			errExpirationTooLarge, seconds, maxExpirationSeconds,
		)
	}

	return nil
}
