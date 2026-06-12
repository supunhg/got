package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/tui"
	"github.com/got-sh/got/internal/worktree"
	"github.com/got-sh/got/internal/worktreewiz"
)

// fakeWorktreeAdapter extends fakeAdapter with worktree fields
// so the worktree CLI tests don't have to construct the full
// Adapter from scratch.
type fakeWorktreeAdapter struct {
	fakeAdapter
	WorktreeListVal     []git.Worktree
	WorktreeListErr     error
	WorktreeAddErr      error
	WorktreeAddCalls    []git.FakeWorktreeAddCall
	WorktreeRemoveErr   error
	WorktreeRemoveCalls []git.FakeWorktreeRemoveCall
	WorktreeLockErr     error
	WorktreeLockCalls   []git.FakeWorktreeLockCall
	WorktreeUnlockErr   error
	WorktreeUnlockCalls []git.FakeWorktreePathCall
	WorktreePruneErr    error
	WorktreePruneCalls  int
}

func (f *fakeWorktreeAdapter) WorktreeList(_ context.Context) ([]git.Worktree, error) {
	f.WorktreeListCalls++
	return f.WorktreeListVal, f.WorktreeListErr
}

func (f *fakeWorktreeAdapter) WorktreeAdd(_ context.Context, path string, opts git.WorktreeAddOpts) error {
	f.WorktreeAddCalls = append(f.WorktreeAddCalls, git.FakeWorktreeAddCall{Path: path, Opts: opts})
	return f.WorktreeAddErr
}

func (f *fakeWorktreeAdapter) WorktreeRemove(_ context.Context, path string, force bool) error {
	f.WorktreeRemoveCalls = append(f.WorktreeRemoveCalls, git.FakeWorktreeRemoveCall{Path: path, Force: force})
	return f.WorktreeRemoveErr
}

func (f *fakeWorktreeAdapter) WorktreeLock(_ context.Context, path, reason string) error {
	f.WorktreeLockCalls = append(f.WorktreeLockCalls, git.FakeWorktreeLockCall{Path: path, Reason: reason})
	return f.WorktreeLockErr
}

func (f *fakeWorktreeAdapter) WorktreeUnlock(_ context.Context, path string) error {
	f.WorktreeUnlockCalls = append(f.WorktreeUnlockCalls, git.FakeWorktreePathCall{Path: path})
	return f.WorktreeUnlockErr
}

func (f *fakeWorktreeAdapter) WorktreePrune(_ context.Context) error {
	f.WorktreePruneCalls++
	return f.WorktreePruneErr
}

// worktreeDepsFor builds a Deps with a fake worktree adapter
// and a stubbed RunWorktreeWizard that returns the canned
// Answers. Tests use this to drive `got worktree attach` without
// spinning up a real Bubbletea program.
func worktreeDepsFor(stdout, stderr *bytes.Buffer, a *fakeWorktreeAdapter, workTree string, ans worktreewiz.Answers) Deps {
	return Deps{
		AdapterFor: func(string) git.Adapter { return a },
		Discover:   func(string) (string, error) { return workTree, nil },
		IsTerminal: func() bool { return false },
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
		User:       func() string { return "alice" },
		GotVersion: "0.1.0-test",
		Stdout:     stdout,
		Stderr:     stderr,
		RunWorktreeWizard: func(_ []worktreewiz.Entry, _ worktreewiz.PrePopulated, _ tui.Theme) (worktreewiz.Answers, error) {
			return ans, nil
		},
	}
}

// initGitRepoWorktree creates a real .git/ dir so deps.Discover
// can find the work tree. The fakeWorktreeAdapter doesn't
// actually shell out to git.
func initGitRepoWorktree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	return dir
}

func TestWorktreeCmd_ListEmpty(t *testing.T) {
	dir := initGitRepoWorktree(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeWorktreeAdapter{WorktreeListVal: nil}
	deps := worktreeDepsFor(stdout, stderr, a, dir, worktreewiz.Answers{})
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"worktree", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("worktree list: %v", err)
	}
	if !strings.Contains(stdout.String(), "(no worktrees)") {
		t.Errorf("expected '(no worktrees)', got:\n%s", stdout.String())
	}
}

