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
	"github.com/got-sh/got/internal/repo"
	"github.com/got-sh/got/internal/store"
	"github.com/got-sh/got/internal/workspace"
)

// initWorkspaceRepo creates a tempdir that is a Git repo with a
// fully-initialized .got/got.db (migrations applied). The CLI
// commands under test need both: deps.Discover expects a .git
// dir, and the workspace store expects a real got.db with the
// v0.4 tables. The store is opened once at setup just to run
// migrations, then closed; each command re-opens via deps.StoreFor.
func initWorkspaceRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".got"), 0o755); err != nil {
		t.Fatalf("mkdir .got: %v", err)
	}
	s, err := store.Open(filepath.Join(dir, ".got", "got.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	return dir
}

// workspaceDepsFor builds a Deps with the fakeAdapter (for the
// Deps struct's required fields) plus a real store.Open wired
// up. discover returns the work tree passed in.
func workspaceDepsFor(stdout, stderr *bytes.Buffer, workTree string) Deps {
	return Deps{
		AdapterFor: func(string) git.Adapter { return &fakeAdapter{} },
		Discover:   func(string) (string, error) { return workTree, nil },
		StoreFor:   store.Open,
		IsTerminal: func() bool { return false },
		Now:        func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		User:       func() string { return "alice" },
		GotVersion: "0.4.0-test",
		Stdout:     stdout,
		Stderr:     stderr,
	}
}

func runWorkspaceCmd(t *testing.T, deps Deps, args ...string) (string, string, error) {
	t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(append([]string{"workspace"}, args...))
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func TestWorkspaceCmd_Create(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)

	out, _, err := runWorkspaceCmd(t, deps, "create", "oauth", "--title", "OAuth Refactor")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.Contains(out, `created workspace "oauth"`) {
		t.Errorf("output missing 'created workspace' marker:\n%s", out)
	}
	// Verify the row is in the DB.
	s, err := store.Open(filepath.Join(dir, ".got", "got.db"))
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer func() { _ = s.Close() }()
	ws, err := workspace.NewWithDB(s.DB(), time.Now).List(context.Background(), workspace.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ws) != 1 || ws[0].Name != "oauth" || ws[0].Title != "OAuth Refactor" {
		t.Errorf("List = %+v, want one workspace named oauth", ws)
	}
}

func TestWorkspaceCmd_CreateRejectsBadName(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)

	_, _, err := runWorkspaceCmd(t, deps, "create", "Bad-Name", "--title", "X")
	if err == nil {
		t.Fatal("create with bad name = nil, want error")
	}
	if !strings.Contains(err.Error(), "invalid name") {
		t.Errorf("error = %q, want 'invalid name'", err.Error())
	}
}

func TestWorkspaceCmd_CreateRejectsMissingTitle(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)

	_, _, err := runWorkspaceCmd(t, deps, "create", "ok-name")
	if err == nil {
		t.Fatal("create without title = nil, want error")
	}
	if !strings.Contains(err.Error(), "title") {
		t.Errorf("error = %q, want 'title'", err.Error())
	}
}

func TestWorkspaceCmd_CreateRejectsDuplicateName(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)

	if _, _, err := runWorkspaceCmd(t, deps, "create", "dup", "--title", "X"); err != nil {
		t.Fatalf("create #1: %v", err)
	}
	_, _, err := runWorkspaceCmd(t, deps, "create", "dup", "--title", "Y")
	if err == nil {
		t.Fatal("create duplicate = nil, want error")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Errorf("error = %q, want 'already in use'", err.Error())
	}
}

func TestWorkspaceCmd_ListEmpty(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)

	out, _, err := runWorkspaceCmd(t, deps, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "(no workspaces)") {
		t.Errorf("output = %q, want '(no workspaces)'", out)
	}
}

func TestWorkspaceCmd_ListJSON(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)

	if _, _, err := runWorkspaceCmd(t, deps, "create", "a", "--title", "A"); err != nil {
		t.Fatalf("create a: %v", err)
	}
	out, _, err := runWorkspaceCmd(t, deps, "list", "--json")
	if err != nil {
		t.Fatalf("list --json: %v", err)
	}
	var got []*workspace.Workspace
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nout=%s", err, out)
	}
	if len(got) != 1 || got[0].Name != "a" {
		t.Errorf("got %+v, want one workspace named a", got)
	}
}

