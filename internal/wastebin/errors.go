package wastebin

import (
	"errors"
	"fmt"
	"net/http"
)

// Error messages follow the unified "<problem>; <next-step guidance>" wording,
// with dynamic values appended as ": <value>". See docs/adr/005-error-model.md.
//
// Sentinel errors mark pure conditions that carry no dynamic data; they are
// matched with errors.Is. Errors that carry dynamic data are represented by the
// typed errors defined below. Each typed error wraps the legacy sentinel it
// replaces so existing errors.Is assertions keep working through the chain.

// Client errors.
var (
	errBothContentAndFilePath = errors.New(
		"provide either 'content' or 'file_path', not both; pick exactly one input mode",
	)
	errNeitherContentNorFilePath = errors.New("provide either 'content' or 'file_path'; supply one of the two inputs")
	errContentEmpty              = errors.New(
		"content cannot be empty; provide non-empty content or use file_path instead",
	)
	errContentTooLarge = errors.New("content exceeds the maximum allowed size; " +
		"split the content into smaller parts and upload each separately")
	errServerRejected = errors.New(
		"server rejected the request; content may contain disallowed data; " +
			"ask the user to check the content or the server logs",
	)
	errContentTooLargeServer = errors.New("content exceeds the server's maximum allowed size; " +
		"split the content into smaller parts and upload each separately")
	errUnknownHTTP = errors.New(
		"unknown HTTP error; ask the user to check the server status or the request",
	)
	errFileNotText = errors.New("file is binary or not valid UTF-8 text and cannot be uploaded. " +
		"Do not attempt again for this file")
	errServerValidation = errors.New(
		"server rejected the request due to a validation error; " +
			"ask the user to review the content and parameters for invalid values",
	)
	errTooManyRedirects = errors.New(
		"stopped after 10 redirects; ask the user to check the server URL for a redirect loop",
	)
	errRedirectDifferentHost = errors.New(
		"redirect to different host blocked; ask the user to check the server URL and its redirects",
	)
	errRedirectSchemeDowngrade = errors.New(
		"redirect scheme downgrade from https to http blocked; use an https server URL",
	)
	errInvalidWastebinResponse = errors.New("invalid Wastebin response")
	errResponseTooLarge        = errors.New(
		"wastebin response exceeds maximum allowed size; ask the user to check the server configuration",
	)
	errPasswordEmpty = errors.New(
		"password must be non-empty when provided; set a non-empty password " +
			"or omit the field entirely",
	)
	errPasswordOverHTTP = errors.New(
		"password-protected pastes are not allowed over non-loopback HTTP " +
			"connections; use HTTPS or set " +
			"WASTEBIN_MCP_ALLOW_INSECURE_PASSWORD=true for local development",
	)
)

// Path validation errors.
var (
	errPathTraversal = errors.New("path traversal is not allowed and will always be rejected. " +
		"Do not attempt again")
	errPathNotAllowed = errors.New("file path is not under any configured allowed path; " +
		"ask the user to check ALLOWED_PATHS if this path should be accessible")
	errBuiltinBlockedPrefix = errors.New("file path is in a blocked system directory " +
		"and will always be rejected. Do not attempt again")
	errBuiltinBlockedComponent = errors.New("file path contains a blocked component " +
		"and will always be rejected. Do not attempt again")
	errUserBlockedPath = errors.New("file path is in a user-blocked directory " +
		"and will always be rejected. Do not attempt again")
	errFilePathCannotBeUsed = errors.New("file path cannot be used; " +
		"ask the user to check the path or file permissions")
	errPathNotFound = errors.New("the specified file does not exist; " +
		"verify the path is correct and do not attempt the same path again")
	errPathPermissionDenied = errors.New("the file exists but is not readable; ask the user to check file permissions")
)

// Config errors.
var (
	errServerURLRequired              = errors.New("WASTEBIN_SERVER_URL is required and must not be empty")
	errNegativeDefaultExpires         = errors.New("WASTEBIN_MCP_DEFAULT_EXPIRES cannot be negative")
	errMaxContentSizeTooSmall         = errors.New("WASTEBIN_MCP_MAX_CONTENT_SIZE must be at least 1")
	errMaxContentSizeTooLarge         = errors.New("WASTEBIN_MCP_MAX_CONTENT_SIZE exceeds the maximum supported value")
	errInvalidDisableBuiltinBlocklist = errors.New("invalid WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST")
	errConfiguredPathNotAbsolute      = errors.New("configured path must be absolute")
)

// Client config errors (used during construction).
var (
	errConfigRequired       = errors.New("config is required")
	errUnsupportedURLScheme = errors.New("server URL must use http or https scheme")
	errURLMissingHost       = errors.New("server URL must include a host")
	errURLUserInfo          = errors.New("server URL must not include user information")
	errURLQuery             = errors.New("server URL must not include a query string")
	errURLFragment          = errors.New("server URL must not include a fragment")
)

