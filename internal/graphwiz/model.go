// graphwiz/model.go: Bubbletea model for the got graph pager.
//
// The pager is a viewer, not a wizard — it has no answers to
// return. The user scrolls through the commit graph, optionally
// searches by commit message, and quits when they're done. Key
// bindings follow the spec §9, with the step-14 extensions:
//
//	/         start (or refresh) a search
//	n / N     jump to the next / previous match
//	j / k     scroll down / up one line
//	up / down scroll one line
//	pgup/pgdn scroll one page
//	h / l     pan left / right one column
//	<- / ->   pan left / right one column
//	H / L     pan to the far left / right
//	+ / -     zoom in / out (changes the effective column width)
//	0         reset zoom + pan
//	tab       focus the next commit (highlight + status line)
//	shift+tab focus the previous commit
//	enter     show the focused commit's SHA + subject in the status
//	          line until any other key is pressed
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

// MinZoom is the smallest zoom level the user can dial in. At
// minZoom the effective width is 4x the viewport width, so the
// user can see "more graph" in a single screen. We refuse to
// shrink further because going below this makes the graph glyphs
// unreadable on real terminals.
const minZoom = 0.25

// MaxZoom is the largest zoom level. At maxZoom the effective
// width is the viewport width divided by 4, so each commit's text
// is shown at 4x. Anything beyond this is just text stretching.
const maxZoom = 4.0

// zoomStep is the multiplicative factor for + / -.
const zoomStep = 1.25

// panStep is the number of columns h / l scroll.
const panStep = 4

