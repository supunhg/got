package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/got-sh/got/internal/config"
	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/repo"
	"github.com/got-sh/got/internal/store"
)

// runGit runs `git args...` in dir and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// initDepsFor builds a Deps value pointed at the given stdout/stderr
// with deterministic time/user/version. The adapter factory returns a
// real *git.ExecAdapter over the given work tree — even though the
// init command path doesn't actually invoke the adapter, the type has
// to be correct.
func initDepsFor(stdout, stderr *bytes.Buffer, version, user string, at time.Time) Deps {
	return Deps{
		AdapterFor: func(workTree string) git.Adapter {
			return git.NewExecAdapter(workTree)
		},
		Discover:   repo.Discover,
		StoreFor:   store.Open,
		Now:        func() time.Time { return at },
		User:       func() string { return user },
		GotVersion: version,
		Stdout:     stdout,
		Stderr:     stderr,
	}
}

func TestInitCmd_FreshRepo(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := initDepsFor(stdout, stderr, "0.1.0-test", "alice", time.Unix(1_700_000_000, 0).UTC())
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init: %v\nstderr=%s", err, stderr.String())
	}

	// .got/ and got.yml should exist.
	for _, p := range []string{
		filepath.Join(dir, "got.yml"),
		filepath.Join(dir, ".got", "config.yaml"),
		filepath.Join(dir, ".got", "got.db"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}

	// got.yml should be valid YAML with the expected defaults.
	cfg, err := config.ReadProjectConfig(filepath.Join(dir, "got.yml"))
	if err != nil {
		t.Fatalf("ReadProjectConfig: %v", err)
	}
	if cfg.Project.Name != filepath.Base(dir) {
		t.Errorf("Project.Name = %q, want %q", cfg.Project.Name, filepath.Base(dir))
	}
	if cfg.Project.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, want main", cfg.Project.DefaultBranch)
	}
	if cfg.Commits.Style != "conventional" {
		t.Errorf("Commits.Style = %q, want conventional", cfg.Commits.Style)
	}
	if !cfg.Commits.AllowBreaking {
		t.Errorf("AllowBreaking = false, want true")
	}
	if cfg.AI.Provider != "heuristic" {
		t.Errorf("AI.Provider = %q, want heuristic", cfg.AI.Provider)
	}

	internalCfg, err := config.ReadInternalConfig(filepath.Join(dir, ".got", "config.yaml"))
	if err != nil {
		t.Fatalf("ReadInternalConfig: %v", err)
	}
	if internalCfg.Version != 1 {
		t.Errorf("internalCfg.Version = %d, want 1", internalCfg.Version)
	}
	if internalCfg.CreatedFrom != "0.1.0-test" {
		t.Errorf("internalCfg.CreatedFrom = %q, want 0.1.0-test", internalCfg.CreatedFrom)
	}

	body, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(body), ".got/") {
		t.Errorf("gitignore missing .got/:\n%s", body)
	}

	st, err := store.Open(filepath.Join(dir, ".got", "got.db"))
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer st.Close()
	if v, _ := st.MetaGet("got_version"); v != "0.1.0-test" {
		t.Errorf("meta got_version = %q, want 0.1.0-test", v)
	}
	if v, _ := st.MetaGet("init_user"); v != "alice" {
		t.Errorf("meta init_user = %q, want alice", v)
	}
	if v, _ := st.MetaGet("init_at"); v != "1700000000000" {
		t.Errorf("meta init_at = %q, want 1700000000000", v)
	}
	if v, _ := st.SchemaVersion(); v != 1 {
		t.Errorf("schema_version = %d, want 1", v)
	}

	if !strings.Contains(stdout.String(), "Initialized GOT") {
		t.Errorf("stdout missing 'Initialized GOT': %q", stdout.String())
	}
}

func TestInitCmd_RefusesIdempotent(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := initDepsFor(stdout, stderr, "0.1.0-test", "alice", time.Unix(1_700_000_000, 0).UTC())
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("first init: %v", err)
	}
	firstAt, err := readMeta(filepath.Join(dir, ".got", "got.db"), "init_at")
	if err != nil {
		t.Fatalf("read init_at: %v", err)
	}

	// Second run with no --force should fail.
	stdout.Reset()
	stderr.Reset()
	err = cmd.Execute()
	if err == nil {
		t.Fatalf("expected error on re-init, got nil; stdout=%s", stdout.String())
	}
	if got := gerr.ExitCode(err); got != 5 {
		t.Errorf("exit code = %d, want 5; err=%v", got, err)
	}

	// --force should succeed and bump init_at.
	stdout.Reset()
	stderr.Reset()
	deps.Now = func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }
	deps.User = func() string { return "bob" }
	cmd = NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init", "--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("force init: %v", err)
	}
	secondAt, err := readMeta(filepath.Join(dir, ".got", "got.db"), "init_at")
	if err != nil {
		t.Fatalf("read init_at #2: %v", err)
	}
	if firstAt == secondAt {
		t.Errorf("init_at did not change after --force: %s", firstAt)
	}
}

