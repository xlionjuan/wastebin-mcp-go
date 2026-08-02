package wastebin_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// errorDocCase pairs one documented error with the docs/MCP_TOOLS.md table row
// that documents it:
//
//   - row is the exact "Error Condition" cell used to locate the row, so a
//     deleted row or a drifted label fails the test.
//   - pattern is the complete "Message Pattern" the row must contain, including
//     its <placeholder> markers, quoting, and Markdown escaping resolved to the
//     runtime form. Verifying the full documented pattern (not just the static
//     wording) catches a placeholder regressing to literal syntax (e.g. `{path}`
//     for `<request URL>`), a dropped `<size>`/`<limit>` suffix, or broken
//     quoting/ordering.
//
// This is a small fixture of the rows documented in docs/MCP_TOOLS.md, not a
// parser of the whole Markdown file. The exact runtime wording of each error is
// pinned separately by errors_test.go and handler_test.go.
type errorDocCase struct {
	name    string
	row     string
	pattern string
}

// errorDocCases returns one case per handler-error row documented in
// docs/MCP_TOOLS.md (the "Always applicable", "File mode errors", and "Sandbox
// errors" tables), spanning every handler-error family documented there.
//
//nolint:lll // fixture patterns are complete documented messages exceeding 120 columns
func errorDocCases() []errorDocCase {
	return []errorDocCase{
		{
			name:    "both content and file_path",
			row:     "Both `content` and `file_path` provided (file mode enabled)",
			pattern: `"Create paste error: provide either 'content' or 'file_path', not both; pick exactly one input mode"`,
		},
		{
			name:    "neither content nor file_path",
			row:     "Neither `content` nor `file_path` provided (file mode enabled)",
			pattern: `"Create paste error: provide either 'content' or 'file_path'; supply one of the two inputs"`,
		},
		{
			name:    "empty content",
			row:     "`content` is empty (content mode)",
			pattern: `"Create paste error: content cannot be empty; provide non-empty content or use file_path instead"`,
		},
		{
			name:    "HTTP 403",
			row:     "HTTP 403 from server",
			pattern: `"Create paste error: server rejected the request; content may contain disallowed data; ask the user to check the content or the server logs"`,
		},
		{
			name:    "HTTP 413",
			row:     "HTTP 413 from server",
			pattern: `"Create paste error: content exceeds the server's maximum allowed size; split the content into smaller parts and upload each separately"`,
		},
		{
			name:    "connection refused or timeout",
			row:     "Connection refused / timeout",
			pattern: `"Create paste error: cannot connect to Wastebin server; verify the server is running: <details>"`,
		},
		{
			name:    "DNS resolution failure",
			row:     "DNS resolution failure",
			pattern: `"Create paste error: cannot resolve the server hostname; verify WASTEBIN_SERVER_URL points to a resolvable host: <details>"`,
		},
		{
			name:    "other HTTP request failure",
			row:     "Other HTTP request failure (e.g. TLS, proxy, unexpected transport error)",
			pattern: `"Create paste error: HTTP request failed; ask the user to check the server URL and the network connection: <details>"`,
		},
		{
			name:    "content too large",
			row:     "Content exceeds `WASTEBIN_MCP_MAX_CONTENT_SIZE`",
			pattern: `"Create paste error: content exceeds the maximum allowed size; split the content into smaller parts and upload each separately: <size> bytes exceeds limit of <limit> bytes"`,
		},
		{
			name:    "password over HTTP",
			row:     "Password over non-loopback HTTP without override",
			pattern: `"Create paste error: password-protected pastes are not allowed over non-loopback HTTP connections; use HTTPS or set WASTEBIN_MCP_ALLOW_INSECURE_PASSWORD=true for local development"`,
		},
		{
			name:    "password empty",
			row:     "`password` is empty string (defensive fallback, `minLength` should catch at schema first)",
			pattern: `"Create paste error: password must be non-empty when provided; set a non-empty password or omit the field entirely"`,
		},
		{
			name:    "unknown HTTP error",
			row:     "Unknown HTTP error",
			pattern: `"Create paste error: unknown HTTP error; ask the user to check the server status or the request: HTTP <code>"`,
		},
		{
			name: "invalid expiration",
			row:  "Invalid expiration",
			pattern: `"Create paste error: <reason>"` + " — " +
				"<reason> is one of: expiration cannot be negative; use a non-negative expiration value, " +
				"unknown expiration unit; use a supported unit (s, m, h, d, w, M, y): <unit>, " +
				"invalid expiration format; use a bare number (seconds) or a number plus unit suffix: <value>, " +
				"expiration overflow; use a smaller expiration value, " +
				"expiration exceeds maximum supported value; use an expiration of at most 315360000 seconds (10 years): <seconds> seconds exceeds maximum of 315360000 seconds, " +
				"invalid expiration number; use a valid numeric expiration value: <details>",
		},
		{
			name:    "invalid extension",
			row:     "Extension contains invalid path or query characters",
			pattern: `"Create paste error: extension contains invalid characters (/, \, ?, #); use a plain extension like 'go' or 'py': "<extension>"`,
		},
		{
			name:    "empty response path",
			row:     "Server returns response with empty paste path",
			pattern: `"Create paste error: invalid Wastebin response; ask the user to check the server configuration: empty path"`,
		},
		{
			name:    "non-relative response path",
			row:     "Server returns response with non-relative paste path",
			pattern: `"Create paste error: invalid Wastebin response; ask the user to check the server configuration: path must be relative, got <path>"`,
		},
		{
			name:    "response missing paste ID",
			row:     "Server returns response without paste ID",
			pattern: `"Create paste error: invalid Wastebin response; ask the user to check the server configuration: path is missing paste ID"`,
		},
		{
			name:    "malformed JSON",
			row:     "Server returns malformed JSON",
			pattern: `"Create paste error: failed to parse Wastebin response; ask the user to check the server configuration: <details>"`,
		},
		{
			name:    "failed to read response body",
			row:     "Failed to read the Wastebin response body",
			pattern: `"Create paste error: failed to read Wastebin response; ask the user to check the server configuration: <details>"`,
		},
		{
			name:    "response too large",
			row:     "Wastebin response exceeds maximum allowed size",
			pattern: `"Create paste error: wastebin response exceeds maximum allowed size; ask the user to check the server configuration"`,
		},
		{
			name:    "trailing content after JSON",
			row:     "Server returns response with trailing non-whitespace content",
			pattern: `"Create paste error: invalid Wastebin response; ask the user to check the server configuration: unexpected content after JSON response"`,
		},
		{
			name:    "HTTP 422 with body",
			row:     "HTTP 422 from server (with body)",
			pattern: `"Create paste error: server rejected the request due to a validation error; ask the user to review the content and parameters for invalid values: <details>"`,
		},
		{
			name:    "HTTP 422 empty body",
			row:     "HTTP 422 from server (empty body)",
			pattern: `"Create paste error: server rejected the request due to a validation error; ask the user to review the content and parameters for invalid values"`,
		},
		{
			name:    "redirect to different host",
			row:     "Cross-host redirect blocked",
			pattern: `"Create paste error: redirect to different host blocked; ask the user to check the server URL and its redirects: <request URL>"`,
		},
		{
			name:    "redirect scheme downgrade",
			row:     "Redirect scheme downgrade from https to http",
			pattern: `"Create paste error: redirect scheme downgrade from https to http blocked; use an https server URL: <request URL>"`,
		},
		{
			name:    "too many redirects",
			row:     "Too many redirects (>10)",
			pattern: `"Create paste error: stopped after 10 redirects; ask the user to check the server URL for a redirect loop: <request URL>"`,
		},
		{
			name:    "file read disabled",
			row:     "File read disabled by configuration",
			pattern: `"Create paste error: file read is disabled by configuration; use inline content instead; do not attempt again"`,
		},
		{
			name:    "path traversal",
			row:     "File path rejected by traversal detection",
			pattern: `"Create paste error: path traversal is not allowed and will always be rejected; do not attempt again"`,
		},
		{
			name:    "file path not under allowed path",
			row:     "File path rejected by allowlist",
			pattern: `"Create paste error: file path is not under any configured allowed path; ask the user to check ALLOWED_PATHS if this path should be accessible"`,
		},
		{
			name:    "blocked prefix",
			row:     "File path rejected by built-in blocklist (system prefix)",
			pattern: `"Create paste error: file path is in a blocked system directory and will always be rejected; do not attempt again: <prefix>"`,
		},
		{
			name:    "blocked component",
			row:     "File path rejected by built-in blocklist (sensitive component)",
			pattern: `"Create paste error: file path contains a blocked component and will always be rejected; do not attempt again: <component>"`,
		},
		{
			name:    "user blocked path",
			row:     "File path rejected by user blocklist",
			pattern: `"Create paste error: file path is in a user-blocked directory and will always be rejected; do not attempt again"`,
		},
		{
			name:    "file not text",
			row:     "File is binary or non-UTF-8",
			pattern: `"Create paste error: file is binary or not valid UTF-8 text and cannot be uploaded; do not attempt again for this file"`,
		},
		{
			name:    "path not found",
			row:     "File does not exist",
			pattern: `"Create paste error: the specified file does not exist; verify the path is correct and do not attempt the same path again: <details>"`,
		},
		{
			name:    "path permission denied",
			row:     "Permission denied accessing file path",
			pattern: `"Create paste error: the file exists but is not readable; ask the user to check file permissions: <details>"`,
		},
		{
			name:    "file path cannot be used",
			row:     "File cannot be read (symlink error, other I/O error, non-regular file)",
			pattern: `"Create paste error: file path cannot be used; ask the user to check the path or file permissions[: <details>]"`,
		},
		{
			name:    "sandbox no mounts",
			row:     "Sandbox translation requested but no mounts configured",
			pattern: `"Create paste error: sandbox path translation was requested but no sandbox mounts are configured; ask the user to check WASTEBIN_MCP_SANDBOX_MOUNTS if translation should be enabled"`,
		},
		{
			name:    "sandbox no match",
			row:     "Sandbox path does not match any configured mount",
			pattern: `"Create paste error: sandbox path does not match any configured mount; ask the user to check the sandbox mount configuration: <path>"`,
		},
	}
}

