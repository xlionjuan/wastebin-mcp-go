package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"wastebin-mcp-go/internal/wastebin"
)

func TestBuildPasteSchemaContentRequired(t *testing.T) {
	t.Parallel()

	cfg := wastebin.DefaultConfig()
	cfg.FileReadEnabled = false

	schema, err := buildPasteSchema(cfg)
	if err != nil {
		t.Fatalf("buildPasteSchema failed: %v", err)
	}

	var parsed map[string]any

	err = json.Unmarshal(schema, &parsed)
	if err != nil {
		t.Fatalf("failed to unmarshal schema: %v", err)
	}

	required, ok := parsed["required"].([]any)
	if !ok {
		t.Fatal("expected 'required' to be an array")
	}

	found := false

	for _, r := range required {
		if r == "content" {
			found = true

			break
		}
	}

	if !found {
		t.Error("expected 'content' to be required when FileReadEnabled=false")
	}

	props, ok := parsed["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected 'properties' to be an object")
	}

	if _, exists := props["file_path"]; exists {
		t.Error("expected no 'file_path' when FileReadEnabled=false")
	}

	if _, exists := props["translate_sandbox_path"]; exists {
		t.Error("expected no 'translate_sandbox_path' when FileReadEnabled=false")
	}
}

func TestBuildPasteSchemaFileModeEnabled(t *testing.T) {
	t.Parallel()

	cfg := wastebin.DefaultConfig()
	cfg.FileReadEnabled = true

	schema, err := buildPasteSchema(cfg)
	if err != nil {
		t.Fatalf("buildPasteSchema failed: %v", err)
	}

	var parsed map[string]any

	err = json.Unmarshal(schema, &parsed)
	if err != nil {
		t.Fatalf("failed to unmarshal schema: %v", err)
	}

	if _, ok := parsed["required"]; ok {
		t.Error("expected no 'required' when FileReadEnabled=true (content is optional)")
	}

	props, ok := parsed["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected 'properties' to be an object")
	}

	if _, exists := props["file_path"]; !exists {
		t.Error("expected 'file_path' when FileReadEnabled=true")
	}

	if _, exists := props["translate_sandbox_path"]; exists {
		t.Error("expected no 'translate_sandbox_path' when no sandbox mounts")
	}
}

func TestBuildPasteSchemaSandboxMounts(t *testing.T) {
	t.Parallel()

	cfg := wastebin.DefaultConfig()
	cfg.FileReadEnabled = true
	cfg.SandboxMounts = []wastebin.SandboxMount{
		{HostPath: "/host/data", SandboxPath: "/sandbox/data"},
	}
	cfg.SandboxTransparent = false

	schema, err := buildPasteSchema(cfg)
	if err != nil {
		t.Fatalf("buildPasteSchema failed: %v", err)
	}

	var parsed map[string]any

	err = json.Unmarshal(schema, &parsed)
	if err != nil {
		t.Fatalf("failed to unmarshal schema: %v", err)
	}

	props, ok := parsed["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected 'properties' to be an object")
	}

	if _, exists := props["translate_sandbox_path"]; !exists {
		t.Error("expected 'translate_sandbox_path' when mounts configured and not transparent")
	}
}

func TestBuildPasteSchemaFilteredSandboxMounts(t *testing.T) {
	tmpDir := t.TempDir()
	allowedDir := filepath.Join(tmpDir, "allowed")
	disallowedDir := filepath.Join(tmpDir, "disallowed")

	for _, dir := range []string{allowedDir, disallowedDir} {
		err := os.Mkdir(dir, 0o750)
		if err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("WASTEBIN_SERVER_URL", "https://bin.example.com")
	t.Setenv("WASTEBIN_MCP_ALLOWED_PATHS", allowedDir)
	t.Setenv("WASTEBIN_MCP_SANDBOX_MOUNTS", disallowedDir+":/workspace")

	cfg, err := wastebin.ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv failed: %v", err)
	}

	schema, err := buildPasteSchema(cfg)
	if err != nil {
		t.Fatalf("buildPasteSchema failed: %v", err)
	}

	var parsed map[string]any

	err = json.Unmarshal(schema, &parsed)
	if err != nil {
		t.Fatalf("failed to unmarshal schema: %v", err)
	}

	props, ok := parsed["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected 'properties' to be an object")
	}

	if _, exists := props["translate_sandbox_path"]; exists {
		t.Error("expected no 'translate_sandbox_path' after all mounts are filtered out")
	}
}

