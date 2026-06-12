package commitwiz

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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

func TestNewModel_PrePopulatedTypeAndScope(t *testing.T) {
	pre := PrePopulated{Type: "fix", Scope: "cli", Subject: "handle nil"}
	m := NewModel(nil, pre, NewHeuristicSuggester(), tui.NoColorTheme())
	if m.answers.Type != "fix" {
		t.Errorf("Type = %q, want fix", m.answers.Type)
	}
	if m.answers.Scope != "cli" {
		t.Errorf("Scope = %q, want cli", m.answers.Scope)
	}
	if m.answers.Subject != "handle nil" {
		t.Errorf("Subject = %q, want 'handle nil'", m.answers.Subject)
	}
}

func TestNewModel_HeuristicSuggesterPicksType(t *testing.T) {
	staged := []string{
		"internal/cli/init.go",
		"internal/cli/status.go",
		"internal/cli/commit.go",
	}
	m := NewModel(staged, PrePopulated{}, NewHeuristicSuggester(), tui.NoColorTheme())
	if m.answers.Scope != "cli" {
		t.Errorf("Scope = %q, want cli (from suggester)", m.answers.Scope)
	}
	// typeIdx should be on "feat" since the staged files are code
	// (not all-tests, not all-docs, not all-build).
	if m.answers.Type != "feat" {
		t.Errorf("Type = %q, want feat", m.answers.Type)
	}
	if m.typeIdx != 0 {
		// ConventionalTypes[0] is "feat". With 3 cli code files
		// (below the 6-file refactor threshold) the suggester
		// should leave the type on "feat", so typeIdx stays 0.
		t.Errorf("typeIdx = %d, want 0 (feat at 0; 3 cli files is below the refactor threshold)", m.typeIdx)
	}
}

func TestNewModel_HeuristicSuggesterPicksTest(t *testing.T) {
	staged := []string{"internal/cli/init_test.go", "internal/store/store_test.go"}
	m := NewModel(staged, PrePopulated{}, NewHeuristicSuggester(), tui.NoColorTheme())
	if m.answers.Type != "test" {
		t.Errorf("Type = %q, want test (heuristic on all-test staged set)", m.answers.Type)
	}
}

func TestUpdate_TypeScreenUpDown(t *testing.T) {
	m := NewModel(nil, PrePopulated{}, NewHeuristicSuggester(), tui.NoColorTheme())
	m.state = stateType
	m.typeIdx = 0
	m = step(t, m, key("down"))
	if m.typeIdx != 1 {
		t.Errorf("after down: typeIdx = %d, want 1", m.typeIdx)
	}
	m = step(t, m, key("up"))
	if m.typeIdx != 0 {
		t.Errorf("after up: typeIdx = %d, want 0", m.typeIdx)
	}
}

func TestUpdate_TypeScreenSelectsType(t *testing.T) {
	m := NewModel(nil, PrePopulated{}, NewHeuristicSuggester(), tui.NoColorTheme())
	m.state = stateType
	m.typeIdx = 1 // "fix"
	m = step(t, m, key("enter"))
	if m.answers.Type != "fix" {
		t.Errorf("Type = %q, want fix", m.answers.Type)
	}
}

func TestUpdate_TypeScreenSAcceptsSuggestion(t *testing.T) {
	staged := []string{"internal/cli/init.go", "internal/cli/status.go"}
	m := NewModel(staged, PrePopulated{}, NewHeuristicSuggester(), tui.NoColorTheme())
	m.state = stateType
	if m.answers.Scope != "cli" {
		t.Fatalf("precondition: scope = %q, want cli", m.answers.Scope)
	}
	m = step(t, m, key("s"))
	// 's' should leave the suggestion's scope and type in place.
	if m.answers.Scope != "cli" {
		t.Errorf("after s: Scope = %q, want cli", m.answers.Scope)
	}
}

func TestUpdate_SubjectRequired(t *testing.T) {
	m := NewModel(nil, PrePopulated{}, NewHeuristicSuggester(), tui.NoColorTheme())
	m.state = stateSubject
	// Empty subject: should bounce.
	m = step(t, m, key("enter"))
	if m.state != stateSubject {
		t.Errorf("empty subject + Enter: state = %d, want stateSubject (bounce)", m.state)
	}
}