func TestWorktreeCmd_ListTableMergesPorcelain(t *testing.T) {
	dir := initGitRepoWorktree(t)
	withChdir(t, dir)
	store := worktree.NewStore(filepath.Join(dir, ".got", worktree.FileName))
	if err := store.Write([]worktree.WorktreeRecord{
		{Path: dir, Label: "main repo", LastAttachedAt: time.Unix(1_700_000_000, 0).UTC()},
	}); err != nil {
		t.Fatalf("store.Write: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeWorktreeAdapter{
		WorktreeListVal: []git.Worktree{
			{Path: dir, Branch: "main", HEAD: "abc1234def", IsMain: true},
		},
	}
	deps := worktreeDepsFor(stdout, stderr, a, dir, worktreewiz.Answers{})
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"worktree", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("worktree list: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{dir, "main", "abc1234", "main repo", "(main)"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestWorktreeCmd_ListJSON(t *testing.T) {
	dir := initGitRepoWorktree(t)
	withChdir(t, dir)
	store := worktree.NewStore(filepath.Join(dir, ".got", worktree.FileName))
	if err := store.Write([]worktree.WorktreeRecord{
		{Path: dir, Label: "main repo"},
	}); err != nil {
		t.Fatalf("store.Write: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeWorktreeAdapter{
		WorktreeListVal: []git.Worktree{
			{Path: dir, Branch: "main", HEAD: "abc1234", IsMain: true},
		},
	}
	deps := worktreeDepsFor(stdout, stderr, a, dir, worktreewiz.Answers{})
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"worktree", "list", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("worktree list --json: %v", err)
	}
	var got []map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nout=%s", err, stdout.String())
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if got[0]["label"] != "main repo" {
		t.Errorf("label = %v, want 'main repo' (porcelain field must be in JSON)", got[0]["label"])
	}
}

func TestWorktreeCmd_Add(t *testing.T) {
	dir := initGitRepoWorktree(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeWorktreeAdapter{}
	deps := worktreeDepsFor(stdout, stderr, a, dir, worktreewiz.Answers{})
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"worktree", "add", "sandbox", "--branch", "feature/x"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("worktree add: %v", err)
	}
	if len(a.WorktreeAddCalls) != 1 {
		t.Fatalf("WorktreeAddCalls = %d, want 1", len(a.WorktreeAddCalls))
	}
	got := a.WorktreeAddCalls[0]
	if got.Path != filepath.Join(dir, "sandbox") {
		t.Errorf("Path = %q, want %q", got.Path, filepath.Join(dir, "sandbox"))
	}
	if got.Opts.Branch != "feature/x" {
		t.Errorf("Opts.Branch = %q, want feature/x", got.Opts.Branch)
	}
	recs, _ := worktree.NewStore(filepath.Join(dir, ".got", worktree.FileName)).Read()
	if len(recs) != 1 || recs[0].Branch != "feature/x" {
		t.Errorf("porcelain record not written, got %+v", recs)
	}
}

func TestWorktreeCmd_RemoveRefusesMain(t *testing.T) {
	dir := initGitRepoWorktree(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeWorktreeAdapter{
		WorktreeListVal: []git.Worktree{
			{Path: dir, IsMain: true},
		},
	}
	deps := worktreeDepsFor(stdout, stderr, a, dir, worktreewiz.Answers{})
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"worktree", "remove", dir})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error refusing to remove main worktree, got nil")
	}
	if !strings.Contains(err.Error(), "main worktree") {
		t.Errorf("expected 'main worktree' in error, got: %v", err)
	}
	if len(a.WorktreeRemoveCalls) != 0 {
		t.Errorf("WorktreeRemoveCalls = %d, want 0 (refused call)", len(a.WorktreeRemoveCalls))
	}
}

