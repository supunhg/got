// Copyright 2026 The GOT Authors. MIT License.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/supunhg/got/internal/store"
)

// newOnboardCmd returns the `got onboard` command tree.
func newOnboardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "onboard",
		Short: "Manage onboarding sessions for new contributors",
		Long: `Start, track, and complete onboarding sessions.

Onboarding helps new contributors discover existing decisions, notes,
and relevant files in a codebase. Sessions track which items a person
has reviewed and which remain.`,

		// Provide a helpful hint if someone runs `got onboard` without a subcommand.
		// There is no default action; the user must pick start/list/complete.
	}

	cmd.AddCommand(newOnboardStartCmd())
	cmd.AddCommand(newOnboardListCmd())
	cmd.AddCommand(newOnboardProgressCmd())
	cmd.AddCommand(newOnboardMarkCmd())
	cmd.AddCommand(newOnboardSkipCmd())
	cmd.AddCommand(newOnboardCompleteCmd())

	return cmd
}

// ── Onboard start ──────────────────────────────────────────────────

func newOnboardStartCmd() *cobra.Command {
	var participant string

	cmd := &cobra.Command{
		Use:   "start [participant]",
		Short: "Start a new onboarding session (or resume an active one)",
		Long: `Start or resume an onboarding session for a participant.

If the participant already has an active session, it is resumed.
Otherwise a new session is created, seeding existing decisions and
notes as items to cover.

Examples:
  got onboard start alice@example.com
  got onboard start --participant bob`,

		// participant is optional: first arg, or --participant flag, or default
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Prefer positional arg over --participant flag.
			if len(args) > 0 {
				participant = args[0]
			}
			return runOnboardStart(cmd, participant)
		},
	}

	cmd.Flags().StringVar(&participant, "participant", "", "Participant name (default: auto-detected or 'unknown')")

	return cmd
}

func runOnboardStart(cmd *cobra.Command, participant string) error {
	ctx := context.Background()

	// Default participant if not provided.
	participant = strings.TrimSpace(participant)
	if participant == "" {
		participant = "unknown"
	}

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	session, err := ks.StartOnboarding(ctx, participant)
	if err != nil {
		return fmt.Errorf("onboard start: %w", err)
	}

	// Fetch progress for display.
	prog, err := ks.GetOnboardingProgress(ctx, session.ID)
	if err != nil {
		// Non-fatal — we still have the session.
		prog = nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Onboarding session:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  ID:          %s\n", session.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Participant: %s\n", session.Participant)
	fmt.Fprintf(cmd.OutOrStdout(), "  Status:      %s\n", session.Status)
	fmt.Fprintf(cmd.OutOrStdout(), "  Created:     %s\n", time.UnixMilli(session.CreatedAt).Format("2006-01-02 15:04 MST"))

	if prog != nil {
		fmt.Fprintln(cmd.OutOrStdout())
		renderProgressTable(cmd, prog)
	}

	return nil
}

// ── Onboard list ───────────────────────────────────────────────────

func newOnboardListCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "list <session-id>",
		Short: "Show onboarding progress and remaining items",
		Long: `Display the progress of an onboarding session, including
coverage grouped by item type and a list of items not yet covered.

Examples:
  got onboard list 01JQZ3ZABC
  got onboard list 01JQZ3ZABC --json`,

		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOnboardList(cmd, args[0], jsonOut)
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")

	return cmd
}

func runOnboardList(cmd *cobra.Command, sessionID string, jsonOut bool) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	// ── Fetch progress ───────────────────────────────────────────
	prog, err := ks.GetOnboardingProgress(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("onboard list: %w", err)
	}

	// ── Fetch unwatched items ────────────────────────────────────
	items, _ := ks.ListUnwatchedItems(ctx, sessionID)

	// ── Output ───────────────────────────────────────────────────
	if jsonOut {
		return renderOnboardListJSON(cmd, prog, items)
	}

	return renderOnboardListTerminal(cmd, prog, items)
}

