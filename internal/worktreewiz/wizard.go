// Package worktreewiz implements the interactive TUI picker for
// `got worktree attach` (spec §14). The picker is a bubbles/list
// view of all known worktrees (main + linked) plus their porcelain
// metadata (label, branch, locked, last-attached time). The user
// can filter, scroll, and confirm a single worktree; the resolved
// path is returned to the CLI which then runs `cd` (or prints
// instructions for the user's shell, since the CLI cannot actually
// change the parent's working directory).
package worktreewiz

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/tui"
)

// Action is the operation the wizard resolved. Exactly one of
// Attach / OpenInEditor is set per Answers; the CLI dispatches on
// Action and reads the relevant field.
type Action string

const (
	ActionNone       Action = ""
	ActionAttach     Action = "attach"
	ActionOpenEditor Action = "open"
)

// Entry is one row in the picker. It bundles the git-side metadata
// (Branch, HEAD, Locked) with the GOT porcelain metadata (Label,
// LastAttachedAt) so the view can render a single tidy line per
// worktree. Path is the absolute filesystem path and is what the
// CLI ultimately uses to `cd` (or to launch the editor).
type Entry struct {
	// Path is the absolute filesystem path of the worktree.
	Path string
	// Branch is the short ref name (e.g. "main"), or "" if HEAD
	// is detached.
	Branch string
	// HEAD is the full commit SHA the worktree is on.
	HEAD string
	// IsMain is true for the primary worktree.
	IsMain bool
	// Locked is true when the worktree is locked against pruning.
	Locked bool
	// Label is the user-friendly alias from .got/worktrees.json.
	// Empty means "no label set; fall back to Path".
	Label string
	// LastAttachedAt is the last time the user attached to this
	// worktree. Used to sort by recency.
	LastAttachedAt time.Time
}

// Title returns the primary display string for the picker row.
// If a Label is set, it is shown first; the branch and a short
// HEAD always follow so two worktrees on the same branch are
// still distinguishable.
func (e Entry) Title() string {
	label := e.Label
	if label == "" {
		label = e.Path
	}
	branch := e.Branch
	if branch == "" {
		branch = "detached"
	}
	short := e.HEAD
	if len(short) > 7 {
		short = short[:7]
	}
	if e.IsMain {
		return fmt.Sprintf("%s  %s  %s  (main)", label, branch, short)
	}
	return fmt.Sprintf("%s  %s  %s", label, branch, short)
}

// Description returns the secondary line shown under Title in the
// picker. It carries the absolute path, the locked marker, and
// the last-attached timestamp (relative if recent, absolute
// otherwise). An empty description is allowed.
func (e Entry) Description() string {
	parts := []string{e.Path}
	if e.Locked {
		parts = append(parts, "🔒 locked")
	}
	if !e.LastAttachedAt.IsZero() {
		parts = append(parts, "last: "+formatRelative(e.LastAttachedAt))
	}
	return fmt.Sprintf("%s", joinComma(parts))
}

// FilterValue is the string the bubbles/list filter matches
// against. We use Title (which already includes the label and
// branch) so typing "main" finds the worktree on main.
func (e Entry) FilterValue() string { return e.Title() }

// Answers is the wizard's output: the user's chosen action and
// the worktree it applies to. The CLI dispatches on Action and
// reads Path.
type Answers struct {
	// Action is the resolved operation.
	Action Action
	// Path is the absolute path of the chosen worktree.
	Path string
	// Label is a copy of the chosen worktree's label (or "" if
	// none was set) so the CLI can update the LastAttachedAt
	// timestamp without a second lookup.
	Label string
}

// PrePopulated carries values the user already supplied via flags
// (only `path` is honored in v0.1: pre-selecting the worktree so
// the picker jumps straight to the confirmation step). All
// fields are optional.
type PrePopulated struct {
	// Path, if set, scrolls the list to the matching worktree.
	Path string
}

// CancelledError is returned by Run when the user quits before
// confirming. Callers can match it via errors.Is or just check
// the message.
var CancelledError = gerr.Validation("worktree picker cancelled")

// Run starts the Bubbletea program for the worktree picker and
// blocks until the user picks a worktree and confirms, or
// cancels. `entries` is the pre-built list to show; the CLI in
// internal/cli/worktree.go builds it from the git adapter plus
// the .got/worktrees.json store. `pre` carries values
// pre-populated from CLI flags. `theme` is the Lip Gloss theme.
func Run(entries []Entry, pre PrePopulated, theme tui.Theme) (Answers, error) {
	theme = theme.Apply()
	m := NewModel(entries, pre, theme)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return Answers{}, fmt.Errorf("worktreewiz: %w", err)
	}
	fm, ok := finalModel.(*Model)
	if !ok {
		return Answers{}, fmt.Errorf("worktreewiz: unexpected model type %T", finalModel)
	}
	if fm.cancelled() {
		return Answers{}, CancelledError
	}
	return fm.answers, nil
}

// joinComma joins strings with ", " (used to keep Description a
// one-liner even with multiple parts).
func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

// formatRelative formats t as a short relative string ("just now",
// "5m ago", "2h ago", "3d ago", or an absolute date for older
// times). We avoid a time-relative library to keep the wizard's
// dependency surface small.
func formatRelative(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}
