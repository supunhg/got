package dashwiz

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/plugin"
	"github.com/got-sh/got/internal/tui"
)

func sampleInputs() Inputs {
	return Inputs{
		WorkTree: "/tmp/repo",
		Status: git.Status{
			Branch:   "main",
			Upstream: "origin/main",
			Entries: []git.StatusEntry{
				{Path: "staged.txt", XY: "A ", IsStaged: true},
				{Path: "dirty.txt", XY: " M", IsUnstaged: true},
				{Path: "fresh.txt", IsUntracked: true},
			},
		},
		Branches: []git.Branch{
			{Name: "main", IsCurrent: true, SHA: "abc1234", Upstream: "origin/main"},
			{Name: "feature/x", SHA: "def5678"},
		},
		Remotes: []git.Remote{
			{Name: "origin", FetchURL: "git@github.com:foo/bar.git"},
		},
		GraphPreview: "* abc1234 (HEAD -> main, origin/main) feat: foo\n* def5678 (feature/x) feat: bar\n",
		Plugins: []plugin.DiscoveredPlugin{
			{Name: "github", Version: "1.0.0", Source: "PATH"},
		},
	}
}

func TestNewModel_DefaultsToStatusTab(t *testing.T) {
	m := NewModel(sampleInputs(), tui.NoColorTheme())
	if m.active != tabStatus {
		t.Errorf("active = %d, want %d (Status tab is the default)", m.active, tabStatus)
	}
}

func TestNewModel_EmptyInputs(t *testing.T) {
	m := NewModel(Inputs{}, tui.NoColorTheme())
	// Should not panic; renders a clean empty state.
	view := m.View()
	if !strings.Contains(view, "Status") {
		t.Errorf("View = %q, expected 'Status' label", view)
	}
}

func TestModel_HandleKey_QuitOnQ(t *testing.T) {
	m := NewModel(sampleInputs(), tui.NoColorTheme())
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Errorf("expected tea.Quit cmd on 'q', got nil")
	}
}

func TestModel_HandleKey_QuitOnEsc(t *testing.T) {
	m := NewModel(sampleInputs(), tui.NoColorTheme())
	_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
}

func TestModel_HandleKey_QuitOnCtrlC(t *testing.T) {
	m := NewModel(sampleInputs(), tui.NoColorTheme())
	_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
}

func TestModel_HandleKey_RightSwitchesTab(t *testing.T) {
	m := NewModel(sampleInputs(), tui.NoColorTheme())
	m.handleKey(tea.KeyMsg{Type: tea.KeyRight})
	if m.active != tabBranches {
		t.Errorf("after 'right' active = %d, want %d (Branches)", m.active, tabBranches)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRight})
	if m.active != tabRemotes {
		t.Errorf("after 2x 'right' active = %d, want %d (Remotes)", m.active, tabRemotes)
	}
}

func TestModel_HandleKey_LeftSwitchesBack(t *testing.T) {
	m := NewModel(sampleInputs(), tui.NoColorTheme())
	m.handleKey(tea.KeyMsg{Type: tea.KeyRight})
	m.handleKey(tea.KeyMsg{Type: tea.KeyRight})
	m.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	if m.active != tabBranches {
		t.Errorf("after right,right,left active = %d, want %d (Branches)", m.active, tabBranches)
	}
}

func TestModel_HandleKey_LeftWrapsFromStatusToPlugins(t *testing.T) {
	m := NewModel(sampleInputs(), tui.NoColorTheme())
	m.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	if m.active != tabPlugins {
		t.Errorf("after 'left' from Status active = %d, want %d (wraps to Plugins)", m.active, tabPlugins)
	}
}

func TestModel_HandleKey_RightWrapsFromPluginsToStatus(t *testing.T) {
	m := NewModel(sampleInputs(), tui.NoColorTheme())
	m.active = tabPlugins
	m.handleKey(tea.KeyMsg{Type: tea.KeyRight})
	if m.active != tabStatus {
		t.Errorf("after 'right' from Plugins active = %d, want %d (wraps to Status)", m.active, tabStatus)
	}
}

func TestModel_HandleKey_DigitShortcuts(t *testing.T) {
	m := NewModel(sampleInputs(), tui.NoColorTheme())
	for i := 1; i <= tabCount; i++ {
		m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{rune('0' + i)}})
		want := tab(i - 1)
		if m.active != want {
			t.Errorf("after pressing %d active = %d, want %d", i, m.active, want)
		}
	}
}

func TestModel_View_StatusTabHasBranchLine(t *testing.T) {
	m := NewModel(sampleInputs(), tui.NoColorTheme())
	view := m.View()
	if !strings.Contains(view, "On branch main") {
		t.Errorf("View = %q, expected 'On branch main' header", view)
	}
	if !strings.Contains(view, "staged.txt") {
		t.Errorf("View = %q, expected 'staged.txt' entry", view)
	}
}