func TestBuildPasteSchemaSandboxTransparent(t *testing.T) {
	t.Parallel()

	cfg := wastebin.DefaultConfig()
	cfg.FileReadEnabled = true
	cfg.SandboxMounts = []wastebin.SandboxMount{
		{HostPath: "/host/data", SandboxPath: "/sandbox/data"},
	}
	cfg.SandboxTransparent = true

	schema, err := buildPasteSchema(cfg)
	if err != nil {
		t.Fatalf("buildPasteSchema failed: %v", err)
	}

	var parsed map[string]any

	err = json.Unmarshal(schema, &parsed)
	if err != nil {
		t.Fatalf("failed to unmarshal schema: %v", err)
	}

	props, ok := parsed["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected 'properties' to be an object")
	}

	if _, exists := props["translate_sandbox_path"]; exists {
		t.Error("expected no 'translate_sandbox_path' when SandboxTransparent=true")
	}
}

func TestBuildPasteSchemaAdditionalProperties(t *testing.T) {
	t.Parallel()

	cfg := wastebin.DefaultConfig()

	schema, err := buildPasteSchema(cfg)
	if err != nil {
		t.Fatalf("buildPasteSchema failed: %v", err)
	}

	var parsed map[string]any

	err = json.Unmarshal(schema, &parsed)
	if err != nil {
		t.Fatalf("failed to unmarshal schema: %v", err)
	}

	if addProps, ok := parsed["additionalProperties"]; !ok || addProps != false {
		t.Error("expected additionalProperties to be false")
	}
}

func TestBuildPasteSchemaFilePathDescription_AllowedPathsConfigured(t *testing.T) {
	t.Parallel()

	cfg := wastebin.DefaultConfig()
	cfg.FileReadEnabled = true
	cfg.AllowedPaths = []string{"/home/allowed"}

	schema, err := buildPasteSchema(cfg)
	if err != nil {
		t.Fatalf("buildPasteSchema failed: %v", err)
	}

	var parsed map[string]any

	err = json.Unmarshal(schema, &parsed)
	if err != nil {
		t.Fatalf("failed to unmarshal schema: %v", err)
	}

	props, ok := parsed["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected 'properties' to be an object")
	}

	filePath, exists := props["file_path"]
	if !exists {
		t.Fatal("expected 'file_path' property")
	}

	filePathMap, ok := filePath.(map[string]any)
	if !ok {
		t.Fatalf("expected file_path to be a map, got %T", filePath)
	}

	desc, ok := filePathMap["description"].(string)
	if !ok {
		t.Fatalf("expected file_path description to be a string, got %T", filePathMap["description"])
	}

	if !strings.Contains(desc, "ALLOWED_PATHS") {
		t.Error("expected file_path description to mention ALLOWED_PATHS when configured")
	}

	if !strings.Contains(desc, "Blocked system paths") {
		t.Error("expected file_path description to mention blocked system paths")
	}
}

func TestBuildPasteSchemaFilePathDescription_NoAllowedPaths(t *testing.T) {
	t.Parallel()

	cfg := wastebin.DefaultConfig()
	cfg.FileReadEnabled = true
	cfg.AllowedPaths = nil

	schema, err := buildPasteSchema(cfg)
	if err != nil {
		t.Fatalf("buildPasteSchema failed: %v", err)
	}

	var parsed map[string]any

	err = json.Unmarshal(schema, &parsed)
	if err != nil {
		t.Fatalf("failed to unmarshal schema: %v", err)
	}

	props, ok := parsed["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected 'properties' to be an object")
	}

	filePath, exists := props["file_path"]
	if !exists {
		t.Fatal("expected 'file_path' property")
	}

	filePathMap, ok := filePath.(map[string]any)
	if !ok {
		t.Fatalf("expected file_path to be a map, got %T", filePath)
	}

	desc, ok := filePathMap["description"].(string)
	if !ok {
		t.Fatalf("expected file_path description to be a string, got %T", filePathMap["description"])
	}

	if !strings.Contains(desc, "blocklist pipeline") {
		t.Error("expected file_path description to mention blocklist pipeline when no ALLOWED_PATHS")
	}

	if !strings.Contains(desc, "Blocked system paths") {
		t.Error("expected file_path description to mention blocked system paths")
	}
}

