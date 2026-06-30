package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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

	// Help text now goes to stderr on error paths.
	if !strings.Contains(stderr, "USAGE:") {
		t.Errorf("expected stderr to contain help text ('USAGE:'), got: %s", stderr)
	}

	if stdout != "" {
		t.Errorf("expected empty stdout, got: %s", stdout)
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

	// Help text now goes to stderr on error paths.
	if !strings.Contains(stderr, "USAGE:") {
		t.Errorf("expected stderr to contain help text ('USAGE:'), got: %s", stderr)
	}

	if stdout != "" {
		t.Errorf("expected empty stdout, got: %s", stdout)
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

// TestCLI_CreateWithPositionalArgs verifies that extra positional arguments
// after flags cause exit code 1 with help text on stderr.
func TestCLI_CreateWithPositionalArgs(t *testing.T) {
	t.Parallel()

	stdout, stderr, exitCode := runCLIBinary(t, []string{"create", "--content", "ok", "trailing"})

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	if !strings.Contains(stderr, "unexpected arguments") {
		t.Errorf("expected stderr to contain 'unexpected arguments', got: %s", stderr)
	}

	if !strings.Contains(stderr, "USAGE:") {
		t.Errorf("expected stderr to contain help text, got: %s", stderr)
	}

	if stdout != "" {
		t.Errorf("expected empty stdout, got: %s", stdout)
	}
}

// TestCLI_CreateNoContentOrFile verifies that create with neither --content nor
// --file-path fails early (before env config loading) with exit code 1.
func TestCLI_CreateNoContentOrFile(t *testing.T) {
	t.Parallel()

	stdout, stderr, exitCode := runCLIBinary(t, []string{"create"})

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	if !strings.Contains(stderr, "--content or --file-path") {
		t.Errorf("expected stderr to contain '--content or --file-path', got: %s", stderr)
	}

	if !strings.Contains(stderr, "USAGE:") {
		t.Errorf("expected stderr to contain help text, got: %s", stderr)
	}

	if stdout != "" {
		t.Errorf("expected empty stdout, got: %s", stdout)
	}
}

func TestRunCLI_Help(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	exitCode := runCLI([]string{"--help"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	if !strings.Contains(stdout.String(), "USAGE:") {
		t.Error("expected stdout to contain 'USAGE:'")
	}

	if stderr.Len() != 0 {
		t.Errorf("expected empty stderr, got: %s", stderr.String())
	}
}

func TestRunCLI_Version(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	exitCode := runCLI([]string{"--version"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	if !strings.Contains(stdout.String(), "wastebin-mcp-go version") {
		t.Errorf("expected stdout to contain version, got: %s", stdout.String())
	}

	if stderr.Len() != 0 {
		t.Errorf("expected empty stderr, got: %s", stderr.String())
	}
}

func TestRunCLI_UnknownCommand(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	exitCode := runCLI([]string{"unknown"}, &stdout, &stderr)

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "ERROR") {
		t.Errorf("expected stderr to contain 'ERROR', got: %s", stderr.String())
	}

	if !strings.Contains(stderr.String(), `"unknown"`) {
		t.Errorf("expected stderr to quote the unknown command, got: %s", stderr.String())
	}

	if !strings.Contains(stderr.String(), "USAGE:") {
		t.Errorf("expected stderr to contain help text, got: %s", stderr.String())
	}

	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got: %s", stdout.String())
	}
}

func TestRunCLI_UnknownFlag(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	exitCode := runCLI([]string{"--bogus"}, &stdout, &stderr)

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "ERROR") {
		t.Errorf("expected stderr to contain 'ERROR', got: %s", stderr.String())
	}

	if !strings.Contains(stderr.String(), `"--bogus"`) {
		t.Errorf("expected stderr to quote the unknown flag, got: %s", stderr.String())
	}

	if !strings.Contains(stderr.String(), "USAGE:") {
		t.Errorf("expected stderr to contain help text, got: %s", stderr.String())
	}

	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got: %s", stdout.String())
	}
}

func TestRunCLI_CreateHelp(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	exitCode := runCLI([]string{"create", "--help"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	if !strings.Contains(stdout.String(), "USAGE:") {
		t.Error("expected stdout to contain 'USAGE:'")
	}

	if stderr.Len() != 0 {
		t.Errorf("expected empty stderr, got: %s", stderr.String())
	}
}

func TestRunCLI_CreateMissingContent(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	exitCode := runCLI([]string{"create"}, &stdout, &stderr)

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "--content or --file-path") {
		t.Errorf("expected stderr to contain '--content or --file-path', got: %s", stderr.String())
	}

	if !strings.Contains(stderr.String(), "USAGE:") {
		t.Errorf("expected stderr to contain help text, got: %s", stderr.String())
	}

	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got: %s", stdout.String())
	}
}

func TestRunCLI_CreatePositionalArgs(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	exitCode := runCLI([]string{"create", "--content", "ok", "trailing"}, &stdout, &stderr)

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "unexpected arguments") {
		t.Errorf("expected stderr to contain 'unexpected arguments', got: %s", stderr.String())
	}

	if !strings.Contains(stderr.String(), "USAGE:") {
		t.Errorf("expected stderr to contain help text, got: %s", stderr.String())
	}

	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got: %s", stdout.String())
	}
}

func TestRunCLI_CreateEmptyContentFlag(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	exitCode := runCLI([]string{"create", "--content", ""}, &stdout, &stderr)

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "--content must not be empty") {
		t.Errorf("expected stderr to contain '--content must not be empty', got: %s", stderr.String())
	}

	if !strings.Contains(stderr.String(), "USAGE:") {
		t.Errorf("expected stderr to contain help text, got: %s", stderr.String())
	}

	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got: %s", stdout.String())
	}
}

