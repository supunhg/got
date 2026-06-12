package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/got-sh/got/internal/branchwiz"
	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/tui"
)

// branchDepsFor builds a Deps value pointed at the given stdout/stderr
// with a fakeAdapter. The wizard is stubbed so branch tests don't need
// a real terminal; tests that need to exercise the wizard path override
// deps.RunBranchWizard after construction.
func branchDepsFor(stdout, stderr *bytes.Buffer, a git.Adapter, workTree string) Deps {
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

func TestBranchCmd_ListEmpty(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{} // BranchesVal is nil -> "no branches"
	deps := branchDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if !strings.Contains(stdout.String(), "(no branches)") {
		t.Errorf("expected '(no branches)' line, got:\n%s", stdout.String())
	}
}

func TestBranchCmd_ListWithBranches(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.BranchesVal = []git.Branch{
		{Name: "main", IsCurrent: true, SHA: "aaaaaaa"},
		{Name: "feature/a", IsCurrent: false, SHA: "bbbbbbb"},
	}
	deps := branchDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("branch: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "* main") {
		t.Errorf("expected '* main' marker, got:\n%s", out)
	}
	if !strings.Contains(out, "  feature/a") {
		t.Errorf("expected '  feature/a' marker, got:\n%s", out)
	}
	if !strings.Contains(out, "aaaaaaa") {
		t.Errorf("expected SHA, got:\n%s", out)
	}
}

func TestBranchCmd_ListAll(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.BranchesVal = []git.Branch{{Name: "main", IsCurrent: true, SHA: "aaaaaaa"}}
	a.RemoteBranchesVal = []git.Branch{{Name: "origin/main", SHA: "ddddddd", IsRemote: true}}
	deps := branchDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch", "--all"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("branch --all: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "main") {
		t.Errorf("expected local main in output:\n%s", out)
	}
	if !strings.Contains(out, "origin/main") {
		t.Errorf("expected remote origin/main in output:\n%s", out)
	}
}

func TestBranchCmd_JSON(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.BranchesVal = []git.Branch{{Name: "main", IsCurrent: true, SHA: "aaaaaaa"}}
	deps := branchDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("branch --json: %v", err)
	}
	var got []git.Branch
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nout=%s", err, stdout.String())
	}
	if len(got) != 1 || got[0].Name != "main" || !got[0].IsCurrent {
		t.Errorf("got = %+v, want one branch named main marked current", got)
	}
}

func TestBranchCmd_NotInGitRepoFails(t *testing.T) {
	dir := t.TempDir()
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := branchDepsFor(stdout, stderr, &fakeAdapter{}, "/nope")
	deps.Discover = func(string) (string, error) {
		return "", gerr.NotInGitRepo(".")
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error outside git repo, got nil")
	}
	if !strings.Contains(err.Error(), "not inside a Git repository") {
		t.Errorf("expected not-in-git-repo error, got: %v", err)
	}
}

func TestBranchCmd_Create(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := branchDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch", "create", "feature/x", "--from", "main"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("branch create: %v\nstderr=%s", err, stderr.String())
	}
	if len(a.CreateBranchCalls) != 1 {
		t.Fatalf("CreateBranchCalls = %d, want 1", len(a.CreateBranchCalls))
	}
	call := a.CreateBranchCalls[0]
	if call.Name != "feature/x" {
		t.Errorf("Name = %q, want feature/x", call.Name)
	}
	if call.StartPoint != "main" {
		t.Errorf("StartPoint = %q, want main", call.StartPoint)
	}
}

func TestBranchCmd_CreateNoFrom(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := branchDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch", "create", "feature/y"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("branch create (no --from): %v", err)
	}
	if len(a.CreateBranchCalls) != 1 {
		t.Fatalf("CreateBranchCalls = %d, want 1", len(a.CreateBranchCalls))
	}
	if a.CreateBranchCalls[0].StartPoint != "" {
		t.Errorf("StartPoint = %q, want empty (defaults to HEAD)", a.CreateBranchCalls[0].StartPoint)
	}
}

func TestBranchCmd_Checkout(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.BranchesVal = []git.Branch{{Name: "main", IsCurrent: true}}
	deps := branchDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch", "checkout", "feature/x"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("branch checkout: %v", err)
	}
	if len(a.CheckoutCalls) != 1 {
		t.Fatalf("CheckoutCalls = %d, want 1", len(a.CheckoutCalls))
	}
	if a.CheckoutCalls[0].Ref != "feature/x" {
		t.Errorf("Ref = %q, want feature/x", a.CheckoutCalls[0].Ref)
	}
}

