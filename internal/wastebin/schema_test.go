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

	if !strings.Contains(desc, "Sensitive components remain blocked") {
		t.Error("expected file_path description to mention sensitive components remain blocked")
	}

	if strings.Contains(desc, "Blocked system paths") {
		t.Error("file_path description should not mention blocked system paths when ALLOWED_PATHS configured")
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

	if !strings.Contains(desc, "built-in blocklist") {
		t.Error("expected file_path description to mention built-in blocklist when no ALLOWED_PATHS")
	}

	if !strings.Contains(desc, "system paths (/etc, /proc, /sys, /dev)") {
		t.Error("expected file_path description to mention blocked system paths")
	}

	if !strings.Contains(desc, "sensitive components") {
		t.Error("expected file_path description to mention sensitive components")
	}

	if strings.Contains(desc, "BLOCKED_PATHS") {
		t.Error("file_path description should not mention BLOCKED_PATHS (no user-defined blocked paths in default config)")
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

func TestSchemaBuilder_BuildToolDescription_FileReadDisabled(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.FileReadEnabled = false
	desc := NewSchemaBuilder(cfg).BuildToolDescription()

	if !strings.Contains(desc, "content") {
		t.Error("expected description to mention content")
	}

	if strings.Contains(desc, "file_path") {
		t.Error("expected description to NOT mention file_path when FileReadEnabled=false")
	}

	if !strings.Contains(desc, "hostname") {
		t.Error("expected description to mention hostname")
	}
}

func TestSchemaBuilder_ExtensionDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fileReadOn   bool
		wantFilePath bool
	}{
		{
			name:         "file read enabled mentions file_path detection",
			fileReadOn:   true,
			wantFilePath: true,
		},
		{
			name:         "file read disabled omits file_path detection",
			fileReadOn:   false,
			wantFilePath: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := DefaultConfig()
			cfg.FileReadEnabled = tt.fileReadOn

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

			ext, ok := props["extension"].(map[string]any)
			if !ok {
				t.Fatal("expected 'extension' property")
			}

			desc, ok := ext["description"].(string)
			if !ok {
				t.Fatal("expected extension description to be a string")
			}

			if tt.wantFilePath && !strings.Contains(desc, "file_path") {
				t.Error("expected extension description to mention file_path when file read enabled")
			}

			if !tt.wantFilePath && strings.Contains(desc, "file_path") {
				t.Error("expected extension description to NOT mention file_path when file read disabled")
			}
		})
	}
}

func TestSchemaBuilder_FilePathSecurityDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		allowedPaths    []string
		disableBuiltin  bool
		blockedPaths    []string
		wantAllowlist   bool
		wantBuiltin     bool
		wantUserBlocked bool
		wantNoRestrict  bool
	}{
		{
			name:           "allowlist + builtin enabled",
			allowedPaths:   []string{"/home/allowed"},
			disableBuiltin: false,
			wantAllowlist:  true,
			wantBuiltin:    true,
		},
		{
			name:           "allowlist + builtin disabled",
			allowedPaths:   []string{"/home/allowed"},
			disableBuiltin: true,
			wantAllowlist:  true,
			wantBuiltin:    false,
		},
		{
			name:            "no allowlist + builtin enabled + user blocked paths",
			allowedPaths:    nil,
			disableBuiltin:  false,
			blockedPaths:    []string{"/etc", "/proc"},
			wantAllowlist:   false,
			wantBuiltin:     true,
			wantUserBlocked: true,
		},
		{
			name:            "no allowlist + builtin disabled + user blocked paths",
			allowedPaths:    nil,
			disableBuiltin:  true,
			blockedPaths:    []string{"/etc"},
			wantUserBlocked: true,
		},
		{
			name:           "no allowlist + builtin disabled + no user blocked paths",
			allowedPaths:   nil,
			disableBuiltin: true,
			blockedPaths:   nil,
			wantNoRestrict: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := DefaultConfig()
			cfg.FileReadEnabled = true
			cfg.AllowedPaths = tt.allowedPaths

			cfg.DisableBuiltinBlocklist = tt.disableBuiltin
			if tt.blockedPaths == nil {
				cfg.BlockedPaths = nil
			} else {
				cfg.BlockedPaths = append([]string{}, tt.blockedPaths...)
			}

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

			if tt.wantAllowlist && !strings.Contains(desc, "ALLOWED_PATHS") {
				t.Error("expected description to mention ALLOWED_PATHS")
			}

			if tt.wantBuiltin && !strings.Contains(desc, "built-in blocklist") &&
				!strings.Contains(desc, "Sensitive components remain blocked") {
				t.Error("expected description to mention built-in blocklist or sensitive components")
			}

			if !tt.wantBuiltin && tt.wantAllowlist && strings.Contains(desc, "built-in blocklist") {
				t.Error("expected description to NOT mention built-in blocklist when disabled with allowlist")
			}

			if tt.wantUserBlocked && !strings.Contains(desc, "BLOCKED_PATHS") {
				t.Error("expected description to mention BLOCKED_PATHS")
			}

			if tt.wantNoRestrict && !strings.Contains(desc, "No additional path restrictions") {
				t.Error("expected description to mention no additional restrictions")
			}

			if tt.wantAllowlist && strings.Contains(desc, "Blocked system paths") {
				t.Error("expected description to NOT mention blocked system paths when ALLOWED_PATHS configured")
			}
		})
	}
}
