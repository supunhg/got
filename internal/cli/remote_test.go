package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
)

// remoteDepsFor builds a Deps value pointed at the given stdout/stderr
// with a fakeAdapter. Mirrors the branchDepsFor / commitDepsFor pattern
// used by sibling tests.
func remoteDepsFor(stdout, stderr *bytes.Buffer, a git.Adapter, workTree string) Deps {
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

func TestRemoteCmd_ListEmpty(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{} // RemotesVal nil -> "(no remotes)"
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote list: %v", err)
	}
	if !strings.Contains(stdout.String(), "(no remotes)") {
		t.Errorf("expected '(no remotes)' line, got:\n%s", stdout.String())
	}
	if a.RemotesCalls != 1 {
		t.Errorf("RemotesCalls = %d, want 1", a.RemotesCalls)
	}
}

func TestRemoteCmd_ListTable(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.RemotesVal = []git.Remote{
		{Name: "origin", FetchURL: "git@github.com:foo/bar.git", PushURL: "git@github.com:foo/bar.git"},
		{Name: "upstream", FetchURL: "https://example.com/up.git", PushURL: "https://example.com/up.git"},
	}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote list: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "FETCH URL") {
		t.Errorf("expected table header, got:\n%s", out)
	}
	if !strings.Contains(out, "origin") || !strings.Contains(out, "upstream") {
		t.Errorf("expected both remotes, got:\n%s", out)
	}
}

func TestRemoteCmd_ListJSON(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.RemotesVal = []git.Remote{
		{Name: "origin", FetchURL: "https://example.com/o.git", PushURL: "https://example.com/o.git"},
	}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "list", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote list --json: %v", err)
	}
	var got []git.Remote
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nout=%s", err, stdout.String())
	}
	if len(got) != 1 || got[0].Name != "origin" {
		t.Errorf("got = %+v, want one remote named origin", got)
	}
}

func TestRemoteCmd_NoArgsDefaultsToList(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.RemotesVal = []git.Remote{{Name: "origin", FetchURL: "x", PushURL: "x"}}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote (no args): %v", err)
	}
	if !strings.Contains(stdout.String(), "origin") {
		t.Errorf("expected list output, got:\n%s", stdout.String())
	}
}

func TestRemoteCmd_Add(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "add", "origin", "https://example.com/repo.git"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote add: %v\nstderr=%s", err, stderr.String())
	}
	if len(a.RemoteAddCalls) != 1 {
		t.Fatalf("RemoteAddCalls = %d, want 1", len(a.RemoteAddCalls))
	}
	call := a.RemoteAddCalls[0]
	if call.Name != "origin" || call.URL != "https://example.com/repo.git" {
		t.Errorf("got %+v, want {origin, https://example.com/repo.git}", call)
	}
	if !strings.Contains(stdout.String(), "Added remote origin") {
		t.Errorf("expected confirmation, got:\n%s", stdout.String())
	}
}

func TestRemoteCmd_AddInvalidURL(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "add", "origin", "ht!tp://nope"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for invalid URL, got nil")
	}
	if !strings.Contains(err.Error(), "invalid remote URL") {
		t.Errorf("expected URL validation error, got: %v", err)
	}
	if len(a.RemoteAddCalls) != 0 {
		t.Errorf("RemoteAddCalls = %d, want 0 (rejected before adapter call)", len(a.RemoteAddCalls))
	}
}

func TestRemoteCmd_AddInvalidName(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "add", "bad name", "https://example.com/r.git"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for invalid name, got nil")
	}
	if !strings.Contains(err.Error(), "invalid remote name") {
		t.Errorf("expected name validation error, got: %v", err)
	}
}

func TestRemoteCmd_Remove(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.RemotesVal = []git.Remote{{Name: "origin", FetchURL: "x", PushURL: "x"}}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "remove", "origin"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote remove: %v", err)
	}
	if len(a.RemoteRemoveCalls) != 1 {
		t.Fatalf("RemoteRemoveCalls = %d, want 1", len(a.RemoteRemoveCalls))
	}
	if a.RemoteRemoveCalls[0].Name != "origin" {
		t.Errorf("Name = %q, want origin", a.RemoteRemoveCalls[0].Name)
	}
	if a.RemoteRemoveCalls[0].Force {
		t.Errorf("Force = true, want false (no --force flag)")
	}
}

func TestRemoteCmd_RemoveUnknownFails(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.RemotesVal = []git.Remote{{Name: "origin", FetchURL: "x", PushURL: "x"}}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "remove", "ghost"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for unknown remote, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected 'does not exist' error, got: %v", err)
	}
	if len(a.RemoteRemoveCalls) != 0 {
		t.Errorf("RemoteRemoveCalls = %d, want 0 (rejected before adapter call)", len(a.RemoteRemoveCalls))
	}
}

