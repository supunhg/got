// Copyright 2026 The GOT Authors. MIT License.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/supunhg/got/internal/events"
	"github.com/supunhg/got/internal/git"
	"github.com/supunhg/got/internal/store"
)

// newDecisionCmd returns the `got decision` command tree. It is the
// parent for all knowledge-engine decision subcommands.
func newDecisionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decision",
		Short: "Manage architectural decision records (ADRs)",
		Long: `Create, list, show, and link architectural decision records.

Decisions capture the "why" behind codebase changes. They are stored
as Markdown files in .got/decisions/ with metadata in SQLite.`,
	}

	cmd.AddCommand(newDecisionCreateCmd())
	cmd.AddCommand(newDecisionListCmd())
	cmd.AddCommand(newDecisionShowCmd())

	cmd.AddCommand(newDecisionLinkCmd())
	cmd.AddCommand(newDecisionSupersedeCmd())
	cmd.AddCommand(newDecisionUpdateCmd())
	cmd.AddCommand(newDecisionDeleteCmd())

	return cmd
}

// newDecisionCreateCmd returns the `got decision create` subcommand.
//
// In non-interactive mode (--no-interactive or all flags provided) the
// command uses the supplied flags directly. Otherwise it prompts the
// user for each field via stdin.
func newDecisionCreateCmd() *cobra.Command {
	var opts struct {
		title         string
		context       string
		decision      string
		alternatives  string
		consequences  string
		workspace     string
		supersedes    string
		noInteractive bool
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new decision record",
		Long: `Create a new architectural decision record.

A decision captures context, the decision itself, alternatives considered,
and consequences. It is stored as a Markdown file in .got/decisions/
with metadata indexed in SQLite.

Use flags to supply fields non-interactively:
  got decision create --title "Use SQLite" --decision "..." --no-interactive

Without flags, the command prompts for each field interactively.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDecisionCreate(cmd, &opts)
		},
	}

	// Flag set matching §6.2 of knowledge-engine-spec.md.
	flags := cmd.Flags()
	flags.StringVar(&opts.title, "title", "", "Decision title")
	flags.StringVar(&opts.context, "context", "", "Context / background")
	flags.StringVar(&opts.decision, "decision", "", "The decision made")
	flags.StringVar(&opts.alternatives, "alternatives", "", "Alternatives considered")
	flags.StringVar(&opts.consequences, "consequences", "", "Positive/negative consequences")
	flags.StringVar(&opts.workspace, "workspace", "", "Scope to a workspace (auto-created if not found)")
	flags.StringVar(&opts.supersedes, "supersedes", "", "ULID of a decision this one supersedes")
	flags.BoolVar(&opts.noInteractive, "no-interactive", false, "Use all flags, do not prompt")

	return cmd
}

// runDecisionCreate executes the create flow: gather input, init DB,
// create the decision, print the result.
func runDecisionCreate(cmd *cobra.Command, opts *struct {
	title         string
	context       string
	decision      string
	alternatives  string
	consequences  string
	workspace     string
	supersedes    string
	noInteractive bool
},
) error {
	ctx := context.Background()

	// ── Gather input ──────────────────────────────────────────────
	title := opts.title
	noInt := opts.noInteractive

	// If --title is empty and we're not forced non-interactive, prompt.
	if title == "" && !noInt {
		prompted, err := promptFields()
		if err != nil {
			return fmt.Errorf("input error: %w", err)
		}
		title = prompted.title
		opts.context = prompted.context
		opts.decision = prompted.decision
		opts.alternatives = prompted.alternatives
		opts.consequences = prompted.consequences
		opts.workspace = prompted.workspace
		opts.supersedes = prompted.supersedes
	} else if title == "" {
		return fmt.Errorf("decision title is required (use --title)")
	}

	// ── Open store, create bus, wire KnowledgeStore ───────────────
	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	// ── Map workspace flag ────────────────────────────────────────
	var workspaceID *string
	if opts.workspace != "" {
		// In v0.4 the workspace engine is not yet built, so we store
		// the workspace name directly as a stand-in for the workspace
		// ULID. A future version will resolve names to workspace IDs.
		workspaceID = &opts.workspace
	}

	// ── Map supersedes flag ──────────────────────────────────────
	var supersedesID *string
	if opts.supersedes != "" {
		supersedesID = &opts.supersedes
	}

	// ── Create the decision ──────────────────────────────────────
	d, err := ks.CreateDecision(ctx, store.CreateDecisionParams{
		Title:        title,
		Context:      opts.context,
		Decision:     opts.decision,
		Alternatives: opts.alternatives,
		Consequences: opts.consequences,
		WorkspaceID:  workspaceID,
		SupersedesID: supersedesID,
	})
	if err != nil {
		return fmt.Errorf("create decision: %w", err)
	}

	// ── Output ───────────────────────────────────────────────────
	fmt.Fprintf(cmd.OutOrStdout(), "Decision created:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  ID:     %s\n", d.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Title:  %s\n", d.Title)
	fmt.Fprintf(cmd.OutOrStdout(), "  Status: %s\n", d.Status)
	fmt.Fprintf(cmd.OutOrStdout(), "  Path:   .got/%s\n", d.BodyPath)

	if opts.supersedes != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Note: decision %s has been marked as superseded\n", opts.supersedes)
	}

	return nil
}

// ── Decision list ───────────────────────────────────────────────────

// listOpts holds the parsed flags for `got decision list`.
type listOpts struct {
	workspace string
	status    string
	limit     int
	all       bool
	jsonOut   bool
}

func newDecisionListCmd() *cobra.Command {
	var opts listOpts

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List decisions, filterable",
		Long: `List architectural decision records with optional filters.

By default shows the 20 most recent decisions. Use --all to show all,
or --limit to set a custom limit.

Examples:
  got decision list
  got decision list --status proposed
  got decision list --workspace engine --all
  got decision list --status accepted --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDecisionList(cmd, &opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.workspace, "workspace", "", "Filter by workspace name")
	flags.StringVar(&opts.status, "status", "", "Filter by status: proposed, accepted, rejected, superseded")
	flags.IntVar(&opts.limit, "limit", 0, "Max results (default 20, 0=default)")
	flags.BoolVar(&opts.all, "all", false, "Show all decisions (no limit)")
	flags.BoolVar(&opts.jsonOut, "json", false, "Output as JSON")

	return cmd
}

func runDecisionList(cmd *cobra.Command, opts *listOpts) error {
	ctx := context.Background()

	// ── Open store ───────────────────────────────────────────────
	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	// ── Validate status ────────────────────────────────────────────
	if opts.status != "" {
		switch opts.status {
		case "proposed", "accepted", "rejected", "superseded":
		default:
			return fmt.Errorf("invalid status %q: must be one of: proposed, accepted, rejected, superseded", opts.status)
		}
	}

	// ── Build filter ─────────────────────────────────────────────
	filter := store.DecisionFilter{
		Limit: opts.limit,
		All:   opts.all,
	}

	if opts.workspace != "" {
		filter.WorkspaceID = &opts.workspace
	}
	if opts.status != "" {
		filter.Status = &opts.status
	}

	// ── Query ────────────────────────────────────────────────────
	decisions, err := ks.ListDecisions(ctx, filter)
	if err != nil {
		return fmt.Errorf("list decisions: %w", err)
	}

	// ── Output ───────────────────────────────────────────────────
	if opts.jsonOut {
		return outputDecisionsJSON(cmd, decisions)
	}
	return outputDecisionsTable(cmd, decisions)
}

// outputDecisionsJSON writes decisions as a JSON array to stdout.
func outputDecisionsJSON(cmd *cobra.Command, decisions []store.Decision) error {
	if decisions == nil {
		decisions = []store.Decision{} // ensure [] not null
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	if err := enc.Encode(decisions); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}

// outputDecisionsTable writes decisions as an aligned table to stdout.
func outputDecisionsTable(cmd *cobra.Command, decisions []store.Decision) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)

	if len(decisions) == 0 {
		fmt.Fprintln(w, "No decisions found.")
		return w.Flush()
	}

	// Header
	fmt.Fprintln(w, "ID\tSTATUS\tTITLE\tWORKSPACE\tCREATED")

	for _, d := range decisions {
		ws := ""
		if d.WorkspaceID != nil {
			ws = *d.WorkspaceID
		}
		date := time.UnixMilli(d.CreatedAt).Format("2006-01-02")
		// Truncate title to 50 chars for table readability.
		title := d.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		// Truncate ID to 18 chars for table readability.
		id := d.ID
		if len(id) > 18 {
			id = id[:15] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", id, d.Status, title, ws, date)
	}

	return w.Flush()
}

// ── Decision show ───────────────────────────────────────────────────

func newDecisionShowCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a decision in full",
		Long: `Display the full contents of an architectural decision record.

Shows the title, status, context, decision, alternatives, consequences,
and any linked commits, files, or workspaces.

Examples:
  got decision show 01JQZ3ZABC
  got decision show 01JQZ3ZABC --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDecisionShow(cmd, args[0], jsonOut)
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func runDecisionShow(cmd *cobra.Command, id string, jsonOut bool) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	// ── Fetch decision ────────────────────────────────────────────
	d, err := ks.GetDecision(ctx, id)
	if err != nil {
		return fmt.Errorf("show decision: %w", err)
	}

	// ── Fetch links ──────────────────────────────────────────────
	links, _ := ks.GetDecisionLinks(ctx, id)

	// ── Output ───────────────────────────────────────────────────
	if jsonOut {
		return showDecisionJSON(cmd, d, links)
	}
	return showDecisionTerminal(cmd, d, links)
}

// showDecisionJSON writes the decision (with embedded links) as JSON.
func showDecisionJSON(cmd *cobra.Command, d *store.Decision, links []store.DecisionLink) error {
	out := struct {
		Decision store.Decision       `json:"decision"`
		Links    []store.DecisionLink `json:"links"`
	}{
		Decision: *d,
		Links:    links,
	}
	if out.Links == nil {
		out.Links = []store.DecisionLink{}
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}

// showDecisionTerminal renders the decision as a formatted terminal view.
func showDecisionTerminal(cmd *cobra.Command, d *store.Decision, links []store.DecisionLink) error {
	w := cmd.OutOrStdout()

	// ── Title & status badge ────────────────────────────────────
	fmt.Fprintf(w, "# %s\n\n", d.Title)

	ws := ""
	if d.WorkspaceID != nil {
		ws = *d.WorkspaceID
	}
	if ws != "" {
		fmt.Fprintf(w, "**Status:** %s  |  **Workspace:** %s  |  **ID:** %s\n", d.Status, ws, d.ID)
	} else {
		fmt.Fprintf(w, "**Status:** %s  |  **ID:** %s\n", d.Status, d.ID)
	}
	fmt.Fprintf(
		w, "**Created:** %s  |  **Updated:** %s\n",
		time.UnixMilli(d.CreatedAt).Format("2006-01-02 15:04 MST"),
		time.UnixMilli(d.UpdatedAt).Format("2006-01-02 15:04 MST"),
	)

	if d.SupersedesID != nil && *d.SupersedesID != "" {
		fmt.Fprintf(w, "**Supersedes:** %s\n", *d.SupersedesID)
	}

	fmt.Fprintln(w)

	// ── Sections ────────────────────────────────────────────────
	if d.Context != "" {
		fmt.Fprintf(w, "## Context\n\n%s\n\n", d.Context)
	}
	if d.Decision != "" {
		fmt.Fprintf(w, "## Decision\n\n%s\n\n", d.Decision)
	}
	if d.Alternatives != "" {
		fmt.Fprintf(w, "## Alternatives Considered\n\n%s\n\n", d.Alternatives)
	}
	if d.Consequences != "" {
		fmt.Fprintf(w, "## Consequences\n\n%s\n\n", d.Consequences)
	}

	// ── Links ───────────────────────────────────────────────────
	if len(links) > 0 {
		fmt.Fprintf(w, "## Links\n\n")
		for _, l := range links {
			lineRange := ""
			if l.LineStart != nil {
				if l.LineEnd != nil && *l.LineEnd != *l.LineStart {
					lineRange = fmt.Sprintf(":%d-%d", *l.LineStart, *l.LineEnd)
				} else {
					lineRange = fmt.Sprintf(":%d", *l.LineStart)
				}
			}
			linkStr := fmt.Sprintf("%s %s%s", l.LinkType, l.Target, lineRange)
			if l.Branch != "" {
				linkStr += fmt.Sprintf(" (branch: %s)", l.Branch)
			}
			fmt.Fprintf(w, "- **%s:** %s\n", l.LinkType, linkStr)
		}
		fmt.Fprintln(w)
	}

	// ── Footer ─────────────────────────────────────────────────
	fmt.Fprintf(w, "---\nFile: .got/%s\n", d.BodyPath)

	return nil
}

// ── Decision link ───────────────────────────────────────────────────

func newDecisionLinkCmd() *cobra.Command {
	var commitSHA, filePath, workspace, branchFlag string
	var lineStart, lineEnd int
	var autoLink, branchLink bool

	cmd := &cobra.Command{
		Use:   "link <id>",
		Short: "Link a decision to commits, files, or workspaces",
		Long: `Attach a link (commit, file, or workspace) to a decision.

At least one of --commit, --file, --workspace, --branch, or --auto must be provided.

--auto links to the most recent commit on the current branch.
--branch links to all commits on the specified branch.

Examples:
  got decision link 01JQZ3ZABC --commit HEAD
  got decision link 01JQZ3ZABC --file src/main.go --line-start 42 --line-end 58
  got decision link 01JQZ3ZABC --file src/main.go --branch feature/x
  got decision link 01JQZ3ZABC --workspace engine
  got decision link 01JQZ3ZABC --auto
  got decision link 01JQZ3ZABC --branch feat/oauth
  got decision link 01JQZ3ZABC --branch feat/oauth --commit abc123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDecisionLink(cmd, args[0], commitSHA, filePath, workspace, branchFlag,
				lineStart, lineEnd, autoLink, branchLink)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&commitSHA, "commit", "", "Link to a commit SHA or ref")
	flags.StringVar(&filePath, "file", "", "Link to a file path")
	flags.StringVar(&workspace, "workspace", "", "Link to a workspace")
	flags.StringVar(&branchFlag, "branch", "", "Branch context for the link, or link to all commits on branch")
	flags.IntVar(&lineStart, "line-start", 0, "Start line (file links only)")
	flags.IntVar(&lineEnd, "line-end", 0, "End line (file links only)")
	flags.BoolVar(&autoLink, "auto", false, "Link to the most recent commit on the current branch")
	flags.BoolVar(&branchLink, "branch-link", false, "Link to all commits on a branch (use with --branch)")

	return cmd
}

