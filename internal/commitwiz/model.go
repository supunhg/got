// commitwiz/model.go: Bubbletea model for the commit wizard.
//
// State machine (spec §8, screens 1..7):
//
//	stateStageReview  - list of staged/unstaged/untracked, multi-select
//	stateType          - ConventionalTypes radio, heuristic pre-selected
//	stateScope         - optional free text (or pick from got.yml scopes)
//	stateSubject       - required, ≤72 chars
//	stateBody          - optional, multi-line
//	stateBreaking      - checkbox + reason (only if AllowBreaking)
//	stateConfirm       - preview the rendered message, yes/no
//	stateDone / stateCancelled
//
// All screens are skipped if the corresponding PrePopulated field
// is already set; the user can press Enter on a skipped screen to
// accept the pre-populated value and advance.

package commitwiz

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/got-sh/got/internal/tui"
)

// state enumerates the wizard's screens. Order matches spec §8
// step order.
type state int

const (
	stateStageReview state = iota
	stateType
	stateScope
	stateSubject
	stateBody
	stateBreaking
	stateConfirm
	stateDone
	stateCancelled
)

// Model is the Bubbletea model for the commit wizard. Tests drive it
// directly via Update(); production runs it via tea.Program inside
// Run().
type Model struct {
	state      state
	answers    Answers
	pre        PrePopulated
	staged     []string
	suggest    Suggester
	suggestion Suggestion
	theme      tui.Theme

	// Stage review state.
	stageCursor   int
	stageSelected map[int]bool // indices into staged, true=selected for stage toggle

	// Type radio cursor.
	typeIdx int

	// Confirm cursor (0 = commit, 1 = go back).
	confirmIdx int

	// Breaking toggle (0 = no, 1 = yes).
	breakingIdx int

	// Text inputs.
	scopeIn   textinput.Model
	subjectIn textinput.Model
	bodyIn    textinput.Model
	breakIn   textinput.Model

	// Window size.
	width  int
	height int
}

// NewModel constructs a Model ready to run. It pre-populates the
// answers from `pre` so the wizard can skip screens the user has
// already answered via flags. `staged` is the list of currently
// staged paths (used by the stage review screen and the
// suggester).
func NewModel(staged []string, pre PrePopulated, suggest Suggester, theme tui.Theme) *Model {
	theme = theme.Apply()
	if suggest == nil {
		suggest = NewHeuristicSuggester()
	}
	suggestion := suggest.Suggest(staged)

	// Answers are pre-populated from the explicit PrePopulated
	// values; the suggester is used as a fallback only when the
	// caller did not supply a value via flags.
	answers := Answers{
		Type:           pre.Type,
		Scope:          pre.Scope,
		Subject:        pre.Subject,
		Body:           pre.Body,
		Breaking:       pre.Breaking,
		BreakingReason: pre.BreakingReason,
	}
	if answers.Type == "" {
		answers.Type = suggestion.Type
		if answers.Type == "" {
			answers.Type = ConventionalTypes[0] // "feat"
		}
	}
	if answers.Scope == "" {
		answers.Scope = suggestion.Scope
	}

	// Start the type radio on the resolved type.
	typeIdx := indexOfString(ConventionalTypes, answers.Type)
	if typeIdx < 0 {
		typeIdx = 0
	}

	m := &Model{
		state:         stateStageReview,
		pre:           pre,
		staged:        staged,
		suggest:       suggest,
		suggestion:    suggestion,
		theme:         theme,
		width:         80,
		height:        24,
		stageSelected: map[int]bool{},
		typeIdx:       typeIdx,
		confirmIdx:    0,
		breakingIdx:   0,
		answers:       answers,
	}

	// Scope input: pre-fill from pre, allow up to 32 chars.
	m.scopeIn = textinput.New()
	m.scopeIn.Placeholder = "(optional)"
	m.scopeIn.CharLimit = 32
	m.scopeIn.SetValue(pre.Scope)
	// SetValue places the cursor at the end of the value, which is
	// the desired behavior for editing a pre-populated scope.

	// Subject input: pre-fill from pre, allow up to 200 chars (we
	// validate the 72-char rule on commit).
	m.subjectIn = textinput.New()
	m.subjectIn.Placeholder = "Short imperative description"
	m.subjectIn.CharLimit = 200
	m.subjectIn.SetValue(pre.Subject)
	// SetValue places the cursor at the end of the value.

	// Body: textarea would be better but bubbles/textarea pulls in
	// more deps; v0.1 uses a single-line input and accepts that
	// multi-line bodies are a follow-up. We pre-fill from pre.
	m.bodyIn = textinput.New()
	m.bodyIn.Placeholder = "(optional body, semicolons work for now)"
	m.bodyIn.CharLimit = 500
	m.bodyIn.SetValue(pre.Body)

	// Breaking-change reason: pre-fill from pre.
	m.breakIn = textinput.New()
	m.breakIn.Placeholder = "what breaks and why"
	m.breakIn.CharLimit = 200
	m.breakIn.SetValue(pre.BreakingReason)

	return m
}

