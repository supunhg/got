// Copyright 2026 Supun Hewagamage. MIT License.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/supunhg/got/internal/git"
	"github.com/supunhg/got/internal/store"
)

// newSnapshotCmd returns the `got snapshot` command tree.
func newSnapshotCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Manage recovery snapshots",
		Long: `Manage recovery snapshots for safe Git operations.

Snapshots record the current state (branch, SHA) before destructive
operations like reset, rebase, or force push. They enable rollback
if something goes wrong.

Examples:
  got snapshot create --reason "before-reset"
  got snapshot list
  got snapshot show <id>`,
	}

	cmd.AddCommand(newSnapshotCreateCmd())
	cmd.AddCommand(newSnapshotListCmd())
	cmd.AddCommand(newSnapshotShowCmd())
	cmd.AddCommand(newSnapshotDeleteCmd())

	return cmd
}

func newSnapshotCreateCmd() *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a recovery snapshot",
		Long: `Record the current repository state as a recovery point.

Captures the current branch/SHA and stores it in the snapshots table.
This is used internally by 'got safe' commands but can also be used
manually.

Examples:
  got snapshot create --reason "before-reset"
  got snapshot create --reason "before-rebase"`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSnapshotCreate(cmd, reason)
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "manual", "Reason for the snapshot")
	return cmd
}

func runSnapshotCreate(cmd *cobra.Command, reason string) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}

	// Get current ref.
	branch, err := adapter.CurrentBranch(ctx)
	if err != nil {
		return fmt.Errorf("snapshot: cannot determine current branch: %w", err)
	}

	// Get current HEAD SHA.
	stdout, _, err := adapter.Run(ctx, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("snapshot: cannot get HEAD SHA: %w", err)
	}

	ref := fmt.Sprintf("refs/heads/%s@%s", branch, stdout[:min(len(stdout), 12)])

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()

	s, err := kc.ks.CreateSnapshot(ctx, store.CreateSnapshotParams{
		Reason: reason,
		Ref:    ref,
	})
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Snapshot created:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  ID:     %s\n", s.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Ref:    %s\n", s.Ref)
	fmt.Fprintf(cmd.OutOrStdout(), "  Reason: %s\n", s.Reason)

	return nil
}

func newSnapshotListCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recovery snapshots",
		Long: `List all recovery snapshots, most recent first.

Examples:
  got snapshot list
  got snapshot list --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSnapshotList(cmd, jsonOut)
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func runSnapshotList(cmd *cobra.Command, jsonOut bool) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()

	snapshots, err := kc.ks.ListSnapshots(ctx, 50)
	if err != nil {
		return fmt.Errorf("list snapshots: %w", err)
	}

	if jsonOut {
		if snapshots == nil {
			snapshots = []store.Snapshot{}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(snapshots)
	}

	if len(snapshots) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No snapshots found.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tDATE\tREASON\tREF")

	for _, s := range snapshots {
		date := time.UnixMilli(s.CreatedAt).Format("2006-01-02 15:04")
		shortID := s.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", shortID, date, s.Reason, s.Ref)
	}

	return w.Flush()
}

func newSnapshotShowCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show snapshot details",
		Long: `Display the full details of a recovery snapshot.

Examples:
  got snapshot show <id>
  got snapshot show <id> --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSnapshotShow(cmd, args[0], jsonOut)
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func runSnapshotShow(cmd *cobra.Command, id string, jsonOut bool) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()

	s, err := kc.ks.GetSnapshot(ctx, id)
	if err != nil {
		return fmt.Errorf("show snapshot: %w", err)
	}

	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(s)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Snapshot:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  ID:        %s\n", s.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Created:   %s\n", time.UnixMilli(s.CreatedAt).Format("2006-01-02 15:04:05 MST"))
	fmt.Fprintf(cmd.OutOrStdout(), "  Reason:    %s\n", s.Reason)
	fmt.Fprintf(cmd.OutOrStdout(), "  Ref:       %s\n", s.Ref)
	if s.ReflogSel != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Reflog:    %s\n", s.ReflogSel)
	}
	if s.StashRef != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Stash:     %s\n", s.StashRef)
	}
	if s.Metadata != "" && s.Metadata != "{}" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Metadata:  %s\n", s.Metadata)
	}

	return nil
}

func newSnapshotDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a snapshot",
		Long: `Remove a recovery snapshot from the database.

This does NOT affect the Git repository — it only removes the
snapshot record.

Examples:
  got snapshot delete <id>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSnapshotDelete(cmd, args[0])
		},
	}

	return cmd
}

func runSnapshotDelete(cmd *cobra.Command, id string) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()

	if err := kc.ks.DeleteSnapshot(ctx, id); err != nil {
		return fmt.Errorf("delete snapshot: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Snapshot %s deleted.\n", id)
	return nil
}
