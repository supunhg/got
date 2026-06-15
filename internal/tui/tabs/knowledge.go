// Package tabs provides the tab implementations for the TUI.
package tabs

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/supunhg/got/internal/store"
	"github.com/supunhg/got/internal/tui/theme"
)

// KnowledgeTab displays decisions and notes.
type KnowledgeTab struct {
	ks       *store.KnowledgeStore
	viewport viewport.Model
	loading  bool
	err      error
	content  string
}

// NewKnowledgeTab creates a new Knowledge tab.
func NewKnowledgeTab(ks *store.KnowledgeStore) *KnowledgeTab {
	return &KnowledgeTab{
		ks:       ks,
		viewport: viewport.New(0, 0),
		loading:  true,
	}
}

// Init loads initial data.
func (t *KnowledgeTab) Init() tea.Cmd {
	return t.loadKnowledge
}

// Update handles messages.
func (t *KnowledgeTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.viewport.Width = msg.Width - 4
		t.viewport.Height = msg.Height - 6
	case knowledgeDataMsg:
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
func (t *KnowledgeTab) View() string {
	if t.loading {
		return theme.Muted.Render("  Loading knowledge...")
	}
	if t.err != nil {
		return theme.Error.Render(fmt.Sprintf("  Error: %v", t.err))
	}
	return t.viewport.View()
}

// Title returns the tab title.
func (t *KnowledgeTab) Title() string {
	return "Knowledge"
}

func (t *KnowledgeTab) loadKnowledge() tea.Msg {
	if t.ks == nil {
		return knowledgeDataMsg{content: theme.Muted.Render("  Knowledge store not available")}
	}

	ctx := context.Background()

	var b strings.Builder
	b.WriteString(theme.Header.Render("Knowledge Base"))
	b.WriteString("\n\n")

	// Load decisions
	decisions, err := t.ks.ListDecisions(ctx, store.DecisionFilter{Limit: 20})
	if err != nil {
		return knowledgeDataMsg{err: fmt.Errorf("load decisions: %w", err)}
	}

	b.WriteString(theme.Section.Render(fmt.Sprintf("Decisions (%d)", len(decisions))))
	b.WriteString("\n")

	if len(decisions) == 0 {
		b.WriteString(theme.Muted.Render("  No decisions recorded"))
		b.WriteString("\n")
	} else {
		for _, d := range decisions {
			status := theme.Muted.Render(d.Status)
			switch d.Status {
			case "accepted":
				status = theme.Success.Render(d.Status)
			case "proposed":
				status = theme.Warning.Render(d.Status)
			case "deprecated":
				status = theme.Error.Render(d.Status)
			}
			b.WriteString(fmt.Sprintf("  %s %s\n", status, theme.Item.Render(truncMsg(d.Title, 50))))
		}
	}

	b.WriteString("\n")

	// Load notes
	notes, err := t.ks.ListNotes(ctx, store.NoteFilter{Limit: 20})
	if err != nil {
		return knowledgeDataMsg{err: fmt.Errorf("load notes: %w", err)}
	}

	b.WriteString(theme.Section.Render(fmt.Sprintf("Notes (%d)", len(notes))))
	b.WriteString("\n")

	if len(notes) == 0 {
		b.WriteString(theme.Muted.Render("  No notes recorded"))
		b.WriteString("\n")
	} else {
		for _, n := range notes {
			b.WriteString(fmt.Sprintf("  %s\n", theme.Item.Render(truncMsg(n.Message, 60))))
		}
	}

	return knowledgeDataMsg{content: b.String()}
}

type knowledgeDataMsg struct {
	content string
	err     error
}
