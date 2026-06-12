package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/dashwiz"
	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/graph"
	"github.com/got-sh/got/internal/tui"
)

// newTUICmd builds the `got tui` dashboard subcommand. The
// default RunE drives the Bubbletea dashboard via Deps.RunDashboardWizard;
// in non-TTY mode (or with --no-tui) it prints a non-interactive
// summary of the same data so CI / scripts can still see the
// snapshot.
//
// Per spec §14 the v0.1 dashboard ships with Status + Branches
// tabs as real interactive surfaces, and Remotes / Graph / Plugins
// as read-only previews backed by the real adapter / discovery
// calls (each carrying a visible "Coming in v0.2" banner).
func newTUICmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Open the GOT dashboard (interactive TUI)",
		Long: `Open the GOT dashboard (got-spec.md §14, locked-in v0.1 scope).

The dashboard has five tabs:

  1. Status   — working tree state (real, backed by git status)
  2. Branches — local branches + current/upstream (real, backed
                by git branch)
  3. Remotes  — read-only list (mutations land in v0.2)
  4. Graph    — read-only 20-line preview (interactive renderer
                in v0.2)
  5. Plugins  — read-only list of discovered plugins (interactive
                loader in v0.2)

Keys:  ←/→ or 1-5 switch tabs, q / esc / ctrl+c quit.

In non-TTY mode (or with ` + "`--no-tui`" + `) the dashboard is
skipped and a plain summary is printed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDashboard(cmd.Context(), cmd, d)
		},
	}
	return cmd
}

// runDashboard is the entry point for the tui subcommand. It
// resolves the repo, fetches the Inputs snapshot from the adapter
// and plugin discovery, and either drives the Bubbletea model
// (TTY) or prints a non-interactive summary (non-TTY / --no-tui).
func runDashboard(ctx context.Context, cmd *cobra.Command, d Deps) error {
	logger := loggerFor(d)
	logger.Info("dashboard starting")
	workTree, err := d.Discover(".")
	if err != nil {
		return err
	}
	inputs, err := buildDashboardInputs(ctx, d, workTree)
	if err != nil {
		logger.Warn("dashboard inputs failed", "err", err.Error())
		return err
	}
	// Honor the global --no-tui flag.
	noTUI := false
	if v, flagErr := cmd.Root().PersistentFlags().GetBool("no-tui"); flagErr == nil {
		noTUI = v
	}
	isTTY := d.IsTerminal != nil && d.IsTerminal()
	if !noTUI && isTTY && d.RunDashboardWizard != nil {
		logger.Info("dashboard launching tui")
		return d.RunDashboardWizard(ctx, inputs, tui.NewTheme())
	}
	// Non-TTY / --no-tui fallback: print a compact text summary
	// of the same data so CI / scripts can see the snapshot.
	logger.Info("dashboard printing non-tty summary")
	return printDashboardSummary(cmd, d, inputs)
}

// buildDashboardInputs assembles the Inputs snapshot from the
// real adapter calls and plugin discovery. The Status call uses
// the user's work tree; Branches / Remotes / Graph / Plugins are
// all read-only and safe to call up front. Errors from any one
// of these are non-fatal: the dashboard renders a placeholder for
// the failed tab and the others work normally.
func buildDashboardInputs(ctx context.Context, d Deps, workTree string) (dashwiz.Inputs, error) {
	inputs := dashwiz.Inputs{WorkTree: workTree}
	a := depsAdapter(d, workTree)
	if s, err := a.Status(ctx); err == nil {
		inputs.Status = s
	}
	if br, err := a.Branches(ctx); err == nil {
		inputs.Branches = br
	}
	if rem, err := a.Remotes(ctx); err == nil {
		inputs.Remotes = rem
	}
	// Graph preview: 20 lines of `git log --graph --oneline`.
	// We do this via the same code path `got graph --no-tui`
	// uses so the preview is always consistent.
	graphASCII, gerr := a.GraphASCII(ctx, git.GraphOpts{All: true, MaxCount: 20})
	// graph.Render("") returns "\n" (not ""), so an empty raw
	// graph would slip past the "" check in
	// printDashboardSummary and print a blank line. We only
	// render when the raw output is non-empty so the "no graph"
	// branch in the summary takes over.
	if gerr == nil && graphASCII != "" {
		inputs.GraphPreview = graph.Render(graphASCII, tui.NewTheme())
	}
	// Plugin discovery: best-effort. A discovery failure (e.g. no
	// PATH) is not fatal; the Plugins tab will show "(no plugins
	// discovered)".
	if d.DiscoverPlugins != nil {
		if pl, perr := d.DiscoverPlugins(ctx); perr == nil {
			inputs.Plugins = pl
		}
	}
	return inputs, nil
}

// printDashboardSummary renders a non-interactive text summary of
// the dashboard inputs. It is intentionally not a TUI; the goal
// is a stable, scriptable text artifact for CI / non-TTY runs.
func printDashboardSummary(cmd *cobra.Command, d Deps, inputs dashwiz.Inputs) error {
	out := cmdWriter(cmd, d)
	fmt.Fprintln(out, "GOT dashboard (non-TTY summary)")
	fmt.Fprintf(out, "Work tree: %s\n", inputs.WorkTree)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Status:")
	if inputs.Status.Branch != "" {
		fmt.Fprintf(out, "  on branch %s\n", inputs.Status.Branch)
	} else if inputs.Status.Detached {
		fmt.Fprintln(out, "  detached HEAD")
	}
	fmt.Fprintf(out, "  %d entry(ies)\n", len(inputs.Status.Entries))
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Branches:")
	fmt.Fprintf(out, "  %d local\n", len(inputs.Branches))
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Remotes:")
	fmt.Fprintf(out, "  %d configured\n", len(inputs.Remotes))
	for _, r := range inputs.Remotes {
		fmt.Fprintf(out, "    %s\t%s\n", r.Name, r.FetchURL)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Graph preview:")
	if inputs.GraphPreview == "" {
		fmt.Fprintln(out, "  (no graph)")
	} else {
		for _, line := range splitLines(inputs.GraphPreview, 20) {
			fmt.Fprintf(out, "  %s\n", line)
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Plugins:")
	if len(inputs.Plugins) == 0 {
		fmt.Fprintln(out, "  (none discovered)")
	} else {
		for _, p := range inputs.Plugins {
			fmt.Fprintf(out, "  %s %s  [%s]  %d commands\n", p.Name, p.Version, p.Source, len(p.Commands))
		}
	}
	return nil
}

// splitLines splits s by newlines and caps the result at max. It
// is a tiny helper kept here so the test file can pin its
// behaviour.
func splitLines(s string, max int) []string {
	out := make([]string, 0, max)
	for i, line := range splitNL(s) {
		if i >= max {
			break
		}
		out = append(out, line)
	}
	return out
}

func splitNL(s string) []string {
	out := []string{}
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
