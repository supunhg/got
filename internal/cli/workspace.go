package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/repo"
	"github.com/got-sh/got/internal/workspace"
)

// newWorkspaceCmd builds the `got workspace` subcommand tree
// (v0.4 Workspace Engine, see docs/WORKSPACE_ENGINE.md).
//
//	got workspace                                       list (default RunE)
//	got workspace list [--all|--open|--archived] [--json]
//	got workspace create <name> [--title T] [--description D] [--color C] [--metadata JSON]
//	got workspace update <name> [--title T] [--description D] [--color C] [--state open|archived] [--metadata JSON]
//	got workspace delete <name>
//	got workspace show <name> [--json]
//	got workspace add-file <name> <path> [--note N]
//	got workspace add-branch <name> <branch>
//	got workspace add-note <name> [--body B|-] [--stdin] [--pinned]
//	got workspace add-decision <name> <title> [--body B|-] [--stdin] [--status proposed|accepted|rejected|superseded]
//
// The list subcommand is duplicated at the parent level (the
// default RunE prints the same table) so `got workspace` with no
// args still works and `got workspace list` is the explicit
// form, matching the `got worktree` pattern.
func newWorkspaceCmd(d Deps) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Manage logical workspaces (groups of files, branches, decisions, notes)",
		Long: `Manage logical workspaces — a v0.4 GOT feature that groups related
files, branches, decisions, and notes under a single named aggregate.

A workspace is independent of Git worktrees and Git branches: it
lives entirely in .got/got.db and does not move files around on
disk. The same workspace can reference any number of files and
any number of branches; deleting a workspace leaves the branches
alone and removes only the workspace's records.

Examples:
  got workspace create oauth --title "OAuth Refactor"
  got workspace add-file oauth internal/auth/oauth.go
  got workspace add-branch oauth feature/oauth-flow
  got workspace add-note oauth "PKCE flow implemented"
  got workspace add-decision oauth "Use PKCE" --body "..."
  got workspace show oauth
  got workspace update oauth --state archived
  got workspace delete oauth`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWorkspaceList(cmd.Context(), cmd, d, asJSON, workspace.StateOpen)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable JSON output")
	cmd.AddCommand(newWorkspaceListCmd(d))
	cmd.AddCommand(newWorkspaceCreateCmd(d))
	cmd.AddCommand(newWorkspaceUpdateCmd(d))
	cmd.AddCommand(newWorkspaceDeleteCmd(d))
	cmd.AddCommand(newWorkspaceShowCmd(d))
	cmd.AddCommand(newWorkspaceAddFileCmd(d))
	cmd.AddCommand(newWorkspaceAddBranchCmd(d))
	cmd.AddCommand(newWorkspaceAddNoteCmd(d))
	cmd.AddCommand(newWorkspaceAddDecisionCmd(d))
	return cmd
}

// ---------------------------------------------------------------------------
// list
// ---------------------------------------------------------------------------

