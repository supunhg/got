// Package branchwiz implements the interactive `got branch` wizard
// (got-spec.md §13). The wizard drives a Bubbletea model through a
// menu-driven flow: list branches, create a new branch, checkout an
// existing branch, or delete a branch. The CLI in internal/cli/branch.go
// uses the wizard when stdout is a TTY and falls back to non-interactive
// defaults (driven by flags) otherwise.
//
// The wizard is intentionally small. It does not implement the rename
// (`got branch move`) or remote-tracking flows; those are still
// plain-CLI in v0.1.
package branchwiz

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/tui"
)

// Action is the operation the wizard resolved. Exactly one field is set
// per Answers value; the CLI dispatches on it.
type Action string

const (
	ActionNone     Action = ""
	ActionCreate   Action = "create"
	ActionCheckout Action = "checkout"
	ActionDelete   Action = "delete"
)

// Answers is the wizard's output: the user's chosen action and any
// fields the action needs. The CLI in internal/cli/branch.go dispatches
// on Action and reads the relevant field.
type Answers struct {
	// Action is the resolved operation.
	Action Action
	// Name is the branch name (used by all three actions).
	Name string
	// StartPoint is the commit/branch to branch from (create only).
	// Empty means "branch from HEAD".
	StartPoint string
	// Force is true for delete when the user confirmed a force
	// delete (git branch -D, not -d).
	Force bool
}

// PrePopulated carries values the user already supplied via flags. The
// wizard uses these to skip the corresponding input screens. Each
// field is optional; empty / false means "ask the user".
type PrePopulated struct {
	// Name from --name / positional <name>.
	Name string
	// StartPoint from --from.
	StartPoint string
	// Force from --force (delete only).
	Force bool
	// Action pins the wizard to a specific action so the menu is
	// skipped. Used by the non-interactive CLI path to drive the
	// wizard directly from flag values.
	Action Action
}

// Defaults returns a zero Answers value. The wizard always fills in the
// relevant field, so this is mostly for tests.
func Defaults() Answers {
	return Answers{}
}

// CancelledError is returned by Run when the user quits before
// finishing the wizard. Callers can match it via errors.Is or just
// check the message.
var CancelledError = gerr.Validation("branch wizard cancelled")

// Run starts the Bubbletea program for the branch wizard and blocks
// until the user picks an action and confirms, or cancels. `branches`
// is the list of currently-known local branches (used by the
// checkout / delete screens). `pre` carries values pre-populated from
// CLI flags. `theme` is the Lip Gloss theme.
//
// Returns CancelledError if the user quits without confirming.
func Run(branches []git.Branch, pre PrePopulated, theme tui.Theme) (Answers, error) {
	theme = theme.Apply()
	m := NewModel(branches, pre, theme)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return Answers{}, fmt.Errorf("branchwiz: %w", err)
	}
	fm, ok := finalModel.(*Model)
	if !ok {
		return Answers{}, fmt.Errorf("branchwiz: unexpected model type %T", finalModel)
	}
	if fm.cancelled() {
		return Answers{}, CancelledError
	}
	return fm.answers, nil
}
