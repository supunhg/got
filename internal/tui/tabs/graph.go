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

const (
	graphPageSize  = 50  // commits per page
	graphMaxBuffer = 200 // max commits to keep in memory
)

// GraphTab displays the commit graph with virtualized rendering.
type GraphTab struct {
	adapter  *git.ExecAdapter
	repoPath string
	viewport viewport.Model
	loading  bool
	err      error

	// Virtualization state
	nodes      []git.GraphNode
	page       int
	totalLoaded int
	allLoaded  bool
	content    string
}

// NewGraphTab creates a new Graph tab.
func NewGraphTab(adapter *git.ExecAdapter, repoPath string) *GraphTab {
	return &GraphTab{
		adapter:  adapter,
		repoPath: repoPath,
		viewport: viewport.New(0, 0),
		loading:  true,
		page:     0,
	}
}

// Init loads initial data.
func (t *GraphTab) Init() tea.Cmd {
	return t.loadGraphPage(0)
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
			// Append new nodes to existing
			t.nodes = append(t.nodes, msg.nodes...)
			t.totalLoaded += len(msg.nodes)
			if len(msg.nodes) < graphPageSize {
				t.allLoaded = true
			}
			t.content = t.renderGraph()
			t.viewport.SetContent(t.content)
		}
		return t, nil
	case graphLoadMoreMsg:
		if !t.allLoaded && !t.loading {
			t.loading = true
			t.page++
			return t, t.loadGraphPage(t.page)
		}
		return t, nil
	}

	// Check if viewport scrolled near bottom → load more
	var cmd tea.Cmd
	t.viewport, cmd = t.viewport.Update(msg)

	// If viewport is near bottom and more data available, trigger load
	if !t.allLoaded && t.viewport.YOffset > 0 {
		totalLines := strings.Count(t.content, "\n")
		visibleBottom := t.viewport.YOffset + t.viewport.Height
		if visibleBottom >= totalLines-5 {
			return t, tea.Batch(cmd, t.triggerLoadMore())
		}
	}

	return t, cmd
}

// View renders the tab.
func (t *GraphTab) View() string {
	if t.loading && len(t.nodes) == 0 {
		return theme.Muted.Render("  Loading graph...")
	}
	if t.err != nil && len(t.nodes) == 0 {
		return theme.Error.Render(fmt.Sprintf("  Error: %v", t.err))
	}
	return t.viewport.View()
}

// Title returns the tab title.
func (t *GraphTab) Title() string {
	return "Graph"
}

func (t *GraphTab) loadGraphPage(page int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if err := t.adapter.OpenRepository(ctx, t.repoPath); err != nil {
			return graphDataMsg{err: fmt.Errorf("open repo: %w", err)}
		}

		currentBranch, _ := t.adapter.CurrentBranch(ctx)
		nodes, err := t.adapter.GetGraph(ctx, currentBranch, graphMaxBuffer)
		if err != nil {
			return graphDataMsg{err: fmt.Errorf("get graph: %w", err)}
		}

		// Slice to get the page we need
		start := page * graphPageSize
		if start >= len(nodes) {
			return graphDataMsg{nodes: nil}
		}
		end := start + graphPageSize
		if end > len(nodes) {
			end = len(nodes)
		}

		return graphDataMsg{nodes: nodes[start:end]}
	}
}

func (t *GraphTab) triggerLoadMore() tea.Cmd {
	return func() tea.Msg {
		return graphLoadMoreMsg{}
	}
}

func (t *GraphTab) renderGraph() string {
	var b strings.Builder
	b.WriteString(theme.Header.Render("Commit Graph"))
	b.WriteString("\n\n")

	if len(t.nodes) == 0 {
		b.WriteString(theme.Muted.Render("  No commits found"))
		return b.String()
	}

	for i, node := range t.nodes {
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

	if !t.allLoaded {
		b.WriteString(theme.Muted.Render("  ↓ scroll for more..."))
		b.WriteString("\n")
	}

	return b.String()
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
	nodes []git.GraphNode
	err   error
}

type graphLoadMoreMsg struct{}