func TestInitCmd_FlagsOverrideDefaults(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := initDepsFor(stdout, stderr, "0.1.0-test", "alice", time.Unix(1_700_000_000, 0).UTC())
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{
		"init",
		"--name", "newname",
		"--branch", "trunk",
		"--style", "freeform",
		"--custom-template", "/tmp/template",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}
	cfg, err := config.ReadProjectConfig(filepath.Join(dir, "got.yml"))
	if err != nil {
		t.Fatalf("ReadProjectConfig: %v", err)
	}
	if cfg.Project.Name != "newname" {
		t.Errorf("Name = %q, want newname", cfg.Project.Name)
	}
	if cfg.Project.DefaultBranch != "trunk" {
		t.Errorf("DefaultBranch = %q, want trunk", cfg.Project.DefaultBranch)
	}
	if cfg.Commits.Style != "freeform" {
		t.Errorf("Commits.Style = %q, want freeform (custom-template should not change style without --style=custom)", cfg.Commits.Style)
	}
	if cfg.Commits.CustomTemplate != "/tmp/template" {
		t.Errorf("Commits.CustomTemplate = %q, want /tmp/template", cfg.Commits.CustomTemplate)
	}
}

func TestInitCmd_OutsideGitRepoFails(t *testing.T) {
	dir := t.TempDir() // No git init.
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := initDepsFor(stdout, stderr, "0.1.0-test", "alice", time.Now())
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init", "--here"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error outside git repo, got nil")
	}
	if got := gerr.ExitCode(err); got != 3 {
		t.Errorf("exit code = %d, want 3 (not in git repo)", got)
	}
}

func TestInitCmd_HereFlag(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := initDepsFor(stdout, stderr, "0.1.0-test", "alice", time.Unix(1_700_000_000, 0).UTC())
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init", "--here"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --here: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".got", "got.db")); err != nil {
		t.Errorf("expected .got/got.db: %v", err)
	}
}

func TestInitCmd_PathArg(t *testing.T) {
	outer := initGitRepo(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := initDepsFor(stdout, stderr, "0.1.0-test", "alice", time.Unix(1_700_000_000, 0).UTC())
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init", outer})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init <path>: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outer, ".got", "got.db")); err != nil {
		t.Errorf("expected .got/got.db in %s: %v", outer, err)
	}
}

// initGitRepo creates a fresh git repo in a tempdir and returns the
// dir. The repo is configured for a no-gpg-sign commit so tests don't
// depend on the host's gpg config.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

// readMeta opens a Store and returns one meta value, then closes it.
func readMeta(dbPath, key string) (string, error) {
	st, err := store.Open(dbPath)
	if err != nil {
		return "", err
	}
	defer st.Close()
	return st.MetaGet(key)
}

// TestInitCmd_ForcePreservesDBContent verifies the spec §7 promise that
// `got init --force` preserves the SQLite database. We plant a custom
// meta row before the force re-init and assert it survives.
func TestInitCmd_ForcePreservesDBContent(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := initDepsFor(stdout, stderr, "0.1.0-test", "alice", time.Unix(1_700_000_000, 0).UTC())
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("first init: %v", err)
	}

	// Plant a custom row the user might have written.
	st, err := store.Open(filepath.Join(dir, ".got", "got.db"))
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	if err := st.MetaSet("user_thing", "preserve_me"); err != nil {
		t.Fatalf("MetaSet: %v", err)
	}
	_ = st.Close()

	// Force re-init. The DB should still be there and our row should
	// survive because TouchInitMeta is the only writer on re-open.
	stdout.Reset()
	stderr.Reset()
	deps.Now = func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }
	cmd = NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init", "--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("force init: %v", err)
	}

	st2, err := store.Open(filepath.Join(dir, ".got", "got.db"))
	if err != nil {
		t.Fatalf("reopen after force: %v", err)
	}
	defer st2.Close()
	if v, _ := st2.MetaGet("user_thing"); v != "preserve_me" {
		t.Errorf("user_thing = %q, want preserve_me", v)
	}
}

// Compile-time guard: gerr.Error implements the error interface. This
// keeps the gerr import live in case the tests stop using it directly.
var _ error = (*gerr.Error)(nil)