// Expiration errors.
var (
	errNegativeExpiration    = errors.New("expiration cannot be negative")
	errUnknownExpirationUnit = errors.New("unknown expiration unit")
	errInvalidExpirationFmt  = errors.New("invalid expiration format")
	errExpirationOverflow    = errors.New("expiration overflow")
	errExpirationTooLarge    = errors.New("expiration exceeds maximum supported value")
)

// File read and sandbox errors.
var (
	errFileReadDisabled = errors.New("file read is disabled by configuration; " +
		"use inline content instead. Do not attempt again")
	errInvalidExtension = errors.New("extension contains invalid characters (/, \\, ?, #); " +
		"use a plain extension like 'go' or 'py'")
	errSandboxTranslationNoMounts = errors.New("sandbox path translation was requested but no sandbox mounts are configured; " + //nolint:lll // long variable name + message
		"ask the user to check WASTEBIN_MCP_SANDBOX_MOUNTS if translation should be enabled")
	errSandboxTranslationNoMatch = errors.New("sandbox path does not match any configured mount; " +
		"ask the user to check the sandbox mount configuration")
)

// Sandbox mount errors.
var (
	errInvalidSandboxMount = errors.New("invalid sandbox mount format")
	errOverlappingMounts   = errors.New("overlapping sandbox mount paths")
)

// File open errors.
var errOpenEmptyPath = errors.New("open: empty relative path")

// ──────────────────────────────────────────────
// Typed errors
// ──────────────────────────────────────────────

// ContentTooLargeError indicates that the paste content or file exceeds the
// configured size limit. It wraps errContentTooLarge so errors.Is keeps
// matching the legacy sentinel.
type ContentTooLargeError struct {
	Size  int64 // Size is the size of the content, in bytes.
	Limit int64 // Limit is the configured maximum size, in bytes.
	err   error
}

var _ error = (*ContentTooLargeError)(nil) //nolint:errcheck // compile-time interface assertion; not a function call

// NewContentTooLargeError creates a ContentTooLargeError.
func NewContentTooLargeError(size, limit int64) *ContentTooLargeError {
	return &ContentTooLargeError{Size: size, Limit: limit, err: errContentTooLarge}
}

// Error implements the error interface for ContentTooLargeError.
func (e *ContentTooLargeError) Error() string {
	return fmt.Sprintf("%v: %d bytes exceeds limit of %d bytes", errContentTooLarge, e.Size, e.Limit)
}

// Is implements the errors.Is interface for ContentTooLargeError.
func (e *ContentTooLargeError) Is(target error) bool {
	return target == errContentTooLarge
}

// Unwrap returns the wrapped legacy sentinel for errors.Is support.
func (e *ContentTooLargeError) Unwrap() error {
	return e.err
}

// InvalidExtensionError indicates that a caller-provided extension contains
// characters that would create malformed Wastebin URLs. It wraps
// errInvalidExtension so errors.Is keeps matching the legacy sentinel.
type InvalidExtensionError struct {
	Ext string // Ext is the caller-provided extension.
	err error
}

var _ error = (*InvalidExtensionError)(nil) //nolint:errcheck // compile-time interface assertion; not a function call

// NewInvalidExtensionError creates an InvalidExtensionError.
func NewInvalidExtensionError(ext string) *InvalidExtensionError {
	return &InvalidExtensionError{Ext: ext, err: errInvalidExtension}
}

// Error implements the error interface for InvalidExtensionError.
func (e *InvalidExtensionError) Error() string {
	return fmt.Sprintf("%v: %q", errInvalidExtension, e.Ext)
}

// Is implements the errors.Is interface for InvalidExtensionError.
func (e *InvalidExtensionError) Is(target error) bool {
	return target == errInvalidExtension
}

// Unwrap returns the wrapped legacy sentinel for errors.Is support.
func (e *InvalidExtensionError) Unwrap() error {
	return e.err
}

// PathNotFoundError indicates that the specified file does not exist. It wraps
// errPathNotFound so errors.Is keeps matching the legacy sentinel.
type PathNotFoundError struct {
	Underlying error // Underlying is the original filesystem error.
	err        error
}

var _ error = (*PathNotFoundError)(nil) //nolint:errcheck // compile-time interface assertion; not a function call

// NewPathNotFoundError creates a PathNotFoundError.
func NewPathNotFoundError(underlying error) *PathNotFoundError {
	return &PathNotFoundError{Underlying: underlying, err: errPathNotFound}
}

// Error implements the error interface for PathNotFoundError.
func (e *PathNotFoundError) Error() string {
	return fmt.Sprintf("%v: %v", errPathNotFound, e.Underlying)
}

// Is implements the errors.Is interface for PathNotFoundError.
func (e *PathNotFoundError) Is(target error) bool {
	return target == errPathNotFound
}

// Unwrap returns the wrapped legacy sentinel for errors.Is support.
func (e *PathNotFoundError) Unwrap() error {
	return e.err
}

// PathPermissionError indicates that the specified file exists but is not
// readable. It wraps errPathPermissionDenied so errors.Is keeps matching the
// legacy sentinel.
type PathPermissionError struct {
	Underlying error // Underlying is the original filesystem error.
	err        error
}

