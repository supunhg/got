package worktreewiz

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/got-sh/got/internal/tui"
)

// Model is the Bubbletea model for the worktree picker. It holds
// the entry list, the selected index, the theme, and the wizard's
// resolved answers. The state is intentionally small: a single
// bool (confirmMode) tracks whether the user has just pressed
// enter on a row and we are showing the "Attach to <path>?"
// confirmation prompt. The embedded bubbles/list handles the
// browse mode.
type Model struct {
	// entries is the pre-built list the wizard renders. Stored
	// as []list.Item so we can hand it directly to
	// bubbles/list.
	entries []list.Item
	// theme is the Lip Gloss theme (already .Apply()'d by Run).
	theme tui.Theme
	// list is the embedded bubbles/list view.
	list list.Model
	// answers is the wizard's output. Populated as soon as
	// the user confirms a row so the bubbletea driver can
	// read it after p.Run() returns.
	answers Answers
	// confirmMode is true when the user has just pressed
	// enter on a row and we are showing the confirmation
	// prompt.
	confirmMode bool
	// pickedPath is the path the user picked, remembered
	// across the confirmation prompt so we can act on it.
	pickedPath string
	// pickedLabel is the label of the picked worktree.
	pickedLabel string
}

// NewModel constructs a Model ready to run. It pre-selects the
// row matching pre.Path (if any) and configures the list with
// sensible defaults.
func NewModel(entries []Entry, pre PrePopulated, theme tui.Theme) *Model {
	theme = theme.Apply()
	items := make([]list.Item, 0, len(entries))
	for i := range entries {
		items = append(items, entries[i])
	}
	l := list.New(items, listDelegate{theme: theme}, 80, 20)
	l.Title = "Worktrees (a: attach, e: open in editor, q: quit)"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = theme.Title
	l.Styles.HelpStyle = theme.Muted
	m := &Model{
		entries: items,
		theme:   theme,
		list:    l,
	}
	if pre.Path != "" {
		for i, it := range items {
			e, ok := it.(Entry)
			if !ok {
				continue
			}
			if e.Path == pre.Path {
				m.list.Select(i)
				break
			}
		}
	}
	return m
}

// Init is the Bubbletea entry point. We have no commands to run
// at startup (the entry list is already populated) so we return
// nil.
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update is the Bubbletea message loop.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w, h := 80, 20
		if msg.Width > 0 {
			w = msg.Width
		}
		if msg.Height > 0 {
			h = msg.Height
		}
		m.list.SetSize(w, h-2) // leave room for the help line
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// handleKey dispatches key events. 'a' attaches (the main
// action), 'e' opens in an editor, 'q'/esc/ctrl+c quits, and
// enter triggers the confirmation prompt before resolving.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirmMode {
		switch msg.String() {
		case "y", "Y", "enter":
			m.answers = Answers{
				Action: ActionAttach,
				Path:   m.pickedPath,
				Label:  m.pickedLabel,
			}
			return m, tea.Quit
		case "n", "N", "esc":
			m.confirmMode = false
			return m, nil
		}
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit
	case "a":
		return m, m.confirmSelected()
	case "e":
		p := m.currentPath()
		m.answers = Answers{
			Action: ActionOpenEditor,
			Path:   p,
			Label:  m.labelForPath(p),
		}
		return m, tea.Quit
	case "enter":
		return m, m.confirmSelected()
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// confirmSelected puts the model in confirm mode for the
// currently-selected row. Returns a tea.Cmd that callers wire
// into Update (we don't have one to run, so it's nil).
func (m *Model) confirmSelected() tea.Cmd {
	p := m.currentPath()
	if p == "" {
		return nil
	}
	m.pickedPath = p
	m.pickedLabel = m.labelForPath(p)
	m.confirmMode = true
	return nil
}

// currentPath returns the absolute path of the currently
// highlighted row, or "" if the list is empty.
func (m *Model) currentPath() string {
	it := m.list.SelectedItem()
	if it == nil {
		return ""
	}
	e, ok := it.(Entry)
	if !ok {
		return ""
	}
	return e.Path
}

// labelForPath looks up the label for the given path in the
// entries slice. Returns "" if no label is set.
func (m *Model) labelForPath(path string) string {
	for _, it := range m.entries {
		e, ok := it.(Entry)
		if !ok {
			continue
		}
		if e.Path == path {
			return e.Label
		}
	}
	return ""
}

// cancelled reports whether the user quit without confirming.
func (m *Model) cancelled() bool {
	return m.answers.Action == ActionNone
}

// View renders the picker. In confirm mode the list is
// replaced by a single-line prompt; otherwise the embedded
// bubbles/list view is shown with a help footer.
func (m *Model) View() string {
	if m.confirmMode {
		label := m.pickedLabel
		if label == "" {
			label = m.pickedPath
		}
		prompt := fmt.Sprintf("Attach to %s? [y/N]", label)
		return m.theme.Body.Render(prompt)
	}
	help := m.theme.Muted.Render("a: attach · e: open in editor · /: filter · q: quit")
	return m.list.View() + "\n" + help
}

// listDelegate adapts an Entry to the bubbles/list.ItemDelegate
// interface. The theme is captured at construction time (we
// always .Apply() before constructing the model).
type listDelegate struct {
	theme tui.Theme
}

// Render renders one row of the picker. The bubbles v1.0.0
// ItemDelegate signature takes an io.Writer and writes directly
// to it (no string return). We render a single-line title and a
// one-line description.
func (d listDelegate) Render(w io.Writer, _ list.Model, _ int, item list.Item) {
	e, ok := item.(Entry)
	if !ok {
		return
	}
	title := d.theme.Selected.Render(e.Title())
	desc := d.theme.Muted.Render(e.Description())
	_, _ = io.WriteString(w, title)
	_, _ = io.WriteString(w, "\n")
	_, _ = io.WriteString(w, desc)
}

// Height returns the row height (always 2: title + description).
func (d listDelegate) Height() int { return 2 }

// Spacing returns the inter-row gap. Zero keeps the picker
// compact.
func (d listDelegate) Spacing() int { return 0 }

// Update is unused (the list doesn't push messages to the
// delegate), but the interface requires it.
func (d listDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