func TestRemoteCmd_RemoveForceSkipsExistenceCheck(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.RemotesVal = nil // adapter would normally refuse unknown remotes
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "remove", "--force", "ghost"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote remove --force: %v", err)
	}
	if len(a.RemoteRemoveCalls) != 1 {
		t.Fatalf("RemoteRemoveCalls = %d, want 1 (force skips existence check)", len(a.RemoteRemoveCalls))
	}
	if !a.RemoteRemoveCalls[0].Force {
		t.Errorf("Force = false, want true")
	}
}

func TestRemoteCmd_Rename(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "rename", "origin", "upstream"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote rename: %v", err)
	}
	if len(a.RemoteRenameCalls) != 1 {
		t.Fatalf("RemoteRenameCalls = %d, want 1", len(a.RemoteRenameCalls))
	}
	call := a.RemoteRenameCalls[0]
	if call.OldName != "origin" || call.NewName != "upstream" {
		t.Errorf("got %+v, want {origin, upstream}", call)
	}
}

func TestRemoteCmd_SetURL(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "set-url", "origin", "https://example.com/new.git"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote set-url: %v", err)
	}
	if len(a.RemoteSetURLCalls) != 1 {
		t.Fatalf("RemoteSetURLCalls = %d, want 1", len(a.RemoteSetURLCalls))
	}
	call := a.RemoteSetURLCalls[0]
	if call.Name != "origin" || call.URL != "https://example.com/new.git" || call.PushURL {
		t.Errorf("got %+v, want {origin, https://example.com/new.git, false}", call)
	}
}

func TestRemoteCmd_SetURLPushFlag(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "set-url", "--push", "origin", "git@github.com:foo/bar.git"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote set-url --push: %v", err)
	}
	if len(a.RemoteSetURLCalls) != 1 || !a.RemoteSetURLCalls[0].PushURL {
		t.Errorf("PushURL = %+v, want true", a.RemoteSetURLCalls)
	}
}

func TestRemoteCmd_SetURLCheckFetches(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "set-url", "--check", "origin", "https://example.com/new.git"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote set-url --check: %v", err)
	}
	if len(a.RemoteSetURLCalls) != 1 {
		t.Errorf("RemoteSetURLCalls = %d, want 1", len(a.RemoteSetURLCalls))
	}
	if len(a.FetchCalls) != 1 || a.FetchCalls[0].Remote != "origin" {
		t.Errorf("FetchCalls = %+v, want one call with Remote=origin", a.FetchCalls)
	}
}

func TestRemoteCmd_Fetch(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "fetch", "origin"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote fetch: %v", err)
	}
	if len(a.FetchCalls) != 1 || a.FetchCalls[0].Remote != "origin" {
		t.Errorf("FetchCalls = %+v, want one call with Remote=origin", a.FetchCalls)
	}
}

func TestRemoteCmd_FetchPrune(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "fetch", "--prune", "origin"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote fetch --prune: %v", err)
	}
	if len(a.FetchPruneCalls) != 1 || a.FetchPruneCalls[0].Remote != "origin" {
		t.Errorf("FetchPruneCalls = %+v, want one call with Remote=origin", a.FetchPruneCalls)
	}
	if len(a.FetchCalls) != 0 {
		t.Errorf("FetchCalls = %+v, want 0 (prune variant should not be used)", a.FetchCalls)
	}
}

func TestRemoteCmd_FetchAll(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "fetch", "--all"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote fetch --all: %v", err)
	}
	if a.FetchAllCalls != 1 {
		t.Errorf("FetchAllCalls = %d, want 1", a.FetchAllCalls)
	}
	if a.FetchAllPrune {
		t.Errorf("FetchAllPrune = true, want false (no --prune)")
	}
}

func TestRemoteCmd_FetchAllPrune(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "fetch", "--all", "--prune"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote fetch --all --prune: %v", err)
	}
	if a.FetchAllCalls != 1 || !a.FetchAllPrune {
		t.Errorf("FetchAllCalls=%d, FetchAllPrune=%v, want 1, true", a.FetchAllCalls, a.FetchAllPrune)
	}
}

func TestRemoteCmd_FetchRequiresNameOrAll(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "fetch"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error from fetch with no args, got nil")
	}
	if !strings.Contains(err.Error(), "requires a remote name or --all") {
		t.Errorf("expected argument error, got: %v", err)
	}
}

func TestRemoteCmd_Push(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "push", "origin", "main"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote push: %v", err)
	}
	if len(a.PushCalls) != 1 {
		t.Fatalf("PushCalls = %d, want 1", len(a.PushCalls))
	}
	call := a.PushCalls[0]
	if call.Remote != "origin" || call.Branch != "main" {
		t.Errorf("got %+v, want {origin, main, {}}", call)
	}
	if call.Opts.Force || call.Opts.ForceWithLease {
		t.Errorf("expected no force flags, got %+v", call.Opts)
	}
}