// Model is the Bubbletea model for the graph pager.
type Model struct {
	state state

	// content is the full styled graph text, broken into lines for
	// search and rendering.
	content string
	lines   []string

	// renderedLines is the same as lines but with the focused
	// commit (if any) rendered with the highlight style. It is
	// rebuilt whenever focus changes; otherwise the viewport
	// keeps the un-highlighted version for performance.
	focusIdx      int // -1 = no focus
	renderedLines []string

	viewport viewport.Model

	// Search state.
	searchInput textinput.Model
	query       string
	matches     []int // line indices that match the current query
	matchIdx    int   // index into matches; -1 when no matches

	re *regexp.Regexp

	// Zoom + pan state. zoom is the multiplier applied to the
	// viewport width: effective = viewport.Width / zoom. At
	// zoom=1.0 the graph fills the viewport; at zoom=2.0 only
	// half the viewport is shown (zoomed in); at zoom=0.5 twice
	// the viewport is shown (zoomed out).
	zoom    float64
	hOffset int // horizontal scroll offset in columns

	// focusedFlash carries the "you pressed enter on a focused
	// commit" details line until the next keypress.
	focusedFlash string

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
		state:         stateBrowse,
		content:       content,
		lines:         lines,
		renderedLines: append([]string(nil), lines...),
		viewport:      vp,
		searchInput:   si,
		matchIdx:      -1,
		focusIdx:      -1,
		zoom:          1.0,
		hOffset:       0,
		theme:         theme,
		width:         80,
		height:        24,
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
		m.viewport.SetContent(m.composeViewport())
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
	// Any keypress clears the focused-commit flash banner.
	m.focusedFlash = ""
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
	case "+", "=":
		m.changeZoom(zoomStep)
		return m, nil
	case "-", "_":
		m.changeZoom(1 / zoomStep)
		return m, nil
	case "0":
		m.zoom = 1.0
		m.hOffset = 0
		m.refreshViewport()
		return m, nil
	case "h", "left":
		m.pan(-panStep)
		return m, nil
	case "l", "right":
		m.pan(panStep)
		return m, nil
	case "H":
		m.hOffset = 0
		m.refreshViewport()
		return m, nil
	case "L":
		// Pan to the far right: pick the largest line width.
		m.hOffset = m.maxLineWidth()
		m.refreshViewport()
		return m, nil
	case "tab":
		m.cycleFocus(1)
		return m, nil
	case "shift+tab":
		m.cycleFocus(-1)
		return m, nil
	case "enter":
		m.showFocusedFlash()
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
			m.refreshViewport()
			m.jumpMatch(0)
		} else {
			m.matchIdx = -1
			m.refreshViewport()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

// changeZoom multiplies the current zoom by factor, clamped to
// [minZoom, maxZoom], and refreshes the viewport. hOffset is
// rescaled so the centre of the visible content stays in place.
func (m *Model) changeZoom(factor float64) {
	oldZoom := m.zoom
	newZoom := oldZoom * factor
	if newZoom < minZoom {
		newZoom = minZoom
	}
	if newZoom > maxZoom {
		newZoom = maxZoom
	}
	if newZoom == oldZoom {
		return
	}
	// Rescale hOffset so the centre column stays put: at old zoom
	// the user was looking at column hOffset+width/2, at new zoom
	// the same column should be at the same screen position.
	if oldZoom != 0 {
		m.hOffset = int(float64(m.hOffset) * newZoom / oldZoom)
	}
	m.zoom = newZoom
	m.clampHOffset()
	m.refreshViewport()
}

// pan shifts the horizontal offset by delta columns, clamped to
// [0, maxLineWidth()].
func (m *Model) pan(delta int) {
	m.hOffset += delta
	m.clampHOffset()
	m.refreshViewport()
}

// clampHOffset keeps m.hOffset in [0, maxLineWidth()].
func (m *Model) clampHOffset() {
	max := m.maxLineWidth()
	if m.hOffset < 0 {
		m.hOffset = 0
	}
	if m.hOffset > max {
		m.hOffset = max
	}
}

// maxLineWidth returns the longest line in the rendered content,
// used to bound hOffset.
func (m *Model) maxLineWidth() int {
	max := 0
	for _, l := range m.renderedLines {
		if w := stringWidth(l); w > max {
			max = w
		}
	}
	return max
}

// effectiveWidth returns the viewport's effective horizontal
// extent after the zoom factor is applied. The viewport itself
// stays at m.viewport.Width so the underlying bubbles/viewport
// math keeps working; we just slice our own content to the
// effective width when rendering.
func (m *Model) effectiveWidth() int {
	w := int(float64(m.viewport.Width) / m.zoom)
	if w < 1 {
		w = 1
	}
	return w
}

// cycleFocus moves the focus index by delta in the list of lines.
// Lines that don't contain a commit are skipped so Tab always
// lands on a real commit row. If no commits are found, the focus
// stays where it was.
//
// We test against the un-styled m.lines (not m.renderedLines) so
// theme-specific styling (e.g. ANSI escape codes from a non-no-color
// theme) cannot fool the commit-line regex.
func (m *Model) cycleFocus(delta int) {
	if len(m.lines) == 0 {
		return
	}
	isCommit := func(line string) bool {
		return commitLineRE.MatchString(line)
	}
	// If the current focus is not on a commit, snap to the first
	// commit (forward) or last commit (backward).
	if m.focusIdx < 0 || m.focusIdx >= len(m.lines) || !isCommit(m.lines[m.focusIdx]) {
		if delta > 0 {
			for i, l := range m.lines {
				if isCommit(l) {
					m.focusIdx = i
					m.refreshFocus()
					return
				}
			}
		} else {
			for i := len(m.lines) - 1; i >= 0; i-- {
				if isCommit(m.lines[i]) {
					m.focusIdx = i
					m.refreshFocus()
					return
				}
			}
		}
		return
	}
	step := delta
	if step < 0 {
		step = -step
	}
	for i := 0; i < step; i++ {
		next := m.focusIdx + delta
		for next >= 0 && next < len(m.lines) {
			if isCommit(m.lines[next]) {
				m.focusIdx = next
				break
			}
			next += delta
		}
		if next < 0 || next >= len(m.lines) {
			return // wrapped (no neighbour) — keep current focus
		}
	}
	m.refreshFocus()
}

// commitLineRE matches a line that contains a 7+ char hex SHA
// (a commit row from `git log --graph --oneline`).
var commitLineRE = regexp.MustCompile(`[0-9a-f]{7,40}`)

// refreshFocus rebuilds the renderedLines slice with the focused
// commit row highlighted, and nudges the viewport to keep it on
// screen. It is a no-op when no focus is set.
func (m *Model) refreshFocus() {
	m.renderedLines = make([]string, len(m.lines))
	for i, l := range m.lines {
		if i == m.focusIdx {
			m.renderedLines[i] = m.theme.Selected.Render(l)
		} else {
			m.renderedLines[i] = l
		}
	}
	m.refreshViewport()
	if m.focusIdx >= 0 {
		m.scrollToLine(m.focusIdx)
	}
}

// showFocusedFlash copies the focused commit's SHA + subject
// (best-effort: any 7-40 char hex token plus the rest of the
// line) into the status-line flash field so the user can read it
// until they press another key. If no commit is focused, the
// flash is cleared.
func (m *Model) showFocusedFlash() {
	if m.focusIdx < 0 || m.focusIdx >= len(m.lines) {
		m.focusedFlash = "(no commit focused)"
		return
	}
	line := m.lines[m.focusIdx]
	if loc := commitLineRE.FindStringIndex(line); loc != nil {
		sha := line[loc[0]:loc[1]]
		rest := strings.TrimSpace(line[loc[1]:])
		m.focusedFlash = fmt.Sprintf("commit %s  %s", sha, rest)
		return
	}
	m.focusedFlash = "(focused line is not a commit)"
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
		m.refreshViewport()
		m.scrollToLine(m.matches[m.matchIdx])
		return
	}
	m.matchIdx = (m.matchIdx + dir + len(m.matches)) % len(m.matches)
	m.refreshViewport()
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

// refreshViewport rebuilds the viewport's content string from
// the (possibly focused-highlighted) lines and applies the
// current zoom + horizontal offset.
func (m *Model) refreshViewport() {
	m.viewport.SetContent(m.composeViewport())
}

// composeViewport assembles the string the viewport should
// display: a horizontal slice (hOffset..hOffset+effectiveWidth)
// of each line, joined by newlines.
func (m *Model) composeViewport() string {
	if len(m.renderedLines) == 0 {
		return ""
	}
	w := m.effectiveWidth()
	out := make([]string, 0, len(m.renderedLines))
	for _, line := range m.renderedLines {
		out = append(out, sliceLine(line, m.hOffset, w))
	}
	return strings.Join(out, "\n")
}

// sliceLine returns a substring of s that starts at `start`
// columns and is at most `width` columns wide. ANSI escape
// sequences do not advance the column counter. The returned
// string preserves any pending ANSI state at the slice start
// (so a half-styled line renders correctly) but does NOT
// re-append a reset at the slice end.
func sliceLine(s string, start, width int) string {
	if width <= 0 {
		return ""
	}
	// pendingCarry is any ANSI escape sequence we crossed over
	// before reaching the slice start. We re-emit it at the
	// beginning of the returned string so the slice renders
	// with the same styles as the original.
	pendingCarry := ""
	byteStart := 0
	visible := 0
	emitted := 0 // characters emitted into the returned slice
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			// Capture the whole CSI sequence (ESC [ params
			// final-byte) so we can carry or skip it.
			end := i + 1
			if end < len(s) && s[end] == '[' {
				end++ // skip '[' introducer
			}
			for end < len(s) && s[end] >= 0x20 && s[end] <= 0x3f {
				end++ // skip parameter / intermediate bytes
			}
			if end < len(s) {
				end++ // include final byte
			}
			if visible < start {
				pendingCarry += s[i:end]
				byteStart = end
			}
			i = end - 1
			continue
		}
		// Multi-byte UTF-8 rune: count as one visible column.
		if s[i]&0x80 != 0 {
			j := i
			for j < len(s) && (s[j]&0xc0) == 0x80 {
				j++
			}
			i = j - 1
		}
		visible++
		if visible == start {
			byteStart = i + 1
			for byteStart > 0 && byteStart < len(s) && s[byteStart-1]&0x80 != 0 && (s[byteStart]&0xc0) == 0x80 {
				byteStart--
			}
		}
		if visible > start {
			emitted++
			if emitted > width {
				return pendingCarry + s[byteStart:i]
			}
		}
	}
	if visible <= start {
		return ""
	}
	return pendingCarry + s[byteStart:]
}

