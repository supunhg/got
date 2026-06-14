// Copyright 2026 Supun Hewagamage. MIT License.
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/supunhg/got/internal/git"
	"github.com/supunhg/got/internal/store"
)

// newSafeCmd returns the `got safe` command tree.
func newSafeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "safe",
		Short: "Safe Git operations with automatic snapshots",
		Long: `Perform destructive Git operations with automatic recovery snapshots.

Before executing a destructive operation (reset, rebase, force push),
GOT creates a snapshot of the current state so you can recover if
something goes wrong.

Examples:
  got safe reset --hard HEAD~1
  got safe push origin main --force-with-lease
  got safe rebase main`,
	}

	cmd.AddCommand(newSafeResetCmd())
	cmd.AddCommand(newSafePushCmd())
	cmd.AddCommand(newSafeRebaseCmd())

	return cmd
}

// createAutoSnapshot creates a snapshot before a destructive operation.
func createAutoSnapshot(ctx context.Context, reason string) (*store.Snapshot, error) {
	kc, err := openKnowledgeStore()
	if err != nil {
		return nil, err
	}
	defer kc.Close()

	repoPath, err := findRepoRoot()
	if err != nil {
		return nil, fmt.Errorf("safe: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return nil, fmt.Errorf("safe: %w", err)
	}

	branch, err := adapter.CurrentBranch(ctx)
	if err != nil {
		return nil, fmt.Errorf("safe: cannot determine current branch: %w", err)
	}

	stdout, _, err := adapter.Run(ctx, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("safe: cannot get HEAD SHA: %w", err)
	}

	headSHA := stdout
	if len(headSHA) > 12 {
		headSHA = headSHA[:12]
	}
	ref := fmt.Sprintf("%s (%s)", branch, headSHA)

	s, err := kc.ks.CreateSnapshot(ctx, store.CreateSnapshotParams{
		Reason: reason,
		Ref:    ref,
	})
	if err != nil {
		return nil, fmt.Errorf("safe: create snapshot: %w", err)
	}

	return s, nil
}

func newSafeResetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset [flags] [commit]",
		Short: "Safe reset with automatic snapshot",
		Long: `Reset the repository with an automatic snapshot for recovery.

Wraps 'git reset' but creates a snapshot first so you can recover
the previous state if needed.

Examples:
  got safe reset --soft HEAD~1
  got safe reset --mixed HEAD~1
  got safe reset --hard HEAD~3`,
		RunE: runSafeReset,
	}

	cmd.Flags().String("mode", "mixed", "Reset mode: soft, mixed, hard")
	return cmd
}

func runSafeReset(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	mode, _ := cmd.Flags().GetString("mode")
	if mode != "soft" && mode != "mixed" && mode != "hard" {
		return fmt.Errorf("invalid reset mode %q: must be soft, mixed, or hard", mode)
	}

	// Create snapshot first.
	snapshot, err := createAutoSnapshot(ctx, fmt.Sprintf("before-reset (%s)", mode))
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Snapshot created: %s\n", snapshot.ID)

	// Run git reset.
	repoPath, err := findRepoRoot()
	if err != nil {
		return err
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return err
	}

	target := "HEAD"
	if len(args) > 0 {
		target = args[0]
	}

	_, stderr, err := adapter.Run(ctx, "reset", "--"+mode, target)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Reset failed. Snapshot %s is available for recovery.\n", snapshot.ID)
		return fmt.Errorf("git reset: %w\n%s", err, stderr)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Reset (%s) to %s complete.\n", mode, target)
	fmt.Fprintf(cmd.OutOrStdout(), "To undo, restore the snapshot: got snapshot show %s\n", snapshot.ID)

	return nil
}

func newSafePushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push [remote] [branch]",
		Short: "Safe force push with automatic snapshot",
		Long: `Force push with an automatic snapshot for recovery.

Wraps 'git push --force-with-lease' but creates a snapshot first.

Examples:
  got safe push origin main
  got safe push origin feature-branch`,
		Args: cobra.RangeArgs(1, 2),
		RunE: runSafePush,
	}

	return cmd
}

func runSafePush(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	remote := args[0]
	branch := ""
	if len(args) > 1 {
		branch = args[1]
	}

	// Create snapshot first.
	snapshot, err := createAutoSnapshot(ctx, fmt.Sprintf("before-force-push (%s)", remote))
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Snapshot created: %s\n", snapshot.ID)

	repoPath, err := findRepoRoot()
	if err != nil {
		return err
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return err
	}

	// Get current branch if not specified.
	if branch == "" {
		branch, err = adapter.CurrentBranch(ctx)
		if err != nil {
			return fmt.Errorf("cannot determine current branch: %w", err)
		}
	}

	_, err = adapter.Push(ctx, remote, branch, true)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Push failed. Snapshot %s is available for recovery.\n", snapshot.ID)
		return fmt.Errorf("git push: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Force push to %s/%s complete.\n", remote, branch)
	return nil
}

func newSafeRebaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rebase [branch]",
		Short: "Safe rebase with automatic snapshot",
		Long: `Rebase the current branch onto another branch with an automatic
snapshot for recovery.

Wraps 'git rebase' but creates a snapshot first.

Examples:
  got safe rebase main
  got safe rebase origin/main`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSafeRebase(cmd, args[0])
		},
	}

	return cmd
}

func runSafeRebase(cmd *cobra.Command, onto string) error {
	ctx := context.Background()

	// Create snapshot first.
	snapshot, err := createAutoSnapshot(ctx, fmt.Sprintf("before-rebase (%s)", onto))
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Snapshot created: %s\n", snapshot.ID)

	repoPath, err := findRepoRoot()
	if err != nil {
		return err
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return err
	}

	_, stderr, err := adapter.Run(ctx, "rebase", onto)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Rebase failed. Snapshot %s is available for recovery.\n", snapshot.ID)
		fmt.Fprintf(cmd.ErrOrStderr(), "To abort: git rebase --abort\n")
		return fmt.Errorf("git rebase: %w\n%s", err, stderr)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Rebase onto %s complete.\n", onto)
	return nil
}