var _ error = (*PathPermissionError)(nil) //nolint:errcheck // compile-time interface assertion; not a function call

// NewPathPermissionError creates a PathPermissionError.
func NewPathPermissionError(underlying error) *PathPermissionError {
	return &PathPermissionError{Underlying: underlying, err: errPathPermissionDenied}
}

// Error implements the error interface for PathPermissionError.
func (e *PathPermissionError) Error() string {
	return fmt.Sprintf("%v: %v", errPathPermissionDenied, e.Underlying)
}

// Is implements the errors.Is interface for PathPermissionError.
func (e *PathPermissionError) Is(target error) bool {
	return target == errPathPermissionDenied
}

// Unwrap returns the wrapped legacy sentinel for errors.Is support.
func (e *PathPermissionError) Unwrap() error {
	return e.err
}

// SandboxNoMatchError indicates that the sandbox path does not match any
// configured mount. It wraps errSandboxTranslationNoMatch so errors.Is keeps
// matching the legacy sentinel.
type SandboxNoMatchError struct {
	Path string // Path is the sandbox path that matched no mount.
	err  error
}

var _ error = (*SandboxNoMatchError)(nil) //nolint:errcheck // compile-time interface assertion; not a function call

// NewSandboxNoMatchError creates a SandboxNoMatchError.
func NewSandboxNoMatchError(path string) *SandboxNoMatchError {
	return &SandboxNoMatchError{Path: path, err: errSandboxTranslationNoMatch}
}

// Error implements the error interface for SandboxNoMatchError.
func (e *SandboxNoMatchError) Error() string {
	return fmt.Sprintf("%v: %s", errSandboxTranslationNoMatch, e.Path)
}

// Is implements the errors.Is interface for SandboxNoMatchError.
func (e *SandboxNoMatchError) Is(target error) bool {
	return target == errSandboxTranslationNoMatch
}

// Unwrap returns the wrapped legacy sentinel for errors.Is support.
func (e *SandboxNoMatchError) Unwrap() error {
	return e.err
}

// BlockedComponentError indicates that a path contains a built-in blocked
// component (e.g. .ssh, .git). It wraps errBuiltinBlockedComponent so errors.Is
// keeps matching the legacy sentinel.
type BlockedComponentError struct {
	Reason string // Reason is the blocked path component.
	err    error
}

var _ error = (*BlockedComponentError)(nil) //nolint:errcheck // compile-time interface assertion; not a function call

// NewBlockedComponentError creates a BlockedComponentError.
func NewBlockedComponentError(reason string) *BlockedComponentError {
	return &BlockedComponentError{Reason: reason, err: errBuiltinBlockedComponent}
}

// Error implements the error interface for BlockedComponentError.
func (e *BlockedComponentError) Error() string {
	return fmt.Sprintf("%v: %s", errBuiltinBlockedComponent, e.Reason)
}

// Is implements the errors.Is interface for BlockedComponentError.
func (e *BlockedComponentError) Is(target error) bool {
	return target == errBuiltinBlockedComponent
}

// Unwrap returns the wrapped legacy sentinel for errors.Is support.
func (e *BlockedComponentError) Unwrap() error {
	return e.err
}

// HTTPError indicates that the Wastebin server returned a non-OK HTTP status.
// It wraps a status-specific legacy sentinel so errors.Is keeps matching the
// legacy sentinels.
type HTTPError struct {
	StatusCode int    // StatusCode is the HTTP status code returned by the server.
	Body       string // Body is the (truncated) response body for diagnostics.
	err        error
}

var _ error = (*HTTPError)(nil) //nolint:errcheck // compile-time interface assertion; not a function call

// NewHTTPError creates an HTTPError for the given status code and body.
func NewHTTPError(statusCode int, body string) *HTTPError {
	var sentinel error

	switch statusCode {
	case http.StatusForbidden:
		sentinel = errServerRejected
	case http.StatusRequestEntityTooLarge:
		sentinel = errContentTooLargeServer
	case http.StatusUnprocessableEntity:
		sentinel = errServerValidation
	default:
		sentinel = errUnknownHTTP
	}

	return &HTTPError{StatusCode: statusCode, Body: body, err: sentinel}
}

// Error implements the error interface for HTTPError.
func (e *HTTPError) Error() string {
	switch e.StatusCode {
	case http.StatusUnprocessableEntity:
		if e.Body == "" {
			return errServerValidation.Error()
		}

		return fmt.Sprintf("%v: %s", errServerValidation, e.Body)
	case http.StatusForbidden, http.StatusRequestEntityTooLarge:
		return e.err.Error()
	default:
		return fmt.Sprintf("%v: HTTP %d", errUnknownHTTP, e.StatusCode)
	}
}

// Is implements the errors.Is interface for HTTPError.
func (e *HTTPError) Is(target error) bool {
	return target == e.err
}

// Unwrap returns the wrapped legacy sentinel for errors.Is support.
func (e *HTTPError) Unwrap() error {
	return e.err
}
