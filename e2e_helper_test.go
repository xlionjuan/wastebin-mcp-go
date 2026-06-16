//go:build e2e

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"wastebin-mcp-go/internal/wastebin"
)

// stderrBuffer is the interface for capturing stderr output from a running
// MCP subprocess. Both *bytes.Buffer and *safeBuffer satisfy it.
type stderrBuffer interface {
	String() string
	io.Writer
}

// safeBuffer is a thread-safe wrapper around bytes.Buffer for use when a
// subprocess writes to the buffer concurrently with test goroutines.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// =============================================================================
// Session lifecycle helpers
// =============================================================================

// validMCPInitialize is a minimal valid MCP initialize message to pass
// the stdin validation gate so we reach config validation.
const validMCPInitialize = `{"jsonrpc":"2.0","method":"initialize"}` + "\n"

var (
	e2eBinaryOnce     sync.Once
	e2eBinaryPath     string
	errE2EBinaryBuild error
)

// e2eMCPBinaryPath returns the path to a built MCP binary, reusing a single
// package-level build unless E2E_MCP_BINARY is set. This avoids rebuilding the
// binary for every top-level E2E test.
func e2eMCPBinaryPath(t *testing.T) string {
	t.Helper()

	if path := os.Getenv("E2E_MCP_BINARY"); path != "" {
		return path
	}

	e2eBinaryOnce.Do(func() {
		// This temp directory is intentionally leaked — it is a package-level
		// build cache that persists across all E2E tests to avoid rebuilding
		// the binary for every top-level test. The OS temp cleaner will
		// eventually reclaim it.
		//nolint:usetesting // package-level cache must outlive individual tests
		dir, err := os.MkdirTemp("", "wastebin-mcp-go-e2e-*")
		if err != nil {
			errE2EBinaryBuild = fmt.Errorf("create temp dir: %w", err)

			return
		}

		path := filepath.Join(dir, "wastebin-mcp-go")
		//nolint:gosec // test builds binary
		out, err := exec.CommandContext(context.Background(), "go", "build", "-o", path, ".").CombinedOutput()
		if err != nil {
			errE2EBinaryBuild = fmt.Errorf("go build: %w\noutput:\n%s", err, string(out))

			return
		}

		e2eBinaryPath = path
	})

	if errE2EBinaryBuild != nil {
		t.Fatalf("build E2E MCP binary failed: %v", errE2EBinaryBuild)
	}

	return e2eBinaryPath
}

// buildE2EMCPBinary compiles the binary and returns its path.
func buildE2EMCPBinary(ctx context.Context, t *testing.T) string {
	t.Helper()

	_ = ctx // cached build does not use per-call context

	return e2eMCPBinaryPath(t)
}

// e2eMCPEnv builds the environment variables for an MCP stdio session.
func e2eMCPEnv(wastebinURL string, extra ...string) []string {
	env := append(os.Environ(), "WASTEBIN_SERVER_URL="+wastebinURL, "WASTEBIN_MCP_FILE_READ_ENABLED=false")
	env = append(env, extra...)

	return env
}

// removeEnv returns a copy of env with the variable named key removed.
func removeEnv(env []string, key string) []string {
	var result []string

	prefix := key + "="
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			result = append(result, e)
		}
	}

	return result
}

// newMCPSession creates an MCP client, connects over stdio, and returns the
// session. The caller is responsible for cleanup.
func newMCPSession(
	ctx context.Context, t *testing.T, cmd *exec.Cmd,
	stderr stderrBuffer, clientName string,
) *mcp.ClientSession {
	t.Helper()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    clientName,
		Version: version,
	}, nil)

	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect MCP stdio session failed: %v\nstderr:\n%s", err, stderr.String())
	}

	return session
}

