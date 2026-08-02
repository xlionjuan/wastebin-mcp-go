package wastebin //nolint:testpackage // contract test produces real runtime errors through unexported code

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// Error sentinels used by the docs-contract fixture to drive the real runtime
// error paths. They are static test errors, following the "wrapped static
// errors" guidance that err113 enforces for errors.New at call sites.
var (
	errDocConnRefused = errors.New("connection refused")
	errDocTLSFailure  = errors.New("tls: handshake failure")
	errDocReadFailure = errors.New("connection reset by peer")
)

// docReplace pairs a concrete diagnostic value that a produced runtime error
// contains with the documented <placeholder> marker that stands for it in
// docs/MCP_TOOLS.md.
type docReplace struct {
	concrete string
	marker   string
}

// errorDocCase connects one handler-error row documented in docs/MCP_TOOLS.md
// to the real runtime error that produces it. produce() runs the actual code
// path and returns the error together with the concrete diagnostic values to
// replace with their documented <placeholder> markers. The documented pattern
// is then derived from the runtime message, so a runtime wording change that is
// reflected in the unit tests but forgotten in the docs fails the contract
// instead of silently passing.
type errorDocCase struct {
	name    string
	row     string
	produce func() ([]docReplace, error)
}

// pattern derives the documented "Message Pattern" cell from the real runtime
// error: the concrete diagnostic values are replaced with their documented
// <placeholder> markers, and the handler message is wrapped in the
// "Create paste error: " prefix plus the quotes used by the docs tables. A
// concrete value that no longer appears in the runtime message means the
// runtime wording changed and the fixture must be updated — that is a loud
// panic, not a silent pass.
func (c errorDocCase) pattern() string {
	replaces, err := c.produce()
	msg := err.Error()

	for _, r := range replaces {
		replaced := strings.Replace(msg, r.concrete, r.marker, 1)
		if replaced == msg {
			panic(fmt.Sprintf(
				"%s: concrete diagnostic %q not found in runtime message %q; "+
					"the runtime wording changed and the fixture must be updated",
				c.name, r.concrete, msg,
			))
		}

		msg = replaced
	}

	return `"Create paste error: ` + msg + `"`
}

// sentinelProducer returns a produce function for an error that carries no
// dynamic diagnostic value.
func sentinelProducer(err error) func() ([]docReplace, error) {
	return func() ([]docReplace, error) {
		return nil, err
	}
}

// docsServerURL and docsPostURL are the fake server the docs-contract fixture
// drives the real sendRequest error paths against, without any network
// connection. postURL matches what NewWastebinClient computes
// (baseURL.JoinPath("/")).
const docsServerURL = "http://example.com"

var docsPostURL = mustURL(docsServerURL).JoinPath("/").String()

// mustURL parses a URL or panics; it is only used for the fixed fixture URLs.
func mustURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}

	return u
}

// docsRoundTripper is a RoundTripper returning a fixed response or error, so
// the real sendRequest error paths are exercised deterministically.
type docsRoundTripper struct {
	status int
	body   io.ReadCloser
	err    error
}

func (d *docsRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	if d.err != nil {
		return nil, d.err
	}

	return &http.Response{
		StatusCode: d.status,
		Header:     make(http.Header),
		Body:       d.body,
	}, nil
}

// failReader is an io.Reader whose Read always fails.
type failReader struct{ err error }

func (r *failReader) Read([]byte) (int, error) {
	return 0, r.err
}

// sendRequestError runs sendRequest against a Handler whose HTTP client uses
// the given RoundTripper and returns the produced error.
func sendRequestError(rt http.RoundTripper) error {
	h := &Handler{
		baseURL:    mustURL(docsServerURL),
		httpClient: &http.Client{Transport: rt},
		config:     &Config{},
		postURL:    docsPostURL,
	}

	_, err := h.sendRequest(context.Background(), []byte(`{"text":"hello"}`))

	return err
}

// wrappedTransportError reproduces the *url.Error shape net/http's Client.Do
// returns for a RoundTripper failure, so the concrete diagnostic tail that
// appears in the produced message can be derived.
func wrappedTransportError(underlying error) error {
	return &url.Error{Op: "Post", URL: docsPostURL, Err: underlying}
}

