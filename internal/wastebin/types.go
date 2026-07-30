// Package wastebin provides core types, configuration, text detection,
// and expiration parsing for the wastebin-mcp-go server.
package wastebin

// CreatePasteArgs holds all input parameters for creating a paste.
type CreatePasteArgs struct {
	Content              *string `json:"content,omitempty"`   // nil when using file_path
	FilePath             *string `json:"file_path,omitempty"` // nil when using content
	Extension            *string `json:"extension,omitempty"` // syntax highlighting extension
	Expires              *string `json:"expires,omitempty"`   // expiration string
	Title                *string `json:"title,omitempty"`
	BurnAfterReading     *bool   `json:"burn_after_reading,omitempty"`
	Password             *string `json:"password,omitempty"`
	TranslateSandboxPath *bool   `json:"translate_sandbox_path,omitempty"`
}

// PasteResponse is the result returned to the MCP client.
type PasteResponse struct {
	Hostname         string `json:"hostname"`
	ID               string `json:"id"`
	URL              string `json:"url"`
	Raw              string `json:"raw"`
	MarkdownRendered string `json:"markdown_rendered,omitempty"`
	Hint             string `json:"hint,omitempty"`
	PasswordHint     string `json:"password_hint,omitempty"`
}

// blockedPathEntry stores both the lexical (as-configured by the operator)
// and the resolved (canonical after EvalSymlinks) form of a user-defined
// blocked path.  Keeping both identities prevents a late-created symlink
// from bypassing a lexical blocklist entry.
type blockedPathEntry struct {
	Lexical  string // absolute, cleaned form exactly as the operator configured
	Resolved string // canonical form after EvalSymlinks (same as Lexical if missing at startup)
}

// Config holds all configuration from environment variables.
type Config struct {
	ServerURL               string
	DefaultExpires          int // seconds
	FileReadEnabled         bool
	AllowedPaths            []string // resolved absolute dirs
	BlockedPaths            []blockedPathEntry
	MaxContentSize          int64 // bytes, default 1MB
	SandboxMounts           []SandboxMount
	SandboxTransparent      bool
	DisableBuiltinBlocklist bool
	Debug                   bool

	// AllowInsecurePassword allows password-protected pastes over
	// non-loopback HTTP connections. When false (default), password-protected
	// pastes are rejected for non-loopback http:// URLs.
	AllowInsecurePassword bool
}
