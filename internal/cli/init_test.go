package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/got-sh/got/internal/config"
	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/initwiz"
	"github.com/got-sh/got/internal/repo"
	"github.com/got-sh/got/internal/store"
	"github.com/got-sh/got/internal/tui"
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
// with deterministic time/user/version. The wizard is stubbed to
// return canned answers; IsTerminal is true by default so the
// wizard code path is exercised unless the test sets opts.noTUI.
func initDepsFor(stdout, stderr *bytes.Buffer, version, user string, at time.Time, answers initwiz.Answers) Deps {
	return Deps{
		AdapterFor: func(workTree string) git.Adapter {
			return git.NewExecAdapter(workTree)
		},
		Discover: repo.Discover,
		StoreFor: store.Open,
		RunWizard: func(d initwiz.Detected, pre initwiz.PrePopulated, theme tui.Theme) (initwiz.Answers, error) {
			// Honor pre-populated values; fall back to canned.
			out := answers
			if pre.Name != "" {
				out.Name = pre.Name
			}
			if pre.DefaultBranch != "" {
				out.DefaultBranch = pre.DefaultBranch
			}
			if pre.CommitStyle != "" {
				out.CommitStyle = pre.CommitStyle
			}
			if pre.CustomTemplate != "" {
				out.CustomTemplate = pre.CustomTemplate
			}
			return out, nil
		},
		IsTerminal: func() bool { return true },
		Now:        func() time.Time { return at },
		User:       func() string { return user },
		GotVersion: version,
		Stdout:     stdout,
		Stderr:     stderr,
	}
}

func cannedAnswers() initwiz.Answers {
	return initwiz.Answers{
		Name:          "wizname",
		DefaultBranch: "wizbranch",
		CommitStyle:   "conventional",
		Plugins:       []string{},
	}
}

func TestInitCmd_FreshRepo_Wizard(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := initDepsFor(stdout, stderr, "0.1.0-test", "alice", time.Unix(1_700_000_000, 0).UTC(), cannedAnswers())
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init: %v\nstderr=%s", err, stderr.String())
	}

	// Wizard produced name "wizname" + branch "wizbranch" — verify
	// got.yml reflects them, not the detected defaults.
	cfg, err := config.ReadProjectConfig(filepath.Join(dir, "got.yml"))
	if err != nil {
		t.Fatalf("ReadProjectConfig: %v", err)
	}
	if cfg.Project.Name != "wizname" {
		t.Errorf("Project.Name = %q, want wizname (from wizard)", cfg.Project.Name)
	}
	if cfg.Project.DefaultBranch != "wizbranch" {
		t.Errorf("DefaultBranch = %q, want wizbranch (from wizard)", cfg.Project.DefaultBranch)
	}
}

func TestInitCmd_FreshRepo_NonInteractive(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := initDepsFor(stdout, stderr, "0.1.0-test", "alice", time.Unix(1_700_000_000, 0).UTC(), cannedAnswers())
	// Simulate --no-tui by setting IsTerminal=false.
	deps.IsTerminal = func() bool { return false }
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init", "--no-tui"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --no-tui: %v", err)
	}
	// Non-interactive: name = dir basename, branch = main (detected
	// branch in a fresh init -b main repo).
	cfg, err := config.ReadProjectConfig(filepath.Join(dir, "got.yml"))
	if err != nil {
		t.Fatalf("ReadProjectConfig: %v", err)
	}
	if cfg.Project.Name != filepath.Base(dir) {
		t.Errorf("Name = %q, want %q (non-interactive default)", cfg.Project.Name, filepath.Base(dir))
	}
}

func TestInitCmd_FreshRepo_FlagsOverrideWizard(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := initDepsFor(stdout, stderr, "0.1.0-test", "alice", time.Unix(1_700_000_000, 0).UTC(), cannedAnswers())
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	// --name should override the wizard's "wizname" answer.
	cmd.SetArgs([]string{"init", "--name", "cliwin"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}
	cfg, _ := config.ReadProjectConfig(filepath.Join(dir, "got.yml"))
	if cfg.Project.Name != "cliwin" {
		t.Errorf("Name = %q, want cliwin (--name should beat wizard)", cfg.Project.Name)
	}
}

func TestInitCmd_RefusesIdempotent(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := initDepsFor(stdout, stderr, "0.1.0-test", "alice", time.Unix(1_700_000_000, 0).UTC(), cannedAnswers())
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init", "--no-tui"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("first init: %v", err)
	}
	firstAt, err := readMeta(filepath.Join(dir, ".got", "got.db"), "init_at")
	if err != nil {
		t.Fatalf("read init_at: %v", err)
	}

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
	cmd.SetArgs([]string{"init", "--force", "--no-tui"})
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
	deps := initDepsFor(stdout, stderr, "0.1.0-test", "alice", time.Unix(1_700_000_000, 0).UTC(), cannedAnswers())
	deps.IsTerminal = func() bool { return false } // non-interactive
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{
		"init",
		"--no-tui",
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
		t.Errorf("Commits.Style = %q, want freeform", cfg.Commits.Style)
	}
}

func TestInitCmd_OutsideGitRepoFails(t *testing.T) {
	dir := t.TempDir() // No git init.
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := initDepsFor(stdout, stderr, "0.1.0-test", "alice", time.Now(), cannedAnswers())
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init", "--here", "--no-tui"})
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
	deps := initDepsFor(stdout, stderr, "0.1.0-test", "alice", time.Unix(1_700_000_000, 0).UTC(), cannedAnswers())
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init", "--here", "--no-tui"})
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
	deps := initDepsFor(stdout, stderr, "0.1.0-test", "alice", time.Unix(1_700_000_000, 0).UTC(), cannedAnswers())
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init", outer, "--no-tui"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init <path>: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outer, ".got", "got.db")); err != nil {
		t.Errorf("expected .got/got.db in %s: %v", outer, err)
	}
}

func TestInitCmd_ForcePreservesDBContent(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := initDepsFor(stdout, stderr, "0.1.0-test", "alice", time.Unix(1_700_000_000, 0).UTC(), cannedAnswers())
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init", "--no-tui"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("first init: %v", err)
	}

	st, err := store.Open(filepath.Join(dir, ".got", "got.db"))
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	if err := st.MetaSet("user_thing", "preserve_me"); err != nil {
		t.Fatalf("MetaSet: %v", err)
	}
	_ = st.Close()

	stdout.Reset()
	stderr.Reset()
	deps.Now = func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }
	cmd = NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init", "--force", "--no-tui"})
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

// initGitRepo creates a fresh git repo in a tempdir and returns the
// dir.
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

// Compile-time guard: gerr.Error implements the error interface.
var _ error = (*gerr.Error)(nil)

// Compile-time guard: errors is used somewhere; keep it from
// being dropped by goimports.
var _ = errors.New