// stringWidth returns the visible (column) width of s, ignoring
// ANSI escape sequences and treating each multi-byte UTF-8 rune
// as one column. Good enough for the graph glyphs we use.
func stringWidth(s string) int {
	w := 0
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			// Skip the rest of the CSI sequence: ESC, optional
			// '[' introducer, then parameter + intermediate
			// bytes (0x20-0x3f), then a final byte (0x40-0x7e).
			i++
			if i < len(s) && s[i] == '[' {
				i++
			}
			for i < len(s) && s[i] >= 0x20 && s[i] <= 0x3f {
				i++
			}
			// i now points at the final byte (or past end);
			// the for loop's i++ will advance past it.
			continue
		}
		if s[i]&0x80 != 0 {
			// Skip continuation bytes of a multi-byte rune.
			j := i
			for j < len(s) && (s[j]&0xc0) == 0x80 {
				j++
			}
			i = j - 1
		}
		w++
	}
	return w
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
	if m.focusedFlash != "" {
		return m.theme.Detected.Render(m.focusedFlash)
	}
	if m.state == stateSearch {
		return m.theme.Box.Render("search: ") + m.searchInput.View()
	}
	if len(m.matches) > 0 {
		return m.theme.Muted.Render(fmt.Sprintf(
			"/%s  match %d/%d  •  zoom %.2fx  •  / search  •  n/N next/prev  •  q quit",
			m.query, m.matchIdx+1, len(m.matches), m.zoom,
		))
	}
	if m.query != "" {
		return m.theme.Muted.Render(
			"/" + m.query + "  (no matches)  •  / search  •  q quit",
		)
	}
	return m.theme.Muted.Render(fmt.Sprintf(
		"zoom %.2fx  •  / search  •  j/k scroll  •  h/l pan  •  Tab focus  •  q quit",
		m.zoom,
	))
}
