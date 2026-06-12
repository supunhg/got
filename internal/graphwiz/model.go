// graphwiz/model.go: Bubbletea model for the got graph pager.
//
// The pager is a viewer, not a wizard — it has no answers to
// return. The user scrolls through the commit graph, optionally
// searches by commit message, and quits when they're done. Key
// bindings follow the spec §9:
//
//	/         start (or refresh) a search
//	n / N     jump to the next / previous match
//	j / k     scroll down / up one line
//	up / down scroll one line
//	pgup/pgdn scroll one page
//	home      jump to the top
//	end       jump to the bottom
//	q / esc   quit
//	ctrl+c    quit
//
// While the search input is active, normal scrolling is suspended
// and the user types a regex. Pressing enter runs the search and
// re-enables scrolling; pressing esc cancels the search.

package graphwiz

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/got-sh/got/internal/tui"
)

// state enumerates the pager's two modes: browsing and searching.
type state int

const (
	stateBrowse state = iota
	stateSearch
)

// Model is the Bubbletea model for the graph pager.
type Model struct {
	state state

	// content is the full styled graph text, broken into lines for
	// search and rendering.
	content string
	lines   []string

	viewport viewport.Model

	// Search state.
	searchInput textinput.Model
	query       string
	matches     []int // line indices that match the current query
	matchIdx    int   // index into matches; -1 when no matches
	re          *regexp.Regexp

	theme tui.Theme

	width  int
	height int
}

// NewModel builds a Model ready to run. `content` is the fully
// styled graph text (one commit per line, possibly with empty
// lines). The theme is applied immediately so every render call
// returns consistent output.
func NewModel(content string, theme tui.Theme) *Model {
	theme = theme.Apply()
	// Drop a single trailing newline so the line count matches the
	// number of physical lines a user sees in the terminal. Without
	// this, a 4-line content with a trailing "\n" becomes 5 entries
	// in the slice (the last being empty), which throws off search
	// math and the status-line "match N/M" counter.
	trimmed := strings.TrimRight(content, "\n")
	var lines []string
	if trimmed == "" {
		lines = []string{}
	} else {
		lines = strings.Split(trimmed, "\n")
	}
	vp := viewport.New(80, 24)
	vp.SetContent(strings.Join(lines, "\n"))
	si := textinput.New()
	si.Placeholder = "search commits (regex)..."
	si.CharLimit = 200
	return &Model{
		state:       stateBrowse,
		content:     content,
		lines:       lines,
		viewport:    vp,
		searchInput: si,
		matchIdx:    -1,
		theme:       theme,
		width:       80,
		height:      24,
	}
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd { return nil }

// Cancelled reports whether the user quits the pager. Always true
// when the program returns; the Run() wrapper uses this only to
// decide between CancelledError and nil.
func (m *Model) Cancelled() bool {
	// The pager has no done state; quitting is always a "cancel"
	// from the wizard's perspective. Callers that just want to
	// block until the user is done can ignore the return value.
	return false
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Reserve one row for the search line / hint line.
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 1
		return m, nil
	case tea.KeyMsg:
		if m.state == stateSearch {
			return m.updateSearch(msg)
		}
		return m.updateBrowse(msg)
	}
	// Forward non-key messages to the viewport so the underlying
	// mouse wheel + scroll behaviour keeps working.
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *Model) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit
	case "/":
		m.state = stateSearch
		m.searchInput.Focus()
		return m, nil
	case "n":
		m.jumpMatch(1)
		return m, nil
	case "N":
		m.jumpMatch(-1)
		return m, nil
	case "home", "g":
		m.viewport.GotoTop()
		return m, nil
	case "end", "G":
		m.viewport.GotoBottom()
		return m, nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Cancel the search and return to browse mode. Keep the
		// previous query and matches so n/N still works.
		m.state = stateBrowse
		m.searchInput.Blur()
		return m, nil
	case "enter":
		m.query = strings.TrimSpace(m.searchInput.Value())
		m.runSearch()
		m.state = stateBrowse
		m.searchInput.Blur()
		if len(m.matches) > 0 {
			m.matchIdx = 0
			m.viewport.SetContent(m.renderWithHighlights())
			m.jumpMatch(0)
		} else {
			m.matchIdx = -1
			m.viewport.SetContent(m.content)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

// runSearch compiles the current query and populates m.matches.
// Invalid regexes yield no matches (and the pager shows the
// unhighlighted content).
func (m *Model) runSearch() {
	if m.query == "" {
		m.re = nil
		m.matches = nil
		m.matchIdx = -1
		return
	}
	re, err := regexp.Compile(m.query)
	if err != nil {
		m.re = nil
		m.matches = nil
		m.matchIdx = -1
		return
	}
	m.re = re
	m.matches = nil
	for i, line := range m.lines {
		if re.MatchString(line) {
			m.matches = append(m.matches, i)
		}
	}
	if len(m.matches) == 0 {
		m.matchIdx = -1
	}
}

// jumpMatch moves the viewport to the next (dir>0) or previous
// (dir<0) match. dir==0 jumps to m.matchIdx without moving. The
// cursor is wrapped around.
func (m *Model) jumpMatch(dir int) {
	if len(m.matches) == 0 {
		return
	}
	if dir == 0 {
		m.viewport.SetContent(m.renderWithHighlights())
		m.scrollToLine(m.matches[m.matchIdx])
		return
	}
	m.matchIdx = (m.matchIdx + dir + len(m.matches)) % len(m.matches)
	m.viewport.SetContent(m.renderWithHighlights())
	m.scrollToLine(m.matches[m.matchIdx])
}

// scrollToLine centers the viewport on absolute line n. The
// viewport treats its content as a single big string, so we just
// have to nudge the offset by enough lines.
func (m *Model) scrollToLine(n int) {
	if n < 0 || n >= len(m.lines) {
		return
	}
	// Bubble's viewport measures offsets in lines from the top.
	// We approximate "center" by setting the offset to n - height/2.
	target := n - m.viewport.Height/2
	if target < 0 {
		target = 0
	}
	m.viewport.YOffset = target
}

// renderWithHighlights returns the content with the current match
// highlighted. Lines that contain a match are rendered with an
// inverse style so the user can see which one is active.
func (m *Model) renderWithHighlights() string {
	if m.matchIdx < 0 || m.matchIdx >= len(m.matches) {
		return m.content
	}
	currentLine := m.matches[m.matchIdx]
	var b strings.Builder
	for i, line := range m.lines {
		if i == currentLine {
			b.WriteString(m.theme.Selected.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// View implements tea.Model. Renders the viewport plus a one-line
// status bar at the bottom showing the current mode, query, and
// match position.
func (m *Model) View() string {
	body := m.viewport.View()
	status := m.statusLine()
	return body + "\n" + status
}

func (m *Model) statusLine() string {
	switch m.state {
	case stateSearch:
		return m.theme.Box.Render("search: ") + m.searchInput.View()
	}
	switch {
	case len(m.matches) > 0:
		return m.theme.Muted.Render(fmt.Sprintf(
			"/%s  match %d/%d  •  n next  •  N prev  •  / new search  •  q quit",
			m.query, m.matchIdx+1, len(m.matches),
		))
	case m.query != "":
		return m.theme.Muted.Render(
			"/" + m.query + "  (no matches)  •  / new search  •  q quit",
		)
	}
	return m.theme.Muted.Render(
		"/ search  •  j/k scroll  •  g/G top/bottom  •  q quit",
	)
}
