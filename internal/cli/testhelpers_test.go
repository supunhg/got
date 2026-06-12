package cli

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/got-sh/got/internal/git"
)

// fakeAdapter is an in-test git.Adapter that returns a preset
// Status and panics on every other method. It exists so status tests
// don't have to depend on a real `git` binary. For tests that need
// a real adapter, use git.NewExecAdapter directly.
type fakeAdapter struct {
	status git.Status
	err    error
}

func (f *fakeAdapter) Status(_ context.Context) (git.Status, error) {
	return f.status, f.err
}

// All other git.Adapter methods are unimplemented on purpose: status
// tests do not exercise them. Calling one will panic loudly so a
// future test that needs a more capable fake is forced to extend
// this type explicitly.
func (f *fakeAdapter) Commit(_ context.Context, _ string, _ git.CommitOpts) (git.SHA, error) {
	panic("fakeAdapter.Commit: not implemented")
}

func (f *fakeAdapter) Branches(_ context.Context) ([]git.Branch, error) {
	panic("fakeAdapter.Branches: not implemented")
}

func (f *fakeAdapter) RemoteBranches(_ context.Context) ([]git.Branch, error) {
	panic("fakeAdapter.RemoteBranches: not implemented")
}

func (f *fakeAdapter) Remotes(_ context.Context) ([]git.Remote, error) {
	panic("fakeAdapter.Remotes: not implemented")
}

func (f *fakeAdapter) Checkout(_ context.Context, _ string, _ git.CheckoutOpts) error {
	panic("fakeAdapter.Checkout: not implemented")
}

func (f *fakeAdapter) Merge(_ context.Context, _ string, _ git.MergeOpts) error {
	panic("fakeAdapter.Merge: not implemented")
}

func (f *fakeAdapter) Reset(_ context.Context, _ string, _ git.ResetMode) error {
	panic("fakeAdapter.Reset: not implemented")
}

func (f *fakeAdapter) Fetch(_ context.Context, _ string) error {
	panic("fakeAdapter.Fetch: not implemented")
}

func (f *fakeAdapter) Push(_ context.Context, _, _ string, _ git.PushOpts) error {
	panic("fakeAdapter.Push: not implemented")
}

func (f *fakeAdapter) Log(_ context.Context, _ string, _ git.LogFormat) (io.Reader, error) {
	panic("fakeAdapter.Log: not implemented")
}

func (f *fakeAdapter) CurrentRef(_ context.Context) (string, error) {
	panic("fakeAdapter.CurrentRef: not implemented")
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
