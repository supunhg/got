package cli

import (
	"bytes"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/got-sh/got/internal/git"
)

// fakeAdapter is the in-package Adapter used by cli tests. It value-embeds
// git.FakeAdapter so the call-recording fields (CommitCalls, StageCalls,
// StageAllTrackedCalls, etc.) and the Adapter methods (Status, Commit,
// Stage, etc.) are all promoted onto *fakeAdapter. A zero-value
// `&fakeAdapter{}` is therefore fully usable: FakeAdapter's zero value
// has safe defaults (empty slices for the call arrays; nil StatusVal
// yields the zero Status).
type fakeAdapter struct {
	git.FakeAdapter
}

// newFakeAdapter returns a fresh fakeAdapter with safe defaults.
func newFakeAdapter() *fakeAdapter { return &fakeAdapter{} }

// fakeAdapterFor returns a fakeAdapter pre-loaded with a Status.
func fakeAdapterFor(s git.Status) *fakeAdapter {
	a := newFakeAdapter()
	a.StatusVal = s
	return a
}

// runGit runs `git args...` in dir and fails the test on error.
// Shared by init_test.go and status_test.go.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// initGitRepo creates a fresh git repo in a tempdir with a configured
// user and returns the dir. The repo has no commits yet; tests that
// need a HEAD should add one with runGit(t, dir, "commit", ...).
// Shared by init_test.go and status_test.go.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

// initStatusRepoWithCommit creates a fresh git repo with a single
// initial commit on main and returns the dir. This is the variant
// used by status tests; init tests use plain initGitRepo because
// they want to control the first commit themselves.
func initStatusRepoWithCommit(t *testing.T) string {
	t.Helper()
	dir := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial commit")
	return dir
}

// depsWithLogger builds a Deps with a buffer-backed *slog.Logger
// (slog text handler, LevelInfo) writing to logBuf, plus the same
// safe defaults used by pluginDepsFor / remoteLogDepsFor /
// cmdLogDepsFor / execLogDepsFor / remoteLog2DepsFor across the
// spec-§16 log tests. The single shared helper replaces the five
// near-identical copies that accumulated as the started/finished
// info + warn-on-failure pattern spread across plugin, remote,
// status, commit, branch, graph, worktree, and tui subcommand
// tests.
//
// Tests that need a different level or a different format can
// post-mutate the returned Deps.Logger; tests that need no logger
// at all should use pluginDepsFor (no Logger field set) or build
// a Deps literal directly.
func depsWithLogger(logBuf, stdout, stderr *bytes.Buffer, a git.Adapter, workTree string) Deps {
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
