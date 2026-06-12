package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/tui"
	"github.com/got-sh/got/internal/worktree"
	"github.com/got-sh/got/internal/worktreewiz"
)

// newWorktreeCmd builds the `got worktree` subcommand tree per
// spec §14:
//
//	got worktree                      list (default RunE)
//	got worktree list [--json]        list with --json
//	got worktree add <path> [flags]   create a new worktree
//	got worktree remove <path> [-f]   remove a worktree
//	got worktree lock <path> [-r]     lock against pruning
//	got worktree unlock <path>        unlock
//	got worktree prune                remove dead bookkeeping
//	got worktree attach [--path P]    interactive TUI picker
//
// The list subcommand is duplicated at the parent level (the
// default RunE prints the same table) so `got worktree` with no
// args still works and `got worktree list` is the explicit
// form.
func newWorktreeCmd(d Deps) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "worktree",
		Short: "Manage Git worktrees (linked working trees)",
		Long: `Manage Git worktrees (linked working trees) per got-spec.md §14.

A worktree is a second working tree attached to the same .git/
database, so you can have multiple branches checked out at once.
got worktree wraps git worktree add/list/remove/lock/unlock/prune
with a small porcelain layer that records friendly labels and
last-attached timestamps in ` + "`.got/worktrees.json`" + `.

When stdout is a TTY, ` + "`got worktree attach`" + ` opens an
interactive picker over every known worktree; the resolved path
is printed so you can ` + "`cd`" + ` into it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWorktreeList(cmd.Context(), cmd, d, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable JSON output")

	cmd.AddCommand(newWorktreeListCmd(d))
	cmd.AddCommand(newWorktreeAddCmd(d))
	cmd.AddCommand(newWorktreeRemoveCmd(d))
	cmd.AddCommand(newWorktreeLockCmd(d))
	cmd.AddCommand(newWorktreeUnlockCmd(d))
	cmd.AddCommand(newWorktreePruneCmd(d))
	cmd.AddCommand(newWorktreeAttachCmd(d))
	return cmd
}

// newWorktreeListCmd builds `got worktree list [--json]`.
func newWorktreeListCmd(d Deps) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List known worktrees",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWorktreeList(cmd.Context(), cmd, d, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable JSON output")
	return cmd
}

// newWorktreeAddCmd builds `got worktree add <path> [--branch B]
// [--commit S] [--detach] [--force]`.
func newWorktreeAddCmd(d Deps) *cobra.Command {
	var (
		branch string
		commit string
		detach bool
		force  bool
	)
	cmd := &cobra.Command{
		Use:   "add <path>",
		Short: "Create a new worktree at <path>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorktreeAdd(cmd.Context(), cmd, d, args[0], git.WorktreeAddOpts{
				Branch: branch,
				Commit: commit,
				Detach: detach,
				Force:  force,
			})
		},
	}
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "branch to check out (created if it doesn't exist when --commit is set)")
	cmd.Flags().StringVar(&commit, "commit", "", "commit/SHA to base the worktree on (defaults to HEAD)")
	cmd.Flags().BoolVar(&detach, "detach", false, "create a detached HEAD worktree (no branch)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "create the worktree even if <path> already exists (must be empty)")
	return cmd
}

// newWorktreeRemoveCmd builds `got worktree remove <path> [-f]`.
func newWorktreeRemoveCmd(d Deps) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "remove <path>",
		Short: "Remove a worktree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorktreeRemove(cmd.Context(), cmd, d, args[0], force)
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "remove even if the worktree has uncommitted changes")
	return cmd
}

// newWorktreeLockCmd builds `got worktree lock <path> [-r reason]`.
func newWorktreeLockCmd(d Deps) *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "lock <path>",
		Short: "Lock a worktree against pruning",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorktreeLock(cmd.Context(), cmd, d, args[0], reason)
		},
	}
	cmd.Flags().StringVarP(&reason, "reason", "r", "", "human-readable reason for the lock (recorded in git metadata)")
	return cmd
}

// newWorktreeUnlockCmd builds `got worktree unlock <path>`.
func newWorktreeUnlockCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unlock <path>",
		Short: "Unlock a worktree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorktreeUnlock(cmd.Context(), cmd, d, args[0])
		},
	}
	return cmd
}

