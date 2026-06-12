package dashwiz

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/tui"
)

// tab enumerates the dashboard's five tabs. Order matches the
// spec §14 "top to bottom in the tab bar".
type tab int

const (
	tabStatus tab = iota
	tabBranches
	tabRemotes
	tabGraph
	tabPlugins
)

// tabCount is the number of tabs. It must be kept in sync with the
// tab constants above.
const tabCount = 5

// tabMeta is the per-tab display metadata: a tab discriminator
// (so callers can look up the meta by tab index), the visible
// title, a one-line description, and a comingSoon flag. The
// description appears in the status line under the tab bar.
type tabMeta struct {
	tab         tab
	title       string
	description string
	// comingSoon, when true, renders the read-only banner that
	// says real mutations / interactive previews are coming in
	// v0.2 (per spec §14).
	comingSoon bool
}

var tabMetas = []tabMeta{
	{tabStatus: "Status", title: "Status", description: "Working tree state (git status)", comingSoon: false},
	{tabBranches: "Branches", title: "Branches", description: "Local branches + current/upstream", comingSoon: false},
	{tabRemotes: "Remotes", title: "Remotes", description: "Read-only: mutation lands in v0.2", comingSoon: true},
	{tabGraph: "Graph", title: "Graph", description: "Read-only preview: interactive renderer in v0.2", comingSoon: true},
	{tabPlugins: "Plugins", title: "Plugins", description: "Read-only: interactive loader in v0.2", comingSoon: true},
}

// Model is the Bubbletea model for the dashboard. It holds a
// snapshot of repo state (the Inputs) and per-tab views: a
// bubbles/list for Status, a bubbles/table for Branches, and
// hand-rolled list views for the three read-only tabs (they don't
// need a full list model since the user can't mutate them).
type Model struct {
	inputs Inputs
	theme  tui.Theme
	active tab

	// Per-tab views. The two interactive ones are full bubbles
	// components; the three read-only ones are nil (rendered by
	// the per-tab view helper).
	statusList  list.Model
	branchTable table.Model

	// Window size.
	width  int
	height int
}

// NewModel constructs a Model ready to run. The Status list and
// Branches table are built from the Inputs; the active tab is
// Status (the first one in the bar).
func NewModel(inputs Inputs, theme tui.Theme) *Model {
	theme = theme.Apply()
	m := &Model{
		inputs: inputs,
		theme:  theme,
		active: tabStatus,
		width:  100,
		height: 24,
	}
	m.statusList = m.buildStatusList()
	m.branchTable = m.buildBranchTable()
	return m
}

// Init implements tea.Model. We have no commands to run at startup
// (everything is pre-populated from the Inputs snapshot) so we
// return nil.
func (m *Model) Init() tea.Cmd { return nil }

// Update implements tea.Model. Handles window size + keys.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.statusList.SetSize(m.listWidth(), m.listHeight())
		// bubbles/table uses SetWidth/SetHeight.
		m.branchTable.SetWidth(m.width - 4)
		m.branchTable.SetHeight(m.tableHeight())
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	// Forward non-key messages to the active tab's view (for
	// mouse / ticks / etc.).
	switch m.active {
	case tabStatus:
		var cmd tea.Cmd
		m.statusList, cmd = m.statusList.Update(msg)
		return m, cmd
	case tabBranches:
		m.branchTable, _ = m.branchTable.Update(msg)
	}
	return m, nil
}

// handleKey dispatches key events. Tab navigation is left/right
// (and the digit keys 1..5 as a shortcut). 'q'/esc/ctrl+c quit.
// The active tab's keys are forwarded to its bubbles component.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch k {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit
	case "left", "h", "[":
		m.active = (m.active - 1 + tabCount) % tabCount
		return m, nil
	case "right", "l", "]":
		m.active = (m.active + 1) % tabCount
		return m, nil
	case "1":
		m.active = tabStatus
		return m, nil
	case "2":
		m.active = tabBranches
		return m, nil
	case "3":
		m.active = tabRemotes
		return m, nil
	case "4":
		m.active = tabGraph
		return m, nil
	case "5":
		m.active = tabPlugins
		return m, nil
	}
	// Forward to the active tab's view.
	switch m.active {
	case tabStatus:
		var cmd tea.Cmd
		m.statusList, cmd = m.statusList.Update(msg)
		return m, cmd
	case tabBranches:
		m.branchTable, _ = m.branchTable.Update(msg)
	}
	return m, nil
}

// View implements tea.Model. Renders the tab bar, the active tab,
// and a help footer.
func (m *Model) View() string {
	var b strings.Builder
	b.WriteString(m.renderTabBar())
	b.WriteString("\n")
	b.WriteString(m.renderActive())
	b.WriteString("\n")
	b.WriteString(m.renderHelp())
	return b.String()
}

