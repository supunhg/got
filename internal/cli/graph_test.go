package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/tui"
)

// graphDepsFor builds a Deps value pointed at the given stdout/stderr
// with a fakeAdapter. Mirrors the pattern used by the other CLI test
// helpers.
func graphDepsFor(stdout, stderr *bytes.Buffer, a git.Adapter, workTree string) Deps {
	return Deps{
		AdapterFor: func(string) git.Adapter { return a },
		Discover:   func(string) (string, error) { return workTree, nil },
		IsTerminal: func() bool { return false },
		Now:        func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		User:       func() string { return "alice" },
		GotVersion: "0.1.0-test",
		Stdout:     stdout,
		Stderr:     stderr,
	}
}

func TestGraphCmd_DotOutput(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.GraphDOTVal = "digraph g { aaa [label=\"aaa\"]; }\n"
	deps := graphDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"graph", "--dot"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("graph --dot: %v", err)
	}
	if !strings.Contains(stdout.String(), "digraph g") {
		t.Errorf("expected DOT output, got:\n%s", stdout.String())
	}
	if len(a.GraphDOTCalls) != 1 {
		t.Errorf("GraphDOTCalls = %d, want 1", len(a.GraphDOTCalls))
	}
	opts := a.GraphDOTCalls[0].Opts
	if opts.MaxCount != 200 {
		t.Errorf("MaxCount = %d, want 200 (default)", opts.MaxCount)
	}
	if !opts.All {
		t.Errorf("All = false, want true (default)")
	}
}

func TestGraphCmd_DotOutputNoTrailingNewlineGetsOne(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.GraphDOTVal = "digraph g { aaa; }" // no trailing newline
	deps := graphDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"graph", "--dot"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("graph --dot: %v", err)
	}
	if !strings.HasSuffix(stdout.String(), "\n") {
		t.Errorf("expected trailing newline, got:\n%s", stdout.String())
	}
}

func TestGraphCmd_NoTUIPlainOutput(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.GraphASCIIVal = "* abc1234 (HEAD -> main) First commit"
	deps := graphDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"graph", "--no-tui"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("graph --no-tui: %v", err)
	}
	if !strings.Contains(stdout.String(), "abc1234") {
		t.Errorf("expected commit SHA in output, got:\n%s", stdout.String())
	}
	if len(a.GraphASCIICalls) != 1 {
		t.Errorf("GraphASCIICalls = %d, want 1", len(a.GraphASCIICalls))
	}
}

func TestGraphCmd_FilterFlags(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := graphDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{
		"graph", "--no-tui",
		"-n", "50",
		"--since", "2024-01-01",
		"--until", "2024-12-31",
		"--author", "Alice",
		"--grep", "fix",
		"--all=false",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("graph with filters: %v", err)
	}
	if len(a.GraphASCIICalls) != 1 {
		t.Fatalf("GraphASCIICalls = %d, want 1", len(a.GraphASCIICalls))
	}
	opts := a.GraphASCIICalls[0].Opts
	if opts.MaxCount != 50 {
		t.Errorf("MaxCount = %d, want 50", opts.MaxCount)
	}
	if opts.Since != "2024-01-01" {
		t.Errorf("Since = %q, want 2024-01-01", opts.Since)
	}
	if opts.Until != "2024-12-31" {
		t.Errorf("Until = %q, want 2024-12-31", opts.Until)
	}
	if opts.Author != "Alice" {
		t.Errorf("Author = %q, want Alice", opts.Author)
	}
	if opts.Grep != "fix" {
		t.Errorf("Grep = %q, want fix", opts.Grep)
	}
	if opts.All {
		t.Errorf("All = true, want false (--all=false)")
	}
}

func TestGraphCmd_WizardPathStubbed(t *testing.T) {
	// When IsTerminal is true and the wizard is stubbed, the CLI
	// must call the stub instead of the real bubbletea program.
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.GraphASCIIVal = "* abc1234 (HEAD -> main) First"
	deps := graphDepsFor(stdout, stderr, a, dir)
	deps.IsTerminal = func() bool { return true }
	called := 0
	deps.RunGraphWizard = func(_ context.Context, _ string, _ tui.Theme) error {
		called++
		return nil
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"graph"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("graph (wizard): %v", err)
	}
	if called != 1 {
		t.Errorf("RunGraphWizard called %d times, want 1", called)
	}
	if len(a.GraphASCIICalls) != 1 {
		t.Errorf("GraphASCIICalls = %d, want 1", len(a.GraphASCIICalls))
	}
}

func TestGraphCmd_WizardNoTUIForcesPlainPath(t *testing.T) {
	// --no-tui wins even when IsTerminal is true and the wizard
	// is wired up; the wizard must not be called.
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.GraphASCIIVal = "* abc1234 (HEAD -> main) First"
	deps := graphDepsFor(stdout, stderr, a, dir)
	deps.IsTerminal = func() bool { return true }
	deps.RunGraphWizard = func(_ context.Context, _ string, _ tui.Theme) error {
		t.Fatalf("RunGraphWizard should not be called when --no-tui is set")
		return nil
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"graph", "--no-tui"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("graph --no-tui: %v", err)
	}
	if !strings.Contains(stdout.String(), "abc1234") {
		t.Errorf("expected plain output, got:\n%s", stdout.String())
	}
}

func TestGraphCmd_NotInGitRepoFails(t *testing.T) {
	dir := t.TempDir() // not a git repo
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := graphDepsFor(stdout, stderr, &fakeAdapter{}, "/nope")
	deps.Discover = func(string) (string, error) {
		return "", gerr.NotInGitRepo(".")
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"graph", "--no-tui"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error outside git repo, got nil")
	}
	if !strings.Contains(err.Error(), "not inside a Git repository") {
		t.Errorf("expected not-in-git-repo error, got: %v", err)
	}
}