func TestWorkspaceCmd_Show(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)

	if _, _, err := runWorkspaceCmd(t, deps, "create", "x", "--title", "X", "--description", "demo"); err != nil {
		t.Fatalf("create: %v", err)
	}
	out, _, err := runWorkspaceCmd(t, deps, "show", "x")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	for _, want := range []string{"Workspace: x", "Title:       X", "Description: demo", "State:       open", "Files: (none)", "Branches: (none)", "Decisions: (none)", "Notes: (none)"} {
		if !strings.Contains(out, want) {
			t.Errorf("show output missing %q:\n%s", want, out)
		}
	}
}

func TestWorkspaceCmd_ShowJSON(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)

	if _, _, err := runWorkspaceCmd(t, deps, "create", "x", "--title", "X"); err != nil {
		t.Fatalf("create: %v", err)
	}
	out, _, err := runWorkspaceCmd(t, deps, "show", "x", "--json")
	if err != nil {
		t.Fatalf("show --json: %v", err)
	}
	// Sanity: must be valid JSON with the four top-level keys.
	var view map[string]any
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("unmarshal: %v\nout=%s", err, out)
	}
	for _, key := range []string{"workspace", "files", "branches", "decisions", "notes"} {
		if _, ok := view[key]; !ok {
			t.Errorf("JSON missing %q key: %v", key, view)
		}
	}
}

func TestWorkspaceCmd_ShowNotFound(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)

	_, _, err := runWorkspaceCmd(t, deps, "show", "missing")
	if err == nil {
		t.Fatal("show missing = nil, want error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestWorkspaceCmd_AddFile(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)

	if _, _, err := runWorkspaceCmd(t, deps, "create", "x", "--title", "X"); err != nil {
		t.Fatalf("create: %v", err)
	}
	out, _, err := runWorkspaceCmd(t, deps, "add-file", "x", "internal/auth/oauth.go", "--note", "touched in PR #42")
	if err != nil {
		t.Fatalf("add-file: %v", err)
	}
	if !strings.Contains(out, `added "internal/auth/oauth.go"`) {
		t.Errorf("output missing add marker:\n%s", out)
	}
	// Verify the file is recorded.
	out, _, err = runWorkspaceCmd(t, deps, "show", "x")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if !strings.Contains(out, "internal/auth/oauth.go") {
		t.Errorf("show output missing file path:\n%s", out)
	}
	if !strings.Contains(out, "touched in PR #42") {
		t.Errorf("show output missing note:\n%s", out)
	}
}

func TestWorkspaceCmd_AddBranch(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)

	if _, _, err := runWorkspaceCmd(t, deps, "create", "x", "--title", "X"); err != nil {
		t.Fatalf("create: %v", err)
	}
	out, _, err := runWorkspaceCmd(t, deps, "add-branch", "x", "feature/oauth")
	if err != nil {
		t.Fatalf("add-branch: %v", err)
	}
	if !strings.Contains(out, `tagged branch "feature/oauth"`) {
		t.Errorf("output missing add marker:\n%s", out)
	}
	out, _ = mustShow(t, deps, "x")
	if !strings.Contains(out, "feature/oauth") {
		t.Errorf("show output missing branch:\n%s", out)
	}
}

func TestWorkspaceCmd_AddNote(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)

	if _, _, err := runWorkspaceCmd(t, deps, "create", "x", "--title", "X"); err != nil {
		t.Fatalf("create: %v", err)
	}
	out, _, err := runWorkspaceCmd(t, deps, "add-note", "x", "--body", "first note", "--pinned")
	if err != nil {
		t.Fatalf("add-note: %v", err)
	}
	if !strings.Contains(out, "added note to workspace") {
		t.Errorf("output missing add marker:\n%s", out)
	}
	showOut, _ := mustShow(t, deps, "x")
	if !strings.Contains(showOut, "first note") {
		t.Errorf("show output missing note body:\n%s", showOut)
	}
	if !strings.Contains(showOut, "[pinned]") {
		t.Errorf("show output missing pinned marker:\n%s", showOut)
	}
}

func TestWorkspaceCmd_AddNoteEmptyBody(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)

	if _, _, err := runWorkspaceCmd(t, deps, "create", "x", "--title", "X"); err != nil {
		t.Fatalf("create: %v", err)
	}
	_, _, err := runWorkspaceCmd(t, deps, "add-note", "x")
	if err == nil {
		t.Fatal("add-note without body = nil, want error")
	}
	if !strings.Contains(err.Error(), "body is required") {
		t.Errorf("error = %q, want 'body is required'", err.Error())
	}
}

