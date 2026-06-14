package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/store"
)

// newWorkspaceCmd returns the `got workspace` command tree.
func newWorkspaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Manage logical workspaces",
		Long: `Manage logical workspaces that group related artifacts.

Workspaces are named contexts (like "oauth", "payment-refactor", or
"k8s-migration") that group files, branches, decisions, and notes
together. They are NOT tied to Git worktrees or branches — they are
organizational units for human intent.`,

		// Running `got workspace` without a subcommand shows the list.
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceList(cmd, false)
		},
	}

	cmd.AddCommand(newWorkspaceCreateCmd())
	cmd.AddCommand(newWorkspaceDeleteCmd())
	cmd.AddCommand(newWorkspaceListCmd())
	cmd.AddCommand(newWorkspaceShowCmd())
	cmd.AddCommand(newWorkspaceAddFileCmd())
	cmd.AddCommand(newWorkspaceAddBranchCmd())
	cmd.AddCommand(newWorkspaceAddNoteCmd())
	cmd.AddCommand(newWorkspaceAddDecisionCmd())
	cmd.AddCommand(newWorkspaceRemoveFileCmd())
	cmd.AddCommand(newWorkspaceRemoveBranchCmd())
	cmd.AddCommand(newWorkspaceStatusCmd())

	return cmd
}

// ---- Workspace create ------------------------------------------------------

func newWorkspaceCreateCmd() *cobra.Command {
	var description string
	var tags []string
	var noInteractive bool

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new workspace",
		Long: `Create a new logical workspace with a human-readable name.

Workspace names should be short, descriptive identifiers (e.g.
"oauth", "payment-refactor", "k8s-migration").

Examples:
  got workspace create oauth
  got workspace create payment-refactor --description "Payment system overhaul"
  got workspace create k8s-migration --tags "infra,ops,urgent" --no-interactive`,

		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceCreate(cmd, args[0], description, tags, noInteractive)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&description, "description", "", "Description of the workspace")
	flags.StringArrayVar(&tags, "tags", nil, "Tags (repeatable or comma-separated)")
	flags.BoolVar(&noInteractive, "no-interactive", false, "Use all flags, do not prompt")

	return cmd
}

func runWorkspaceCreate(cmd *cobra.Command, name, description string, tags []string, noInteractive bool) error {
	ctx := context.Background()

	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("workspace name is required")
	}

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	w, err := ks.CreateWorkspace(ctx, store.CreateWorkspaceParams{
		Name:        name,
		Description: description,
		Tags:        tags,
	})
	if err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Workspace created:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  ID:          %s\n", w.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Name:        %s\n", w.Name)
	if w.Description != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Description: %s\n", w.Description)
	}
	if len(w.Tags) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  Tags:        %s\n", strings.Join(w.Tags, ", "))
	}

	return nil
}

// ---- Workspace delete ------------------------------------------------------

func newWorkspaceDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a workspace",
		Long: `Delete a workspace and all its associations.

This removes the workspace record and all tracked files/branches.
Linked decisions and notes are NOT deleted — their workspace
association is cleared instead.

Examples:
  got workspace delete oauth
  got workspace delete payment-refactor`,

		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceDelete(cmd, args[0])
		},
	}

	return cmd
}

func runWorkspaceDelete(cmd *cobra.Command, name string) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	w, err := ks.DeleteWorkspace(ctx, name)
	if err != nil {
		return fmt.Errorf("delete workspace: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Workspace deleted:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  ID:   %s\n", w.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Name: %s\n", w.Name)

	return nil
}

// ---- Workspace list --------------------------------------------------------

func newWorkspaceListCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all workspaces",
		Long: `List all workspaces with their status, description, and tags.

Examples:
  got workspace list
  got workspace list --json`,

		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWorkspaceList(cmd, jsonOut)
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func runWorkspaceList(cmd *cobra.Command, jsonOut bool) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	workspaces, err := ks.ListWorkspaces(ctx)
	if err != nil {
		return fmt.Errorf("list workspaces: %w", err)
	}

	if jsonOut {
		if workspaces == nil {
			workspaces = []store.Workspace{}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(workspaces)
	}

	return outputWorkspacesTable(cmd, workspaces)
}

func outputWorkspacesTable(cmd *cobra.Command, workspaces []store.Workspace) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)

	if len(workspaces) == 0 {
		fmt.Fprintln(w, "No workspaces found.")
		return w.Flush()
	}

	fmt.Fprintln(w, "NAME\tSTATUS\tDESCRIPTION\tTAGS\tCREATED")

	for _, ws := range workspaces {
		date := time.UnixMilli(ws.CreatedAt).Format("2006-01-02")
		desc := ws.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		tags := strings.Join(ws.Tags, ", ")
		if len(tags) > 30 {
			tags = tags[:27] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", ws.Name, ws.Status, desc, tags, date)
	}

	return w.Flush()
}

// ---- Workspace show --------------------------------------------------------

func newWorkspaceShowCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show workspace details and contents",
		Long: `Display a workspace's metadata and all its linked items.

Shows the name, description, status, tags, tracked files, tracked
branches, linked decisions, and linked notes.

Examples:
  got workspace show oauth
  got workspace show payment-refactor --json`,

		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceShow(cmd, args[0], jsonOut)
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func runWorkspaceShow(cmd *cobra.Command, name string, jsonOut bool) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	status, err := ks.GetWorkspaceStatus(ctx, name)
	if err != nil {
		return fmt.Errorf("show workspace: %w", err)
	}

	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	return outputWorkspaceShow(cmd, status)
}

func outputWorkspaceShow(cmd *cobra.Command, s *store.WorkspaceStatus) error {
	ws := s.Workspace

	fmt.Fprintf(cmd.OutOrStdout(), "# %s\n\n", ws.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "**ID:** %s  |  **Status:** %s  |  **Created:** %s\n",
		ws.ID, ws.Status, time.UnixMilli(ws.CreatedAt).Format("2006-01-02"))

	if ws.Description != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n", ws.Description)
	}

	if len(ws.Tags) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\n**Tags:** %s\n", strings.Join(ws.Tags, ", "))
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\n**Items:** %d total\n", s.ItemCount)

	if len(s.Files) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\n## Tracked Files\n\n")
		for _, f := range s.Files {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", f.Path)
		}
	}

	if len(s.Branches) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\n## Tracked Branches\n\n")
		for _, b := range s.Branches {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", b.BranchName)
		}
	}

	if len(s.Decisions) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\n## Linked Decisions\n\n")
		for _, d := range s.Decisions {
			fmt.Fprintf(cmd.OutOrStdout(), "- **%s** (%s): %s\n", d.ID[:8]+"...", d.Status, d.Title)
		}
	}

	if len(s.Notes) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\n## Linked Notes\n\n")
		for _, n := range s.Notes {
			msg := truncate(n.Message, 60)
			fmt.Fprintf(cmd.OutOrStdout(), "- **%s**: %s\n", n.ID[:8]+"...", msg)
		}
	}

	return nil
}

// ---- Workspace add-file ----------------------------------------------------

func newWorkspaceAddFileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-file <workspace> <path>",
		Short: "Track a file path in a workspace",
		Long: `Add a file path to be tracked under a workspace.

The path is stored as a string reference only — no Git integration
yet. A future version will connect this to actual Git paths.

Examples:
  got workspace add-file oauth src/auth/oauth.go
  got workspace add-file payment-refactor internal/payment/`,

		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceAddFile(cmd, args[0], args[1])
		},
	}

	return cmd
}

func runWorkspaceAddFile(cmd *cobra.Command, workspaceName, filePath string) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	f, err := ks.AddWorkspaceFile(ctx, workspaceName, filePath)
	if err != nil {
		return fmt.Errorf("add file: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Added file %q to workspace %q\n", f.Path, workspaceName)
	return nil
}

// ---- Workspace add-branch --------------------------------------------------

func newWorkspaceAddBranchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-branch <workspace> <branch>",
		Short: "Track a branch name in a workspace",
		Long: `Add a branch name to be tracked under a workspace.

The branch is stored as a string reference only — no Git integration
yet. A future version will connect this to actual Git branches.

Examples:
  got workspace add-branch oauth feat/oauth2-refresh
  got workspace add-branch payment-refactor fix/payment-bug`,

		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceAddBranch(cmd, args[0], args[1])
		},
	}

	return cmd
}

func runWorkspaceAddBranch(cmd *cobra.Command, workspaceName, branchName string) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	b, err := ks.AddWorkspaceBranch(ctx, workspaceName, branchName)
	if err != nil {
		return fmt.Errorf("add branch: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Added branch %q to workspace %q\n", b.BranchName, workspaceName)
	return nil
}

// ---- Workspace add-note ----------------------------------------------------

func newWorkspaceAddNoteCmd() *cobra.Command {
	var branch, commitHash string

	cmd := &cobra.Command{
		Use:   "add-note <workspace> <message>",
		Short: "Create a note and link it to a workspace",
		Long: `Create a freeform knowledge note scoped to a workspace.

The note is created with the workspace association, making it
discoverable when viewing the workspace.

Examples:
  got workspace add-note oauth "Need to review the OAuth2 flow"
  got workspace add-note payment-refactor "Fixed rounding bug" --branch fix/rounding`,

		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceAddNote(cmd, args[0], args[1], branch, commitHash)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&branch, "branch", "", "Attach note to a branch")
	flags.StringVar(&commitHash, "commit", "", "Attach note to a commit")

	return cmd
}

