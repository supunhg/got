package initwiz

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/tui"
)

// state is the wizard's current screen.
type state int

const (
	stateWelcome state = iota
	stateCommitStyle
	stateCustomTemplate
	statePlugins
	stateConfirm
	stateDone
	stateCancelled
)

// commitStyles is the fixed list of choices on the commit-style
// screen. Order matches the spec (§7 step 2).
var commitStyles = []string{
	"conventional",
	"freeform",
	"custom",
}

// Model is the Bubbletea model for the init wizard. The model is
// driven by tea.Program in Run; tests construct one directly and
// poke Update() with synthetic key messages.
type Model struct {
	state      state
	answers    Answers
	detected   Detected
	pre        PrePopulated
	styles     tui.Theme
	styleIdx   int // radio cursor for stateCommitStyle
	confirmIdx int // 0 = confirm, 1 = cancel
	tmpl       textinput.Model

	// Width/height are set by tea.WindowSizeMsg. The default 80x24
	// is what most terminals will report on first paint; we use it
	// to wrap content.
	width  int
	height int
}

// PrePopulated carries answers that the user already supplied via
// command-line flags. The wizard uses these to skip screens that the
// user has already answered, and to pre-fill text inputs.
type PrePopulated struct {
	Name           string
	DefaultBranch  string
	CommitStyle    string
	CustomTemplate string
}

// New returns a Model ready to run. It pre-populates the answers
// from `pre` so the wizard can skip screens the user has already
// answered via flags. `detected` drives the Welcome screen's
// "Detected" panel.
func New(detected Detected, pre PrePopulated, styles tui.Theme) *Model {
	styles = styles.Apply()
	m := &Model{
		state:    stateWelcome,
		detected: detected,
		pre:      pre,
		styles:   styles,
		width:    80,
		height:   24,
		answers: Answers{
			Name:           pre.Name,
			DefaultBranch:  pre.DefaultBranch,
			CommitStyle:    pre.CommitStyle,
			CustomTemplate: pre.CustomTemplate,
			Plugins:        []string{},
		},
	}
	// Default-branch fallback: detected branch or "main".
	if m.answers.DefaultBranch == "" {
		m.answers.DefaultBranch = detected.Branch
		if m.answers.DefaultBranch == "" {
			m.answers.DefaultBranch = "main"
		}
	}
	// Project-name fallback: detected dir basename.
	if m.answers.Name == "" {
		m.answers.Name = detected.Name
	}
	// Commit-style fallback + index.
	if m.answers.CommitStyle == "" {
		m.answers.CommitStyle = "conventional"
	}
	m.styleIdx = indexOf(commitStyles, m.answers.CommitStyle)
	if m.styleIdx < 0 {
		m.styleIdx = 0
	}
	// Text input for the custom-template screen.
	m.tmpl = textinput.New()
	m.tmpl.Placeholder = "/path/to/template.tmpl"
	m.tmpl.CharLimit = 256
	m.tmpl.SetValue(m.answers.CustomTemplate)
	return m
}

// Init implements tea.Model. It asks for the initial window size so
// the first paint knows the terminal dimensions.
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model. Each state has its own key map; the
// shared rules are:
//
//   - esc goes back one screen (from the first screen it cancels)
//   - ctrl+c cancels immediately
//   - q cancels immediately
//   - enter advances / confirms
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		k := msg.String()
		// Global quit keys (ctrl+c is the Bubbletea convention).
		if k == "ctrl+c" {
			m.state = stateCancelled
			return m, tea.Quit
		}
		switch m.state {
		case stateWelcome:
			return m.updateWelcome(k)
		case stateCommitStyle:
			return m.updateCommitStyle(k)
		case stateCustomTemplate:
			return m.updateCustomTemplate(msg)
		case statePlugins:
			return m.updatePlugins(k)
		case stateConfirm:
			return m.updateConfirm(k)
		}
	}
	// For textinput we always need to forward non-special keys.
	if m.state == stateCustomTemplate {
		var cmd tea.Cmd
		m.tmpl, cmd = m.tmpl.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) updateWelcome(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "enter":
		// New() already populated Name + DefaultBranch from
		// pre -> detected defaults. Nothing to do here except
		// advance to the next screen.
		return m.advance(), nil
	case "esc", "q":
		m.state = stateCancelled
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) updateCommitStyle(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "up", "k":
		if m.styleIdx > 0 {
			m.styleIdx--
		}
	case "down", "j":
		if m.styleIdx < len(commitStyles)-1 {
			m.styleIdx++
		}
	case "enter":
		m.answers.CommitStyle = commitStyles[m.styleIdx]
		if m.answers.CommitStyle == "custom" {
			m.state = stateCustomTemplate
			m.tmpl.Focus()
			return m, nil
		}
		m.answers.CustomTemplate = ""
		return m.advance(), nil
	case "esc":
		m.state = stateWelcome
		return m, nil
	case "q":
		m.state = stateCancelled
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) updateCustomTemplate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch k {
	case "enter":
		m.answers.CustomTemplate = strings.TrimSpace(m.tmpl.Value())
		if m.answers.CustomTemplate == "" {
			// Empty template is invalid for style=custom; bounce
			// back to commit style so the user picks something.
			m.answers.CommitStyle = "conventional"
			m.styleIdx = 0
			m.state = stateCommitStyle
			return m, nil
		}
		return m.advance(), nil
	case "esc":
		m.state = stateCommitStyle
		return m, nil
	case "ctrl+c":
		m.state = stateCancelled
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.tmpl, cmd = m.tmpl.Update(msg)
	return m, cmd
}

