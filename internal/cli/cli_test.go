package cli

import (
	"bytes"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/git"
)

// setupCLITest returns a fresh root command wired to the given fake
// adapter, plus the buffers that capture stdout and stderr. The discover
// function returns a fixed work tree path; tests that want to exercise
// discover errors should pass a custom Deps instead.
//
// The buffers are wired into the command via SetOut/SetErr so Cobra
// routes output through them. Without these calls Cobra would write to
// os.Stdout/os.Stderr and the test would not see anything.
func setupCLITest(a git.Adapter) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := Deps{
		AdapterFor: func(string) git.Adapter { return a },
		Discover:   func(string) (string, error) { return "/fake/worktree", nil },
		Stdout:     stdout,
		Stderr:     stderr,
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd, stdout, stderr
}