// startMCPSession starts an MCP stdio session with shared lifecycle. It
// resolves the binary path, builds the command, connects, and registers
// cleanup. Returns the session, stderr buffer, and command (for optional
// post-cleanup inspection).
//
// This helper uses the client name "wastebin-mcp-go-e2e-test".
func startMCPSession(
	ctx context.Context, t *testing.T, wastebinURL string,
	extraEnv ...string,
) (*mcp.ClientSession, *safeBuffer, *exec.Cmd) { //nolint:unparam // test helper returns cmd for optional caller use
	t.Helper()

	binaryPath := e2eMCPBinaryPath(t)

	t.Logf("using MCP binary: %s", binaryPath)

	var stderr safeBuffer

	cmd := exec.CommandContext(ctx, binaryPath) //nolint:gosec // test runs built binary
	cmd.Env = e2eMCPEnv(wastebinURL, extraEnv...)
	cmd.Stderr = &stderr

	var session *mcp.ClientSession

	t.Cleanup(func() {
		if session != nil {
			closeErr := session.Close()
			if closeErr != nil && !strings.Contains(closeErr.Error(), "signal: terminated") {
				t.Logf("close MCP session: %v\nstderr:\n%s", closeErr, stderr.String())
			}
		}

		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()    //nolint:errcheck // best-effort cleanup
			_, _ = cmd.Process.Wait() //nolint:errcheck // best-effort cleanup
		}
	})

	session = newMCPSession(ctx, t, cmd, &stderr, "wastebin-mcp-go-e2e-test")
	t.Log("MCP stdio session connected")

	return session, &stderr, cmd
}

// =============================================================================
// Tool call helpers
// =============================================================================

// createPaste calls the create_paste tool with the given arguments.
func createPaste(
	ctx context.Context,
	t *testing.T,
	session *mcp.ClientSession,
	arguments map[string]any,
	stderr stderrBuffer,
) *mcp.CallToolResult {
	t.Helper()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "create_paste",
		Arguments: arguments,
	})
	if err != nil {
		t.Fatalf("tools/call create_paste failed with arguments %#v: %v\nstderr:\n%s", arguments, err, stderr.String())
	}

	return result
}

// requirePasteResponse calls create_paste, asserts no tool error, parses
// the JSON response, and returns it. name is used for logging.
func requirePasteResponse(
	ctx context.Context,
	t *testing.T,
	session *mcp.ClientSession,
	arguments map[string]any,
	stderr stderrBuffer,
	name string,
) wastebin.PasteResponse {
	t.Helper()

	t.Logf("%s: sending arguments %#v", name, arguments)

	result := createPaste(ctx, t, session, arguments, stderr)

	if result.IsError {
		t.Fatalf("%s returned tool error: %s\nstderr:\n%s", name, toolText(t, result), stderr.String())
	}

	response := parsePasteResponse(t, result, stderr)
	t.Logf("%s parsed: id=%q, hostname=%q, url=%q, raw=%q",
		name, response.ID, response.Hostname, response.URL, response.Raw)

	return response
}

// parsePasteResponse unmarshals the tool result text into a PasteResponse.
func parsePasteResponse(t *testing.T, result *mcp.CallToolResult, stderr stderrBuffer) wastebin.PasteResponse {
	t.Helper()

	text := toolText(t, result)

	var response wastebin.PasteResponse

	err := json.Unmarshal([]byte(text), &response)
	if err != nil {
		t.Fatalf("create_paste tool text is not PasteResponse JSON: %v\ntext:\n%s\nstderr:\n%s", err, text, stderr.String())
	}

	return response
}

