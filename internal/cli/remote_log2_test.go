package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/got-sh/got/internal/git"
)

// remoteLog2DepsFor is a thin alias for the shared depsWithLogger
// helper. Kept as a named call site so the remote_log2 tests read
// consistently with remote_log_test.go.
func remoteLog2DepsFor(logBuf, stdout, stderr *bytes.Buffer, a git.Adapter, workTree string) Deps {
	return depsWithLogger(logBuf, stdout, stderr, a, workTree)
}

func TestRemoteCmd_List_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := remoteLog2DepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote list: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="remote list starting"`) {
		t.Errorf("expected 'remote list starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="remote list finished"`) {
		t.Errorf("expected 'remote list finished' log, got:\n%s", out)
	}
}

func TestRemoteCmd_Add_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := remoteLog2DepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "add", "origin", "https://example.com/repo.git"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote add: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="remote add starting"`) {
		t.Errorf("expected 'remote add starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="remote add finished"`) {
		t.Errorf("expected 'remote add finished' log, got:\n%s", out)
	}
	if !strings.Contains(out, "name=origin") {
		t.Errorf("expected 'name=origin' in log, got:\n%s", out)
	}
}

func TestRemoteCmd_Add_InvalidURLEmitsWarnLog(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := remoteLog2DepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "add", "origin", "not a valid url"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error for invalid URL, got nil")
	}
	out := logBuf.String()
	if !strings.Contains(out, `level=WARN`) {
		t.Errorf("expected WARN record, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="remote add failed: invalid url"`) {
		t.Errorf("expected 'remote add failed: invalid url' log, got:\n%s", out)
	}
}

func TestRemoteCmd_Remove_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Configure fakeAdapter to report "origin" as a known remote
	// so the remove doesn't hit the "does not exist" guard.
	a := &fakeAdapter{}
	a.RemotesVal = []git.Remote{{Name: "origin", FetchURL: "https://example.com/repo.git", PushURL: "https://example.com/repo.git"}}
	deps := remoteLog2DepsFor(logBuf, stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "remove", "origin"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote remove: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="remote remove starting"`) {
		t.Errorf("expected 'remote remove starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="remote remove finished"`) {
		t.Errorf("expected 'remote remove finished' log, got:\n%s", out)
	}
	if !strings.Contains(out, "name=origin") {
		t.Errorf("expected 'name=origin' in log, got:\n%s", out)
	}
}

func TestRemoteCmd_Remove_NotFoundEmitsWarnLog(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// No remotes configured; remove should hit the "does not exist" guard.
	deps := remoteLog2DepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "remove", "ghost"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error for unknown remote, got nil")
	}
	out := logBuf.String()
	if !strings.Contains(out, `level=WARN`) {
		t.Errorf("expected WARN record, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="remote remove refused: not found"`) {
		t.Errorf("expected 'remote remove refused: not found' log, got:\n%s", out)
	}
}

func TestRemoteCmd_Rename_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := remoteLog2DepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "rename", "origin", "upstream"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote rename: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="remote rename starting"`) {
		t.Errorf("expected 'remote rename starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="remote rename finished"`) {
		t.Errorf("expected 'remote rename finished' log, got:\n%s", out)
	}
	if !strings.Contains(out, "old=origin") {
		t.Errorf("expected 'old=origin' in log, got:\n%s", out)
	}
	if !strings.Contains(out, "new=upstream") {
		t.Errorf("expected 'new=upstream' in log, got:\n%s", out)
	}
}

func TestRemoteCmd_SetURL_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := remoteLog2DepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "set-url", "origin", "https://example.com/new.git"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote set-url: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="remote set-url starting"`) {
		t.Errorf("expected 'remote set-url starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="remote set-url finished"`) {
		t.Errorf("expected 'remote set-url finished' log, got:\n%s", out)
	}
	if !strings.Contains(out, "push=false") {
		t.Errorf("expected 'push=false' in log, got:\n%s", out)
	}
}

func TestRemoteCmd_Prune_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := remoteLog2DepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "prune", "origin"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote prune: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="remote prune starting"`) {
		t.Errorf("expected 'remote prune starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="remote prune finished"`) {
		t.Errorf("expected 'remote prune finished' log, got:\n%s", out)
	}
}
