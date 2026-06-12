package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/initwiz"
	"github.com/got-sh/got/internal/store"
	"github.com/got-sh/got/internal/tui"
)

// initDepsForStatus builds a Deps value pointed at the given
// stdout/stderr with deterministic time/user/version, and a fake
// adapter factory. The store factory defaults to a real store.Open
// so tests that need to read .got/ can do so; tests that want
// "not initialized" semantics leave .got/ alone.
func initDepsForStatus(stdout, stderr *bytes.Buffer, a git.Adapter, workTree string) Deps {
	return Deps{
		AdapterFor: func(string) git.Adapter { return a },
		Discover:   func(string) (string, error) { return workTree, nil },
		StoreFor:   store.Open,
		IsTerminal: func() bool { return false },
		Now:        func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		User:       func() string { return "alice" },
		GotVersion: "0.1.0-test",
		Stdout:     stdout,
		Stderr:     stderr,
	}
}

// TestStatusCmd_CleanRepo shows the "nothing to commit" line and an
// initialized GOT section (or uninitialized hint).
func TestStatusCmd_CleanRepo(t *testing.T) {
	dir := initStatusRepoWithCommit(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := fakeAdapterFor(git.Status{Branch: "main"})
	deps := initDepsForStatus(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("status: %v\nstderr=%s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "On branch main") {
		t.Errorf("expected 'On branch main' in output:\n%s", out)
	}
	if !strings.Contains(out, "nothing to commit") {
		t.Errorf("expected 'nothing to commit' in output:\n%s", out)
	}
	// The GOT section is always present, initialized or not.
	if !strings.Contains(out, "GOT:") {
		t.Errorf("expected 'GOT:' section in output:\n%s", out)
	}
	// We did not run `got init`, so .got/ should not exist.
	if _, err := os.Stat(filepath.Join(dir, ".got")); err == nil {
		t.Errorf(".got/ should not exist; the test would be misleading if it did")
	}
	if !strings.Contains(out, "not initialized") {
		t.Errorf("expected 'not initialized' hint in output:\n%s", out)
	}
}

// TestStatusCmd_NotInGitRepo verifies the runStatus path that hands
// off to deps.Discover. The error from Discover is returned as-is
// (with exit code 3) and the GOT section is never rendered.
func TestStatusCmd_NotInGitRepo(t *testing.T) {
	dir := t.TempDir() // not a git repo
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := initDepsForStatus(stdout, stderr, &fakeAdapter{}, "/nope")
	// Override Discover to surface the not-in-git-repo error using
	// the real gerr.NotInGitRepo helper so the message matches what
	// production code would emit.
	deps.Discover = func(string) (string, error) {
		return "", gerr.NotInGitRepo(".")
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"status"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error outside git repo, got nil")
	}
	if !strings.Contains(err.Error(), "not inside a Git repository") {
		t.Errorf("expected not-in-git-repo error, got: %v", err)
	}
}

// TestStatusCmd_DirtyRepo shows staged/unstaged/untracked groups in
// the human-readable output. We use a real git repo (no fake adapter)
// so we exercise the actual porcelain v2 parser end-to-end.
func TestStatusCmd_DirtyRepo(t *testing.T) {
	dir := initStatusRepoWithCommit(t)
	withChdir(t, dir)
	// Stage a new file, leave another file untracked, and modify a
	// tracked file (which produces an unstaged entry).
	if err := os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("s\n"), 0o644); err != nil {
		t.Fatalf("write staged: %v", err)
	}
	runGit(t, dir, "add", "staged.txt")
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("u\n"), 0o644); err != nil {
		t.Fatalf("write untracked: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello (modified)\n"), 0o644); err != nil {
		t.Fatalf("modify README: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := initDepsForStatus(stdout, stderr, nil, dir)
	// Drop the fake adapter factory; we want a real adapter for this test.
	deps.AdapterFor = func(workTree string) git.Adapter { return git.NewExecAdapter(workTree) }
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("status: %v\nstderr=%s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Changes to be committed:") {
		t.Errorf("expected staged section, got:\n%s", out)
	}
	if !strings.Contains(out, "staged.txt") {
		t.Errorf("expected staged.txt in staged section, got:\n%s", out)
	}
	if !strings.Contains(out, "Changes not staged for commit:") {
		t.Errorf("expected unstaged section, got:\n%s", out)
	}
	if !strings.Contains(out, "README.md") {
		t.Errorf("expected README.md in unstaged section, got:\n%s", out)
	}
	if !strings.Contains(out, "Untracked files:") {
		t.Errorf("expected untracked section, got:\n%s", out)
	}
	if !strings.Contains(out, "untracked.txt") {
		t.Errorf("expected untracked.txt in untracked section, got:\n%s", out)
	}
}

// TestStatusCmd_Short uses the fake adapter to emit a known set of
// entries and verifies --short renders them in porcelain format.
func TestStatusCmd_Short(t *testing.T) {
	dir := initStatusRepoWithCommit(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := fakeAdapterFor(git.Status{
		Branch: "main",
		Entries: []git.StatusEntry{
			{Path: "a.go", XY: "M ", IsStaged: true},
			{Path: "b.go", XY: " M", IsUnstaged: true},
			{Path: "c.go", IsUntracked: true},
		},
	})
	deps := initDepsForStatus(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"status", "--short"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("status --short: %v", err)
	}
	out := stdout.String()
	wantLines := []string{"M  a.go", " M b.go", "?? c.go"}
	for _, want := range wantLines {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in --short output:\n%s", want, out)
		}
	}
	// The GOT section is suppressed in --short mode.
	if strings.Contains(out, "GOT:") {
		t.Errorf("did not expect 'GOT:' in --short output:\n%s", out)
	}
}

// TestStatusCmd_JSON_WithMeta runs `got init --no-tui` first so
// .got/got.db exists with meta, then verifies --json output has the
// right shape: git status nested under "git", GOT block with
// Initialized=true, schema version, got version, init user, and
// zero counts for the v0.1 forward-compat tables.
//
// Note: --no-tui short-circuits resolveAnswers before IsTerminal is
// consulted, so IsTerminal=false in initDepsForStatus does not affect
// this test.
func TestStatusCmd_JSON_WithMeta(t *testing.T) {
	dir := initStatusRepoWithCommit(t)
	withChdir(t, dir)
	// Run `got init --no-tui` to populate .got/ and meta.
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := initDepsForStatus(stdout, stderr, &fakeAdapter{}, dir)
	// Use a stub RunWizard so resolveAnswers goes through the wizard
	// path with canned answers.
	deps.RunWizard = func(detected initwiz.Detected, pre initwiz.PrePopulated, theme tui.Theme) (initwiz.Answers, error) {
		return initwiz.Answers{
			Name:          "demo",
			DefaultBranch: "main",
			CommitStyle:   "conventional",
		}, nil
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init", "--no-tui"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --no-tui: %v\nstderr=%s", err, stderr.String())
	}

	// Now run `got status --json` with a real git adapter (so we
	// exercise the end-to-end JSON path) and the real store factory.
	stdout.Reset()
	stderr.Reset()
	deps.AdapterFor = func(workTree string) git.Adapter { return git.NewExecAdapter(workTree) }
	deps.StoreFor = store.Open
	cmd = NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"status", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status --json: %v\nstderr=%s", err, stderr.String())
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal JSON: %v\nout=%s", err, stdout.String())
	}
	if _, ok := got["git"]; !ok {
		t.Errorf("JSON missing top-level 'git' key; got keys: %v", keysOf(got))
	}
	gotBlockRaw, ok := got["got"]
	if !ok {
		t.Fatalf("JSON missing top-level 'got' key; got keys: %v", keysOf(got))
	}
	var gotBlock statusGOTBlock
	if err := json.Unmarshal(gotBlockRaw, &gotBlock); err != nil {
		t.Fatalf("unmarshal got block: %v\nblock=%s", err, string(gotBlockRaw))
	}
	if !gotBlock.Initialized {
		t.Errorf("GOT.Initialized = false, want true (after `got init`)")
	}
	if gotBlock.SchemaVersion < 1 {
		t.Errorf("GOT.SchemaVersion = %d, want >= 1", gotBlock.SchemaVersion)
	}
	if gotBlock.GotVersion == "" {
		t.Errorf("GOT.GotVersion is empty")
	}
	if gotBlock.InitUser != "alice" {
		t.Errorf("GOT.InitUser = %q, want alice", gotBlock.InitUser)
	}
	if gotBlock.Counts.Snapshots != 0 || gotBlock.Counts.Decisions != 0 || gotBlock.Counts.Workspaces != 0 {
		t.Errorf("GOT.Counts = %+v, want all zero in v0.1", gotBlock.Counts)
	}
}

// TestStatusCmd_JSON_NotInitialized exercises the "no .got/" branch of
// loadGOTBlock. The JSON report should still have a GOT block, with
// Initialized=false and NotInitializedReason set.
func TestStatusCmd_JSON_NotInitialized(t *testing.T) {
	dir := initStatusRepoWithCommit(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := fakeAdapterFor(git.Status{Branch: "main"})
	deps := initDepsForStatus(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"status", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("status --json: %v", err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal JSON: %v\nout=%s", err, stdout.String())
	}
	gotBlockRaw, ok := got["got"]
	if !ok {
		t.Fatalf("JSON missing 'got' block; got keys: %v", keysOf(got))
	}
	var gotBlock statusGOTBlock
	if err := json.Unmarshal(gotBlockRaw, &gotBlock); err != nil {
		t.Fatalf("unmarshal got block: %v", err)
	}
	if gotBlock.Initialized {
		t.Errorf("GOT.Initialized = true, want false (no .got/)")
	}
	if gotBlock.NotInitializedReason == "" {
		t.Errorf("GOT.NotInitializedReason is empty; want a reason")
	}
}

// keysOf returns the keys of a string-keyed map (helper for nicer test
// failure output).
func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
