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
	"* 9a0b9b1 (tag: v1.0) Initial commit\n"

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

// --- step 14: zoom, pan, focus ---

func TestUpdate_ZoomInChangesZoom(t *testing.T) {
	m := newTestModel(t)
	if m.zoom != 1.0 {
		t.Fatalf("initial zoom = %v, want 1.0", m.zoom)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})
	if m.zoom <= 1.0 {
		t.Errorf("after + zoom = %v, want > 1.0", m.zoom)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})
	if m.zoom <= 1.25 {
		t.Errorf("after ++ zoom = %v, want > 1.25", m.zoom)
	}
}

func TestUpdate_ZoomOutChangesZoom(t *testing.T) {
	m := newTestModel(t)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'-'}})
	if m.zoom >= 1.0 {
		t.Errorf("after - zoom = %v, want < 1.0", m.zoom)
	}
}

func TestUpdate_ZeroResetsZoomAndPan(t *testing.T) {
	m := newTestModel(t)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}}) // zoom in
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}) // pan right
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	if m.zoom != 1.0 {
		t.Errorf("after 0 zoom = %v, want 1.0", m.zoom)
	}
	if m.hOffset != 0 {
		t.Errorf("after 0 hOffset = %d, want 0", m.hOffset)
	}
}

func TestUpdate_ZoomClampsAtMax(t *testing.T) {
	m := newTestModel(t)
	for i := 0; i < 200; i++ {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})
	}
	if m.zoom > maxZoom {
		t.Errorf("zoom = %v, want <= maxZoom (%v)", m.zoom, maxZoom)
	}
}

func TestUpdate_ZoomClampsAtMin(t *testing.T) {
	m := newTestModel(t)
	for i := 0; i < 200; i++ {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'-'}})
	}
	if m.zoom < minZoom {
		t.Errorf("zoom = %v, want >= minZoom (%v)", m.zoom, minZoom)
	}
}

func TestUpdate_PanRightShiftsHOffset(t *testing.T) {
	m := newTestModel(t)
	start := m.hOffset
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if m.hOffset <= start {
		t.Errorf("after l hOffset = %d, want > %d", m.hOffset, start)
	}
}

func TestUpdate_PanLeftShiftsHOffset(t *testing.T) {
	m := newTestModel(t)
	// Pan right first so we have room to pan left.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	before := m.hOffset
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if m.hOffset >= before {
		t.Errorf("after h hOffset = %d, want < %d", m.hOffset, before)
	}
}

func TestUpdate_PanLeftClampsAtZero(t *testing.T) {
	m := newTestModel(t)
	// Already at 0; pan left should stay at 0.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if m.hOffset != 0 {
		t.Errorf("hOffset = %d, want 0 (clamped)", m.hOffset)
	}
}

func TestUpdate_PanJumpsToExtreme(t *testing.T) {
	m := newTestModel(t)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	if m.hOffset != m.maxLineWidth() {
		t.Errorf("after L hOffset = %d, want %d (max line width)", m.hOffset, m.maxLineWidth())
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'H'}})
	if m.hOffset != 0 {
		t.Errorf("after H hOffset = %d, want 0", m.hOffset)
	}
}

func TestUpdate_TabCyclesFocus(t *testing.T) {
	m := newTestModel(t)
	if m.focusIdx != -1 {
		t.Fatalf("initial focusIdx = %d, want -1", m.focusIdx)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusIdx != 0 {
		t.Errorf("after first Tab focusIdx = %d, want 0 (first commit)", m.focusIdx)
	}
	// Second Tab should skip the connector row (|/).
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusIdx != 1 {
		t.Errorf("after second Tab focusIdx = %d, want 1 (skipped connector)", m.focusIdx)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusIdx != 3 {
		t.Errorf("after third Tab focusIdx = %d, want 3 (third commit, skipped empty row)", m.focusIdx)
	}
}

func TestUpdate_ShiftTabCyclesFocusBack(t *testing.T) {
	m := newTestModel(t)
	// Land on a known focus point: Tab x3 lands on row 3.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusIdx != 3 {
		t.Fatalf("setup: focusIdx = %d, want 3", m.focusIdx)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focusIdx != 1 {
		t.Errorf("after Shift+Tab focusIdx = %d, want 1", m.focusIdx)
	}
}

func TestUpdate_EnterShowsFocusedFlash(t *testing.T) {
	m := newTestModel(t)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // focus first commit
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.focusedFlash, "abc1234") {
		t.Errorf("focusedFlash = %q, want substring 'abc1234'", m.focusedFlash)
	}
	if !strings.Contains(m.focusedFlash, "First commit") {
		t.Errorf("focusedFlash = %q, want substring 'First commit'", m.focusedFlash)
	}
	// Any subsequent keypress clears the flash.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if m.focusedFlash != "" {
		t.Errorf("focusedFlash after q = %q, want empty", m.focusedFlash)
	}
}

func TestView_StatusLineShowsZoom(t *testing.T) {
	m := newTestModel(t)
	view := m.View()
	if !strings.Contains(view, "zoom") {
		t.Errorf("status line should mention zoom, got:\n%s", view)
	}
	if !strings.Contains(view, "1.00x") {
		t.Errorf("status line should show 1.00x initial zoom, got:\n%s", view)
	}
}

func TestSliceLine_BasicRange(t *testing.T) {
	got := sliceLine("abcdefghij", 2, 4)
	if got != "cdef" {
		t.Errorf("sliceLine(2,4) on 'abcdefghij' = %q, want 'cdef'", got)
	}
}

func TestSliceLine_StartBeyondWidth(t *testing.T) {
	got := sliceLine("abc", 10, 5)
	if got != "" {
		t.Errorf("sliceLine(10,5) on 'abc' = %q, want ''", got)
	}
}

func TestSliceLine_ZeroWidth(t *testing.T) {
	got := sliceLine("abc", 0, 0)
	if got != "" {
		t.Errorf("sliceLine(0,0) = %q, want ''", got)
	}
}

func TestStringWidth_Basic(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"hello", 5},
		{"a b c d e", 9},
		{"\x1b[31mred\x1b[0m", 3}, // ANSI escapes don't count
	}
	for _, c := range cases {
		got := stringWidth(c.in)
		if got != c.want {
			t.Errorf("stringWidth(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestComposeViewport_HonoursHOffset(t *testing.T) {
	m := newTestModel(t)
	// Push the horizontal offset to 5.
	m.hOffset = 5
	m.refreshViewport()
	view := m.viewport.View()
	// The first few columns should not contain "abc1234" (it
	// starts at column 2 in the unstyled content).
	if strings.HasPrefix(view, "ab") {
		t.Errorf("after pan right, viewport should not start with 'ab', got:\n%s", view)
	}
}