func newWorkspaceListCmd(d Deps) *cobra.Command {
	var (
		asJSON   bool
		showAll  bool
		showOpen bool
		showArch bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workspaces",
		Long: `List workspaces. By default, only open workspaces are shown.

Pass --all to also show archived workspaces, --archived for
archived only, or --open (the default) for open only. The three
flags are mutually exclusive; the last one wins.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			state := workspace.StateOpen
			switch {
			case showAll:
				state = ""
			case showArch:
				state = workspace.StateArchived
			case showOpen:
				state = workspace.StateOpen
			}
			return runWorkspaceList(cmd.Context(), cmd, d, asJSON, state)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable JSON output")
	cmd.Flags().BoolVar(&showAll, "all", false, "show all workspaces regardless of state")
	cmd.Flags().BoolVar(&showOpen, "open", false, "show only open workspaces (the default)")
	cmd.Flags().BoolVar(&showArch, "archived", false, "show only archived workspaces")
	cmd.MarkFlagsMutuallyExclusive("all", "open", "archived")
	return cmd
}

func runWorkspaceList(ctx context.Context, cmd *cobra.Command, d Deps, asJSON bool, state workspace.State) error {
	logger := loggerFor(d)
	logger.Info("workspace list starting", "state", state, "json", asJSON)
	st, _, err := openWorkspaceStore(ctx, d)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	opts := workspace.ListOptions{}
	if state != "" {
		opts.State = state
	}
	all, err := st.List(ctx, opts)
	if err != nil {
		return err
	}
	logger.Info("workspace list finished", "count", len(all))
	out := cmdWriter(cmd, d)
	if asJSON {
		return writeJSON(out, all)
	}
	// Fetch per-workspace counts so the table is useful at a
	// glance. We do this in one batched query per child table
	// (4 queries total for the whole list, not per-row) and
	// stitch them into the rows. This is cheap because v0.4
	// workspaces are small (single-digit N per repo).
	counts, err := st.CountsByWorkspace(ctx, all)
	if err != nil {
		return err
	}
	return writeWorkspaceTable(out, all, counts)
}

// writeWorkspaceTable renders workspaces as a tab-separated
// table. The "FILES / BRANCHES / DECISIONS / NOTES" columns are
// the per-workspace child counts (joined in from a single batch
// query by runWorkspaceList). The "UPDATED" column is the
// workspace's own updated_at, which is the time of the most
// recent create / update / add-* / remove-* operation against
// the workspace row itself.
func writeWorkspaceTable(w io.Writer, ws []*workspace.Workspace, counts workspace.CountsByWorkspace) error {
	if len(ws) == 0 {
		_, err := fmt.Fprintln(w, "(no workspaces)")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tTITLE\tSTATE\tFILES\tBRANCHES\tDECISIONS\tNOTES\tUPDATED")
	for _, w := range ws {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\t%d\t%s\n",
			w.Name, w.Title, w.State,
			counts.Files[w.ID], counts.Branches[w.ID], counts.Decisions[w.ID], counts.Notes[w.ID],
			w.UpdatedAt.Format("2006-01-02 15:04"))
	}
	return tw.Flush()
}

// ---------------------------------------------------------------------------
// create
// ---------------------------------------------------------------------------

func newWorkspaceCreateCmd(d Deps) *cobra.Command {
	var (
		title       string
		description string
		color       string
		metadataRaw string
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new workspace",
		Long: `Create a new workspace.

The name is a unique slug: 1-63 lowercase characters, starting
with a letter, then letters / digits / hyphens / underscores
(regex ^[a-z][a-z0-9_-]{0,62}$). The title is required; the
description and color are optional. State defaults to "open".

Use --metadata to pass a JSON object (key/value) that plugins
can read via ` + "`got workspace show --json`" + `. The value is
stored verbatim in the workspaces.metadata column.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceCreate(cmd.Context(), cmd, d, args[0], title, description, color, metadataRaw)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "human-readable title (required)")
	cmd.Flags().StringVar(&description, "description", "", "one-line description")
	cmd.Flags().StringVar(&color, "color", "", "optional label color (hex, e.g. #3b82f6)")
	cmd.Flags().StringVar(&metadataRaw, "metadata", "", "JSON object stored in the workspace's metadata column")
	return cmd
}

func runWorkspaceCreate(ctx context.Context, cmd *cobra.Command, d Deps, name, title, description, color, metadataRaw string) error {
	logger := loggerFor(d)
	logger.Info("workspace create starting", "name", name, "title", title)
	st, _, err := openWorkspaceStore(ctx, d)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	w := &workspace.Workspace{
		Name:        name,
		Title:       title,
		Description: description,
		Color:       color,
	}
	if metadataRaw != "" {
		var meta map[string]any
		if err := json.Unmarshal([]byte(metadataRaw), &meta); err != nil {
			return gerr.Validation(fmt.Sprintf("invalid --metadata JSON: %v", err))
		}
		w.Metadata = meta
	}
	if err := st.Create(ctx, w); err != nil {
		return err
	}
	logger.Info("workspace create finished", "id", w.ID, "name", w.Name)
	publishWorkspaceEvent(ctx, d, topicWorkspaceCreated, w)
	out := cmdWriter(cmd, d)
	_, _ = fmt.Fprintf(out, "[got] created workspace %q (%s)\n", w.Name, w.ID)
	return nil
}

// ---------------------------------------------------------------------------
// update
// ---------------------------------------------------------------------------

