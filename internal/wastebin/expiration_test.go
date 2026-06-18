package wastebin //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"strconv"
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

	tests := []struct {
		name    string
		input   string
		wantMsg string
	}{
		{"bare minus", "-", ""},
		{"bare number", "-1", "expiration cannot be negative"},
		{"with unit", "-1h", "expiration cannot be negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseExpiration(tt.input, 0)
			if err == nil {
				t.Fatalf("expected error for input %q", tt.input)
			}

			if tt.wantMsg != "" && err.Error() != tt.wantMsg {
				t.Errorf("got error %q, want %q", err.Error(), tt.wantMsg)
			}
		})
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
	n, err := ParseExpiration("999999999999999d", 3600)
	if err != nil {
		// Overflow detected — correct behavior.
		return
	}

	if n <= 0 {
		t.Errorf("expected overflow error or positive value for huge day value, got %d", n)
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

	if strconv.IntSize < 64 {
		t.Skip("test requires 64-bit int")
	}

	// Large year value — 999,999,999 × 31,536,000 ≈ 3.15×10¹⁶,
	// which fits within int64 range (≈9.22×10¹⁸). Verify no overflow.
	n, err := ParseExpiration("999999999y", 3600)
	if err != nil {
		t.Fatalf("unexpected error for large year value: %v", err)
	}

	if n <= 0 {
		t.Error("expected positive expiration for large year value")
	}
}

func FuzzParseExpiration(f *testing.F) {
	f.Add("")
	f.Add("0")
	f.Add("3600")
	f.Add("1h")
	f.Add("7d")
	f.Add("1M")
	f.Add("1y")
	f.Add("-1")
	f.Add("-")
	f.Add("abc")
	f.Add("10x")
	f.Add("  3600  ")

	f.Fuzz(func(t *testing.T, s string) {
		n, err := ParseExpiration(s, 3600)
		if err != nil {
			return
		}

		if n < 0 {
			t.Errorf("ParseExpiration(%q, 3600) = %d, want >= 0", s, n)
		}
	})
}
