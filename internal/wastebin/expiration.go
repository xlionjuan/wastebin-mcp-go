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

	result := n * multiplier
	if multiplier != 0 && result/multiplier != n {
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

// DescribeDefaultExpires returns a human-readable description of a default
// expiration value in seconds for use in MCP tool descriptions.
func DescribeDefaultExpires(seconds int) string {
	if seconds == 0 {
		return "no expiration (paste persists until manually deleted)"
	}

	years := seconds / secondsPerYear
	r := seconds % secondsPerYear
	days := r / secondsPerDay
	r %= secondsPerDay
	hours := r / secondsPerHour
	r %= secondsPerHour
	minutes := r / secondsPerMinute
	secs := r % secondsPerMinute

	var parts []string
	if years == 1 {
		parts = append(parts, "1 year")
	} else if years > 1 {
		parts = append(parts, fmt.Sprintf("%d years", years))
	}

	if days == 1 {
		parts = append(parts, "1 day")
	} else if days > 1 {
		parts = append(parts, fmt.Sprintf("%d days", days))
	}

	if hours == 1 {
		parts = append(parts, "1 hour")
	} else if hours > 1 {
		parts = append(parts, fmt.Sprintf("%d hours", hours))
	}

	if minutes == 1 {
		parts = append(parts, "1 minute")
	} else if minutes > 1 {
		parts = append(parts, fmt.Sprintf("%d minutes", minutes))
	}

	if secs == 1 {
		parts = append(parts, "1 second")
	} else if secs > 1 {
		parts = append(parts, fmt.Sprintf("%d seconds", secs))
	}

	if len(parts) == 0 {
		return "0 seconds"
	}

	human := strings.Join(parts, ", ")

	return fmt.Sprintf("%d seconds (%s)", seconds, human)
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
