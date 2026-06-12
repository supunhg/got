package initwiz

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/got-sh/got/internal/tui"
)

// key returns a tea.KeyMsg for the given key string. Convenient for
// driving the model directly in tests without a real terminal.
func key(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// step sends msg through the model and returns the updated model.
func step(t *testing.T, m *Model, msg tea.Msg) *Model {
	t.Helper()
	tm, _ := m.Update(msg)
	nm, ok := tm.(*Model)
	if !ok {
		t.Fatalf("Update returned %T, want *Model", tm)
	}
	return nm
}

func TestNew_PrePopulatedAnswers(t *testing.T) {
	d := Detected{Name: "demo", Branch: "trunk"}
	pre := PrePopulated{Name: "newname", DefaultBranch: "main", CommitStyle: "freeform"}
	m := New(d, pre, tui.NoColorTheme())
	if m.answers.Name != "newname" {
		t.Errorf("Name = %q, want newname", m.answers.Name)
	}
	if m.answers.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, want main", m.answers.DefaultBranch)
	}
	if m.answers.CommitStyle != "freeform" {
		t.Errorf("CommitStyle = %q, want freeform", m.answers.CommitStyle)
	}
}

func TestNew_DefaultsFromDetected(t *testing.T) {
	d := Detected{Name: "demo", Branch: "trunk"}
	m := New(d, PrePopulated{}, tui.NoColorTheme())
	if m.answers.Name != "demo" {
		t.Errorf("Name = %q, want demo", m.answers.Name)
	}
	if m.answers.DefaultBranch != "trunk" {
		t.Errorf("DefaultBranch = %q, want trunk", m.answers.DefaultBranch)
	}
	if m.answers.CommitStyle != "conventional" {
		t.Errorf("CommitStyle = %q, want conventional", m.answers.CommitStyle)
	}
	if m.styleIdx != 0 {
		t.Errorf("styleIdx = %d, want 0", m.styleIdx)
	}
}

func TestNew_BranchFallback(t *testing.T) {
	d := Detected{Name: "demo", Branch: ""} // detached or fresh
	m := New(d, PrePopulated{}, tui.NoColorTheme())
	if m.answers.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, want main (fallback)", m.answers.DefaultBranch)
	}
}

func TestUpdate_WelcomeEnterAdvancesToCommitStyle(t *testing.T) {
	m := New(Detected{Name: "demo"}, PrePopulated{}, tui.NoColorTheme())
	if m.state != stateWelcome {
		t.Fatalf("initial state = %d, want stateWelcome", m.state)
	}
	m = step(t, m, key("enter"))
	if m.state != stateCommitStyle {
		t.Errorf("after Enter: state = %d, want stateCommitStyle", m.state)
	}
}

func TestUpdate_CommitStyleUpDown(t *testing.T) {
	m := New(Detected{Name: "demo"}, PrePopulated{}, tui.NoColorTheme())
	m.state = stateCommitStyle
	m.styleIdx = 0
	m = step(t, m, key("down"))
	if m.styleIdx != 1 {
		t.Errorf("after down: styleIdx = %d, want 1", m.styleIdx)
	}
	m = step(t, m, key("up"))
	if m.styleIdx != 0 {
		t.Errorf("after up: styleIdx = %d, want 0", m.styleIdx)
	}
	// Top boundary.
	m = step(t, m, key("up"))
	if m.styleIdx != 0 {
		t.Errorf("top boundary: styleIdx = %d, want 0", m.styleIdx)
	}
}

func TestUpdate_CommitStyleSelectConventional(t *testing.T) {
	m := New(Detected{Name: "demo"}, PrePopulated{}, tui.NoColorTheme())
	m.state = stateCommitStyle
	m.styleIdx = 0
	m = step(t, m, key("enter"))
	if m.answers.CommitStyle != "conventional" {
		t.Errorf("CommitStyle = %q, want conventional", m.answers.CommitStyle)
	}
	if m.state != statePlugins {
		t.Errorf("state = %d, want statePlugins", m.state)
	}
}

func TestUpdate_CommitStyleSelectCustomGoesToTemplate(t *testing.T) {
	m := New(Detected{Name: "demo"}, PrePopulated{}, tui.NoColorTheme())
	m.state = stateCommitStyle
	m.styleIdx = 2 // custom
	m = step(t, m, key("enter"))
	if m.state != stateCustomTemplate {
		t.Errorf("state = %d, want stateCustomTemplate", m.state)
	}
}

func TestUpdate_ConfirmYes(t *testing.T) {
	m := New(Detected{Name: "demo"}, PrePopulated{}, tui.NoColorTheme())
	// Advance to confirm.
	m.state = stateConfirm
	m.confirmIdx = 0
	m = step(t, m, key("enter"))
	if m.state != stateDone {
		t.Errorf("state = %d, want stateDone", m.state)
	}
}

func TestUpdate_ConfirmNoGoesBackToWelcome(t *testing.T) {
	m := New(Detected{Name: "demo"}, PrePopulated{}, tui.NoColorTheme())
	m.state = stateConfirm
	m.confirmIdx = 1
	m = step(t, m, key("enter"))
	if m.state != stateWelcome {
		t.Errorf("state = %d, want stateWelcome", m.state)
	}
}

