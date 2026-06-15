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

// PluginsTab displays installed plugins and their status.
type PluginsTab struct {
	ks       *store.KnowledgeStore
	viewport viewport.Model
	loading  bool
	err      error
	content  string
}

// NewPluginsTab creates a new Plugins tab.
func NewPluginsTab(ks *store.KnowledgeStore) *PluginsTab {
	return &PluginsTab{
		ks:       ks,
		viewport: viewport.New(0, 0),
		loading:  true,
	}
}

// Init loads initial data.
func (t *PluginsTab) Init() tea.Cmd {
	return t.loadPlugins
}

// Update handles messages.
func (t *PluginsTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.viewport.Width = msg.Width - 4
		t.viewport.Height = msg.Height - 6
	case pluginsDataMsg:
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
func (t *PluginsTab) View() string {
	if t.loading {
		return theme.Muted.Render("  Loading plugins...")
	}
	if t.err != nil {
		return theme.Error.Render(fmt.Sprintf("  Error: %v", t.err))
	}
	return t.viewport.View()
}

// Title returns the tab title.
func (t *PluginsTab) Title() string {
	return "Plugins"
}

func (t *PluginsTab) loadPlugins() tea.Msg {
	if t.ks == nil {
		return pluginsDataMsg{
			content: theme.Muted.Render("  GOT not initialized (run 'got init')") + "\n\n" +
				theme.Muted.Render("  Install a plugin with: got plugin install <path>"),
		}
	}

	ctx := context.Background()
	plugins, err := t.ks.ListPlugins(ctx)
	if err != nil {
		return pluginsDataMsg{err: fmt.Errorf("list plugins: %w", err)}
	}

	var b strings.Builder
	b.WriteString(theme.Header.Render("Plugins"))
	b.WriteString("\n\n")

	if len(plugins) == 0 {
		b.WriteString(theme.Muted.Render("  No plugins installed"))
		b.WriteString("\n\n")
		b.WriteString(theme.Muted.Render("  Install a plugin with: got plugin install <path>"))
		return pluginsDataMsg{content: b.String()}
	}

	for _, p := range plugins {
		status := theme.Muted.Render("disabled")
		if p.Enabled {
			status = theme.Success.Render("enabled")
		}

		b.WriteString(theme.Section.Render(p.Name) + " " + status)
		b.WriteString("\n")
		b.WriteString(theme.Item.Render("Version: " + p.Version))
		b.WriteString("\n")
		if p.Description != "" {
			b.WriteString(theme.Item.Render(truncMsg(p.Description, 60)))
			b.WriteString("\n")
		}
		if p.Manifest != nil {
			if len(p.Manifest.Commands) > 0 {
				var cmds []string
				for _, c := range p.Manifest.Commands {
					cmds = append(cmds, c.Name)
				}
				b.WriteString(theme.Item.Render("Commands: " + strings.Join(cmds, ", ")))
				b.WriteString("\n")
			}
			if len(p.Manifest.Hooks) > 0 {
				var hooks []string
				for ev := range p.Manifest.Hooks {
					hooks = append(hooks, ev)
				}
				b.WriteString(theme.Item.Render("Hooks: " + strings.Join(hooks, ", ")))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	return pluginsDataMsg{content: b.String()}
}

type pluginsDataMsg struct {
	content string
	err     error
}
