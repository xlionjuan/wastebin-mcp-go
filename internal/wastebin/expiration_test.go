package wastebin //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"errors"
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

	_, err := ParseExpiration("999999999999999d", 3600)
	if err == nil {
		t.Fatal("expected overflow error for huge day value")
	}

	if !errors.Is(err, errExpirationOverflow) {
		t.Errorf("expected errExpirationOverflow, got %v", err)
	}
}

func TestParseExpiration_HugeSeconds(t *testing.T) {
	t.Parallel()

	expires, err := ParseExpiration("999999999", 3600)
	if err != nil {
		t.Fatalf("unexpected error for valid seconds: %v", err)
	}

	if expires != 999999999 {
		t.Errorf("expected %d, got %d", 999999999, expires)
	}
}

func TestParseExpiration_OverflowYears(t *testing.T) {
	t.Parallel()

	if strconv.IntSize < 64 {
		t.Skip("test requires 64-bit int")
	}

	n, err := ParseExpiration("999999999y", 3600)
	if err != nil {
		t.Fatalf("unexpected error for large year value: %v", err)
	}

	want := int64(999999999) * int64(31536000) // 31,535,999,968,464,000
	if int64(n) != want {
		t.Errorf("got %d, want %d", n, want)
	}
}

func TestParseExpiration_NearOverflowDays(t *testing.T) {
	t.Parallel()

	if strconv.IntSize < 64 {
		t.Skip("test requires 64-bit int")
	}

	// 99,999,999,999,999d — just under the overflow boundary.
	// 99,999,999,999,999 × 86,400 = 8,639,999,999,999,913,600 which
	// fits within int64 max (9,223,372,036,854,775,807).
	n, err := ParseExpiration("99999999999999d", 3600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := int64(99999999999999) * int64(86400) // 8,639,999,999,999,913,600
	if int64(n) != want {
		t.Errorf("got %d, want %d", n, want)
	}
}

func TestValidateExpiration_Zero(t *testing.T) {
	t.Parallel()

	err := ValidateExpiration(0)
	if err != nil {
		t.Errorf("expected nil for 0, got %v", err)
	}
}

func TestValidateExpiration_MaxValue(t *testing.T) {
	t.Parallel()

	err := ValidateExpiration(maxExpirationSeconds)
	if err != nil {
		t.Errorf("expected nil for max value %d, got %v", maxExpirationSeconds, err)
	}
}

func TestValidateExpiration_AboveMax(t *testing.T) {
	t.Parallel()

	err := ValidateExpiration(maxExpirationSeconds + 1)
	if err == nil {
		t.Fatal("expected error for value above max, got nil")
	}

	if !errors.Is(err, errExpirationTooLarge) {
		t.Errorf("expected errExpirationTooLarge, got %v", err)
	}
}

func TestValidateExpiration_Negative(t *testing.T) {
	t.Parallel()

	err := ValidateExpiration(-1)
	if err == nil {
		t.Fatal("expected error for negative value, got nil")
	}

	if !errors.Is(err, errNegativeExpiration) {
		t.Errorf("expected errNegativeExpiration, got %v", err)
	}
}

func TestValidateExpiration_WellBelowMax(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		val  int
	}{
		{"one second", 1},
		{"one hour", 3600},
		{"one day", 86400},
		{"one week", 604800},
		{"one month", 2592000},
		{"one year", 31536000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateExpiration(tt.val)
			if err != nil {
				t.Errorf("unexpected error for %d: %v", tt.val, err)
			}
		})
	}
}

func TestDescribeDefaultExpires_Zero(t *testing.T) {
	t.Parallel()

	got := DescribeDefaultExpires(0)
	want := "no expiration (paste persists until manually deleted)"

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDescribeDefaultExpires_SingleUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		seconds int
		want    string
	}{
		{1, "1 seconds (1 second)"},
		{59, "59 seconds (59 seconds)"},
		{60, "60 seconds (1 minute)"},
		{3540, "3540 seconds (59 minutes)"},
		{3600, "3600 seconds (1 hour)"},
		{82800, "82800 seconds (23 hours)"},
		{86400, "86400 seconds (1 day)"},
		{172800, "172800 seconds (2 days)"},
		{2592000, "2592000 seconds (30 days)"},
		{5184000, "5184000 seconds (60 days)"},
		{31536000, "31536000 seconds (1 year)"},
	}

	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.seconds), func(t *testing.T) {
			t.Parallel()

			got := DescribeDefaultExpires(tt.seconds)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDescribeDefaultExpires_Mixed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		seconds int
		want    string
	}{
		{3661, "3661 seconds (1 hour, 1 minute, 1 second)"},
		{90061, "90061 seconds (1 day, 1 hour, 1 minute, 1 second)"},
		{31626061, "31626061 seconds (1 year, 1 day, 1 hour, 1 minute, 1 second)"},
		{12345, "12345 seconds (3 hours, 25 minutes, 45 seconds)"},
		{100000, "100000 seconds (1 day, 3 hours, 46 minutes, 40 seconds)"},
	}

	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.seconds), func(t *testing.T) {
			t.Parallel()

			got := DescribeDefaultExpires(tt.seconds)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDescribeDefaultExpires_Plural(t *testing.T) {
	t.Parallel()

	tests := []struct {
		seconds int
		want    string
	}{
		{63072000, "63072000 seconds (2 years)"},
		{259200, "259200 seconds (3 days)"},
		{14400, "14400 seconds (4 hours)"},
		{600, "600 seconds (10 minutes)"},
		{120, "120 seconds (2 minutes)"},
	}

	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.seconds), func(t *testing.T) {
			t.Parallel()

			got := DescribeDefaultExpires(tt.seconds)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
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
