package wastebin //nolint:testpackage // contract tests need access to unexported sentinels

import (
	"errors"
	"fmt"
	"net/http"
	"os"
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
			err:  newContentTooLargeError(10, 5),
			want: "content exceeds the maximum allowed size; " +
				"split the content into smaller parts and upload each separately: " +
				"10 bytes exceeds limit of 5 bytes",
		},
		{
			name: "InvalidExtensionError",
			err:  newInvalidExtensionError("a/b"),
			want: `extension contains invalid characters (/, \, ?, #); ` +
				`use a plain extension like 'go' or 'py': "a/b"`,
		},
		{
			name: "PathNotFoundError",
			err:  newPathNotFoundError(syscall.ENOENT),
			want: "the specified file does not exist; verify the path is correct " +
				"and do not attempt the same path again: " + syscall.ENOENT.Error(),
		},
		{
			name: "PathPermissionError",
			err:  newPathPermissionError(syscall.EACCES),
			want: "the file exists but is not readable; ask the user to check " +
				"file permissions: " + syscall.EACCES.Error(),
		},
		{
			name: "SandboxNoMatchError",
			err:  newSandboxNoMatchError("/workspace/report.md"),
			want: "sandbox path does not match any configured mount; " +
				"ask the user to check the sandbox mount configuration: " +
				"/workspace/report.md",
		},
		{
			name: "BlockedComponentError",
			err:  newBlockedComponentError(".ssh"),
			want: "file path contains a blocked component and will always be " +
				"rejected; do not attempt again: .ssh",
		},
		{
			name: "BlockedPrefixError",
			err:  newBlockedPrefixError("/etc"),
			want: "file path is in a blocked system directory and will always be " +
				"rejected; do not attempt again: /etc",
		},
		{
			name: "HTTPError forbidden",
			err:  newHTTPError(http.StatusForbidden, "forbidden body"),
			want: "server rejected the request; content may contain disallowed data; " +
				"ask the user to check the content or the server logs",
		},
		{
			name: "HTTPError request entity too large",
			err:  newHTTPError(http.StatusRequestEntityTooLarge, "too large"),
			want: "content exceeds the server's maximum allowed size; " +
				"split the content into smaller parts and upload each separately",
		},
		{
			name: "HTTPError unprocessable entity with body",
			err:  newHTTPError(http.StatusUnprocessableEntity, "expiration value is invalid"),
			want: "server rejected the request due to a validation error; " +
				"ask the user to review the content and parameters for invalid values: " +
				"expiration value is invalid",
		},
		{
			name: "HTTPError unprocessable entity empty body",
			err:  newHTTPError(http.StatusUnprocessableEntity, ""),
			want: "server rejected the request due to a validation error; " +
				"ask the user to review the content and parameters for invalid values",
		},
		{
			name: "HTTPError unknown status",
			err:  newHTTPError(http.StatusInternalServerError, "internal error"),
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
			err:      newContentTooLargeError(10, 5),
			sentinel: errContentTooLarge,
		},
		{
			name:     "InvalidExtensionError",
			err:      newInvalidExtensionError("a/b"),
			sentinel: errInvalidExtension,
		},
		{
			name:     "PathNotFoundError",
			err:      newPathNotFoundError(syscall.ENOENT),
			sentinel: errPathNotFound,
		},
		{
			name:     "PathPermissionError",
			err:      newPathPermissionError(syscall.EACCES),
			sentinel: errPathPermissionDenied,
		},
		{
			name:     "SandboxNoMatchError",
			err:      newSandboxNoMatchError("/x"),
			sentinel: errSandboxTranslationNoMatch,
		},
		{
			name:     "BlockedComponentError",
			err:      newBlockedComponentError(".ssh"),
			sentinel: errBuiltinBlockedComponent,
		},
		{
			name:     "BlockedPrefixError",
			err:      newBlockedPrefixError("/etc"),
			sentinel: errBuiltinBlockedPrefix,
		},
		{
			name:     "HTTPError forbidden",
			err:      newHTTPError(http.StatusForbidden, ""),
			sentinel: errServerRejected,
		},
		{
			name:     "HTTPError request entity too large",
			err:      newHTTPError(http.StatusRequestEntityTooLarge, ""),
			sentinel: errContentTooLargeServer,
		},
		{
			name:     "HTTPError unprocessable entity",
			err:      newHTTPError(http.StatusUnprocessableEntity, ""),
			sentinel: errServerValidation,
		},
		{
			name:     "HTTPError unknown status",
			err:      newHTTPError(http.StatusInternalServerError, ""),
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

	wrapped := fmt.Errorf("context: %w", newContentTooLargeError(10, 5))

	var target *ContentTooLargeError
	if !errors.As(wrapped, &target) {
		t.Fatal("expected errors.As to match *ContentTooLargeError through the chain")
	}

	if target.Size != 10 || target.Limit != 5 {
		t.Errorf("Size/Limit = %d/%d, want 10/5", target.Size, target.Limit)
	}
}

