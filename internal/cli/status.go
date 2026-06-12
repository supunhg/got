package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/git"
)

func newStatusCmd(deps Deps) *cobra.Command {
	var asJSON bool
	var asShort bool
	cmd := &cobra.Command{
		Use:   "status [path]",
		Short: "Show the working tree status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, deps, args, asJSON, asShort)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable JSON output")
	cmd.Flags().BoolVarP(&asShort, "short", "s", false, "porcelain output (one line per file)")
	return cmd
}

func runStatus(cmd *cobra.Command, deps Deps, args []string, asJSON, asShort bool) error {
	start := "."
	if len(args) > 0 {
		start = args[0]
	}
	workTree, err := deps.Discover(start)
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)
	s, err := a.Status(cmd.Context())
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if out == nil {
		out = deps.Stdout
	}
	switch {
	case asJSON:
		return writeJSON(out, s)
	case asShort:
		return writeStatusShort(out, s)
	default:
		return writeStatusHuman(out, s)
	}
}

func writeStatusHuman(w io.Writer, s git.Status) error {
	if s.Detached {
		fmt.Fprintln(w, "HEAD detached")
	} else if s.Branch != "" {
		fmt.Fprintf(w, "On branch %s\n", s.Branch)
	}
	if s.Upstream != "" {
		switch {
		case s.Ahead > 0 && s.Behind > 0:
			fmt.Fprintf(w, "Your branch and %s have diverged (%d ahead, %d behind).\n", s.Upstream, s.Ahead, s.Behind)
		case s.Ahead > 0:
			fmt.Fprintf(w, "Your branch is ahead of %s by %d commit(s).\n", s.Upstream, s.Ahead)
		case s.Behind > 0:
			fmt.Fprintf(w, "Your branch is behind %s by %d commit(s).\n", s.Upstream, s.Behind)
		default:
			fmt.Fprintf(w, "Your branch is up to date with %s.\n", s.Upstream)
		}
	}
	staged, unstaged, untracked := splitStatus(s.Entries)
	if len(staged) > 0 {
		fmt.Fprintln(w, "\nChanges to be committed:")
		for _, e := range staged {
			fmt.Fprintf(w, "  %s\n", statusLabel(e))
		}
	}
	if len(unstaged) > 0 {
		fmt.Fprintln(w, "\nChanges not staged for commit:")
		for _, e := range unstaged {
			fmt.Fprintf(w, "  %s\n", statusLabel(e))
		}
	}
	if len(untracked) > 0 {
		fmt.Fprintln(w, "\nUntracked files:")
		for _, e := range untracked {
			fmt.Fprintf(w, "  %s\n", e.Path)
		}
	}
	if len(s.Entries) == 0 && s.Upstream == "" && !s.Detached {
		fmt.Fprintln(w, "nothing to commit, working tree clean")
	}
	return nil
}

func writeStatusShort(w io.Writer, s git.Status) error {
	for _, e := range s.Entries {
		if e.IsUntracked {
			fmt.Fprintf(w, "?? %s\n", e.Path)
			continue
		}
		fmt.Fprintf(w, "%s %s\n", e.XY, e.Path)
	}
	return nil
}

// statusLabel renders a one-line human description of an entry.
func statusLabel(e git.StatusEntry) string {
	if e.IsRenamed {
		return fmt.Sprintf("renamed: %s -> %s", e.OriginalPath, e.Path)
	}
	// e.XY is two characters: index (staged), worktree (unstaged).
	switch {
	case e.IsStaged && e.IsUnstaged:
		return fmt.Sprintf("%s %s", e.XY, e.Path)
	case e.IsStaged:
		return fmt.Sprintf("%s- %s", string(e.XY[0]), e.Path)
	case e.IsUnstaged:
		return fmt.Sprintf("-%s %s", string(e.XY[1]), e.Path)
	default:
		return "-- " + e.Path
	}
}

func splitStatus(entries []git.StatusEntry) (staged, unstaged, untracked []git.StatusEntry) {
	for _, e := range entries {
		switch {
		case e.IsUntracked:
			untracked = append(untracked, e)
		case e.IsStaged:
			staged = append(staged, e)
		default:
			unstaged = append(unstaged, e)
		}
	}
	return staged, unstaged, untracked
}

// writeJSON encodes v to w as indented JSON.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