func TestUpdate_SubjectAdvancesOnNonEmpty(t *testing.T) {
	m := NewModel(nil, PrePopulated{}, NewHeuristicSuggester(), tui.NoColorTheme())
	m.state = stateSubject
	m.subjectIn.SetValue("add a thing")
	m = step(t, m, key("enter"))
	if m.answers.Subject != "add a thing" {
		t.Errorf("Subject = %q, want 'add a thing'", m.answers.Subject)
	}
	if m.state == stateSubject {
		t.Errorf("state did not advance from stateSubject")
	}
}

func TestUpdate_BreakingRequiresReason(t *testing.T) {
	m := NewModel(nil, PrePopulated{AllowBreaking: true}, NewHeuristicSuggester(), tui.NoColorTheme())
	m.state = stateBreaking
	m.breakingIdx = 1 // Yes
	m = step(t, m, key("enter"))
	// Empty reason: should bounce, leaving Breaking == false and
	// breakingIdx reset to 0.
	if m.answers.Breaking {
		t.Errorf("Breaking = true; want false when reason is empty (the wizard must refuse to mark a breaking change without a reason)")
	}
	if m.breakingIdx != 0 {
		t.Errorf("breakingIdx = %d, want 0 (the toggle should reset on bounce)", m.breakingIdx)
	}
}

func TestUpdate_BreakingSetsReason(t *testing.T) {
	m := NewModel(nil, PrePopulated{AllowBreaking: true}, NewHeuristicSuggester(), tui.NoColorTheme())
	m.state = stateBreaking
	m.breakingIdx = 1
	m.breakIn.SetValue("requires git 2.30+")
	m = step(t, m, key("enter"))
	if !m.answers.Breaking {
		t.Errorf("Breaking = false; want true after reason entered")
	}
	if m.answers.BreakingReason != "requires git 2.30+" {
		t.Errorf("BreakingReason = %q, want 'requires git 2.30+'", m.answers.BreakingReason)
	}
}

func TestUpdate_ConfirmYesCommits(t *testing.T) {
	m := NewModel(nil, PrePopulated{Type: "fix", Scope: "cli", Subject: "x"}, NewHeuristicSuggester(), tui.NoColorTheme())
	m.state = stateConfirm
	m.confirmIdx = 0
	m = step(t, m, key("enter"))
	if m.state != stateDone {
		t.Errorf("state = %d, want stateDone", m.state)
	}
}

func TestUpdate_ConfirmNoGoesBack(t *testing.T) {
	m := NewModel(nil, PrePopulated{Type: "fix", Scope: "cli", Subject: "x"}, NewHeuristicSuggester(), tui.NoColorTheme())
	m.state = stateConfirm
	m.confirmIdx = 1
	m = step(t, m, key("enter"))
	if m.state != stateSubject {
		t.Errorf("state = %d, want stateSubject (back from confirm)", m.state)
	}
}

func TestUpdate_StageReviewSelectsEntries(t *testing.T) {
	m := NewModel([]string{"a.go", "b.go"}, PrePopulated{}, NewHeuristicSuggester(), tui.NoColorTheme())
	m.state = stateStageReview
	m = step(t, m, key(" ")) // toggle index 0
	if !m.stageSelected[0] {
		t.Errorf("after space: stageSelected[0] = false, want true")
	}
	m = step(t, m, key("j")) // move to index 1
	if m.stageCursor != 1 {
		t.Errorf("after j: stageCursor = %d, want 1", m.stageCursor)
	}
	m = step(t, m, key("enter"))
	if got := m.answers.StagedAfter; len(got) != 1 || got[0] != "a.go" {
		t.Errorf("StagedAfter = %v, want [a.go]", got)
	}
}