// TestDocsMCPTools_ErrorRows guards the documentation against drift: every
// handler-error row documented in docs/MCP_TOOLS.md must exist with its exact
// "Error Condition" label and a "Message Pattern" that carries the complete
// documented wording, including its <placeholder> markers, quoting, and Markdown
// escaping. Matching is row-scoped (not a whole-file search), so deleting a row,
// regressing a placeholder to literal syntax, dropping a value placeholder, or
// reordering/requoting the pattern all fail instead of silently passing.
func TestDocsMCPTools_ErrorRows(t *testing.T) {
	t.Parallel()

	docsPath := filepath.Join("..", "..", "docs", "MCP_TOOLS.md")

	//nolint:gosec // fixed relative path to the repo's own docs file
	data, err := os.ReadFile(docsPath)
	if err != nil {
		t.Fatalf("read %s: %v", docsPath, err)
	}

	rows := parseErrorDocRows(string(data))

	for _, tc := range errorDocCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pattern, ok := rows[tc.row]
			if !ok {
				t.Errorf("docs/MCP_TOOLS.md is missing the error row %q", tc.row)

				return
			}

			if !strings.Contains(pattern, tc.pattern) {
				t.Errorf(
					"docs/MCP_TOOLS.md row %q does not document pattern %q; row documents %q",
					tc.row, tc.pattern, pattern,
				)
			}
		})
	}
}

// parseErrorDocRows extracts the "Message Pattern" cell of every table row in
// docs/MCP_TOOLS.md, keyed by its "Error Condition" cell. Cells are normalized
// (Markdown code-span backticks stripped, backslash escapes resolved) so they
// match the runtime wording.
func parseErrorDocRows(docs string) map[string]string {
	rows := make(map[string]string)

	for line := range strings.SplitSeq(docs, "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 4 {
			continue
		}

		condition := strings.TrimSpace(cells[1])
		pattern := strings.TrimSpace(cells[2])

		if condition == "" || pattern == "" || pattern == "---" {
			continue
		}

		rows[condition] = normalizeDocCell(pattern)
	}

	return rows
}

// normalizeDocCell strips Markdown code-span backticks and resolves backslash
// escapes used inside code spans, so the cell matches the runtime message
// wording (e.g. the invalid-extension row escapes a literal backslash as `\\`
// in the source). Backticks only delimit code spans in these tables; no
// documented message contains a literal backtick.
func normalizeDocCell(s string) string {
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, `\\`, `\`)
	s = strings.ReplaceAll(s, `\"`, `"`)

	return s
}