func TestRunCLI_CreateWithServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		//nolint:errcheck,gosec // test helper
		w.Write([]byte(`{"path":"/ABCDEFG"}`))
	}))
	defer ts.Close()

	t.Setenv("WASTEBIN_SERVER_URL", ts.URL)

	var stdout, stderr bytes.Buffer

	exitCode := runCLI([]string{"create", "--content", "hello"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	if stderr.Len() != 0 {
		t.Errorf("expected empty stderr, got: %s", stderr.String())
	}
}

func TestRunCLI_NoArgsNoEnv(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	exitCode := runCLI([]string{}, &stdout, &stderr)

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "ERROR") {
		t.Errorf("expected stderr to contain 'ERROR', got: %s", stderr.String())
	}

	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got: %s", stdout.String())
	}
}

func TestRunCLI_NoArgsMCPModeClientError(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "ftp://server")
	t.Setenv("DEBUG", "true")

	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("failed to create pipe: %v", pipeErr)
	}

	//nolint:errcheck,gosec // test helper: pipe write best-effort
	w.WriteString(`{"jsonrpc":"2.0","method":"initialize"}` + "\n")

	//nolint:errcheck,gosec // test helper: pipe close best-effort
	w.Close()

	oldStdin := os.Stdin

	t.Cleanup(func() { os.Stdin = oldStdin })

	os.Stdin = r

	var stdout, stderr bytes.Buffer

	exitCode := runCLI([]string{}, &stdout, &stderr)

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got: %s", stdout.String())
	}
}

func TestRunCLI_NoArgsInvalidStdin(t *testing.T) {
	t.Setenv("WASTEBIN_SERVER_URL", "http://localhost:9999")

	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("failed to create pipe: %v", pipeErr)
	}

	//nolint:errcheck,gosec // test helper: pipe write best-effort
	w.WriteString("not a valid mcp initialize message\n")

	//nolint:errcheck,gosec // test helper: pipe close best-effort
	w.Close()

	oldStdin := os.Stdin

	t.Cleanup(func() { os.Stdin = oldStdin })

	os.Stdin = r

	var stdout, stderr bytes.Buffer

	exitCode := runCLI([]string{}, &stdout, &stderr)

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "ERROR") {
		t.Errorf("expected stderr to contain 'ERROR', got: %s", stderr.String())
	}

	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got: %s", stdout.String())
	}
}

func TestParseCreateFlags_InvalidFlag(t *testing.T) {
	t.Parallel()

	_, err := parseCreateFlags([]string{"--unknown-flag"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestRunCreateCommand_InvalidFlag(t *testing.T) {
	t.Parallel()

	err := runCreateCommand([]string{"--unknown-flag"}, io.Discard)
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestRunCreateCommand_HelpFlag(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer

	cmdErr := runCreateCommand([]string{"--help"}, &stdout)
	if cmdErr != nil {
		t.Errorf("expected nil error for --help, got: %v", cmdErr)
	}

	if !strings.Contains(stdout.String(), "USAGE:") {
		t.Error("expected help text on stdout to contain USAGE:")
	}
}

func TestRunCreateCommand_MissingContentOrFile(t *testing.T) {
	t.Parallel()

	err := runCreateCommand([]string{}, io.Discard)
	if !errors.Is(err, errMissingContentOrFile) {
		t.Errorf("expected errMissingContentOrFile, got: %v", err)
	}
}

func TestRunCreateCommand_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		//nolint:errcheck,gosec // test helper
		w.Write([]byte(`{"path":"/ABCDEFG"}`))
	}))
	defer ts.Close()

	t.Setenv("WASTEBIN_SERVER_URL", ts.URL)

	err := runCreateCommand([]string{"--content", "hello"}, io.Discard)
	if err != nil {
		t.Fatalf("runCreateCommand failed: %v", err)
	}
}

func TestRunCreateCommand_EmptyContentFlag(t *testing.T) {
	t.Parallel()

	err := runCreateCommand([]string{"--content", ""}, io.Discard)
	if !errors.Is(err, errContentEmptyCLI) {
		t.Errorf("expected errContentEmptyCLI, got: %v", err)
	}
}

func TestRunCreateCommand_PositionalArgs(t *testing.T) {
	t.Parallel()

	err := runCreateCommand([]string{"--content", "hello", "trailing"}, io.Discard)
	if !errors.Is(err, errUnexpectedArgs) {
		t.Errorf("expected errUnexpectedArgs, got: %v", err)
	}
}

func TestRunCreateCommand_WithTestServerContent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		//nolint:errcheck,gosec // test helper
		w.Write([]byte(`{"path":"/XYZ"}`))
	}))
	defer ts.Close()

	t.Setenv("WASTEBIN_SERVER_URL", ts.URL)

	err := runCreateCommand([]string{"--content", "hello from test"}, io.Discard)
	if err != nil {
		t.Fatalf("runCreateCommand failed: %v", err)
	}
}

func TestRunCreateCommand_WithTestServerDebug(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		//nolint:errcheck,gosec // test helper
		w.Write([]byte(`{"path":"/ABCDEFG"}`))
	}))
	defer ts.Close()

	t.Setenv("WASTEBIN_SERVER_URL", ts.URL)

	err := runCreateCommand([]string{"--content", "hello", "--debug"}, io.Discard)
	if err != nil {
		t.Fatalf("runCreateCommand failed: %v", err)
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
		{
			name:    "positional arguments rejected",
			args:    []string{"--content", "hello", "trailing"},
			wantErr: errUnexpectedArgs,
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