// newWorktreePruneCmd builds `got worktree prune`.
func newWorktreePruneCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove worktree bookkeeping for deleted worktrees",
		Long: `Remove worktree bookkeeping for deleted worktrees.

If you delete a worktree directory by hand (e.g. ` + "`rm -rf`" + `)
without running ` + "`git worktree remove`" + `, the administrative
files in ` + "`.git/worktrees/<id>/`" + ` linger. This command
removes that bookkeeping so the next ` + "`git worktree list`" + `
is clean.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWorktreePrune(cmd.Context(), cmd, d)
		},
	}
	return cmd
}

// newWorktreeAttachCmd builds `got worktree attach [--path P]`.
func newWorktreeAttachCmd(d Deps) *cobra.Command {
	var pathFlag string
	cmd := &cobra.Command{
		Use:   "attach",
		Short: "Pick a worktree to attach to (interactive TUI)",
		Long: `Open the worktree picker (interactive TUI when stdout is a TTY).

The picker shows every known worktree with its branch, short
SHA, label, lock state, and last-attached timestamp. Use
` + "`/`" + ` to filter, ` + "`a`" + ` to attach (prints the path so
you can ` + "`cd`" + ` into it), ` + "`e`" + ` to open in $EDITOR,
` + "`q`" + ` to quit.

In non-TTY mode (or with ` + "`--no-tui`" + `) the picker is
skipped and the command prints a plain list.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWorktreeAttach(cmd.Context(), cmd, d, pathFlag)
		},
	}
	cmd.Flags().StringVar(&pathFlag, "path", "", "pre-select the worktree at this path in the picker")
	return cmd
}

// runWorktreeList is the entry point for the list path. It
// reads the git worktree list, merges the .got/worktrees.json
// porcelain records, and renders a table or JSON.
func runWorktreeList(ctx context.Context, cmd *cobra.Command, d Deps, asJSON bool) error {
	workTree, err := d.Discover(".")
	if err != nil {
		return err
	}
	a := depsAdapter(d, workTree)
	wts, err := a.WorktreeList(ctx)
	if err != nil {
		return err
	}
	records, err := readWorktreeRecords(workTree)
	if err != nil {
		return err
	}
	merged := mergeWorktreePorcelain(wts, records)
	if asJSON {
		return writeJSON(cmdWriter(cmd, d), merged)
	}
	return writeWorktreeTable(cmdWriter(cmd, d), merged)
}

// runWorktreeAdd wraps WorktreeAdd and, on success, writes (or
// updates) the corresponding porcelain record so the picker
// shows it on the next `got worktree list`.
func runWorktreeAdd(ctx context.Context, cmd *cobra.Command, d Deps, path string, opts git.WorktreeAddOpts) error {
	workTree, err := d.Discover(".")
	if err != nil {
		return err
	}
	// Resolve the user-supplied path to an absolute one before
	// passing it to the adapter, so the porcelain record
	// (keyed on the same path) matches what git itself
	// reports.
	abs := absPath(workTree, path)
	a := depsAdapter(d, workTree)
	if err := a.WorktreeAdd(ctx, abs, opts); err != nil {
		return err
	}
	store := worktree.NewStore(worktreePath(workTree))
	_ = store.Upsert(abs, func(existing *worktree.WorktreeRecord, found bool) worktree.WorktreeRecord {
		if !found {
			existing.Path = abs
		}
		if opts.Branch != "" {
			existing.Branch = opts.Branch
		} else if opts.Detach {
			existing.Branch = ""
		}
		return *existing
	})
	_, _ = fmt.Fprintf(d.Stdout, "[got] created worktree at %s\n", abs)
	return nil
}

// runWorktreeRemove wraps WorktreeRemove and drops the porcelain
// record so the picker stops showing the deleted worktree.
// Refuses to remove the main worktree.
func runWorktreeRemove(ctx context.Context, cmd *cobra.Command, d Deps, path string, force bool) error {
	workTree, err := d.Discover(".")
	if err != nil {
		return err
	}
	a := depsAdapter(d, workTree)
	wts, err := a.WorktreeList(ctx)
	if err != nil {
		return err
	}
	abs := absPath(workTree, path)
	for _, w := range wts {
		if w.Path == abs && w.IsMain {
			return gerr.Validation(fmt.Sprintf("refusing to remove the main worktree (%s); switch to another worktree first", abs))
		}
	}
	if err := a.WorktreeRemove(ctx, abs, force); err != nil {
		return err
	}
	// Drop the porcelain record.
	store := worktree.NewStore(worktreePath(workTree))
	_, _ = store.Remove(abs)
	if force {
		_, _ = fmt.Fprintf(d.Stdout, "[got] force-removed worktree at %s\n", abs)
	} else {
		_, _ = fmt.Fprintf(d.Stdout, "[got] removed worktree at %s\n", abs)
	}
	return nil
}