// errorDocCases returns one case per handler-error row documented in
// docs/MCP_TOOLS.md (the "Always applicable", "File mode errors", and "Sandbox
// errors" tables), spanning every handler-error family documented there. Each
// case's pattern is derived from a real runtime error produced by the actual
// code path.
//
//nolint:maintidx // large row-scoped fixture spanning every documented handler-error row
func errorDocCases() []errorDocCase {
	return []errorDocCase{
		// Always applicable (regardless of configuration).
		{
			name:    "both content and file_path",
			row:     "Both `content` and `file_path` provided (file mode enabled)",
			produce: sentinelProducer(errBothContentAndFilePath),
		},
		{
			name:    "neither content nor file_path",
			row:     "Neither `content` nor `file_path` provided (file mode enabled)",
			produce: sentinelProducer(errNeitherContentNorFilePath),
		},
		{
			name:    "empty content",
			row:     "`content` is empty (content mode)",
			produce: sentinelProducer(errContentEmpty),
		},
		{
			name: "HTTP 403",
			row:  "HTTP 403 from server",
			produce: func() ([]docReplace, error) {
				return nil, newHTTPError(http.StatusForbidden, "forbidden body")
			},
		},
		{
			name: "HTTP 413",
			row:  "HTTP 413 from server",
			produce: func() ([]docReplace, error) {
				return nil, newHTTPError(http.StatusRequestEntityTooLarge, "too large")
			},
		},
		{
			name: "connection refused or timeout",
			row:  "Connection refused / timeout",
			produce: func() ([]docReplace, error) {
				underlying := &net.OpError{Op: "dial", Net: "tcp", Err: errDocConnRefused}
				err := sendRequestError(&docsRoundTripper{err: underlying})

				return []docReplace{{concrete: wrappedTransportError(underlying).Error(), marker: "<details>"}}, err
			},
		},
		{
			name: "DNS resolution failure",
			row:  "DNS resolution failure",
			produce: func() ([]docReplace, error) {
				underlying := &net.DNSError{Err: "no such host", Name: "example.com"}
				err := sendRequestError(&docsRoundTripper{err: underlying})

				return []docReplace{{concrete: wrappedTransportError(underlying).Error(), marker: "<details>"}}, err
			},
		},
		{
			name: "other HTTP request failure",
			row:  "Other HTTP request failure (e.g. TLS, proxy, unexpected transport error)",
			produce: func() ([]docReplace, error) {
				underlying := errDocTLSFailure
				err := translateRequestError(wrappedTransportError(underlying))

				return []docReplace{{concrete: wrappedTransportError(underlying).Error(), marker: "<details>"}}, err
			},
		},
		{
			name: "content too large",
			row:  "Content exceeds `WASTEBIN_MCP_MAX_CONTENT_SIZE`",
			produce: func() ([]docReplace, error) {
				return []docReplace{
					{concrete: "10 bytes exceeds limit of 5 bytes", marker: "<size> bytes exceeds limit of <limit> bytes"},
				}, newContentTooLargeError(10, 5)
			},
		},
		{
			name:    "password over HTTP",
			row:     "Password over non-loopback HTTP without override",
			produce: sentinelProducer(errPasswordOverHTTP),
		},
		{
			name:    "password empty",
			row:     "`password` is empty string (defensive fallback, `minLength` should catch at schema first)",
			produce: sentinelProducer(errPasswordEmpty),
		},
		{
			name: "unknown HTTP error",
			row:  "Unknown HTTP error",
			produce: func() ([]docReplace, error) {
				return []docReplace{{concrete: "HTTP 500", marker: "HTTP <code>"}},
					newHTTPError(http.StatusInternalServerError, "internal error")
			},
		},
		{
			name: "expiration negative",
			row:  "Expiration is negative",
			produce: func() ([]docReplace, error) {
				return nil, ValidateExpiration(-1)
			},
		},
		{
			name: "expiration unit unknown",
			row:  "Expiration unit is unknown",
			produce: func() ([]docReplace, error) {
				_, err := ParseExpiration("5x", 0)

				return []docReplace{{concrete: `"x"`, marker: `"<unit>"`}}, err
			},
		},
		{
			name: "expiration format invalid",
			row:  "Expiration format is invalid",
			produce: func() ([]docReplace, error) {
				_, err := ParseExpiration("abc", 0)

				return []docReplace{{concrete: `"abc"`, marker: `"<value>"`}}, err
			},
		},
		{
			name: "expiration overflow",
			row:  "Expiration value overflows",
			produce: func() ([]docReplace, error) {
				_, err := ParseExpiration("999999999999999999y", 0)

				return nil, err
			},
		},
		{
			name: "expiration too large",
			row:  "Expiration exceeds the maximum supported value",
			produce: func() ([]docReplace, error) {
				err := ValidateExpiration(maxExpirationSeconds + 1)
				concrete := strconv.Itoa(maxExpirationSeconds+1) + " seconds exceeds maximum of " +
					strconv.Itoa(maxExpirationSeconds) + " seconds"
				marker := "<seconds> seconds exceeds maximum of " + strconv.Itoa(maxExpirationSeconds) + " seconds"

				return []docReplace{{concrete: concrete, marker: marker}}, err
			},
		},
		{
			name: "expiration number invalid",
			row:  "Expiration number is not numeric",
			produce: func() ([]docReplace, error) {
				_, atoiErr := strconv.Atoi("-")
				_, err := ParseExpiration("-", 0)

				return []docReplace{{concrete: atoiErr.Error(), marker: "<details>"}}, err
			},
		},
		{
			name: "invalid extension",
			row:  "Extension contains invalid path or query characters",
			produce: func() ([]docReplace, error) {
				return []docReplace{{concrete: `"a/b"`, marker: `"<extension>"`}}, newInvalidExtensionError("a/b")
			},
		},
		{
			name: "empty response path",
			row:  "Server returns response with empty paste path",
			produce: func() ([]docReplace, error) {
				return nil, validateWastebinResponse(wastebinResponse{Path: ""})
			},
		},
		{
			name: "non-relative response path",
			row:  "Server returns response with non-relative paste path",
			produce: func() ([]docReplace, error) {
				return []docReplace{{concrete: `"abc"`, marker: `"<path>"`}},
					validateWastebinResponse(wastebinResponse{Path: "abc"})
			},
		},
		{
			name: "response missing paste ID",
			row:  "Server returns response without paste ID",
			produce: func() ([]docReplace, error) {
				return nil, validateWastebinResponse(wastebinResponse{Path: "/"})
			},
		},
		{
			name: "malformed JSON",
			row:  "Server returns malformed JSON",
			produce: func() ([]docReplace, error) {
				body := `{invalid json`
				decErr := json.NewDecoder(strings.NewReader(body)).Decode(new(wastebinResponse))
				err := sendRequestError(&docsRoundTripper{
					status: http.StatusOK,
					body:   io.NopCloser(strings.NewReader(body)),
				})

				return []docReplace{{concrete: decErr.Error(), marker: "<details>"}}, err
			},
		},
		{
			name: "failed to read response body",
			row:  "Failed to read the Wastebin response body",
			produce: func() ([]docReplace, error) {
				readErr := errDocReadFailure
				err := sendRequestError(&docsRoundTripper{
					status: http.StatusOK,
					body:   io.NopCloser(&failReader{err: readErr}),
				})

				return []docReplace{{concrete: readErr.Error(), marker: "<details>"}}, err
			},
		},
		{
			name: "response too large",
			row:  "Wastebin response exceeds maximum allowed size",
			produce: func() ([]docReplace, error) {
				body := strings.Repeat("A", maxResponseBodyLength+1)
				err := sendRequestError(&docsRoundTripper{
					status: http.StatusOK,
					body:   io.NopCloser(strings.NewReader(body)),
				})

				return nil, err
			},
		},
		{
			name: "trailing content after JSON",
			row:  "Server returns response with trailing non-whitespace content",
			produce: func() ([]docReplace, error) {
				body := `{"path":"/abc"}extra`
				err := sendRequestError(&docsRoundTripper{
					status: http.StatusOK,
					body:   io.NopCloser(strings.NewReader(body)),
				})

				return nil, err
			},
		},
		{
			name: "HTTP 422 with body",
			row:  "HTTP 422 from server (with body)",
			produce: func() ([]docReplace, error) {
				body := "expiration value is invalid"
				err := translateHTTPError(http.StatusUnprocessableEntity, body)

				return []docReplace{{concrete: body, marker: "<details>"}}, err
			},
		},
		{
			name: "HTTP 422 empty body",
			row:  "HTTP 422 from server (empty body)",
			produce: func() ([]docReplace, error) {
				return nil, translateHTTPError(http.StatusUnprocessableEntity, "")
			},
		},
		{
			name: "redirect to different host",
			row:  "Cross-host redirect blocked",
			produce: func() ([]docReplace, error) {
				redirectURL := "http://evil.example.com/malicious"
				err := translateRequestError(&url.Error{Op: "Post", URL: redirectURL, Err: errRedirectDifferentHost})

				return []docReplace{{concrete: redirectURL, marker: "<request URL>"}}, err
			},
		},
		{
			name: "redirect scheme downgrade",
			row:  "Redirect scheme downgrade from https to http",
			produce: func() ([]docReplace, error) {
				redirectURL := "http://bin.example.com/redirect"
				err := translateRequestError(&url.Error{Op: "Post", URL: redirectURL, Err: errRedirectSchemeDowngrade})

				return []docReplace{{concrete: redirectURL, marker: "<request URL>"}}, err
			},
		},
		{
			name: "too many redirects",
			row:  "Too many redirects (>10)",
			produce: func() ([]docReplace, error) {
				redirectURL := "http://bin.example.com/redirect"
				err := translateRequestError(&url.Error{Op: "Post", URL: redirectURL, Err: errTooManyRedirects})

				return []docReplace{{concrete: redirectURL, marker: "<request URL>"}}, err
			},
		},

		// File mode errors (only when WASTEBIN_MCP_FILE_READ_ENABLED=true).
		{
			name:    "file read disabled",
			row:     "File read disabled by configuration",
			produce: sentinelProducer(errFileReadDisabled),
		},
		{
			name:    "path traversal",
			row:     "File path rejected by traversal detection",
			produce: sentinelProducer(errPathTraversal),
		},
		{
			name:    "file path not under allowed path",
			row:     "File path rejected by allowlist",
			produce: sentinelProducer(errPathNotAllowed),
		},
		{
			name: "blocked prefix",
			row:  "File path rejected by built-in blocklist (system prefix)",
			produce: func() ([]docReplace, error) {
				return []docReplace{{concrete: "/etc", marker: "<prefix>"}}, newBlockedPrefixError("/etc")
			},
		},
		{
			name: "blocked component",
			row:  "File path rejected by built-in blocklist (sensitive component)",
			produce: func() ([]docReplace, error) {
				return []docReplace{{concrete: ".ssh", marker: "<component>"}}, newBlockedComponentError(".ssh")
			},
		},
		{
			name:    "user blocked path",
			row:     "File path rejected by user blocklist",
			produce: sentinelProducer(errUserBlockedPath),
		},
		{
			name:    "file not text",
			row:     "File is binary or non-UTF-8",
			produce: sentinelProducer(errFileNotText),
		},
		{
			name: "path not found",
			row:  "File does not exist",
			produce: func() ([]docReplace, error) {
				underlying := syscall.ENOENT

				return []docReplace{{concrete: underlying.Error(), marker: "<details>"}},
					newPathNotFoundError(underlying)
			},
		},
		{
			name: "path permission denied",
			row:  "Permission denied accessing file path",
			produce: func() ([]docReplace, error) {
				underlying := syscall.EACCES

				return []docReplace{{concrete: underlying.Error(), marker: "<details>"}},
					newPathPermissionError(underlying)
			},
		},
		{
			name: "file cannot be read with underlying error",
			row:  "File cannot be read (symlink error, other I/O error)",
			produce: func() ([]docReplace, error) {
				underlying := syscall.EACCES

				return []docReplace{{concrete: underlying.Error(), marker: "<details>"}},
					fmt.Errorf("%w: %w", errFilePathCannotBeUsed, underlying)
			},
		},
		{
			name:    "file is not a regular file",
			row:     "File is not a regular file",
			produce: sentinelProducer(errFilePathCannotBeUsed),
		},

		// Sandbox errors (only when sandbox mounts are configured).
		{
			name:    "sandbox no mounts",
			row:     "Sandbox translation requested but no mounts configured",
			produce: sentinelProducer(errSandboxTranslationNoMounts),
		},
		{
			name: "sandbox no match",
			row:  "Sandbox path does not match any configured mount",
			produce: func() ([]docReplace, error) {
				return []docReplace{{concrete: "/workspace/report.md", marker: "<path>"}},
					newSandboxNoMatchError("/workspace/report.md")
			},
		},
	}
}