// findCreatePasteTool lists tools and returns the "create_paste" tool, verifying
// its InputSchema contains the expected properties. The expected properties are
// based on the default configuration (WASTEBIN_MCP_FILE_READ_ENABLED=false), so
// file_path and translate_sandbox_path are excluded from the schema.
func findCreatePasteTool(ctx context.Context, t *testing.T, session *mcp.ClientSession, stderr stderrBuffer) *mcp.Tool {
	t.Helper()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list failed: %v\nstderr:\n%s", err, stderr.String())
	}

	t.Logf("tools/list returned %d tools", len(tools.Tools))

	var tool *mcp.Tool

	for i, t2 := range tools.Tools {
		t.Logf("  tool: %s - %s", t2.Name, t2.Description)

		if t2.Name == "create_paste" {
			tool = tools.Tools[i]

			break
		}
	}

	if tool == nil {
		t.Fatalf("tools/list did not include create_paste tool; got %#v\nstderr:\n%s",
			tools.Tools, stderr.String())
	}

	// Validate InputSchema
	if tool.InputSchema == nil {
		t.Fatal("create_paste tool has no InputSchema")
	}

	schema := requireSchemaMap(t, tool.InputSchema, stderr)

	if schema["type"] != "object" {
		t.Errorf("create_paste InputSchema type = %v, want %q", schema["type"], "object")
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("create_paste InputSchema has no 'properties' or it is not an object")
	}

	expectedProps := []string{"content", "extension", "expires", "title", "burn_after_reading", "password"}

	for _, prop := range expectedProps {
		if _, exists := props[prop]; !exists {
			t.Errorf("create_paste InputSchema missing expected property %q", prop)
		}
	}

	// Check that the required field includes content
	requiredRaw, hasRequired := schema["required"]
	if hasRequired {
		switch req := requiredRaw.(type) {
		case []any:
			found := false

			for _, r := range req {
				if r == "content" {
					found = true

					break
				}
			}

			if !found {
				t.Errorf("create_paste InputSchema required does not include 'content'; got %v", req)
			}
		case []string:
			found := false

			for _, r := range req {
				if r == "content" {
					found = true

					break
				}
			}

			if !found {
				t.Errorf("create_paste InputSchema required does not include 'content'; got %v", req)
			}
		default:
			t.Errorf("create_paste InputSchema required has unexpected type %T: %v", requiredRaw, requiredRaw)
		}
	}

	return tool
}

// toolText extracts the text content from a tool result, failing if empty.
func toolText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()

	text, ok := toolTextFromResult(result)
	if !ok {
		t.Fatal("tool result has no content")
	}

	return text
}

// toolTextFromResult extracts text content from a tool result, returning
// false if content is missing or not text.
func toolTextFromResult(result *mcp.CallToolResult) (string, bool) {
	if len(result.Content) == 0 {
		return "", false
	}

	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return "", false
	}

	return textContent.Text, true
}

// =============================================================================
// Schema assertion helpers
// =============================================================================

func requireSchemaMap(t *testing.T, schema any, stderr stderrBuffer) map[string]any {
	t.Helper()

	if schemaMap, ok := schema.(map[string]any); ok {
		return schemaMap
	}

	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal InputSchema failed: %v\nschema type: %T\nstderr:\n%s", err, schema, stderr.String())
	}

	var schemaMap map[string]any

	err = json.Unmarshal(data, &schemaMap)
	if err != nil {
		t.Fatalf("unmarshal InputSchema failed: %v\nschema JSON: %s\nstderr:\n%s", err, string(data), stderr.String())
	}

	return schemaMap
}

func requireProperty(t *testing.T, props map[string]any, name string, stderr stderrBuffer) map[string]any {
	t.Helper()

	prop, ok := props[name].(map[string]any)
	if !ok {
		t.Fatalf("schema property %q type = %T, want map[string]any"+
			"\nproperties: %#v\nstderr:\n%s", name, props[name], props, stderr.String())
	}

	return prop
}

// schemaNumber extracts a JSON number field as float64. It returns 0 when the
// field is missing or not a number so callers can decide whether to enforce
// presence separately.
func schemaNumber(prop map[string]any, field string) float64 {
	v, ok := prop[field].(float64)
	if !ok {
		return 0
	}

	return v
}

// =============================================================================
// Warning summary helpers
// =============================================================================

// e2eWarnings is a goroutine-safe accumulator for test warnings. Tests should
// create one per top-level test function and call Report at the end.
type e2eWarnings struct {
	mu       sync.Mutex
	warnings []string
}

// Addf records a warning message. It is safe for concurrent calls.
func (w *e2eWarnings) Addf(format string, args ...any) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.warnings = append(w.warnings, fmt.Sprintf(format, args...))
}

// Report prints the WARNING SUMMARY block if any warnings were collected.
func (w *e2eWarnings) Report(t *testing.T) {
	t.Helper()

	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.warnings) > 0 {
		t.Logf("--- WARNING SUMMARY ---")

		for _, warning := range w.warnings {
			t.Logf("  WARN: %s", warning)
		}
	}
}
