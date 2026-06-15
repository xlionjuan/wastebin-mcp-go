package main

import (
	"errors"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestParseCreateFlags_InvalidFlag(t *testing.T) {
	t.Parallel()

	_, err := parseCreateFlags([]string{"--unknown-flag"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestParseCreateFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    func(t *testing.T, flags *CLIFlags)
		wantErr error
	}{
		{
			name: "help flag",
			args: []string{"--help"},
			want: func(t *testing.T, flags *CLIFlags) {
				t.Helper()

				if !flags.Help {
					t.Error("expected Help=true")
				}

				if flags.Version {
					t.Error("expected Version=false")
				}
			},
		},
		{
			name: "version flag",
			args: []string{"--version"},
			want: func(t *testing.T, flags *CLIFlags) {
				t.Helper()

				if !flags.Version {
					t.Error("expected Version=true")
				}

				if flags.Help {
					t.Error("expected Help=false")
				}
			},
		},
		{
			name: "all content flags",
			args: []string{
				"--content", "hello",
				"--extension", "md",
				"--expires", "3600",
				"--title", "test paste",
				"--burn-after-reading",
				"--password", "secret",
				"--debug",
			},
			want: func(t *testing.T, flags *CLIFlags) {
				t.Helper()

				if flags.Content != "hello" {
					t.Errorf("expected Content=%q, got %q", "hello", flags.Content)
				}

				if flags.Extension != "md" {
					t.Errorf("expected Extension=%q, got %q", "md", flags.Extension)
				}

				if flags.Expires != "3600" {
					t.Errorf("expected Expires=%q, got %q", "3600", flags.Expires)
				}

				if flags.Title != "test paste" {
					t.Errorf("expected Title=%q, got %q", "test paste", flags.Title)
				}

				if !flags.BurnAfterReading {
					t.Error("expected BurnAfterReading=true")
				}

				if flags.Password != "secret" {
					t.Errorf("expected Password=%q, got %q", "secret", flags.Password)
				}

				if !flags.Debug {
					t.Error("expected Debug=true")
				}

				if flags.Help {
					t.Error("expected Help=false")
				}

				if flags.Version {
					t.Error("expected Version=false")
				}
			},
		},
		{
			name: "file path flag",
			args: []string{"--file-path", "/tmp/doc.md"},
			want: func(t *testing.T, flags *CLIFlags) {
				t.Helper()

				if flags.FilePath != "/tmp/doc.md" {
					t.Errorf("expected FilePath=%q, got %q", "/tmp/doc.md", flags.FilePath)
				}

				if flags.Content != "" {
					t.Errorf("expected empty Content, got %q", flags.Content)
				}
			},
		},
		{
			name:    "empty content error",
			args:    []string{"--content", ""},
			wantErr: errContentEmptyCLI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			flags, err := parseCreateFlags(tt.args)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseCreateFlags failed: %v", err)
			}

			if tt.want != nil {
				tt.want(t, flags)
			}
		})
	}
}