func runWorkspaceAddNote(cmd *cobra.Command, workspaceName, message, branch, commitHash string) error {
	ctx := context.Background()

	message = strings.TrimSpace(message)
	if message == "" {
		return fmt.Errorf("note message cannot be empty")
	}

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	// Verify workspace exists by fetching it.
	_, err = ks.GetWorkspace(ctx, workspaceName)
	if err != nil {
		return fmt.Errorf("add note: %w", err)
	}

	n, err := ks.CreateNote(ctx, store.CreateNoteParams{
		Message:     message,
		WorkspaceID: &workspaceName,
		Branch:      branch,
		CommitHash:  commitHash,
	})
	if err != nil {
		return fmt.Errorf("add note: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Note added to workspace %q:\n", workspaceName)
	fmt.Fprintf(cmd.OutOrStdout(), "  ID:      %s\n", n.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Message: %s\n", truncate(n.Message, 60))

	return nil
}

// ---- Workspace add-decision -------------------------------------------------

func newWorkspaceAddDecisionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-decision <workspace> <decision-id>",
		Short: "Link an existing decision to a workspace",
		Long: `Associate an existing architectural decision with a workspace.

The decision's workspace_id is set to the workspace name, making it
appear when viewing the workspace's linked decisions.

Examples:
  got workspace add-decision oauth 01JQZ3ZABC
  got workspace add-decision payment-refactor 01JQZ4ZABC`,

		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceAddDecision(cmd, args[0], args[1])
		},
	}

	return cmd
}

func runWorkspaceAddDecision(cmd *cobra.Command, workspaceName, decisionID string) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	// Verify workspace exists.
	_, err = ks.GetWorkspace(ctx, workspaceName)
	if err != nil {
		return fmt.Errorf("add decision: %w", err)
	}

	// Verify decision exists by fetching it.
	d, err := ks.GetDecision(ctx, decisionID)
	if err != nil {
		return fmt.Errorf("add decision: %w", err)
	}

	// Update the decision's workspace_id.
	_, err = ks.UpdateDecision(ctx, decisionID, store.UpdateDecisionParams{
		WorkspaceID: &workspaceName,
	})
	if err != nil {
		return fmt.Errorf("add decision: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Linked decision %q to workspace %q\n", d.Title, workspaceName)
	return nil
}

// ---- Workspace remove-file -------------------------------------------------

func newWorkspaceRemoveFileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove-file <workspace> <path>",
		Short: "Remove a tracked file from a workspace",
		Long: `Remove a file path from a workspace's tracked files.

The file is not deleted from disk — it is only removed from
the workspace's file tracking list.

Examples:
  got workspace remove-file oauth src/auth/oauth.go`,

		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceRemoveFile(cmd, args[0], args[1])
		},
	}

	return cmd
}

func runWorkspaceRemoveFile(cmd *cobra.Command, workspaceName, filePath string) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	if err := ks.RemoveWorkspaceFile(ctx, workspaceName, filePath); err != nil {
		return fmt.Errorf("remove file: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed file %q from workspace %q\n", filePath, workspaceName)
	return nil
}

// ---- Workspace remove-branch ------------------------------------------------

func newWorkspaceRemoveBranchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove-branch <workspace> <branch>",
		Short: "Remove a tracked branch from a workspace",
		Long: `Remove a branch name from a workspace's tracked branches.

The branch is not deleted from Git — it is only removed from
the workspace's branch tracking list.

Examples:
  got workspace remove-branch oauth feat/oauth2-refresh`,

		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceRemoveBranch(cmd, args[0], args[1])
		},
	}

	return cmd
}

func runWorkspaceRemoveBranch(cmd *cobra.Command, workspaceName, branchName string) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	if err := ks.RemoveWorkspaceBranch(ctx, workspaceName, branchName); err != nil {
		return fmt.Errorf("remove branch: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed branch %q from workspace %q\n", branchName, workspaceName)
	return nil
}

// ---- Workspace status ------------------------------------------------------

func newWorkspaceStatusCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "status <name>",
		Short: "Show workspace summary with item counts and recent activity",
		Long: `Display a concise summary of a workspace's contents.

Shows the same information as 'got workspace show' but formatted
as a compact summary with item counts and the last activity timestamp.

Examples:
  got workspace status oauth
  got workspace status payment-refactor --json`,

		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceStatus(cmd, args[0], jsonOut)
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func runWorkspaceStatus(cmd *cobra.Command, name string, jsonOut bool) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	status, err := ks.GetWorkspaceStatus(ctx, name)
	if err != nil {
		return fmt.Errorf("workspace status: %w", err)
	}

	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Workspace: %s\n", status.Workspace.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "  Status:      %s\n", status.Workspace.Status)
	fmt.Fprintf(cmd.OutOrStdout(), "  Files:       %d\n", len(status.Files))
	fmt.Fprintf(cmd.OutOrStdout(), "  Branches:    %d\n", len(status.Branches))
	fmt.Fprintf(cmd.OutOrStdout(), "  Decisions:   %d\n", len(status.Decisions))
	fmt.Fprintf(cmd.OutOrStdout(), "  Notes:       %d\n", len(status.Notes))
	fmt.Fprintf(cmd.OutOrStdout(), "  Total items: %d\n", status.ItemCount)
	if status.LastActivity > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  Last active: %s\n", time.UnixMilli(status.LastActivity).Format("2006-01-02 15:04 MST"))
	}

	return nil
}