func runDecisionLink(cmd *cobra.Command, decisionID, commitSHA, filePath, workspace, branchFlag string,
	lineStart, lineEnd int, autoLink, branchLink bool,
) error {
	ctx := context.Background()

	// ── Determine link type and target ───────────────────────────
	var linkType, target string
	commitCount := 0

	// Handle --auto: resolve current branch HEAD.
	if autoLink {
		repoPath, repoErr := findRepoRoot()
		if repoErr != nil {
			return fmt.Errorf("--auto: not in a Git repository: %w", repoErr)
		}
		adapter := git.NewExecAdapter(nil)
		if err := adapter.OpenRepository(ctx, repoPath); err != nil {
			return fmt.Errorf("--auto: open repo: %w", err)
		}
		sha, _, err := adapter.Run(ctx, "rev-parse", "HEAD")
		if err != nil {
			return fmt.Errorf("--auto: resolve HEAD: %w", err)
		}
		commitSHA = sha
	}

	// Handle --branch-link: if --branch is set and --commit is not, link to the branch.
	// The actual commit linking happens per-commit below.
	if branchLink && branchFlag != "" && commitSHA == "" {
		// Just link to the branch itself.
		linkType = "branch"
		target = branchFlag
		commitCount++
	}

	if commitSHA != "" {
		linkType = "commit"
		target = commitSHA
		commitCount++
	}
	if filePath != "" {
		linkType = "file"
		target = filePath
		commitCount++
	}
	if workspace != "" {
		linkType = "workspace"
		target = workspace
		commitCount++
	}
	// If only --branch is provided (no --branch-link), it's just context for another link.
	if commitCount == 0 && branchFlag != "" && !branchLink {
		return fmt.Errorf("link decision: --branch alone only provides context; use with --commit, --file, --workspace, or --branch-link")
	}

	if commitCount == 0 && !branchLink {
		return fmt.Errorf("link decision: specify one of --commit, --file, --workspace, --auto, or --branch --branch-link")
	}
	if commitCount > 1 {
		return fmt.Errorf("link decision: specify only one of --commit, --file, --workspace, or --branch --branch-link")
	}

	// ── Map line numbers to pointers (0 means unset) ─────────────
	var ls, le *int
	if lineStart > 0 {
		ls = &lineStart
	}
	if lineEnd > 0 {
		le = &lineEnd
	}

	// ── Open store, create bus, wire KnowledgeStore ───────────────
	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	// ── Create the link ──────────────────────────────────────────
	if err := ks.LinkDecision(ctx, store.LinkDecisionParams{
		DecisionID: decisionID,
		LinkType:   linkType,
		Target:     target,
		LineStart:  ls,
		LineEnd:    le,
		Branch:     branchFlag,
	}); err != nil {
		return fmt.Errorf("link decision: %w", err)
	}

	// ── Show existing links ──────────────────────────────────────
	links, _ := ks.GetDecisionLinks(ctx, decisionID)

	fmt.Fprintf(cmd.OutOrStdout(), "Linked %s %s to decision %s\n", linkType, target, decisionID)

	if len(links) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\nExisting links:")
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "TYPE\tTARGET\tBRANCH\tCREATED")
		for _, l := range links {
			date := time.UnixMilli(l.CreatedAt).Format("2006-01-02")
			lineInfo := ""
			if l.LineStart != nil {
				if l.LineEnd != nil && *l.LineEnd != *l.LineStart {
					lineInfo = fmt.Sprintf(":%d-%d", *l.LineStart, *l.LineEnd)
				} else {
					lineInfo = fmt.Sprintf(":%d", *l.LineStart)
				}
			}
			targetDisplay := l.Target + lineInfo
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", l.LinkType, targetDisplay, l.Branch, date)
		}
		_ = w.Flush()
	}

	return nil
}