func TestBuildPasteSchemaBasicFields(t *testing.T) {
	t.Parallel()

	cfg := wastebin.DefaultConfig()

	schema, err := buildPasteSchema(cfg)
	if err != nil {
		t.Fatalf("buildPasteSchema failed: %v", err)
	}

	var parsed map[string]any

	err = json.Unmarshal(schema, &parsed)
	if err != nil {
		t.Fatalf("failed to unmarshal schema: %v", err)
	}

	props, ok := parsed["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected 'properties' to be an object")
	}

	expectedFields := []string{
		"content",
		"extension",
		"expires",
		"title",
		"burn_after_reading",
		"password",
	}

	for _, field := range expectedFields {
		if _, exists := props[field]; !exists {
			t.Errorf("expected property %q to exist", field)
		}
	}

	// Password must have minLength: 1.
	passwordProp, ok := props["password"]
	if !ok {
		t.Fatal("expected 'password' property to exist")
	}

	passwordMap, ok := passwordProp.(map[string]any)
	if !ok {
		t.Fatalf("expected password to be a map, got %T", passwordProp)
	}

	minLength, ok := passwordMap["minLength"]
	if !ok {
		t.Fatal("expected password to have minLength")
	}

	if minLength != float64(1) {
		t.Errorf("expected password minLength=1, got %v", minLength)
	}

	passwordDesc, ok := passwordMap["description"].(string)
	if !ok {
		t.Fatalf("expected password description to be a string, got %T", passwordMap["description"])
	}

	if !strings.Contains(passwordDesc, "Wastebin-Password header") {
		t.Error("password description should mention Wastebin-Password header")
	}

	if strings.Contains(passwordDesc, "query parameter") {
		t.Error("password description must not recommend query-parameter password transport (URL secrets are logged)")
	}
}

func TestCreatePasteToolAnnotations(t *testing.T) {
	t.Parallel()

	tool := newCreatePasteTool(json.RawMessage(`{}`), "test description")

	if tool.Annotations == nil {
		t.Fatal("create_paste tool should have ToolAnnotations set")
	}

	if tool.Annotations.DestructiveHint == nil {
		t.Fatal("create_paste tool DestructiveHint should be set (non-nil)")
	}

	if *tool.Annotations.DestructiveHint {
		t.Error("create_paste tool DestructiveHint should be false (additive only)")
	}

	if tool.Annotations.OpenWorldHint == nil {
		t.Fatal("create_paste tool OpenWorldHint should be set (non-nil)")
	}

	if *tool.Annotations.OpenWorldHint {
		t.Error("create_paste tool OpenWorldHint should be false (closed domain)")
	}
}

func TestCreatePasteToolName(t *testing.T) {
	t.Parallel()

	tool := newCreatePasteTool(json.RawMessage(`{}`), "test description")

	if tool.Name != "create_paste" {
		t.Errorf("expected tool name 'create_paste', got %q", tool.Name)
	}
}

func TestBuildToolDescription(t *testing.T) {
	t.Parallel()

	desc := wastebin.NewSchemaBuilder(wastebin.DefaultConfig()).BuildToolDescription()

	if !strings.Contains(desc, "content") {
		t.Error("expected description to mention content")
	}

	if !strings.Contains(desc, "file_path") {
		t.Error("expected description to mention file_path")
	}

	if !strings.Contains(desc, "hostname") {
		t.Error("expected description to mention hostname")
	}

	if !strings.Contains(desc, "raw") {
		t.Error("expected description to mention raw")
	}

	if !strings.Contains(desc, "markdown_rendered") {
		t.Error("expected description to mention markdown_rendered")
	}

	if !strings.Contains(desc, "Wastebin-Password header") {
		t.Error("tool description should mention Wastebin-Password header")
	}

	if strings.Contains(desc, "query parameter") {
		t.Error("tool description must not recommend query-parameter password transport (URL secrets are logged)")
	}
}

