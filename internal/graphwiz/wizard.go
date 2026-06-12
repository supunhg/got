// graphwiz/wizard.go: public surface of the got graph pager.
//
// Unlike the other wizards, the graph pager is a viewer — it
// doesn't return answers, it just blocks until the user quits.
// The Run entry point drives the model in model.go via
// tea.Program.
//
// One caller hits this file: internal/cli/graph.go, which calls
// Run when stdout is a TTY and --no-tui is not set. The CLI
// builds the styled content via internal/graph.Render and passes
// the result here.

package graphwiz

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/tui"
)

// CancelledError is returned by Run when the user quits before
// the pager would otherwise terminate. In practice, "quitting" is
// the only way out, so callers can usually treat any non-nil
// error from Run as the user dismissing the view.
var CancelledError = gerr.Validation("graph pager cancelled")

// Run starts the Bubbletea program for the graph pager and blocks
// until the user quits. `content` is the fully styled graph text
// (one commit per line, with lipgloss colour codes already
// applied by internal/graph.Render). `theme` is the Lip Gloss
// theme.
//
// The provided context is honoured by bubbletea: if ctx is
// cancelled the program will return.
func Run(ctx context.Context, content string, theme tui.Theme) error {
	m := NewModel(content, theme)
	p := tea.NewProgram(m, tea.WithContext(ctx))
	finalModel, err := p.Run()
	if err != nil {
		// bubbletea returns tea.ErrProgramKilled when the context is
		// cancelled; surface that as CancelledError.
		return fmt.Errorf("graphwiz: %w", err)
	}
	_ = finalModel
	return nil
}