func (m *Model) updatePlugins(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "enter":
		// v0.1 has no bundled plugins; the user can only Skip.
		m.answers.Plugins = []string{}
		return m.advance(), nil
	case "esc":
		// Plugins is the third screen; going back lands on commit
		// style (or the custom-template screen, but skipping that
		// is fine — the user can re-enter it).
		if m.answers.CommitStyle == "custom" {
			m.state = stateCustomTemplate
			m.tmpl.Focus()
			return m, nil
		}
		m.state = stateCommitStyle
		return m, nil
	case "q":
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
		m.state = stateWelcome
		return m, nil
	case "esc":
		m.state = statePlugins
		return m, nil
	case "q":
		m.state = stateCancelled
		return m, tea.Quit
	}
	return m, nil
}

// advance moves to the next screen in the spec'd flow:
// welcome -> commit style -> (custom template) -> plugins -> confirm.
// When the user pre-populated a flag from the CLI, the corresponding
// wizard screen is skipped (the answer is locked in from the flag).
func (m *Model) advance() *Model {
	switch m.state {
	case stateWelcome:
		// If the commit style was pre-populated, skip the radio:
		//   - "custom" -> jump to the template screen
		//   - anything else -> jump to plugins
		if m.pre.CommitStyle != "" {
			m.answers.CommitStyle = m.pre.CommitStyle
			if m.pre.CommitStyle == "custom" {
				m.state = stateCustomTemplate
				m.tmpl.Focus()
				return m
			}
			m.state = statePlugins
			return m
		}
		m.state = stateCommitStyle
	case stateCommitStyle:
		m.state = statePlugins
	case stateCustomTemplate:
		m.state = statePlugins
	case statePlugins:
		m.confirmIdx = 0
		m.state = stateConfirm
	}
	return m
}

// View implements tea.Model. It renders the current screen.
func (m *Model) View() string {
	switch m.state {
	case stateWelcome:
		return m.viewWelcome()
	case stateCommitStyle:
		return m.viewCommitStyle()
	case stateCustomTemplate:
		return m.viewCustomTemplate()
	case statePlugins:
		return m.viewPlugins()
	case stateConfirm:
		return m.viewConfirm()
	case stateDone:
		return m.styles.Success.Render("  Initialized GOT. ") + m.styles.Muted.Render("Run `got status` to get started.")
	case stateCancelled:
		return m.styles.Error.Render("  init cancelled")
	}
	return ""
}

func (m *Model) viewWelcome() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("  GOT Init  "))
	b.WriteString("\n\n")
	b.WriteString(m.styles.Body.Render("Detected values for this repository:"))
	b.WriteString("\n\n")
	if m.answers.Name != "" {
		b.WriteString(m.styles.Body.Render(fmt.Sprintf("  project name       %s", m.styles.Accent.Render(m.answers.Name))))
		b.WriteString("\n")
	}
	if m.answers.DefaultBranch != "" {
		b.WriteString(m.styles.Body.Render(fmt.Sprintf("  default branch     %s", m.styles.Accent.Render(m.answers.DefaultBranch))))
		b.WriteString("\n")
	}
	if len(m.detected.Languages) > 0 {
		b.WriteString(m.styles.Body.Render(fmt.Sprintf("  languages          %s", m.styles.Detected.Render(strings.Join(m.detected.Languages, ", ")))))
		b.WriteString("\n")
	}
	if len(m.detected.Frameworks) > 0 {
		b.WriteString(m.styles.Body.Render(fmt.Sprintf("  frameworks         %s", m.styles.Detected.Render(strings.Join(m.detected.Frameworks, ", ")))))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.styles.Hint.Render("Press Enter to continue  •  Esc to cancel"))
	b.WriteString("\n")
	return b.String()
}

