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

func newRemoteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Manage Git remotes",
		Long: `Manage Git remotes — list, add, remove, push, and pull.

Without arguments, lists all configured remotes.

Subcommands:
  got remote                      List remotes
  got remote add <name> <url>     Add a remote
  got remote remove <name>        Remove a remote
  got remote push  <remote> <branch>  Push to a remote
  got remote pull  <remote> <branch>  Pull from a remote

Examples:
  got remote
  got remote add origin https://github.com/user/repo.git
  got remote remove origin
  got remote push origin main
  got remote pull origin main --ff-only
  got remote --json`,

		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemoteList(cmd, false)
		},
	}

	cmd.AddCommand(newRemoteAddCmd())
	cmd.AddCommand(newRemoteRemoveCmd())
	cmd.AddCommand(newRemotePushCmd())
	cmd.AddCommand(newRemotePullCmd())

	cmd.Flags().Bool("json", false, "Output as JSON")

	return cmd
}

func newRemoteAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name> <url>",
		Short: "Add a remote",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemoteAdd(cmd, args[0], args[1])
		},
	}
	return cmd
}

func newRemoteRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a remote",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemoteRemove(cmd, args[0])
		},
	}
	return cmd
}

func newRemotePushCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "push <remote> <branch>",
		Short: "Push to a remote",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemotePush(cmd, args[0], args[1], force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force push (with lease)")
	return cmd
}

func newRemotePullCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull <remote> <branch>",
		Short: "Pull from a remote",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemotePull(cmd, args[0], args[1])
		},
	}
	return cmd
}

func runRemoteList(cmd *cobra.Command, _ bool) error {
	jsonOut, _ := cmd.Flags().GetBool("json")

	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("remote: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("remote: %w", err)
	}

	remotes, err := adapter.GetRemotes(ctx)
	if err != nil {
		return fmt.Errorf("remote: %w", err)
	}

	if jsonOut {
		if remotes == nil {
			remotes = []git.Remote{}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(remotes)
	}

	if len(remotes) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No remotes configured.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tURL\tPUSH URL")

	for _, r := range remotes {
		pushURL := r.PushURL
		if pushURL == "" {
			pushURL = "(same)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, r.URL, pushURL)
	}

	return w.Flush()
}

func runRemoteAdd(cmd *cobra.Command, name, url string) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("remote: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("remote: %w", err)
	}

	if err := adapter.AddRemote(ctx, name, url); err != nil {
		return fmt.Errorf("remote: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Added remote %q → %s\n", name, url)
	return nil
}

func runRemoteRemove(cmd *cobra.Command, name string) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("remote: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("remote: %w", err)
	}

	if err := adapter.RemoveRemote(ctx, name); err != nil {
		return fmt.Errorf("remote: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed remote %q\n", name)
	return nil
}

func runRemotePush(cmd *cobra.Command, remote, branch string, force bool) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("remote: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("remote: %w", err)
	}

	result, err := adapter.Push(ctx, remote, branch, force)
	if err != nil {
		// Print partial output even on error.
		if result != nil && result.Output != "" {
			fmt.Fprintln(cmd.OutOrStdout(), result.Output)
		}
		return fmt.Errorf("remote: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), result.Output)
	return nil
}

func runRemotePull(cmd *cobra.Command, remote, branch string) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("remote: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("remote: %w", err)
	}

	result, err := adapter.Pull(ctx, remote, branch)
	if err != nil {
		if result != nil && result.Output != "" {
			fmt.Fprintln(cmd.OutOrStdout(), result.Output)
		}
		return fmt.Errorf("remote: %w", err)
	}

	mode := "merge"
	if result.FastForward {
		mode = "fast-forward"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Pulled (%s): %s\n", mode, result.Output)
	return nil
}
