package wastebin //nolint:testpackage // contract tests need access to unexported sentinels

import (
	"errors"
	"fmt"
	"net/http"
	"syscall"
	"testing"
)

// TestTypedErrors_ExactOutput pins the exact Error() output of every typed
// error so the message contract is enforced by tests instead of hand-copied
// documentation (see docs/adr/005-error-model.md).
func TestTypedErrors_ExactOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "ContentTooLargeError",
			err:  NewContentTooLargeError(10, 5),
			want: "content exceeds the maximum allowed size; " +
				"split the content into smaller parts and upload each separately: " +
				"10 bytes exceeds limit of 5 bytes",
		},
		{
			name: "InvalidExtensionError",
			err:  NewInvalidExtensionError("a/b"),
			want: `extension contains invalid characters (/, \, ?, #); ` +
				`use a plain extension like 'go' or 'py': "a/b"`,
		},
		{
			name: "PathNotFoundError",
			err:  NewPathNotFoundError(syscall.ENOENT),
			want: "the specified file does not exist; verify the path is correct " +
				"and do not attempt the same path again: " + syscall.ENOENT.Error(),
		},
		{
			name: "PathPermissionError",
			err:  NewPathPermissionError(syscall.EACCES),
			want: "the file exists but is not readable; ask the user to check " +
				"file permissions: " + syscall.EACCES.Error(),
		},
		{
			name: "SandboxNoMatchError",
			err:  NewSandboxNoMatchError("/workspace/report.md"),
			want: "sandbox path does not match any configured mount; " +
				"ask the user to check the sandbox mount configuration: " +
				"/workspace/report.md",
		},
		{
			name: "BlockedComponentError",
			err:  NewBlockedComponentError(".ssh"),
			want: "file path contains a blocked component and will always be " +
				"rejected. Do not attempt again: .ssh",
		},
		{
			name: "HTTPError forbidden",
			err:  NewHTTPError(http.StatusForbidden, "forbidden body"),
			want: "server rejected the request; content may contain disallowed data; " +
				"ask the user to check the content or the server logs",
		},
		{
			name: "HTTPError request entity too large",
			err:  NewHTTPError(http.StatusRequestEntityTooLarge, "too large"),
			want: "content exceeds the server's maximum allowed size; " +
				"split the content into smaller parts and upload each separately",
		},
		{
			name: "HTTPError unprocessable entity with body",
			err:  NewHTTPError(http.StatusUnprocessableEntity, "expiration value is invalid"),
			want: "server rejected the request due to a validation error; " +
				"ask the user to review the content and parameters for invalid values: " +
				"expiration value is invalid",
		},
		{
			name: "HTTPError unprocessable entity empty body",
			err:  NewHTTPError(http.StatusUnprocessableEntity, ""),
			want: "server rejected the request due to a validation error; " +
				"ask the user to review the content and parameters for invalid values",
		},
		{
			name: "HTTPError unknown status",
			err:  NewHTTPError(http.StatusInternalServerError, "internal error"),
			want: "unknown HTTP error; ask the user to check the server status " +
				"or the request: HTTP 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestTypedErrors_IsLegacySentinels verifies that every typed error still
// matches its legacy sentinel through errors.Is, so callers that check
// errors.Is(err, errContentTooLarge) keep working through the chain.
func TestTypedErrors_IsLegacySentinels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		sentinel error
	}{
		{
			name:     "ContentTooLargeError",
			err:      NewContentTooLargeError(10, 5),
			sentinel: errContentTooLarge,
		},
		{
			name:     "InvalidExtensionError",
			err:      NewInvalidExtensionError("a/b"),
			sentinel: errInvalidExtension,
		},
		{
			name:     "PathNotFoundError",
			err:      NewPathNotFoundError(syscall.ENOENT),
			sentinel: errPathNotFound,
		},
		{
			name:     "PathPermissionError",
			err:      NewPathPermissionError(syscall.EACCES),
			sentinel: errPathPermissionDenied,
		},
		{
			name:     "SandboxNoMatchError",
			err:      NewSandboxNoMatchError("/x"),
			sentinel: errSandboxTranslationNoMatch,
		},
		{
			name:     "BlockedComponentError",
			err:      NewBlockedComponentError(".ssh"),
			sentinel: errBuiltinBlockedComponent,
		},
		{
			name:     "HTTPError forbidden",
			err:      NewHTTPError(http.StatusForbidden, ""),
			sentinel: errServerRejected,
		},
		{
			name:     "HTTPError request entity too large",
			err:      NewHTTPError(http.StatusRequestEntityTooLarge, ""),
			sentinel: errContentTooLargeServer,
		},
		{
			name:     "HTTPError unprocessable entity",
			err:      NewHTTPError(http.StatusUnprocessableEntity, ""),
			sentinel: errServerValidation,
		},
		{
			name:     "HTTPError unknown status",
			err:      NewHTTPError(http.StatusInternalServerError, ""),
			sentinel: errUnknownHTTP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if !errors.Is(tt.err, tt.sentinel) {
				t.Errorf("errors.Is(%v, %v) = false, want true", tt.err, tt.sentinel)
			}
		})
	}
}

// TestTypedErrors_As verifies that errors.As can extract a typed error from a
// wrapped chain and expose its documented fields.
func TestTypedErrors_As(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("context: %w", NewContentTooLargeError(10, 5))

	var target *ContentTooLargeError
	if !errors.As(wrapped, &target) {
		t.Fatal("expected errors.As to match *ContentTooLargeError through the chain")
	}

	if target.Size != 10 || target.Limit != 5 {
		t.Errorf("Size/Limit = %d/%d, want 10/5", target.Size, target.Limit)
	}
}