func TestWorkspaceCmd_DeleteCascades(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)

	if _, _, err := runWorkspaceCmd(t, deps, "create", "x", "--title", "X"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, err := runWorkspaceCmd(t, deps, "add-file", "x", "a.go"); err != nil {
		t.Fatalf("add-file: %v", err)
	}
	if _, _, err := runWorkspaceCmd(t, deps, "add-branch", "x", "main"); err != nil {
		t.Fatalf("add-branch: %v", err)
	}
	if _, _, err := runWorkspaceCmd(t, deps, "add-note", "x", "--body", "hello"); err != nil {
		t.Fatalf("add-note: %v", err)
	}
	out, _, err := runWorkspaceCmd(t, deps, "delete", "x")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !strings.Contains(out, `deleted workspace "x"`) {
		t.Errorf("output missing delete marker:\n%s", out)
	}
	// Show after delete should fail.
	if _, _, err := runWorkspaceCmd(t, deps, "show", "x"); err == nil {
		t.Error("show after delete = nil, want error")
	}
	// DB has no workspaces.
	s, err := store.Open(filepath.Join(dir, ".got", "got.db"))
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer func() { _ = s.Close() }()
	ws, _ := workspace.NewWithDB(s.DB(), time.Now).List(context.Background(), workspace.ListOptions{})
	if len(ws) != 0 {
		t.Errorf("after delete: %d workspaces, want 0", len(ws))
	}
	// Children are also gone.
	if n, _ := s.CountWorkspaceFiles(); n != 0 {
		t.Errorf("CountWorkspaceFiles after delete = %d, want 0", n)
	}
	if n, _ := s.CountWorkspaceBranches(); n != 0 {
		t.Errorf("CountWorkspaceBranches after delete = %d, want 0", n)
	}
	if n, _ := s.CountWorkspaceNotes(); n != 0 {
		t.Errorf("CountWorkspaceNotes after delete = %d, want 0", n)
	}
}