// runWorktreeLock wraps WorktreeLock.
func runWorktreeLock(ctx context.Context, cmd *cobra.Command, d Deps, path, reason string) error {
	workTree, err := d.Discover(".")
	if err != nil {
		return err
	}
	a := depsAdapter(d, workTree)
	abs := absPath(workTree, path)
	if err := a.WorktreeLock(ctx, abs, reason); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(d.Stdout, "[got] locked worktree at %s\n", abs)
	return nil
}

// runWorktreeUnlock wraps WorktreeUnlock.
func runWorktreeUnlock(ctx context.Context, cmd *cobra.Command, d Deps, path string) error {
	workTree, err := d.Discover(".")
	if err != nil {
		return err
	}
	a := depsAdapter(d, workTree)
	abs := absPath(workTree, path)
	if err := a.WorktreeUnlock(ctx, abs); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(d.Stdout, "[got] unlocked worktree at %s\n", abs)
	return nil
}

// runWorktreePrune wraps WorktreePrune.
func runWorktreePrune(ctx context.Context, cmd *cobra.Command, d Deps) error {
	workTree, err := d.Discover(".")
	if err != nil {
		return err
	}
	a := depsAdapter(d, workTree)
	if err := a.WorktreePrune(ctx); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(d.Stdout, "[got] pruned worktree bookkeeping")
	return nil
}

// runWorktreeAttach drives the worktree picker (when TTY) or
// prints a plain list (when non-TTY / --no-tui). The picker
// resolves a single path; we then update the porcelain record's
// LastAttachedAt and print a hint to `cd` into the worktree.
func runWorktreeAttach(ctx context.Context, cmd *cobra.Command, d Deps, pathFlag string) error {
	workTree, err := d.Discover(".")
	if err != nil {
		return err
	}
	a := depsAdapter(d, workTree)
	wts, err := a.WorktreeList(ctx)
	if err != nil {
		return err
	}
	records, err := readWorktreeRecords(workTree)
	if err != nil {
		return err
	}
	merged := mergeWorktreePorcelain(wts, records)
	entries := entriesForPicker(merged)

	// TUI path: when stdout is a TTY and --no-tui is not set.
	noTUI := false
	if v, err := cmd.Root().PersistentFlags().GetBool("no-tui"); err == nil {
		noTUI = v
	}
	isTTY := d.IsTerminal != nil && d.IsTerminal()
	if !noTUI && isTTY && d.RunWorktreeWizard != nil {
		ans, err := d.RunWorktreeWizard(entries, worktreewiz.PrePopulated{Path: pathFlag}, tui.NewTheme())
		if err != nil {
			return err
		}
		switch ans.Action {
		case worktreewiz.ActionAttach:
			// Update LastAttachedAt.
			store := worktree.NewStore(worktreePath(workTree))
			_ = store.Upsert(ans.Path, func(existing *worktree.WorktreeRecord, found bool) worktree.WorktreeRecord {
				if !found {
					existing.Path = ans.Path
				}
				if ans.Label != "" {
					existing.Label = ans.Label
				}
				existing.LastAttachedAt = time.Now().UTC()
				return *existing
			})
			_, _ = fmt.Fprintf(d.Stdout, "[got] attached to %s\n", ans.Path)
			_, _ = fmt.Fprintf(d.Stdout, "       (hint: cd %s in your shell to switch the working tree)\n", ans.Path)
			return nil
		case worktreewiz.ActionOpenEditor:
			_, _ = fmt.Fprintf(d.Stdout, "[got] would open editor at %s (not yet wired; v0.1 just prints the path)\n", ans.Path)
			return nil
		}
		// ActionNone shouldn't happen because the wizard
		// always resolves to Attach/OpenEditor or returns
		// CancelledError; fall through to the plain list
		// for safety.
	}
	// Non-TTY fallback: print the same rows the picker would
	// show, so scripts and CI get a useful list.
	for _, e := range entries {
		label := e.Label
		if label == "" {
			label = e.Path
		}
		_, _ = fmt.Fprintf(d.Stdout, "%s\n", label)
	}
	return nil
}

