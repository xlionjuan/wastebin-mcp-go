package wastebin //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ──────────────────────────────────────────────
// Test helpers
// ──────────────────────────────────────────────

// newTestServer creates an httptest.Server that responds with JSON {"path": path}.
//
//nolint:unused // Intentionally provided for future test extraction
func newTestServer(t *testing.T, path string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": path}) //nolint:errcheck // Test helper OK
	}))
}

// newTestServerWithHandler creates an httptest.Server with a custom handler.
// The handler is wrapped so that Content-Type is set to application/json.
//
//nolint:unused // Intentionally provided for future test extraction
func newTestServerWithHandler(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}))
}

// createTestClient creates a WastebinClient pointing at the given server URL.
//
//nolint:unused // Intentionally provided for future test extraction
func createTestClient(t *testing.T, serverURL string) *WastebinClient {
	t.Helper()

	cfg := DefaultConfig()
	cfg.ServerURL = serverURL

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	return client
}

func TestCreatePaste_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		if r.URL.Path != "/" {
			t.Errorf("expected /, got %s", r.URL.Path)
		}

		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		var req map[string]any

		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req["text"] != "hello world" {
			t.Errorf("expected text 'hello world', got %v", req["text"])
		}

		if req["extension"] != "go" {
			t.Errorf("expected extension 'go', got %v", req["extension"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/ABC123.go"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "hello world"
	ext := "go"

	resp, err := client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content:   &content,
		Extension: &ext,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "ABC123" {
		t.Errorf("expected ID 'ABC123', got %q", resp.ID)
	}

	if resp.Hostname != server.URL {
		t.Errorf("expected hostname %q, got %q", server.URL, resp.Hostname)
	}

	if resp.URL != "/ABC123.go" {
		t.Errorf("expected URL '/ABC123.go', got %q", resp.URL)
	}

	if resp.Raw != "/raw/ABC123.go" {
		t.Errorf("expected Raw '/raw/ABC123.go', got %q", resp.Raw)
	}

	if resp.MarkdownRendered != "" {
		t.Errorf("expected empty MarkdownRendered, got %q", resp.MarkdownRendered)
	}

	if resp.Hint != "" {
		t.Errorf("expected empty Hint, got %q", resp.Hint)
	}
}

func TestCreatePaste_ExpirationParsed(t *testing.T) {
	t.Parallel()

	var capturedExt, capturedExpires any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any

		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		capturedExt = req["extension"]
		capturedExpires = req["expires"]

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/EXP123"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "test"
	expires := "1h"
	ext := "go"

	resp, err := client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content:   &content,
		Expires:   &expires,
		Extension: &ext,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "EXP123" {
		t.Errorf("expected ID EXP123, got %q", resp.ID)
	}

	// Verify expiration was parsed: "1h" = 3600 seconds
	if capturedExpires != float64(3600) {
		t.Errorf("expected expires=3600, got %v", capturedExpires)
	}

	if capturedExt != "go" {
		t.Errorf("expected extension 'go', got %v", capturedExt)
	}
}

func TestCreatePaste_403Response(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden")) //nolint:errcheck // Test helper write error is acceptable
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "test"

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content: &content,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "server rejected the request") {
		t.Errorf("expected 403 error message, got: %v", err)
	}
}

func TestCreatePaste_413Response(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_, _ = w.Write([]byte("too large")) //nolint:errcheck // Test helper write error is acceptable
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "test"

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content: &content,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "content exceeds the server's maximum allowed size") {
		t.Errorf("expected 413 error message, got: %v", err)
	}
}

func TestCreatePaste_UnknownHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error")) //nolint:errcheck // Test helper write error is acceptable
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "test"

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content: &content,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "unknown HTTP error: HTTP 500") {
		t.Errorf("expected unknown HTTP error message, got: %v", err)
	}

	if strings.Contains(err.Error(), "internal error") {
		t.Errorf("expected body NOT to be leaked in error message, got: %v", err)
	}
}

