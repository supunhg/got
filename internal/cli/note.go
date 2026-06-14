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

// newNoteCmd returns the `got note` command tree.
func newNoteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "note",
		Short: "Manage freeform knowledge notes",
		Long: `Add, list, and view freeform notes linked to workspaces, branches, or commits.

Notes are lightweight Markdown snippets (< 4 KB) for capturing observations,
links, and rationale that don't warrant a full decision record.`,
	}

	cmd.AddCommand(newNoteAddCmd())
	cmd.AddCommand(newNoteListCmd())
	cmd.AddCommand(newNoteShowCmd())
	cmd.AddCommand(newNoteDeleteCmd())

	return cmd
}

// ── Note add ────────────────────────────────────────────────────────

func newNoteAddCmd() *cobra.Command {
	var workspace, branch, commitHash string

	cmd := &cobra.Command{
		Use:   "add <message>",
		Short: "Add a new note",
		Long: `Add a freeform note with an inline message.

The message is a single argument — use quotes for multi-line content.

Examples:
  got note add "Investigated SQLite WAL mode"
  got note add "Need to revisit the Bubbletea viewport" --workspace tui
  got note add "Fixed race condition" --branch fix/race --commit abc1234`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNoteAdd(cmd, args[0], workspace, branch, commitHash)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&workspace, "workspace", "", "Scope note to a workspace")
	flags.StringVar(&branch, "branch", "", "Attach note to a branch")
	flags.StringVar(&commitHash, "commit", "", "Attach note to a commit")

	return cmd
}

func runNoteAdd(cmd *cobra.Command, message, workspace, branch, commitHash string) error {
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

	var workspaceID *string
	if workspace != "" {
		workspaceID = &workspace
	}

	n, err := ks.CreateNote(ctx, store.CreateNoteParams{
		Message:     message,
		WorkspaceID: workspaceID,
		Branch:      branch,
		CommitHash:  commitHash,
	})
	if err != nil {
		return fmt.Errorf("add note: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Note added:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  ID:      %s\n", n.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Message: %s\n", truncate(n.Message, 60))
	if branch != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Branch:  %s\n", branch)
	}
	if commitHash != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Commit:  %s\n", commitHash)
	}

	return nil
}

// ── Note list ───────────────────────────────────────────────────────

func newNoteListCmd() *cobra.Command {
	var workspace string
	var limit int
	var all, jsonOut bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List notes, filterable",
		Long: `List notes with optional filters.

By default shows the 20 most recent notes. Use --all to show all,
or --limit to set a custom limit.

Examples:
  got note list
  got note list --workspace engine
  got note list --all
  got note list --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runNoteList(cmd, workspace, limit, all, jsonOut)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&workspace, "workspace", "", "Filter by workspace name")
	flags.IntVar(&limit, "limit", 0, "Max results (default 20, 0=default)")
	flags.BoolVar(&all, "all", false, "Show all notes (no limit)")
	flags.BoolVar(&jsonOut, "json", false, "Output as JSON")

	return cmd
}

func runNoteList(cmd *cobra.Command, workspace string, limit int, all, jsonOut bool) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	filter := store.NoteFilter{
		Limit: limit,
		All:   all,
	}
	if workspace != "" {
		filter.WorkspaceID = &workspace
	}

	notes, err := ks.ListNotes(ctx, filter)
	if err != nil {
		return fmt.Errorf("list notes: %w", err)
	}

	if jsonOut {
		if notes == nil {
			notes = []store.Note{}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(notes)
	}

	return outputNotesTable(cmd, notes)
}

func outputNotesTable(cmd *cobra.Command, notes []store.Note) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)

	if len(notes) == 0 {
		fmt.Fprintln(w, "No notes found.")
		return w.Flush()
	}

	fmt.Fprintln(w, "ID\tMESSAGE\tWORKSPACE\tBRANCH\tCREATED")

	for _, n := range notes {
		ws := ""
		if n.WorkspaceID != nil {
			ws = *n.WorkspaceID
		}
		date := time.UnixMilli(n.CreatedAt).Format("2006-01-02")

		id := truncate(n.ID, 18)
		msg := truncate(n.Message, 50)
		branch := truncate(n.Branch, 20)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", id, msg, ws, branch, date)
	}

	return w.Flush()
}

// ── Note delete ─────────────────────────────────────────────────────

func newNoteDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Permanently delete a note",
		Long: `Delete a freeform knowledge note from the database.

This permanently removes the note. This action cannot be undone.

Examples:
  got note delete 01JQZ3ZABC`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNoteDelete(cmd, args[0])
		},
	}

	return cmd
}

func runNoteDelete(cmd *cobra.Command, id string) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	// Fetch first so we can show what's being deleted.
	n, err := ks.GetNote(ctx, id)
	if err != nil {
		return fmt.Errorf("delete note: %w", err)
	}

	if err := ks.DeleteNote(ctx, id); err != nil {
		return fmt.Errorf("delete note: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Note deleted:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  ID:      %s\n", n.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Message: %s\n", truncate(n.Message, 60))

	return nil
}

// ── Note show ───────────────────────────────────────────────────────

func newNoteShowCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a note in full",
		Long: `Display the full contents of a freeform knowledge note.

Shows the message, workspace, branch, commit, and timestamps.

Examples:
  got note show 01JQZ3ZABC
  got note show 01JQZ3ZABC --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNoteShow(cmd, args[0], jsonOut)
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func runNoteShow(cmd *cobra.Command, id string, jsonOut bool) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	n, err := ks.GetNote(ctx, id)
	if err != nil {
		return fmt.Errorf("show note: %w", err)
	}

	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(n)
	}

	// ── Terminal output ─────────────────────────────────────────
	fmt.Fprintf(cmd.OutOrStdout(), "# Note %s\n\n", n.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "**Created:** %s\n", time.UnixMilli(n.CreatedAt).Format("2006-01-02 15:04 MST"))
	if n.UpdatedAt != n.CreatedAt {
		fmt.Fprintf(cmd.OutOrStdout(), "**Updated:** %s\n", time.UnixMilli(n.UpdatedAt).Format("2006-01-02 15:04 MST"))
	}
	if n.WorkspaceID != nil && *n.WorkspaceID != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "**Workspace:** %s\n", *n.WorkspaceID)
	}
	if n.Branch != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "**Branch:** %s\n", n.Branch)
	}
	if n.CommitHash != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "**Commit:** %s\n", n.CommitHash)
	}

	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), n.Message)

	fmt.Fprintf(cmd.OutOrStdout(), "\n---\nID: %s\n", n.ID)

	return nil
}

// truncate shortens s to at most n runes, appending "..." if truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-3]) + "..."
}