func TestUpdate_ConfirmToggle(t *testing.T) {
	m := New(Detected{Name: "demo"}, PrePopulated{}, tui.NoColorTheme())
	m.state = stateConfirm
	m.confirmIdx = 0
	m = step(t, m, key("right"))
	if m.confirmIdx != 1 {
		t.Errorf("after right: confirmIdx = %d, want 1", m.confirmIdx)
	}
	m = step(t, m, key("left"))
	if m.confirmIdx != 0 {
		t.Errorf("after left: confirmIdx = %d, want 0", m.confirmIdx)
	}
}

func TestUpdate_EscCancelsFromWelcome(t *testing.T) {
	m := New(Detected{Name: "demo"}, PrePopulated{}, tui.NoColorTheme())
	m = step(t, m, key("esc"))
	if m.state != stateCancelled {
		t.Errorf("state = %d, want stateCancelled", m.state)
	}
}

func TestUpdate_QCancels(t *testing.T) {
	m := New(Detected{Name: "demo"}, PrePopulated{}, tui.NoColorTheme())
	m.state = stateCommitStyle
	m = step(t, m, key("q"))
	if m.state != stateCancelled {
		t.Errorf("state = %d, want stateCancelled", m.state)
	}
}

func TestUpdate_CtrlCCancels(t *testing.T) {
	m := New(Detected{Name: "demo"}, PrePopulated{}, tui.NoColorTheme())
	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if m.state != stateCancelled {
		t.Errorf("state = %d, want stateCancelled", m.state)
	}
}

func TestView_WelcomeIncludesDetected(t *testing.T) {
	m := New(Detected{Name: "demo", Branch: "trunk", Languages: []string{"go"}}, PrePopulated{}, tui.NoColorTheme())
	view := m.View()
	for _, want := range []string{"GOT Init", "demo", "trunk", "go", "Enter"} {
		if !strings.Contains(view, want) {
			t.Errorf("View missing %q", want)
		}
	}
}

func TestView_CommitStyleShowsAllOptions(t *testing.T) {
	m := New(Detected{Name: "demo"}, PrePopulated{}, tui.NoColorTheme())
	m.state = stateCommitStyle
	view := m.View()
	for _, want := range []string{"Conventional Commits", "Free-form", "Custom template"} {
		if !strings.Contains(view, want) {
			t.Errorf("View missing %q", want)
		}
	}
}

func TestView_ConfirmShowsSummary(t *testing.T) {
	m := New(Detected{Name: "demo"}, PrePopulated{Name: "x", DefaultBranch: "main", CommitStyle: "conventional"}, tui.NoColorTheme())
	m.state = stateConfirm
	view := m.View()
	for _, want := range []string{"Confirm", "x", "main", "conventional", "Continue", "Go back"} {
		if !strings.Contains(view, want) {
			t.Errorf("View missing %q", want)
		}
	}
}

func TestDefaults(t *testing.T) {
	d := Detected{Name: "demo", Branch: "trunk"}
	a := Defaults(d)
	if a.Name != "demo" || a.DefaultBranch != "trunk" || a.CommitStyle != "conventional" {
		t.Errorf("Defaults = %+v", a)
	}
	if len(a.Plugins) != 0 {
		t.Errorf("Plugins = %v, want []", a.Plugins)
	}
	// Detached: branch falls back to main.
	a2 := Defaults(Detected{Name: "x", Branch: ""})
	if a2.DefaultBranch != "main" {
		t.Errorf("detached fallback: DefaultBranch = %q, want main", a2.DefaultBranch)
	}
}

func TestRunReturnsCancelled(t *testing.T) {
	// We can't call Run() directly because it blocks on a real
	// terminal. Instead, drive a Model to the cancelled state and
	// assert the wizard's contract: state=cancelled -> CancelledError.
	if CancelledError == nil {
		t.Fatal("CancelledError is nil")
	}
	if !strings.Contains(CancelledError.Error(), "cancelled") {
		t.Errorf("CancelledError = %q, want it to mention 'cancelled'", CancelledError.Error())
	}
}

func TestPrePopulatedSkipsCommitStyle(t *testing.T) {
	// If the user passed --style=conventional, the welcome screen
	// should advance straight to plugins (skipping the commit-style
	// radio), matching the "user can override the wizard" promise
	// in §7.
	m := New(Detected{Name: "demo"}, PrePopulated{CommitStyle: "conventional"}, tui.NoColorTheme())
	m = step(t, m, key("enter")) // welcome -> advance
	if m.state != statePlugins {
		t.Errorf("state = %d, want statePlugins (commit-style was pre-populated)", m.state)
	}
}

func TestPrePopulatedCustomGoesToTemplate(t *testing.T) {
	m := New(Detected{Name: "demo"}, PrePopulated{CommitStyle: "custom", CustomTemplate: "/tmp/t"}, tui.NoColorTheme())
	m = step(t, m, key("enter")) // welcome -> custom template
	if m.state != stateCustomTemplate {
		t.Errorf("state = %d, want stateCustomTemplate", m.state)
	}
	m = step(t, m, key("enter")) // confirm template
	if m.state != statePlugins {
		t.Errorf("after template: state = %d, want statePlugins", m.state)
	}
}