// ── Decision supersede ──────────────────────────────────────────────

func newDecisionSupersedeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "supersede <old-id> <new-id>",
		Short: "Mark a decision as superseded by another",
		Long: `Mark an existing decision as superseded by a newer one.

This sets the old decision's status to 'superseded' and updates the
new decision's supersedes_id to point to the old one. Both decisions
must exist.

Examples:
  got decision supersede 01JQZ3ZABC 01JQZ4ZABC`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDecisionSupersede(cmd, args[0], args[1])
		},
	}

	return cmd
}

func runDecisionSupersede(cmd *cobra.Command, oldID, newID string) error {
	ctx := context.Background()

	if oldID == newID {
		return fmt.Errorf("supersede: a decision cannot supersede itself")
	}

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	if err := ks.SupersedeDecision(ctx, oldID, newID); err != nil {
		return fmt.Errorf("supersede: %w", err)
	}

	// Show both decisions for context.
	oldD, _ := ks.GetDecision(ctx, oldID)
	newD, _ := ks.GetDecision(ctx, newID)

	fmt.Fprintf(cmd.OutOrStdout(), "Decision %s has been superseded by %s\n", oldID, newID)
	if oldD != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s (old): %s\n", oldID, oldD.Title)
	}
	if newD != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s (new): %s\n", newID, newD.Title)
	}

	return nil
}