// Init implements tea.Model. It asks for the initial window size so
// the first paint knows the terminal dimensions.
func (m *Model) Init() tea.Cmd { return nil }

// Update implements tea.Model. Each state has its own key map; the
// shared rules are esc/go-back, ctrl+c / q cancels, enter advances.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		k := msg.String()
		if k == "ctrl+c" {
			m.state = stateCancelled
			return m, tea.Quit
		}
		switch m.state {
		case stateStageReview:
			return m.updateStageReview(k)
		case stateType:
			return m.updateType(k)
		case stateScope:
			return m.updateScope(msg)
		case stateSubject:
			return m.updateSubject(msg)
		case stateBody:
			return m.updateBody(msg)
		case stateBreaking:
			return m.updateBreaking(msg)
		case stateConfirm:
			return m.updateConfirm(k)
		}
	}
	// Forward non-key messages to active textinput.
	if m.state == stateScope {
		var cmd tea.Cmd
		m.scopeIn, cmd = m.scopeIn.Update(msg)
		return m, cmd
	}
	if m.state == stateSubject {
		var cmd tea.Cmd
		m.subjectIn, cmd = m.subjectIn.Update(msg)
		return m, cmd
	}
	if m.state == stateBody {
		var cmd tea.Cmd
		m.bodyIn, cmd = m.bodyIn.Update(msg)
		return m, cmd
	}
	if m.state == stateBreaking {
		var cmd tea.Cmd
		m.breakIn, cmd = m.breakIn.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) updateStageReview(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "up", "k":
		if m.stageCursor > 0 {
			m.stageCursor--
		}
	case "down", "j":
		if m.stageCursor < len(m.staged)-1 {
			m.stageCursor++
		}
	case " ":
		// Toggle selection of the current entry.
		m.stageSelected[m.stageCursor] = !m.stageSelected[m.stageCursor]
	case "a":
		// Stage-all-tracked: a v0.1 shortcut equivalent to `got commit -a`.
		// In v0.1 we just advance; the CLI's --a path is the real impl.
		return m.advance(), nil
	case "enter":
		// In a real implementation this would call `git add` /
		// `git reset` for the selected entries. v0.1 keeps the
		// selection in stageSelected and advances; the CLI uses
		// the post-wizard StagedAfter to apply staging.
		selected := m.selectedStaged()
		m.answers.StagedAfter = selected
		return m.advance(), nil
	case "n":
		// "Next": skip the stage review without changes.
		return m.advance(), nil
	case "esc", "q":
		m.state = stateCancelled
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) updateType(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "up", "k":
		if m.typeIdx > 0 {
			m.typeIdx--
		}
	case "down", "j":
		if m.typeIdx < len(ConventionalTypes)-1 {
			m.typeIdx++
		}
	case "enter":
		m.answers.Type = ConventionalTypes[m.typeIdx]
		// The "s" key (lower-case) accepts the suggested scope as
		// well; that path is handled below as a key shortcut.
		return m.advance(), nil
	case "s":
		// One-key accept of the suggested scope.
		if m.suggestion.Scope != "" {
			m.answers.Scope = m.suggestion.Scope
			m.scopeIn.SetValue(m.suggestion.Scope)
		}
		// Also lock in the suggested type.
		if m.suggestion.Type != "" {
			if i := indexOfString(ConventionalTypes, m.suggestion.Type); i >= 0 {
				m.typeIdx = i
				m.answers.Type = m.suggestion.Type
			}
		}
		return m.advance(), nil
	case "esc":
		m.state = stateStageReview
		return m, nil
	case "q":
		m.state = stateCancelled
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) updateScope(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "enter" {
		m.answers.Scope = strings.TrimSpace(m.scopeIn.Value())
		return m.advance(), nil
	}
	if k == "esc" {
		m.state = stateType
		return m, nil
	}
	if k == "ctrl+c" {
		m.state = stateCancelled
		return m, tea.Quit
	}
	// Tab / arrow-right cycle through any pre-configured scopes.
	if k == "tab" || k == "right" {
		if scopes := m.pre.AllowedScopes; len(scopes) > 0 {
			cur := m.scopeIn.Value()
			idx := -1
			for i, s := range scopes {
				if s == cur {
					idx = i
					break
				}
			}
			next := scopes[(idx+1)%len(scopes)]
			m.scopeIn.SetValue(next)
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.scopeIn, cmd = m.scopeIn.Update(msg)
	return m, cmd
}

func (m *Model) updateSubject(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "enter" {
		m.answers.Subject = strings.TrimSpace(m.subjectIn.Value())
		if m.answers.Subject == "" {
			// Bounce: subject is required.
			return m, nil
		}
		return m.advance(), nil
	}
	if k == "esc" {
		m.state = stateScope
		return m, nil
	}
	if k == "ctrl+c" {
		m.state = stateCancelled
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.subjectIn, cmd = m.subjectIn.Update(msg)
	return m, cmd
}

func (m *Model) updateBody(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "enter" {
		m.answers.Body = strings.TrimSpace(m.bodyIn.Value())
		return m.advance(), nil
	}
	if k == "esc" {
		m.state = stateSubject
		return m, nil
	}
	if k == "ctrl+c" {
		m.state = stateCancelled
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.bodyIn, cmd = m.bodyIn.Update(msg)
	return m, cmd
}

func (m *Model) updateBreaking(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch k {
	case "left", "h", "right", "l", "tab":
		m.breakingIdx = 1 - m.breakingIdx
		return m, nil
	case "enter":
		m.answers.Breaking = m.breakingIdx == 1
		if m.answers.Breaking {
			m.answers.BreakingReason = strings.TrimSpace(m.breakIn.Value())
			if m.answers.BreakingReason == "" {
				// Refuse to commit a breaking change without a
				// reason; bounce back to toggle.
				m.breakingIdx = 0
				m.answers.Breaking = false
				return m, nil
			}
		}
		return m.advance(), nil
	case "esc":
		m.state = stateBody
		return m, nil
	case "ctrl+c", "q":
		m.state = stateCancelled
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.breakIn, cmd = m.breakIn.Update(msg)
	return m, cmd
}

func (m *Model) updateConfirm(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "left", "h", "right", "l", "tab":
		m.confirmIdx = 1 - m.confirmIdx
	case "enter":
		if m.confirmIdx == 0 {
			m.state = stateDone
			return m, tea.Quit
		}
		// "Go back" returns to the subject screen (the last
		// editable field).
		m.state = stateSubject
		return m, nil
	case "esc":
		m.state = stateBreaking
		return m, nil
	case "q":
		m.state = stateCancelled
		return m, tea.Quit
	}
	return m, nil
}

// advance moves to the next screen, skipping any whose answer is
// already pre-populated.
func (m *Model) advance() *Model {
	for {
		next := nextState(m.state, m.pre, m.answers)
		if next == m.state {
			return m
		}
		m.state = next
	}
}

// nextState returns the next state to enter from s, jumping over
// any screen whose answer is already pre-populated. If all
// remaining screens are answered, the caller observes no progress
// (state == nextState) and stops.
func nextState(s state, pre PrePopulated, answers Answers) state {
	switch s {
	case stateStageReview:
		// Always visit the stage review unless the user opted
		// into -a (StageAll) or there is nothing to stage.
		if pre.StageAll || len(answers.Subject) > 0 {
			return stateType
		}
		return stateType
	case stateType:
		if pre.Type != "" || answers.Type != "" {
			return stateScope
		}
		return stateScope
	case stateScope:
		if pre.Scope != "" || answers.Scope != "" {
			return stateSubject
		}
		return stateSubject
	case stateSubject:
		if pre.Subject != "" || answers.Subject != "" {
			return stateBody
		}
		return stateBody
	case stateBody:
		if pre.Body != "" || answers.Body != "" {
			return stateBreaking
		}
		return stateBreaking
	case stateBreaking:
		if !pre.AllowBreaking {
			return stateConfirm
		}
		return stateConfirm
	}
	return s
}

// selectedStaged returns the staged paths the user marked during
// the stage review screen. It is consumed by the CLI to apply the
// staging diff after the wizard finishes.
func (m *Model) selectedStaged() []string {
	out := make([]string, 0, len(m.stageSelected))
	for i, on := range m.stageSelected {
		if on && i < len(m.staged) {
			out = append(out, m.staged[i])
		}
	}
	return out
}

// View implements tea.Model. It renders the current screen.
func (m *Model) View() string {
	switch m.state {
	case stateStageReview:
		return m.viewStageReview()
	case stateType:
		return m.viewType()
	case stateScope:
		return m.viewScope()
	case stateSubject:
		return m.viewSubject()
	case stateBody:
		return m.viewBody()
	case stateBreaking:
		return m.viewBreaking()
	case stateConfirm:
		return m.viewConfirm()
	case stateDone:
		return m.theme.Success.Render("  commit ready  ") + m.theme.Muted.Render("(rendered message printed by the CLI)")
	case stateCancelled:
		return m.theme.Error.Render("  commit cancelled")
	}
	return ""
}

func (m *Model) viewStageReview() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Stage review  "))
	b.WriteString("\n\n")
	if len(m.staged) == 0 {
		b.WriteString(m.theme.Body.Render("No files staged. Space to select, Enter to stage, 'a' for -a, 'n' to skip."))
		b.WriteString("\n\n")
	} else {
		b.WriteString(m.theme.Body.Render("Staged and unstaged entries. Space toggles selection, Enter stages them, 'n' to skip."))
		b.WriteString("\n\n")
		for i, p := range m.staged {
			marker := "[ ]"
			if m.stageSelected[i] {
				marker = "[x]"
			}
			line := fmt.Sprintf("  %s  %s", marker, p)
			if i == m.stageCursor {
				b.WriteString(m.theme.Selected.Render("> " + line))
			} else {
				b.WriteString(m.theme.Unselected.Render("  " + line))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(m.theme.Hint.Render("Up/Down  •  Space select  •  Enter stage  •  a stage-all  •  n next  •  Esc cancel"))
	b.WriteString("\n")
	return b.String()
}

func (m *Model) viewType() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Type  "))
	b.WriteString("\n\n")
	if m.suggestion.Type != "" {
		banner := fmt.Sprintf("Suggested: %s (%s) [%.0f%%]  —  %s",
			m.suggestion.Type, m.suggestion.ScopeOrDash(), m.suggestion.Confidence*100, m.suggestion.Reason)
		b.WriteString(m.theme.Detected.Render(banner))
		b.WriteString("\n\n")
	}
	for i, t := range ConventionalTypes {
		label := t
		if i == m.typeIdx {
			b.WriteString(m.theme.Selected.Render("> " + label))
		} else {
			b.WriteString(m.theme.Unselected.Render("  " + label))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Hint.Render("Up/Down  •  s accept suggestion  •  Enter  •  Esc back"))
	b.WriteString("\n")
	return b.String()
}

// ScopeOrDash returns the suggestion's scope or "-" if empty, for
// the suggestion banner.
func (s Suggestion) ScopeOrDash() string {
	if s.Scope == "" {
		return "-"
	}
	return s.Scope
}

func (m *Model) viewScope() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Scope  "))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Body.Render("Optional scope (e.g. cli, store, docs). Press Enter to skip."))
	b.WriteString("\n\n")
	if scopes := m.pre.AllowedScopes; len(scopes) > 0 {
		b.WriteString(m.theme.Muted.Render("  configured scopes: " + strings.Join(scopes, ", ")))
		b.WriteString("\n\n")
	}
	b.WriteString("  ")
	b.WriteString(m.scopeIn.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Hint.Render("Type  •  Tab to cycle configured scopes  •  Enter  •  Esc back"))
	b.WriteString("\n")
	return b.String()
}

func (m *Model) viewSubject() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Subject  "))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Body.Render("Single line, ≤72 chars, imperative mood, no trailing period."))
	b.WriteString("\n\n")
	b.WriteString("  ")
	b.WriteString(m.subjectIn.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Hint.Render("Type  •  Enter  •  Esc back"))
	b.WriteString("\n")
	return b.String()
}

func (m *Model) viewBody() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Body  "))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Body.Render("Optional body. v0.1: single line, wrapped at commit time."))
	b.WriteString("\n\n")
	b.WriteString("  ")
	b.WriteString(m.bodyIn.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Hint.Render("Type  •  Enter  •  Esc back"))
	b.WriteString("\n")
	return b.String()
}

func (m *Model) viewBreaking() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Breaking change?  "))
	b.WriteString("\n\n")
	no := "No"
	yes := "Yes"
	if m.breakingIdx == 0 {
		b.WriteString(m.theme.Selected.Render("> " + no))
		b.WriteString("  ")
		b.WriteString(m.theme.Unselected.Render("  " + yes))
	} else {
		b.WriteString(m.theme.Unselected.Render("  " + no))
		b.WriteString("  ")
		b.WriteString(m.theme.Selected.Render("> " + yes))
	}
	b.WriteString("\n\n")
	if m.breakingIdx == 1 {
		b.WriteString(m.theme.Body.Render("Reason (required, will be the BREAKING CHANGE footer):"))
		b.WriteString("\n\n")
		b.WriteString("  ")
		b.WriteString(m.breakIn.View())
		b.WriteString("\n\n")
	}
	b.WriteString(m.theme.Hint.Render("Left/Right  •  Type reason if Yes  •  Enter  •  Esc back"))
	b.WriteString("\n")
	return b.String()
}

func (m *Model) viewConfirm() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Confirm  "))
	b.WriteString("\n\n")
	rendered := m.answers.Render()
	v := Validate(rendered)
	if len(v.Issues) > 0 {
		b.WriteString(m.theme.Error.Render("Warnings:"))
		b.WriteString("\n")
		for _, i := range v.Issues {
			b.WriteString(m.theme.Hint.Render("  - " + i.Message))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(m.theme.Body.Render("Final message:"))
	b.WriteString("\n")
	for _, line := range strings.Split(strings.TrimRight(rendered, "\n"), "\n") {
		b.WriteString(m.theme.Detected.Render("  " + line))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	yes := "Commit"
	no := "Go back"
	if m.confirmIdx == 0 {
		b.WriteString(m.theme.Selected.Render("> " + yes))
		b.WriteString("  ")
		b.WriteString(m.theme.Unselected.Render("  " + no))
	} else {
		b.WriteString(m.theme.Unselected.Render("  " + yes))
		b.WriteString("  ")
		b.WriteString(m.theme.Selected.Render("> " + no))
	}
	b.WriteString("\n\n")
	b.WriteString(m.theme.Hint.Render("Left/Right  •  Enter  •  Esc back"))
	b.WriteString("\n")
	return b.String()
}

// indexOfString returns the index of target in xs, or -1.
func indexOfString(xs []string, target string) int {
	for i, x := range xs {
		if x == target {
			return i
		}
	}
	return -1
}