func newWorkspaceUpdateCmd(d Deps) *cobra.Command {
	var (
		title       string
		description string
		color       string
		state       string
		metadataRaw string
		clearMeta   bool
	)
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update a workspace's title, description, color, state, or metadata",
		Long: `Update a workspace by name or ID. Empty flags are ignored; only the
fields you set are changed. To clear --metadata, pass --clear-metadata.

Examples:
  got workspace update oauth --title "OAuth 2.0 (PKCE)"
  got workspace update oauth --state archived
  got workspace update oauth --metadata '{"reviewers":["alice","bob"]}'
  got workspace update oauth --clear-metadata`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceUpdate(cmd.Context(), cmd, d, args[0], title, description, color, state, metadataRaw, clearMeta)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "new human-readable title")
	cmd.Flags().StringVar(&description, "description", "", "new one-line description")
	cmd.Flags().StringVar(&color, "color", "", "new label color (hex)")
	cmd.Flags().StringVar(&state, "state", "", "new state: open or archived")
	cmd.Flags().StringVar(&metadataRaw, "metadata", "", "new JSON metadata object")
	cmd.Flags().BoolVar(&clearMeta, "clear-metadata", false, "clear the metadata column (overrides --metadata)")
	return cmd
}

func runWorkspaceUpdate(ctx context.Context, cmd *cobra.Command, d Deps, name, title, description, color, state, metadataRaw string, clearMeta bool) error {
	logger := loggerFor(d)
	logger.Info("workspace update starting", "name", name, "state", state)
	st, _, err := openWorkspaceStore(ctx, d)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	w, err := st.Get(ctx, name)
	if err != nil {
		return err
	}
	// Empty strings mean "leave alone". The store's Update uses
	// COALESCE(NULLIF(?, ''), column) so empty inputs are
	// already no-ops; we only set the fields the user asked
	// for.
	if title != "" {
		w.Title = title
	}
	if description != "" {
		w.Description = description
	}
	if color != "" {
		w.Color = color
	}
	if state != "" {
		w.State = workspace.State(state)
	}
	if clearMeta {
		w.Metadata = nil
	} else if metadataRaw != "" {
		var meta map[string]any
		if err := json.Unmarshal([]byte(metadataRaw), &meta); err != nil {
			return gerr.Validation(fmt.Sprintf("invalid --metadata JSON: %v", err))
		}
		w.Metadata = meta
	}
	if err := st.Update(ctx, w); err != nil {
		return err
	}
	logger.Info("workspace update finished", "name", w.Name, "state", w.State)
	publishWorkspaceEvent(ctx, d, topicWorkspaceUpdated, w)
	out := cmdWriter(cmd, d)
	_, _ = fmt.Fprintf(out, "[got] updated workspace %q\n", w.Name)
	return nil
}

// ---------------------------------------------------------------------------
// delete
// ---------------------------------------------------------------------------

func newWorkspaceDeleteCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a workspace (cascades to its files, branches, decisions, notes)",
		Long: `Delete a workspace by name or ID.

ON DELETE CASCADE removes every file, branch, decision, and note
attached to the workspace in the same transaction. The
underlying Git branches are NOT deleted; only the workspace's
records of them are.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceDelete(cmd.Context(), cmd, d, args[0])
		},
	}
	return cmd
}

func runWorkspaceDelete(ctx context.Context, cmd *cobra.Command, d Deps, name string) error {
	logger := loggerFor(d)
	logger.Info("workspace delete starting", "name", name)
	st, _, err := openWorkspaceStore(ctx, d)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	if err := st.Delete(ctx, name); err != nil {
		return err
	}
	logger.Info("workspace delete finished", "name", name)
	out := cmdWriter(cmd, d)
	_, _ = fmt.Fprintf(out, "[got] deleted workspace %q\n", name)
	return nil
}

// ---------------------------------------------------------------------------
// show
// ---------------------------------------------------------------------------

func newWorkspaceShowCmd(d Deps) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show a workspace and all of its attached files, branches, decisions, notes",
		Long: `Show a workspace and all of its child records.

The human-readable form (no flag) renders the workspace summary
followed by a section per child table (Files, Branches,
Decisions, Notes). Pass --json for a single machine-readable
object containing every field.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceShow(cmd.Context(), cmd, d, args[0], asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable JSON output")
	return cmd
}

