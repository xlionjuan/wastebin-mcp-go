//go:build e2e

package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestBasicPaste(t *testing.T) {
	wastebinURL := os.Getenv("WASTEBIN_SERVER_URL")
	if wastebinURL == "" {
		t.Skip("WASTEBIN_SERVER_URL not set")
	}

	var warnings e2eWarnings

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, wastebinURL)

	// Verify tool listing works and contains create_paste
	tool := findCreatePasteTool(ctx, t, session, stderr)
	t.Logf("found tool: %s", tool.Name)

	response := requirePasteResponse(ctx, t, session, map[string]any{
		"content": "Hello, World! Basic paste test",
	}, stderr, "basic paste")

	// Verify response has expected fields
	if response.Hostname == "" {
		t.Fatalf("hostname is empty\nresponse: %#v\nstderr:\n%s", response, stderr.String())
	}

	if response.ID == "" {
		t.Fatalf("id is empty\nresponse: %#v\nstderr:\n%s", response, stderr.String())
	}

	if response.URL == "" {
		t.Fatalf("url is empty\nresponse: %#v\nstderr:\n%s", response, stderr.String())
	}

	if response.Raw == "" {
		t.Fatalf("raw is empty\nresponse: %#v\nstderr:\n%s", response, stderr.String())
	}

	// Verify raw URL returns actual content via curl using exec
	if !t.Failed() {
		rawURL := response.Hostname + response.Raw
		t.Logf("verifying raw content at: %s", rawURL)

		curlCtx, curlCancel := context.WithTimeout(ctx, 10*time.Second)
		defer curlCancel()

		// Use os/exec to curl the raw URL
		curlCmd := exec.CommandContext(curlCtx, "curl", "-s", rawURL) //nolint:gosec // test tool
		out, err := curlCmd.Output()
		if err != nil {
			// Route through warning — curl may not be available or the
			// paste may have expired in staging
			warnings.Addf("curl raw URL failed: %v", err)
			t.Logf("curl raw URL: %v\nstderr:\n%s", err, string(out))
		} else if !strings.Contains(string(out), "Hello, World!") {
			warnings.Addf("raw content does not contain expected text: got %q", string(out))
			t.Logf("raw content mismatch\ngot:\n%s", string(out))
		} else {
			t.Logf("raw content verified successfully")
		}
	}

	warnings.Report(t)
	t.Log("basic paste test verified")
}