func (m *Model) viewCommitStyle() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("  Commit style  "))
	b.WriteString("\n\n")
	b.WriteString(m.styles.Body.Render("How should `got commit` validate messages?"))
	b.WriteString("\n\n")
	for i, s := range commitStyles {
		label := formatCommitStyleLabel(s)
		if i == m.styleIdx {
			b.WriteString(m.styles.Selected.Render("> " + label))
		} else {
			b.WriteString(m.styles.Unselected.Render("  " + label))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.styles.Hint.Render("Up/Down to choose  •  Enter to continue  •  Esc to go back"))
	b.WriteString("\n")
	return b.String()
}

func (m *Model) viewCustomTemplate() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("  Custom template  "))
	b.WriteString("\n\n")
	b.WriteString(m.styles.Body.Render("Path to a custom commit-message template:"))
	b.WriteString("\n\n")
	b.WriteString("  ")
	b.WriteString(m.tmpl.View())
	b.WriteString("\n\n")
	b.WriteString(m.styles.Hint.Render("Type a path  •  Enter to confirm  •  Esc to go back"))
	b.WriteString("\n")
	return b.String()
}

func (m *Model) viewPlugins() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("  Plugins  "))
	b.WriteString("\n\n")
	b.WriteString(m.styles.Body.Render("No bundled plugins in v0.1."))
	b.WriteString("\n")
	b.WriteString(m.styles.Muted.Render("Skip and finish — you can add plugins later via the plugin manager."))
	b.WriteString("\n\n")
	b.WriteString(m.styles.Hint.Render("Enter to continue  •  Esc to go back"))
	b.WriteString("\n")
	return b.String()
}

func (m *Model) viewConfirm() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("  Confirm  "))
	b.WriteString("\n\n")
	b.WriteString(m.styles.Body.Render("This will create:"))
	b.WriteString("\n")
	b.WriteString(m.styles.Body.Render("  - .got/         (snapshots, workspaces, decisions, health, cache, plugins)"))
	b.WriteString("\n")
	b.WriteString(m.styles.Body.Render("  - .got/config.yaml"))
	b.WriteString("\n")
	b.WriteString(m.styles.Body.Render("  - got.yml       (project config)"))
	b.WriteString("\n")
	b.WriteString(m.styles.Body.Render("  - .got/got.db   (SQLite store)"))
	b.WriteString("\n")
	b.WriteString(m.styles.Body.Render("  - .got/         appended to .gitignore"))
	b.WriteString("\n\n")
	b.WriteString(m.styles.Body.Render("Summary:"))
	b.WriteString("\n")
	b.WriteString(m.styles.Body.Render(fmt.Sprintf("  name           %s", m.styles.Accent.Render(m.answers.Name))))
	b.WriteString("\n")
	b.WriteString(m.styles.Body.Render(fmt.Sprintf("  default branch %s", m.styles.Accent.Render(m.answers.DefaultBranch))))
	b.WriteString("\n")
	b.WriteString(m.styles.Body.Render(fmt.Sprintf("  commit style   %s", m.styles.Accent.Render(m.answers.CommitStyle))))
	if m.answers.CommitStyle == "custom" {
		b.WriteString("\n")
		b.WriteString(m.styles.Body.Render(fmt.Sprintf("  template       %s", m.styles.Accent.Render(m.answers.CustomTemplate))))
	}
	b.WriteString("\n\n")

	yes := "Continue"
	no := "Go back"
	if m.confirmIdx == 0 {
		b.WriteString(m.styles.Selected.Render("> " + yes))
		b.WriteString("  ")
		b.WriteString(m.styles.Unselected.Render("  " + no))
	} else {
		b.WriteString(m.styles.Unselected.Render("  " + yes))
		b.WriteString("  ")
		b.WriteString(m.styles.Selected.Render("> " + no))
	}
	b.WriteString("\n\n")
	b.WriteString(m.styles.Hint.Render("Left/Right to choose  •  Enter to confirm  •  Esc to go back"))
	b.WriteString("\n")
	return b.String()
}

// formatCommitStyleLabel turns "conventional" into "Conventional
// Commits (default)" etc. for the radio list.
func formatCommitStyleLabel(s string) string {
	switch s {
	case "conventional":
		return "Conventional Commits  (default)"
	case "freeform":
		return "Free-form"
	case "custom":
		return "Custom template"
	}
	return s
}

// indexOf returns the index of target in xs, or -1.
func indexOf(xs []string, target string) int {
	for i, x := range xs {
		if x == target {
			return i
		}
	}
	return -1
}

// CancelledError is returned by Run when the user quits before
// finishing the wizard. Callers can match it via errors.Is or just
// check the message.
var CancelledError = gerr.Validation("init cancelled")
