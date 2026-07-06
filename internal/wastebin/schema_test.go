package wastebin //nolint:testpackage // white-box tests need access to unexported types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSchemaBuilder_ContentRequired(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.FileReadEnabled = false

	schema, err := NewSchemaBuilder(cfg).BuildToolSchema()
	if err != nil {
		t.Fatalf("BuildToolSchema failed: %v", err)
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

func TestSchemaBuilder_FileModeEnabled(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.FileReadEnabled = true

	schema, err := NewSchemaBuilder(cfg).BuildToolSchema()
	if err != nil {
		t.Fatalf("BuildToolSchema failed: %v", err)
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

func TestSchemaBuilder_SandboxMounts(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.FileReadEnabled = true
	cfg.SandboxMounts = []SandboxMount{
		{HostPath: "/host/data", SandboxPath: "/sandbox/data"},
	}
	cfg.SandboxTransparent = false

	schema, err := NewSchemaBuilder(cfg).BuildToolSchema()
	if err != nil {
		t.Fatalf("BuildToolSchema failed: %v", err)
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

func TestSchemaBuilder_SandboxTransparent(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.FileReadEnabled = true
	cfg.SandboxMounts = []SandboxMount{
		{HostPath: "/host/data", SandboxPath: "/sandbox/data"},
	}
	cfg.SandboxTransparent = true

	schema, err := NewSchemaBuilder(cfg).BuildToolSchema()
	if err != nil {
		t.Fatalf("BuildToolSchema failed: %v", err)
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

func TestSchemaBuilder_AdditionalProperties(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()

	schema, err := NewSchemaBuilder(cfg).BuildToolSchema()
	if err != nil {
		t.Fatalf("BuildToolSchema failed: %v", err)
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

func TestSchemaBuilder_FilePathDescription_AllowedPathsConfigured(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.FileReadEnabled = true
	cfg.AllowedPaths = []string{"/home/allowed"}

	schema, err := NewSchemaBuilder(cfg).BuildToolSchema()
	if err != nil {
		t.Fatalf("BuildToolSchema failed: %v", err)
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

func TestSchemaBuilder_FilePathDescription_NoAllowedPaths(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.FileReadEnabled = true
	cfg.AllowedPaths = nil

	schema, err := NewSchemaBuilder(cfg).BuildToolSchema()
	if err != nil {
		t.Fatalf("BuildToolSchema failed: %v", err)
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

func TestSchemaBuilder_BasicFields(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()

	schema, err := NewSchemaBuilder(cfg).BuildToolSchema()
	if err != nil {
		t.Fatalf("BuildToolSchema failed: %v", err)
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

	// Password description must not recommend query-string secret transport.
	passwordProp, ok := props["password"]
	if !ok {
		t.Fatal("expected 'password' property to exist")
	}

	passwordMap, ok := passwordProp.(map[string]any)
	if !ok {
		t.Fatalf("expected password to be a map, got %T", passwordProp)
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

func TestSchemaBuilder_BuildToolDescription(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	desc := NewSchemaBuilder(cfg).BuildToolDescription()

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

func TestSchemaBuilder_SchemaTypeObject(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()

	schema, err := NewSchemaBuilder(cfg).BuildToolSchema()
	if err != nil {
		t.Fatalf("BuildToolSchema failed: %v", err)
	}

	var parsed map[string]any

	err = json.Unmarshal(schema, &parsed)
	if err != nil {
		t.Fatalf("failed to unmarshal schema: %v", err)
	}

	if typ, ok := parsed["type"]; !ok || typ != "object" {
		t.Errorf("expected type 'object', got %v", typ)
	}
}

func TestSchemaBuilder_ExpiresDescriptionIncludesDefault(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.DefaultExpires = 3600 // 1 hour

	schema, err := NewSchemaBuilder(cfg).BuildToolSchema()
	if err != nil {
		t.Fatalf("BuildToolSchema failed: %v", err)
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

	expires, ok := props["expires"].(map[string]any)
	if !ok {
		t.Fatal("expected 'expires' property")
	}

	desc, ok := expires["description"].(string)
	if !ok {
		t.Fatal("expected expires description to be a string")
	}

	if !strings.Contains(desc, "1 hour") {
		t.Errorf("expected expires description to mention '1 hour', got: %s", desc)
	}
}

func TestSchemaBuilder_NoSandboxMounts_NoTranslateParam(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.FileReadEnabled = true
	cfg.SandboxMounts = nil

	schema, err := NewSchemaBuilder(cfg).BuildToolSchema()
	if err != nil {
		t.Fatalf("BuildToolSchema failed: %v", err)
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
		t.Error("expected no 'translate_sandbox_path' when no sandbox mounts")
	}
}

func TestSchemaBuilder_ContentDescriptionWithFileMode(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.FileReadEnabled = true

	schema, err := NewSchemaBuilder(cfg).BuildToolSchema()
	if err != nil {
		t.Fatalf("BuildToolSchema failed: %v", err)
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

	content, ok := props["content"].(map[string]any)
	if !ok {
		t.Fatal("expected 'content' property")
	}

	desc, ok := content["description"].(string)
	if !ok {
		t.Fatal("expected content description to be a string")
	}

	if !strings.Contains(desc, "file_path") {
		t.Error("expected content description to mention file_path when file mode is enabled")
	}
}

func TestSchemaBuilder_ContentDescriptionWithoutFileMode(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.FileReadEnabled = false

	schema, err := NewSchemaBuilder(cfg).BuildToolSchema()
	if err != nil {
		t.Fatalf("BuildToolSchema failed: %v", err)
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

	content, ok := props["content"].(map[string]any)
	if !ok {
		t.Fatal("expected 'content' property")
	}

	desc, ok := content["description"].(string)
	if !ok {
		t.Fatal("expected content description to be a string")
	}

	if strings.Contains(desc, "When file_path is provided instead") {
		t.Error("expected content description to NOT mention file_path alternative when file mode is disabled")
	}
}

func TestSchemaBuilder_FilePathDescription_SandboxTranslationMentioned(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.FileReadEnabled = true
	cfg.SandboxMounts = []SandboxMount{
		{HostPath: "/host/data", SandboxPath: "/sandbox/data"},
	}
	cfg.SandboxTransparent = false

	schema, err := NewSchemaBuilder(cfg).BuildToolSchema()
	if err != nil {
		t.Fatalf("BuildToolSchema failed: %v", err)
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

	filePath, ok := props["file_path"].(map[string]any)
	if !ok {
		t.Fatal("expected 'file_path' property")
	}

	desc, ok := filePath["description"].(string)
	if !ok {
		t.Fatal("expected file_path description to be a string")
	}

	if !strings.Contains(desc, "Sandbox path translation") {
		t.Error("expected file_path description to mention sandbox path translation when mounts configured")
	}
}

func TestSchemaBuilder_BuildToolDescription_Static(t *testing.T) {
	t.Parallel()

	// BuildToolDescription should return the same result regardless of config.
	desc1 := NewSchemaBuilder(DefaultConfig()).BuildToolDescription()

	cfg := DefaultConfig()
	cfg.FileReadEnabled = false
	desc2 := NewSchemaBuilder(cfg).BuildToolDescription()

	if desc1 != desc2 {
		t.Error("BuildToolDescription should be static and not depend on config")
	}
}