func TestRemoteCmd_PushForceWithLease(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "push", "--force-with-lease", "origin", "main"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote push --force-with-lease: %v", err)
	}
	if len(a.PushCalls) != 1 || !a.PushCalls[0].Opts.ForceWithLease {
		t.Errorf("ForceWithLease = %+v, want true", a.PushCalls[0].Opts)
	}
}

func TestRemoteCmd_PushForceAndForceWithLeaseAreMutuallyExclusive(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "push", "--force", "--force-with-lease", "origin", "main"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error from --force + --force-with-lease, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutual-exclusion error, got: %v", err)
	}
}

func TestRemoteCmd_PushRefusesNonFastForward(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.PushErr = gerr.GitError(errors.New("[rejected] main -> main (non-fast-forward)"), "push", "origin", "main")
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "push", "origin", "main"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error from non-FF push, got nil")
	}
	if !strings.Contains(err.Error(), "non-fast-forward") {
		t.Errorf("expected NFF error guidance, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--force-with-lease") {
		t.Errorf("expected --force-with-lease guidance, got: %v", err)
	}
}

func TestRemoteCmd_PushNFFAllowedWithForceWithLease(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	a.PushErr = gerr.GitError(errors.New("[rejected] main -> main (non-fast-forward)"), "push", "origin", "main")
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "push", "--force-with-lease", "origin", "main"})

	// With --force-with-lease, the raw GitError should pass through
	// unchanged (we don't try to rewrap it with the guidance message).
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected non-FF error to bubble up, got nil")
	} else if strings.Contains(err.Error(), "re-run with --force-with-lease") {
		t.Errorf("did not expect guidance wrap when --force-with-lease is set, got: %v", err)
	}
}

func TestRemoteCmd_PushSetUpstreamAndTags(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "push", "--set-upstream", "--tags", "origin", "main"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote push --set-upstream --tags: %v", err)
	}
	opts := a.PushCalls[0].Opts
	if !opts.SetUpstream || !opts.Tags {
		t.Errorf("SetUpstream=%v Tags=%v, want true, true", opts.SetUpstream, opts.Tags)
	}
}

func TestRemoteCmd_Prune(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := remoteDepsFor(stdout, stderr, a, dir)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "prune", "origin"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote prune: %v", err)
	}
	if len(a.RemotePruneCalls) != 1 || a.RemotePruneCalls[0].Remote != "origin" {
		t.Errorf("RemotePruneCalls = %+v, want one call with Remote=origin", a.RemotePruneCalls)
	}
}

func TestRemoteCmd_NotInGitRepoFails(t *testing.T) {
	dir := t.TempDir() // not a git repo
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := remoteDepsFor(stdout, stderr, &fakeAdapter{}, "/nope")
	deps.Discover = func(string) (string, error) {
		return "", gerr.NotInGitRepo(".")
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"remote", "list"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error outside git repo, got nil")
	}
	if !strings.Contains(err.Error(), "not inside a Git repository") {
		t.Errorf("expected not-in-git-repo error, got: %v", err)
	}
}

func TestValidateRemoteURL(t *testing.T) {
	cases := []struct {
		url       string
		wantError bool
	}{
		{"https://example.com/r.git", false},
		{"http://example.com/r.git", false},
		{"git@github.com:foo/bar.git", false}, // scp-style
		{"git://example.com/r.git", false},
		{"ssh://git@example.com/r.git", false},
		{"file:///tmp/r.git", false},
		{"/tmp/repo.git", false},
		{"./sibling.git", false},
		{"../other.git", false},
		{"", true},
		{"ht!tp://nope", true},
		{"javascript:alert(1)", true},
		{"://broken", true},
		{"https://", true}, // host missing
	}
	for _, c := range cases {
		err := validateRemoteURL(c.url)
		if c.wantError && err == nil {
			t.Errorf("validateRemoteURL(%q) = nil, want error", c.url)
		}
		if !c.wantError && err != nil {
			t.Errorf("validateRemoteURL(%q) = %v, want nil", c.url, err)
		}
	}
}

func TestValidateRemoteName(t *testing.T) {
	cases := []struct {
		name      string
		wantError bool
	}{
		{"origin", false},
		{"upstream-1", false},
		{"a.b.c", false},
		{"", true},
		{".", true},
		{"..", true},
		{"has space", true},
		{"has\ttab", true},
		{".leading", true},
		{"trailing/", true},
	}
	for _, c := range cases {
		err := validateRemoteName(c.name)
		if c.wantError && err == nil {
			t.Errorf("validateRemoteName(%q) = nil, want error", c.name)
		}
		if !c.wantError && err != nil {
			t.Errorf("validateRemoteName(%q) = %v, want nil", c.name, err)
		}
	}
}