func runWorkspaceShow(ctx context.Context, cmd *cobra.Command, d Deps, name string, asJSON bool) error {
	logger := loggerFor(d)
	logger.Info("workspace show starting", "name", name, "json", asJSON)
	st, _, err := openWorkspaceStore(ctx, d)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	view, err := st.Show(ctx, name)
	if err != nil {
		return err
	}
	logger.Info("workspace show finished",
		"name", name,
		"files", len(view.Files),
		"branches", len(view.Branches),
		"decisions", len(view.Decisions),
		"notes", len(view.Notes))
	out := cmdWriter(cmd, d)
	if asJSON {
		return writeJSON(out, view)
	}
	return writeWorkspaceShowHuman(out, view)
}

// writeWorkspaceShowHuman renders a workspace.ShowView as the
// multi-section human-readable block used by `got workspace
// show`. The ShowView type is the same one the JSON path emits,
// so a single struct powers both the human and the machine
// view.
func writeWorkspaceShowHuman(w io.Writer, v *workspace.ShowView) error {
	if v == nil {
		return errors.New("nil ShowView")
	}
	ws := v.Workspace
	if _, err := fmt.Fprintf(w, "Workspace: %s\n", ws.Name); err != nil {
		return err
	}
	if ws.Title != "" {
		_, _ = fmt.Fprintf(w, "  Title:       %s\n", ws.Title)
	}
	if ws.Description != "" {
		_, _ = fmt.Fprintf(w, "  Description: %s\n", ws.Description)
	}
	if ws.Color != "" {
		_, _ = fmt.Fprintf(w, "  Color:       %s\n", ws.Color)
	}
	_, _ = fmt.Fprintf(w, "  State:       %s\n", ws.State)
	_, _ = fmt.Fprintf(w, "  Created:     %s\n", ws.CreatedAt.Format("2006-01-02 15:04:05 MST"))
	_, _ = fmt.Fprintf(w, "  Updated:     %s\n", ws.UpdatedAt.Format("2006-01-02 15:04:05 MST"))
	_, _ = fmt.Fprintf(w, "  ID:          %s\n", ws.ID)

	if len(v.Files) == 0 {
		_, _ = fmt.Fprintln(w, "\nFiles: (none)")
	} else {
		_, _ = fmt.Fprintf(w, "\nFiles (%d):\n", len(v.Files))
		for _, f := range v.Files {
			note := ""
			if f.Note != "" {
				note = "  -- " + f.Note
			}
			_, _ = fmt.Fprintf(w, "  %s%s\n", f.Path, note)
		}
	}
	if len(v.Branches) == 0 {
		_, _ = fmt.Fprintln(w, "\nBranches: (none)")
	} else {
		_, _ = fmt.Fprintf(w, "\nBranches (%d):\n", len(v.Branches))
		for _, b := range v.Branches {
			_, _ = fmt.Fprintf(w, "  %s\n", b.Branch)
		}
	}
	if len(v.Decisions) == 0 {
		_, _ = fmt.Fprintln(w, "\nDecisions: (none)")
	} else {
		_, _ = fmt.Fprintf(w, "\nDecisions (%d):\n", len(v.Decisions))
		for _, d := range v.Decisions {
			_, _ = fmt.Fprintf(w, "  [%s] %s\n", d.Status, d.Title)
		}
	}
	if len(v.Notes) == 0 {
		_, _ = fmt.Fprintln(w, "\nNotes: (none)")
	} else {
		_, _ = fmt.Fprintf(w, "\nNotes (%d):\n", len(v.Notes))
		for _, n := range v.Notes {
			pinned := ""
			if n.Pinned {
				pinned = " [pinned]"
			}
			_, _ = fmt.Fprintf(w, "  %s%s\n  %s\n", n.UpdatedAt.Format("2006-01-02 15:04"), pinned, n.Body)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// add-file
// ---------------------------------------------------------------------------

func newWorkspaceAddFileCmd(d Deps) *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:   "add-file <name> <path>",
		Short: "Attach a repo-relative path to a workspace",
		Long: `Attach a repo-relative path to a workspace.

The path is stored verbatim; the workspace engine does not check
that the file exists on disk (you can add a path before the
file is created). Re-adding the same path is idempotent: the
note is overwritten and the added_at timestamp is bumped.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceAddFile(cmd.Context(), cmd, d, args[0], args[1], note)
		},
	}
	cmd.Flags().StringVar(&note, "note", "", `optional note (e.g. "touched in PR #42")`)
	return cmd
}

func runWorkspaceAddFile(ctx context.Context, cmd *cobra.Command, d Deps, name, path, note string) error {
	logger := loggerFor(d)
	logger.Info("workspace add-file starting", "name", name, "path", path)
	st, _, err := openWorkspaceStore(ctx, d)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	w, err := st.Get(ctx, name)
	if err != nil {
		return err
	}
	if err := st.AddFile(ctx, w.ID, path, note); err != nil {
		return err
	}
	logger.Info("workspace add-file finished", "name", name, "path", path)
	out := cmdWriter(cmd, d)
	_, _ = fmt.Fprintf(out, "[got] added %q to workspace %q\n", path, w.Name)
	return nil
}

// ---------------------------------------------------------------------------
// add-branch
// ---------------------------------------------------------------------------

func newWorkspaceAddBranchCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-branch <name> <branch>",
		Short: "Tag a Git branch as relevant to a workspace",
		Long: `Tag a Git branch as relevant to a workspace.

The branch is recorded by short ref name (e.g. "feature/x" or
"main"). The workspace does not own the branch; deleting the
workspace leaves the branch alone.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceAddBranch(cmd.Context(), cmd, d, args[0], args[1])
		},
	}
	return cmd
}