// writeWorktreeTable renders the merged worktree list as a
// human-readable table. Path is the absolute path; the main
// worktree is marked with "(main)"; locked entries carry a
// marker; and the label (from .got/worktrees.json) is shown
// when set.
func writeWorktreeTable(w io.Writer, wts []mergedWorktree) error {
	if len(wts) == 0 {
		_, err := fmt.Fprintln(w, "(no worktrees)")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PATH\tBRANCH\tHEAD\tLABEL\tLOCKED\tPRUNABLE")
	for _, w := range wts {
		branch := w.Branch
		if branch == "" {
			branch = "(detached)"
		}
		if w.IsMain {
			branch += " (main)"
		}
		locked := ""
		if w.Locked {
			locked = "yes"
			if w.LockReason != "" {
				locked += ": " + w.LockReason
			}
		}
		prunable := ""
		if w.Prunable {
			prunable = "yes"
		}
		short := w.HEAD
		if len(short) > 7 {
			short = short[:7]
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", w.Path, branch, short, w.Label, locked, prunable)
	}
	return tw.Flush()
}

// mergedWorktree is a single worktree as seen by the CLI: the
// git-side metadata (Path/Branch/HEAD/IsMain/Locked/Prunable)
// merged with the .got/worktrees.json porcelain (Label/Editor/
// LastAttachedAt/Notes). The merge is keyed on Path.
type mergedWorktree struct {
	git.Worktree
	// Label is the user-friendly alias from the porcelain
	// tracker. git.Worktree has no Label field, so we keep it
	// on the outer wrapper.
	Label string `json:"label,omitempty"`
	// LastAttachedAt, Notes, Editor are likewise porcelain
	// fields layered on top of the embedded Worktree.
	LastAttachedAt time.Time `json:"lastAttachedAt,omitempty"`
	Notes          string    `json:"notes,omitempty"`
	Editor         string    `json:"editor,omitempty"`
}

// mergeWorktreePorcelain joins the git worktree list (the
// authoritative source for what is checked out) with the
// .got/worktrees.json records (the authoritative source for
// GOT-specific metadata). When both are present for the same
// path, the porcelain fields are layered on top. The result
// is sorted with the main worktree first, then by path.
func mergeWorktreePorcelain(wts []git.Worktree, records []worktree.WorktreeRecord) []mergedWorktree {
	byPath := make(map[string]worktree.WorktreeRecord, len(records))
	for _, r := range records {
		byPath[r.Path] = r
	}
	out := make([]mergedWorktree, 0, len(wts))
	for _, w := range wts {
		m := mergedWorktree{Worktree: w}
		if r, ok := byPath[w.Path]; ok {
			m.Label = r.Label
			m.LastAttachedAt = r.LastAttachedAt
			m.Notes = r.Notes
			m.Editor = r.Editor
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsMain != out[j].IsMain {
			return out[i].IsMain
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// entriesForPicker converts the merged worktree list to the
// wizard's Entry type. The picker doesn't need every column
// of the table, just Path + a few display fields.
func entriesForPicker(wts []mergedWorktree) []worktreewiz.Entry {
	out := make([]worktreewiz.Entry, 0, len(wts))
	for _, w := range wts {
		out = append(out, worktreewiz.Entry{
			Path:           w.Path,
			Branch:         w.Branch,
			HEAD:           w.HEAD,
			IsMain:         w.IsMain,
			Locked:         w.Locked,
			Label:          w.Label,
			LastAttachedAt: w.LastAttachedAt,
		})
	}
	return out
}

// depsAdapter returns the git adapter for the given work tree.
func depsAdapter(d Deps, workTree string) git.Adapter {
	if d.AdapterFor != nil {
		return d.AdapterFor(workTree)
	}
	return git.NewExecAdapter(workTree)
}

// worktreePath returns the absolute path to .got/worktrees.json
// for the given work tree. Empty work tree yields "".
func worktreePath(workTree string) string {
	if workTree == "" {
		return ""
	}
	return filepath.Clean(filepath.Join(workTree, ".got", worktree.FileName))
}

// readWorktreeRecords reads the porcelain tracker. A missing
// file is not an error.
func readWorktreeRecords(workTree string) ([]worktree.WorktreeRecord, error) {
	store := worktree.NewStore(worktreePath(workTree))
	return store.Read()
}

// absPath returns an absolute path for a user-supplied path,
// resolving relative paths against the work tree root. If
// the input is already absolute it is returned verbatim. The
// result is cleaned (no trailing slashes, no ./ components)
// so it matches the path `git worktree list` reports.
func absPath(workTree, p string) string {
	if p == "" {
		return p
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(workTree, p))
}
