package wastebin //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"testing"
)

func TestParseSandboxMounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantErr  bool
		wantHost []string
		wantSand []string
	}{
		{
			name:     "empty string",
			input:    "",
			wantErr:  false,
			wantHost: nil,
			wantSand: nil,
		},
		{
			name:     "single mount",
			input:    "/host/path:/sandbox/path",
			wantErr:  false,
			wantHost: []string{"/host/path"},
			wantSand: []string{"/sandbox/path"},
		},
		{
			name:     "multiple mounts",
			input:    "/a:/x,/b:/y",
			wantErr:  false,
			wantHost: []string{"/a", "/b"},
			wantSand: []string{"/x", "/y"},
		},
		{
			name:     "empty pair skipped",
			input:    "/a:/x,,/b:/y",
			wantErr:  false,
			wantHost: []string{"/a", "/b"},
			wantSand: []string{"/x", "/y"},
		},
		{
			name:    "invalid format (no colon)",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "empty host path",
			input:   ":/sandbox",
			wantErr: true,
		},
		{
			name:    "empty sandbox path",
			input:   "/host:",
			wantErr: true,
		},
		{
			name:    "relative host path (dot prefix)",
			input:   "./workspace:/mnt",
			wantErr: true,
		},
		{
			name:    "relative host path (no dot)",
			input:   "relative/path:/sandbox",
			wantErr: true,
		},
		{
			name:     "sandbox path cleaning",
			input:    "/host://sandbox//path///",
			wantErr:  false,
			wantHost: []string{"/host"},
			wantSand: []string{"/sandbox/path"},
		},
		{
			name:     "whitespace handling",
			input:    "  /a:/x  ,  /b:/y  ",
			wantErr:  false,
			wantHost: []string{"/a", "/b"},
			wantSand: []string{"/x", "/y"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mounts, err := ParseSandboxMounts(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			wantCount := len(tt.wantHost)
			if len(mounts) != wantCount {
				t.Fatalf("expected %d mounts, got %d", wantCount, len(mounts))
			}

			for i := range wantCount {
				if mounts[i].HostPath != tt.wantHost[i] {
					t.Errorf("mount[%d] HostPath: got %q, want %q", i, mounts[i].HostPath, tt.wantHost[i])
				}

				if mounts[i].SandboxPath != tt.wantSand[i] {
					t.Errorf("mount[%d] SandboxPath: got %q, want %q", i, mounts[i].SandboxPath, tt.wantSand[i])
				}
			}
		})
	}
}

func TestParseSandboxMounts_Overlapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "prefix overlap",
			input: "/host/workspace:/workspace,/host/workspace-sub:/workspace/sub",
		},
		{
			name:  "same path (duplicate)",
			input: "/host/a:/workspace,/host/b:/workspace",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseSandboxMounts(tt.input)
			if err == nil {
				t.Fatal("expected error for overlapping sandbox mounts")
			}
		})
	}
}

func TestTranslator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mounts  []SandboxMount
		sandbox string
		wantOK  bool
		want    string
	}{
		{
			name:    "no mounts",
			mounts:  nil,
			sandbox: "/any/path",
			wantOK:  false,
			want:    "",
		},
		{
			name:    "exact match",
			mounts:  []SandboxMount{{HostPath: "/host/workspace", SandboxPath: "/workspace"}},
			sandbox: "/workspace",
			wantOK:  true,
			want:    "/host/workspace",
		},
		{
			name:    "prefix match",
			mounts:  []SandboxMount{{HostPath: "/host/workspace", SandboxPath: "/workspace"}},
			sandbox: "/workspace/subdir/file.go",
			wantOK:  true,
			want:    "/host/workspace/subdir/file.go",
		},
		{
			name:    "no match",
			mounts:  []SandboxMount{{HostPath: "/host/workspace", SandboxPath: "/workspace"}},
			sandbox: "/other/path",
			wantOK:  false,
			want:    "",
		},
		{
			name:    "prefix not partial",
			mounts:  []SandboxMount{{HostPath: "/host/workspace", SandboxPath: "/workspace"}},
			sandbox: "/workspace2",
			wantOK:  false,
			want:    "",
		},
		{
			name: "multiple mounts",
			mounts: []SandboxMount{
				{HostPath: "/host/data", SandboxPath: "/data"},
				{HostPath: "/host/config", SandboxPath: "/config"},
			},
			sandbox: "/config/app.yaml",
			wantOK:  true,
			want:    "/host/config/app.yaml",
		},
		{
			name:    "path traversal rejected",
			mounts:  []SandboxMount{{HostPath: "/host/workspace", SandboxPath: "/workspace"}},
			sandbox: "/workspace/../secret.txt",
			wantOK:  false,
			want:    "",
		},
		{
			name:    "..vault non-traversal",
			mounts:  []SandboxMount{{HostPath: "/host/workspace", SandboxPath: "/workspace"}},
			sandbox: "/workspace/..vault/file.go",
			wantOK:  true,
			want:    "/host/workspace/..vault/file.go",
		},
		{
			name:    "double dotdot rejected",
			mounts:  []SandboxMount{{HostPath: "/host/workspace", SandboxPath: "/workspace"}},
			sandbox: "/workspace/../../etc/passwd",
			wantOK:  false,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tr := NewTranslator(tt.mounts)

			host, ok := tr.Translate(tt.sandbox)
			if ok != tt.wantOK {
				t.Errorf("Translate(%q) ok=%v, want ok=%v", tt.sandbox, ok, tt.wantOK)
			}

			if tt.wantOK && host != tt.want {
				t.Errorf("Translate(%q) = %q, want %q", tt.sandbox, host, tt.want)
			}
		})
	}
}

func TestIsUnderMountHost_ExactMatch(t *testing.T) {
	t.Parallel()

	mounts := []SandboxMount{
		{HostPath: "/host/workspace", SandboxPath: "/workspace"},
	}

	if !isUnderMountHost("/host/workspace", mounts) {
		t.Error("expected exact match to be under mount")
	}
}

func TestIsUnderMountHost_Subdirectory(t *testing.T) {
	t.Parallel()

	mounts := []SandboxMount{
		{HostPath: "/host/workspace", SandboxPath: "/workspace"},
	}

	if !isUnderMountHost("/host/workspace/subdir/file.go", mounts) {
		t.Error("expected subdirectory to be under mount")
	}
}

func TestIsUnderMountHost_Escaped(t *testing.T) {
	t.Parallel()

	mounts := []SandboxMount{
		{HostPath: "/host/workspace", SandboxPath: "/workspace"},
	}

	if isUnderMountHost("/host/secret.txt", mounts) {
		t.Error("expected /host/secret.txt to NOT be under /host/workspace")
	}

	if isUnderMountHost("/etc/passwd", mounts) {
		t.Error("expected /etc/passwd to NOT be under /host/workspace")
	}
}

func TestIsUnderMountHost_MultipleMounts(t *testing.T) {
	t.Parallel()

	mounts := []SandboxMount{
		{HostPath: "/host/data", SandboxPath: "/data"},
		{HostPath: "/host/config", SandboxPath: "/config"},
	}

	if !isUnderMountHost("/host/config/app.yaml", mounts) {
		t.Error("expected /host/config/app.yaml to match /host/config mount")
	}

	if isUnderMountHost("/host/other/secret.txt", mounts) {
		t.Error("expected /host/other/secret.txt to NOT match any mount")
	}
}
