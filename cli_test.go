package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"wastebin-mcp-go/internal/wastebin"
)

func TestBuildCreatePasteArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		flags                    CLIFlags
		wantContent              *string
		wantFilePath             *string
		wantExtension            *string
		wantExpires              *string
		wantTitle                *string
		wantBurnAfterReading     *bool
		wantPassword             *string
		wantTranslateSandboxPath *bool
	}{
		{
			name:        "content only",
			flags:       CLIFlags{Content: "hello world"},
			wantContent: new("hello world"),
		},
		{
			name:         "file_path only",
			flags:        CLIFlags{FilePath: "/tmp/doc.md"},
			wantFilePath: new("/tmp/doc.md"),
		},
		{
			name:  "content and file_path both empty (nil output)",
			flags: CLIFlags{},
			// All fields should remain nil
		},
		{
			name:          "extension override",
			flags:         CLIFlags{Content: "code", Extension: "go"},
			wantContent:   new("code"),
			wantExtension: new("go"),
		},
		{
			name:        "expires",
			flags:       CLIFlags{Content: "ephemeral", Expires: "1h"},
			wantContent: new("ephemeral"),
			wantExpires: new("1h"),
		},
		{
			name:        "title",
			flags:       CLIFlags{Content: "my paste", Title: "My Paste Title"},
			wantContent: new("my paste"),
			wantTitle:   new("My Paste Title"),
		},
		{
			name:                 "burn_after_reading",
			flags:                CLIFlags{Content: "secret", BurnAfterReading: true},
			wantContent:          new("secret"),
			wantBurnAfterReading: new(true),
		},
		{
			name:        "burn_after_reading false (default) — pointer stays nil",
			flags:       CLIFlags{Content: "normal"},
			wantContent: new("normal"),
			// BurnAfterReading is false in flags, so pointer should be nil
		},
		{
			name:         "password",
			flags:        CLIFlags{Content: "encrypted", Password: "hunter2"},
			wantContent:  new("encrypted"),
			wantPassword: new("hunter2"),
		},
		{
			name:        "translate_sandbox_path — no field in CLIFlags, always nil",
			flags:       CLIFlags{Content: "paste"},
			wantContent: new("paste"),
			// TranslateSandboxPath is not set by buildCreatePasteArgs
		},
		{
			name: "all fields set",
			flags: CLIFlags{
				Content:          "full paste",
				FilePath:         "/path/to/file",
				Extension:        "py",
				Expires:          "7d",
				Title:            "Full Test",
				BurnAfterReading: true,
				Password:         "secret123",
			},
			wantContent:          new("full paste"),
			wantFilePath:         new("/path/to/file"),
			wantExtension:        new("py"),
			wantExpires:          new("7d"),
			wantTitle:            new("Full Test"),
			wantBurnAfterReading: new(true),
			wantPassword:         new("secret123"),
			// TranslateSandboxPath has no corresponding CLIFlags field
		},
		{
			name: "various combinations — content + extension + expires",
			flags: CLIFlags{
				Content:   "combo",
				Extension: "md",
				Expires:   "30M",
			},
			wantContent:   new("combo"),
			wantExtension: new("md"),
			wantExpires:   new("30M"),
		},
		{
			name: "various combinations — file_path + title + password",
			flags: CLIFlags{
				FilePath: "/data/report.txt",
				Title:    "Report",
				Password: "1234",
			},
			wantFilePath: new("/data/report.txt"),
			wantTitle:    new("Report"),
			wantPassword: new("1234"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			args := buildCreatePasteArgs(&tt.flags)

			// Compare each field
			assertStringPtr(t, "Content", args.Content, tt.wantContent)
			assertStringPtr(t, "FilePath", args.FilePath, tt.wantFilePath)
			assertStringPtr(t, "Extension", args.Extension, tt.wantExtension)
			assertStringPtr(t, "Expires", args.Expires, tt.wantExpires)
			assertStringPtr(t, "Title", args.Title, tt.wantTitle)
			assertStringPtr(t, "Password", args.Password, tt.wantPassword)
			assertBoolPtr(t, "BurnAfterReading", args.BurnAfterReading, tt.wantBurnAfterReading)
			assertBoolPtr(t, "TranslateSandboxPath", args.TranslateSandboxPath, tt.wantTranslateSandboxPath)
		})
	}
}