func runWorkspaceAddBranch(ctx context.Context, cmd *cobra.Command, d Deps, name, branch string) error {
	logger := loggerFor(d)
	logger.Info("workspace add-branch starting", "name", name, "branch", branch)
	st, _, err := openWorkspaceStore(ctx, d)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	w, err := st.Get(ctx, name)
	if err != nil {
		return err
	}
	if err := st.AddBranch(ctx, w.ID, branch); err != nil {
		return err
	}
	logger.Info("workspace add-branch finished", "name", name, "branch", branch)
	out := cmdWriter(cmd, d)
	_, _ = fmt.Fprintf(out, "[got] tagged branch %q on workspace %q\n", branch, w.Name)
	return nil
}

// ---------------------------------------------------------------------------
// add-note
// ---------------------------------------------------------------------------

func newWorkspaceAddNoteCmd(d Deps) *cobra.Command {
	var (
		body      string
		fromStdin bool
		pinned    bool
	)
	cmd := &cobra.Command{
		Use:   "add-note <name>",
		Short: "Attach a free-form note to a workspace",
		Long: `Attach a free-form markdown note to a workspace.

The body comes from --body. Pass --body "-" to read the body
from stdin (useful for multiline notes via a heredoc). The
--pinned flag sticks the note to the top of ` + "`got workspace show`" + `.

Note: empty bodies are rejected. Markdown is preserved verbatim
and never rendered.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceAddNote(cmd.Context(), cmd, d, args[0], body, fromStdin, pinned)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", `note body (use "-" to read from stdin)`)
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "read body from stdin (equivalent to --body -)")
	cmd.Flags().BoolVar(&pinned, "pinned", false, "pin the note to the top of `got workspace show`")
	return cmd
}

func runWorkspaceAddNote(ctx context.Context, cmd *cobra.Command, d Deps, name, body string, fromStdin, pinned bool) error {
	logger := loggerFor(d)
	logger.Info("workspace add-note starting", "name", name)
	resolved, err := resolveNoteBody(body, fromStdin)
	if err != nil {
		return err
	}
	if resolved == "" {
		return gerr.Validation("note body is required (use --body, --body -, or --stdin)")
	}
	st, _, err := openWorkspaceStore(ctx, d)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	w, err := st.Get(ctx, name)
	if err != nil {
		return err
	}
	n := &workspace.WorkspaceNote{WorkspaceID: w.ID, Body: resolved, Pinned: pinned}
	if err := st.AddNote(ctx, n); err != nil {
		return err
	}
	logger.Info("workspace add-note finished", "name", name, "id", n.ID, "pinned", pinned)
	out := cmdWriter(cmd, d)
	_, _ = fmt.Fprintf(out, "[got] added note to workspace %q (id=%s)\n", w.Name, n.ID)
	return nil
}

// ---------------------------------------------------------------------------
// add-decision
// ---------------------------------------------------------------------------

func newWorkspaceAddDecisionCmd(d Deps) *cobra.Command {
	var (
		body      string
		fromStdin bool
		status    string
	)
	cmd := &cobra.Command{
		Use:   "add-decision <name> <title>",
		Short: "Record a decision (lightweight ADR) on a workspace",
		Long: `Record a decision on a workspace. A decision is a title plus an
optional markdown body and a status from {proposed, accepted,
rejected, superseded}. The default status is "proposed"; pass
--status to set it explicitly. The body comes from --body, or
from stdin when --body "-" / --stdin is given.

Decisions are scoped to the workspace and never appear in the
global ADR list. Promote a workspace decision to a global ADR
out of band (or by copy-pasting the body into a .got/decisions/
file).`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceAddDecision(cmd.Context(), cmd, d, args[0], args[1], body, fromStdin, status)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", `decision body (use "-" to read from stdin)`)
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "read body from stdin (equivalent to --body -)")
	cmd.Flags().StringVar(&status, "status", "proposed", "decision status: proposed|accepted|rejected|superseded")
	return cmd
}

func runWorkspaceAddDecision(ctx context.Context, cmd *cobra.Command, d Deps, name, title, body string, fromStdin bool, status string) error {
	logger := loggerFor(d)
	logger.Info("workspace add-decision starting", "name", name, "title", title, "status", status)
	resolved, err := resolveNoteBody(body, fromStdin)
	if err != nil {
		return err
	}
	st, _, err := openWorkspaceStore(ctx, d)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	w, err := st.Get(ctx, name)
	if err != nil {
		return err
	}
	d2 := &workspace.WorkspaceDecision{
		WorkspaceID: w.ID,
		Title:       title,
		Body:        resolved,
		Status:      workspace.DecisionStatus(status),
	}
	if err := st.AddDecision(ctx, d2); err != nil {
		return err
	}
	logger.Info("workspace add-decision finished", "name", name, "id", d2.ID, "status", d2.Status)
	out := cmdWriter(cmd, d)
	_, _ = fmt.Fprintf(out, "[got] added decision %q (%s) to workspace %q\n", d2.Title, d2.Status, w.Name)
	return nil
}

// ---------------------------------------------------------------------------
// shared helpers
// ---------------------------------------------------------------------------

// resolveNoteBody picks the right body source: --stdin, the
// --body "-" sentinel, or the literal --body value. Returns an
// empty string (and no error) if no body was provided, so the
// caller can decide whether to error on empty. Used by both
// add-note and add-decision.
func resolveNoteBody(body string, fromStdin bool) (string, error) {
	if fromStdin {
		return readStdin()
	}
	if body == "-" {
		return readStdin()
	}
	return body, nil
}

// readStdin reads stdin until EOF and trims the trailing
// newline. It is intentionally permissive about empty input: an
// empty stdin becomes an empty string, which the caller rejects
// via a clear validation error.
func readStdin() (string, error) {
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", gerr.Wrap(gerr.CodeGeneric, err, "reading body from stdin")
	}
	return strings.TrimRight(string(b), "\n"), nil
}

// openWorkspaceStore discovers the work tree, opens the
// .got/got.db SQLite store, and wraps it in a *workspace.Store.
// The caller is responsible for closing the returned
// *store.Store via defer.
//
// The (workTree, error) pair lets callers log the work tree
// path or include it in error messages; today it is unused by
// every caller but the function returns it for symmetry with
// the other CLI command helpers.
func openWorkspaceStore(ctx context.Context, d Deps) (*workspace.Store, string, error) {
	workTree, err := d.Discover(".")
	if err != nil {
		return nil, "", err
	}
	if d.StoreFor == nil {
		return nil, "", gerr.Validation("internal: Deps.StoreFor is nil")
	}
	s, err := d.StoreFor(repo.NewPaths(workTree).DBFile)
	if err != nil {
		return nil, "", gerr.Wrap(gerr.CodeGeneric, err, "opening .got/got.db (run `got init` first?)")
	}
	return workspace.New(s), workTree, nil
}
