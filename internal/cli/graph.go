// Copyright 2026 The GOT Authors. MIT License.
package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/supunhg/got/internal/git"
)

func newGraphCmd() *cobra.Command {
	var branch string
	var maxCount int

	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Display a text-based commit graph",
		Long: `Display a text-based commit graph (like git log --graph).

Shows parent-child relationships between commits with branch/tag
decorations.

Examples:
  got graph
  got graph --branch main
  got graph --max-count 10`,

		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runGraph(cmd, branch, maxCount)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&branch, "branch", "b", "", "Branch to show graph for")
	flags.IntVarP(&maxCount, "max-count", "n", 20, "Maximum number of commits to show")

	return cmd
}

func runGraph(cmd *cobra.Command, branch string, maxCount int) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("graph: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("graph: %w", err)
	}

	nodes, err := adapter.GetGraph(ctx, branch, maxCount)
	if err != nil {
		return fmt.Errorf("graph: %w", err)
	}

	if len(nodes) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No commits found.")
		return nil
	}

	// Build a simple text-based graph.
	// For each node, we show:
	//   * SHA (first 8 chars) Message
	//   | Branch/tag decorations
	//   * (next commit)
	//
	// Simple indentation approach — each level of history adds an indent.

	for i, node := range nodes {
		prefix := "* "
		if i > 0 {
			// Check if this is a parent of the previous node.
			prefix = "* "
		}

		shortSHA := node.SHA
		if len(shortSHA) > 8 {
			shortSHA = shortSHA[:8]
		}

		line := fmt.Sprintf("%s%s %s", prefix, shortSHA, node.Message)

		if node.Refs != "" {
			line += fmt.Sprintf(" (%s)", node.Refs)
		}

		fmt.Fprintln(cmd.OutOrStdout(), line)

		// Show parent relationships if there are multiple parents (merge).
		if len(node.Parents) > 1 {
			var parentShorts []string
			for _, p := range node.Parents {
				ps := p
				if len(ps) > 8 {
					ps = ps[:8]
				}
				parentShorts = append(parentShorts, ps)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  Merge: %s\n", strings.Join(parentShorts, " "))
		}
	}

	return nil
}
