package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/got-sh/got/internal/commitwiz"
	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
)

// osWriteFile644 is a thin wrapper so the test helper above stays
// short. It deliberately does not exist as a package-level helper
// elsewhere; it lives here to keep commit_test.go self-contained.
func osWriteFile644(path string, body []byte) error {
	return os.WriteFile(path, body, 0o644)
}

// commitDepsFor builds a Deps value pointed at the given stdout/stderr
// with a fakeAdapter that records the Commit call. The wizard is
// stubbed so commit tests don't need a real terminal; tests that
// need to exercise the wizard path override deps.RunCommitWizard
// after construction.
func commitDepsFor(stdout, stderr *bytes.Buffer, a git.Adapter, workTree string) Deps {
	return Deps{
		AdapterFor: func(string) git.Adapter { return a },
		Discover:   func(string) (string, error) { return workTree, nil },
		StoreFor:   nil, // commit doesn't open the store
		IsTerminal: func() bool { return false },
		Now:        func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		User:       func() string { return "alice" },
		GotVersion: "0.1.0-test",
		Stdout:     stdout,
		Stderr:     stderr,
	}
}

func TestCommitCmd_MessageFlag(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := commitDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"commit", "-m", "feat: add foo"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("commit -m: %v\nstderr=%s", err, stderr.String())
	}
	if len(a.CommitCalls) != 1 {
		t.Errorf("CommitCalls = %d, want 1", len(a.CommitCalls))
	} else {
		call := a.CommitCalls[0]
		if !strings.Contains(call.Msg, "feat: add foo") {
			t.Errorf("Msg = %q, want it to contain 'feat: add foo'", call.Msg)
		}
	}
}

func TestCommitCmd_NoTUI(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := commitDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"commit", "--no-tui", "-m", "fix: handle nil"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("commit --no-tui: %v\nstderr=%s", err, stderr.String())
	}
	if len(a.CommitCalls) != 1 {
		t.Errorf("CommitCalls = %d, want 1", len(a.CommitCalls))
	}
}

func TestCommitCmd_AllFlag(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	// Create a tracked file and modify it so `git add -u` has work
	// to do.
	if err := writeFile644(t, dir, "foo.txt", "v1\n"); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "foo.txt")
	runGit(t, dir, "commit", "-m", "add foo")

	if err := writeFile644(t, dir, "foo.txt", "v2\n"); err != nil {
		t.Fatal(err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := commitDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"commit", "--all", "--no-tui", "-m", "fix: update"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("commit --all: %v", err)
	}
	// The CLI calls StageAllTracked before Commit when --all is set.
	if a.StageAllTrackedCalls != 1 {
		t.Errorf("StageAllTrackedCalls = %d, want 1", a.StageAllTrackedCalls)
	}
	if len(a.CommitCalls) != 1 {
		t.Errorf("CommitCalls = %d, want 1", len(a.CommitCalls))
	}
}

func TestCommitCmd_AmendFlag(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := commitDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"commit", "--no-tui", "-m", "fix: typo", "--amend"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("commit --amend: %v", err)
	}
	if len(a.CommitCalls) != 1 {
		t.Fatalf("CommitCalls = %d, want 1", len(a.CommitCalls))
	}
	if !a.CommitCalls[0].Opts.Amend {
		t.Errorf("Opts.Amend = false, want true")
	}
}

func TestCommitCmd_NoVerifyFlag(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := commitDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"commit", "--no-tui", "-m", "fix: skip", "--no-verify"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("commit --no-verify: %v", err)
	}
	if len(a.CommitCalls) != 1 {
		t.Fatalf("CommitCalls = %d, want 1", len(a.CommitCalls))
	}
	if !a.CommitCalls[0].Opts.NoVerify {
		t.Errorf("Opts.NoVerify = false, want true")
	}
}

func TestCommitCmd_NotInGitRepoFails(t *testing.T) {
	dir := t.TempDir() // not a git repo
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := commitDepsFor(stdout, stderr, a, "/nope")
	// Override Discover so runCommit returns the not-in-git-repo
	// error from gerr, matching what production code would emit
	// when `got commit` runs outside a git work tree.
	deps.Discover = func(string) (string, error) {
		return "", gerr.NotInGitRepo(".")
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"commit", "-m", "feat: x"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error outside git repo, got nil")
	}
	if !strings.Contains(err.Error(), "not inside a Git repository") {
		t.Errorf("expected not-in-git-repo error, got: %v", err)
	}
}

func TestCommitCmd_BreakingViaMessage(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := commitDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"commit", "--no-tui", "-m", "feat(api)!: drop v1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("commit breaking: %v", err)
	}
	if len(a.CommitCalls) != 1 {
		t.Fatalf("CommitCalls = %d, want 1", len(a.CommitCalls))
	}
	if !strings.Contains(a.CommitCalls[0].Msg, "feat(api)!:") {
		t.Errorf("Msg = %q, want it to contain 'feat(api)!:'", a.CommitCalls[0].Msg)
	}
}

func TestCommitCmd_EmptySubjectFails(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := commitDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"commit", "--no-tui", "-m", ""})

	err := cmd.Execute()
	// An empty message is a validation failure (gerr.Validation -> exit 5).
	if err == nil {
		t.Errorf("expected error for empty subject, got nil")
	}
	if len(a.CommitCalls) != 0 {
		t.Errorf("CommitCalls = %d, want 0 (no commit on empty subject)", len(a.CommitCalls))
	}
}

func TestCommitCmd_PrintsSHAOnSuccess(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := commitDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"commit", "--no-tui", "-m", "feat: success"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !strings.Contains(stdout.String(), "[got") {
		t.Errorf("expected '[got' prefix on success line; got:\n%s", stdout.String())
	}
}

// TestCommitCmd_WizardPath verifies that when the wizard is wired
// up and IsTerminal is true, the wizard gets called and its
// StagedAfter paths are forwarded to the adapter's Stage method.
func TestCommitCmd_WizardPath(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := commitDepsFor(stdout, stderr, a, dir)
	deps.IsTerminal = func() bool { return true }
	// Pre-populated so the wizard "returns" immediately with
	// deterministic Answers.
	deps.RunCommitWizard = func(_ []string, pre commitwiz.PrePopulated) (commitwiz.Answers, error) {
		return commitwiz.Answers{
			Type:        "fix",
			Scope:       "cli",
			Subject:     "wizard-test",
			StagedAfter: []string{"new-file.go"},
		}, nil
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"commit"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("commit (wizard): %v", err)
	}
	// Wizard path should have called Stage on the post-wizard
	// entries.
	if len(a.StageCalls) != 1 {
		t.Errorf("StageCalls = %d, want 1", len(a.StageCalls))
	} else if len(a.StageCalls[0]) != 1 || a.StageCalls[0][0] != "new-file.go" {
		t.Errorf("StageCalls[0] = %v, want [new-file.go]", a.StageCalls[0])
	}
	if len(a.CommitCalls) != 1 {
		t.Errorf("CommitCalls = %d, want 1", len(a.CommitCalls))
	}
}

// writeFile644 is a tiny helper that writes a file with 0o644
// permissions and fails the test on error.
func writeFile644(t *testing.T, dir, name, body string) error {
	t.Helper()
	return osWriteFile644(filepath.Join(dir, name), []byte(body))
}
