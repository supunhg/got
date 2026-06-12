package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/got-sh/got/internal/git"
)

// execLogDepsFor is a thin alias for the shared depsWithLogger
// helper. Kept as a named call site so the cmd_log2 tests read
// consistently with cmd_log_test.go.
func execLogDepsFor(logBuf, stdout, stderr *bytes.Buffer, a git.Adapter, workTree string) Deps {
	return depsWithLogger(logBuf, stdout, stderr, a, workTree)
}

func TestGraphCmd_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := execLogDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"graph", "--no-tui"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("graph: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="graph starting"`) {
		t.Errorf("expected 'graph starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="graph finished"`) {
		t.Errorf("expected 'graph finished' log, got:\n%s", out)
	}
	if !strings.Contains(out, "format=ascii") {
		t.Errorf("expected 'format=ascii' in finished log, got:\n%s", out)
	}
}

func TestWorktreeCmd_List_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := execLogDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"worktree", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("worktree list: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="worktree list starting"`) {
		t.Errorf("expected 'worktree list starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="worktree list finished"`) {
		t.Errorf("expected 'worktree list finished' log, got:\n%s", out)
	}
}

func TestWorktreeCmd_Prune_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := execLogDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"worktree", "prune"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("worktree prune: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="worktree prune starting"`) {
		t.Errorf("expected 'worktree prune starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="worktree prune finished"`) {
		t.Errorf("expected 'worktree prune finished' log, got:\n%s", out)
	}
}

func TestWorktreeCmd_Attach_EmitsStartedLog(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := execLogDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"worktree", "attach"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("worktree attach: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="worktree attach starting"`) {
		t.Errorf("expected 'worktree attach starting' log, got:\n%s", out)
	}
}

func TestTUICmd_EmitsStartedAndSummaryLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := execLogDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"tui"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("tui: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="dashboard starting"`) {
		t.Errorf("expected 'dashboard starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="dashboard printing non-tty summary"`) {
		t.Errorf("expected 'dashboard printing non-tty summary' log, got:\n%s", out)
	}
}