// ── Decision update ────────────────────────────────────────────────

func newDecisionUpdateCmd() *cobra.Command {
	var opts struct {
		title          string
		context        string
		decision       string
		alternatives   string
		consequences   string
		workspace      string
		clearWorkspace bool
	}

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a decision's body fields",
		Long: `Update the body fields of an existing architectural decision record.

Only the flags that are explicitly set will be updated. Unset flags
leave the existing values unchanged.

To clear the workspace, use --workspace "" or --clear-workspace.

Examples:
  got decision update 01JQZ3ZABC --decision "Revised decision"
  got decision update 01JQZ3ZABC --context "New context" --consequences "Updated"
  got decision update 01JQZ3ZABC --title "New title"
  got decision update 01JQZ3ZABC --clear-workspace`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDecisionUpdate(cmd, args[0], &opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.title, "title", "", "Update the title")
	flags.StringVar(&opts.context, "context", "", "Update the context/background")
	flags.StringVar(&opts.decision, "decision", "", "Update the decision made")
	flags.StringVar(&opts.alternatives, "alternatives", "", "Update alternatives considered")
	flags.StringVar(&opts.consequences, "consequences", "", "Update positive/negative consequences")
	flags.StringVar(&opts.workspace, "workspace", "", "Set workspace (empty string to clear)")
	flags.BoolVar(&opts.clearWorkspace, "clear-workspace", false, "Clear the workspace assignment")

	return cmd
}