// renderTabBar paints the five tab labels, with the active one
// highlighted.
func (m *Model) renderTabBar() string {
	parts := make([]string, 0, tabCount)
	for i, meta := range tabMetas {
		label := fmt.Sprintf(" %d %s ", i+1, meta.title)
		if tab(i) == m.active {
			parts = append(parts, m.theme.Selected.Render(label))
		} else {
			parts = append(parts, m.theme.Unselected.Render(label))
		}
	}
	return strings.Join(parts, " ")
}

// renderActive dispatches to the per-tab view helper.
func (m *Model) renderActive() string {
	switch m.active {
	case tabStatus:
		return m.renderStatusTab()
	case tabBranches:
		return m.renderBranchesTab()
	case tabRemotes:
		return m.renderRemotesTab()
	case tabGraph:
		return m.renderGraphTab()
	case tabPlugins:
		return m.renderPluginsTab()
	}
	return ""
}

// renderHelp paints the footer: tab-description on the left, key
// hints on the right.
func (m *Model) renderHelp() string {
	meta := tabMetas[m.active]
	left := m.theme.Muted.Render(meta.description)
	right := m.theme.Hint.Render("←/→ or 1-5: switch tab  ·  q: quit")
	return left + "    " + right
}

// listWidth / listHeight return the dimensions the embedded
// bubbles/list should use, leaving room for the tab bar and help
// footer.
func (m *Model) listWidth() int {
	w := m.width - 2
	if w < 20 {
		w = 20
	}
	return w
}

func (m *Model) listHeight() int {
	h := m.height - 6
	if h < 5 {
		h = 5
	}
	return h
}

func (m *Model) tableHeight() int {
	h := m.height - 6
	if h < 5 {
		h = 5
	}
	return h
}

// renderStatusTab is the real Status tab. It shows the git status
// branches/header line, then a bubbles/list of entries grouped by
// staged / unstaged / untracked.
func (m *Model) renderStatusTab() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Status  "))
	b.WriteString("\n")
	b.WriteString(m.theme.Body.Render(m.statusHeaderLine()))
	b.WriteString("\n\n")
	if len(m.statusList.Items()) == 0 {
		b.WriteString(m.theme.Muted.Render("  Nothing to display (clean working tree)."))
		b.WriteString("\n")
		return b.String()
	}
	b.WriteString(m.statusList.View())
	b.WriteString("\n")
	return b.String()
}

// statusHeaderLine returns the first line(s) the Status tab
// prints before the entry list. It mirrors writeGitStatusHuman's
// header so the dashboard and `got status` agree.
func (m *Model) statusHeaderLine() string {
	s := m.inputs.Status
	if s.Detached {
		return "On detached HEAD"
	}
	if s.Branch != "" {
		line := "On branch " + s.Branch
		if s.Upstream != "" {
			switch {
			case s.Ahead > 0 && s.Behind > 0:
				line += fmt.Sprintf(" (diverged from %s: %d ahead, %d behind)", s.Upstream, s.Ahead, s.Behind)
			case s.Ahead > 0:
				line += fmt.Sprintf(" (ahead of %s by %d commit)", s.Upstream, s.Ahead)
			case s.Behind > 0:
				line += fmt.Sprintf(" (behind %s by %d commit)", s.Upstream, s.Behind)
			default:
				line += " (up to date with " + s.Upstream + ")"
			}
		}
		return line
	}
	return ""
}

// buildStatusList builds the bubbles/list shown in the Status
// tab. We use a list rather than the table component because the
// status entries vary in shape (staged/unstaged/untracked have
// different prefixes) and list's title per item is easier to
// style.
func (m *Model) buildStatusList() list.Model {
	items := make([]list.Item, 0)
	for _, e := range m.inputs.Status.Entries {
		items = append(items, statusListItem{entry: e})
	}
	// SetSize is called on the first WindowSizeMsg; the initial
	// size is just a sensible default.
	l := list.New(items, statusListDelegate{theme: m.theme}, 96, 18)
	l.Title = "Entries"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	return l
}

// statusListItem adapts a git.StatusEntry to bubbles/list.Item.
type statusListItem struct {
	entry git.StatusEntry
}

func (s statusListItem) FilterValue() string { return s.entry.Path }
func (s statusListItem) Title() string {
	return statusItemTitle(s.entry)
}

func (s statusListItem) Description() string {
	return ""
}

// statusItemTitle returns the human-readable title for a status
// entry, including the staged/unstaged prefix.
func statusItemTitle(e git.StatusEntry) string {
	switch {
	case e.IsUntracked:
		return "??  " + e.Path
	case e.IsRenamed:
		return fmt.Sprintf("R   %s -> %s", e.OriginalPath, e.Path)
	case e.IsStaged && e.IsUnstaged:
		return e.XY + "  " + e.Path
	case e.IsStaged:
		return string(e.XY[0]) + "-  " + e.Path
	case e.IsUnstaged:
		return "-" + string(e.XY[1]) + "  " + e.Path
	}
	return "--  " + e.Path
}

// statusListDelegate renders a status list row. The bubbles
// v1.0.0 ItemDelegate signature is Render(w io.Writer, m list.Model,
// index int, item list.Item) (no return).
type statusListDelegate struct {
	theme tui.Theme
}

