// branchwiz/model.go: Bubbletea model for the branch wizard.
//
// State machine:
//
//	stateMenu        - main menu: create / checkout / delete / quit
//	stateCreateName  - text input for the new branch name
//	stateCreateFrom  - optional text input for the start point
//	stateCheckout    - pick from the local branches list
//	stateDelete      - pick from the local branches list (current disabled)
//	stateForce       - "Force delete? Unmerged work will be lost" y/n
//	stateConfirm     - preview the resolved action, commit / back
//	stateDone / stateCancelled
//
// All states honor esc (back one level, or cancel from the menu), q /
// ctrl+c (cancel immediately), and enter (advance / confirm).

package branchwiz

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/tui"
)

// state enumerates the wizard's screens. Order is the menu order.
type state int

const (
	stateMenu state = iota
	stateCreateName
	stateCreateFrom
	stateCheckout
	stateDelete
	stateForce
	stateConfirm
	stateDone
	stateCancelled
)

// menuChoice enumerates the entries on the main menu.
type menuChoice int

const (
	menuCreate menuChoice = iota
	menuCheckout
	menuDelete
	menuQuit
)

// Model is the Bubbletea model for the branch wizard. Tests drive it
// directly via Update(); production runs it via tea.Program inside Run().
type Model struct {
	state    state
	answers  Answers
	pre      PrePopulated
	branches []git.Branch
	theme    tui.Theme

	// Menu cursor.
	menuIdx menuChoice

	// Picker cursors for the checkout / delete screens.
	checkoutIdx int
	deleteIdx   int

	// Force-delete toggle (0 = no, 1 = yes).
	forceIdx int

	// Confirm cursor (0 = commit, 1 = go back).
	confirmIdx int

	// Text inputs.
	nameIn textinput.Model
	fromIn textinput.Model

	// Window size.
	width  int
	height int
}

// NewModel constructs a Model ready to run. It pre-populates answers
// from `pre` so the wizard can skip screens the user has already
// answered via flags. `branches` is the list of local branches (used
// by the checkout / delete pickers and the menu's current-branch
// indicator).
func NewModel(branches []git.Branch, pre PrePopulated, theme tui.Theme) *Model {
	theme = theme.Apply()
	answers := Answers{
		Action:     pre.Action,
		Name:       pre.Name,
		StartPoint: pre.StartPoint,
		Force:      pre.Force,
	}

	m := &Model{
		state:    stateMenu,
		answers:  answers,
		pre:      pre,
		branches: branches,
		theme:    theme,
		width:    80,
		height:   24,
		forceIdx: 0,
	}

	m.nameIn = textinput.New()
	m.nameIn.Placeholder = "feature/my-branch"
	m.nameIn.CharLimit = 100
	m.nameIn.SetValue(pre.Name)

	m.fromIn = textinput.New()
	m.fromIn.Placeholder = "(optional, defaults to HEAD)"
	m.fromIn.CharLimit = 100
	m.fromIn.SetValue(pre.StartPoint)

	// If the caller pre-pinned an action, jump straight to the
	// appropriate non-menu state. The wizard will still ask for
	// any fields that weren't pre-populated.
	switch pre.Action {
	case ActionCreate:
		m.state = stateCreateName
	case ActionCheckout:
		m.state = stateCheckout
	case ActionDelete:
		m.state = stateDelete
	}

	return m
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd { return nil }

// cancelled reports whether the wizard ended in a cancellation.
func (m *Model) cancelled() bool {
	return m.state == stateCancelled
}

// Update implements tea.Model. Each state has its own key map; the
// shared rules are esc (back), q / ctrl+c (cancel), enter (advance).
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
		case stateMenu:
			return m.updateMenu(k)
		case stateCreateName:
			return m.updateCreateName(msg)
		case stateCreateFrom:
			return m.updateCreateFrom(msg)
		case stateCheckout:
			return m.updateCheckout(k)
		case stateDelete:
			return m.updateDelete(k)
		case stateForce:
			return m.updateForce(k)
		case stateConfirm:
			return m.updateConfirm(k)
		}
	}
	// Forward non-key messages to active text inputs.
	if m.state == stateCreateName {
		var cmd tea.Cmd
		m.nameIn, cmd = m.nameIn.Update(msg)
		return m, cmd
	}
	if m.state == stateCreateFrom {
		var cmd tea.Cmd
		m.fromIn, cmd = m.fromIn.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) updateMenu(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "up", "k":
		if m.menuIdx > 0 {
			m.menuIdx--
		}
	case "down", "j":
		if m.menuIdx < menuQuit {
			m.menuIdx++
		}
	case "enter":
		switch m.menuIdx {
		case menuCreate:
			m.answers.Action = ActionCreate
			m.state = stateCreateName
			return m, nil
		case menuCheckout:
			m.answers.Action = ActionCheckout
			m.state = stateCheckout
			return m, nil
		case menuDelete:
			m.answers.Action = ActionDelete
			m.state = stateDelete
			return m, nil
		case menuQuit:
			m.state = stateCancelled
			return m, tea.Quit
		}
	case "q", "esc":
		m.state = stateCancelled
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) updateCreateName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "enter" {
		m.answers.Name = strings.TrimSpace(m.nameIn.Value())
		if m.answers.Name == "" {
			// Bounce: a branch name is required.
			return m, nil
		}
		m.state = stateCreateFrom
		return m, nil
	}
	if k == "esc" {
		m.state = stateMenu
		return m, nil
	}
	if k == "ctrl+c" {
		m.state = stateCancelled
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.nameIn, cmd = m.nameIn.Update(msg)
	return m, cmd
}

