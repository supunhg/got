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

// GraphTab displays the commit graph.
type GraphTab struct {
	adapter  *git.ExecAdapter
	repoPath string
	viewport viewport.Model
	loading  bool
	err      error
	content  string
}

// NewGraphTab creates a new Graph tab.
func NewGraphTab(adapter *git.ExecAdapter, repoPath string) *GraphTab {
	return &GraphTab{
		adapter:  adapter,
		repoPath: repoPath,
		viewport: viewport.New(0, 0),
		loading:  true,
	}
}

// Init loads initial data.
func (t *GraphTab) Init() tea.Cmd {
	return t.loadGraph
}

// Update handles messages.
func (t *GraphTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.viewport.Width = msg.Width - 4
		t.viewport.Height = msg.Height - 6
	case graphDataMsg:
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
func (t *GraphTab) View() string {
	if t.loading {
		return theme.Muted.Render("  Loading graph...")
	}
	if t.err != nil {
		return theme.Error.Render(fmt.Sprintf("  Error: %v", t.err))
	}
	return t.viewport.View()
}

// Title returns the tab title.
func (t *GraphTab) Title() string {
	return "Graph"
}

func (t *GraphTab) loadGraph() tea.Msg {
	ctx := context.Background()
	if err := t.adapter.OpenRepository(ctx, t.repoPath); err != nil {
		return graphDataMsg{err: fmt.Errorf("open repo: %w", err)}
	}

	currentBranch, _ := t.adapter.CurrentBranch(ctx)
	nodes, err := t.adapter.GetGraph(ctx, currentBranch, 50)
	if err != nil {
		return graphDataMsg{err: fmt.Errorf("get graph: %w", err)}
	}

	var b strings.Builder
	b.WriteString(theme.Header.Render("Commit Graph"))
	b.WriteString("\n\n")

	if len(nodes) == 0 {
		b.WriteString(theme.Muted.Render("  No commits found"))
		return graphDataMsg{content: b.String()}
	}

	for i, node := range nodes {
		graph := buildGraphLine(i, len(node.Parents))
		sha := node.SHA
		if len(sha) > 8 {
			sha = sha[:8]
		}
		line := graph + " " + theme.CommitSHA.Render(sha) + " " + theme.Item.Render(truncMsg(node.Message, 60))
		if node.Refs != "" {
			line += " " + theme.Success.Render("("+node.Refs+")")
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	return graphDataMsg{content: b.String()}
}

func buildGraphLine(idx, parentCount int) string {
	if parentCount > 1 {
		return theme.GraphEdge.Render("┤") + theme.GraphNode.Render("●")
	}
	if idx == 0 {
		return theme.GraphNode.Render("●")
	}
	return theme.GraphEdge.Render("│") + theme.GraphNode.Render("●")
}

func truncMsg(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

type graphDataMsg struct {
	content string
	err     error
}
