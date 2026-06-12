package cli

import (
	"strings"
	"testing"

	"github.com/got-sh/got/internal/git"
)

func TestBranchCmd_ListTable(t *testing.T) {
	a := git.NewFake()
	a.BranchesVal = []git.Branch{
		{Name: "main", IsCurrent: true, SHA: "abc1234"},
		{Name: "feature", SHA: "def5678", Upstream: "origin/feature"},
	}
	cmd, stdout, _ := setupCLITest(a)
	cmd.SetArgs([]string{"branch"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, "* main") {
		t.Errorf("missing '* main' marker: %q", out)
	}
	if !strings.Contains(out, "feature") {
		t.Errorf("missing 'feature': %q", out)
	}
	if !strings.Contains(out, "-> origin/feature") {
		t.Errorf("missing upstream arrow: %q", out)
	}
}

func TestBranchCmd_ListEmpty(t *testing.T) {
	a := git.NewFake()
	a.BranchesVal = []git.Branch{}
	cmd, stdout, _ := setupCLITest(a)
	cmd.SetArgs([]string{"branch"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "(no branches)") {
		t.Errorf("missing empty message: %q", stdout.String())
	}
}

func TestBranchCmd_ListJSON(t *testing.T) {
	a := git.NewFake()
	a.BranchesVal = []git.Branch{{Name: "main", IsCurrent: true, SHA: "abc1234"}}
	cmd, stdout, _ := setupCLITest(a)
	cmd.SetArgs([]string{"branch", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"name": "main"`) {
		t.Errorf("missing JSON: %q", stdout.String())
	}
}

func TestBranchCmd_ListRemotes(t *testing.T) {
	a := git.NewFake()
	a.BranchesVal = []git.Branch{{Name: "main", IsCurrent: true, SHA: "abc1234"}}
	a.RemoteBranchesVal = []git.Branch{
		{Name: "origin/main", IsRemote: true, SHA: "def5678"},
	}
	cmd, stdout, _ := setupCLITest(a)
	cmd.SetArgs([]string{"branch", "-r"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if a.RemoteBranchesCalls != 1 {
		t.Errorf("RemoteBranchesCalls = %d, want 1", a.RemoteBranchesCalls)
	}
	if !strings.Contains(stdout.String(), "origin/main") {
		t.Errorf("missing remote branch: %q", stdout.String())
	}
}

func TestBranchCmd_ListAll(t *testing.T) {
	a := git.NewFake()
	a.BranchesVal = []git.Branch{{Name: "main", IsCurrent: true, SHA: "abc1234"}}
	a.RemoteBranchesVal = []git.Branch{
		{Name: "origin/main", IsRemote: true, SHA: "def5678"},
	}
	cmd, stdout, _ := setupCLITest(a)
	cmd.SetArgs([]string{"branch", "-a"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if a.RemoteBranchesCalls != 1 {
		t.Errorf("RemoteBranchesCalls = %d, want 1", a.RemoteBranchesCalls)
	}
	for _, want := range []string{"main", "origin/main"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("missing %q: %q", want, stdout.String())
		}
	}
}

func TestBranchCmd_DeleteCurrent(t *testing.T) {
	a := git.NewFake()
	a.BranchesVal = []git.Branch{
		{Name: "main", IsCurrent: true, SHA: "abc1234"},
		{Name: "feature", SHA: "def5678"},
	}
	cmd, _, _ := setupCLITest(a)
	cmd.SetArgs([]string{"branch", "delete", "main"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when deleting current branch")
	}
	if !strings.Contains(err.Error(), "currently on") {
		t.Errorf("error should mention current branch: %v", err)
	}
}

func TestBranchCmd_DeleteNotFound(t *testing.T) {
	a := git.NewFake()
	a.BranchesVal = []git.Branch{{Name: "main", IsCurrent: true}}
	cmd, _, _ := setupCLITest(a)
	cmd.SetArgs([]string{"branch", "delete", "nope"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing branch")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should say not found: %v", err)
	}
}
