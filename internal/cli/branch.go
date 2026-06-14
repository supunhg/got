// Copyright 2026 Supun Hewagamage. MIT License.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/supunhg/got/internal/git"
)

func newBranchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "branch",
		Short: "List, create, delete, and checkout branches",
		Long: `Manage Git branches.

Without arguments, lists all local branches with the current one marked.

Subcommands:
  got branch                    List branches
  got branch create <name>      Create a new branch at HEAD
  got branch delete <name>      Delete a branch
  got branch checkout <name>    Switch to a branch

Examples:
  got branch
  got branch create feature/oauth
  got branch checkout main
  got branch delete feature/oauth
  got branch --json`,

		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// No args -> list.
			return runBranchList(cmd)
		},
	}

	cmd.AddCommand(newBranchCreateCmd())
	cmd.AddCommand(newBranchDeleteCmd())
	cmd.AddCommand(newBranchCheckoutCmd())

	cmd.Flags().Bool("json", false, "Output as JSON")

	return cmd
}

func newBranchCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new branch at HEAD",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBranchCreate(cmd, args[0])
		},
	}

	return cmd
}

func newBranchDeleteCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBranchDelete(cmd, args[0], force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force delete")
	return cmd
}

func newBranchCheckoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checkout <name>",
		Short: "Switch to a branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBranchCheckout(cmd, args[0])
		},
	}

	return cmd
}

func runBranchList(cmd *cobra.Command) error {
	jsonOut, _ := cmd.Flags().GetBool("json")

	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("branch: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("branch: %w", err)
	}

	branches, err := adapter.ListBranches(ctx)
	if err != nil {
		return fmt.Errorf("branch: %w", err)
	}

	if jsonOut {
		if branches == nil {
			branches = []git.Branch{}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(branches)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)

	if len(branches) == 0 {
		fmt.Fprintln(w, "No branches found.")
		return w.Flush()
	}

	fmt.Fprintln(w, "BRANCH\tSTATUS\tUPSTREAM")

	for _, b := range branches {
		status := " "
		if b.Current {
			status = "*"
		}
		upstream := b.Upstream
		if upstream == "" {
			upstream = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", b.Name, status, upstream)
	}

	return w.Flush()
}

func runBranchCreate(cmd *cobra.Command, name string) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("branch: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("branch: %w", err)
	}

	if err := adapter.CreateBranch(ctx, name); err != nil {
		return fmt.Errorf("branch: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created branch %q\n", name)
	return nil
}

func runBranchDelete(cmd *cobra.Command, name string, force bool) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("branch: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("branch: %w", err)
	}

	if err := adapter.DeleteBranch(ctx, name, force); err != nil {
		return fmt.Errorf("branch: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Deleted branch %q\n", name)
	return nil
}

func runBranchCheckout(cmd *cobra.Command, name string) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("branch: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("branch: %w", err)
	}

	if err := adapter.CheckoutBranch(ctx, name); err != nil {
		return fmt.Errorf("branch: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Switched to branch %q\n", name)
	return nil
}
