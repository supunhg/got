package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
)

func newBranchCmd(deps Deps) *cobra.Command {
	var (
		showAll     bool
		showRemotes bool
		asJSON      bool
	)
	cmd := &cobra.Command{
		Use:   "branch",
		Short: "List, create, or delete branches",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBranchList(cmd, deps, showAll, showRemotes, asJSON)
		},
	}
	cmd.Flags().BoolVarP(&showAll, "all", "a", false, "list both local and remote-tracking branches")
	cmd.Flags().BoolVarP(&showRemotes, "remotes", "r", false, "list remote-tracking branches")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable JSON output")

	cmd.AddCommand(newBranchDeleteCmd(deps))
	cmd.AddCommand(newBranchMoveCmd())
	return cmd
}

func newBranchDeleteCmd(deps Deps) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBranchDelete(cmd, deps, args[0], force)
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "force deletion (v0.1 still won't delete the current branch)")
	return cmd
}

// newBranchMoveCmd is a stub for `got branch move`. v0.1 doesn't ship
// the rename implementation; the stub is wired so `got branch --help`
// shows the subcommand and `got branch move` returns a clear
// "not yet implemented" error.
func newBranchMoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "move <old> <new>",
		Short: "Rename a branch",
		Args:  cobra.ExactArgs(2),
		RunE: func(*cobra.Command, []string) error {
			return gerr.Validation("`got branch move` is not yet implemented in v0.1; use `git branch -m` for now")
		},
	}
}

func runBranchList(cmd *cobra.Command, deps Deps, all, remotes, asJSON bool) error {
	workTree, err := deps.Discover(".")
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)
	branches, err := a.Branches(cmd.Context())
	if err != nil {
		return err
	}
	// If the user asked for remote-tracking branches, fetch them via
	// a second `for-each-ref` call against refs/remotes/ and append.
	if remotes || all {
		remote, err := a.RemoteBranches(cmd.Context())
		if err != nil {
			return err
		}
		branches = append(branches, remote...)
	}
	out := cmd.OutOrStdout()
	if out == nil {
		out = deps.Stdout
	}
	if asJSON {
		return writeJSON(out, branches)
	}
	return writeBranchTable(out, branches)
}

func writeBranchTable(w io.Writer, branches []git.Branch) error {
	if len(branches) == 0 {
		fmt.Fprintln(w, "(no branches)")
		return nil
	}
	nameW := 0
	for _, b := range branches {
		if l := len(b.Name); l > nameW {
			nameW = l
		}
	}
	for _, b := range branches {
		marker := "  "
		if b.IsCurrent {
			marker = "* "
		}
		upstream := ""
		if b.Upstream != "" {
			upstream = " -> " + b.Upstream
		}
		fmt.Fprintf(w, "%s%s  %s%s\n", marker, pad(b.Name, nameW), b.SHA, upstream)
	}
	return nil
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func runBranchDelete(cmd *cobra.Command, deps Deps, name string, force bool) error {
	workTree, err := deps.Discover(".")
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)
	branches, err := a.Branches(cmd.Context())
	if err != nil {
		return err
	}
	var target *git.Branch
	for i := range branches {
		if branches[i].Name == name {
			target = &branches[i]
			break
		}
	}
	if target == nil {
		return gerr.Validation(fmt.Sprintf("branch %q not found", name))
	}
	if target.IsCurrent {
		return gerr.Validation(fmt.Sprintf("cannot delete the branch you are currently on (%q); switch to another branch first", name))
	}
	_ = force
	return gerr.Validation("`got branch delete` is not yet implemented in v0.1; use `git branch -d " + name + "` for now")
}