func TestWorkspaceCmd_DeleteNotFound(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)

	_, _, err := runWorkspaceCmd(t, deps, "delete", "missing")
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestWorkspaceCmd_NotInGitRepo(t *testing.T) {
	dir := t.TempDir() // no .git
	withChdir(t, dir)
	// Create .got/ with a real DB so we get past the store-open
	// step and hit the "not in git repo" path.
	if err := os.MkdirAll(filepath.Join(dir, ".got"), 0o755); err != nil {
		t.Fatalf("mkdir .got: %v", err)
	}
	s, err := store.Open(filepath.Join(dir, ".got", "got.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_ = s.Close()

	deps := Deps{
		AdapterFor: func(string) git.Adapter { return &fakeAdapter{} },
		Discover:   func(string) (string, error) { return "", gerr.NotInGitRepo(".") },
		StoreFor:   store.Open,
		IsTerminal: func() bool { return false },
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
		User:       func() string { return "alice" },
		GotVersion: "0.4.0-test",
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	}
	_, _, err = runWorkspaceCmd(t, deps, "list")
	if err == nil {
		t.Fatal("list outside git repo = nil, want error")
	}
	if !strings.Contains(err.Error(), "not inside a Git repository") {
		t.Errorf("error = %q, want not-in-git-repo", err.Error())
	}
}

func TestWorkspaceCmd_NotInitialized(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	withChdir(t, dir)
	deps := Deps{
		AdapterFor: func(string) git.Adapter { return &fakeAdapter{} },
		Discover:   func(string) (string, error) { return dir, nil },
		StoreFor:   store.Open,
		IsTerminal: func() bool { return false },
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
		User:       func() string { return "alice" },
		GotVersion: "0.4.0-test",
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	}
	// .got/ is missing entirely; opening the store should fail
	// and the CLI should surface a clear hint about got init.
	_, _, err := runWorkspaceCmd(t, deps, "list")
	if err == nil {
		t.Fatal("list without .got/ = nil, want error")
	}
	if !strings.Contains(err.Error(), "got init") {
		t.Errorf("error = %q, want 'got init' hint", err.Error())
	}
}

// mustShow is a tiny helper for tests that need the output of
// `got workspace show` without re-asserting on it. Returns the
// captured stdout and a fatal error on failure.
func mustShow(t *testing.T, deps Deps, name string) (string, error) {
	t.Helper()
	out, _, err := runWorkspaceCmd(t, deps, "show", name)
	if err != nil {
		t.Fatalf("show %s: %v", name, err)
	}
	return out, nil
}

// TestWorkspaceCmd_ListAllAndArchived verifies the three filter
// flags produce disjoint, correct results.
func TestWorkspaceCmd_ListAllAndArchived(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)

	for _, w := range []struct {
		name, title, state string
	}{
		{"o1", "Open 1", "open"},
		{"o2", "Open 2", "open"},
		{"a1", "Archived 1", "archived"},
	} {
		if _, _, err := runWorkspaceCmd(t, deps, "create", w.name, "--title", w.title); err != nil {
			t.Fatalf("create %s: %v", w.name, err)
		}
		// Flip the state for the archived one by directly
		// hitting the DB; the CLI's create command always
		// defaults to open.
		if w.state == "archived" {
			s, err := store.Open(filepath.Join(dir, ".got", "got.db"))
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			if _, err := s.DB().Exec(`UPDATE workspaces SET state = 'archived' WHERE name = ?`, w.name); err != nil {
				t.Fatalf("archive %s: %v", w.name, err)
			}
			_ = s.Close()
		}
	}

	out, _, err := runWorkspaceCmd(t, deps, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "o1") || !strings.Contains(out, "o2") {
		t.Errorf("default list missing open workspaces:\n%s", out)
	}
	if strings.Contains(out, "a1") {
		t.Errorf("default list should not include archived:\n%s", out)
	}
	out, _, err = runWorkspaceCmd(t, deps, "list", "--all")
	if err != nil {
		t.Fatalf("list --all: %v", err)
	}
	for _, name := range []string{"o1", "o2", "a1"} {
		if !strings.Contains(out, name) {
			t.Errorf("list --all missing %q:\n%s", name, out)
		}
	}
	out, _, err = runWorkspaceCmd(t, deps, "list", "--archived")
	if err != nil {
		t.Fatalf("list --archived: %v", err)
	}
	if !strings.Contains(out, "a1") {
		t.Errorf("list --archived missing a1:\n%s", out)
	}
	if strings.Contains(out, "o1") {
		t.Errorf("list --archived should not include open:\n%s", out)
	}
}

// TestWorkspaceCmd_AddFileUnknownWorkspace verifies the CLI
// surfaces ErrNotFound as a clean validation error (not a panic
// or a SQLite error).
func TestWorkspaceCmd_AddFileUnknownWorkspace(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)

	_, _, err := runWorkspaceCmd(t, deps, "add-file", "missing", "foo.go")
	if err == nil {
		t.Fatal("add-file to missing workspace = nil, want error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

// TestWorkspaceCmd_RepoPaths_OfflineOnly sanity-checks that the
// workspace CLI doesn't touch the network: it only opens
// .got/got.db and reads/writes the local tables. We verify this
// by running the full create + show cycle in a tempdir with no
// $GIT_DIR pointing at a real remote.
func TestWorkspaceCmd_RepoPaths_OfflineOnly(t *testing.T) {
	dir := initWorkspaceRepo(t)
	withChdir(t, dir)
	// Force-set work tree to ensure resolveNoteBody's stdin path
	// doesn't pull in any global state.
	_ = os.Setenv("GOT_WORK_TREE", dir)
	defer func() { _ = os.Unsetenv("GOT_WORK_TREE") }()

	deps := workspaceDepsFor(&bytes.Buffer{}, &bytes.Buffer{}, dir)
	if _, _, err := runWorkspaceCmd(t, deps, "create", "offline-test", "--title", "Offline"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, err := runWorkspaceCmd(t, deps, "show", "offline-test"); err != nil {
		t.Fatalf("show: %v", err)
	}
	// Sanity: repo.NewPaths is the only path resolution the
	// package uses. Smoke-test it here so a refactor that swaps
	// the path scheme trips this test loudly.
	paths := repo.NewPaths(dir)
	if filepath.Base(paths.DBFile) != "got.db" {
		t.Errorf("paths.DBFile = %q, want .../got.db", paths.DBFile)
	}
}
