package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/branchwiz"
	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/tui"
)

// newBranchCmd builds the `got branch` command tree:
//
//	got branch                          list local branches
//	got branch -a                       list local + remote
//	got branch -r                       list remote only
//	got branch --json                   machine-readable JSON
//	got branch create <name>            create a branch (no checkout)
//	got branch create <name> --from X   create at start point X
//	got branch checkout <name>          switch to a branch
//	got branch delete <name>            delete a branch
//	got branch delete -f <name>         force-delete (git branch -D)
//	got branch move <old> <new>         rename (v0.1: stubbed)
//
// Interactive TUI path: when stdout is a TTY and no positional
// argument is given, the CLI drops into the branchwiz wizard
// (list / create / checkout / delete / confirm).
func newBranchCmd(d Deps) *cobra.Command {
	var (
		showAll     bool
		showRemotes bool
		asJSON      bool
	)
	cmd := &cobra.Command{
		Use:   "branch",
		Short: "List, create, checkout, or delete branches",
		Long: `List, create, checkout, or delete branches.

By default ` + "`got branch`" + ` lists local branches. Use -a/--all to
include remote-tracking branches, -r/--remotes for remote only, or
--json for machine-readable output.

When stdout is a TTY and no subcommand is given, the command drops
into an interactive wizard (list / create / checkout / delete).
Pass --no-tui to force plain CLI output.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBranchRoot(cmd, d, showAll, showRemotes, asJSON)
		},
	}
	cmd.Flags().BoolVarP(&showAll, "all", "a", false, "list both local and remote-tracking branches")
	cmd.Flags().BoolVarP(&showRemotes, "remotes", "r", false, "list remote-tracking branches")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable JSON output")

	cmd.AddCommand(newBranchCreateCmd(d))
	cmd.AddCommand(newBranchCheckoutCmd(d))
	cmd.AddCommand(newBranchDeleteCmd(d))
	cmd.AddCommand(newBranchMoveCmd())
	return cmd
}

// newBranchCreateCmd builds `got branch create <name> [--from X]`.
func newBranchCreateCmd(d Deps) *cobra.Command {
	var from string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new branch at HEAD (or at --from <ref>)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBranchCreate(cmd.Context(), d, args[0], from)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "start point: a commit SHA, branch, or tag (defaults to HEAD)")
	return cmd
}

// newBranchCheckoutCmd builds `got branch checkout <name>`. Distinct
// from `git checkout` so the subcommand tree stays discoverable.
func newBranchCheckoutCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checkout <name>",
		Short: "Switch the working tree to the named branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBranchCheckout(cmd.Context(), d, args[0])
		},
	}
	return cmd
}

// newBranchDeleteCmd builds `got branch delete <name> [-f]`.
func newBranchDeleteCmd(d Deps) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a branch (refuses to delete the current branch)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBranchDelete(cmd.Context(), d, args[0], force)
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "force-delete even if the branch is not fully merged (git branch -D)")
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

// runBranchRoot is the entry point for the `got branch` parent
// command (no subcommand). It picks between the list path and the
// interactive wizard based on TTY + --no-tui.
func runBranchRoot(cmd *cobra.Command, deps Deps, all, remotes, asJSON bool) error {
	// Honor the global --no-tui flag the same way commit does.
	noTUI := false
	if v, err := cmd.Root().PersistentFlags().GetBool("no-tui"); err == nil {
		noTUI = v
	}
	useWizard := !noTUI && deps.IsTerminal != nil && deps.IsTerminal() && deps.RunBranchWizard != nil
	if useWizard {
		return runBranchWizard(cmd, deps)
	}
	return runBranchList(cmd, deps, all, remotes, asJSON)
}

// runBranchList lists branches and renders them as a table or JSON.
func runBranchList(cmd *cobra.Command, deps Deps, all, remotes, asJSON bool) error {
	logger := loggerFor(deps)
	logger.Info("branch list starting", "all", all, "remotes", remotes, "json", asJSON)
	workTree, err := deps.Discover(".")
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)
	branches, err := a.Branches(cmd.Context())
	if err != nil {
		logger.Warn("branch list failed", "err", err.Error())
		return err
	}
	if remotes || all {
		remote, err := a.RemoteBranches(cmd.Context())
		if err != nil {
			logger.Warn("branch list remote lookup failed", "err", err.Error())
			return err
		}
		branches = append(branches, remote...)
	}
	logger.Info("branch list finished", "count", len(branches))
	out := cmd.OutOrStdout()
	if out == nil {
		out = deps.Stdout
	}
	if asJSON {
		return writeJSON(out, branches)
	}
	return writeBranchTable(out, branches)
}

// runBranchCreate creates a new branch at the named start point.
// Does NOT check the branch out.
func runBranchCreate(ctx context.Context, deps Deps, name, from string) error {
	logger := loggerFor(deps)
	logger.Info("branch create starting", "name", name, "from", from)
	workTree, err := deps.Discover(".")
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)
	if err := a.CreateBranch(ctx, name, from); err != nil {
		logger.Warn("branch create failed", "name", name, "err", err.Error())
		return err
	}
	logger.Info("branch create finished", "name", name)
	_, _ = fmt.Fprintf(deps.Stdout, "[got] created branch %q\n", name)
	return nil
}

// runBranchCheckout switches the working tree to the named branch.
func runBranchCheckout(ctx context.Context, deps Deps, name string) error {
	logger := loggerFor(deps)
	logger.Info("branch checkout starting", "name", name)
	workTree, err := deps.Discover(".")
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)
	if err := a.Checkout(ctx, name, git.CheckoutOpts{}); err != nil {
		logger.Warn("branch checkout failed", "name", name, "err", err.Error())
		return err
	}
	logger.Info("branch checkout finished", "name", name)
	_, _ = fmt.Fprintf(deps.Stdout, "[got] switched to branch %q\n", name)
	return nil
}

// runBranchDelete deletes the named branch. Refuses to delete the
// current branch. With force=true, uses `git branch -D` (unmerged
// work is lost).
func runBranchDelete(ctx context.Context, deps Deps, name string, force bool) error {
	logger := loggerFor(deps)
	logger.Info("branch delete starting", "name", name, "force", force)
	workTree, err := deps.Discover(".")
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)
	branches, err := a.Branches(ctx)
	if err != nil {
		logger.Warn("branch lookup failed", "err", err.Error())
		return err
	}
	for _, b := range branches {
		if b.Name == name && b.IsCurrent {
			logger.Warn("branch delete refused: current branch", "name", name)
			return gerr.Validation(fmt.Sprintf("cannot delete the branch you are currently on (%q); switch to another branch first", name))
		}
	}
	if err := a.DeleteBranch(ctx, name, force); err != nil {
		logger.Warn("branch delete failed", "name", name, "err", err.Error())
		return err
	}
	logger.Info("branch delete finished", "name", name, "force", force)
	if force {
		_, _ = fmt.Fprintf(deps.Stdout, "[got] force-deleted branch %q\n", name)
	} else {
		_, _ = fmt.Fprintf(deps.Stdout, "[got] deleted branch %q\n", name)
	}
	return nil
}

// runBranchWizard drives the branchwiz package: list local branches,
// drop the user into the wizard, and apply the resolved action.
func runBranchWizard(cmd *cobra.Command, deps Deps) error {
	workTree, err := deps.Discover(".")
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)
	branches, err := a.Branches(cmd.Context())
	if err != nil {
		return err
	}
	ans, err := deps.RunBranchWizard(branches, branchwiz.PrePopulated{}, tui.NewTheme())
	if err != nil {
		return err
	}
	switch ans.Action {
	case branchwiz.ActionCreate:
		return runBranchCreate(cmd.Context(), deps, ans.Name, ans.StartPoint)
	case branchwiz.ActionCheckout:
		return runBranchCheckout(cmd.Context(), deps, ans.Name)
	case branchwiz.ActionDelete:
		return runBranchDelete(cmd.Context(), deps, ans.Name, ans.Force)
	}
	// No action resolved; this shouldn't happen because the wizard
	// always sets one before reaching stateDone.
	return gerr.Validation("branch wizard returned no action")
}

// writeBranchTable renders branches as a human-readable table with
// a `*` marker on the current branch and a `-> upstream` suffix on
// branches that have an upstream.
func writeBranchTable(w io.Writer, branches []git.Branch) error {
	if len(branches) == 0 {
		_, err := fmt.Fprintln(w, "(no branches)")
		return err
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
		if _, err := fmt.Fprintf(w, "%s%s  %s%s\n", marker, pad(b.Name, nameW), b.SHA, upstream); err != nil {
			return err
		}
	}
	return nil
}

// pad right-pads s with spaces to width n.
func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
