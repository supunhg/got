// Copyright 2026 Supun Hewagamage. MIT License.
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/supunhg/got/internal/git"
)

func newWorktreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worktree",
		Short: "Manage Git worktrees",
		Long:  `Manage Git worktrees for parallel development branches.`,
	}

	cmd.AddCommand(newWorktreeListCmd())
	cmd.AddCommand(newWorktreeCreateCmd())
	cmd.AddCommand(newWorktreeDeleteCmd())

	return cmd
}

func newWorktreeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all worktrees",
		Long:  `List all worktrees in the current repository.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWorktreeList(cmd)
		},
	}
}

func newWorktreeCreateCmd() *cobra.Command {
	var branch string

	cmd := &cobra.Command{
		Use:   "create <path>",
		Short: "Create a new worktree",
		Long:  `Create a new worktree at the specified path.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorktreeCreate(cmd, args[0], branch)
		},
	}

	cmd.Flags().StringVarP(&branch, "branch", "b", "", "Branch to checkout in the worktree")

	return cmd
}

func newWorktreeDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <path>",
		Short: "Delete a worktree",
		Long:  `Delete the worktree at the specified path.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorktreeDelete(cmd, args[0])
		},
	}
}

func runWorktreeList(cmd *cobra.Command) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("worktree list: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("worktree list: %w", err)
	}

	worktrees, err := adapter.ListWorktrees(ctx)
	if err != nil {
		return fmt.Errorf("worktree list: %w", err)
	}

	if len(worktrees) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No worktrees found.")
		return nil
	}

	for _, w := range worktrees {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", w.Path, w.Branch, w.Head)
	}

	return nil
}

func runWorktreeCreate(cmd *cobra.Command, path, branch string) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("worktree create: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("worktree create: %w", err)
	}

	if err := adapter.CreateWorktree(ctx, path, branch); err != nil {
		return fmt.Errorf("worktree create: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created worktree at %s\n", path)
	return nil
}

func runWorktreeDelete(cmd *cobra.Command, path string) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("worktree delete: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("worktree delete: %w", err)
	}

	if err := adapter.DeleteWorktree(ctx, path); err != nil {
		return fmt.Errorf("worktree delete: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Deleted worktree at %s\n", path)
	return nil
}