// TestDocsMCPTools_ErrorRows guards the documentation against drift: every
// handler-error row documented in docs/MCP_TOOLS.md must exist with its exact
// "Error Condition" label and a "Message Pattern" cell that equals the complete
// message produced by the real runtime error, with only the concrete diagnostic
// values replaced by their documented <placeholder> markers. Each expected
// pattern is derived from runtime output (see errorDocCase.pattern), so a
// runtime wording change that updates the unit tests but forgets the docs fails
// the contract; deleting a row, regressing a placeholder to literal syntax,
// dropping a value placeholder, or reordering/requoting the pattern all fail
// too.
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

			want := tc.pattern()
			if pattern != want {
				t.Errorf(
					"docs/MCP_TOOLS.md row %q documents %q, want the runtime-derived pattern %q",
					tc.row, pattern, want,
				)
			}
		})
	}
}

// parseErrorDocRows extracts the "Message Pattern" cell of every table row in
// docs/MCP_TOOLS.md, keyed by its "Error Condition" cell. Pattern cells are
// normalized for Markdown code-span syntax (backticks stripped) so they match
// the runtime wording; no other transformation is applied because Markdown code
// spans do not process backslash escapes or other inline markup.
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

// normalizeDocCell strips the Markdown code-span backticks that delimit the
// pattern cells. Backticks are the only syntax Markdown actually transforms
// inside these cells: code spans do not process backslash escapes, so a single
// literal backslash in the cell (e.g. the invalid-extension row) stays as-is
// and must match the runtime message verbatim.
func normalizeDocCell(s string) string {
	return strings.ReplaceAll(s, "`", "")
}