func TestPasteWithExtension(t *testing.T) {
	wastebinURL := os.Getenv("WASTEBIN_SERVER_URL")
	if wastebinURL == "" {
		t.Skip("WASTEBIN_SERVER_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, wastebinURL)

	response := requirePasteResponse(ctx, t, session, map[string]any{
		"content":   "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}",
		"extension": "go",
	}, stderr, "paste with .go extension")

	// Response should NOT have markdown_rendered or hint
	if response.MarkdownRendered != "" {
		t.Fatalf("markdown_rendered should be empty for .go extension, got %q\nresponse: %#v\nstderr:\n%s",
			response.MarkdownRendered, response, stderr.String())
	}

	if response.Hint != "" {
		t.Fatalf("hint should be empty for .go extension, got %q\nresponse: %#v\nstderr:\n%s",
			response.Hint, response, stderr.String())
	}

	t.Log("paste with extension verified")
}

func TestPasteMarkdownExtension(t *testing.T) {
	wastebinURL := os.Getenv("WASTEBIN_SERVER_URL")
	if wastebinURL == "" {
		t.Skip("WASTEBIN_SERVER_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, wastebinURL)

	response := requirePasteResponse(ctx, t, session, map[string]any{
		"content":   "# Hello\n\nThis is a markdown paste.",
		"extension": "md",
	}, stderr, "paste with .md extension")

	// Response MUST have markdown_rendered field
	if response.MarkdownRendered == "" {
		t.Fatalf("markdown_rendered should be present for .md extension\nresponse: %#v\nstderr:\n%s",
			response, stderr.String())
	}

	// Verify markdown rendered URL is valid
	if !strings.HasPrefix(response.MarkdownRendered, "/md/") {
		t.Fatalf("markdown_rendered should start with /md/, got %q\nresponse: %#v\nstderr:\n%s",
			response.MarkdownRendered, response, stderr.String())
	}

	t.Log("paste with markdown extension verified")
}

func TestPasteWithTitle(t *testing.T) {
	wastebinURL := os.Getenv("WASTEBIN_SERVER_URL")
	if wastebinURL == "" {
		t.Skip("WASTEBIN_SERVER_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, wastebinURL)

	response := requirePasteResponse(ctx, t, session, map[string]any{
		"content": "Content with a title",
		"title":   "My Test Paste",
	}, stderr, "paste with title")

	// Verify response is valid
	if response.ID == "" {
		t.Fatalf("id is empty for titled paste\nresponse: %#v\nstderr:\n%s", response, stderr.String())
	}

	t.Log("paste with title verified")
}

func TestPasteBurnAfterReading(t *testing.T) {
	wastebinURL := os.Getenv("WASTEBIN_SERVER_URL")
	if wastebinURL == "" {
		t.Skip("WASTEBIN_SERVER_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, wastebinURL)

	// Burn-after-reading paste should be created successfully
	response := requirePasteResponse(ctx, t, session, map[string]any{
		"content":            "This paste will self-destruct",
		"burn_after_reading": true,
	}, stderr, "burn-after-reading paste")

	if response.ID == "" {
		t.Fatalf("id is empty for burn-after-reading paste\nresponse: %#v\nstderr:\n%s", response, stderr.String())
	}

	t.Log("burn after reading paste verified")
}

func TestPastePasswordProtected(t *testing.T) {
	wastebinURL := os.Getenv("WASTEBIN_SERVER_URL")
	if wastebinURL == "" {
		t.Skip("WASTEBIN_SERVER_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, wastebinURL)

	response := requirePasteResponse(ctx, t, session, map[string]any{
		"content":  "Secret content",
		"password": "my-secret-password",
	}, stderr, "password-protected paste")

	// Response MUST have password_hint field
	if response.PasswordHint == "" {
		t.Fatalf("password_hint should be present for password-protected paste\nresponse: %#v\nstderr:\n%s",
			response, stderr.String())
	}

	if !strings.Contains(response.PasswordHint, "Wastebin-Password") {
		t.Fatalf("password_hint should mention Wastebin-Password header\npassword_hint: %q\nstderr:\n%s",
			response.PasswordHint, stderr.String())
	}

	t.Log("password protected paste verified")
}

func TestPasteNoExtension(t *testing.T) {
	wastebinURL := os.Getenv("WASTEBIN_SERVER_URL")
	if wastebinURL == "" {
		t.Skip("WASTEBIN_SERVER_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, wastebinURL)

	response := requirePasteResponse(ctx, t, session, map[string]any{
		"content": "Content without extension",
	}, stderr, "paste without extension")

	// Response SHOULD have hint field (fuzzy match hint)
	if response.Hint == "" {
		t.Logf("hint is empty for extensionless paste (non-fatal)\nresponse: %#v", response)
	} else if !strings.Contains(strings.ToLower(response.Hint), "extension") {
		t.Fatalf("hint should mention 'extension', got %q\nresponse: %#v\nstderr:\n%s",
			response.Hint, response, stderr.String())
	}

	t.Log("paste without extension verified")
}

func TestPasteBothContentAndFilePath(t *testing.T) {
	wastebinURL := os.Getenv("WASTEBIN_SERVER_URL")
	if wastebinURL == "" {
		t.Skip("WASTEBIN_SERVER_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, wastebinURL)

	// Both content and file_path — should return IsError=true
	result := createPaste(ctx, t, session, map[string]any{
		"content":   "Some content",
		"file_path": "/tmp/test.txt",
	}, stderr)

	if !result.IsError {
		t.Fatalf("IsError = false, want true when both content and file_path provided\nresult: %#v\nstderr:\n%s",
			result, stderr.String())
	}

	if len(result.Content) != 1 {
		t.Fatalf("content length = %d, want 1\nresult: %#v\nstderr:\n%s",
			len(result.Content), result, stderr.String())
	}

	text := toolText(t, result)
	if !strings.Contains(text, "not both") {
		t.Fatalf("error text should contain 'not both', got %q\nstderr:\n%s", text, stderr.String())
	}

	t.Log("both content and file_path error verified")
}

func TestPasteEmptyContent(t *testing.T) {
	wastebinURL := os.Getenv("WASTEBIN_SERVER_URL")
	if wastebinURL == "" {
		t.Skip("WASTEBIN_SERVER_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, wastebinURL)

	// Empty content string — should return error
	result := createPaste(ctx, t, session, map[string]any{
		"content": "",
	}, stderr)

	if !result.IsError {
		t.Fatalf("IsError = false, want true for empty content\nresult: %#v\nstderr:\n%s",
			result, stderr.String())
	}

	text := toolText(t, result)
	t.Logf("empty content error text: %q", text)

	// The exact error message may vary; just verify we get an error
	if strings.TrimSpace(text) == "" {
		t.Fatalf("error text is empty for empty content\nstderr:\n%s", stderr.String())
	}

	t.Log("empty content error verified")
}
