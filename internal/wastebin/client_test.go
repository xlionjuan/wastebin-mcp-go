package wastebin //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
