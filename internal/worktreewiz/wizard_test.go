package worktreewiz

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/got-sh/got/internal/tui"
)

func sampleEntries() []Entry {
	now := time.Now().UTC()
	return []Entry{
		{Path: "/tmp/repo", Branch: "main", HEAD: "abc1234", IsMain: true, Label: "main worktree"},
		{Path: "/tmp/feature", Branch: "feature/x", HEAD: "def5678", Label: "sandbox", LastAttachedAt: now.Add(-2 * time.Hour)},
		{Path: "/tmp/detached", Branch: "", HEAD: "9990000", Locked: true},
	}
}

func TestEntry_TitleIncludesLabelAndBranch(t *testing.T) {
	e := Entry{Path: "/tmp/feature", Branch: "feature/x", HEAD: "def5678"}
	got := e.Title()
	for _, want := range []string{"feature/x", "def5678"} {
		if !strings.Contains(got, want) {
			t.Errorf("Title %q missing %q", got, want)
		}
	}
}

func TestEntry_TitleMainMarker(t *testing.T) {
	e := Entry{Path: "/tmp/repo", Branch: "main", HEAD: "abc1234", IsMain: true}
	if !strings.Contains(e.Title(), "(main)") {
		t.Errorf("Title %q missing (main) marker", e.Title())
	}
}

func TestEntry_TitleDetachedFallback(t *testing.T) {
	e := Entry{Path: "/tmp/detached", HEAD: "9990000"}
	if !strings.Contains(e.Title(), "detached") {
		t.Errorf("Title %q missing 'detached' fallback", e.Title())
	}
}

func TestEntry_DescriptionHasPathAndLockAndRelative(t *testing.T) {
	e := Entry{Path: "/tmp/feature", Locked: true, LastAttachedAt: time.Now().Add(-3 * time.Minute)}
	d := e.Description()
	for _, want := range []string{"/tmp/feature", "locked", "ago"} {
		if !strings.Contains(d, want) {
			t.Errorf("Description %q missing %q", d, want)
		}
	}
}

func TestEntry_DescriptionEmptyWhenNothingSet(t *testing.T) {
	e := Entry{Path: "/tmp/feature"}
	if e.Description() != "/tmp/feature" {
		t.Errorf("Description = %q, want just the path", e.Description())
	}
}

func TestEntry_FilterValueMatchesTitle(t *testing.T) {
	e := Entry{Path: "/tmp/feature", Branch: "feature/x", HEAD: "def5678"}
	if e.FilterValue() != e.Title() {
		t.Errorf("FilterValue %q != Title %q", e.FilterValue(), e.Title())
	}
}

func TestNewModel_PreSelectsMatchingPath(t *testing.T) {
	entries := sampleEntries()
	m := NewModel(entries, PrePopulated{Path: "/tmp/feature"}, tui.NoColorTheme())
	got := m.currentPath()
	if got != "/tmp/feature" {
		t.Errorf("currentPath = %q, want /tmp/feature (pre-populated path should be highlighted)", got)
	}
}

func TestNewModel_NoPrePopulatedFallsBackToFirst(t *testing.T) {
	entries := sampleEntries()
	m := NewModel(entries, PrePopulated{}, tui.NoColorTheme())
	got := m.currentPath()
	if got != "/tmp/repo" {
		t.Errorf("currentPath = %q, want /tmp/repo (first entry)", got)
	}
}

func TestNewModel_EmptyEntries(t *testing.T) {
	m := NewModel(nil, PrePopulated{}, tui.NoColorTheme())
	if m.currentPath() != "" {
		t.Errorf("currentPath on empty entries = %q, want empty", m.currentPath())
	}
}

func TestModel_HandleKey_QuitOnQ(t *testing.T) {
	m := NewModel(sampleEntries(), PrePopulated{}, tui.NoColorTheme())
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Errorf("expected tea.Quit cmd on 'q', got nil")
	}
	if !m.cancelled() {
		t.Errorf("expected cancelled() = true after 'q'")
	}
}

func TestModel_HandleKey_QuitOnEsc(t *testing.T) {
	m := NewModel(sampleEntries(), PrePopulated{}, tui.NoColorTheme())
	_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if !m.cancelled() {
		t.Errorf("expected cancelled() = true after 'esc'")
	}
}

func TestModel_HandleKey_QuitOnCtrlC(t *testing.T) {
	m := NewModel(sampleEntries(), PrePopulated{}, tui.NoColorTheme())
	_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m.cancelled() {
		t.Errorf("expected cancelled() = true after 'ctrl+c'")
	}
}

