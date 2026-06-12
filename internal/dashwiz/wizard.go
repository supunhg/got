// Package dashwiz implements the interactive `got tui` dashboard
// (got-spec.md §14, locked-in scope for v0.1). The dashboard has
// five tabs:
//
//  1. Status   — REAL: git.GitAdapter.Status + bubbles/list
//  2. Branches — REAL: git.GitAdapter.Branches + bubbles/table
//  3. Remotes  — READ-ONLY: git.GitAdapter.Remotes + v0.2 banner
//  4. Graph    — READ-ONLY: 20-line got-graph preview + v0.2 banner
//  5. Plugins  — READ-ONLY: internal/plugin/discover + v0.2 banner
//
// Spec §14 is explicit: Status and Branches are real interactive
// tabs (they prove the Bubbletea integration on day one). The
// other three are read-only previews backed by real adapter /
// discovery calls so the layout, Bubbletea plumbing, and adapter
// integration are all exercised end-to-end.
package dashwiz

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/plugin"
	"github.com/got-sh/got/internal/tui"
)

// Inputs is everything the dashboard needs to render its tabs. The
// CLI builds this from the real adapter / discovery calls before
// driving the model; tests can construct it directly.
type Inputs struct {
	// WorkTree is the absolute path of the work tree. Used for
	// display in the header line.
	WorkTree string
	// Status is the result of `git status`. Required for the
	// Status tab; an empty value renders the tab as "no data".
	Status git.Status
	// Branches is the list of local branches. Required for the
	// Branches tab; an empty slice renders "no branches".
	Branches []git.Branch
	// Remotes is the result of `git remote`. Used by the
	// Remotes tab's read-only list.
	Remotes []git.Remote
	// GraphPreview is a pre-rendered ASCII graph (typically the
	// first ~20 lines of `got graph --no-tui`). Used by the
	// Graph tab.
	GraphPreview string
	// Plugins is the list of discovered plugins (path + repo
	// dirs). Used by the Plugins tab.
	Plugins []plugin.DiscoveredPlugin
}

// CancelledError is returned by Run when the user quits before
// interacting with any tab. Callers can match it via errors.Is.
var CancelledError = gerr.Validation("dashboard closed")

// Run starts the Bubbletea program for the dashboard and blocks
// until the user quits (q, esc, ctrl+c). `inputs` is the snapshot
// of repo state the dashboard renders. `theme` is the Lip Gloss
// theme.
//
// The provided context is honoured by bubbletea: if ctx is
// cancelled the program returns.
func Run(ctx context.Context, inputs Inputs, theme tui.Theme) error {
	theme = theme.Apply()
	m := NewModel(inputs, theme)
	p := tea.NewProgram(m, tea.WithContext(ctx))
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("dashwiz: %w", err)
	}
	_ = finalModel
	return nil
}
