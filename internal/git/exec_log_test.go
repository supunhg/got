// exec_log_test.go: tests for the spec §16 "raw git invocations
// and exit codes" debug logging in ExecAdapter.
package git

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os/exec"
	"strings"
	"testing"
)

// newCapturingLogger returns a JSON-format slog.Logger writing to
// buf at LevelDebug, plus a helper to decode the resulting lines.
func newCapturingLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// decodeLogLines returns the list of decoded JSON log records from
// the buffer. Test-only helper.
func decodeLogLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(buf.String(), "\n") {
		if line == "" {
			continue
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("log line is not JSON: %v\nline=%s", err, line)
		}
		out = append(out, got)
	}
	return out
}

// TestExecAdapter_Logger_RecordsInvocation verifies that a logger
// attached via NewExecAdapterWithLogger emits a "git" + "git exit"
// pair for a successful `git --version` invocation, with the
// expected args and code=0 attributes.
func TestExecAdapter_Logger_RecordsInvocation(t *testing.T) {
	var buf bytes.Buffer
	a := NewExecAdapterWithLogger(t.TempDir(), newCapturingLogger(&buf))
	out, _, err := a.run(context.Background(), "--version")
	if err != nil {
		t.Fatalf("--version: %v", err)
	}
	if !bytes.Contains(out, []byte("git version")) {
		t.Errorf("--version output missing 'git version', got: %q", out)
	}

	lines := decodeLogLines(t, &buf)
	if len(lines) < 2 {
		t.Fatalf("expected >= 2 log lines, got %d: %s", len(lines), buf.String())
	}
	// First record: "git" with the args.
	if lines[0]["msg"] != "git" {
		t.Errorf("first msg = %v, want git", lines[0]["msg"])
	}
	args, ok := lines[0]["args"].([]any)
	if !ok || len(args) != 1 || args[0] != "--version" {
		t.Errorf("first args = %v, want [--version]", lines[0]["args"])
	}
	// Last record: "git exit" with code=0 and err="".
	last := lines[len(lines)-1]
	if last["msg"] != "git exit" {
		t.Errorf("last msg = %v, want git exit", last["msg"])
	}
	// JSON numbers decode as float64.
	if code, _ := last["code"].(float64); code != 0 {
		t.Errorf("last code = %v, want 0", last["code"])
	}
	if errStr, _ := last["err"].(string); errStr != "" {
		t.Errorf("last err = %q, want empty", errStr)
	}
}

// TestExecAdapter_Logger_RecordsFailure verifies that a non-zero
// exit code is captured with code != 0 and err populated. We use
// `git bogus-subcommand` which always exits 1.
func TestExecAdapter_Logger_RecordsFailure(t *testing.T) {
	var buf bytes.Buffer
	a := NewExecAdapterWithLogger(t.TempDir(), newCapturingLogger(&buf))
	_, _, err := a.run(context.Background(), "this-is-not-a-real-subcommand")
	if err == nil {
		t.Fatalf("expected error for bogus subcommand, got nil")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() == 0 {
		t.Errorf("expected non-zero exit code, got 0")
	}

	lines := decodeLogLines(t, &buf)
	last := lines[len(lines)-1]
	if last["msg"] != "git exit" {
		t.Fatalf("last msg = %v, want git exit", last["msg"])
	}
	code, _ := last["code"].(float64)
	if int(code) != exitErr.ExitCode() {
		t.Errorf("code = %v, want %d", code, exitErr.ExitCode())
	}
	if errStr, _ := last["err"].(string); errStr == "" {
		t.Errorf("err should be non-empty for failed invocation")
	}
}

// TestExecAdapter_Logger_NilNoPanic verifies that the nil-logger
// path (the production default for --log-level=error) does not
// panic and emits no output.
func TestExecAdapter_Logger_NilNoPanic(t *testing.T) {
	a := NewExecAdapter(t.TempDir()) // no logger
	if a.Logger != nil {
		t.Fatalf("Logger should be nil")
	}
	// No panic + still functional.
	out, _, err := a.run(context.Background(), "--version")
	if err != nil {
		t.Fatalf("--version: %v", err)
	}
	if !bytes.Contains(out, []byte("git version")) {
		t.Errorf("--version output missing 'git version', got: %q", out)
	}
}

// TestExecAdapter_Logger_LevelFiltering verifies that a logger
// configured at LevelInfo drops the Debug "git" / "git exit"
// records (spec §16 contract: only at debug do we see invocations).
func TestExecAdapter_Logger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	a := NewExecAdapterWithLogger(t.TempDir(), logger)
	_, _, err := a.run(context.Background(), "--version")
	if err != nil {
		t.Fatalf("--version: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no records at LevelInfo, got: %s", buf.String())
	}
}

// TestExitCode covers the small exitCode helper used by logExit.
func TestExitCode(t *testing.T) {
	if got := exitCode(nil); got != 0 {
		t.Errorf("exitCode(nil) = %d, want 0", got)
	}
	if got := exitCode(context.Canceled); got != 0 {
		t.Errorf("exitCode(non-ExitError) = %d, want 0", got)
	}
	// *exec.ExitError with a real exit code.
	cmd := exec.Command("false")
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			if got := exitCode(ee); got != ee.ExitCode() {
				t.Errorf("exitCode(ExitError) = %d, want %d", got, ee.ExitCode())
			}
			return
		}
	}
	t.Skip("`false` did not produce *exec.ExitError on this platform")
}