// TestTypedErrors_UnwrapChain wraps each typed error in a fmt.Errorf chain and
// asserts errors.Is still matches through the chain. It also calls Unwrap()
// directly on each typed error: errors.Is alone is short-circuited by the
// typed errors' Is() methods before Unwrap() runs, so a direct call is
// required to exercise the unwrap path. For path errors it additionally
// asserts the underlying OS error class is reachable through the join.
func TestTypedErrors_UnwrapChain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		sentinel error
		osTarget error
	}{
		{
			name:     "ContentTooLargeError",
			err:      newContentTooLargeError(10, 5),
			sentinel: errContentTooLarge,
		},
		{
			name:     "InvalidExtensionError",
			err:      newInvalidExtensionError("a/b"),
			sentinel: errInvalidExtension,
		},
		{
			name:     "PathNotFoundError",
			err:      newPathNotFoundError(syscall.ENOENT),
			sentinel: errPathNotFound,
			osTarget: os.ErrNotExist,
		},
		{
			name:     "PathPermissionError",
			err:      newPathPermissionError(syscall.EACCES),
			sentinel: errPathPermissionDenied,
			osTarget: os.ErrPermission,
		},
		{
			name:     "SandboxNoMatchError",
			err:      newSandboxNoMatchError("/x"),
			sentinel: errSandboxTranslationNoMatch,
		},
		{
			name:     "BlockedComponentError",
			err:      newBlockedComponentError(".ssh"),
			sentinel: errBuiltinBlockedComponent,
		},
		{
			name:     "BlockedPrefixError",
			err:      newBlockedPrefixError("/etc"),
			sentinel: errBuiltinBlockedPrefix,
		},
		{
			name:     "HTTPError forbidden",
			err:      newHTTPError(http.StatusForbidden, ""),
			sentinel: errServerRejected,
		},
		{
			name:     "HTTPError request entity too large",
			err:      newHTTPError(http.StatusRequestEntityTooLarge, ""),
			sentinel: errContentTooLargeServer,
		},
		{
			name:     "HTTPError unprocessable entity",
			err:      newHTTPError(http.StatusUnprocessableEntity, ""),
			sentinel: errServerValidation,
		},
		{
			name:     "HTTPError unknown status",
			err:      newHTTPError(http.StatusInternalServerError, ""),
			sentinel: errUnknownHTTP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			wrapped := fmt.Errorf("ctx: %w", tt.err)

			if !errors.Is(wrapped, tt.sentinel) {
				t.Errorf("errors.Is(%v, %v) = false, want true", wrapped, tt.sentinel)
			}

			if tt.osTarget != nil && !errors.Is(wrapped, tt.osTarget) {
				t.Errorf("errors.Is(%v, %v) = false, want true", wrapped, tt.osTarget)
			}

			unwrapper, ok := tt.err.(interface{ Unwrap() error })
			if !ok {
				t.Fatalf("%T does not implement Unwrap", tt.err)
			}

			unwrapped := unwrapper.Unwrap()
			if !errors.Is(unwrapped, tt.sentinel) {
				t.Errorf("Unwrap() = %v, want legacy sentinel %v reachable", unwrapped, tt.sentinel)
			}

			if tt.osTarget != nil && !errors.Is(unwrapped, tt.osTarget) {
				t.Errorf("Unwrap() = %v, want OS error class %v reachable", unwrapped, tt.osTarget)
			}
		})
	}
}