func TestModel_HandleKey_AttachEntersConfirmMode(t *testing.T) {
	m := NewModel(sampleEntries(), PrePopulated{}, tui.NoColorTheme())
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if !m.confirmMode {
		t.Errorf("expected confirmMode = true after 'a'")
	}
}

func TestModel_HandleKey_OpenEditorResolvesImmediately(t *testing.T) {
	m := NewModel(sampleEntries(), PrePopulated{Path: "/tmp/feature"}, tui.NoColorTheme())
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if cmd == nil {
		t.Errorf("expected tea.Quit cmd on 'e', got nil")
	}
	if m.answers.Action != ActionOpenEditor {
		t.Errorf("answers.Action = %q, want %q", m.answers.Action, ActionOpenEditor)
	}
	if m.answers.Path != "/tmp/feature" {
		t.Errorf("answers.Path = %q, want /tmp/feature", m.answers.Path)
	}
}

func TestModel_HandleKey_ConfirmYesCommitsAttach(t *testing.T) {
	m := NewModel(sampleEntries(), PrePopulated{Path: "/tmp/feature"}, tui.NoColorTheme())
	// First 'a' enters confirm mode.
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	// Then 'y' confirms.
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Errorf("expected tea.Quit cmd on confirm, got nil")
	}
	if m.answers.Action != ActionAttach {
		t.Errorf("answers.Action = %q, want %q", m.answers.Action, ActionAttach)
	}
	if m.answers.Path != "/tmp/feature" {
		t.Errorf("answers.Path = %q, want /tmp/feature", m.answers.Path)
	}
	if m.answers.Label != "sandbox" {
		t.Errorf("answers.Label = %q, want sandbox", m.answers.Label)
	}
}

func TestModel_HandleKey_ConfirmNoReturnsToBrowse(t *testing.T) {
	m := NewModel(sampleEntries(), PrePopulated{Path: "/tmp/feature"}, tui.NoColorTheme())
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}) // confirm
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}) // deny
	if m.confirmMode {
		t.Errorf("confirmMode should be false after 'n'")
	}
	if m.answers.Action != ActionNone {
		t.Errorf("answers.Action = %q, want %q (still no action)", m.answers.Action, ActionNone)
	}
}

func TestModel_View_ConfirmModeShowsPrompt(t *testing.T) {
	m := NewModel(sampleEntries(), PrePopulated{Path: "/tmp/feature"}, tui.NoColorTheme())
	m.confirmMode = true
	m.pickedPath = "/tmp/feature"
	m.pickedLabel = "sandbox"
	view := m.View()
	if !strings.Contains(view, "Attach to sandbox?") {
		t.Errorf("View in confirm mode = %q, expected to contain 'Attach to sandbox?'", view)
	}
}

func TestModel_View_BrowseModeShowsList(t *testing.T) {
	m := NewModel(sampleEntries(), PrePopulated{}, tui.NoColorTheme())
	view := m.View()
	if !strings.Contains(view, "Worktrees") {
		t.Errorf("View in browse mode = %q, expected to contain 'Worktrees' (list title)", view)
	}
	if !strings.Contains(view, "attach") {
		t.Errorf("View in browse mode = %q, expected help text containing 'attach'", view)
	}
}

func TestFormatRelative(t *testing.T) {
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{10 * time.Second, "just now"},
		{2 * time.Minute, "2m ago"},
		{3 * time.Hour, "3h ago"},
		{2 * 24 * time.Hour, "2d ago"},
		{30 * 24 * time.Hour, ""}, // absolute date — we just check it doesn't crash
	}
	for _, c := range cases {
		got := formatRelative(time.Now().Add(-c.ago))
		if c.want == "" {
			if strings.Contains(got, "ago") {
				t.Errorf("formatRelative(-%v) = %q, expected absolute date", c.ago, got)
			}
			continue
		}
		if got != c.want {
			t.Errorf("formatRelative(-%v) = %q, want %q", c.ago, got, c.want)
		}
	}
}

func TestJoinComma(t *testing.T) {
	if got := joinComma(nil); got != "" {
		t.Errorf("joinComma(nil) = %q, want empty", got)
	}
	if got := joinComma([]string{"a"}); got != "a" {
		t.Errorf("joinComma([a]) = %q, want a", got)
	}
	if got := joinComma([]string{"a", "b", "c"}); got != "a, b, c" {
		t.Errorf("joinComma = %q, want 'a, b, c'", got)
	}
}
