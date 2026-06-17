// Package main provides the MCP stdio server for wastebin-mcp-go.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"wastebin-mcp-go/internal/wastebin"
)

// buildPasteSchema generates the JSON Schema for the create_paste tool input
// dynamically based on the server configuration.
func buildPasteSchema(cfg *wastebin.Config) (json.RawMessage, error) {
	props := map[string]any{}

	var required []string

	// content — always included
	contentDesc := "The text content of the paste. Provide this OR file_path (not both)."
	if cfg.FileReadEnabled {
		contentDesc += " When file_path is provided instead, this field is not needed."
	}

	props["content"] = map[string]any{
		"type":        "string",
		"description": contentDesc,
	}

	if !cfg.FileReadEnabled {
		required = append(required, "content")
	}

	// file_path — only when FileReadEnabled
	if cfg.FileReadEnabled {
		filePathDesc := "Path to a local file to read and upload as paste content. " +
			"Provide this OR content (not both). The file must be a text file."

		if len(cfg.SandboxMounts) > 0 && !cfg.SandboxTransparent {
			filePathDesc += " Sandbox path translation is available: when " +
				"`translate_sandbox_path` is set to `true`, the path is " +
				"translated to the corresponding host path."
		}

		filePathDesc += " SECURITY: Only paths under ALLOWED_PATHS are accepted. " +
			"Blocked system paths (/etc, /proc, /sys, /dev by default) are rejected."

		props["file_path"] = map[string]any{
			"type":        "string",
			"description": filePathDesc,
		}
	}

	// extension — optional
	props["extension"] = map[string]any{
		"type": "string",
		"description": "File extension for syntax highlighting (e.g. 'go', 'py', 'js', " +
			"'md'). When using file_path, the extension is detected from the " +
			"file name if not provided.",
	}

	// expires — optional
	props["expires"] = map[string]any{
		"type": "string",
		"description": "Expiration time: bare number (seconds) or number plus unit " +
			"suffix (s, m, h, d, w, M=30d, y=365d). Examples: '3600', '1h', " +
			"'7d', '30M'. Defaults to server-configured expiry.",
	}

	// title — optional
	props["title"] = map[string]any{
		"type":        "string",
		"description": "Optional title for the paste.",
	}

	// burn_after_reading — optional
	props["burn_after_reading"] = map[string]any{
		"type":        "boolean",
		"description": "If true, the paste is deleted after being read once.",
	}

	// password — optional
	props["password"] = map[string]any{
		"type": "string",
		"description": "Optional password to protect the paste. NOTE: " +
			"Password-protected pastes cannot be retrieved via /raw/{id}; " +
			"use curl with the Wastebin-Password header or password query parameter instead.",
	}

	// translate_sandbox_path — only when mounts configured and not transparent
	if cfg.FileReadEnabled && len(cfg.SandboxMounts) > 0 && !cfg.SandboxTransparent {
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

// buildToolDescription returns the tool-level description for create_paste.
func buildToolDescription() string {
	return "Create a text paste on the configured Wastebin instance. " +
		"Use 'content' for inline text or 'file_path' to upload a local " +
		"file (when file mode is enabled). " +
		"Content supports multiple lines naturally — include newlines " +
		"directly in the string value. " +
		"The response includes 'hostname', " +
		"'id', 'url', and 'raw' fields. Reconstruct full URLs as " +
		"{hostname}{url} or {hostname}{raw}. " +
		"When 'extension' is 'md' or 'markdown', a 'markdown_rendered' " +
		"field appears with the rendered view URL. " +
		"Password-protected pastes require the Wastebin-Password header " +
		"or password query parameter for retrieval. " +
		"File mode requires ALLOWED_PATHS configuration."
}

type mcpInitializeMessage struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
}

const mcpInitializeMaxBytes = 1 << 20

var errInvalidMCPInitializeMessage = errors.New(
	"stdin does not contain a valid MCP initialize message",
)

// prepareMCPStdin reads the first line of stdin to verify it contains a valid
// MCP initialize message (JSON-RPC 2.0 with method "initialize"), preventing
// the MCP server from hanging when piped non-MCP input.
func prepareMCPStdin(stdin io.Reader) (io.Reader, error) {
	reader := bufio.NewReaderSize(stdin, mcpInitializeMaxBytes+1)

	firstLine, err := reader.ReadBytes('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		return nil, errInvalidMCPInitializeMessage
	}

	if err != nil && !errors.Is(err, io.EOF) {
		return nil, errInvalidMCPInitializeMessage
	}

	if len(firstLine) > mcpInitializeMaxBytes ||
		!isValidMCPInitializeMessage(firstLine) {
		return nil, errInvalidMCPInitializeMessage
	}

	return io.MultiReader(bytes.NewReader(firstLine), reader), nil
}

// isValidMCPInitializeMessage checks whether the given byte slice is a valid
// JSON-RPC 2.0 initialize message.
func isValidMCPInitializeMessage(line []byte) bool {
	if len(bytes.TrimSpace(line)) == 0 {
		return false
	}

	var msg mcpInitializeMessage

	err := json.Unmarshal(line, &msg)
	if err != nil {
		return false
	}

	return msg.JSONRPC == "2.0" && msg.Method == "initialize"
}

// runMCPMode starts the MCP stdio server, registers the create_paste tool,
// and blocks until a signal (SIGINT/SIGTERM) is received or the server exits.
func runMCPMode(cfg *wastebin.Config, stdin io.Reader) error {
	client, err := wastebin.NewWastebinClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create wastebin client: %w", err)
	}

	schema, err := buildPasteSchema(cfg)
	if err != nil {
		return fmt.Errorf("failed to build paste schema: %w", err)
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "wastebin-mcp-go",
		Version: version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_paste",
		Description: buildToolDescription(),
		InputSchema: schema,
	}, NewCreatePasteHandler(client))

	slog.Info("starting Wastebin MCP server")

	ctx, stop := signal.NotifyContext(
		context.Background(), syscall.SIGINT, syscall.SIGTERM,
	)
	defer stop()

	err = server.Run(ctx, &mcp.IOTransport{
		Reader: io.NopCloser(stdin),
		Writer: os.Stdout,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}

		return fmt.Errorf("server failed: %w", err)
	}

	return nil
}

// PasteCreator is the interface for creating a paste on the Wastebin server.
type PasteCreator interface {
	CreatePaste(ctx context.Context, args *wastebin.CreatePasteArgs) (*wastebin.PasteResponse, error)
}

// NewCreatePasteHandler creates an MCP tool handler for create_paste.
func NewCreatePasteHandler(
	client PasteCreator,
) func(
	context.Context,
	*mcp.CallToolRequest,
	wastebin.CreatePasteArgs,
) (*mcp.CallToolResult, any, error) {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		args wastebin.CreatePasteArgs,
	) (*mcp.CallToolResult, any, error) {
		resp, err := client.CreatePaste(ctx, &args)
		if err != nil {
			slog.Debug("create paste failed", "error", err)

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: "Create paste error: " + err.Error(),
					},
				},
				IsError: true,
			}, nil, nil
		}

		jsonBytes, err := json.Marshal(resp)
		if err != nil {
			slog.Debug(
				"failed to marshal paste response", "error", err,
			)

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: "Create paste error: failed to format results",
					},
				},
				IsError: true,
			}, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(jsonBytes)},
			},
		}, nil, nil
	}
}