func runDecisionUpdate(cmd *cobra.Command, id string, opts *struct {
	title          string
	context        string
	decision       string
	alternatives   string
	consequences   string
	workspace      string
	clearWorkspace bool
},
) error {
	ctx := context.Background()

	// ── Build params from non-empty flags ────────────────────────
	var params store.UpdateDecisionParams

	if opts.title != "" {
		params.Title = &opts.title
	}
	if opts.context != "" {
		params.Context = &opts.context
	}
	if opts.decision != "" {
		params.Decision = &opts.decision
	}
	if opts.alternatives != "" {
		params.Alternatives = &opts.alternatives
	}
	if opts.consequences != "" {
		params.Consequences = &opts.consequences
	}

	// Handle workspace: --workspace "" or --clear-workspace both clear it.
	if cmd.Flags().Changed("workspace") {
		params.WorkspaceID = &opts.workspace
	} else if opts.clearWorkspace {
		empty := ""
		params.WorkspaceID = &empty
	}

	// ── Check that at least one field was provided ───────────────
	if params.Title == nil && params.Context == nil && params.Decision == nil &&
		params.Alternatives == nil && params.Consequences == nil && params.WorkspaceID == nil {
		return fmt.Errorf("update decision: specify at least one field to update (--context, --decision, --consequences, etc.)")
	}

	// ── Open store ──────────────────────────────────────────────
	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	// ── Update ──────────────────────────────────────────────────
	updated, err := ks.UpdateDecision(ctx, id, params)
	if err != nil {
		return fmt.Errorf("update decision: %w", err)
	}

	// ── Output ──────────────────────────────────────────────────
	fmt.Fprintf(cmd.OutOrStdout(), "Decision updated:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  ID:     %s\n", updated.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Title:  %s\n", updated.Title)
	fmt.Fprintf(cmd.OutOrStdout(), "  Status: %s\n", updated.Status)
	fmt.Fprintf(cmd.OutOrStdout(), "  Path:   .got/%s\n", updated.BodyPath)

	return nil
}

// ── Decision delete ──────────────────────────────────────────────────

func newDecisionDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Permanently delete a decision and its links",
		Long: `Delete an architectural decision record from the database.

This permanently removes the decision, all its associated links
(commits, files, workspaces), and the decision body file from disk.

This action cannot be undone. Consider using 'got decision update
--status rejected' if you want to keep the record but indicate the
decision was not adopted.

Examples:
  got decision delete 01JQZ3ZABC`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDecisionDelete(cmd, args[0])
		},
	}

	return cmd
}