func TestWorktreeCmd_RemoveLinked(t *testing.T) {
	dir := initGitRepoWorktree(t)
	withChdir(t, dir)
	linked := filepath.Join(dir, "sandbox")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeWorktreeAdapter{
		WorktreeListVal: []git.Worktree{
			{Path: dir, IsMain: true},
			{Path: linked, Branch: "feature/x", HEAD: "abc1234"},
		},
	}
	deps := worktreeDepsFor(stdout, stderr, a, dir, worktreewiz.Answers{})
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"worktree", "remove", linked, "--force"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("worktree remove: %v", err)
	}
	if len(a.WorktreeRemoveCalls) != 1 {
		t.Fatalf("WorktreeRemoveCalls = %d, want 1", len(a.WorktreeRemoveCalls))
	}
	got := a.WorktreeRemoveCalls[0]
	if got.Path != linked || !got.Force {
		t.Errorf("Remove call = %+v, want Path=%q Force=true", got, linked)
	}
}

func TestWorktreeCmd_Lock(t *testing.T) {
	dir := initGitRepoWorktree(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeWorktreeAdapter{}
	deps := worktreeDepsFor(stdout, stderr, a, dir, worktreewiz.Answers{})
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"worktree", "lock", filepath.Join(dir, "sandbox"), "--reason", "WIP"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("worktree lock: %v", err)
	}
	if len(a.WorktreeLockCalls) != 1 {
		t.Fatalf("WorktreeLockCalls = %d, want 1", len(a.WorktreeLockCalls))
	}
	if a.WorktreeLockCalls[0].Reason != "WIP" {
		t.Errorf("Reason = %q, want WIP", a.WorktreeLockCalls[0].Reason)
	}
}

func TestWorktreeCmd_Unlock(t *testing.T) {
	dir := initGitRepoWorktree(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeWorktreeAdapter{}
	deps := worktreeDepsFor(stdout, stderr, a, dir, worktreewiz.Answers{})
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"worktree", "unlock", filepath.Join(dir, "sandbox")})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("worktree unlock: %v", err)
	}
	if len(a.WorktreeUnlockCalls) != 1 {
		t.Errorf("WorktreeUnlockCalls = %d, want 1", len(a.WorktreeUnlockCalls))
	}
}

func TestWorktreeCmd_Prune(t *testing.T) {
	dir := initGitRepoWorktree(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeWorktreeAdapter{}
	deps := worktreeDepsFor(stdout, stderr, a, dir, worktreewiz.Answers{})
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"worktree", "prune"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("worktree prune: %v", err)
	}
	if a.WorktreePruneCalls != 1 {
		t.Errorf("WorktreePruneCalls = %d, want 1", a.WorktreePruneCalls)
	}
}

func TestWorktreeCmd_AttachUpdatesLastAttached(t *testing.T) {
	dir := initGitRepoWorktree(t)
	withChdir(t, dir)
	linked := filepath.Join(dir, "sandbox")
	store := worktree.NewStore(filepath.Join(dir, ".got", worktree.FileName))
	if err := store.Write([]worktree.WorktreeRecord{
		{Path: linked, Label: "sandbox"},
	}); err != nil {
		t.Fatalf("store.Write: %v", err)
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeWorktreeAdapter{
		WorktreeListVal: []git.Worktree{
			{Path: dir, IsMain: true},
			{Path: linked, Branch: "feature/x", HEAD: "abc1234"},
		},
	}
	ans := worktreewiz.Answers{
		Action: worktreewiz.ActionAttach,
		Path:   linked,
		Label:  "sandbox",
	}
	deps := worktreeDepsFor(stdout, stderr, a, dir, ans)
	// Force the TUI path even though IsTerminal is false.
	deps.IsTerminal = func() bool { return true }
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"worktree", "attach"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("worktree attach: %v", err)
	}
	recs, _ := store.Read()
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	if recs[0].LastAttachedAt.IsZero() {
		t.Errorf("LastAttachedAt not set after attach: %+v", recs[0])
	}
}

func TestWorktreeCmd_NotInGitRepoFails(t *testing.T) {
	dir := t.TempDir() // not a git repo
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeWorktreeAdapter{}
	deps := worktreeDepsFor(stdout, stderr, a, dir, worktreewiz.Answers{})
	deps.Discover = func(string) (string, error) {
		return "", gerr.NotInGitRepo(".")
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"worktree", "list"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error outside git repo, got nil")
	}
	if !strings.Contains(err.Error(), "not inside a Git repository") {
		t.Errorf("expected not-in-git-repo error, got: %v", err)
	}
}