// ---------------------------------------------------------------------------
// isValidMCPFirstMessage tests
// ---------------------------------------------------------------------------

func TestIsValidMCPFirstMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    []byte
		expected bool
	}{
		{
			name:     "valid initialize",
			input:    []byte(`{"jsonrpc":"2.0","method":"initialize"}`),
			expected: true,
		},
		{
			name:     "valid server discover",
			input:    []byte(`{"jsonrpc":"2.0","method":"server/discover"}`),
			expected: true,
		},
		{
			name: "server discover with params",
			input: []byte(
				`{"jsonrpc":"2.0","method":"server/discover","params":{"_meta":{` +
					`"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`,
			),
			expected: true,
		},
		{
			name: "stateless tools/list first request",
			input: []byte(
				`{"jsonrpc":"2.0","method":"tools/list","params":{"_meta":{` +
					`"io.modelcontextprotocol/clientCapabilities":{},` +
					`"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`,
			),
			expected: true,
		},
		{
			name: "stateless request with legacy method name",
			input: []byte(
				`{"jsonrpc":"2.0","method":"notifications/initialized","params":{"_meta":{` +
					`"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`,
			),
			expected: true,
		},
		{
			name:     "tools/list without stateless meta",
			input:    []byte(`{"jsonrpc":"2.0","method":"tools/list"}`),
			expected: false,
		},
		{
			name: "tools/list with meta but no protocolVersion",
			input: []byte(
				`{"jsonrpc":"2.0","method":"tools/list","params":{"_meta":{` +
					`"io.modelcontextprotocol/clientCapabilities":{}}}}`,
			),
			expected: false,
		},
		{
			name: "stateless request with wrong jsonrpc version",
			input: []byte(
				`{"jsonrpc":"1.0","method":"tools/list","params":{"_meta":{` +
					`"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`,
			),
			expected: false,
		},
		{
			name: "stateless request with non-object params",
			input: []byte(
				`{"jsonrpc":"2.0","method":"tools/list","params":[1,2,3]}`,
			),
			expected: false,
		},
		{
			name: "stateless request with non-object meta",
			input: []byte(
				`{"jsonrpc":"2.0","method":"tools/list","params":{"_meta":"nope"}}`,
			),
			expected: false,
		},
		{
			name:     "wrong method",
			input:    []byte(`{"jsonrpc":"2.0","method":"ping"}`),
			expected: false,
		},
		{
			name:     "wrong jsonrpc version initialize",
			input:    []byte(`{"jsonrpc":"1.0","method":"initialize"}`),
			expected: false,
		},
		{
			name:     "wrong jsonrpc version server discover",
			input:    []byte(`{"jsonrpc":"1.0","method":"server/discover"}`),
			expected: false,
		},
		{
			name:     "empty input",
			input:    []byte{},
			expected: false,
		},
		{
			name:     "non-JSON",
			input:    []byte(`not json at all`),
			expected: false,
		},
		{
			name:     "whitespace only",
			input:    []byte("   	\n  "),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isValidMCPFirstMessage(tt.input)

			if got != tt.expected {
				t.Errorf(
					"isValidMCPFirstMessage(%q) = %v, want %v",
					string(tt.input), got, tt.expected,
				)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// prepareMCPStdin tests
// ---------------------------------------------------------------------------

func TestPrepareMCPStdin_ValidWithExtraContent(t *testing.T) {
	t.Parallel()

	input := `{"jsonrpc":"2.0","method":"initialize"}
some extra content
more content`
	stdin := strings.NewReader(input)

	reader, err := prepareMCPStdin(stdin, wastebin.DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatalf("failed to read result: %v", readErr)
	}

	if string(got) != input {
		t.Errorf("reader should contain the full original stdin\nwant: %q\ngot:  %q", input, string(got))
	}
}

func TestPrepareMCPStdin_ValidServerDiscover(t *testing.T) {
	t.Parallel()

	input := `{"jsonrpc":"2.0","method":"server/discover","params":{"_meta":{` +
		`"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}` + "\n" +
		`more content`
	stdin := strings.NewReader(input)

	reader, err := prepareMCPStdin(stdin, wastebin.DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatalf("failed to read result: %v", readErr)
	}

	if string(got) != input {
		t.Errorf("reader should contain the full original stdin\nwant: %q\ngot:  %q", input, string(got))
	}
}

func TestPrepareMCPStdin_ValidStatelessFirstRequest(t *testing.T) {
	t.Parallel()

	// In the stateless protocol (>= 2026-07-28) there is no handshake: a
	// direct tools/list request carrying the per-request protocol metadata is
	// a valid first message (SEP-2575).
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{` +
		`"io.modelcontextprotocol/clientCapabilities":{},` +
		`"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}` + "\n" +
		`more content`
	stdin := strings.NewReader(input)

	reader, err := prepareMCPStdin(stdin, wastebin.DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatalf("failed to read result: %v", readErr)
	}

	if string(got) != input {
		t.Errorf("reader should contain the full original stdin\nwant: %q\ngot:  %q", input, string(got))
	}
}

func TestPrepareMCPStdin_ValidMessageWithoutNewlineAtEOF(t *testing.T) {
	t.Parallel()

	// A valid first message that ends at EOF without a trailing newline must
	// still be accepted.
	input := `{"jsonrpc":"2.0","method":"initialize"}`
	stdin := strings.NewReader(input)

	reader, err := prepareMCPStdin(stdin, wastebin.DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatalf("failed to read result: %v", readErr)
	}

	if string(got) != input {
		t.Errorf("reader should contain the full original stdin\nwant: %q\ngot:  %q", input, string(got))
	}
}

func TestPrepareMCPStdin_EmptyStdin(t *testing.T) {
	t.Parallel()

	stdin := strings.NewReader("")

	_, err := prepareMCPStdin(stdin, wastebin.DefaultConfig())
	if !errors.Is(err, errInvalidMCPFirstMessage) {
		t.Errorf("expected errInvalidMCPFirstMessage, got %v", err)
	}
}

func TestPrepareMCPStdin_NonMCPStdin(t *testing.T) {
	t.Parallel()

	stdin := strings.NewReader("not an MCP first message\n")

	_, err := prepareMCPStdin(stdin, wastebin.DefaultConfig())
	if !errors.Is(err, errInvalidMCPFirstMessage) {
		t.Errorf("expected errInvalidMCPFirstMessage, got %v", err)
	}
}

func TestMCPFirstMessageMaxBytes(t *testing.T) {
	t.Parallel()

	cfg := wastebin.DefaultConfig()
	if got, want := mcpFirstMessageMaxBytes(cfg), int(cfg.MaxContentSize)+mcpFirstMessageEnvelopeAllowance; got != want {
		t.Errorf("mcpFirstMessageMaxBytes(default) = %d, want %d", got, want)
	}

	cfg.MaxContentSize = 512 * 1024
	if got, want := mcpFirstMessageMaxBytes(cfg), 512*1024+mcpFirstMessageEnvelopeAllowance; got != want {
		t.Errorf("mcpFirstMessageMaxBytes(512 KiB) = %d, want %d", got, want)
	}
}

func TestPrepareMCPStdin_FirstRequestNearContentLimit(t *testing.T) {
	t.Parallel()

	cfg := wastebin.DefaultConfig()

	// Regression for the order-dependent payload ceiling: a first-request
	// tools/call with content just below WASTEBIN_MCP_MAX_CONTENT_SIZE exceeds
	// a fixed 1 MiB first-line cap once the JSON envelope is included, but must
	// be accepted when the gate is derived from the configured content limit.
	content := strings.Repeat("a", int(cfg.MaxContentSize)-76)

	msg := statelessToolsCallFirstRequest(content)
	if len(msg) <= int(cfg.MaxContentSize) {
		t.Fatalf("test setup: expected wire size above the content limit, got %d", len(msg))
	}

	reader, err := prepareMCPStdin(strings.NewReader(msg), cfg)
	if err != nil {
		t.Fatalf("expected near-limit first-request tools/call to be accepted, got: %v", err)
	}

	got, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatalf("failed to read result: %v", readErr)
	}

	if len(got) != len(msg) {
		t.Errorf("replayed stdin length = %d, want %d", len(got), len(msg))
	}
}

func TestPrepareMCPStdin_FirstRequestExactLimit(t *testing.T) {
	t.Parallel()

	cfg := wastebin.DefaultConfig()
	maxBytes := mcpFirstMessageMaxBytes(cfg)

	msg := statelessToolsCallOfWireSize(t, maxBytes)

	// The exact-limit message with a trailing newline forms a line of
	// maxBytes+1 bytes, which fits the reader's buffer (maxBytes+1).
	input := msg + "\n" + "tail"

	reader, err := prepareMCPStdin(strings.NewReader(input), cfg)
	if err != nil {
		t.Fatalf("expected exact-limit first request to be accepted, got: %v", err)
	}

	got, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatalf("failed to read result: %v", readErr)
	}

	if len(got) != len(input) {
		t.Errorf("replayed stdin length = %d, want %d", len(got), len(input))
	}
}

func TestPrepareMCPStdin_FirstRequestOverLimit(t *testing.T) {
	t.Parallel()

	cfg := wastebin.DefaultConfig()
	maxBytes := mcpFirstMessageMaxBytes(cfg)

	// A syntactically valid first request one byte over the limit. The gate
	// must reject it for its size (bounded read), not because the JSON is
	// invalid: the counting reader proves the gate stopped at the bound.
	msg := statelessToolsCallOfWireSize(t, maxBytes+1)

	counted := &countingReader{r: strings.NewReader(msg)}

	_, err := prepareMCPStdin(counted, cfg)
	if !errors.Is(err, errInvalidMCPFirstMessage) {
		t.Fatalf("expected errInvalidMCPFirstMessage for over-limit first request, got %v", err)
	}

	if counted.n > maxBytes+1 {
		t.Errorf("gate consumed %d bytes, want at most %d (bounded read)", counted.n, maxBytes+1)
	}
}

func TestPrepareMCPStdin_BoundedReadOfOversizedInput(t *testing.T) {
	t.Parallel()

	cfg := wastebin.DefaultConfig()
	maxBytes := mcpFirstMessageMaxBytes(cfg)

	// An attacker pipes a first line far larger than any valid MCP first
	// message. The gate must stop reading at the buffer bound instead of
	// buffering the whole line.
	huge := bytes.Repeat([]byte("a"), 8*maxBytes+1)
	counted := &countingReader{r: bytes.NewReader(huge)}

	_, err := prepareMCPStdin(counted, cfg)
	if !errors.Is(err, errInvalidMCPFirstMessage) {
		t.Fatalf("expected errInvalidMCPFirstMessage for oversized line, got %v", err)
	}

	if counted.n > maxBytes+1 {
		t.Errorf("gate consumed %d bytes, want at most %d (bounded read)", counted.n, maxBytes+1)
	}
}

// countingReader wraps an io.Reader and records the total number of bytes the
// underlying reader delivered, so tests can assert how much of an
// attacker-controlled stdin the first-message gate actually consumed.
type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n

	return n, err
}

// statelessToolsCallFirstRequest returns a syntactically valid stateless
// (>= 2026-07-28) tools/call first request for create_paste with the given
// inline content, without a trailing newline. The content is JSON-encoded, so
// the returned wire size includes escaping overhead.
func statelessToolsCallFirstRequest(content string) string {
	encoded, err := json.Marshal(content)
	if err != nil {
		panic(err)
	}

	return `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"_meta":{` +
		`"io.modelcontextprotocol/clientCapabilities":{},` +
		`"io.modelcontextprotocol/clientInfo":{"name":"test","version":"0.0.0"},` +
		`"io.modelcontextprotocol/protocolVersion":"2026-07-28"},` +
		`"name":"create_paste","arguments":{"content":` + string(encoded) + `}}}`
}

// statelessToolsCallOfWireSize returns a valid stateless tools/call first
// request whose wire length, excluding any trailing newline, is exactly target
// bytes. The content is padded with 'a' characters, which JSON escaping does
// not expand, so the size is exact.
func statelessToolsCallOfWireSize(t *testing.T, target int) string {
	t.Helper()

	envelope := len(statelessToolsCallFirstRequest(""))
	if target < envelope {
		t.Fatalf("target wire size %d is below the %d-byte JSON envelope", target, envelope)
	}

	msg := statelessToolsCallFirstRequest(strings.Repeat("a", target-envelope))
	if len(msg) != target {
		t.Fatalf("wire size mismatch: got %d, want %d", len(msg), target)
	}

	return msg
}

// errFailReader is the sentinel error returned by failReader.
var errFailReader = errors.New("simulated read failure")

// failReader returns an error on every Read call.
type failReader struct{}

func (failReader) Read(_ []byte) (int, error) {
	return 0, errFailReader
}

func TestPrepareMCPStdin_ReadError(t *testing.T) {
	t.Parallel()

	stdin := &failReader{}

	_, err := prepareMCPStdin(stdin, wastebin.DefaultConfig())
	if !errors.Is(err, errInvalidMCPFirstMessage) {
		t.Errorf("expected errInvalidMCPFirstMessage for read error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// NewCreatePasteHandler tests
// ---------------------------------------------------------------------------

func TestNewCreatePasteHandler_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": "/ABC123.go"}) //nolint:errcheck // Test helper OK
	}))
	defer server.Close()

	cfg := wastebin.DefaultConfig()
	cfg.ServerURL = server.URL

	client, err := wastebin.NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	handler := NewCreatePasteHandler(client)

	content := "hello world"
	ext := "go"

	result, _, handlerErr := handler(
		context.Background(),
		&mcp.CallToolRequest{},
		wastebin.CreatePasteArgs{
			Content:   &content,
			Extension: &ext,
		},
	)
	if handlerErr != nil {
		t.Fatalf("unexpected handler error: %v", handlerErr)
	}

	if result == nil {
		t.Fatal("expected non-nil CallToolResult")
	}

	if result.IsError {
		t.Fatal("expected IsError to be false")
	}

	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}

	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	var resp wastebin.PasteResponse

	err = json.Unmarshal([]byte(tc.Text), &resp)
	if err != nil {
		t.Fatalf("failed to unmarshal response JSON: %v", err)
	}

	if resp.ID != "ABC123" {
		t.Errorf("expected ID 'ABC123', got %q", resp.ID)
	}

	if resp.URL != "/ABC123.go" {
		t.Errorf("expected URL '/ABC123.go', got %q", resp.URL)
	}

	if resp.Hostname != server.URL {
		t.Errorf("expected hostname %q, got %q", server.URL, resp.Hostname)
	}

	if resp.Raw != "/raw/ABC123.go" {
		t.Errorf("expected Raw '/raw/ABC123.go', got %q", resp.Raw)
	}
}