func runDecisionDelete(cmd *cobra.Command, id string) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	// Fetch first so we can show what's being deleted.
	d, err := ks.GetDecision(ctx, id)
	if err != nil {
		return fmt.Errorf("delete decision: %w", err)
	}

	if err := ks.DeleteDecision(ctx, id); err != nil {
		return fmt.Errorf("delete decision: %w", err)
	}

	// ── Remove the body file from disk ───────────────────────────
	gotDir, err := findGotDir()
	if err == nil && d.BodyPath != "" {
		bodyPath := filepath.Join(gotDir, d.BodyPath)
		if err := os.Remove(bodyPath); err != nil && !os.IsNotExist(err) {
			// Warn but don't fail — the DB delete already succeeded.
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not remove body file %s: %v\n", bodyPath, err)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Decision deleted:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  ID:     %s\n", d.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Title:  %s\n", d.Title)
	fmt.Fprintf(cmd.OutOrStdout(), "  Status: %s\n", d.Status)

	return nil
}

// ── Interactive prompting ───────────────────────────────────────────

type promptedFields struct {
	title, context, decision, alternatives, consequences, workspace, supersedes string
}

// promptFields reads decision fields from stdin interactively.
func promptFields() (*promptedFields, error) {
	reader := bufio.NewReader(os.Stdin)
	p := &promptedFields{}
	var err error

	if p.title, err = prompt(reader, "Title: "); err != nil {
		return nil, err
	}
	if p.title == "" {
		return nil, errors.New("decision title is required")
	}

	if p.context, err = promptMultiline(reader, "Context (Ctrl+D to end, empty to skip): "); err != nil {
		return nil, err
	}

	if p.decision, err = promptMultiline(reader, "Decision (Ctrl+D to end, empty to skip): "); err != nil {
		return nil, err
	}

	if p.alternatives, err = promptMultiline(reader, "Alternatives (Ctrl+D to end, empty to skip): "); err != nil {
		return nil, err
	}

	if p.consequences, err = promptMultiline(reader, "Consequences (Ctrl+D to end, empty to skip): "); err != nil {
		return nil, err
	}

	if p.workspace, err = prompt(reader, "Workspace (optional): "); err != nil {
		return nil, err
	}

	if p.supersedes, err = prompt(reader, "Supersedes decision ID (optional): "); err != nil {
		return nil, err
	}

	return p, nil
}

// prompt reads a single-line string from the reader.
func prompt(reader *bufio.Reader, label string) (string, error) {
	fmt.Fprint(os.Stderr, label)
	text, err := reader.ReadString('\n')
	if err != nil {
		// EOF is fine — treat as empty input.
		return "", nil
	}
	return strings.TrimSpace(text), nil
}

// promptMultiline reads multiple lines until EOF (Ctrl+D).
func promptMultiline(reader *bufio.Reader, label string) (string, error) {
	fmt.Fprint(os.Stderr, label)
	var lines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "")), nil
}

