// Copyright 2026 Supun Hewagamage. MIT License.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/supunhg/got/internal/git"
	"github.com/supunhg/got/internal/store"
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
	cmd.AddCommand(newWorkspaceSyncCmd())
	cmd.AddCommand(newWorkspaceDiffCmd())

	return cmd
}

// ---- Workspace create ------------------------------------------------------

func newWorkspaceCreateCmd() *cobra.Command {
	var description string
	var tags []string
	var noInteractive bool
	var createBranch bool

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new workspace",
		Long: `Create a new logical workspace with a human-readable name.

Workspace names should be short, descriptive identifiers (e.g.
"oauth", "payment-refactor", "k8s-migration").

Examples:
  got workspace create oauth
  got workspace create payment-refactor --description "Payment system overhaul"
  got workspace create k8s-migration --tags "infra,ops,urgent" --create-branch
  got workspace create oauth --no-interactive --description "OAuth 2.0"`,

		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceCreate(cmd, args[0], description, tags, noInteractive, createBranch)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&description, "description", "", "Description of the workspace")
	flags.StringArrayVar(&tags, "tags", nil, "Tags (repeatable or comma-separated)")
	flags.BoolVar(&noInteractive, "no-interactive", false, "Use all flags, do not prompt")
	flags.BoolVar(&createBranch, "create-branch", false, "Create a Git branch with the same name")

	return cmd
}

func runWorkspaceCreate(cmd *cobra.Command, name, description string, tags []string, noInteractive, createBranch bool) error {
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

	if createBranch {
		repoPath, repoErr := findRepoRoot()
		if repoErr == nil {
			adapter := git.NewExecAdapter(kc.bus)
			if openErr := adapter.OpenRepository(ctx, repoPath); openErr == nil {
				if branchErr := adapter.CreateBranch(ctx, name); branchErr == nil {
					fmt.Fprintf(cmd.OutOrStdout(), "  Branch:    %s (created)\n", name)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "  warning: could not create branch %q: %v\n", name, branchErr)
				}
			}
		} else {
			fmt.Fprintf(cmd.ErrOrStderr(), "  warning: not in a Git repository, cannot create branch: %v\n", repoErr)
		}
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

	fmt.Fprintln(w, "NAME\tSTATUS\tDESCRIPTION\tTAGS\tCREATED\tLAST COMMIT")

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
		lastSHA := ws.LastCommitSHA
		if len(lastSHA) > 8 {
			lastSHA = lastSHA[:8]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", ws.Name, ws.Status, desc, tags, date, lastSHA)
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
branches, linked decisions, and linked notes. When run inside a Git
repository, also shows live Git information for tracked branches
(ahead/behind, latest commit).

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

	if ws.LastCommitSHA != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "**Last commit:** %s\n", ws.LastCommitSHA)
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

		branchInfo := resolveBranchInfo(s.Branches)

		for _, b := range s.Branches {
			info := b.BranchName
			if bi, ok := branchInfo[b.BranchName]; ok {
				if bi.exists {
					info += fmt.Sprintf(" (exists, %s)", bi.status)
					if bi.aheadBehind != "" {
						info += fmt.Sprintf(" [%s]", bi.aheadBehind)
					}
					if bi.latestCommit != "" {
						info += fmt.Sprintf(" \u2014 %s", bi.latestCommit)
					}
				} else {
					info += " (not found in repository)"
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", info)
		}
	}

	if len(s.Decisions) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\n## Linked Decisions\n\n")
		for _, d := range s.Decisions {
			shortID := d.ID
			if len(shortID) > 8 {
				shortID = shortID[:8] + "..."
			}
			fmt.Fprintf(cmd.OutOrStdout(), "- **%s** (%s): %s\n", shortID, d.Status, d.Title)
		}
	}

	if len(s.Notes) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\n## Linked Notes\n\n")
		for _, n := range s.Notes {
			shortID := n.ID
			if len(shortID) > 8 {
				shortID = shortID[:8] + "..."
			}
			msg := truncate(n.Message, 60)
			fmt.Fprintf(cmd.OutOrStdout(), "- **%s**: %s\n", shortID, msg)
		}
	}

	if len(s.Commits) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\n## Recent Commits\n\n")
		for _, c := range s.Commits {
			sha := c.CommitSHA
			if len(sha) > 8 {
				sha = sha[:8]
			}
			msg := truncate(c.Message, 60)
			fmt.Fprintf(cmd.OutOrStdout(), "- **%s**", sha)
			if c.BranchName != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " on %s", c.BranchName)
			}
			fmt.Fprintf(cmd.OutOrStdout(), ": %s\n", msg)
		}
	}

	if len(s.PullRequests) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\n## Linked Pull Requests\n\n")
		for _, pr := range s.PullRequests {
			url := pr.URL
			stateInfo := pr.State
			if pr.MergeCommitSHA != "" && pr.State == "merged" {
				stateInfo = fmt.Sprintf("merged (%s)", pr.MergeCommitSHA[:8])
			}
			if url != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "- [#%d %s](%s) (%s)\n", pr.Number, pr.Title, url, stateInfo)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "- #%d %s (%s)\n", pr.Number, pr.Title, stateInfo)
			}
		}
	}

	if len(s.Issues) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\n## Linked Issues\n\n")
		for _, iss := range s.Issues {
			url := iss.URL
			if url != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "- [#%d %s](%s) (%s)\n", iss.Number, iss.Title, url, iss.State)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "- #%d %s (%s)\n", iss.Number, iss.Title, iss.State)
			}
		}
	}

	return nil
}