func TestNewCreatePasteHandler_Failure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error")) //nolint:errcheck // Test helper
	}))
	defer server.Close()

	cfg := wastebin.DefaultConfig()
	cfg.ServerURL = server.URL

	client, err := wastebin.NewWastebinClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	handler := NewCreatePasteHandler(client)

	content := "hello world"

	result, _, handlerErr := handler(
		context.Background(),
		&mcp.CallToolRequest{},
		wastebin.CreatePasteArgs{
			Content: &content,
		},
	)
	if handlerErr != nil {
		t.Fatalf("unexpected handler error: %v", handlerErr)
	}

	if result == nil {
		t.Fatal("expected non-nil CallToolResult")
	}

	if !result.IsError {
		t.Fatal("expected IsError to be true")
	}

	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}

	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	if !strings.Contains(tc.Text, "Create paste error") {
		t.Errorf("expected error message to contain 'Create paste error', got: %q", tc.Text)
	}
}

func TestRunMCPMode_ClientError(t *testing.T) {
	t.Parallel()

	// Config without ServerURL should make NewWastebinClient fail.
	cfg := &wastebin.Config{}
	stdin := strings.NewReader("")

	err := runMCPMode(cfg, stdin)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "failed to create wastebin client") {
		t.Errorf("expected client creation error, got: %v", err)
	}
}