func TestUpdate_StageReviewNSkips(t *testing.T) {
	m := NewModel([]string{"a.go"}, PrePopulated{Type: "fix", Scope: "cli", Subject: "x"}, NewHeuristicSuggester(), tui.NoColorTheme())
	m.state = stateStageReview
	m = step(t, m, key("n"))
	if m.state == stateStageReview {
		t.Errorf("n should have advanced past stage review")
	}
}

func TestUpdate_EscCancelsFromType(t *testing.T) {
	m := NewModel(nil, PrePopulated{}, NewHeuristicSuggester(), tui.NoColorTheme())
	m.state = stateType
	m = step(t, m, key("esc"))
	if m.state != stateStageReview {
		t.Errorf("state = %d, want stateStageReview (back from type)", m.state)
	}
}

func TestUpdate_QCancelsFromAnywhere(t *testing.T) {
	m := NewModel(nil, PrePopulated{}, NewHeuristicSuggester(), tui.NoColorTheme())
	m.state = stateType
	m = step(t, m, key("q"))
	if m.state != stateCancelled {
		t.Errorf("state = %d, want stateCancelled", m.state)
	}
}

func TestUpdate_CtrlCCancels(t *testing.T) {
	m := NewModel(nil, PrePopulated{}, NewHeuristicSuggester(), tui.NoColorTheme())
	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if m.state != stateCancelled {
		t.Errorf("state = %d, want stateCancelled", m.state)
	}
}

func TestView_StageReviewShowsEntries(t *testing.T) {
	m := NewModel([]string{"a.go", "b.go"}, PrePopulated{}, NewHeuristicSuggester(), tui.NoColorTheme())
	view := m.View()
	for _, want := range []string{"Stage review", "a.go", "b.go", "Space"} {
		if !strings.Contains(view, want) {
			t.Errorf("View missing %q", want)
		}
	}
}

func TestView_TypeShowsSuggestion(t *testing.T) {
	staged := []string{"internal/cli/init.go", "internal/cli/status.go"}
	m := NewModel(staged, PrePopulated{}, NewHeuristicSuggester(), tui.NoColorTheme())
	m.state = stateType
	view := m.View()
	if !strings.Contains(view, "Suggested:") {
		t.Errorf("expected 'Suggested:' in type view, got:\n%s", view)
	}
}

func TestView_ConfirmShowsRenderedMessage(t *testing.T) {
	m := NewModel(nil, PrePopulated{Type: "feat", Scope: "cli", Subject: "add foo"}, NewHeuristicSuggester(), tui.NoColorTheme())
	m.state = stateConfirm
	view := m.View()
	for _, want := range []string{"feat(cli): add foo", "Commit", "Go back"} {
		if !strings.Contains(view, want) {
			t.Errorf("View missing %q", want)
		}
	}
}

func TestAnswers_RenderMinimal(t *testing.T) {
	a := Answers{Type: "fix", Subject: "handle nil"}
	got := a.Render()
	if got != "fix: handle nil\n" {
		t.Errorf("Render = %q, want %q", got, "fix: handle nil\n")
	}
}

func TestAnswers_RenderWithBreaking(t *testing.T) {
	a := Answers{Type: "feat", Scope: "api", Subject: "drop v1", Breaking: true, BreakingReason: "requires v2 client"}
	got := a.Render()
	if !strings.Contains(got, "BREAKING CHANGE: requires v2 client") {
		t.Errorf("Render missing BREAKING CHANGE footer:\n%s", got)
	}
	if strings.Contains(got, "!:") {
		t.Errorf("Render with footer should not also emit '!':\n%s", got)
	}
}

func TestDefaults_DefaultTypeIsFeat(t *testing.T) {
	d := Defaults()
	if d.Type != "feat" {
		t.Errorf("Type = %q, want feat", d.Type)
	}
	if d.Subject != "" {
		t.Errorf("Subject = %q, want empty (user must always type one)", d.Subject)
	}
}

func TestRunReturnsCancelledContract(t *testing.T) {
	if CancelledError == nil {
		t.Fatal("CancelledError is nil")
	}
	if !strings.Contains(CancelledError.Error(), "cancelled") {
		t.Errorf("CancelledError = %q, want it to mention 'cancelled'", CancelledError.Error())
	}
}
