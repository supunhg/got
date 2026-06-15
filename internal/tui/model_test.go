package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/supunhg/got/internal/tui/theme"
)

func TestDefaultKeyMap(t *testing.T) {
	km := DefaultKeyMap()

	// Verify all keybindings are set
	if km.Quit.Keys() == nil || len(km.Quit.Keys()) == 0 {
		t.Error("Quit keybinding not set")
	}
	if km.Refresh.Keys() == nil || len(km.Refresh.Keys()) == 0 {
		t.Error("Refresh keybinding not set")
	}
	if km.Help.Keys() == nil || len(km.Help.Keys()) == 0 {
		t.Error("Help keybinding not set")
	}
	if km.NextTab.Keys() == nil || len(km.NextTab.Keys()) == 0 {
		t.Error("NextTab keybinding not set")
	}
	if km.PrevTab.Keys() == nil || len(km.PrevTab.Keys()) == 0 {
		t.Error("PrevTab keybinding not set")
	}
	if km.Up.Keys() == nil || len(km.Up.Keys()) == 0 {
		t.Error("Up keybinding not set")
	}
	if km.Down.Keys() == nil || len(km.Down.Keys()) == 0 {
		t.Error("Down keybinding not set")
	}
	if km.Enter.Keys() == nil || len(km.Enter.Keys()) == 0 {
		t.Error("Enter keybinding not set")
	}
}

func TestKeyMapShortHelp(t *testing.T) {
	km := DefaultKeyMap()
	help := km.ShortHelp()

	if len(help) != 8 {
		t.Errorf("ShortHelp() returned %d bindings, want 8", len(help))
	}
}

func TestKeyMapFullHelp(t *testing.T) {
	km := DefaultKeyMap()
	help := km.FullHelp()

	if len(help) != 3 {
		t.Errorf("FullHelp() returned %d rows, want 3", len(help))
	}
}

func TestModelInit(t *testing.T) {
	// NewModel requires a real git repo, so we test the KeyMap instead
	km := DefaultKeyMap()
	if km.Quit.Keys() == nil {
		t.Error("DefaultKeyMap not initialized")
	}
}

func TestModelUpdateQuit(t *testing.T) {
	// Create a minimal model for testing key handling
	m := Model{
		activeTab: theme.TabStatus,
		keys:      DefaultKeyMap(),
		tabModels: [theme.TabCount]tea.Model{},
	}

	// Test quit key
	quitMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	updated, cmd := m.Update(quitMsg)

	if cmd == nil {
		t.Error("quit key should return a command")
	}

	// Verify the command is tea.Quit
	result := cmd()
	if _, ok := result.(tea.QuitMsg); !ok {
		t.Error("quit command should return QuitMsg")
	}

	_ = updated
}

func TestModelUpdateNextTab(t *testing.T) {
	m := Model{
		activeTab: theme.TabStatus,
		keys:      DefaultKeyMap(),
		tabModels: [theme.TabCount]tea.Model{},
	}

	// Test next tab
	nextMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}
	updated, _ := m.Update(nextMsg)

	newModel := updated.(Model)
	if newModel.activeTab != theme.TabBranches {
		t.Errorf("activeTab = %d after 'l', want %d", newModel.activeTab, theme.TabBranches)
	}
}

func TestModelUpdatePrevTab(t *testing.T) {
	m := Model{
		activeTab: theme.TabBranches,
		keys:      DefaultKeyMap(),
		tabModels: [theme.TabCount]tea.Model{},
	}

	// Test prev tab
	prevMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	updated, _ := m.Update(prevMsg)

	newModel := updated.(Model)
	if newModel.activeTab != theme.TabStatus {
		t.Errorf("activeTab = %d after 'h', want %d", newModel.activeTab, theme.TabStatus)
	}
}

func TestModelUpdateHelp(t *testing.T) {
	m := Model{
		activeTab: theme.TabStatus,
		keys:      DefaultKeyMap(),
		tabModels: [theme.TabCount]tea.Model{},
	}

	// Toggle help on
	helpMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}
	updated, _ := m.Update(helpMsg)

	newModel := updated.(Model)
	if !newModel.showHelp {
		t.Error("showHelp should be true after pressing '?'")
	}

	// Toggle help off
	updated2, _ := newModel.Update(helpMsg)
	newModel2 := updated2.(Model)
	if newModel2.showHelp {
		t.Error("showHelp should be false after pressing '?' again")
	}
}

func TestModelUpdateTabWrap(t *testing.T) {
	m := Model{
		activeTab: theme.TabKnowledge, // last tab
		keys:      DefaultKeyMap(),
		tabModels: [theme.TabCount]tea.Model{},
	}

	// Test tab wraps around
	nextMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}
	updated, _ := m.Update(nextMsg)

	newModel := updated.(Model)
	if newModel.activeTab != theme.TabStatus {
		t.Errorf("activeTab = %d after wrap, want %d (TabStatus)", newModel.activeTab, theme.TabStatus)
	}
}

func TestModelViewZeroSize(t *testing.T) {
	m := Model{
		width:  0,
		height: 0,
	}

	view := m.View()
	if view != "Initializing..." {
		t.Errorf("View() = %q, want %q", view, "Initializing...")
	}
}