type branchInfo struct {
	exists       bool
	status       string
	aheadBehind  string
	latestCommit string
}

func resolveBranchInfo(branches []store.WorkspaceBranch) map[string]branchInfo {
	info := make(map[string]branchInfo)

	repoPath, err := findRepoRoot()
	if err != nil {
		return info
	}

	ctx := context.Background()
	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return info
	}

	realBranches, err := adapter.ListBranches(ctx)
	if err != nil {
		return info
	}

	realBranchSet := make(map[string]bool, len(realBranches))
	for _, b := range realBranches {
		realBranchSet[b.Name] = true
	}

	currentBranch, _ := adapter.CurrentBranch(ctx)

	for _, b := range branches {
		bi := branchInfo{}
		if !realBranchSet[b.BranchName] {
			bi.exists = false
			info[b.BranchName] = bi
			continue
		}
		bi.exists = true

		if b.BranchName == currentBranch {
			status, err := adapter.GetStatus(ctx)
			if err == nil && status != nil {
				if status.Clean {
					bi.status = "clean"
				} else {
					bi.status = "dirty"
				}
			}
		} else {
			bi.status = "clean"
		}

		if upstream, _, runErr := adapter.Run(ctx, "rev-list", "--left-right", "--count",
			b.BranchName+"..."+b.BranchName+"@{u}"); runErr == nil && upstream != "" {
			parts := strings.Fields(upstream)
			if len(parts) == 2 {
				bi.aheadBehind = fmt.Sprintf("ahead %s, behind %s", parts[0], parts[1])
			}
		}

		if log, _, logErr := adapter.Run(ctx, "log", "-1", "--format=%s", b.BranchName); logErr == nil && log != "" {
			bi.latestCommit = truncate(log, 50)
		}

		info[b.BranchName] = bi
	}

	return info
}

// ---- Workspace add-file ----------------------------------------------------

func newWorkspaceAddFileCmd() *cobra.Command {
	var noValidate bool

	cmd := &cobra.Command{
		Use:   "add-file <workspace> <path>",
		Short: "Track a file path in a workspace",
		Long: `Add a file path to be tracked under a workspace.

The file path is validated against the working tree — it must exist
on disk or be tracked by Git. Use --no-validate to skip validation.

Examples:
  got workspace add-file oauth src/auth/oauth.go
  got workspace add-file payment-refactor internal/payment/
  got workspace add-file oauth new/file.go --no-validate`,

		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceAddFile(cmd, args[0], args[1], noValidate)
		},
	}

	cmd.Flags().BoolVar(&noValidate, "no-validate", false, "Skip file existence validation")
	return cmd
}

