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

// BranchesTab displays all local and remote branches.
type BranchesTab struct {
	adapter  *git.ExecAdapter
	repoPath string
	viewport viewport.Model
	loading  bool
	err      error
	content  string
}

// NewBranchesTab creates a new Branches tab.
func NewBranchesTab(adapter *git.ExecAdapter, repoPath string) *BranchesTab {
	return &BranchesTab{
		adapter:  adapter,
		repoPath: repoPath,
		viewport: viewport.New(0, 0),
		loading:  true,
	}
}

// Init loads initial data.
func (t *BranchesTab) Init() tea.Cmd {
	return t.loadBranches
}

// Update handles messages.
func (t *BranchesTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.viewport.Width = msg.Width - 4
		t.viewport.Height = msg.Height - 6
	case branchesDataMsg:
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
func (t *BranchesTab) View() string {
	if t.loading {
		return theme.Muted.Render("  Loading branches...")
	}
	if t.err != nil {
		return theme.Error.Render(fmt.Sprintf("  Error: %v", t.err))
	}
	return t.viewport.View()
}

// Title returns the tab title.
func (t *BranchesTab) Title() string {
	return "Branches"
}

func (t *BranchesTab) loadBranches() tea.Msg {
	ctx := context.Background()
	if err := t.adapter.OpenRepository(ctx, t.repoPath); err != nil {
		return branchesDataMsg{err: fmt.Errorf("open repo: %w", err)}
	}

	branches, err := t.adapter.ListBranches(ctx)
	if err != nil {
		return branchesDataMsg{err: fmt.Errorf("list branches: %w", err)}
	}

	currentBranch, _ := t.adapter.CurrentBranch(ctx)

	var b strings.Builder
	b.WriteString(theme.Header.Render("Branches"))
	b.WriteString("\n\n")

	if len(branches) == 0 {
		b.WriteString(theme.Muted.Render("  No branches found"))
		return branchesDataMsg{content: b.String()}
	}

	var local, remote []git.Branch
	for _, br := range branches {
		if br.Remote {
			remote = append(remote, br)
		} else {
			local = append(local, br)
		}
	}

	if len(local) > 0 {
		b.WriteString(theme.Section.Render(fmt.Sprintf("Local (%d)", len(local))))
		b.WriteString("\n")
		for _, br := range local {
			prefix := "  "
			nameStyle := theme.Item
			if br.Name == currentBranch {
				prefix = theme.Selected.Render(" * ")
				nameStyle = theme.BranchCurrent
			}
			b.WriteString(prefix + nameStyle.Render(br.Name))
			if br.Upstream != "" {
				b.WriteString(theme.Muted.Render(" → " + br.Upstream))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(remote) > 0 {
		b.WriteString(theme.Section.Render(fmt.Sprintf("Remote (%d)", len(remote))))
		b.WriteString("\n")
		for _, br := range remote {
			b.WriteString("  " + theme.BranchRemote.Render(br.Name))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return branchesDataMsg{content: b.String()}
}

type branchesDataMsg struct {
	content string
	err     error
}