// renderOnboardListJSON writes the full progress + items as JSON.
func renderOnboardListJSON(cmd *cobra.Command, prog *store.OnboardingProgress, items []store.OnboardingItem) error {
	if items == nil {
		items = []store.OnboardingItem{}
	}

	out := struct {
		Progress *store.OnboardingProgress `json:"progress"`
		Items    []store.OnboardingItem    `json:"unwatched_items"`
	}{
		Progress: prog,
		Items:    items,
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}

// renderOnboardListTerminal displays progress summary + unwatched items.
func renderOnboardListTerminal(cmd *cobra.Command, prog *store.OnboardingProgress, items []store.OnboardingItem) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)

	// ── Session header ───────────────────────────────────────────
	fmt.Fprintf(w, "Session:     %s\n", prog.Session.ID)
	fmt.Fprintf(w, "Participant: %s\n", prog.Session.Participant)
	fmt.Fprintf(w, "Status:      %s\n", prog.Session.Status)
	fmt.Fprintf(w, "Created:     %s\n", time.UnixMilli(prog.Session.CreatedAt).Format("2006-01-02 15:04 MST"))
	if prog.Session.UpdatedAt > 0 {
		fmt.Fprintf(w, "Updated:     %s\n", time.UnixMilli(prog.Session.UpdatedAt).Format("2006-01-02 15:04 MST"))
	}
	fmt.Fprintln(w)

	// ── Progress by type ─────────────────────────────────────────
	fmt.Fprintln(w, "ITEM TYPE\tTOTAL\tCOVERED\tSKIPPED\tREMAINING")
	fmt.Fprintln(w, "---------\t-----\t-------\t-------\t--------")

	// Sort types for deterministic output.
	typeNames := sortedKeys(prog.ByType)
	for _, t := range typeNames {
		tp := prog.ByType[t]
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\n", t, tp.Total, tp.Covered, tp.Skipped, tp.Remaining)
	}

	fmt.Fprintln(w, "---------\t-----\t-------\t-------\t--------")
	fmt.Fprintf(w, "TOTAL\t%d\t%d\t%d\t%d\n", prog.TotalItems, prog.Covered, prog.Skipped, prog.Remaining)
	fmt.Fprintln(w)

	// ── Unwatched items ──────────────────────────────────────────
	if len(items) > 0 {
		fmt.Fprintln(w, "Unwatched items:")

		var currentType string
		for _, item := range items {
			if item.ItemType != currentType {
				if currentType != "" {
					fmt.Fprintln(w)
				}
				currentType = item.ItemType
				fmt.Fprintf(w, "  %s:\n", currentType)
			}
			fmt.Fprintf(w, "    - %s\n", item.ItemTarget)
		}
	} else if prog.Remaining > 0 {
		// This shouldn't happen in practice, but guard against it.
		fmt.Fprintln(w, "No unwatched items found (remaining > 0 suggests a data inconsistency).")
	} else {
		fmt.Fprintln(w, "All items covered! 🎉")
	}

	return w.Flush()
}

// ── Onboard complete ───────────────────────────────────────────────

func newOnboardCompleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "complete <session-id>",
		Short: "Mark an onboarding session as completed",
		Long: `Complete an active onboarding session.

Displays a final progress summary before marking the session as completed.
After completion, no further items can be covered.

Examples:
  got onboard complete 01JQZ3ZABC`,

		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOnboardComplete(cmd, args[0])
		},
	}

	return cmd
}

func runOnboardComplete(cmd *cobra.Command, sessionID string) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	// ── Fetch progress for final report ──────────────────────────
	prog, err := ks.GetOnboardingProgress(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("onboard complete: %w", err)
	}

	// ── Mark complete ────────────────────────────────────────────
	if err := ks.CompleteOnboarding(ctx, sessionID); err != nil {
		return fmt.Errorf("onboard complete: %w", err)
	}

	// ── Output ───────────────────────────────────────────────────
	fmt.Fprintf(cmd.OutOrStdout(), "Onboarding session %s completed!\n\n", sessionID)
	renderProgressTable(cmd, prog)

	if prog.Remaining > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nNote: %d item(s) were not covered or skipped.\n", prog.Remaining)
	}

	return nil
}

// ── Onboard progress ───────────────────────────────────────────────

func newOnboardProgressCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "progress <session-id>",
		Short: "Show onboarding session progress summary",
		Long: `Display a structured progress report for an onboarding session,
showing coverage counts grouped by item type (decisions, notes).

Examples:
  got onboard progress 01JQZ3ZABC
  got onboard progress 01JQZ3ZABC --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOnboardProgress(cmd, args[0], jsonOut)
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func runOnboardProgress(cmd *cobra.Command, sessionID string, jsonOut bool) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	prog, err := ks.GetOnboardingProgress(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("onboard progress: %w", err)
	}

	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(prog)
	}

	// ── Terminal output ─────────────────────────────────────────
	fmt.Fprintf(cmd.OutOrStdout(), "Onboarding session for %s (%s)\n", prog.Session.Participant, prog.Session.Status)
	fmt.Fprintf(cmd.OutOrStdout(), "Started: %s\n", time.UnixMilli(prog.Session.CreatedAt).Format("2006-01-02"))
	fmt.Fprintln(cmd.OutOrStdout())

	renderProgressTable(cmd, prog)

	if prog.TotalItems > 0 {
		pct := (prog.Covered * 100) / prog.TotalItems
		fmt.Fprintf(cmd.OutOrStdout(), "\nOverall: %d / %d items covered (%d%%)\n", prog.Covered, prog.TotalItems, pct)
	}

	return nil
}

// ── Onboard mark ────────────────────────────────────────────────────

func newOnboardMarkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mark <session-id> <type> <id>",
		Short: "Mark an onboarding item as covered",
		Long: `Mark an onboarding item (decision, note, or file) as covered.

<type> must be one of: decision, note, file.
<id> is the decision ID, note ID, or file path.

Examples:
  got onboard mark 01JQZ3ZABC decision 01JQZ4ZABC
  got onboard mark 01JQZ3ZABC note 01JQZ5ZABC
  got onboard mark 01JQZ3ZABC file src/main.go`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOnboardMarkSkip(cmd, args[0], args[1], args[2], true)
		},
	}

	return cmd
}

func newOnboardSkipCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skip <session-id> <type> <id>",
		Short: "Skip an onboarding item",
		Long: `Skip an onboarding item (decision, note, or file).

Skipped items are not shown again unless --all is used.

<type> must be one of: decision, note, file.
<id> is the decision ID, note ID, or file path.

Examples:
  got onboard skip 01JQZ3ZABC decision 01JQZ4ZABC
  got onboard skip 01JQZ3ZABC file src/main.go`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOnboardMarkSkip(cmd, args[0], args[1], args[2], false)
		},
	}

	return cmd
}

func runOnboardMarkSkip(cmd *cobra.Command, sessionID, itemType, itemTarget string, mark bool) error {
	ctx := context.Background()

	cmdName := "onboard mark"
	if !mark {
		cmdName = "onboard skip"
	}

	// Validate item type.
	switch itemType {
	case "decision", "note", "file":
	default:
		return fmt.Errorf("%s: invalid type %q: must be one of: decision, note, file", cmdName, itemType)
	}

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	var action string
	if mark {
		action = "covered"
		if err := ks.MarkOnboardingItem(ctx, sessionID, itemType, itemTarget); err != nil {
			return fmt.Errorf("%s: %w", cmdName, err)
		}
	} else {
		action = "skipped"
		if err := ks.SkipOnboardingItem(ctx, sessionID, itemType, itemTarget); err != nil {
			return fmt.Errorf("%s: %w", cmdName, err)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Marked %s %s as %s\n", itemType, itemTarget, action)
	return nil
}

// ── Shared helpers ─────────────────────────────────────────────────

// renderProgressTable displays a progress report using tabwriter.
func renderProgressTable(cmd *cobra.Command, prog *store.OnboardingProgress) {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)

	fmt.Fprintf(w, "Coverage summary:\n\n")
	fmt.Fprintln(w, "ITEM TYPE\tTOTAL\tCOVERED\tSKIPPED\tREMAINING")
	fmt.Fprintln(w, "---------\t-----\t-------\t-------\t--------")

	for _, t := range sortedKeys(prog.ByType) {
		tp := prog.ByType[t]
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\n", t, tp.Total, tp.Covered, tp.Skipped, tp.Remaining)
	}

	fmt.Fprintln(w, "---------\t-----\t-------\t-------\t--------")
	fmt.Fprintf(w, "TOTAL\t%d\t%d\t%d\t%d\n", prog.TotalItems, prog.Covered, prog.Skipped, prog.Remaining)

	_ = w.Flush()
}

// sortedKeys returns the keys of the map sorted lexicographically.
func sortedKeys(m map[string]store.TypeProgress) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple insertion sort for small maps (at most 3-4 types).
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
