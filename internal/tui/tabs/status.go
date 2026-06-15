// Package tabs provides the tab implementations for the TUI.
package tabs

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/supunhg/got/internal/git"
	"github.com/supunhg/got/internal/tui/theme"
)

// StatusTab displays the Git working tree status.
type StatusTab struct {
	adapter  *git.ExecAdapter
	repoPath string
	viewport viewport.Model
	loading  bool
	err      error
	content  string
}

// NewStatusTab creates a new Status tab.
func NewStatusTab(adapter *git.ExecAdapter, repoPath string) *StatusTab {
	return &StatusTab{
		adapter:  adapter,
		repoPath: repoPath,
		viewport: viewport.New(0, 0),
		loading:  true,
	}
}

// Init loads initial data.
func (t *StatusTab) Init() tea.Cmd {
	return t.loadStatus
}

// Update handles messages.
func (t *StatusTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.viewport.Width = msg.Width - 4
		t.viewport.Height = msg.Height - 6
	case statusDataMsg:
		t.loading = false
		t.err = msg.err
		if msg.err == nil {
			t.content = msg.content
			t.viewport.SetContent(msg.content)
		}
		return t, nil
	}
	var cmd tea.Cmd
	t.viewport, cmd = t.viewport.Update(msg)
	return t, cmd
}

// View renders the tab.
func (t *StatusTab) View() string {
	if t.loading {
		return theme.Muted.Render("  Loading status...")
	}
	if t.err != nil {
		return theme.Error.Render(fmt.Sprintf("  Error: %v", t.err))
	}
	return t.viewport.View()
}

// Title returns the tab title.
func (t *StatusTab) Title() string {
	return "Status"
}

func (t *StatusTab) loadStatus() tea.Msg {
	ctx := context.Background()
	if err := t.adapter.OpenRepository(ctx, t.repoPath); err != nil {
		return statusDataMsg{err: fmt.Errorf("open repo: %w", err)}
	}

	status, err := t.adapter.GetStatus(ctx)
	if err != nil {
		return statusDataMsg{err: fmt.Errorf("get status: %w", err)}
	}

	var b strings.Builder
	b.WriteString(theme.Header.Render("Working Tree Status"))
	b.WriteString("\n\n")

	b.WriteString(theme.Section.Render(fmt.Sprintf("Branch: %s", status.Branch)))
	if status.Clean {
		b.WriteString("  " + theme.Success.Render("clean"))
	} else {
		b.WriteString("  " + theme.Warning.Render("dirty"))
	}
	b.WriteString("\n\n")

	if len(status.Staged) > 0 {
		b.WriteString(theme.Section.Render(fmt.Sprintf("Staged (%d)", len(status.Staged))))
		b.WriteString("\n")
		for _, e := range status.Staged {
			b.WriteString("  " + theme.Item.Render(colorStatus(e.IndexStatus)+" "+e.Path))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(status.Unstaged) > 0 {
		b.WriteString(theme.Section.Render(fmt.Sprintf("Unstaged (%d)", len(status.Unstaged))))
		b.WriteString("\n")
		for _, e := range status.Unstaged {
			b.WriteString("  " + theme.Item.Render(colorStatus(e.WorktreeStatus)+" "+e.Path))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(status.Untracked) > 0 {
		b.WriteString(theme.Section.Render(fmt.Sprintf("Untracked (%d)", len(status.Untracked))))
		b.WriteString("\n")
		for _, p := range status.Untracked {
			b.WriteString("  " + theme.Item.Render("? "+p))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(status.Staged) == 0 && len(status.Unstaged) == 0 && len(status.Untracked) == 0 {
		b.WriteString(theme.Muted.Render("  No changes"))
		b.WriteString("\n")
	}

	return statusDataMsg{content: b.String()}
}

type statusDataMsg struct {
	content string
	err     error
}

func colorStatus(s string) string {
	switch s {
	case "M":
		return theme.Warning.Render("M")
	case "A":
		return theme.Success.Render("A")
	case "D":
		return theme.Error.Render("D")
	case "R":
		return theme.Mag.Render("R")
	case "C":
		return theme.Cya.Render("C")
	case "?":
		return theme.Muted.Render("?")
	default:
		return theme.Muted.Render(s)
	}
}
