package cli

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/got-sh/got/internal/git"
)

// cmdLogDepsFor builds a Deps with a slog Logger writing to logBuf
// at LevelInfo. The adapter is supplied by the caller.
func cmdLogDepsFor(logBuf, stdout, stderr *bytes.Buffer, a git.Adapter, workTree string) Deps {
	return Deps{
		AdapterFor: func(string) git.Adapter { return a },
		Discover:   func(string) (string, error) { return workTree, nil },
		IsTerminal: func() bool { return false },
		Now:        func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		User:       func() string { return "alice" },
		GotVersion: "0.1.0-test",
		Stdout:     stdout,
		Stderr:     stderr,
		Logger:     slog.New(slog.NewTextHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelInfo})),
	}
}

func TestStatusCmd_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := cmdLogDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="status starting"`) {
		t.Errorf("expected 'status starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="status finished"`) {
		t.Errorf("expected 'status finished' log, got:\n%s", out)
	}
}

func TestStatusCmd_FailureEmitsWarnLog(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Use a failing adapter so the Status call errors out.
	failing := &failingStatusAdapter{err: errors.New("status: boom")}
	deps := cmdLogDepsFor(logBuf, stdout, stderr, failing, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"status"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error from failing adapter, got nil")
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="status starting"`) {
		t.Errorf("expected 'status starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `level=WARN`) {
		t.Errorf("expected WARN record, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="status failed"`) {
		t.Errorf("expected 'status failed' log, got:\n%s", out)
	}
}

func TestBranchCmd_List_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := cmdLogDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch", "--no-tui"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("branch list: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="branch list starting"`) {
		t.Errorf("expected 'branch list starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="branch list finished"`) {
		t.Errorf("expected 'branch list finished' log, got:\n%s", out)
	}
}

func TestBranchCmd_Create_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := cmdLogDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch", "create", "feature-x"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("branch create: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="branch create starting"`) {
		t.Errorf("expected 'branch create starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="branch create finished"`) {
		t.Errorf("expected 'branch create finished' log, got:\n%s", out)
	}
	if !strings.Contains(out, "name=feature-x") {
		t.Errorf("expected 'name=feature-x' in log, got:\n%s", out)
	}
}

func TestBranchCmd_Checkout_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := cmdLogDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch", "checkout", "main"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("branch checkout: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="branch checkout starting"`) {
		t.Errorf("expected 'branch checkout starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="branch checkout finished"`) {
		t.Errorf("expected 'branch checkout finished' log, got:\n%s", out)
	}
}

func TestBranchCmd_Delete_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := cmdLogDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch", "delete", "feature-x"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("branch delete: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="branch delete starting"`) {
		t.Errorf("expected 'branch delete starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="branch delete finished"`) {
		t.Errorf("expected 'branch delete finished' log, got:\n%s", out)
	}
	if !strings.Contains(out, "force=false") {
		t.Errorf("expected 'force=false' in log, got:\n%s", out)
	}
}

func TestBranchCmd_Delete_CurrentBranchEmitsWarnLog(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Configure the adapter to report "main" as the current branch
	// so the "cannot delete current" guard fires and emits a warn
	// record.
	a := &fakeAdapter{}
	a.BranchesVal = []git.Branch{{Name: "main", IsCurrent: true, SHA: "aaaaaaa"}}
	deps := cmdLogDepsFor(logBuf, stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch", "delete", "main"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error deleting current branch, got nil")
	}
	out := logBuf.String()
	if !strings.Contains(out, `level=WARN`) {
		t.Errorf("expected WARN record, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="branch delete refused: current branch"`) {
		t.Errorf("expected 'branch delete refused: current branch' log, got:\n%s", out)
	}
}

func TestLoggerFor_NilDepsLoggerIsNoop(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := Deps{
		AdapterFor: func(string) git.Adapter { return &fakeAdapter{} },
		Discover:   func(string) (string, error) { return dir, nil },
		Stdout:     stdout,
		Stderr:     stderr,
		// Logger is nil.
	}
	if got := loggerFor(deps); got == nil {
		t.Fatalf("loggerFor(nil-Deps) returned nil; want a non-nil discard logger")
	}
}

// failingStatusAdapter wraps fakeAdapter and returns err from Status.
type failingStatusAdapter struct {
	fakeAdapter
	err error
}

func (a *failingStatusAdapter) Status(_ context.Context) (git.Status, error) {
	return git.Status{}, a.err
}
