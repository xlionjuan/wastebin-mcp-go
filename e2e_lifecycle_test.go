//go:build e2e

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// startMCPLifecycleSession starts an MCP stdio session without registering
// automatic cleanup. The caller is responsible for closing the session.
// The subprocess lifecycle is managed by mcp.CommandTransport.
func startMCPLifecycleSession(
	ctx context.Context, t *testing.T, wastebinURL string,
) (*mcp.ClientSession, *safeBuffer, *exec.Cmd) {
	t.Helper()

	binaryPath := os.Getenv("E2E_MCP_BINARY")
	if binaryPath == "" {
		binaryPath = buildE2EMCPBinary(ctx, t)
	}

	t.Logf("using MCP binary: %s", binaryPath)

	var stderr safeBuffer

	cmd := exec.CommandContext(ctx, binaryPath) //nolint:gosec // test runs built binary
	cmd.Env = e2eMCPEnv(wastebinURL)
	cmd.Stderr = &stderr

	session := newMCPSession(ctx, t, cmd, &stderr, "wastebin-mcp-go-lifecycle-test")
	t.Log("MCP stdio lifecycle session connected")

	return session, &stderr, cmd
}

func TestMCPLifecycle_SIGINTGracefulShutdown(t *testing.T) {
	wastebinURL := os.Getenv("WASTEBIN_SERVER_URL")
	if wastebinURL == "" {
		t.Skip("WASTEBIN_SERVER_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	session, stderr, cmd := startMCPLifecycleSession(ctx, t, wastebinURL)

	defer func() {
		// Kill the subprocess only if it is still running (test failed).
		// Do NOT call session.Close or cmd.Wait here: the SDK's cleanup
		// goroutine calls pipeRWC.Close → cmd.Wait internally when the
		// process exits; we must not race with that.
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill() //nolint:errcheck // best-effort cleanup
		}
	}()

	err := cmd.Process.Signal(syscall.SIGINT)
	if err != nil {
		t.Fatalf("send SIGINT to MCP process: %v\nstderr:\n%s", err, stderr.String())
	}

	// Wait for the session to close naturally. The signal causes the server
	// to exit; the SDK picks up EOF on stdout, runs pipeRWC.Close (which
	// calls cmd.Wait), and closes the connection. session.Wait returns the
	// result — without us calling cmd.Wait or session.Close from the test.
	waitErr := waitForSessionClose(ctx, t, session, stderr)
	assertCleanSessionClose(t, waitErr, stderr, "SIGINT")
}

func TestMCPLifecycle_SIGTERMGracefulShutdown(t *testing.T) {
	wastebinURL := os.Getenv("WASTEBIN_SERVER_URL")
	if wastebinURL == "" {
		t.Skip("WASTEBIN_SERVER_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	session, stderr, cmd := startMCPLifecycleSession(ctx, t, wastebinURL)

	defer func() {
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill() //nolint:errcheck // best-effort cleanup
		}
	}()

	err := cmd.Process.Signal(syscall.SIGTERM)
	if err != nil {
		t.Fatalf("send SIGTERM to MCP process: %v\nstderr:\n%s", err, stderr.String())
	}

	waitErr := waitForSessionClose(ctx, t, session, stderr)
	assertCleanSessionClose(t, waitErr, stderr, "SIGTERM")
}

func TestMCPLifecycle_InvalidJSONAfterInitialize(t *testing.T) {
	wastebinURL := os.Getenv("WASTEBIN_SERVER_URL")
	if wastebinURL == "" {
		t.Skip("WASTEBIN_SERVER_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	cmd, stdin, stderr, _ := startRawMCPProcess(ctx, t, wastebinURL)

	defer func() {
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()    //nolint:errcheck // best-effort cleanup
			_, _ = cmd.Process.Wait() //nolint:errcheck // best-effort cleanup
		}
	}()

	sendInvalidJSONInput(t, stdin, stderr)

	waitErr := waitForRawProcessExit(ctx, t, cmd, stderr)
	assertAcceptableExitCode(t, waitErr, stderr, "invalid JSON", 0, 2)
}

func TestMCPHandshake_ServerDiscoverAccepted(t *testing.T) {
	wastebinURL := os.Getenv("WASTEBIN_SERVER_URL")
	if wastebinURL == "" {
		t.Skip("WASTEBIN_SERVER_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	cmd, stdin, stderr, stdout := startRawMCPProcess(ctx, t, wastebinURL)

	defer func() {
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()    //nolint:errcheck // best-effort cleanup
			_, _ = cmd.Process.Wait() //nolint:errcheck // best-effort cleanup
		}
	}()

	// A go-sdk v1.7.0+ client probes server/discover as its first message on
	// stdio (SEP-2575). The stdin gate must accept it.
	_, err := fmt.Fprint(stdin, validMCPDiscover)
	if err != nil {
		t.Fatalf("write server/discover message: %v\nstderr:\n%s", err, stderr.String())
	}

	// Wait for the server to answer before closing stdin; closing stdin too
	// early would race the asynchronous response write and lose the reply.
	waitForStdoutContains(ctx, t, cmd, stdout, `"supportedVersions"`, stderr)

	out := stdout.String()
	t.Logf("server stdout:\n%s", out)

	if !strings.Contains(out, `"result"`) {
		t.Fatalf("server/discover did not produce a result\nstdout:\n%s\nstderr:\n%s", out, stderr.String())
	}

	// Closing stdin makes the server see EOF after answering the request, so
	// the process exits cleanly with code 0.
	if err := stdin.Close(); err != nil {
		t.Fatalf("close stdin: %v\nstderr:\n%s", err, stderr.String())
	}

	waitErr := waitForRawProcessExit(ctx, t, cmd, stderr)
	assertAcceptableExitCode(t, waitErr, stderr, "server/discover handshake", 0)
}

func TestMCPHandshake_StatelessFirstRequestAccepted(t *testing.T) {
	wastebinURL := os.Getenv("WASTEBIN_SERVER_URL")
	if wastebinURL == "" {
		t.Skip("WASTEBIN_SERVER_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	cmd, stdin, stderr, stdout := startRawMCPProcess(ctx, t, wastebinURL)

	defer func() {
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()    //nolint:errcheck // best-effort cleanup
			_, _ = cmd.Process.Wait() //nolint:errcheck // best-effort cleanup
		}
	}()

	// In the stateless protocol (>= 2026-07-28) there is no handshake: a
	// direct tools/list request carrying the per-request _meta metadata is a
	// valid first message (SEP-2575). The stdin gate must accept it.
	_, err := fmt.Fprint(stdin, validMCPToolsList)
	if err != nil {
		t.Fatalf("write tools/list message: %v\nstderr:\n%s", err, stderr.String())
	}

	// Wait for the server to answer before closing stdin; closing stdin too
	// early would race the asynchronous response write and lose the reply.
	waitForStdoutContains(ctx, t, cmd, stdout, `"tools"`, stderr)

	out := stdout.String()
	t.Logf("server stdout:\n%s", out)

	if !strings.Contains(out, `"result"`) {
		t.Fatalf("tools/list did not produce a result\nstdout:\n%s\nstderr:\n%s", out, stderr.String())
	}

	if !strings.Contains(out, `"create_paste"`) {
		t.Fatalf("tools/list result does not include create_paste\nstdout:\n%s\nstderr:\n%s", out, stderr.String())
	}

	// Closing stdin makes the server see EOF after answering the request, so
	// the process exits cleanly with code 0.
	if err := stdin.Close(); err != nil {
		t.Fatalf("close stdin: %v\nstderr:\n%s", err, stderr.String())
	}

	waitErr := waitForRawProcessExit(ctx, t, cmd, stderr)
	assertAcceptableExitCode(t, waitErr, stderr, "stateless first request", 0)
}

// waitForStdoutContains polls stdout until it contains want, failing the test
// on timeout or if the process exits first. stdout is drained by exec.Cmd's
// copy goroutine, so the child never blocks writing to the pipe.
func waitForStdoutContains(
	ctx context.Context,
	t *testing.T,
	cmd *exec.Cmd,
	stdout *safeBuffer,
	want string,
	stderr *safeBuffer,
) {
	t.Helper()

	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		if strings.Contains(stdout.String(), want) {
			return
		}

		if cmd.ProcessState != nil {
			t.Fatalf("process exited before stdout contained %q\nstdout:\n%s\nstderr:\n%s",
				want, stdout.String(), stderr.String())
		}

		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for stdout to contain %q\nstdout:\n%s\nstderr:\n%s",
				want, stdout.String(), stderr.String())
		case <-deadline.C:
			t.Fatalf("timeout waiting for stdout to contain %q\nstdout:\n%s\nstderr:\n%s",
				want, stdout.String(), stderr.String())
		case <-ticker.C:
		}
	}
}

// startRawMCPProcess builds and starts the MCP binary with a raw stdin pipe,
// capturing stderr and stdout. The caller is responsible for process cleanup.
func startRawMCPProcess(
	ctx context.Context, t *testing.T, wastebinURL string,
) (*exec.Cmd, io.WriteCloser, *safeBuffer, *safeBuffer) {
	t.Helper()

	binaryPath := os.Getenv("E2E_MCP_BINARY")
	if binaryPath == "" {
		binaryPath = buildE2EMCPBinary(ctx, t)
	}

	t.Logf("using MCP binary: %s", binaryPath)

	var stderr, stdout safeBuffer

	cmd := exec.CommandContext(ctx, binaryPath) //nolint:gosec // test runs built binary
	cmd.Env = e2eMCPEnv(wastebinURL)
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}

	err = cmd.Start()
	if err != nil {
		t.Fatalf("start MCP process: %v", err)
	}

	return cmd, stdin, &stderr, &stdout
}

// sendInvalidJSONInput writes a valid initialize message, waits briefly, then
// writes an invalid JSON line and closes stdin.
func sendInvalidJSONInput(t *testing.T, stdin io.WriteCloser, stderr *safeBuffer) {
	t.Helper()

	_, err := fmt.Fprint(stdin, validMCPInitialize)
	if err != nil {
		t.Fatalf("write initialize message: %v\nstderr:\n%s", err, stderr.String())
	}

	// Give the server time to finish startup and begin reading messages.
	time.Sleep(200 * time.Millisecond)

	_, err = fmt.Fprint(stdin, "this is not valid json\n")
	if err != nil {
		t.Fatalf("write invalid JSON: %v\nstderr:\n%s", err, stderr.String())
	}

	err = stdin.Close()
	if err != nil {
		t.Fatalf("close stdin: %v\nstderr:\n%s", err, stderr.String())
	}
}

// waitForRawProcessExit waits for a process started outside of
// mcp.CommandTransport to exit, failing the test on timeout.
func waitForRawProcessExit(
	ctx context.Context, t *testing.T, cmd *exec.Cmd, stderr *safeBuffer,
) error {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		t.Fatalf("timeout waiting for process to exit\nstderr:\n%s", stderr.String())

		return nil
	}
}

