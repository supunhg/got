package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/git"
)

func newCommitCmd() *cobra.Command {
	var message string
	var allowEmpty bool
	var all bool

	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Create a commit with staged changes",
		Long: `Create a commit with the given message.

Stages all tracked files (git add -A) before committing unless
changes are already staged. Use -m to supply a message directly.

Examples:
  got commit -m "feat: add user authentication"
  got commit -m "fix: resolve null pointer in login" --all`,

		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCommit(cmd, message, allowEmpty, all)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&message, "message", "m", "", "Commit message")
	flags.BoolVar(&allowEmpty, "allow-empty", false, "Allow empty commit")
	flags.BoolVarP(&all, "all", "a", false, "Stage all tracked files first")

	return cmd
}

func runCommit(cmd *cobra.Command, message string, allowEmpty, all bool) error {
	ctx := context.Background()

	message = strings.TrimSpace(message)
	if message == "" {
		return fmt.Errorf("commit message is required (use -m \"<message>\")")
	}

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Stage all if --all is set.
	if all {
		if _, _, err := adapter.Run(ctx, "add", "-A"); err != nil {
			return fmt.Errorf("commit: stage all: %w", err)
		}
	}

	// Use CreateCommit which handles events and SHA retrieval.
	// For --allow-empty, use raw Run since CreateCommit doesn't support that flag.
	var sha string

	if allowEmpty {
		_, stderr, err := adapter.Run(ctx, "commit", "-m", message, "--allow-empty")
		if err != nil {
			return fmt.Errorf("commit: %w\n%s", err, stderr)
		}
		sha, _, _ = adapter.Run(ctx, "rev-parse", "HEAD")
	} else {
		var err error
		sha, err = adapter.CreateCommit(ctx, message, "")
		if err != nil {
			return fmt.Errorf("commit: %w", err)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Committed: %s\n", truncate(message, 60))
	fmt.Fprintf(cmd.OutOrStdout(), "  SHA: %s\n", sha)

	return nil
}