func runWorkspaceAddFile(cmd *cobra.Command, workspaceName, filePath string, noValidate bool) error {
	ctx := context.Background()

	if !noValidate {
		if err := validateFilePath(filePath); err != nil {
			return fmt.Errorf("add file: %w", err)
		}
	}

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

func validateFilePath(filePath string) error {
	absPath, err := filepath.Abs(filePath)
	if err == nil {
		if _, statErr := os.Stat(absPath); statErr == nil {
			return nil
		}
	}

	repoPath, repoErr := findRepoRoot()
	if repoErr == nil {
		fullPath := filepath.Join(repoPath, filePath)
		if _, statErr := os.Stat(fullPath); statErr == nil {
			return nil
		}
	}

	if repoErr == nil {
		ctx := context.Background()
		adapter := git.NewExecAdapter(nil)
		if openErr := adapter.OpenRepository(ctx, repoPath); openErr == nil {
			if status, statusErr := adapter.GetStatus(ctx); statusErr == nil {
				for _, e := range status.Staged {
					if e.Path == filePath || strings.HasSuffix(e.Path, "/"+filePath) {
						return nil
					}
				}
				for _, e := range status.Unstaged {
					if e.Path == filePath || strings.HasSuffix(e.Path, "/"+filePath) {
						return nil
					}
				}
				for _, u := range status.Untracked {
					if u == filePath || strings.HasSuffix(u, "/"+filePath) {
						return nil
					}
				}
			}
		}
	}

	return fmt.Errorf("file %q does not exist on disk or in the Git working tree (use --no-validate to skip)", filePath)
}

func newWorkspaceAddBranchCmd() *cobra.Command {
	var noValidate bool

	cmd := &cobra.Command{
		Use:   "add-branch <workspace> <branch>",
		Short: "Track a branch name in a workspace",
		Long: `Add a branch name to be tracked under a workspace.

The branch name is validated against the Git repository — it must
exist as a local branch. Use --no-validate to skip validation.

Examples:
  got workspace add-branch oauth feat/oauth2-refresh
  got workspace add-branch payment-refactor main
  got workspace add-branch oauth new-branch --no-validate`,

		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceAddBranch(cmd, args[0], args[1], noValidate)
		},
	}

	cmd.Flags().BoolVar(&noValidate, "no-validate", false, "Skip branch existence validation")
	return cmd
}

func runWorkspaceAddBranch(cmd *cobra.Command, workspaceName, branchName string, noValidate bool) error {
	ctx := context.Background()

	if !noValidate {
		if err := validateBranchName(branchName); err != nil {
			return fmt.Errorf("add branch: %w", err)
		}
	}

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

func validateBranchName(branchName string) error {
	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("not in a Git repository: %w", err)
	}

	ctx := context.Background()
	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("open repository: %w", err)
	}

	branches, err := adapter.ListBranches(ctx)
	if err != nil {
		return fmt.Errorf("list branches: %w", err)
	}

	for _, b := range branches {
		if b.Name == branchName {
			return nil
		}
	}

	return fmt.Errorf("branch %q not found in repository (use --no-validate to skip)", branchName)
}

// ---- Workspace add-note/decision/remove-branch/file -------------------------

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

	_, err = ks.GetWorkspace(ctx, workspaceName)
	if err != nil {
		return fmt.Errorf("add decision: %w", err)
	}

	d, err := ks.GetDecision(ctx, decisionID)
	if err != nil {
		return fmt.Errorf("add decision: %w", err)
	}

	_, err = ks.UpdateDecision(ctx, decisionID, store.UpdateDecisionParams{
		WorkspaceID: &workspaceName,
	})
	if err != nil {
		return fmt.Errorf("add decision: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Linked decision %q to workspace %q\n", d.Title, workspaceName)
	return nil
}

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

	// Resolve live Git info for this workspace.
	gitBranch := ""
	gitClean := "unknown"
	if repoPath, repoErr := findRepoRoot(); repoErr == nil {
		adapter := git.NewExecAdapter(nil)
		if openErr := adapter.OpenRepository(ctx, repoPath); openErr == nil {
			if branch, cbErr := adapter.CurrentBranch(ctx); cbErr == nil {
				gitBranch = branch
			}
			if gitStatus, stErr := adapter.GetStatus(ctx); stErr == nil {
				if gitStatus.Clean {
					gitClean = "clean"
				} else {
					gitClean = "dirty"
				}
			}
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Workspace: %s\n", status.Workspace.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "  Status:      %s\n", status.Workspace.Status)
	if gitBranch != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Git branch:  %s (%s)\n", gitBranch, gitClean)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  Files:       %d\n", len(status.Files))
	fmt.Fprintf(cmd.OutOrStdout(), "  Branches:    %d\n", len(status.Branches))
	fmt.Fprintf(cmd.OutOrStdout(), "  Decisions:   %d\n", len(status.Decisions))
	fmt.Fprintf(cmd.OutOrStdout(), "  Notes:       %d\n", len(status.Notes))
	if len(status.Commits) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  Commits:     %d\n", len(status.Commits))
	}
	if len(status.PullRequests) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  Pull Reqs:   %d\n", len(status.PullRequests))
	}
	if len(status.Issues) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  Issues:      %d\n", len(status.Issues))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  Total items: %d\n", status.ItemCount)
	if status.Workspace.LastCommitSHA != "" {
		sha := status.Workspace.LastCommitSHA
		if len(sha) > 12 {
			sha = sha[:12]
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  Last commit: %s\n", sha)
	}
	if status.LastActivity > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  Last active: %s\n", time.UnixMilli(status.LastActivity).Format("2006-01-02 15:04 MST"))
	}

	return nil
}