// assertStringPtr compares two *string values for equality.
func assertStringPtr(t *testing.T, name string, got, want *string) {
	t.Helper()

	switch {
	case got == nil && want == nil:
		// Both nil — OK
	case got == nil:
		t.Errorf("%s: got nil, want %v", name, *want)
	case want == nil:
		t.Errorf("%s: got %v, want nil", name, *got)
	case *got != *want:
		t.Errorf("%s: got %q, want %q", name, *got, *want)
	}
}

// assertBoolPtr compares two *bool values for equality.
func assertBoolPtr(t *testing.T, name string, got, want *bool) {
	t.Helper()

	switch {
	case got == nil && want == nil:
		// Both nil — OK
	case got == nil:
		t.Errorf("%s: got nil, want %v", name, *want)
	case want == nil:
		t.Errorf("%s: got %v, want nil", name, *got)
	case *got != *want:
		t.Errorf("%s: got %t, want %t", name, *got, *want)
	}
}

func TestBuildCreatePasteArgsWithParseCreateFlagsContent(t *testing.T) {
	t.Parallel()

	// Integration-style: verify that parseCreateFlags -> buildCreatePasteArgs
	// produces the expected CreatePasteArgs for a full flags input.
	flags, err := parseCreateFlags([]string{
		"--content", "hello",
		"--extension", "md",
		"--expires", "3600",
		"--title", "test paste",
		"--burn-after-reading",
		"--password", "secret",
	})
	if err != nil {
		t.Fatalf("parseCreateFlags failed: %v", err)
	}

	args := buildCreatePasteArgs(flags)

	assertStringPtr(t, "Content", args.Content, new("hello"))
	assertStringPtr(t, "Extension", args.Extension, new("md"))
	assertStringPtr(t, "Expires", args.Expires, new("3600"))
	assertStringPtr(t, "Title", args.Title, new("test paste"))
	assertStringPtr(t, "Password", args.Password, new("secret"))
	assertBoolPtr(t, "BurnAfterReading", args.BurnAfterReading, new(true))

	// Fields not passed should remain nil
	assertStringPtr(t, "FilePath", args.FilePath, nil)
	assertBoolPtr(t, "TranslateSandboxPath", args.TranslateSandboxPath, nil)
}

func TestBuildCreatePasteArgsWithParseCreateFlagsFilePath(t *testing.T) {
	t.Parallel()

	flags, err := parseCreateFlags([]string{
		"--file-path", "/tmp/doc.md",
	})
	if err != nil {
		t.Fatalf("parseCreateFlags failed: %v", err)
	}

	args := buildCreatePasteArgs(flags)

	assertStringPtr(t, "FilePath", args.FilePath, new("/tmp/doc.md"))
	assertStringPtr(t, "Content", args.Content, nil)
	assertStringPtr(t, "Extension", args.Extension, nil)
	assertStringPtr(t, "Expires", args.Expires, nil)
	assertStringPtr(t, "Title", args.Title, nil)
	assertStringPtr(t, "Password", args.Password, nil)
	assertBoolPtr(t, "BurnAfterReading", args.BurnAfterReading, nil)
	assertBoolPtr(t, "TranslateSandboxPath", args.TranslateSandboxPath, nil)
}

func TestBuildCreatePasteArgsTypeCheck(t *testing.T) {
	t.Parallel()

	// Verify that buildCreatePasteArgs returns a *wastebin.CreatePasteArgs
	// (compile-time check; the real assertion is that this compiles).
	flags := &CLIFlags{Content: "type-check"}
	args := buildCreatePasteArgs(flags)

	if _, ok := any(args).(*wastebin.CreatePasteArgs); !ok {
		t.Error("buildCreatePasteArgs did not return *wastebin.CreatePasteArgs")
	}
}

func TestRunCLIModeWithTestServer(t *testing.T) {
	// Cannot use t.Parallel() due to environment variable manipulation.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		decodeErr := json.NewDecoder(r.Body).Decode(&req)
		if decodeErr != nil {
			t.Errorf("failed to decode request: %v", decodeErr)
		}

		if req["text"] != "hello" {
			t.Errorf("expected text 'hello', got %v", req["text"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/ABCDEFG"}) //nolint:errcheck // Test helper OK
	}))
	defer ts.Close()

	t.Setenv("WASTEBIN_MCP_SERVER_URL", ts.URL)
	t.Setenv("DEBUG", "false")

	err := runCLIMode(&CLIFlags{Content: "hello"})
	if err != nil {
		t.Fatalf("runCLIMode failed: %v", err)
	}
}
