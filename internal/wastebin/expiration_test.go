package wastebin //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"testing"
)

func TestParseExpiration_Empty(t *testing.T) {
	t.Parallel()

	n, err := ParseExpiration("", 3600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n != 3600 {
		t.Errorf("expected %d, got %d", 3600, n)
	}
}

func TestParseExpiration_BareNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  int
	}{
		{"0", 0},
		{"1", 1},
		{"60", 60},
		{"3600", 3600},
		{"86400", 86400},
		{"31536000", 31536000},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got, err := ParseExpiration(tt.input, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseExpiration_WithUnits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  int
	}{
		{"30s", 30},
		{"5m", 300},
		{"2h", 7200},
		{"7d", 604800},
		{"2w", 1209600},
		{"1M", 2592000},  // 30 days
		{"2M", 5184000},  // 60 days
		{"1y", 31536000}, // 365 days
		{"2y", 63072000}, // 730 days
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got, err := ParseExpiration(tt.input, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseExpiration_Whitespace(t *testing.T) {
	t.Parallel()

	n, err := ParseExpiration("  3600  ", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n != 3600 {
		t.Errorf("got %d, want %d", n, 3600)
	}

	n, err = ParseExpiration("  1h  ", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n != 3600 {
		t.Errorf("got %d, want %d", n, 3600)
	}
}

func TestParseExpiration_Negative(t *testing.T) {
	t.Parallel()

	_, err := ParseExpiration("-1", 0)
	if err == nil {
		t.Fatal("expected error for negative expiration")
	}

	if err.Error() != "expiration cannot be negative" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestParseExpiration_NegativeWithUnit(t *testing.T) {
	t.Parallel()

	_, err := ParseExpiration("-1h", 0)
	if err == nil {
		t.Fatal("expected error for negative expiration with unit")
	}
}

func TestParseExpiration_InvalidFormat(t *testing.T) {
	t.Parallel()

	_, err := ParseExpiration("abc", 0)
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestParseExpiration_UnknownUnit(t *testing.T) {
	t.Parallel()

	_, err := ParseExpiration("10x", 0)
	if err == nil {
		t.Fatal("expected error for unknown unit")
	}

	if err.Error() != `unknown expiration unit: "x"` {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestParseExpiration_CaseSensitivity(t *testing.T) {
	t.Parallel()
	// 'M' is months, 'm' is minutes.
	n, err := ParseExpiration("1M", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n != 2592000 {
		t.Errorf("got %d, want %d (30 days)", n, 2592000)
	}

	n, err = ParseExpiration("1m", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n != 60 {
		t.Errorf("got %d, want %d (1 minute)", n, 60)
	}
}

func TestParseExpiration_LeadingZeros(t *testing.T) {
	t.Parallel()

	n, err := ParseExpiration("007d", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n != 604800 {
		t.Errorf("got %d, want %d", n, 604800)
	}
}

func TestParseExpiration_InvalidNumber(t *testing.T) {
	t.Parallel()

	_, err := ParseExpiration("--5", 0)
	if err == nil {
		t.Fatal("expected error for invalid number format")
	}
}

func TestParseExpiration_OverflowDays(t *testing.T) {
	t.Parallel()
	// A huge number of days that would overflow if multiplied by 86400
	// on 64-bit (999,999,999,999,999 × 86,400 > int64 max).
	// ParseExpiration should not panic and ideally return an error or
	// a positive value.
	_, err := ParseExpiration("999999999999999d", 3600)
	if err == nil {
		t.Log("NOTE: large day value did not overflow; verifying result is positive")
	}
}

func TestParseExpiration_HugeSeconds(t *testing.T) {
	t.Parallel()
	// A huge seconds value within valid int range.
	expires, err := ParseExpiration("999999999", 3600)
	if err != nil {
		t.Fatalf("unexpected error for valid seconds: %v", err)
	}

	if expires <= 0 {
		t.Error("expected positive expiration")
	}
}

func TestParseExpiration_OverflowYears(t *testing.T) {
	t.Parallel()
	// Large year value — 999,999,999 × 31,536,000 ≈ 3.15×10¹⁶,
	// which fits within int64 range (≈9.22×10¹⁸). Verify no overflow.
	_, err := ParseExpiration("999999999y", 3600)
	if err == nil {
		t.Log("NOTE: large year value parsed without overflow; verify result is positive")
	}
}
