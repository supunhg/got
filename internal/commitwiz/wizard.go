// commitwiz/wizard.go: public surface of the commit wizard.
//
// The Bubbletea model lives in model.go; this file holds the
// non-interactive data types (Answers, PrePopulated), the Defaults
// helper, the message renderer, and the Run entry point that drives
// the model via tea.Program.
//
// Two callers hit this file:
//
//   - internal/cli/commit.go, which calls Run when stdout is a TTY
//     and --no-tui is not set. The CLI passes a PrePopulated built
//     from flag values + heuristic suggestion.
//   - internal/cli/commit.go, in the --no-tui path, builds
//     PrePopulated from flags and uses Defaults to fill the gaps,
//     then calls Render to produce the final commit message string.
//
// The wizard is intentionally small: 7 screens (stage review, type,
// scope, subject, body, breaking, confirm) and a single Suggester
// dependency. Future LLM-backed suggesters plug in via the
// Suggester interface without touching this file.
package commitwiz

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/tui"
)

// Answers is the wizard's output: the user's choices for the
// commit message fields. The CLI in internal/cli/commit.go uses
// this to either (a) render the final message via Render, or (b)
// pass the structured fields directly to git commit.
type Answers struct {
	// Type is the Conventional Commits type.
	Type string
	// Scope is the optional scope (e.g. "cli"). Empty is OK.
	Scope string
	// Subject is the one-line subject (≤72 chars, no trailing period).
	Subject string
	// Body is the optional body. May contain multiple lines.
	Body string
	// Breaking is true if the commit is a breaking change.
	Breaking bool
	// BreakingReason is the reason text used in the BREAKING CHANGE
	// footer. Required when Breaking=true; the wizard enforces this
	// on the breaking-change screen.
	BreakingReason string
	// StagedAfter is the set of paths the wizard staged (or
	// unstaged) during the stage-review screen. The CLI uses this
	// to update the work tree after the wizard finishes. May be nil.
	StagedAfter []string
}

// PrePopulated carries values the user already supplied via
// command-line flags. The wizard uses these to skip screens and
// pre-fill text inputs. Each field is optional; an empty string or
// false means "ask the user".
type PrePopulated struct {
	// Type from --type, or empty.
	Type string
	// Scope from --scope, or empty.
	Scope string
	// Subject from --subject / -m. When set, the wizard skips
	// the subject screen (and most other screens if the message
	// is also fully specified).
	Subject string
	// Body from --body, or empty.
	Body string
	// Breaking from --breaking, or false.
	Breaking bool
	// BreakingReason from --breaking-reason, or empty.
	BreakingReason string
	// StageAll is true when --all / -a is set.
	StageAll bool
	// NoVerify from --no-verify, or false.
	NoVerify bool
	// AllowBreaking from got.yml's commits.allow_breaking. False
	// hides the breaking-change screen.
	AllowBreaking bool
	// AllowedScopes from got.yml's commits.scopes. When non-empty
	// the wizard offers them as completions on the scope screen.
	AllowedScopes []string
}

// Defaults returns the spec §8 adaptive defaults for a commit
// message. It does NOT fill Subject — the user must always type
// one. Use suggester.Suggest(staged) for the type/scope pick.
func Defaults() Answers {
	return Answers{
		Type: "feat",
	}
}

// Render formats a as a Conventional Commits message suitable for
// `git commit -F -`. Empty fields are omitted. The returned string
// always ends with a newline.
func (a Answers) Render() string {
	v := Validated{
		Type:     a.Type,
		Scope:    a.Scope,
		Subject:  a.Subject,
		Body:     a.Body,
		Breaking: a.Breaking,
		Footers:  buildFooters(a),
	}
	return v.Render()
}

// buildFooters turns the structured Answers fields into the
// Conventional Commits footer list. Currently only the BREAKING
// CHANGE footer is synthesized; user-supplied footers would be
// added in a future step.
func buildFooters(a Answers) []string {
	if !a.Breaking {
		return nil
	}
	reason := a.BreakingReason
	if reason == "" {
		reason = "see commit body"
	}
	return []string{"BREAKING CHANGE: " + reason}
}

// Run starts the Bubbletea program for the commit wizard and blocks
// until the user confirms or cancels. `staged` is the list of
// currently-staged paths; the wizard uses it to drive the stage
// review screen and to feed the suggester. `pre` carries values
// pre-populated from CLI flags. `suggest` is the heuristic
// engine (or a stub) used to pre-fill the type/scope.
//
// Returns CancelledError if the user quits without confirming.
func Run(staged []string, pre PrePopulated, suggest Suggester, theme tui.Theme) (Answers, error) {
	m := NewModel(staged, pre, suggest, theme)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return Answers{}, fmt.Errorf("commitwiz: %w", err)
	}
	fm, ok := finalModel.(*Model)
	if !ok {
		return Answers{}, fmt.Errorf("commitwiz: unexpected model type %T", finalModel)
	}
	if fm.state == stateCancelled {
		return Answers{}, CancelledError
	}
	if fm.state != stateDone {
		return Answers{}, CancelledError
	}
	return fm.answers, nil
}

// CancelledError is returned by Run when the user quits before
// finishing the wizard. Callers can match it via errors.Is or just
// check the message.
var CancelledError = gerr.Validation("commit cancelled")
