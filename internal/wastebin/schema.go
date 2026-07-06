package wastebin

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SchemaBuilder builds JSON Schema and tool descriptions for the create_paste
// MCP tool based on server configuration.
type SchemaBuilder struct {
	cfg *Config
}

// NewSchemaBuilder creates a new SchemaBuilder for the given config.
func NewSchemaBuilder(cfg *Config) *SchemaBuilder {
	return &SchemaBuilder{cfg: cfg}
}

// BuildToolSchema generates the JSON Schema for the create_paste tool input
// dynamically based on the server configuration.
func (b *SchemaBuilder) BuildToolSchema() (json.RawMessage, error) {
	props := map[string]any{}

	var required []string

	// content -- always included
	contentDesc := "The text content of the paste. Provide this OR file_path (not both)."
	if b.cfg.FileReadEnabled {
		contentDesc += " When file_path is provided instead, this field is not needed."
	}

	props["content"] = map[string]any{
		"type":        "string",
		"description": contentDesc,
	}

	if !b.cfg.FileReadEnabled {
		required = append(required, "content")
	}

	// file_path -- only when FileReadEnabled
	if b.cfg.FileReadEnabled {
		filePathDesc := "Path to a local file to read and upload as paste content. " +
			"Provide this OR content (not both). The file must be a text file."

		if len(b.cfg.SandboxMounts) > 0 && !b.cfg.SandboxTransparent {
			filePathDesc += " Sandbox path translation is available: when " +
				"`translate_sandbox_path` is set to `true`, the path is " +
				"translated to the corresponding host path."
		}

		if len(b.cfg.AllowedPaths) > 0 {
			filePathDesc += " SECURITY: Only paths under ALLOWED_PATHS are accepted."
		} else {
			filePathDesc += " SECURITY: Paths pass through the built-in and user blocklist pipeline."
		}

		filePathDesc += " Blocked system paths (/etc, /proc, /sys, /dev by default) are rejected."

		props["file_path"] = map[string]any{
			"type":        "string",
			"description": filePathDesc,
		}
	}

	// extension -- optional
	props["extension"] = map[string]any{
		"type": "string",
		"description": "File extension for syntax highlighting (e.g. 'go', 'py', 'js', " +
			"'md'). When using file_path, the extension is detected from the " +
			"file name if not provided.",
	}

	// expires -- optional
	defaultExpiresDesc := DescribeDefaultExpires(b.cfg.DefaultExpires)
	props["expires"] = map[string]any{
		"type": "string",
		"description": "Expiration time: bare number (seconds) or number plus unit " +
			"suffix (s, m, h, d, w, M=30d, y=365d). Examples: '3600', '1h', " +
			"'7d', '30M'. Defaults to " + defaultExpiresDesc + ". " +
			"Configured via WASTEBIN_MCP_DEFAULT_EXPIRES.",
	}

	// title -- optional
	props["title"] = map[string]any{
		"type":        "string",
		"description": "Optional title for the paste.",
	}

	// burn_after_reading -- optional
	props["burn_after_reading"] = map[string]any{
		"type": "boolean",
		"description": "If true, the paste is deleted automatically after being " +
			"retrieved via any access method (raw, web, API) for the first " +
			"time. The agent's own reads also count -- creating a " +
			"burn-after-reading paste and then reading it back will delete it.",
	}

	// password -- optional
	props["password"] = map[string]any{
		"type": "string",
		"description": "Optional password to protect the paste. " +
			"Password-protected pastes require the Wastebin-Password header for retrieval.",
	}

	// translate_sandbox_path -- only when mounts configured and not transparent
	if b.cfg.FileReadEnabled && len(b.cfg.SandboxMounts) > 0 && !b.cfg.SandboxTransparent {
		props["translate_sandbox_path"] = map[string]any{
			"type": "boolean",
			"description": "Set to true if file_path is a sandbox-internal " +
				"path that should be translated to the corresponding host " +
				"path using configured sandbox mounts.",
		}
	}

	schema := map[string]any{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	data, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshal paste schema: %w", err)
	}

	return json.RawMessage(data), nil
}

// DescribeDefaultExpires returns a human-readable description of a default
// expiration value in seconds for use in MCP tool descriptions.
func DescribeDefaultExpires(seconds int) string {
	if seconds == 0 {
		return "no expiration (paste persists until manually deleted)"
	}

	years := seconds / secondsPerYear
	r := seconds % secondsPerYear
	days := r / secondsPerDay
	r %= secondsPerDay
	hours := r / secondsPerHour
	r %= secondsPerHour
	minutes := r / secondsPerMinute
	secs := r % secondsPerMinute

	var parts []string
	if years == 1 {
		parts = append(parts, "1 year")
	} else if years > 1 {
		parts = append(parts, fmt.Sprintf("%d years", years))
	}

	if days == 1 {
		parts = append(parts, "1 day")
	} else if days > 1 {
		parts = append(parts, fmt.Sprintf("%d days", days))
	}

	if hours == 1 {
		parts = append(parts, "1 hour")
	} else if hours > 1 {
		parts = append(parts, fmt.Sprintf("%d hours", hours))
	}

	if minutes == 1 {
		parts = append(parts, "1 minute")
	} else if minutes > 1 {
		parts = append(parts, fmt.Sprintf("%d minutes", minutes))
	}

	if secs == 1 {
		parts = append(parts, "1 second")
	} else if secs > 1 {
		parts = append(parts, fmt.Sprintf("%d seconds", secs))
	}

	if len(parts) == 0 {
		return "0 seconds"
	}

	human := strings.Join(parts, ", ")

	return fmt.Sprintf("%d seconds (%s)", seconds, human)
}

// BuildToolDescription returns the tool-level description for create_paste.
func (b *SchemaBuilder) BuildToolDescription() string {
	return "Create a text paste on the configured Wastebin instance. " +
		"Use 'content' for inline text or 'file_path' to upload a local " +
		"file (when file mode is enabled). " +
		"Content supports multiple lines naturally -- include newlines " +
		"directly in the string value. " +
		"The response includes 'hostname', " +
		"'id', 'url', and 'raw' fields. Reconstruct full URLs as " +
		"{hostname}{url} or {hostname}{raw}. " +
		"When 'extension' is 'md' or 'markdown', a 'markdown_rendered' " +
		"field appears with the rendered view URL. " +
		"Password-protected pastes require the Wastebin-Password header " +
		"for retrieval. " +
		"File mode validates paths against an allowlist (when configured) and blocklist pipeline."
}
