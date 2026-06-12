package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/graph"
	"github.com/got-sh/got/internal/graphwiz"
	"github.com/got-sh/got/internal/tui"
)

// newGraphCmd builds the `got graph` subcommand per spec §9.
//
//	got graph                        TUI pager (when stdout is a TTY)
//	got graph --no-tui               print graph to stdout and exit
//	got graph --dot                  emit Graphviz DOT to stdout
//	got graph -n 500                 cap the number of commits (default 200)
//	got graph --since / --until      date filters
//	got graph --author / --grep      author / commit-message filters
//	got graph --all / --first-parent include all branches (default) or only the
//	                                first-parent chain
//
// The TUI pager uses bubbles/viewport with `/` search and `n/N`
// jumps per spec §9.
func newGraphCmd(d Deps) *cobra.Command {
	var (
		maxCount    int
		since       string
		until       string
		author      string
		grep        string
		allBranches bool
		asDOT       bool
	)
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Render the commit graph",
		Long: `Render the commit graph for the current repository.

By default the command drops into a Bubbletea pager with / search
and n/N jumps (see got-spec.md §9). Pass --no-tui (a global flag)
to print the graph to stdout and exit. Pass --dot to emit a
Graphviz DOT representation suitable for piping to ` + "`dot -Tsvg`" + `.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGraph(cmd, d, git.GraphOpts{
				MaxCount: maxCount,
				Since:    since,
				Until:    until,
				Author:   author,
				Grep:     grep,
				All:      allBranches,
			}, asDOT)
		},
	}
	cmd.Flags().IntVarP(&maxCount, "max-count", "n", 200, "maximum number of commits to show")
	cmd.Flags().StringVar(&since, "since", "", "show commits more recent than a specific date")
	cmd.Flags().StringVar(&until, "until", "", "show commits older than a specific date")
	cmd.Flags().StringVar(&author, "author", "", "filter by author pattern (passed to git log --author)")
	cmd.Flags().StringVar(&grep, "grep", "", "filter by commit message pattern (passed to git log --grep)")
	cmd.Flags().BoolVar(&allBranches, "all", true, "include all branches (passed to git log --all); pass --all=false for first-parent only")
	cmd.Flags().BoolVar(&asDOT, "dot", false, "emit Graphviz DOT instead of an interactive pager")
	return cmd
}

// runGraph is the entry point for `got graph`. It picks between
// three output paths based on flags + TTY:
//
//	--dot            -> Graphviz DOT to stdout
//	--no-tui / non-TTY -> raw styled graph to stdout
//	TTY + no --no-tui -> Bubbletea pager via graphwiz.Run
func runGraph(cmd *cobra.Command, deps Deps, opts git.GraphOpts, asDOT bool) error {
	workTree, err := deps.Discover(".")
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)

	if asDOT {
		dot, err := a.GraphDOT(cmd.Context(), opts)
		if err != nil {
			return err
		}
		out := cmdWriter(cmd, deps)
		if !endsWithNewline(dot) {
			dot += "\n"
		}
		_, err = io.WriteString(out, dot)
		return err
	}

	raw, err := a.GraphASCII(cmd.Context(), opts)
	if err != nil {
		return err
	}

	// Decide between pager and stdout.
	noTUI := false
	if v, err := cmd.Root().PersistentFlags().GetBool("no-tui"); err == nil {
		noTUI = v
	}
	isTTY := deps.IsTerminal != nil && deps.IsTerminal()
	if noTUI || !isTTY {
		out := cmdWriter(cmd, deps)
		_, err := io.WriteString(out, raw)
		if err != nil {
			return err
		}
		if raw != "" && !endsWithNewline(raw) {
			_, err = io.WriteString(out, "\n")
		}
		return err
	}

	// TUI path: style the content, then drop into the pager.
	theme := tui.NewTheme()
	styled := graph.Render(raw, theme)
	if err := deps.RunGraphWizard(cmd.Context(), styled, theme); err != nil {
		// CancelledError is a normal exit; surface as a no-op.
		if err == graphwiz.CancelledError {
			return nil
		}
		return err
	}
	return nil
}

// endsWithNewline reports whether s ends with "\n".
func endsWithNewline(s string) bool {
	return len(s) > 0 && s[len(s)-1] == '\n'
}