func TestCreatePaste_ContentTooLarge(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.ServerURL = "http://localhost:12345" // Not actually used since pre-check fails
	cfg.MaxContentSize = 10

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "this content is way longer than ten bytes"

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content: &content,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "content exceeds the maximum allowed size") {
		t.Errorf("expected content too large error, got: %v", err)
	}
}

func TestCreatePaste_MutualExclusivity(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.ServerURL = "http://localhost:12345"

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "test content"
	filePath := "/tmp/test.txt"

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content:  &content,
		FilePath: &filePath,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, errBothContentAndFilePath) {
		t.Errorf("expected errBothContentAndFilePath, got: %v", err)
	}
}

func TestCreatePaste_NeitherContentNorFile(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.ServerURL = "http://localhost:12345"

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, errNeitherContentNorFilePath) {
		t.Errorf("expected errNeitherContentNorFilePath, got: %v", err)
	}
}

func TestCreatePaste_FileMode(t *testing.T) {
	t.Parallel()
	// Set up a temp directory as allowed path.
	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	// Create a text file in the allowed directory.
	fileContent := "hello from file"

	filePath := filepath.Join(allowedDir, "testfile.txt")

	err = os.WriteFile(filePath, []byte(fileContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any

		err = json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req["text"] != fileContent {
			t.Errorf("expected text %q, got %v", fileContent, req["text"])
		}
		// Extension derived from file name (.txt).
		if req["extension"] != "txt" {
			t.Errorf("expected extension 'txt', got %v", req["extension"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/XYZ789.txt"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL
	cfg.AllowedPaths = []string{allowedDir}

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	resp, err := client.CreatePaste(context.Background(), &CreatePasteArgs{
		FilePath: &filePath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "XYZ789" {
		t.Errorf("expected ID 'XYZ789', got %q", resp.ID)
	}

	if resp.URL != "/XYZ789.txt" {
		t.Errorf("expected URL '/XYZ789.txt', got %q", resp.URL)
	}

	if resp.Raw != "/raw/XYZ789.txt" {
		t.Errorf("expected Raw '/raw/XYZ789.txt', got %q", resp.Raw)
	}
}

func TestCreatePaste_FileMode_ExtensionOverride(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(allowedDir, "script.py")

	err = os.WriteFile(filePath, []byte("print('hello')"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any

		err = json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		// Even though the file has .py extension, args.Extension overrides to 'go'.
		if req["extension"] != "go" {
			t.Errorf("expected extension 'go', got %v", req["extension"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/ABC.go"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL
	cfg.AllowedPaths = []string{allowedDir}

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ext := "go"

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		FilePath:  &filePath,
		Extension: &ext,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreatePaste_MarkdownRendered(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/MARKDOWN.md"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "# Hello"
	ext := "md"

	resp, err := client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content:   &content,
		Extension: &ext,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.MarkdownRendered != "/md/MARKDOWN.md" {
		t.Errorf("expected MarkdownRendered '/md/MARKDOWN.md', got %q", resp.MarkdownRendered)
	}

	if resp.Hint != "" {
		t.Errorf("expected empty Hint, got %q", resp.Hint)
	}
}

func TestCreatePaste_MarkdownRendered_WithMarkdownExt(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/POST.markdown"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "# Post"
	ext := "markdown"

	resp, err := client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content:   &content,
		Extension: &ext,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.MarkdownRendered != "/md/POST.markdown" {
		t.Errorf("expected MarkdownRendered '/md/POST.markdown', got %q", resp.MarkdownRendered)
	}
}

func TestCreatePaste_Hint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/NOPATH"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "no extension paste"

	resp, err := client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content: &content,
		// No Extension set — ext will be "".
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Hint == "" {
		t.Error("expected Hint to be non-empty for pastes without extension")
	}

	if !strings.Contains(resp.Hint, "Extension not detected") {
		t.Errorf("expected hint about extension, got %q", resp.Hint)
	}

	if resp.MarkdownRendered != "" {
		t.Errorf("expected empty MarkdownRendered, got %q", resp.MarkdownRendered)
	}
}

func TestCreatePaste_FileMode_BinaryFileRejected(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	// Write a binary file.
	filePath := filepath.Join(allowedDir, "binary.bin")

	err = os.WriteFile(filePath, []byte{0x00, 0xFF, 0xFE}, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Server should not be reached.
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("server should not be called for binary file")
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL
	cfg.AllowedPaths = []string{allowedDir}

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		FilePath: &filePath,
	})
	if err == nil {
		t.Fatal("expected error for binary file, got nil")
	}

	if !errors.Is(err, errFileNotText) {
		t.Errorf("expected errFileNotText, got: %v", err)
	}
}

func TestCreatePaste_FileMode_StatError(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.ServerURL = "http://localhost:12345"
	cfg.AllowedPaths = []string{allowedDir}

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// File doesn't exist yet — should get errFilePathCannotBeUsed.
	nonExistentPath := filepath.Join(allowedDir, "nonexistent.txt")

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		FilePath: &nonExistentPath,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}

	if !errors.Is(err, errFilePathCannotBeUsed) {
		t.Errorf("expected errFilePathCannotBeUsed, got: %v", err)
	}
}

func TestCreatePaste_FileMode_FileTooLarge(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	// Create a file larger than MaxContentSize.
	filePath := filepath.Join(allowedDir, "large.txt")
	data := make([]byte, 100)

	err = os.WriteFile(filePath, data, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.ServerURL = "http://localhost:12345"
	cfg.AllowedPaths = []string{allowedDir}
	cfg.MaxContentSize = 50 // Smaller than our 100-byte file.

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		FilePath: &filePath,
	})
	if err == nil {
		t.Fatal("expected error for oversized file")
	}

	if !strings.Contains(err.Error(), "content exceeds the maximum allowed size") {
		t.Errorf("expected content too large error, got: %v", err)
	}
}

func TestCreatePaste_FileMode_ReadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	// Use a directory path — os.Stat succeeds (it's a valid path),
	// but os.ReadFile fails deterministically (can't read a directory's
	// content as a file), regardless of whether running as root or not.
	dirPath := allowedDir

	cfg := DefaultConfig()
	cfg.ServerURL = "http://localhost:12345"
	cfg.AllowedPaths = []string{allowedDir}

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		FilePath: &dirPath,
	})
	if err == nil {
		t.Fatal("expected error for unreadable file")
	}

	if !errors.Is(err, errFilePathCannotBeUsed) {
		t.Errorf("expected errFilePathCannotBeUsed, got: %v", err)
	}
}

func TestCreatePaste_FileMode_PathNotAllowed(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	otherDir := filepath.Join(tmpDir, "other")

	err = os.Mkdir(otherDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	// Write a file in the non-allowed directory.
	filePath := filepath.Join(otherDir, "test.txt")

	err = os.WriteFile(filePath, []byte("hello"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.ServerURL = "http://localhost:12345"
	cfg.AllowedPaths = []string{allowedDir}

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		FilePath: &filePath,
	})
	if err == nil {
		t.Fatal("expected error for disallowed path, got nil")
	}

	if !errors.Is(err, errPathNotAllowed) {
		t.Errorf("expected errPathNotAllowed, got: %v", err)
	}
}

func TestCreatePaste_FileMode_SandboxTranslation(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	// Create the actual file on the host side (mapped path).
	// The sandbox path /sandbox/foo will be translated to allowedDir/foo.
	subDir := filepath.Join(allowedDir, "sub")

	err = os.Mkdir(subDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	hostFilePath := filepath.Join(subDir, "host_file.txt")

	err = os.WriteFile(hostFilePath, []byte("translated content"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any

		err = json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req["text"] != "translated content" {
			t.Errorf("expected text 'translated content', got %v", req["text"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/TRANSLATED.txt"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL
	cfg.AllowedPaths = []string{allowedDir}
	cfg.SandboxMounts = []SandboxMount{
		{HostPath: allowedDir, SandboxPath: "/workspace"},
	}

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	sandboxPath := "/workspace/sub/host_file.txt"
	translate := true

	resp, err := client.CreatePaste(context.Background(), &CreatePasteArgs{
		FilePath:             &sandboxPath,
		TranslateSandboxPath: &translate,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "TRANSLATED" {
		t.Errorf("expected ID 'TRANSLATED', got %q", resp.ID)
	}
}

func TestCreatePaste_FileMode_SandboxTranslationWithoutFlag(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}
	// Create file directly in allowed dir (no translation needed).
	filePath := filepath.Join(allowedDir, "direct.txt")

	err = os.WriteFile(filePath, []byte("direct content"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any

		err = json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req["text"] != "direct content" {
			t.Errorf("expected text 'direct content', got %v", req["text"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/DIRECT.txt"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL
	cfg.AllowedPaths = []string{allowedDir}
	cfg.SandboxMounts = []SandboxMount{
		{HostPath: allowedDir, SandboxPath: "/workspace"},
	}

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Without TranslateSandboxPath, the path is used as-is and must exist on host.
	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		FilePath: &filePath,
		// TranslateSandboxPath not set.
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreatePaste_FileMode_SandboxTranslationNoMounts(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.ServerURL = "http://localhost:12345"

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	sandboxPath := "/workspace/file.txt"
	translate := true

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		FilePath:             &sandboxPath,
		TranslateSandboxPath: &translate,
	})
	if err == nil {
		t.Fatal("expected error for sandbox translation with no mounts")
	}

	if !errors.Is(err, errSandboxTranslationNoMounts) {
		t.Errorf("expected errSandboxTranslationNoMounts, got: %v", err)
	}
}

func TestCreatePaste_FileMode_SandboxTranslationNoMatch(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.ServerURL = "http://localhost:12345"
	cfg.SandboxMounts = []SandboxMount{
		{HostPath: "/tmp", SandboxPath: "/workspace"},
	}

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Path does not match any configured mount.
	sandboxPath := "/other/path/file.txt"
	translate := true

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		FilePath:             &sandboxPath,
		TranslateSandboxPath: &translate,
	})
	if err == nil {
		t.Fatal("expected error for unmounted sandbox path")
	}

	if !errors.Is(err, errSandboxTranslationNoMatch) {
		t.Errorf("expected errSandboxTranslationNoMatch, got: %v", err)
	}
}

func TestCreatePaste_SandboxTranslationUnderMountHost_Rejected(t *testing.T) {
	t.Parallel()

	// When HostPath is "/", isUnderMountHost uses "//" as the prefix check.
	// No path starts with "//", so every translated path fails the check
	// and is rejected, even though it's technically under "/".
	cfg := DefaultConfig()
	cfg.ServerURL = "http://localhost:12345"
	cfg.SandboxMounts = []SandboxMount{
		{HostPath: "/", SandboxPath: "/workspace"},
	}

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	sandboxPath := "/workspace/foo"
	translate := true

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		FilePath:             &sandboxPath,
		TranslateSandboxPath: &translate,
	})
	if err == nil {
		t.Fatal("expected error for escaped sandbox path, got nil")
	}

	if !errors.Is(err, errPathTraversal) {
		t.Errorf("expected errPathTraversal, got: %v", err)
	}
}

func TestCreatePaste_SandboxPathTraversal_Rejected(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	// Create a target outside the allowed dir to confirm it's NOT read.
	outsideDir := filepath.Join(tmpDir, "outside")

	err = os.Mkdir(outsideDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("leaked"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("server should not be reached when path traversal is detected")
	}))
	t.Cleanup(server.Close)

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL
	cfg.AllowedPaths = []string{allowedDir}
	cfg.SandboxMounts = []SandboxMount{
		{HostPath: allowedDir, SandboxPath: "/workspace"},
	}

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{"parent dir escape", "/workspace/../secret.txt"},
		{"double parent dir escape", "/workspace/../../etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			translate := true

			_, err := client.CreatePaste(context.Background(), &CreatePasteArgs{
				FilePath:             &tt.path,
				TranslateSandboxPath: &translate,
			})
			if err == nil {
				t.Fatal("expected error for sandbox path traversal, got nil")
			}

			if !errors.Is(err, errPathTraversal) {
				t.Errorf("expected errPathTraversal, got: %v", err)
			}
		})
	}
}

func TestCreatePaste_SandboxPathWithoutTraversalFlag_StillBlocksTraversal(t *testing.T) {
	t.Parallel()
	// Even without translate_sandbox_path, the raw path with ".." should be
	// rejected by validateFilePath's own traversal check.
	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("server should not be reached")
	}))
	t.Cleanup(server.Close)

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL
	cfg.AllowedPaths = []string{allowedDir}

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	traversalPath := "../secret.txt"

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		FilePath: &traversalPath,
	})
	if err == nil {
		t.Fatal("expected error for path traversal without sandbox flag, got nil")
	}

	if !errors.Is(err, errPathTraversal) {
		t.Errorf("expected errPathTraversal, got: %v", err)
	}
}

func TestCreatePaste_ExtensionDetectionFromFilePath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(allowedDir, "script.py")

	err = os.WriteFile(filePath, []byte("print('hello')"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any

		err = json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req["extension"] != "py" {
			t.Errorf("expected extension 'py' from file path, got %v", req["extension"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/SCRIPT.py"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL
	cfg.AllowedPaths = []string{allowedDir}

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	resp, err := client.CreatePaste(context.Background(), &CreatePasteArgs{
		FilePath: &filePath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The response path includes .py, so ID should be without it.
	if resp.ID != "SCRIPT" {
		t.Errorf("expected ID 'SCRIPT', got %q", resp.ID)
	}
}

func TestCreatePaste_HintFromFileModeNoExtension(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	// File without extension (like a Makefile or Dockerfile).
	filePath := filepath.Join(allowedDir, "Dockerfile")

	err = os.WriteFile(filePath, []byte("FROM alpine"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any

		err = json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		// No extension detected from "Dockerfile" — should be omitted from JSON.
		if ext, ok := req["extension"]; ok {
			t.Errorf("expected extension to be omitted, got %v", ext)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/DOCKERFILE"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL
	cfg.AllowedPaths = []string{allowedDir}

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	resp, err := client.CreatePaste(context.Background(), &CreatePasteArgs{
		FilePath: &filePath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Hint == "" {
		t.Error("expected Hint to be non-empty for extensionless file")
	}

	if !strings.Contains(resp.Hint, "Extension not detected") {
		t.Errorf("expected hint about extension, got %q", resp.Hint)
	}
}

func TestCreatePaste_ConnectionError(t *testing.T) {
	t.Parallel()
	// Create a server that we immediately close to trigger a connection error.
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		// Not reached.
	}))
	server.Close() // Close immediately — subsequent dial gets "connection refused".

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "test"

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content: &content,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "cannot connect to Wastebin server") {
		t.Errorf("expected connection error message, got: %v", err)
	}
}

func TestCreatePaste_MarkdownRenderedInFileMode(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	allowedDir := filepath.Join(tmpDir, "allowed")

	err := os.Mkdir(allowedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(allowedDir, "readme.md")

	err = os.WriteFile(filePath, []byte("# Readme"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/README.md"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL
	cfg.AllowedPaths = []string{allowedDir}

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	resp, err := client.CreatePaste(context.Background(), &CreatePasteArgs{
		FilePath: &filePath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.MarkdownRendered != "/md/README.md" {
		t.Errorf("expected MarkdownRendered '/md/README.md', got %q", resp.MarkdownRendered)
	}
}

func TestNewWastebinClient_NilConfig(t *testing.T) {
	t.Parallel()

	_, err := NewWastebinClient(nil)
	if err == nil {
		t.Fatal("expected error for nil config, got nil")
	}

	if !errors.Is(err, errConfigRequired) {
		t.Errorf("expected errConfigRequired, got: %v", err)
	}
}

func TestNewWastebinClient_EmptyServerURL(t *testing.T) {
	t.Parallel()

	_, err := NewWastebinClient(&Config{})
	if err == nil {
		t.Fatal("expected error for empty server URL, got nil")
	}
}

func TestNewWastebinClient_BadURL(t *testing.T) {
	t.Parallel()

	_, err := NewWastebinClient(&Config{ServerURL: "://bad"})
	if err == nil {
		t.Fatal("expected error for bad URL, got nil")
	}
}

func TestNewWastebinClient_NoScheme(t *testing.T) {
	t.Parallel()

	_, err := NewWastebinClient(&Config{ServerURL: "localhost:8080"})
	if err == nil {
		t.Fatal("expected error for missing scheme, got nil")
	}
}

func TestNewWastebinClient_FTPScheme(t *testing.T) {
	t.Parallel()

	_, err := NewWastebinClient(&Config{ServerURL: "ftp://server"})
	if !errors.Is(err, errUnsupportedURLScheme) {
		t.Errorf("expected errUnsupportedURLScheme, got: %v", err)
	}
}

func TestNewWastebinClient_URLWithPathNoHost(t *testing.T) {
	t.Parallel()

	_, err := NewWastebinClient(&Config{ServerURL: "http:///path"})
	if !errors.Is(err, errURLMissingHost) {
		t.Errorf("expected errURLMissingHost, got: %v", err)
	}
}

func TestNewWastebinClient_NoHost(t *testing.T) {
	t.Parallel()

	_, err := NewWastebinClient(&Config{ServerURL: "http://"})
	if err == nil {
		t.Fatal("expected error for missing host, got nil")
	}
}

// TestBuildPasteResponse tests the response builder directly.

func TestBuildPasteResponse_WithExt(t *testing.T) {
	t.Parallel()

	baseURL, _ := url.Parse("https://bin.example.com") //nolint:errcheck // URL literal is safe
	resp := buildPasteResponse(baseURL, "/ABC.go", "go", false)

	if resp.ID != "ABC" {
		t.Errorf("expected ID 'ABC', got %q", resp.ID)
	}

	if resp.URL != "/ABC.go" {
		t.Errorf("expected URL '/ABC.go', got %q", resp.URL)
	}

	if resp.Raw != "/raw/ABC.go" {
		t.Errorf("expected Raw '/raw/ABC.go', got %q", resp.Raw)
	}

	if resp.MarkdownRendered != "" {
		t.Errorf("expected empty MarkdownRendered, got %q", resp.MarkdownRendered)
	}

	if resp.Hint != "" {
		t.Errorf("expected empty Hint, got %q", resp.Hint)
	}
}

func TestBuildPasteResponse_Markdown(t *testing.T) {
	t.Parallel()

	baseURL, _ := url.Parse("https://bin.example.com") //nolint:errcheck // URL literal is safe
	resp := buildPasteResponse(baseURL, "/POST.md", "md", false)

	if resp.ID != "POST" {
		t.Errorf("expected ID 'POST', got %q", resp.ID)
	}

	if resp.MarkdownRendered != "/md/POST.md" {
		t.Errorf("expected MarkdownRendered '/md/POST.md', got %q", resp.MarkdownRendered)
	}

	if resp.Hint != "" {
		t.Errorf("expected empty Hint, got %q", resp.Hint)
	}
}

func TestBuildPasteResponse_NoExt(t *testing.T) {
	t.Parallel()

	baseURL, _ := url.Parse("https://bin.example.com") //nolint:errcheck // URL literal is safe
	resp := buildPasteResponse(baseURL, "/NOPATH", "", false)

	if resp.ID != "NOPATH" {
		t.Errorf("expected ID 'NOPATH', got %q", resp.ID)
	}

	if resp.MarkdownRendered != "" {
		t.Errorf("expected empty MarkdownRendered, got %q", resp.MarkdownRendered)
	}

	if resp.Hint == "" {
		t.Error("expected non-empty Hint for no extension")
	}
}

func TestBuildPasteResponse_MultiPartExt(t *testing.T) {
	t.Parallel()

	baseURL, _ := url.Parse("https://bin.example.com") //nolint:errcheck // URL literal is safe
	resp := buildPasteResponse(baseURL, "/abc.tar.gz", "tar.gz", false)

	if resp.ID != "abc" {
		t.Errorf("expected ID 'abc', got %q", resp.ID)
	}

	if resp.URL != "/abc.tar.gz" {
		t.Errorf("expected URL '/abc.tar.gz', got %q", resp.URL)
	}

	if resp.Raw != "/raw/abc.tar.gz" {
		t.Errorf("expected Raw '/raw/abc.tar.gz', got %q", resp.Raw)
	}
}

func TestBuildPasteResponse_TrailingSlashBaseURL(t *testing.T) {
	t.Parallel()

	baseURL, _ := url.Parse("https://bin.example.com/") //nolint:errcheck // URL literal is safe
	resp := buildPasteResponse(baseURL, "/XYZ.go", "go", false)

	if resp.Hostname != "https://bin.example.com" {
		t.Errorf("expected hostname without trailing slash, got %q", resp.Hostname)
	}
}

// errorReader is a helper for testing closeResponseBody with a body that
// returns an error on Close. It implements io.ReadCloser.
type errorReader struct{}

func (errorReader) Read(_ []byte) (int, error) { return 0, io.EOF }

func (errorReader) Close() error { return errors.New("close error") } //nolint:err113 // test helper

//nolint:paralleltest // intentional: modifies global slog state
func TestCloseResponseBody_CloseErrorLogged(t *testing.T) {
	// NOTE: no t.Parallel() — slog.SetDefault() modifies global state and
	// would race with other parallel tests that log via slog.Debug().

	// Create a slog handler that captures log records.
	var logBuf bytes.Buffer

	handler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})
	original := slog.Default()

	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(original)

	// Response body whose Close returns an error.
	resp := &http.Response{
		Body: &errorReader{},
	}

	closeResponseBody(resp)

	output := logBuf.String()
	if !strings.Contains(output, "failed to close response body") {
		t.Error("expected debug log about close error, got:", output)
	}
}

func TestIsConnectionError_NonMatch(t *testing.T) {
	t.Parallel()

	errTestNonDial := errors.New("some other error") //nolint:err113 // test helper error
	result := isConnectionError(errTestNonDial)

	if result {
		t.Error("expected false for non-dial error")
	}
}

func TestNewWastebinClient_RedirectFollowed(t *testing.T) {
	t.Parallel()

	// Server that redirects once to final destination.
	// baseURL.JoinPath("/") appends "/", so the POST path has a trailing slash.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect/" {
			// Redirect to the root which returns the JSON response.
			http.Redirect(w, r, "/", http.StatusFound)

			return
		}

		// Final destination.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/REDIRECTED"}) //nolint:errcheck // Test helper OK
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = ts.URL + "/redirect" // postURL becomes ts.URL + "/redirect/".

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "test"

	resp, err := client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content: &content,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "REDIRECTED" {
		t.Errorf("expected ID 'REDIRECTED', got %q", resp.ID)
	}
}

func TestNewWastebinClient_RedirectDifferentHostBlocked(t *testing.T) {
	t.Parallel()

	// baseURL.JoinPath("/") appends "/", so the POST path has a trailing slash.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect/" {
			// Redirect to a completely different host.
			http.Redirect(w, r, "http://evil.example.com/malicious", http.StatusFound)

			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = ts.URL + "/redirect" // postURL becomes ts.URL + "/redirect/".

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "test"

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content: &content,
	})
	if err == nil {
		t.Fatal("expected redirect error, got nil")
	}

	if !strings.Contains(err.Error(), "redirect to different host") {
		t.Errorf("expected redirect blocked error, got: %v", err)
	}
}

func TestCreatePaste_NilArgs(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.ServerURL = "http://localhost:12345"

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.CreatePaste(context.Background(), nil)
	if !errors.Is(err, errArgsRequired) {
		t.Errorf("expected errArgsRequired, got: %v", err)
	}
}

func TestCreatePaste_ContentEmpty(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.ServerURL = "http://localhost:12345"

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	empty := ""

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content: &empty,
	})
	if !errors.Is(err, errContentEmpty) {
		t.Errorf("expected errContentEmpty, got: %v", err)
	}
}

func TestCreatePaste_PasswordOverHTTP(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any

		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if _, ok := req["password"]; !ok {
			t.Error("expected password in request body")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/PASSWD"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL // httptest uses http, so password over HTTP warning triggers.

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "secret"
	password := "hunter2"

	resp, err := client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content:  &content,
		Password: &password,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.PasswordHint == "" {
		t.Error("expected PasswordHint to be set for password-protected paste")
	}
}

func TestCreatePaste_InvalidExpiration(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/EXPIRED"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "test"
	badExpiry := "not-a-valid-expiry"

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content: &content,
		Expires: &badExpiry,
	})
	if err == nil {
		t.Fatal("expected error for invalid expiration")
	}

	if !strings.Contains(err.Error(), "invalid expiration") {
		t.Errorf("expected invalid expiration error, got: %v", err)
	}
}

func TestCreatePaste_JSONDecodeError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json`)) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "test"

	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content: &content,
	})
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}

	if !strings.Contains(err.Error(), "failed to parse Wastebin response") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

func TestCreatePaste_TitleInBody(t *testing.T) {
	t.Parallel()

	var capturedTitle any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any

		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		capturedTitle = req["title"]

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/TITLE"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "test"
	title := "My Test Paste"

	resp, err := client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content: &content,
		Title:   &title,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "TITLE" {
		t.Errorf("expected ID TITLE, got %q", resp.ID)
	}

	if capturedTitle != "My Test Paste" {
		t.Errorf("expected title 'My Test Paste', got %v", capturedTitle)
	}
}

func TestCreatePaste_BurnAfterReadingInBody(t *testing.T) {
	t.Parallel()

	var capturedBurn any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any

		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		capturedBurn = req["burn_after_reading"]

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/BURN"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "test"
	burn := true

	resp, err := client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content:          &content,
		BurnAfterReading: &burn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "BURN" {
		t.Errorf("expected ID BURN, got %q", resp.ID)
	}

	if capturedBurn != true {
		t.Errorf("expected burn_after_reading=true, got %v", capturedBurn)
	}
}

func TestCreatePaste_DefaultExpiresInBody(t *testing.T) {
	t.Parallel()

	var capturedExpires any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any

		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		capturedExpires = req["expires"]

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/EXP"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "test"
	// No Expires set — should use default (31536000)
	resp, err := client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content: &content,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "EXP" {
		t.Errorf("expected ID EXP, got %q", resp.ID)
	}

	if capturedExpires != float64(31536000) {
		t.Errorf("expected default expires 31536000, got %v", capturedExpires)
	}
}

func TestCreatePaste_TitleOmittedWhenNil(t *testing.T) {
	t.Parallel()

	var capturedTitle any

	hasTitleKey := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any

		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		_, hasTitleKey = req["title"]
		capturedTitle = req["title"]

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/NOTITLE"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.ServerURL = server.URL

	client, err := NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content := "test"
	// No Title set — should not be in JSON body
	_, err = client.CreatePaste(context.Background(), &CreatePasteArgs{
		Content: &content,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hasTitleKey {
		t.Error("expected title key to be omitted from JSON body when Title is nil")
	}

	if capturedTitle != nil {
		t.Errorf("expected nil title, got %v", capturedTitle)
	}
}
