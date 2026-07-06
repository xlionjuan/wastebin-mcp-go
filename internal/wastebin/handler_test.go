package wastebin //nolint:testpackage // white-box tests need access to unexported functions

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestNewHandler_RedirectFollowed(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect/" {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/REDIRECTED"})
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = ts.URL + "/redirect"

	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	resp, err := h.sendRequest(t.Context(), []byte(`{"text":"test"}`))
	if err != nil {
		t.Fatalf("sendRequest: %v", err)
	}

	if resp.Path != "/REDIRECTED" {
		t.Errorf("expected path /REDIRECTED, got %q", resp.Path)
	}
}

func TestNewHandler_RedirectDifferentHostBlocked(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect/" {
			http.Redirect(w, r, "http://evil.example.com/malicious", http.StatusFound)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = ts.URL + "/redirect"

	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	_, err = h.sendRequest(t.Context(), []byte(`{"text":"test"}`))
	if err == nil {
		t.Fatal("expected redirect error, got nil")
	}

	if !strings.Contains(err.Error(), "redirect to different host") {
		t.Errorf("expected redirect blocked error, got: %v", err)
	}
}

func TestNewHandler_RedirectSchemeDowngradeBlocked(t *testing.T) {
	t.Parallel()

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect/" {
			http.Redirect(w, r, "http://"+r.Host+"/", http.StatusFound)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = ts.URL + "/redirect"

	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	h.httpClient.Transport = ts.Client().Transport

	_, err = h.sendRequest(t.Context(), []byte(`{"text":"test"}`))
	if err == nil {
		t.Fatal("expected redirect scheme downgrade error, got nil")
	}

	if !strings.Contains(err.Error(), "redirect scheme downgrade") {
		t.Errorf("expected scheme downgrade error, got: %v", err)
	}
}

func TestSendRequest_Success(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		if r.URL.Path != "/" {
			t.Errorf("expected /, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/ABC123"})
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = ts.URL

	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	resp, err := h.sendRequest(t.Context(), []byte(`{"text":"hello"}`))
	if err != nil {
		t.Fatalf("sendRequest: %v", err)
	}

	if resp.Path != "/ABC123" {
		t.Errorf("expected path /ABC123, got %q", resp.Path)
	}
}

func TestSendRequest_NonOKResponse(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = ts.URL

	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	_, err = h.sendRequest(t.Context(), []byte(`{"text":"test"}`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "unknown HTTP error: HTTP 500") {
		t.Errorf("expected unknown HTTP error, got: %v", err)
	}
}

func TestSendRequest_JSONDecodeError(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = ts.URL

	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	_, err = h.sendRequest(t.Context(), []byte(`{"text":"test"}`))
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}

	if !strings.Contains(err.Error(), "failed to parse Wastebin response") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

func TestSendRequest_EmptyPathResponse(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": ""})
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = ts.URL

	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	_, err = h.sendRequest(t.Context(), []byte(`{"text":"test"}`))
	if err == nil {
		t.Fatal("expected error for empty path response")
	}

	if !errors.Is(err, errInvalidWastebinResponse) {
		t.Errorf("expected errInvalidWastebinResponse, got: %v", err)
	}
}

func TestBuildRequest_WithTitle(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.ServerURL = "https://bin.example.com"

	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	title := "My Paste"
	args := &CreatePasteArgs{Title: &title}

	body, err := h.buildRequest(args, "content", "go", 3600)
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}

	var parsed map[string]any
	_ = json.Unmarshal(body, &parsed)

	if parsed["title"] != "My Paste" {
		t.Errorf("expected title 'My Paste', got %v", parsed["title"])
	}

	if parsed["text"] != "content" {
		t.Errorf("expected text 'content', got %v", parsed["text"])
	}
}

func TestBuildRequest_WithBurnAfterReading(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.ServerURL = "https://bin.example.com"

	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	burn := true
	args := &CreatePasteArgs{BurnAfterReading: &burn}

	body, err := h.buildRequest(args, "content", "go", 0)
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}

	var parsed map[string]any
	_ = json.Unmarshal(body, &parsed)

	if parsed["burn_after_reading"] != true {
		t.Errorf("expected burn_after_reading=true, got %v", parsed["burn_after_reading"])
	}
}

func TestBuildRequest_PasswordLoopback(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		// Not reached — testing buildRequest only.
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = ts.URL

	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	password := "hunter2"
	args := &CreatePasteArgs{Password: &password}

	body, err := h.buildRequest(args, "secret", "go", 0)
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}

	var parsed map[string]any
	_ = json.Unmarshal(body, &parsed)

	if parsed["password"] != "hunter2" {
		t.Errorf("expected password 'hunter2', got %v", parsed["password"])
	}
}

func TestBuildRequest_PasswordNonLoopbackRejected(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.ServerURL = "http://wastebin.example.com:8080"

	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	password := "hunter2"
	args := &CreatePasteArgs{Password: &password}

	_, err = h.buildRequest(args, "secret", "go", 0)
	if err == nil {
		t.Fatal("expected error for password over non-loopback HTTP")
	}

	if !errors.Is(err, errPasswordOverHTTP) {
		t.Errorf("expected errPasswordOverHTTP, got: %v", err)
	}
}

func TestReadFileContent_NonRegularNoAllowedPaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	cfg := DefaultConfig()
	cfg.ServerURL = "http://localhost:12345"

	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	_, _, err = h.readFileContent(tmpDir, nil, nil)
	if err == nil {
		t.Fatal("expected error for non-regular path, got nil")
	}

	if !errors.Is(err, errFilePathCannotBeUsed) {
		t.Errorf("expected errFilePathCannotBeUsed, got: %v", err)
	}
}

func TestReadFileContent_InvalidExtensionRejected(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	filePath := filepath.Join(tmpDir, "test.txt")

	err := os.WriteFile(filePath, []byte("hello"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.ServerURL = "http://localhost:12345"
	cfg.AllowedPaths = []string{tmpDir}

	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	badExt := "a/b"
	translate := false

	_, _, err = h.readFileContent(filePath, &translate, &badExt)
	if err == nil {
		t.Fatal("expected error for invalid extension, got nil")
	}

	if !errors.Is(err, errInvalidExtension) {
		t.Errorf("expected errInvalidExtension, got: %v", err)
	}
}