// ── Store + bus wiring ─────────────────────────────────────────────

// openKnowledgeStore opens the GOT database, creates an event bus, wires
// the KnowledgeStore and EventLogger, and returns a closer. The caller
// must call Close on the returned closer when done.
type knowledgeStoreCloser struct {
	ks    *store.KnowledgeStore
	st    *store.Store
	bus   *events.Bus
	el    *store.EventLogger
	owned bool // true if this closer owns the bus (should close it)
}

func (c *knowledgeStoreCloser) Close() {
	c.el.Close()
	// Only close the bus if we own it (not the shared globalBus).
	if c.owned {
		c.bus.Close()
	}
	c.st.Close()
}

func openKnowledgeStore() (*knowledgeStoreCloser, error) {
	gotDir, err := findGotDir()
	if err != nil {
		return nil, fmt.Errorf("GOT not initialized: %w", err)
	}

	dbPath := filepath.Join(gotDir, "got.db")
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("GOT database not found at %s (run 'got init' first)", dbPath)
	}

	s, err := store.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	// Use the shared global bus if it exists (loaded by PersistentPreRunE),
	// otherwise create a new one. The shared bus ensures plugin hooks
	// receive events published by this command.
	bus := globalBus
	owned := false
	if bus == nil {
		bus = events.New()
		owned = true
	}

	ks := store.NewKnowledgeStore(s.DB(), bus)
	el := store.NewEventLogger(s.DB(), bus)

	return &knowledgeStoreCloser{ks: ks, st: s, bus: bus, el: el, owned: owned}, nil
}

// ── Repository discovery ────────────────────────────────────────────

// findGotDir walks up from the current directory (or --cwd) looking for
// a .got/ directory. Returns the path to .got/ on success.
func findGotDir() (string, error) {
	// Use --cwd if provided, otherwise cwd.
	// In v0.4 this is a simple file-walk; a future version will
	// integrate with the Git adapter for proper repo discovery.
	start, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	dir := start
	for {
		gotPath := filepath.Join(dir, ".got")
		if info, err := os.Stat(gotPath); err == nil && info.IsDir() {
			return gotPath, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root without finding .got/.
			return "", fmt.Errorf("no .got/ directory found in %s or any parent", start)
		}
		dir = parent
	}
}
