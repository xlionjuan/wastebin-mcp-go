// Package main provides the MCP stdio server for wastebin-mcp-go.
package main

import (
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
	return wastebin.NewSchemaBuilder(cfg).BuildToolSchema()
}

// newCreatePasteTool creates the create_paste tool definition with annotations
// marking it as non-destructive and closed-world: it only creates new pastes
// (additive), and targets a configured Wastebin instance with scoped file
// access rather than an open external domain.
func newCreatePasteTool(schema json.RawMessage, description string) *mcp.Tool {
	destructiveHint := false
	openWorldHint := false

	return &mcp.Tool{
		Name:        "create_paste",
		Description: description,
		InputSchema: schema,
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &destructiveHint,
			OpenWorldHint:   &openWorldHint,
		},
	}
}

// mcpFirstMessage is the JSON-RPC envelope of the first message an MCP client
// sends over stdin to start a session.
type mcpFirstMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// Valid JSON-RPC methods an MCP client may send as the first message of a
// session. Legacy clients use the "initialize" handshake; clients negotiating
// protocol version 2026-07-28 or later probe the stateless "server/discover"
// RPC first (SEP-2575).
const (
	mcpMethodInitialize = "initialize"
	mcpMethodDiscover   = "server/discover"
)

// metaKeyProtocolVersion is the per-request metadata key that marks a request
// as following the stateless (>= 2026-07-28) protocol (SEP-2575). It mirrors
// mcp.MetaKeyProtocolVersion in the go-sdk, which the server uses to detect
// stateless requests.
const metaKeyProtocolVersion = "io.modelcontextprotocol/protocolVersion"

// mcpFirstMessageEnvelopeAllowance is the headroom added to the configured max
// paste content size when bounding the first line of stdin. It covers the
// JSON-RPC envelope (method, id, params._meta, tool arguments) and typical JSON
// escaping of the embedded content, so a first-request tools/call carrying
// content up to WASTEBIN_MCP_MAX_CONTENT_SIZE passes the transport gate. The
// gate is a bounded-read guard, not a content validator; full content and
// metadata validation is left to the MCP SDK and the paste handler.
const mcpFirstMessageEnvelopeAllowance = 64 << 10 // 64 KiB

var (
	errInvalidMCPFirstMessage = errors.New(
		"stdin does not contain a valid MCP first message",
	)

	errMCPConfigRequired           = errors.New("config is required")
	errFirstMessageContentTooSmall = errors.New("WASTEBIN_MCP_MAX_CONTENT_SIZE must be at least 1")
	errFirstMessageContentTooLarge = errors.New("WASTEBIN_MCP_MAX_CONTENT_SIZE exceeds the maximum supported value")
)

// mcpFirstMessageMaxBytes returns the maximum wire size, in bytes, of the
// first line of stdin (the MCP session starter), excluding its trailing
// newline. It is derived from the configured max paste content size so that a
// first-request tools/call carrying content up to WASTEBIN_MCP_MAX_CONTENT_SIZE
// is admitted once the JSON-RPC envelope and JSON escaping are included. The
// configured size is validated against wastebin.MaxContentSizeLimit before the
// int conversion and the 64 KiB allowance addition, so the arithmetic cannot
// overflow on any supported platform (including 32-bit int targets). Heavy
// escaping (quotes, control characters, HTML metacharacters) expands the wire
// representation and lowers the effective first-call content ceiling; see
// docs/INSTALL.md.
func mcpFirstMessageMaxBytes(cfg *wastebin.Config) (int, error) {
	if cfg == nil {
		return 0, errMCPConfigRequired
	}

	maxSize := cfg.MaxContentSize
	if maxSize < 1 {
		return 0, errFirstMessageContentTooSmall
	}

	if maxSize > wastebin.MaxContentSizeLimit {
		return 0, fmt.Errorf(
			"%w (%d exceeds the maximum of %d)",
			errFirstMessageContentTooLarge, maxSize, wastebin.MaxContentSizeLimit,
		)
	}

	// maxSize is in [1, wastebin.MaxContentSizeLimit], a range far below the
	// int limits on every supported platform, so the conversion and the
	// allowance addition cannot overflow.
	return int(maxSize) + mcpFirstMessageEnvelopeAllowance, nil
}

