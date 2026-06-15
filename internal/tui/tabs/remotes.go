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

// RemotesTab displays Git remotes.
type RemotesTab struct {
	adapter  *git.ExecAdapter
	repoPath string
	viewport viewport.Model
	loading  bool
	err      error
	content  string
}

// NewRemotesTab creates a new Remotes tab.
func NewRemotesTab(adapter *git.ExecAdapter, repoPath string) *RemotesTab {
	return &RemotesTab{
		adapter:  adapter,
		repoPath: repoPath,
		viewport: viewport.New(0, 0),
		loading:  true,
	}
}

// Init loads initial data.
func (t *RemotesTab) Init() tea.Cmd {
	return t.loadRemotes
}

// Update handles messages.
func (t *RemotesTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.viewport.Width = msg.Width - 4
		t.viewport.Height = msg.Height - 6
	case remotesDataMsg:
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
func (t *RemotesTab) View() string {
	if t.loading {
		return theme.Muted.Render("  Loading remotes...")
	}
	if t.err != nil {
		return theme.Error.Render(fmt.Sprintf("  Error: %v", t.err))
	}
	return t.viewport.View()
}

// Title returns the tab title.
func (t *RemotesTab) Title() string {
	return "Remotes"
}

func (t *RemotesTab) loadRemotes() tea.Msg {
	ctx := context.Background()
	if err := t.adapter.OpenRepository(ctx, t.repoPath); err != nil {
		return remotesDataMsg{err: fmt.Errorf("open repo: %w", err)}
	}

	remotes, err := t.adapter.GetRemotes(ctx)
	if err != nil {
		return remotesDataMsg{err: fmt.Errorf("list remotes: %w", err)}
	}

	var b strings.Builder
	b.WriteString(theme.Header.Render("Remotes"))
	b.WriteString("\n\n")

	if len(remotes) == 0 {
		b.WriteString(theme.Muted.Render("  No remotes configured"))
		return remotesDataMsg{content: b.String()}
	}

	for _, r := range remotes {
		b.WriteString(theme.Section.Render(r.Name))
		b.WriteString("\n")
		b.WriteString(theme.Item.Render("URL:     " + r.URL))
		b.WriteString("\n")
		if r.PushURL != "" {
			b.WriteString(theme.Item.Render("Push:    " + r.PushURL))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return remotesDataMsg{content: b.String()}
}

type remotesDataMsg struct {
	content string
	err     error
}