func (m *Model) updateCreateFrom(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "enter" {
		m.answers.StartPoint = strings.TrimSpace(m.fromIn.Value())
		m.state = stateConfirm
		return m, nil
	}
	if k == "esc" {
		m.state = stateCreateName
		return m, nil
	}
	if k == "ctrl+c" {
		m.state = stateCancelled
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.fromIn, cmd = m.fromIn.Update(msg)
	return m, cmd
}

func (m *Model) updateCheckout(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "up", "k":
		if m.checkoutIdx > 0 {
			m.checkoutIdx--
		}
	case "down", "j":
		if m.checkoutIdx < len(m.branches)-1 {
			m.checkoutIdx++
		}
	case "enter":
		if m.checkoutIdx >= len(m.branches) {
			return m, nil
		}
		m.answers.Name = m.branches[m.checkoutIdx].Name
		m.state = stateConfirm
		return m, nil
	case "esc":
		m.state = stateMenu
		return m, nil
	case "q", "ctrl+c":
		m.state = stateCancelled
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) updateDelete(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "up", "k":
		if m.deleteIdx > 0 {
			m.deleteIdx--
		}
	case "down", "j":
		if m.deleteIdx < len(m.branches)-1 {
			m.deleteIdx++
		}
	case "enter":
		if m.deleteIdx >= len(m.branches) {
			return m, nil
		}
		// Refuse to delete the current branch.
		if m.branches[m.deleteIdx].IsCurrent {
			// Bounce: same screen, but the user can't pick this.
			return m, nil
		}
		m.answers.Name = m.branches[m.deleteIdx].Name
		m.state = stateForce
		return m, nil
	case "esc":
		m.state = stateMenu
		return m, nil
	case "q", "ctrl+c":
		m.state = stateCancelled
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) updateForce(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "left", "h", "right", "l", "tab":
		m.forceIdx = 1 - m.forceIdx
	case "enter":
		m.answers.Force = m.forceIdx == 1
		m.state = stateConfirm
		return m, nil
	case "esc":
		m.state = stateDelete
		return m, nil
	case "q", "ctrl+c":
		m.state = stateCancelled
		return m, tea.Quit
	}
	return m, nil
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
		// "Go back" returns to the appropriate screen.
		switch m.answers.Action {
		case ActionCreate:
			m.state = stateCreateFrom
		case ActionCheckout:
			m.state = stateCheckout
		case ActionDelete:
			m.state = stateForce
		default:
			m.state = stateMenu
		}
		return m, nil
	case "esc":
		// Esc from confirm goes back to the last input screen for
		// this action.
		switch m.answers.Action {
		case ActionCreate:
			m.state = stateCreateFrom
		case ActionCheckout:
			m.state = stateCheckout
		case ActionDelete:
			m.state = stateForce
		}
		return m, nil
	case "q", "ctrl+c":
		m.state = stateCancelled
		return m, tea.Quit
	}
	return m, nil
}

// View implements tea.Model. It renders the current screen.
func (m *Model) View() string {
	switch m.state {
	case stateMenu:
		return m.viewMenu()
	case stateCreateName:
		return m.viewCreateName()
	case stateCreateFrom:
		return m.viewCreateFrom()
	case stateCheckout:
		return m.viewCheckout()
	case stateDelete:
		return m.viewDelete()
	case stateForce:
		return m.viewForce()
	case stateConfirm:
		return m.viewConfirm()
	case stateDone:
		return m.theme.Success.Render("  branch action ready  ")
	case stateCancelled:
		return m.theme.Error.Render("  branch wizard cancelled  ")
	}
	return ""
}