// ---- Workspace sync --------------------------------------------------------

func newWorkspaceSyncCmd() *cobra.Command {
	var noCleanup bool

	cmd := &cobra.Command{
		Use:   "sync [name]",
		Short: "Refresh workspace view against Git state",
		Long: `Refresh a workspace's view of its files and branches by
comparing against the current Git repository state.

Automatically detects files and branches that no longer exist in the
repository and lists them for removal. Use --no-cleanup to skip the
cleanup offer and just report.

Examples:
  got workspace sync oauth
  got workspace sync payment-refactor --no-cleanup`,

		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceSync(cmd, args[0], noCleanup)
		},
	}

	cmd.Flags().BoolVar(&noCleanup, "no-cleanup", false, "Report stale items without removing them")
	return cmd
}

func runWorkspaceSync(cmd *cobra.Command, name string, noCleanup bool) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	status, err := ks.GetWorkspaceStatus(ctx, name)
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	repoPath, repoErr := findRepoRoot()
	if repoErr != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Not in a Git repository \u2014 cannot validate files/branches.\n")
		return nil
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("sync: open repo: %w", err)
	}

	realBranches, err := adapter.ListBranches(ctx)
	realBranchSet := make(map[string]bool)
	if err == nil {
		for _, b := range realBranches {
			realBranchSet[b.Name] = true
		}
	}

	var staleFiles []string
	for _, f := range status.Files {
		fullPath := filepath.Join(repoPath, f.Path)
		if _, statErr := os.Stat(fullPath); os.IsNotExist(statErr) {
			staleFiles = append(staleFiles, f.Path)
		}
	}

	var staleBranches []string
	for _, b := range status.Branches {
		if !realBranchSet[b.BranchName] {
			staleBranches = append(staleBranches, b.BranchName)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Workspace %q sync results:\n", name)
	fmt.Fprintf(cmd.OutOrStdout(), "  Files:        %d tracked, %d stale\n", len(status.Files), len(staleFiles))
	fmt.Fprintf(cmd.OutOrStdout(), "  Branches:     %d tracked, %d stale\n", len(status.Branches), len(staleBranches))

	if len(staleFiles) == 0 && len(staleBranches) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  Everything is up to date.\n")
		return nil
	}

	if len(staleFiles) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nStale files (no longer on disk):\n")
		for _, f := range staleFiles {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", f)
		}
	}

	if len(staleBranches) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nStale branches (no longer in repository):\n")
		for _, b := range staleBranches {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", b)
		}
	}

	if !noCleanup {
		for _, f := range staleFiles {
			if removeErr := ks.RemoveWorkspaceFile(ctx, name, f); removeErr == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "  Removed stale file: %s\n", f)
			}
		}
		for _, b := range staleBranches {
			if removeErr := ks.RemoveWorkspaceBranch(ctx, name, b); removeErr == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "  Removed stale branch: %s\n", b)
			}
		}
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "\nRun without --no-cleanup to remove stale items.\n")
	}

	return nil
}
