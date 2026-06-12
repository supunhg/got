package branchwiz

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/tui"
)

// key returns a tea.KeyMsg for the given key string. Drives the
// model directly in tests without a real terminal.
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

func sampleBranches() []git.Branch {
	return []git.Branch{
		{Name: "main", IsCurrent: true, SHA: "aaaaaaa"},
		{Name: "feature/a", IsCurrent: false, SHA: "bbbbbbb"},
		{Name: "feature/b", IsCurrent: false, SHA: "ccccccc"},
	}
}

func TestNewModel_DefaultsToMenu(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{}, tui.NoColorTheme())
	if m.state != stateMenu {
		t.Errorf("state = %d, want stateMenu (default)", m.state)
	}
	if m.answers.Action != "" {
		t.Errorf("Action = %q, want empty", m.answers.Action)
	}
}

func TestNewModel_PrePinnedAction(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{Action: ActionCreate, Name: "x"}, tui.NoColorTheme())
	if m.state != stateCreateName {
		t.Errorf("state = %d, want stateCreateName (pre-pinned Action=Create)", m.state)
	}
	if m.answers.Name != "x" {
		t.Errorf("Name = %q, want x (pre-populated)", m.answers.Name)
	}
}

func TestUpdate_MenuUpDown(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{}, tui.NoColorTheme())
	m = step(t, m, key("down"))
	if m.menuIdx != 1 {
		t.Errorf("after down: menuIdx = %d, want 1", m.menuIdx)
	}
	m = step(t, m, key("up"))
	if m.menuIdx != 0 {
		t.Errorf("after up: menuIdx = %d, want 0", m.menuIdx)
	}
}

func TestUpdate_MenuEnterCreate(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{}, tui.NoColorTheme())
	m = step(t, m, key("enter")) // menuIdx=0 = create
	// The create screen needs a name; the menu sets answers.Action
	// and the user is dropped on the name input.
	if m.answers.Action != ActionCreate {
		t.Errorf("Action = %q, want create after menu select", m.answers.Action)
	}
}

func TestUpdate_CreateNameEmptyBounces(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{Action: ActionCreate}, tui.NoColorTheme())
	// state is already stateCreateName.
	m = step(t, m, key("enter"))
	if m.state != stateCreateName {
		t.Errorf("empty name + Enter: state = %d, want stateCreateName (bounce)", m.state)
	}
}

func TestUpdate_CreateNameAdvances(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{Action: ActionCreate}, tui.NoColorTheme())
	m.nameIn.SetValue("feature/new")
	m = step(t, m, key("enter"))
	if m.answers.Name != "feature/new" {
		t.Errorf("Name = %q, want feature/new", m.answers.Name)
	}
	if m.state != stateCreateFrom {
		t.Errorf("state = %d, want stateCreateFrom", m.state)
	}
}

func TestUpdate_CreateFromAdvancesToConfirm(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{Action: ActionCreate, Name: "feature/new"}, tui.NoColorTheme())
	// We were put into stateCreateName; advance to stateCreateFrom.
	m = step(t, m, key("enter"))
	// stateCreateFrom with empty start point + Enter -> confirm.
	m = step(t, m, key("enter"))
	if m.state != stateConfirm {
		t.Errorf("state = %d, want stateConfirm", m.state)
	}
}

func TestUpdate_CheckoutPickerSetsName(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{Action: ActionCheckout}, tui.NoColorTheme())
	m.checkoutIdx = 1 // feature/a
	m = step(t, m, key("enter"))
	if m.answers.Name != "feature/a" {
		t.Errorf("Name = %q, want feature/a", m.answers.Name)
	}
	if m.state != stateConfirm {
		t.Errorf("state = %d, want stateConfirm", m.state)
	}
}

func TestUpdate_DeleteRefusesCurrentBranch(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{Action: ActionDelete}, tui.NoColorTheme())
	// deleteIdx=0 is "main", which IsCurrent.
	m.deleteIdx = 0
	m = step(t, m, key("enter"))
	if m.state != stateDelete {
		t.Errorf("current branch + Enter: state = %d, want stateDelete (bounce)", m.state)
	}
	if m.answers.Name != "" {
		t.Errorf("Name = %q, want empty (refused to set current branch)", m.answers.Name)
	}
}