func (d statusListDelegate) Render(w io.Writer, _ list.Model, _ int, item list.Item) {
	s, ok := item.(statusListItem)
	if !ok {
		return
	}
	_, _ = fmt.Fprintf(w, "%s", s.Title())
}
func (d statusListDelegate) Height() int  { return 1 }
func (d statusListDelegate) Spacing() int { return 0 }
func (d statusListDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

// renderBranchesTab is the real Branches tab. It shows a
// bubbles/table with the current branch, name, short SHA, and
// upstream.
func (m *Model) renderBranchesTab() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Branches  "))
	b.WriteString("\n")
	if len(m.inputs.Branches) == 0 {
		b.WriteString(m.theme.Muted.Render("  (no local branches)"))
		b.WriteString("\n")
		return b.String()
	}
	b.WriteString(m.branchTable.View())
	b.WriteString("\n")
	return b.String()
}

// buildBranchTable builds the bubbles/table shown in the Branches
// tab. Columns: name (with "(current)" marker), short SHA,
// upstream.
func (m *Model) buildBranchTable() table.Model {
	columns := []table.Column{
		{Title: "Branch", Width: 30},
		{Title: "SHA", Width: 8},
		{Title: "Upstream", Width: 30},
	}
	rows := make([]table.Row, 0, len(m.inputs.Branches))
	for _, br := range m.inputs.Branches {
		name := br.Name
		if br.IsCurrent {
			name = "* " + br.Name
		}
		sha := br.SHA
		if len(sha) > 7 {
			sha = sha[:7]
		}
		rows = append(rows, table.Row{name, sha, br.Upstream})
	}
	t := table.New(columns, rows)
	t.SetHeight(20)
	// The default bubbles/table styles are unstyled; the
	// dashboard applies the GOT theme so the table blends in
	// with the rest of the surface.
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderTop(false).
		BorderBottom(true).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(false)
	s.Selected = s.Selected.
		Foreground(m.theme.Selected.GetForeground()).
		Bold(true)
	t.SetStyles(s)
	return t
}

// renderRemotesTab is the read-only Remotes tab. It shows a small
// table of configured remotes with a v0.2 banner. Per spec §14,
// mutation is CLI-only in v0.1; the dashboard previews what the
// CLI operates on.
func (m *Model) renderRemotesTab() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Remotes  "))
	b.WriteString("\n")
	if len(m.inputs.Remotes) == 0 {
		b.WriteString(m.theme.Muted.Render("  (no remotes configured)"))
		b.WriteString("\n")
	} else {
		for _, r := range m.inputs.Remotes {
			line := fmt.Sprintf("  %s\t%s", r.Name, r.FetchURL)
			if r.FetchURL != r.PushURL && r.PushURL != "" {
				line += "  (push: " + r.PushURL + ")"
			}
			b.WriteString(m.theme.Body.Render(line))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(m.comingSoonBanner("Real interactive mutations land in v0.2"))
	b.WriteString("\n")
	return b.String()
}

// renderGraphTab is the read-only Graph tab. It shows a 20-line
// preview of `got graph --no-tui` and the v0.2 banner.
func (m *Model) renderGraphTab() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Graph  "))
	b.WriteString("\n")
	preview := m.inputs.GraphPreview
	if preview == "" {
		b.WriteString(m.theme.Muted.Render("  (no graph available)"))
		b.WriteString("\n")
	} else {
		// Truncate to at most 20 lines so the tab is compact
		// even in a deep repo.
		lines := strings.Split(preview, "\n")
		if len(lines) > 20 {
			lines = lines[:20]
		}
		for _, line := range lines {
			b.WriteString(m.theme.Body.Render("  " + line))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(m.comingSoonBanner("Real interactive renderer lands in v0.2"))
	b.WriteString("\n")
	return b.String()
}

// renderPluginsTab is the read-only Plugins tab. It shows the
// discovered plugins (name + version + commands) with the v0.2
// banner.
func (m *Model) renderPluginsTab() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Plugins  "))
	b.WriteString("\n")
	if len(m.inputs.Plugins) == 0 {
		b.WriteString(m.theme.Muted.Render("  (no plugins discovered)"))
		b.WriteString("\n")
	} else {
		for _, p := range m.inputs.Plugins {
			b.WriteString(m.theme.Body.Render(fmt.Sprintf("  %s %s  [%s]  %d commands", p.Name, p.Version, p.Source, len(p.Commands))))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(m.comingSoonBanner("Real interactive loader lands in v0.2"))
	b.WriteString("\n")
	return b.String()
}

// comingSoonBanner renders the per-spec v0.2 placeholder banner.
// Per spec §14 the read-only tabs MUST carry a visible "Coming in
// v0.2" message so users understand the panel is a preview, not
// the real thing.
func (m *Model) comingSoonBanner(msg string) string {
	box := m.theme.Box.Render("  " + msg + "  ")
	return box
}
