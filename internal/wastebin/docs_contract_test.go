package wastebin //nolint:testpackage // needs access to unexported sentinels/constructors

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// errorDocPattern pairs a representative error, produced through real code,
// with the docs/MCP_TOOLS.md row that documents it. The test derives the
// canonical wording from the code and asserts it appears in the doc file, so
// wording drift between code and docs is caught by CI instead of by an agent
// eyeballing both. This is a small fixture of representative rows, not a
// parser: rows whose message carries a dynamic ": <value>" suffix are checked
// on their static "<problem>; <next-step guidance>" portion only, because the
// docs describe those values with <placeholder> markers.
type errorDocPattern struct {
	name string
	err  func() error
}

// errorDocCases returns representative errors spanning every error family.
func errorDocCases() []errorDocPattern {
	return []errorDocPattern{
		{name: "both content and file_path", err: func() error { return errBothContentAndFilePath }},
		{name: "neither content nor file_path", err: func() error { return errNeitherContentNorFilePath }},
		{name: "empty content", err: func() error { return errContentEmpty }},
		{name: "content too large", err: func() error { return newContentTooLargeError(10, 5) }},
		{name: "HTTP 403", err: func() error { return newHTTPError(http.StatusForbidden, "") }},
		{name: "HTTP 413", err: func() error { return newHTTPError(http.StatusRequestEntityTooLarge, "") }},
		{name: "HTTP 422 empty body", err: func() error { return newHTTPError(http.StatusUnprocessableEntity, "") }},
		{name: "HTTP unknown status", err: func() error { return newHTTPError(http.StatusInternalServerError, "") }},
		{name: "empty response path", err: func() error { return validateWastebinResponse(wastebinResponse{Path: ""}) }},
		{
			name: "response missing paste ID",
			err:  func() error { return validateWastebinResponse(wastebinResponse{Path: "/"}) },
		},
		{name: "response too large", err: func() error { return errResponseTooLarge }},
		{name: "path traversal", err: func() error { return errPathTraversal }},
		{name: "blocked prefix", err: func() error { return newBlockedPrefixError("/etc") }},
		{name: "blocked component", err: func() error { return newBlockedComponentError(".ssh") }},
		{name: "user blocked path", err: func() error { return errUserBlockedPath }},
		{name: "file not text", err: func() error { return errFileNotText }},
		{name: "file read disabled", err: func() error { return errFileReadDisabled }},
		{name: "path not found", err: func() error { return newPathNotFoundError(syscall.ENOENT) }},
		{name: "path permission denied", err: func() error { return newPathPermissionError(syscall.EACCES) }},
		{name: "file path cannot be used", err: func() error { return errFilePathCannotBeUsed }},
		{name: "sandbox no mounts", err: func() error { return errSandboxTranslationNoMounts }},
		{name: "sandbox no match", err: func() error { return newSandboxNoMatchError("/workspace/report.md") }},
		{name: "password over HTTP", err: func() error { return errPasswordOverHTTP }},
		{name: "password empty", err: func() error { return errPasswordEmpty }},
		{name: "HTTP request failed", err: func() error { return translateRequestError(io.ErrUnexpectedEOF) }},
		{name: "redirect different host", err: func() error { return errRedirectDifferentHost }},
		{name: "redirect too many", err: func() error { return errTooManyRedirects }},
		{name: "redirect scheme downgrade", err: func() error { return errRedirectSchemeDowngrade }},
		{name: "expiration negative", err: func() error { return ValidateExpiration(-1) }},
		{
			name: "expiration invalid format",
			err: func() error {
				_, err := ParseExpiration("abc", 0)

				return err
			},
		},
		{
			name: "expiration unknown unit",
			err: func() error {
				_, err := ParseExpiration("5x", 0)

				return err
			},
		},
		{
			name: "expiration overflow",
			err: func() error {
				_, err := ParseExpiration("999999999999999d", 3600)

				return err
			},
		},
		{name: "expiration too large", err: func() error { return ValidateExpiration(400000000) }},
	}
}

// TestDocsMCPTools_RuntimeErrorWording guards the documentation against
// drift: the canonical wording produced by the code for representative errors
// must appear verbatim in docs/MCP_TOOLS.md. Without this, an error-string
// change in code can pass every unit test while the docs silently diverge.
func TestDocsMCPTools_RuntimeErrorWording(t *testing.T) {
	t.Parallel()

	docsPath := filepath.Join("..", "..", "docs", "MCP_TOOLS.md")

	//nolint:gosec // fixed relative path to the repo's own docs file
	data, err := os.ReadFile(docsPath)
	if err != nil {
		t.Fatalf("read %s: %v", docsPath, err)
	}

	docs := string(data)

	for _, tc := range errorDocCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.err()
			if err == nil {
				t.Fatal("expected a non-nil error")
			}

			msg := err.Error()

			// Messages with a ": <value>" suffix are documented with a
			// <placeholder> marker, so only the static problem+guidance
			// portion must appear verbatim. Everything before the first
			// ": " is the static wording.
			needle := msg
			if static, _, ok := strings.Cut(msg, ": "); ok {
				needle = static
			}

			if !strings.Contains(docs, needle) {
				t.Errorf("docs/MCP_TOOLS.md does not contain canonical wording %q (from %q)", needle, msg)
			}
		})
	}
}
