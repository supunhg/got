package graphwiz

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/got-sh/got/internal/tui"
)

// sampleContent is a tiny commit graph for the tests. The leading
// spaces are part of the prefix; the * markers are unrendered.
const sampleContent = "" +
	"* abc1234 (HEAD -> main) First commit\n" +
	"| * def5678 (origin/main) Second commit\n" +
	"|/\n" +
	"* ghi9012 (tag: v1.0) Initial commit\n"

func newTestModel(t *testing.T) *Model {
	t.Helper()
	m := NewModel(sampleContent, tui.NoColorTheme())
	// Force a known window size so viewport math is deterministic.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return updated.(*Model)
}

func TestNewModel_LinesAreSplit(t *testing.T) {
	m := NewModel(sampleContent, tui.NoColorTheme())
	if len(m.lines) != 4 {
		t.Errorf("lines = %d, want 4 (3 commits + 1 connector row)", len(m.lines))
	}
}

func TestNewModel_ViewportHasContent(t *testing.T) {
	m := NewModel(sampleContent, tui.NoColorTheme())
	if !strings.Contains(m.viewport.View(), "abc1234") {
		t.Errorf("viewport missing commit SHA, got:\n%s", m.viewport.View())
	}
}

func TestUpdate_WindowSizeResizesViewport(t *testing.T) {
	m := newTestModel(t)
	if m.viewport.Width != 80 || m.viewport.Height != 23 {
		t.Errorf("viewport = %dx%d, want 80x23 (one row reserved for status)", m.viewport.Width, m.viewport.Height)
	}
}

func TestUpdate_QuitOnQ(t *testing.T) {
	m := newTestModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatalf("expected tea.Quit on 'q', got nil cmd")
	}
}

func TestUpdate_QuitOnCtrlC(t *testing.T) {
	m := newTestModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("expected tea.Quit on ctrl+c, got nil cmd")
	}
}

func TestUpdate_QuitOnEsc(t *testing.T) {
	m := newTestModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatalf("expected tea.Quit on esc, got nil cmd")
	}
}

func TestUpdate_SlashEntersSearch(t *testing.T) {
	m := newTestModel(t)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if m.state != stateSearch {
		t.Errorf("state = %v, want stateSearch after /", m.state)
	}
}

func TestUpdate_EnterOnSearchRunsQuery(t *testing.T) {
	m := newTestModel(t)
	// Enter search mode.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	// Type the query "Second".
	for _, r := range "Second" {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	// Press enter.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != stateBrowse {
		t.Errorf("state = %v, want stateBrowse after enter", m.state)
	}
	if m.query != "Second" {
		t.Errorf("query = %q, want Second", m.query)
	}
	if len(m.matches) != 1 {
		t.Errorf("matches = %d, want 1 (the 'Second commit' line)", len(m.matches))
	}
	if len(m.matches) >= 1 && m.matches[0] != 1 {
		t.Errorf("matches[0] = %d, want 1", m.matches[0])
	}
}

func TestUpdate_EnterOnSearchNoMatch(t *testing.T) {
	m := newTestModel(t)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "nonexistent" {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.matchIdx != -1 {
		t.Errorf("matchIdx = %d, want -1 for no matches", m.matchIdx)
	}
}

func TestUpdate_InvalidRegex(t *testing.T) {
	m := newTestModel(t)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "[unclosed" {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.matches) != 0 {
		t.Errorf("matches = %d, want 0 for invalid regex", len(m.matches))
	}
}

func TestUpdate_NextPrevMatch(t *testing.T) {
	m := newTestModel(t)
	// Match the regex "commit" — should hit all 3 commit lines.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "commit" {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.matches) != 3 {
		t.Fatalf("matches = %d, want 3 (one per commit line)", len(m.matches))
	}
	if m.matchIdx != 0 {
		t.Fatalf("matchIdx = %d after enter, want 0", m.matchIdx)
	}
	// n jumps forward.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.matchIdx != 1 {
		t.Errorf("matchIdx = %d after n, want 1", m.matchIdx)
	}
	// N jumps backward.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	if m.matchIdx != 0 {
		t.Errorf("matchIdx = %d after N, want 0", m.matchIdx)
	}
	// N from 0 wraps to last.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	if m.matchIdx != 2 {
		t.Errorf("matchIdx = %d after wrap-N, want 2", m.matchIdx)
	}
}

func TestUpdate_EscCancelsSearch(t *testing.T) {
	m := newTestModel(t)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "foo" {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != stateBrowse {
		t.Errorf("state = %v after esc, want stateBrowse", m.state)
	}
}

func TestView_StatusLineHasHints(t *testing.T) {
	m := newTestModel(t)
	view := m.View()
	if !strings.Contains(view, "search") {
		t.Errorf("status line should mention / search, got:\n%s", view)
	}
}

func TestView_StatusLineShowsMatchCount(t *testing.T) {
	m := newTestModel(t)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "commit" {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := m.View()
	if !strings.Contains(view, "match 1/3") {
		t.Errorf("status line should show 'match 1/3', got:\n%s", view)
	}
}