func TestModel_View_BranchesTabHasTable(t *testing.T) {
	m := NewModel(sampleInputs(), tui.NoColorTheme())
	m.active = tabBranches
	view := m.View()
	if !strings.Contains(view, "main") {
		t.Errorf("View = %q, expected 'main' in Branches table", view)
	}
	if !strings.Contains(view, "abc1234") {
		t.Errorf("View = %q, expected short SHA 'abc1234' in Branches table", view)
	}
}

func TestModel_View_RemotesTabHasBanner(t *testing.T) {
	m := NewModel(sampleInputs(), tui.NoColorTheme())
	m.active = tabRemotes
	view := m.View()
	if !strings.Contains(view, "v0.2") {
		t.Errorf("View = %q, expected 'v0.2' coming-soon banner on Remotes tab", view)
	}
	if !strings.Contains(view, "origin") {
		t.Errorf("View = %q, expected 'origin' remote entry", view)
	}
}

func TestModel_View_GraphTabHasPreviewAndBanner(t *testing.T) {
	m := NewModel(sampleInputs(), tui.NoColorTheme())
	m.active = tabGraph
	view := m.View()
	if !strings.Contains(view, "feat: foo") {
		t.Errorf("View = %q, expected graph preview content", view)
	}
	if !strings.Contains(view, "v0.2") {
		t.Errorf("View = %q, expected 'v0.2' coming-soon banner on Graph tab", view)
	}
}

func TestModel_View_GraphTabTruncatesTo20Lines(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line"
	}
	m := NewModel(Inputs{GraphPreview: strings.Join(lines, "\n")}, tui.NoColorTheme())
	m.active = tabGraph
	view := m.View()
	// Count the actual lines rendered (excluding the banner).
	got := 0
	for _, l := range strings.Split(view, "\n") {
		if strings.HasPrefix(l, "  line") {
			got++
		}
	}
	if got > 20 {
		t.Errorf("graph tab rendered %d lines, want <= 20", got)
	}
}

func TestModel_View_PluginsTabHasListAndBanner(t *testing.T) {
	m := NewModel(sampleInputs(), tui.NoColorTheme())
	m.active = tabPlugins
	view := m.View()
	if !strings.Contains(view, "github") {
		t.Errorf("View = %q, expected 'github' plugin entry", view)
	}
	if !strings.Contains(view, "v0.2") {
		t.Errorf("View = %q, expected 'v0.2' coming-soon banner on Plugins tab", view)
	}
}

func TestModel_View_EmptyRemotesListIsFriendly(t *testing.T) {
	m := NewModel(Inputs{WorkTree: "/tmp/x"}, tui.NoColorTheme())
	m.active = tabRemotes
	view := m.View()
	if !strings.Contains(view, "no remotes") {
		t.Errorf("View = %q, expected 'no remotes' friendly message", view)
	}
}

func TestModel_View_TabBarShowsAllFive(t *testing.T) {
	m := NewModel(sampleInputs(), tui.NoColorTheme())
	view := m.View()
	for _, want := range []string{"Status", "Branches", "Remotes", "Graph", "Plugins"} {
		if !strings.Contains(view, want) {
			t.Errorf("View = %q, expected tab label %q in tab bar", view, want)
		}
	}
}

func TestStatusItemTitle(t *testing.T) {
	cases := []struct {
		name string
		ent  git.StatusEntry
		want string
	}{
		{"untracked", git.StatusEntry{Path: "x", IsUntracked: true}, "??  x"},
		{"renamed", git.StatusEntry{OriginalPath: "a", Path: "b", IsRenamed: true, XY: "R "}, "R   a -> b"},
		{"staged+unstaged", git.StatusEntry{Path: "x", XY: "MM", IsStaged: true, IsUnstaged: true}, "MM  x"},
		{"staged only", git.StatusEntry{Path: "x", XY: "A ", IsStaged: true}, "A-  x"},
		{"unstaged only", git.StatusEntry{Path: "x", XY: " M", IsUnstaged: true}, "-M  x"},
		{"default", git.StatusEntry{Path: "x"}, "--  x"},
	}
	for _, c := range cases {
		got := statusItemTitle(c.ent)
		if got != c.want {
			t.Errorf("%s: statusItemTitle = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestModel_Update_WindowSizePropagates(t *testing.T) {
	m := NewModel(sampleInputs(), tui.NoColorTheme())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.width != 120 || m.height != 40 {
		t.Errorf("size = %dx%d, want 120x40", m.width, m.height)
	}
}
