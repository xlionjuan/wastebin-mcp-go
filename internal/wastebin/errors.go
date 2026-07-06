package wastebin

import "errors"

// Client errors.
var (
	errBothContentAndFilePath    = errors.New("provide either 'content' or 'file_path', not both")
	errNeitherContentNorFilePath = errors.New("provide either 'content' or 'file_path'")
	errContentEmpty              = errors.New("content cannot be empty")
	errContentTooLarge           = errors.New("content exceeds the maximum allowed size")
	errServerRejected            = errors.New("server rejected the request; content may contain disallowed data")
	errContentTooLargeServer     = errors.New("content exceeds the server's maximum allowed size")
	errUnknownHTTP               = errors.New("unknown HTTP error")
	errFileNotText               = errors.New("file is binary or not valid UTF-8 text")
	errServerValidation          = errors.New("server rejected the request due to a validation error")
	errTooManyRedirects          = errors.New("stopped after 10 redirects")
	errRedirectDifferentHost     = errors.New("redirect to different host blocked")
	errRedirectSchemeDowngrade   = errors.New("redirect scheme downgrade from https to http blocked")
	errInvalidWastebinResponse   = errors.New("invalid Wastebin response")
	errPasswordOverHTTP          = errors.New(
		"password-protected pastes are not allowed over non-loopback HTTP " +
			"connections; use HTTPS or set " +
			"WASTEBIN_MCP_ALLOW_INSECURE_PASSWORD=true for local development",
	)
)

// Path validation errors.
var (
	errPathTraversal           = errors.New("path traversal is not allowed")
	errPathNotAllowed          = errors.New("file path is not under any allowed path")
	errBuiltinBlockedPrefix    = errors.New("file path is in a blocked system directory")
	errBuiltinBlockedComponent = errors.New("file path contains a blocked component")
	errUserBlockedPath         = errors.New("file path is in a user-blocked directory")
	errFilePathCannotBeUsed    = errors.New("file path cannot be used")
	errPathNotFound            = errors.New("file path not found")
	errPathPermissionDenied    = errors.New("permission denied for file path")
)

// Config errors.
var (
	errServerURLRequired              = errors.New("WASTEBIN_SERVER_URL is required and must not be empty")
	errNegativeDefaultExpires         = errors.New("WASTEBIN_MCP_DEFAULT_EXPIRES cannot be negative")
	errMaxContentSizeTooSmall         = errors.New("WASTEBIN_MCP_MAX_CONTENT_SIZE must be at least 1")
	errInvalidDisableBuiltinBlocklist = errors.New("invalid WASTEBIN_MCP_DISABLE_BUILTIN_BLOCKLIST")
	errConfiguredPathNotAbsolute      = errors.New("configured path must be absolute")
)

// Client config errors (used during construction).
var (
	errConfigRequired       = errors.New("config is required")
	errUnsupportedURLScheme = errors.New("server URL must use http or https scheme")
	errURLMissingHost       = errors.New("server URL must include a host")
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
	errFileReadDisabled           = errors.New("file read is disabled by configuration")
	errInvalidExtension           = errors.New("extension contains invalid path or query characters")
	errSandboxTranslationNoMounts = errors.New("sandbox path translation requested but no mounts configured")
	errSandboxTranslationNoMatch  = errors.New("sandbox path does not match any configured mount")
)

// Sandbox mount errors.
var (
	errInvalidSandboxMount = errors.New("invalid sandbox mount format")
	errOverlappingMounts   = errors.New("overlapping sandbox mount paths")
)

// File open errors.
var errOpenEmptyPath = errors.New("open: empty relative path")
