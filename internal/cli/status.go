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

func newStatusCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show working tree status",
		Long: `Show the working tree status, similar to 'git status'.

Displays the current branch, staged changes, unstaged changes,
and untracked files in a readable format.

Examples:
  got status
  got status --json`,

		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStatus(cmd, jsonOut)
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func runStatus(cmd *cobra.Command, jsonOut bool) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("status: %w", err)
	}

	status, err := adapter.GetStatus(ctx)
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}

	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "On branch %s\n", status.Branch)
	if status.Clean {
		fmt.Fprintln(w, "\n  nothing to commit, working tree clean")
		return w.Flush()
	}
	fmt.Fprintln(w)

	// Staged.
	if len(status.Staged) > 0 {
		fmt.Fprintln(w, "Changes staged:")
		fmt.Fprintln(w, "  STATUS\tFILE")
		for _, e := range status.Staged {
			label := statusLabel(e.IndexStatus)
			fmt.Fprintf(w, "  %s\t%s\n", label, e.Path)
		}
		fmt.Fprintln(w)
	}

	// Unstaged.
	if len(status.Unstaged) > 0 {
		fmt.Fprintln(w, "Changes not staged:")
		fmt.Fprintln(w, "  STATUS\tFILE")
		for _, e := range status.Unstaged {
			label := statusLabel(e.WorktreeStatus)
			fmt.Fprintf(w, "  %s\t%s\n", label, e.Path)
		}
		fmt.Fprintln(w)
	}

	// Untracked.
	if len(status.Untracked) > 0 {
		fmt.Fprintln(w, "Untracked files:")
		for _, p := range status.Untracked {
			fmt.Fprintf(w, "  %s\n", p)
		}
		fmt.Fprintln(w)
	}

	return w.Flush()
}

func statusLabel(s string) string {
	switch s {
	case "M":
		return "modified"
	case "A":
		return "added"
	case "D":
		return "deleted"
	case "R":
		return "renamed"
	case "C":
		return "copied"
	case "?":
		return "untracked"
	default:
		return s
	}
}

// findRepoRoot uses the Git adapter to find the repository root.
// Falls back to walking up for .git/ if the adapter approach doesn't work.
func findRepoRoot() (string, error) {
	// Use findGitDir from init.go logic — walk up for .git/
	return findGitDir(".")
}