// prepareMCPStdin reads the first line of stdin to verify it starts a valid
// MCP session (JSON-RPC 2.0 "initialize", "server/discover", or any request
// carrying the stateless protocol metadata), preventing the MCP server from
// hanging when piped non-MCP input. The line is bounded by
// mcpFirstMessageMaxBytes(cfg); readFirstLine rejects a first line as soon as
// it exceeds that bound, so memory usage stays bounded and the configured
// maximum is never allocated up front, even for attacker-controlled stdin.
func prepareMCPStdin(stdin io.Reader, cfg *wastebin.Config) (io.Reader, error) {
	maxBytes, err := mcpFirstMessageMaxBytes(cfg)
	if err != nil {
		return nil, err
	}

	firstLine, leftover, err := readFirstLine(stdin, maxBytes)
	if err != nil {
		return nil, errInvalidMCPFirstMessage
	}

	if !isValidMCPFirstMessage(firstLine) {
		return nil, errInvalidMCPFirstMessage
	}

	return io.MultiReader(bytes.NewReader(firstLine), bytes.NewReader(leftover), stdin), nil
}

// mcpFirstMessageReadChunkSize bounds how many bytes readFirstLine requests
// from stdin in a single read. The first line is accumulated in memory only up
// to the transport bound, and the reader never allocates the configured
// maximum up front.
const mcpFirstMessageReadChunkSize = 32 << 10 // 32 KiB

// readFirstLine reads the first line of stdin, accepting at most maxBytes
// message bytes plus a trailing newline, and returns the line including the
// newline together with any bytes that were read past it. It reads
// incrementally with a fixed-size buffer and never requests more bytes than
// the remaining budget, so the total number of bytes read is at most
// maxBytes+1 and memory usage stays bounded for attacker-controlled stdin. An
// oversized first line is rejected without buffering the entire line. A line
// without a trailing newline is accepted when it ends at EOF; content
// validation is done by prepareMCPStdin.
func readFirstLine(stdin io.Reader, maxBytes int) ([]byte, []byte, error) {
	line := make([]byte, 0, mcpFirstMessageReadChunkSize)
	buf := make([]byte, mcpFirstMessageReadChunkSize)

	for {
		want := min(maxBytes+1-len(line), len(buf))

		n, err := stdin.Read(buf[:want])
		if n > 0 {
			for i := range n {
				if buf[i] == '\n' {
					return append(line, '\n'), append([]byte(nil), buf[i+1:n]...), nil
				}

				line = append(line, buf[i])
				if len(line) > maxBytes {
					return nil, nil, errInvalidMCPFirstMessage
				}
			}
		}

		if n == 0 && err == nil {
			return nil, nil, errInvalidMCPFirstMessage
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(line) > 0 {
					return line, nil, nil
				}

				return nil, nil, errInvalidMCPFirstMessage
			}

			return nil, nil, errInvalidMCPFirstMessage
		}
	}
}

// isValidMCPFirstMessage checks whether the given byte slice is a valid
// JSON-RPC 2.0 message that can start an MCP session: the legacy "initialize"
// handshake, the stateless "server/discover" RPC, or any request carrying the
// per-request protocol metadata (the stateless protocol has no handshake).
func isValidMCPFirstMessage(line []byte) bool {
	if len(bytes.TrimSpace(line)) == 0 {
		return false
	}

	var msg mcpFirstMessage

	err := json.Unmarshal(line, &msg)
	if err != nil {
		return false
	}

	if msg.JSONRPC != "2.0" {
		return false
	}

	// The legacy "initialize" handshake and the stateless "server/discover"
	// probe are always valid session starters.
	if msg.Method == mcpMethodInitialize || msg.Method == mcpMethodDiscover {
		return true
	}

	// In the stateless protocol (>= 2026-07-28) there is no handshake: any
	// request (e.g. tools/list) may be the first one, provided it carries the
	// per-request protocol metadata in params._meta (SEP-2575). Full metadata
	// validation is left to the go-sdk.
	return msg.hasStatelessProtocolMeta()
}

// hasStatelessProtocolMeta reports whether the message carries the per-request
// metadata that marks it as following the stateless (>= 2026-07-28) protocol,
// in which any request may be the first one. Only the presence of the protocol
// version key is checked; the go-sdk validates the full metadata.
func (m mcpFirstMessage) hasStatelessProtocolMeta() bool {
	if len(m.Params) == 0 {
		return false
	}

	var params struct {
		//nolint:tagliatelle // matches the wire key "_meta"
		Meta map[string]json.RawMessage `json:"_meta"`
	}

	err := json.Unmarshal(m.Params, &params)
	if err != nil {
		return false
	}

	_, ok := params.Meta[metaKeyProtocolVersion]

	return ok
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

	sb := wastebin.NewSchemaBuilder(cfg)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "wastebin-mcp-go",
		Version: version,
	}, nil)

	mcp.AddTool(server, newCreatePasteTool(schema, sb.BuildToolDescription()), NewCreatePasteHandler(client))

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
