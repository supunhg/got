package cli

import (
	"strings"
	"testing"

	"github.com/got-sh/got/internal/git"
)

func TestRemoteListCmd_Table(t *testing.T) {
	a := git.NewFake()
	a.RemotesVal = []git.Remote{
		{Name: "origin", FetchURL: "https://example.com/foo.git", PushURL: "git@example.com:foo.git"},
		{Name: "upstream", FetchURL: "https://example.com/bar.git", PushURL: "https://example.com/bar.git"},
	}
	cmd, stdout, _ := setupCLITest(a)
	cmd.SetArgs([]string{"remote", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, want := range []string{"origin", "upstream", "https://example.com/foo.git", "git@example.com:foo.git"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q: %q", want, out)
		}
	}
}

func TestRemoteListCmd_Empty(t *testing.T) {
	a := git.NewFake()
	a.RemotesVal = []git.Remote{}
	cmd, stdout, _ := setupCLITest(a)
	cmd.SetArgs([]string{"remote", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "(no remotes)") {
		t.Errorf("missing empty message: %q", stdout.String())
	}
}

func TestRemoteListCmd_JSON(t *testing.T) {
	a := git.NewFake()
	a.RemotesVal = []git.Remote{{Name: "origin", FetchURL: "u", PushURL: "u"}}
	cmd, stdout, _ := setupCLITest(a)
	cmd.SetArgs([]string{"remote", "list", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"name": "origin"`) {
		t.Errorf("missing JSON: %q", stdout.String())
	}
}

func TestRemoteAddCmd_NotImplemented(t *testing.T) {
	a := git.NewFake()
	cmd, _, _ := setupCLITest(a)
	cmd.SetArgs([]string{"remote", "add", "origin", "https://x"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error for unimplemented subcommand")
	}
}
