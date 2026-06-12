package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/got-sh/got/internal/dashwiz"
	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/tui"
)

// fakeDashboardAdapter extends fakeAdapter with the few methods
// the dashboard calls. Each test sets only the methods it needs;
// everything else falls back to the no-op fakeAdapter.
type fakeDashboardAdapter struct {
	fakeAdapter
	StatusVal     git.Status
	StatusErr     error
	BranchesVal   []git.Branch
	BranchesErr   error
	RemotesVal    []git.Remote
	RemotesErr    error
	GraphASCIIVal string
	GraphASCIIErr error
}

func (f *fakeDashboardAdapter) Status(_ context.Context) (git.Status, error) {
	return f.StatusVal, f.StatusErr
}

func (f *fakeDashboardAdapter) Branches(_ context.Context) ([]git.Branch, error) {
	return f.BranchesVal, f.BranchesErr
}

func (f *fakeDashboardAdapter) Remotes(_ context.Context) ([]git.Remote, error) {
	return f.RemotesVal, f.RemotesErr
}

func (f *fakeDashboardAdapter) GraphASCII(_ context.Context, _ git.GraphOpts) (string, error) {
	return f.GraphASCIIVal, f.GraphASCIIErr
}

// dashboardDepsFor builds a Deps with a fake dashboard adapter
// and a stubbed RunDashboardWizard that records the call (and
// optionally returns an error). Tests use it to drive
// `got tui` without spinning up a real Bubbletea program.
func dashboardDepsFor(stdout, stderr *bytes.Buffer, a *fakeDashboardAdapter, workTree string, wizardErr error, wizardCalled *bool) Deps {
	d := Deps{
		AdapterFor: func(string) git.Adapter { return a },
		Discover:   func(string) (string, error) { return workTree, nil },
		IsTerminal: func() bool { return false },
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
		User:       func() string { return "alice" },
		GotVersion: "0.1.0-test",
		Stdout:     stdout,
		Stderr:     stderr,
		RunDashboardWizard: func(_ context.Context, _ dashwiz.Inputs, _ tui.Theme) error {
			if wizardCalled != nil {
				*wizardCalled = true
			}
			return wizardErr
		},
	}
	return d
}

func TestTUICmd_NoTUIPrintsSummary(t *testing.T) {
	dir := initGitRepoWorktree(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeDashboardAdapter{
		StatusVal: git.Status{
			Branch:   "main",
			Upstream: "origin/main",
			Entries:  []git.StatusEntry{{Path: "x", XY: "A ", IsStaged: true}},
		},
		BranchesVal:   []git.Branch{{Name: "main", IsCurrent: true, SHA: "abc1234"}},
		RemotesVal:    []git.Remote{{Name: "origin", FetchURL: "git@github.com:foo/bar.git"}},
		GraphASCIIVal: "* abc1234 (HEAD -> main) feat: foo",
	}
	deps := dashboardDepsFor(stdout, stderr, a, dir, nil, nil)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"tui", "--no-tui"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("got tui --no-tui: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"GOT dashboard (non-TTY summary)",
		"Work tree: " + dir,
		"on branch main",
		"1 entry(ies)",
		"1 local",
		"1 configured",
		"github",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
	// The summary should NOT contain any ANSI / styled banner
	// text since it's a non-TTY fallback.
	for _, banned := range []string{"Coming in v0.2", "v0.2"} {
		if strings.Contains(out, banned) {
			t.Errorf("non-TTY summary should not contain %q, got:\n%s", banned, out)
		}
	}
}

func TestTUICmd_NoTUIPrintEmptyStateFriendly(t *testing.T) {
	dir := initGitRepoWorktree(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeDashboardAdapter{} // everything empty / nil
	deps := dashboardDepsFor(stdout, stderr, a, dir, nil, nil)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"tui", "--no-tui"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("got tui --no-tui (empty): %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"0 entry(ies)",
		"0 local",
		"0 configured",
		"(no graph)",
		"(none discovered)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in empty-state output, got:\n%s", want, out)
		}
	}
}

func TestTUICmd_TTYDrivesWizard(t *testing.T) {
	dir := initGitRepoWorktree(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeDashboardAdapter{
		StatusVal:   git.Status{Branch: "main"},
		BranchesVal: []git.Branch{{Name: "main", IsCurrent: true}},
	}
	var called bool
	deps := dashboardDepsFor(stdout, stderr, a, dir, nil, &called)
	// Force the TUI path.
	deps.IsTerminal = func() bool { return true }
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"tui"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("got tui: %v", err)
	}
	if !called {
		t.Errorf("expected RunDashboardWizard to be called in TTY mode, but it wasn't")
	}
}

func TestTUICmd_NotInGitRepoFails(t *testing.T) {
	dir := t.TempDir() // not a git repo
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeDashboardAdapter{}
	deps := dashboardDepsFor(stdout, stderr, a, dir, nil, nil)
	deps.Discover = func(string) (string, error) {
		return "", gerr.NotInGitRepo(".")
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"tui", "--no-tui"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error outside git repo, got nil")
	}
	if !strings.Contains(err.Error(), "not inside a Git repository") {
		t.Errorf("expected not-in-git-repo error, got: %v", err)
	}
}

func TestTUICmd_GraphPreviewTruncatesTo20(t *testing.T) {
	dir := initGitRepoWorktree(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line"
	}
	a := &fakeDashboardAdapter{
		GraphASCIIVal: strings.Join(lines, "\n"),
	}
	deps := dashboardDepsFor(stdout, stderr, a, dir, nil, nil)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"tui", "--no-tui"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("got tui --no-tui: %v", err)
	}
	out := stdout.String()
	got := 0
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "  line") {
			got++
		}
	}
	if got > 20 {
		t.Errorf("graph preview rendered %d lines, want <= 20", got)
	}
}
