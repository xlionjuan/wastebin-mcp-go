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
	"reflect"
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
	errDocDialTimeout = context.DeadlineExceeded
	errDocTLSFailure  = errors.New("tls: handshake failure")
	errDocReadFailure = errors.New("connection reset by peer")

	// errDocDuplicateRow wraps the duplicate handler-error row message, so the
	// parse error wraps a static error per the err113 rule.
	errDocDuplicateRow = errors.New("duplicate error row in the handler-error tables")
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

// errorCase returns a case whose produce returns the given error with the given
// concrete→marker replacements applied. It is the compact form for errors
// constructed directly (sentinels and typed errors).
func errorCase(name, row string, replaces []docReplace, err error) errorDocCase {
	return errorDocCase{
		name: name,
		row:  row,
		produce: func() ([]docReplace, error) {
			return replaces, err
		},
	}
}

// mustParseExpirationErr parses a fixed fixture expiration string and returns
// the resulting error, panicking if parsing unexpectedly succeeds. It is only
// used to build errorDocCases from the real ParseExpiration path.
func mustParseExpirationErr(s string, def int) error {
	_, err := ParseExpiration(s, def)
	if err == nil {
		panic("fixture expiration input unexpectedly parsed: " + s)
	}

	return err
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

// dialErrorCase drives the real sendRequest path with a dial-level net.OpError
// (connection refused or dial timeout) and asserts it is routed to the
// "cannot connect to Wastebin server" message rather than the generic HTTP
// request failure.
func dialErrorCase(name, row string, dialErr error) errorDocCase {
	return errorDocCase{
		name: name,
		row:  row,
		produce: func() ([]docReplace, error) {
			underlying := &net.OpError{Op: "dial", Net: "tcp", Err: dialErr}
			err := sendRequestError(&docsRoundTripper{err: underlying})

			return []docReplace{{concrete: wrappedTransportError(underlying).Error(), marker: "<details>"}}, err
		},
	}
}

// redirectCase exercises translateRequestError with one of the three redirect
// sentinels in the *url.Error shape net/http produces, and asserts the request
// URL appears only as a ": <request URL>" suffix.
func redirectCase(name, row, redirectURL string, sentinel error) errorDocCase {
	return errorDocCase{
		name: name,
		row:  row,
		produce: func() ([]docReplace, error) {
			err := translateRequestError(&url.Error{Op: "Post", URL: redirectURL, Err: sentinel})

			return []docReplace{{concrete: redirectURL, marker: "<request URL>"}}, err
		},
	}
}

// errorDocCases returns a representative subset of the handler-error rows
// documented in docs/MCP_TOOLS.md (the "Always applicable", "File mode errors",
// and "Sandbox errors" tables). One or more cases are kept per error-shape
// family — transport routing, HTTP status mapping, typed errors, diagnostic
// suffixes, response validation/processing, redirects, and file/sandbox errors
// — and every case's pattern is derived from a real runtime error produced by
// the actual code path. Some cases duplicate rows already pinned verbatim by
// the exact-output unit tests (errors_test.go, handler_test.go,
// expiration_test.go); those duplicates deliberately exercise placeholder
// normalization, turning a concrete diagnostic value into its documented
// <placeholder> marker. This is a representative contract, not an exhaustive
// second copy of the error tables.
func errorDocCases() []errorDocCase {
	return []errorDocCase{
		// Transport routing, through the real sendRequest/translateRequestError
		// paths. Connection refused and dial timeouts share one row and both
		// must be routed to the "cannot connect" message.
		dialErrorCase("connection refused", "Connection refused / dial timeout", errDocConnRefused),
		dialErrorCase("dial timeout", "Connection refused / dial timeout", errDocDialTimeout),
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

		// HTTP status mapping through the typed HTTPError.
		errorCase("HTTP 403", "HTTP 403 from server", nil, newHTTPError(http.StatusForbidden, "forbidden body")),
		errorCase(
			"HTTP 422 with body", "HTTP 422 from server (with body)",
			[]docReplace{{concrete: "expiration value is invalid", marker: "<details>"}},
			translateHTTPError(http.StatusUnprocessableEntity, "expiration value is invalid"),
		),
		errorCase(
			"unknown HTTP error", "Unknown HTTP error",
			[]docReplace{{concrete: "HTTP 500", marker: "HTTP <code>"}},
			newHTTPError(http.StatusInternalServerError, "internal error"),
		),

		// Typed errors carrying structured data.
		errorCase(
			"content too large", "Content exceeds `WASTEBIN_MCP_MAX_CONTENT_SIZE`",
			[]docReplace{{concrete: "10 bytes exceeds limit of 5 bytes", marker: "<size> bytes exceeds limit of <limit> bytes"}},
			newContentTooLargeError(10, 5),
		),
		errorCase(
			"invalid extension", "Extension contains invalid path or query characters",
			[]docReplace{{concrete: `"a/b"`, marker: `"<extension>"`}},
			newInvalidExtensionError("a/b"),
		),
		errorCase(
			"blocked prefix", "File path rejected by built-in blocklist (system prefix)",
			[]docReplace{{concrete: "/etc", marker: "<prefix>"}},
			newBlockedPrefixError("/etc"),
		),
		errorCase(
			"sandbox no match", "Sandbox path does not match any configured mount",
			[]docReplace{{concrete: "/workspace/report.md", marker: "<path>"}},
			newSandboxNoMatchError("/workspace/report.md"),
		),

		// Plain sentinel with no dynamic value.
		errorCase("content is empty", "`content` is empty (content mode)", nil, errContentEmpty),

		// Expiration family: diagnostic values as %q-quoted and numeric
		// suffixes.
		errorCase(
			"expiration unit unknown", "Expiration unit is unknown",
			[]docReplace{{concrete: `"x"`, marker: `"<unit>"`}},
			mustParseExpirationErr("5x", 0),
		),
		errorCase(
			"expiration exceeds maximum", "Expiration exceeds the maximum supported value",
			[]docReplace{{
				concrete: strconv.Itoa(maxExpirationSeconds+1) + " seconds exceeds maximum of " +
					strconv.Itoa(maxExpirationSeconds) + " seconds",
				marker: "<seconds> seconds exceeds maximum of " + strconv.Itoa(maxExpirationSeconds) + " seconds",
			}},
			ValidateExpiration(maxExpirationSeconds+1),
		),

		// Wastebin response validation and processing through the real
		// sendRequest path.
		errorCase(
			"empty response path", "Server returns response with empty paste path",
			nil, validateWastebinResponse(wastebinResponse{Path: ""}),
		),
		errorCase(
			"non-relative response path", "Server returns response with non-relative paste path",
			[]docReplace{{concrete: `"abc"`, marker: `"<path>"`}},
			validateWastebinResponse(wastebinResponse{Path: "abc"}),
		),
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

		// Redirect family: the request URL appears only as a ": <request URL>"
		// suffix.
		redirectCase(
			"redirect to different host", "Cross-host redirect blocked",
			"http://evil.example.com/malicious", errRedirectDifferentHost,
		),
		redirectCase(
			"redirect scheme downgrade", "Redirect scheme downgrade from https to http",
			"http://bin.example.com/redirect", errRedirectSchemeDowngrade,
		),
		redirectCase(
			"too many redirects", "Too many redirects (>10)",
			"http://bin.example.com/redirect", errTooManyRedirects,
		),

		// File mode errors.
		errorCase("file not text", "File is binary or non-UTF-8", nil, errFileNotText),
		errorCase(
			"path not found", "File does not exist",
			[]docReplace{{concrete: syscall.ENOENT.Error(), marker: "<details>"}},
			newPathNotFoundError(syscall.ENOENT),
		),
		errorCase(
			"file cannot be read with underlying error", "File cannot be read (symlink error, other I/O error)",
			[]docReplace{{concrete: syscall.EACCES.Error(), marker: "<details>"}},
			wrapFilePathCannotBeUsed(syscall.EACCES),
		),

		// Sandbox errors.
		errorCase(
			"sandbox no mounts", "Sandbox translation requested but no mounts configured",
			nil, errSandboxTranslationNoMounts,
		),
	}
}

// TestDocsMCPTools_ErrorRows guards the documentation against drift: every
// representative error row (see errorDocCases) must exist in the
// "Handler-Generated Errors" section of docs/MCP_TOOLS.md with its exact
// "Error Condition" label and a "Message Pattern" cell that equals the complete
// message produced by the real runtime error, with only the concrete diagnostic
// values replaced by their documented <placeholder> markers. Each expected
// pattern is derived from runtime output (see errorDocCase.pattern), so a
// runtime wording change that updates the unit tests but forgets the docs fails
// the contract; deleting a row, regressing a placeholder to literal syntax,
// dropping a value placeholder, or reordering/requoting the pattern all fail
// too. The fixture is a representative subset of the documented rows, not an
// exhaustive duplicate of the error tables; cases that overlap the exact-output
// unit tests do so deliberately, exercising placeholder normalization on real
// runtime output. The parser guardrails behind this test are covered by
// TestParseErrorDocRows_Behavior.
func TestDocsMCPTools_ErrorRows(t *testing.T) {
	t.Parallel()

	docsPath := filepath.Join("..", "..", "docs", "MCP_TOOLS.md")

	//nolint:gosec // fixed relative path to the repo's own docs file
	data, err := os.ReadFile(docsPath)
	if err != nil {
		t.Fatalf("read %s: %v", docsPath, err)
	}

	rows, err := parseErrorDocRows(string(data))
	if err != nil {
		t.Fatalf("parse docs/MCP_TOOLS.md error tables: %v", err)
	}

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

// handlerErrorsHeading is the Markdown heading that begins the handler-generated
// error tables in docs/MCP_TOOLS.md. Rows are only extracted between this
// heading and the next heading at the same level, so unrelated tables elsewhere
// in the document cannot collide with or mask the handler-error rows.
const handlerErrorsHeading = "#### Handler-Generated Errors"

// parseErrorDocRows extracts the "Message Pattern" cell of every row in the
// handler-error tables (under the "#### Handler-Generated Errors" heading in
// docs/MCP_TOOLS.md), keyed by its "Error Condition" cell. Pattern cells are
// normalized for Markdown code-span syntax (backticks stripped) so they match
// the runtime wording; no other transformation is applied because Markdown code
// spans do not process backslash escapes or other inline markup. A duplicate
// condition label within the section is an error instead of a silent map
// overwrite, so a same-named row cannot mask the intended handler row.
func parseErrorDocRows(docs string) (map[string]string, error) {
	rows := make(map[string]string)

	inSection := false

	for line := range strings.SplitSeq(docs, "\n") {
		if strings.HasPrefix(line, "#### ") {
			if strings.HasPrefix(line, handlerErrorsHeading) {
				inSection = true
			} else if inSection {
				break
			}

			continue
		}

		if !inSection {
			continue
		}

		cells := strings.Split(line, "|")
		if len(cells) < 4 {
			continue
		}

		condition := strings.TrimSpace(cells[1])
		pattern := strings.TrimSpace(cells[2])

		if condition == "" || pattern == "" || pattern == "---" || condition == "Error Condition" {
			continue
		}

		if _, dup := rows[condition]; dup {
			return nil, fmt.Errorf("%w: %q", errDocDuplicateRow, condition)
		}

		rows[condition] = normalizeDocCell(pattern)
	}

	return rows, nil
}

// normalizeDocCell strips the Markdown code-span backticks that delimit the
// pattern cells. Backticks are the only syntax Markdown actually transforms
// inside these cells: code spans do not process backslash escapes, so a single
// literal backslash in the cell (e.g. the invalid-extension row) stays as-is
// and must match the runtime message verbatim.
func normalizeDocCell(s string) string {
	return strings.ReplaceAll(s, "`", "")
}

// TestParseErrorDocRows_Behavior covers the guardrails of parseErrorDocRows
// that keep the docs contract from going false-green: a row sharing a condition
// label with a handler-error row but living outside the handler-error section
// must be ignored (so an unrelated table cannot mask or collide with the
// intended row), a duplicate condition label inside the section must fail
// closed instead of silently overwriting the first entry, and extraction must
// stop at the next heading at the same level. These are behavioral tests for
// the contract-test machinery, not coverage-only tests.
func TestParseErrorDocRows_Behavior(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		docs string
		want map[string]string
		err  error
	}{
		{
			name: "same-named row outside the handler section is ignored",
			docs: "" +
				"#### Parameters\n" +
				"\n" +
				"| Error Condition | Message Pattern |\n" +
				"|---|---|\n" +
				"| same row | outside |\n" +
				"\n" +
				"#### Handler-Generated Errors\n" +
				"\n" +
				"| Error Condition | Message Pattern |\n" +
				"|---|---|\n" +
				"| same row | inside |\n",
			want: map[string]string{"same row": "inside"},
		},
		{
			name: "duplicate row inside the section fails closed",
			docs: "" +
				"#### Handler-Generated Errors\n" +
				"\n" +
				"| Error Condition | Message Pattern |\n" +
				"|---|---|\n" +
				"| same row | first |\n" +
				"| same row | second |\n",
			err: errDocDuplicateRow,
		},
		{
			name: "parsing stops at the next heading",
			docs: "" +
				"#### Handler-Generated Errors\n" +
				"\n" +
				"| Error Condition | Message Pattern |\n" +
				"|---|---|\n" +
				"| keep me | pattern |\n" +
				"\n" +
				"#### Unrelated Section\n" +
				"\n" +
				"| Error Condition | Message Pattern |\n" +
				"|---|---|\n" +
				"| drop me | pattern |\n",
			want: map[string]string{"keep me": "pattern"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rows, err := parseErrorDocRows(tc.docs)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("parseErrorDocRows() error = %v, want errors.Is(err, %v)", err, tc.err)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseErrorDocRows() unexpected error: %v", err)
			}

			if !reflect.DeepEqual(rows, tc.want) {
				t.Errorf("parseErrorDocRows() = %v, want %v", rows, tc.want)
			}
		})
	}
}
