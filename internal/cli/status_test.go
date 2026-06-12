package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
)

func TestStatusCmd_NotInGitRepo(t *testing.T) {
	a := git.NewFake()
	deps := Deps{
		AdapterFor: func(string) git.Adapter { return a },
		Discover:   func(string) (string, error) { return "", gerr.NotInGitRepo("/fake/start") },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(deps.Stdout)
	cmd.SetErr(deps.Stderr)
	cmd.SetArgs([]string{"status"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when not in a git repo")
	}
	if got := gerr.ExitCode(err); got != int(gerr.CodeNotInGitRepo) {
		t.Errorf("ExitCode = %d, want %d", got, gerr.CodeNotInGitRepo)
	}
	if a.StatusCalls != 0 {
		t.Errorf("StatusCalls = %d, want 0 (adapter should not be called when discover fails)", a.StatusCalls)
	}
}

func TestStatusCmd_JSON(t *testing.T) {
	a := git.NewFake()
	a.StatusVal = git.Status{
		Branch:  "main",
		Ahead:   1,
		Entries: []git.StatusEntry{{Path: "foo.go", XY: " M", IsUnstaged: true}},
	}
	cmd, stdout, _ := setupCLITest(a)
	cmd.SetArgs([]string{"status", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got git.Status
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if got.Branch != "main" || got.Ahead != 1 {
		t.Errorf("got = %+v", got)
	}
	if len(got.Entries) != 1 || got.Entries[0].Path != "foo.go" {
		t.Errorf("entries = %+v", got.Entries)
	}
}

func TestStatusCmd_Short(t *testing.T) {
	a := git.NewFake()
	a.StatusVal = git.Status{
		Branch: "main",
		Entries: []git.StatusEntry{
			{Path: "staged.go", XY: "M ", IsStaged: true},
			{Path: "dirty.go", XY: " M", IsUnstaged: true},
			{Path: "new.go", IsUntracked: true},
		},
	}
	cmd, stdout, _ := setupCLITest(a)
	cmd.SetArgs([]string{"status", "--short"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, want := range []string{"M  staged.go", " M dirty.go", "?? new.go"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestStatusCmd_HumanClean(t *testing.T) {
	a := git.NewFake()
	a.StatusVal = git.Status{Branch: "main", Entries: []git.StatusEntry{}}
	cmd, stdout, _ := setupCLITest(a)
	cmd.SetArgs([]string{"status"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "On branch main") {
		t.Errorf("missing branch line: %q", stdout.String())
	}
}

func TestStatusCmd_HumanDiverged(t *testing.T) {
	a := git.NewFake()
	a.StatusVal = git.Status{Branch: "main", Upstream: "origin/main", Ahead: 2, Behind: 3}
	cmd, stdout, _ := setupCLITest(a)
	cmd.SetArgs([]string{"status"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "diverged") {
		t.Errorf("missing diverged line: %q", stdout.String())
	}
}
