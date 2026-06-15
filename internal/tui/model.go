// Package tui provides the interactive terminal dashboard for GOT.
package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/supunhg/got/internal/events"
	"github.com/supunhg/got/internal/git"
	"github.com/supunhg/got/internal/store"
	"github.com/supunhg/got/internal/tui/tabs"
	"github.com/supunhg/got/internal/tui/theme"
)

// KeyMap defines all keybindings for the TUI.
type KeyMap struct {
	Quit    key.Binding
	Refresh key.Binding
	Help    key.Binding
	NextTab key.Binding
	PrevTab key.Binding
	Up      key.Binding
	Down    key.Binding
	Enter   key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Help: key.NewBinding(
			key.WithKeys("?", "h"),
			key.WithHelp("?", "help"),
		),
		NextTab: key.NewBinding(
			key.WithKeys("l", "tab"),
			key.WithHelp("l/tab", "next tab"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("h", "shift+tab"),
			key.WithHelp("h", "prev tab"),
		),
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/↑", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/↓", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
	}
}

func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Quit, k.NextTab, k.PrevTab, k.Up, k.Down, k.Enter, k.Refresh, k.Help}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Quit, k.Refresh, k.Help},
		{k.NextTab, k.PrevTab},
		{k.Up, k.Down, k.Enter},
	}
}

// Model is the top-level Bubbletea model for the GOT dashboard.
type Model struct {
	activeTab int
	width     int
	height    int
	showHelp  bool
	repoPath  string
	repoFound bool

	tabModels [theme.TabCount]tea.Model
	keys      KeyMap
}

// NewModel creates and initializes the dashboard model.
func NewModel() Model {
	repoPath, repoErr := findRepoRoot()
	adapter := git.NewExecAdapter(nil)
	if repoErr == nil && repoPath != "" {
		_ = adapter.OpenRepository(context.Background(), repoPath)
	}

	ks := openKS()

	m := Model{
		activeTab: theme.TabStatus,
		repoPath:  repoPath,
		repoFound: repoErr == nil,
		keys:      DefaultKeyMap(),
	}

	m.tabModels[theme.TabStatus] = tabs.NewStatusTab(adapter, repoPath)
	m.tabModels[theme.TabBranches] = tabs.NewBranchesTab(adapter, repoPath)
	m.tabModels[theme.TabRemotes] = tabs.NewRemotesTab(adapter, repoPath)
	m.tabModels[theme.TabGraph] = tabs.NewGraphTab(adapter, repoPath)
	m.tabModels[theme.TabPlugins] = tabs.NewPluginsTab(ks)
	m.tabModels[theme.TabKnowledge] = tabs.NewKnowledgeTab(ks)

	return m
}

// ── Helpers (self-contained, no cli import) ─────────────────────────

func openKS() *store.KnowledgeStore {
	gotDir, err := findGotDir()
	if err != nil {
		return nil
	}
	dbPath := filepath.Join(gotDir, "got.db")
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}
	s, err := store.Open(dbPath)
	if err != nil {
		return nil
	}
	bus := events.New()
	return store.NewKnowledgeStore(s.DB(), bus)
}

func findGotDir() (string, error) {
	start, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	dir := start
	for {
		gotPath := filepath.Join(dir, ".got")
		if info, err := os.Stat(gotPath); err == nil && info.IsDir() {
			return gotPath, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .got/ found")
		}
		dir = parent
	}
}

func findRepoRoot() (string, error) {
	start, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := start
	for {
		gitPath := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			if info.IsDir() || info.Mode().IsRegular() {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .git found")
		}
		dir = parent
	}
}

// ── tea.Model ───────────────────────────────────────────────────────

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, theme.TabCount)
	for _, tab := range m.tabModels {
		if tab != nil {
			cmds = append(cmds, tab.Init())
		}
	}
	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		for _, tab := range m.tabModels {
			if tab != nil {
				updated, cmd := tab.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				_ = updated
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.NextTab):
			m.activeTab = (m.activeTab + 1) % theme.TabCount
			return m, nil

		case key.Matches(msg, m.keys.PrevTab):
			m.activeTab = (m.activeTab - 1 + theme.TabCount) % theme.TabCount
			return m, nil

		case key.Matches(msg, m.keys.Help):
			m.showHelp = !m.showHelp
			return m, nil

		default:
			if m.activeTab < theme.TabCount && m.tabModels[m.activeTab] != nil {
				updated, cmd := m.tabModels[m.activeTab].Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				m.tabModels[m.activeTab] = updated
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	var b strings.Builder

	b.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Left,
		theme.Title.Render("GOT Dashboard")))
	b.WriteString("\n")

	b.WriteString(m.renderTabBar())
	b.WriteString("\n")

	contentHeight := m.height - 6
	if m.showHelp {
		contentHeight -= 3
	}

	content := ""
	if m.activeTab < theme.TabCount && m.tabModels[m.activeTab] != nil {
		content = m.tabModels[m.activeTab].View()
	}

	lines := strings.Split(content, "\n")
	for len(lines) < contentHeight {
		lines = append(lines, "")
	}
	if len(lines) > contentHeight {
		lines = lines[:contentHeight]
	}
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n")

	if m.showHelp {
		b.WriteString(m.renderHelp())
	}

	b.WriteString(m.renderStatusBar())
	return b.String()
}

func (m Model) renderTabBar() string {
	parts := make([]string, 0, theme.TabCount)
	for i, name := range theme.TabNames {
		if i == m.activeTab {
			parts = append(parts, theme.TabActive.Render(name))
		} else {
			parts = append(parts, theme.TabInactive.Render(name))
		}
	}
	return theme.TabBorder.Render(strings.Join(parts, "│"))
}

func (m Model) renderHelp() string {
	parts := make([]string, 0)
	for _, b := range m.keys.ShortHelp() {
		parts = append(parts, theme.Help.Render(b.Help().Key+" "+b.Help().Desc))
	}
	return theme.Help.Render(strings.Join(parts, " │ "))
}

func (m Model) renderStatusBar() string {
	repoInfo := theme.Muted.Render("no repo")
	if m.repoFound {
		repoInfo = theme.Success.Render(m.repoPath)
	}
	controls := theme.Muted.Render("q quit │ h/l tabs │ j/k scroll │ ? help")
	return theme.StatusBar.Render(
		lipgloss.PlaceHorizontal(m.width, lipgloss.Left, repoInfo) +
			lipgloss.PlaceHorizontal(m.width, lipgloss.Right, controls),
	)
}
