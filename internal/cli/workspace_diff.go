// Copyright 2026 The GOT Authors. MIT License.
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/supunhg/got/internal/git"
)

// newWorkspaceDiffCmd returns the `got workspace diff` subcommand.
func newWorkspaceDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <name>",
		Short: "Show diff of tracked files vs HEAD",
		Long: `Show the diff of tracked workspace files against HEAD.

Displays changes in files tracked by the workspace. Useful for
reviewing what's changed in the files relevant to a specific
piece of work.

Examples:
  got workspace diff oauth
  got workspace diff payment-refactor`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceDiff(cmd, args[0])
		},
	}

	return cmd
}

func runWorkspaceDiff(cmd *cobra.Command, name string) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()

	status, err := kc.ks.GetWorkspaceStatus(ctx, name)
	if err != nil {
		return fmt.Errorf("workspace diff: %w", err)
	}

	if len(status.Files) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Workspace %q has no tracked files.\n", name)
		return nil
	}

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("workspace diff: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("workspace diff: %w", err)
	}

	// Build list of tracked file paths that exist on disk.
	var existingFiles []string
	for _, f := range status.Files {
		fullPath := filepath.Join(repoPath, f.Path)
		if _, statErr := os.Stat(fullPath); statErr == nil {
			existingFiles = append(existingFiles, f.Path)
		}
	}

	if len(existingFiles) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Workspace %q: no tracked files found on disk.\n", name)
		return nil
	}

	// Run git diff for the tracked files.
	args := append([]string{"diff", "HEAD", "--"}, existingFiles...)
	stdout, stderr, err := adapter.Run(ctx, args...)
	if err != nil {
		return fmt.Errorf("workspace diff: %w\n%s", err, stderr)
	}

	if stdout == "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Workspace %q: no changes in tracked files.\n", name)
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Workspace %q — diff of tracked files vs HEAD:\n\n", name)
	fmt.Fprint(cmd.OutOrStdout(), stdout)

	return nil
}
