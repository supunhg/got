package initwiz

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/got-sh/got/internal/tui"
)

// Answers is the wizard's output: the user's choices for the
// project + commit + plugins. The CLI uses this to write got.yml,
// .got/config.yaml, and to open the SQLite store.
type Answers struct {
	Name           string
	DefaultBranch  string
	CommitStyle    string
	CustomTemplate string
	Plugins        []string
}

// Defaults returns the spec §7 adaptive defaults: name from the
// detected dir basename, branch from the detected branch (or "main"),
// style=conventional, no scopes, no plugins, heuristic AI.
func Defaults(d Detected) Answers {
	name := d.Name
	branch := d.Branch
	if branch == "" {
		branch = "main"
	}
	return Answers{
		Name:          name,
		DefaultBranch: branch,
		CommitStyle:   "conventional",
		Plugins:       []string{},
	}
}

// Run starts the Bubbletea program for the wizard and blocks until
// the user confirms or cancels. `pre` carries values pre-populated
// from CLI flags; screens the user has already answered are
// skipped. Returns CancelledError if the user quits.
func Run(detected Detected, pre PrePopulated, theme tui.Theme) (Answers, error) {
	m := New(detected, pre, theme)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return Answers{}, fmt.Errorf("wizard: %w", err)
	}
	fm, ok := finalModel.(*Model)
	if !ok {
		return Answers{}, fmt.Errorf("wizard: unexpected model type %T", finalModel)
	}
	if fm.state == stateCancelled {
		return Answers{}, CancelledError
	}
	if fm.state != stateDone {
		return Answers{}, CancelledError
	}
	return fm.answers, nil
}

// FinalModel is exposed for tests that want to drive a model
// directly. The normal Run() path is used in production.
func FinalModel(m *Model) Answers { return m.answers }