// waitForSessionClose waits for the MCP session to close naturally — the
// server exits (e.g. due to a signal), the SDK picks up EOF on stdout and
// cleans up the connection, then session.Wait returns. Unlike
// waitForRawProcessExit this does NOT call cmd.Wait, avoiding a data race
// with the SDK's internal pipeRWC.Close → cmd.Wait path.
func waitForSessionClose(
	ctx context.Context, t *testing.T, session *mcp.ClientSession, stderr *safeBuffer,
) error {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- session.Wait() }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		t.Fatalf("timeout waiting for session close after signal\nstderr:\n%s", stderr.String())

		return nil
	}
}

// assertAcceptableExitCode fails the test unless waitErr represents one of the
// allowed exit codes. An error other than *exec.ExitError is also a failure.
func assertAcceptableExitCode(
	t *testing.T, waitErr error, stderr *safeBuffer, label string, allowed ...int,
) {
	t.Helper()

	if waitErr == nil {
		return
	}

	var exitErr *exec.ExitError

	if !errors.As(waitErr, &exitErr) {
		t.Fatalf("unexpected wait error after %s: %v\nstderr:\n%s", label, waitErr, stderr.String())
	}

	code := exitErr.ExitCode()
	if slices.Contains(allowed, code) {
		return
	}

	t.Fatalf("process exited with code %d after %s, want one of %v\nstderr:\n%s",
		code, label, allowed, stderr.String())
}

// assertCleanSessionClose fails the test if closeErr indicates the subprocess
// did not exit cleanly after a signal. A nil error means exit code 0; an
// exec.ExitError with code 0 is also accepted.
func assertCleanSessionClose(
	t *testing.T, closeErr error, stderr *safeBuffer, label string,
) {
	t.Helper()

	if closeErr == nil {
		return
	}

	var exitErr *exec.ExitError

	if errors.As(closeErr, &exitErr) {
		if exitErr.ExitCode() == 0 {
			return
		}

		t.Fatalf("process exited with code %d after %s, want 0\nstderr:\n%s",
			exitErr.ExitCode(), label, stderr.String())
	}

	t.Fatalf("close MCP session after %s: %v\nstderr:\n%s", label, closeErr, stderr.String())
}
