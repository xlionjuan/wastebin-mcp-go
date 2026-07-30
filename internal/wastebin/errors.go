package wastebin

import "errors"

// Client errors.
var (
	errBothContentAndFilePath    = errors.New("provide either 'content' or 'file_path', not both")
	errNeitherContentNorFilePath = errors.New("provide either 'content' or 'file_path'")
	errContentEmpty              = errors.New("content cannot be empty")
	errContentTooLarge           = errors.New("content exceeds the maximum allowed size; " +
		"split the content into smaller parts and upload each separately")
	errServerRejected        = errors.New("server rejected the request; content may contain disallowed data")
	errContentTooLargeServer = errors.New("content exceeds the server's maximum allowed size; " +
		"split the content into smaller parts and upload each separately")
	errUnknownHTTP = errors.New("unknown HTTP error")
	errFileNotText = errors.New("file is binary or not valid UTF-8 text and cannot be uploaded. " +
		"Do not attempt again for this file")
	errServerValidation        = errors.New("server rejected the request due to a validation error")
	errTooManyRedirects        = errors.New("stopped after 10 redirects")
	errRedirectDifferentHost   = errors.New("redirect to different host blocked")
	errRedirectSchemeDowngrade = errors.New("redirect scheme downgrade from https to http blocked")
	errInvalidWastebinResponse = errors.New("invalid Wastebin response")
	errResponseTooLarge        = errors.New("wastebin response exceeds maximum allowed size")
	errPasswordEmpty           = errors.New(
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
	errFilePathCannotBeUsed = errors.New("file path cannot be used")
	errPathNotFound         = errors.New("the specified file does not exist; " +
		"verify the path is correct and do not attempt the same path again")
	errPathPermissionDenied = errors.New("the file exists but is not readable; ask the user to check file permissions")
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
