package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/goleak"
)

// cliBinary holds the path to the pre-built CLI binary for subprocess tests.
var cliBinary string

func TestMain(m *testing.M) {
	// Build the CLI binary once for subprocess tests.
	// go run . re-maps all non-zero exit codes to 1, which defeats exit-code
	// testing. Building the binary gives us direct access to actual exit codes.
	tmpDir, tmpDirErr := os.MkdirTemp("", "wastebin-mcp-go-test-*")
	if tmpDirErr != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", tmpDirErr)
		os.Exit(1)
	}

	binaryPath := filepath.Join(tmpDir, "wastebin-mcp-go")
	//nolint:gosec // test build: binaryPath is a temp dir, not user-controlled
	buildCmd := exec.CommandContext(
		context.Background(),
		"go", "build", "-o", binaryPath, ".",
	)
	// buildCmd.Dir defaults to the current working directory, which is the
	// module root when running go test from the project root.
	buildCmd.Stderr = os.Stderr
	buildCmd.Stdout = os.Stdout

	buildErr := buildCmd.Run()
	if buildErr != nil {
		fmt.Fprintf(os.Stderr, "failed to build CLI binary: %v\n", buildErr)
		os.Exit(1)
	}

	cliBinary = binaryPath

	// Run tests and check for goroutine leaks (replaces goleak.VerifyTestMain).
	exitCode := m.Run()

	leakErr := goleak.Find()
	if leakErr != nil {
		fmt.Fprintf(os.Stderr, "goleak: %v\n", leakErr)

		exitCode = -1
	}

	//nolint:errcheck,gosec // best-effort cleanup of temp dir
	os.RemoveAll(tmpDir)

	os.Exit(exitCode)
}

// runCLIBinary runs the pre-built CLI binary as a subprocess with the given
// arguments. extraEnv allows appending environment variables (e.g. "KEY=val").
func runCLIBinary(t *testing.T, args []string, extraEnv ...string) (string, string, int) {
	t.Helper()

	//nolint:gosec // test helper: args come from test code, not user input
	cmd := exec.CommandContext(context.Background(), cliBinary, args...)

	var outb, errb bytes.Buffer

	cmd.Stdout = &outb
	cmd.Stderr = &errb

	if len(extraEnv) > 0 {
		cmd.Env = append(cmd.Environ(), extraEnv...)
	}

	runErr := cmd.Run()

	var exitErr *exec.ExitError

	switch {
	case errors.As(runErr, &exitErr):
		return outb.String(), errb.String(), exitErr.ExitCode()
	case runErr != nil:
		return outb.String(), errb.String(), -1
	default:
		return outb.String(), errb.String(), 0
	}
}

// TestCLI_Help verifies that --help exits 0 and prints usage to stdout.
func TestCLI_Help(t *testing.T) {
	t.Parallel()

	stdout, stderr, exitCode := runCLIBinary(t, []string{"--help"})

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	if !strings.Contains(stdout, "USAGE:") {
		t.Error("expected stdout to contain 'USAGE:'")
	}

	if !strings.Contains(stdout, "wastebin-mcp-go") {
		t.Error("expected stdout to contain 'wastebin-mcp-go'")
	}

	if stderr != "" {
		t.Errorf("expected empty stderr, got: %s", stderr)
	}
}

// TestCLI_Version verifies that --version exits 0 and prints version info to
// stdout.
func TestCLI_Version(t *testing.T) {
	t.Parallel()

	stdout, stderr, exitCode := runCLIBinary(t, []string{"--version"})

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	if !strings.Contains(stdout, "wastebin-mcp-go version") {
		t.Errorf("expected stdout to contain 'wastebin-mcp-go version', got: %s", stdout)
	}

	if !strings.Contains(stdout, "commit:") {
		t.Errorf("expected stdout to contain 'commit:', got: %s", stdout)
	}

	if !strings.Contains(stdout, "built:") {
		t.Errorf("expected stdout to contain 'built:', got: %s", stdout)
	}

	if stderr != "" {
		t.Errorf("expected empty stderr, got: %s", stderr)
	}
}

// TestCLI_UnknownCommand verifies that an unknown subcommand exits 1, writes
// the error to stderr, and prints help text to stdout.
func TestCLI_UnknownCommand(t *testing.T) {
	t.Parallel()

	stdout, stderr, exitCode := runCLIBinary(t, []string{"unknown"})

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	if !strings.Contains(stderr, "ERROR") {
		t.Errorf("expected stderr to contain 'ERROR', got: %s", stderr)
	}

	if !strings.Contains(stderr, `"unknown"`) {
		t.Errorf("expected stderr to quote the unknown command, got: %s", stderr)
	}

	// printCLIHelp() writes to stdout, so USAGE: appears on stdout.
	if !strings.Contains(stdout, "USAGE:") {
		t.Errorf("expected stdout to contain help text ('USAGE:'), got: %s", stdout)
	}
}

// TestCLI_UnknownFlag verifies that an unknown flag exits 1, writes the error
// to stderr, and prints help text to stdout.
func TestCLI_UnknownFlag(t *testing.T) {
	t.Parallel()

	stdout, stderr, exitCode := runCLIBinary(t, []string{"--bogus"})

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	if !strings.Contains(stderr, "ERROR") {
		t.Errorf("expected stderr to contain 'ERROR', got: %s", stderr)
	}

	if !strings.Contains(stderr, `"--bogus"`) {
		t.Errorf("expected stderr to quote the unknown flag, got: %s", stderr)
	}

	// printCLIHelp() writes to stdout, so USAGE: appears on stdout.
	if !strings.Contains(stdout, "USAGE:") {
		t.Errorf("expected stdout to contain help text ('USAGE:'), got: %s", stdout)
	}
}

// TestCLI_NoArgsNoEnv verifies that MCP mode without required env vars
// (WASTEBIN_SERVER_URL) exits 2 and prints an error to stderr.
func TestCLI_NoArgsNoEnv(t *testing.T) {
	t.Parallel()

	// Ensure WASTEBIN_SERVER_URL is cleared in the subprocess environment,
	// so MCP mode fails with exit code 2.
	stdout, stderr, exitCode := runCLIBinary(t, nil, "WASTEBIN_SERVER_URL=")

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	if !strings.Contains(stderr, "ERROR") {
		t.Errorf("expected stderr to contain 'ERROR', got: %s", stderr)
	}

	if stdout != "" {
		t.Errorf("expected empty stdout, got: %s", stdout)
	}
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