func TestUpdate_DeleteAdvancesToForce(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{Action: ActionDelete}, tui.NoColorTheme())
	m.deleteIdx = 1 // feature/a
	m = step(t, m, key("enter"))
	if m.answers.Name != "feature/a" {
		t.Errorf("Name = %q, want feature/a", m.answers.Name)
	}
	if m.state != stateForce {
		t.Errorf("state = %d, want stateForce", m.state)
	}
}

func TestUpdate_ForceToggleAndEnter(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{Action: ActionDelete, Name: "feature/a"}, tui.NoColorTheme())
	// We need to get to stateForce. NewModel with pre.Action=Delete
	// jumps straight to stateDelete; move from there.
	m = step(t, m, key("enter")) // picks first branch (main, current -> bounce)
	if m.state != stateDelete {
		t.Fatalf("setup: state = %d, want stateDelete", m.state)
	}
	m.deleteIdx = 1
	m = step(t, m, key("enter")) // advances to stateForce
	if m.state != stateForce {
		t.Fatalf("after picking feature/a: state = %d, want stateForce", m.state)
	}
	// Toggle: tab to Yes.
	m = step(t, m, key("tab"))
	if m.forceIdx != 1 {
		t.Errorf("after tab: forceIdx = %d, want 1", m.forceIdx)
	}
	m = step(t, m, key("enter"))
	if !m.answers.Force {
		t.Errorf("Force = false; want true after toggle to Yes + Enter")
	}
	if m.state != stateConfirm {
		t.Errorf("state = %d, want stateConfirm", m.state)
	}
}

func TestUpdate_ConfirmYesCommits(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{Action: ActionCreate, Name: "feature/new"}, tui.NoColorTheme())
	// Advance from stateCreateName to stateCreateFrom to stateConfirm.
	m = step(t, m, key("enter"))
	m = step(t, m, key("enter"))
	if m.state != stateConfirm {
		t.Fatalf("setup: state = %d, want stateConfirm", m.state)
	}
	m.confirmIdx = 0
	m = step(t, m, key("enter"))
	if m.state != stateDone {
		t.Errorf("state = %d, want stateDone", m.state)
	}
}

func TestUpdate_ConfirmNoGoesBack(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{Action: ActionCreate, Name: "feature/new"}, tui.NoColorTheme())
	m = step(t, m, key("enter")) // -> stateCreateFrom
	m = step(t, m, key("enter")) // -> stateConfirm
	m.confirmIdx = 1
	m = step(t, m, key("enter"))
	if m.state != stateCreateFrom {
		t.Errorf("state = %d, want stateCreateFrom (back from confirm on create)", m.state)
	}
}

func TestUpdate_QCancels(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{}, tui.NoColorTheme())
	m = step(t, m, key("q"))
	if m.state != stateCancelled {
		t.Errorf("state = %d, want stateCancelled", m.state)
	}
}

func TestUpdate_CtrlCCancels(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{}, tui.NoColorTheme())
	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if m.state != stateCancelled {
		t.Errorf("state = %d, want stateCancelled", m.state)
	}
}

func TestUpdate_EscFromMenuCancels(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{}, tui.NoColorTheme())
	m = step(t, m, key("esc"))
	if m.state != stateCancelled {
		t.Errorf("state = %d, want stateCancelled (esc on menu = cancel)", m.state)
	}
}

func TestView_MenuHasEntries(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{}, tui.NoColorTheme())
	view := m.View()
	for _, want := range []string{"Branch", "Create", "Checkout", "Delete", "main"} {
		if !strings.Contains(view, want) {
			t.Errorf("View missing %q", want)
		}
	}
}

func TestView_ConfirmShowsAction(t *testing.T) {
	m := NewModel(sampleBranches(), PrePopulated{Action: ActionCreate, Name: "feature/x"}, tui.NoColorTheme())
	m = step(t, m, key("enter")) // -> stateCreateFrom
	m = step(t, m, key("enter")) // -> stateConfirm
	view := m.View()
	if !strings.Contains(view, "Create branch") {
		t.Errorf("Confirm view should describe the action; got:\n%s", view)
	}
	if !strings.Contains(view, "feature/x") {
		t.Errorf("Confirm view should include the branch name; got:\n%s", view)
	}
}

func TestCancelledErrorContract(t *testing.T) {
	if CancelledError == nil {
		t.Fatal("CancelledError is nil")
	}
	if !strings.Contains(CancelledError.Error(), "cancelled") {
		t.Errorf("CancelledError = %q, want it to mention 'cancelled'", CancelledError.Error())
	}
}