func TestBranchCmd_Delete(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.BranchesVal = []git.Branch{
		{Name: "main", IsCurrent: true},
		{Name: "feature/x"},
	}
	deps := branchDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch", "delete", "feature/x"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("branch delete: %v", err)
	}
	if len(a.DeleteBranchCalls) != 1 {
		t.Fatalf("DeleteBranchCalls = %d, want 1", len(a.DeleteBranchCalls))
	}
	if a.DeleteBranchCalls[0].Name != "feature/x" {
		t.Errorf("Name = %q, want feature/x", a.DeleteBranchCalls[0].Name)
	}
	if a.DeleteBranchCalls[0].Force {
		t.Errorf("Force = true, want false (no -f flag)")
	}
}

func TestBranchCmd_DeleteForce(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.BranchesVal = []git.Branch{
		{Name: "main", IsCurrent: true},
		{Name: "feature/x"},
	}
	deps := branchDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch", "delete", "-f", "feature/x"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("branch delete -f: %v", err)
	}
	if len(a.DeleteBranchCalls) != 1 {
		t.Fatalf("DeleteBranchCalls = %d, want 1", len(a.DeleteBranchCalls))
	}
	if !a.DeleteBranchCalls[0].Force {
		t.Errorf("Force = false, want true (with -f)")
	}
}

func TestBranchCmd_DeleteRefusesCurrentBranch(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.BranchesVal = []git.Branch{{Name: "main", IsCurrent: true}}
	deps := branchDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch", "delete", "main"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error deleting current branch, got nil")
	}
	if !strings.Contains(err.Error(), "cannot delete the branch you are currently on") {
		t.Errorf("expected current-branch error, got: %v", err)
	}
	if len(a.DeleteBranchCalls) != 0 {
		t.Errorf("DeleteBranchCalls = %d, want 0 (refused before adapter call)", len(a.DeleteBranchCalls))
	}
}

func TestBranchCmd_MoveIsStubbed(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := branchDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch", "move", "old", "new"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error from move stub, got nil")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("expected 'not yet implemented' message, got: %v", err)
	}
}

// TestBranchCmd_WizardPath verifies that when the wizard is wired up
// and IsTerminal is true, the wizard gets called and its resolved
// action is forwarded to the right adapter method.
func TestBranchCmd_WizardPath(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.BranchesVal = []git.Branch{{Name: "main", IsCurrent: true}}
	deps := branchDepsFor(stdout, stderr, a, dir)
	deps.IsTerminal = func() bool { return true }
	deps.RunBranchWizard = func(_ []git.Branch, _ branchwiz.PrePopulated, _ tui.Theme) (branchwiz.Answers, error) {
		return branchwiz.Answers{
			Action: branchwiz.ActionCreate,
			Name:   "feature/wiz",
		}, nil
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("branch (wizard): %v", err)
	}
	if len(a.CreateBranchCalls) != 1 {
		t.Fatalf("CreateBranchCalls = %d, want 1 (wizard resolved Action=Create)", len(a.CreateBranchCalls))
	}
	if a.CreateBranchCalls[0].Name != "feature/wiz" {
		t.Errorf("Name = %q, want feature/wiz", a.CreateBranchCalls[0].Name)
	}
}

// TestBranchCmd_WizardNoTUIForcesList verifies that --no-tui always
// routes to the list path even when IsTerminal is true and the
// wizard is wired up.
func TestBranchCmd_WizardNoTUIForcesList(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.BranchesVal = []git.Branch{{Name: "main", IsCurrent: true, SHA: "aaaaaaa"}}
	deps := branchDepsFor(stdout, stderr, a, dir)
	deps.IsTerminal = func() bool { return true }
	deps.RunBranchWizard = func([]git.Branch, branchwiz.PrePopulated, tui.Theme) (branchwiz.Answers, error) {
		t.Fatalf("RunBranchWizard should not be called when --no-tui is set")
		return branchwiz.Answers{}, nil
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"branch", "--no-tui"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("branch --no-tui: %v", err)
	}
	if !strings.Contains(stdout.String(), "* main") {
		t.Errorf("expected list output with '* main', got:\n%s", stdout.String())
	}
}
