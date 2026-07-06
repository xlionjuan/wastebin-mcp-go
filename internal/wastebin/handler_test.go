package wastebin //nolint:testpackage // white-box tests need access to unexported functions

import (
	"strings"
	"testing"
)

func TestCheckContentSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		maxSize int64
		wantErr string
	}{
		{
			name:    "empty content within limit",
			content: "",
			maxSize: 100,
			wantErr: "",
		},
		{
			name:    "content exactly at limit",
			content: strings.Repeat("a", 100),
			maxSize: 100,
			wantErr: "",
		},
		{
			name:    "content within limit",
			content: "hello",
			maxSize: 100,
			wantErr: "",
		},
		{
			name:    "content exceeds limit",
			content: "this is too long",
			maxSize: 5,
			wantErr: "content exceeds the maximum allowed size",
		},
		{
			name:    "zero max size rejects all",
			content: "a",
			maxSize: 0,
			wantErr: "content exceeds the maximum allowed size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := checkContentSize(tt.content, tt.maxSize)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
			}
		})
	}
}

func TestParseExpires(t *testing.T) {
	t.Parallel()

	validExp := func(s string, defaultExp int) int {
		t.Helper()

		n, err := parseExpires(&s, defaultExp)
		if err != nil {
			t.Fatalf("parseExpires(%q, %d): unexpected error: %v", s, defaultExp, err)
		}

		return n
	}

	t.Run("nil falls back to default", func(t *testing.T) {
		t.Parallel()

		n, err := parseExpires(nil, 3600)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if n != 3600 {
			t.Errorf("expected 3600, got %d", n)
		}
	})

	t.Run("empty string falls back to default", func(t *testing.T) {
		t.Parallel()

		n, err := parseExpires(new(string), 7200)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if n != 7200 {
			t.Errorf("expected 7200, got %d", n)
		}
	})

	t.Run("bare number treated as seconds", func(t *testing.T) {
		t.Parallel()

		if n := validExp("3600", 0); n != 3600 {
			t.Errorf("expected 3600, got %d", n)
		}
	})

	t.Run("number with unit", func(t *testing.T) {
		t.Parallel()

		if n := validExp("1h", 0); n != 3600 {
			t.Errorf("expected 3600, got %d", n)
		}
	})

	t.Run("days unit", func(t *testing.T) {
		t.Parallel()

		if n := validExp("7d", 0); n != 604800 {
			t.Errorf("expected 604800, got %d", n)
		}
	})

	t.Run("weeks unit", func(t *testing.T) {
		t.Parallel()

		if n := validExp("2w", 0); n != 1209600 {
			t.Errorf("expected 1209600, got %d", n)
		}
	})

	t.Run("zero is valid", func(t *testing.T) {
		t.Parallel()

		n, err := parseExpires(nil, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if n != 0 {
			t.Errorf("expected 0, got %d", n)
		}
	})

	t.Run("maximum allowed value", func(t *testing.T) {
		t.Parallel()

		if n := validExp("10y", 0); n != maxExpirationSeconds {
			t.Errorf("expected %d, got %d", maxExpirationSeconds, n)
		}
	})

	t.Run("negative number rejected", func(t *testing.T) {
		t.Parallel()

		v := "-1"

		_, err := parseExpires(&v, 0)
		if err == nil {
			t.Fatal("expected error for negative expiration, got nil")
		}

		if !strings.Contains(err.Error(), "invalid expiration") {
			t.Errorf("expected 'invalid expiration' in error, got %q", err.Error())
		}
	})

	t.Run("unknown unit rejected", func(t *testing.T) {
		t.Parallel()

		v := "5x"

		_, err := parseExpires(&v, 0)
		if err == nil {
			t.Fatal("expected error for unknown unit, got nil")
		}

		if !strings.Contains(err.Error(), "invalid expiration") {
			t.Errorf("expected 'invalid expiration' in error, got %q", err.Error())
		}
	})

	t.Run("value too large rejected", func(t *testing.T) {
		t.Parallel()

		v := "200y"

		_, err := parseExpires(&v, 0)
		if err == nil {
			t.Fatal("expected error for too-large expiration, got nil")
		}

		if !strings.Contains(err.Error(), "invalid expiration") {
			t.Errorf("expected 'invalid expiration' in error, got %q", err.Error())
		}

		if !strings.Contains(err.Error(), "exceeds maximum") {
			t.Errorf("expected 'exceeds maximum' in error, got %q", err.Error())
		}
	})
}

func TestParseExpires_WithDefault(t *testing.T) {
	t.Parallel()

	// Custom default should be validated too.
	tooLarge := 999999999

	_, err := parseExpires(nil, tooLarge)
	if err == nil {
		t.Fatal("expected error for too-large default, got nil")
	}

	if !strings.Contains(err.Error(), "invalid expiration") {
		t.Errorf("expected 'invalid expiration' in error, got %q", err.Error())
	}
}

func TestSendRequest_DNSError(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.ServerURL = "http://nonexistent.example.invalid"

	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	_, err = h.sendRequest(t.Context(), []byte(`{"text":"test"}`))
	if err == nil {
		t.Fatal("expected DNS error, got nil")
	}

	if !strings.Contains(err.Error(), "cannot resolve the server hostname") {
		t.Errorf("expected DNS resolution error, got: %v", err)
	}
}

func TestSendRequest_ConnectionRefused(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.ServerURL = "http://127.0.0.1:1"

	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	_, err = h.sendRequest(t.Context(), []byte(`{"text":"test"}`))
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}

	if !strings.Contains(err.Error(), "cannot connect to Wastebin server") {
		t.Errorf("expected connection error, got: %v", err)
	}
}

func TestNewHandler_Valid(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.ServerURL = "http://localhost:8080"

	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	if h.postURL != "http://localhost:8080/" {
		t.Errorf("expected postURL 'http://localhost:8080/', got %q", h.postURL)
	}
}

func TestNewHandler_Invalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *Config
	}{
		{name: "nil config", cfg: nil},
		{name: "empty server URL", cfg: &Config{ServerURL: ""}},
		{name: "invalid URL", cfg: &Config{ServerURL: "://invalid"}},
		{name: "unsupported scheme", cfg: &Config{ServerURL: "ftp://localhost"}},
		{name: "missing host", cfg: &Config{ServerURL: "http://"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewHandler(tt.cfg)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}
