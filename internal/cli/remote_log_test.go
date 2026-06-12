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

// remoteLogDepsFor builds a Deps with a slog Logger that writes to
// logBuf at LevelInfo. The adapter is supplied by the caller.
func remoteLogDepsFor(logBuf, stdout, stderr *bytes.Buffer, a git.Adapter, workTree string) Deps {
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

func TestRemoteCmd_Fetch_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := remoteLogDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "fetch", "origin"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote fetch: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="remote fetch starting"`) {
		t.Errorf("expected 'remote fetch starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `name=origin`) {
		t.Errorf("expected 'name=origin' in fetch log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="remote fetch finished"`) {
		t.Errorf("expected 'remote fetch finished' log, got:\n%s", out)
	}
}

func TestRemoteCmd_FetchAll_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := remoteLogDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "fetch", "--all"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote fetch --all: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="remote fetch-all starting"`) {
		t.Errorf("expected 'remote fetch-all starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="remote fetch-all finished"`) {
		t.Errorf("expected 'remote fetch-all finished' log, got:\n%s", out)
	}
}

func TestRemoteCmd_Fetch_FailureEmitsWarnLog(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Make the adapter's Fetch return an error so the warn path fires.
	failing := &failingFetchAdapter{err: errors.New("network unreachable")}
	deps := remoteLogDepsFor(logBuf, stdout, stderr, failing, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "fetch", "origin"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error from failing adapter, got nil")
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="remote fetch starting"`) {
		t.Errorf("expected 'remote fetch starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `level=WARN`) {
		t.Errorf("expected a WARN level record, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="remote fetch failed"`) {
		t.Errorf("expected 'remote fetch failed' log, got:\n%s", out)
	}
	if !strings.Contains(out, "network unreachable") {
		t.Errorf("expected error string in warn log, got:\n%s", out)
	}
}

func TestRemoteCmd_Push_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := remoteLogDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "push", "origin", "main"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote push: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="remote push starting"`) {
		t.Errorf("expected 'remote push starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `name=origin`) {
		t.Errorf("expected 'name=origin' in push log, got:\n%s", out)
	}
	if !strings.Contains(out, `branch=main`) {
		t.Errorf("expected 'branch=main' in push log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="remote push finished"`) {
		t.Errorf("expected 'remote push finished' log, got:\n%s", out)
	}
}

func TestRemoteCmd_Push_NonFastForwardEmitsWarnLog(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Simulate a non-fast-forward rejection.
	failing := &failingPushAdapter{err: errors.New("[rejected] main -> main (non-fast-forward)")}
	deps := remoteLogDepsFor(logBuf, stdout, stderr, failing, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "push", "origin", "main"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error from NFF push, got nil")
	}
	out := logBuf.String()
	if !strings.Contains(out, `level=WARN`) {
		t.Errorf("expected a WARN record, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="remote push rejected"`) {
		t.Errorf("expected 'remote push rejected' log, got:\n%s", out)
	}
}

func TestRemoteLogger_NilDepsLoggerIsNoop(t *testing.T) {
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
	if got := remoteLogger(deps); got == nil {
		t.Fatalf("remoteLogger(nil-Deps) returned nil; want a non-nil discard logger")
	}
}

// failingFetchAdapter wraps fakeAdapter and returns err from Fetch /
// FetchAll / FetchPrune. Other methods fall through to fakeAdapter.
type failingFetchAdapter struct {
	fakeAdapter
	err error
}

func (a *failingFetchAdapter) Fetch(_ context.Context, _ string) error      { return a.err }
func (a *failingFetchAdapter) FetchAll(_ context.Context, _ bool) error     { return a.err }
func (a *failingFetchAdapter) FetchPrune(_ context.Context, _ string) error { return a.err }

// failingPushAdapter wraps fakeAdapter and returns err from Push.
type failingPushAdapter struct {
	fakeAdapter
	err error
}

func (a *failingPushAdapter) Push(_ context.Context, _, _ string, _ git.PushOpts) error {
	return a.err
}