func (m *Model) viewMenu() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Branch  "))
	b.WriteString("\n\n")

	current := currentBranchName(m.branches)
	if current != "" {
		b.WriteString(m.theme.Muted.Render(fmt.Sprintf("On branch: %s", current)))
		b.WriteString("\n\n")
	}

	entries := []string{"Create a new branch", "Checkout an existing branch", "Delete a branch", "Quit"}
	for i, e := range entries {
		line := "  " + e
		if menuChoice(i) == m.menuIdx {
			b.WriteString(m.theme.Selected.Render("> " + e))
		} else {
			b.WriteString(m.theme.Unselected.Render(line))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Hint.Render("Up/Down  •  Enter  •  q quit"))
	b.WriteString("\n")
	return b.String()
}

func (m *Model) viewCreateName() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Create branch  "))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Body.Render("Pick a name for the new branch. Letters, digits, '/', '-', '_', and '.' are allowed; spaces and shell metacharacters are not."))
	b.WriteString("\n\n")
	b.WriteString("  ")
	b.WriteString(m.nameIn.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Hint.Render("Type  •  Enter  •  Esc back"))
	b.WriteString("\n")
	return b.String()
}

func (m *Model) viewCreateFrom() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Branch from...  "))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Body.Render("Optional start point: a commit SHA, a branch name, or a tag. Leave empty to branch from HEAD."))
	b.WriteString("\n\n")
	b.WriteString("  ")
	b.WriteString(m.fromIn.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Hint.Render("Type  •  Enter  •  Esc back"))
	b.WriteString("\n")
	return b.String()
}

func (m *Model) viewCheckout() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Checkout branch  "))
	b.WriteString("\n\n")
	if len(m.branches) == 0 {
		b.WriteString(m.theme.Muted.Render("No local branches."))
		b.WriteString("\n")
	} else {
		for i, br := range m.branches {
			label := br.Name
			if br.IsCurrent {
				label = label + " (current)"
			}
			if i == m.checkoutIdx {
				b.WriteString(m.theme.Selected.Render("> " + label))
			} else {
				b.WriteString(m.theme.Unselected.Render("  " + label))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Hint.Render("Up/Down  •  Enter  •  Esc back  •  q cancel"))
	b.WriteString("\n")
	return b.String()
}

func (m *Model) viewDelete() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Delete branch  "))
	b.WriteString("\n\n")
	if len(m.branches) == 0 {
		b.WriteString(m.theme.Muted.Render("No local branches."))
		b.WriteString("\n")
	} else {
		for i, br := range m.branches {
			label := br.Name
			if br.IsCurrent {
				label = label + " (cannot delete current branch)"
			}
			if i == m.deleteIdx {
				b.WriteString(m.theme.Selected.Render("> " + label))
			} else {
				b.WriteString(m.theme.Unselected.Render("  " + label))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Hint.Render("Up/Down  •  Enter  •  Esc back  •  q cancel"))
	b.WriteString("\n")
	return b.String()
}

func (m *Model) viewForce() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Force delete?  "))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Body.Render(fmt.Sprintf("Branch %q might not be fully merged. Force delete? Unmerged work will be lost.", m.answers.Name)))
	b.WriteString("\n\n")
	no := "No (safe delete)"
	yes := "Yes (force, -D)"
	if m.forceIdx == 0 {
		b.WriteString(m.theme.Selected.Render("> " + no))
		b.WriteString("  ")
		b.WriteString(m.theme.Unselected.Render("  " + yes))
	} else {
		b.WriteString(m.theme.Unselected.Render("  " + no))
		b.WriteString("  ")
		b.WriteString(m.theme.Selected.Render("> " + yes))
	}
	b.WriteString("\n\n")
	b.WriteString(m.theme.Hint.Render("Left/Right  •  Enter  •  Esc back"))
	b.WriteString("\n")
	return b.String()
}

func (m *Model) viewConfirm() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("  Confirm  "))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Body.Render(describe(m.answers)))
	b.WriteString("\n\n")
	yes := "Apply"
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
	b.WriteString(m.theme.Hint.Render("Left/Right  •  Enter  •  q cancel"))
	b.WriteString("\n")
	return b.String()
}

// describe returns a one-line human-readable summary of the resolved
// action for the confirm screen.
func describe(a Answers) string {
	switch a.Action {
	case ActionCreate:
		if a.StartPoint == "" {
			return fmt.Sprintf("Create branch %q from HEAD.", a.Name)
		}
		return fmt.Sprintf("Create branch %q from %q.", a.Name, a.StartPoint)
	case ActionCheckout:
		return fmt.Sprintf("Checkout branch %q.", a.Name)
	case ActionDelete:
		flag := "-d"
		if a.Force {
			flag = "-D"
		}
		return fmt.Sprintf("Delete branch %q (git branch %s).", a.Name, flag)
	}
	return "(no action)"
}

// currentBranchName returns the name of the current branch, or "" if
// none of the entries are current.
func currentBranchName(branches []git.Branch) string {
	for _, b := range branches {
		if b.IsCurrent {
			return b.Name
		}
	}
	return ""
}
