package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/commitwiz"
	"github.com/got-sh/got/internal/config"
	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
)

// commitOptions holds the resolved flag set for `got commit`. Flags
// follow §13 of got-spec.md: -a/--all to stage all tracked, -m/--message
// to skip the wizard with a single-line message, --no-verify to skip
// the conventional-commits validation, --amend to amend the previous
// commit, --no-tui/--no-interactive to force the non-interactive
// path even when stdout is a TTY.
type commitOptions struct {
	all        bool
	message    string
	noVerify   bool
	amend      bool
	noTUI      bool
	noInteract bool
	// Optional overrides the wizard would otherwise collect.
	typ            string
	scope          string
	body           string
	breaking       bool
	breakingReason string
}

// newCommitCmd builds the `got commit` subcommand.
func newCommitCmd(d Deps) *cobra.Command {
	opts := &commitOptions{}
	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Record changes to the repository with a Conventional Commits message",
		Long: `Record changes to the repository (alias: ci).

By default ` + "`got commit`" + ` runs an interactive Bubbletea wizard that
walks through stage review, type, scope, subject, body, breaking
change, and a final confirm. The wizard uses a heuristic suggester
that reads the staged files to recommend a Conventional Commits
type and scope.

Flags skip the wizard:
  -m, --message <msg>   single-line subject; uses --type=fix by default
  --no-tui, --no-interactive  use defaults, do not prompt
  -a, --all             stage modifications to all tracked files first
  --no-verify           skip Conventional Commits validation
  --amend               amend the previous commit`,
		Aliases: []string{"ci"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommit(cmd, d, opts)
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&opts.all, "all", "a", false, "stage all tracked files first")
	f.StringVarP(&opts.message, "message", "m", "", "single-line subject; bypasses the wizard")
	f.BoolVar(&opts.noVerify, "no-verify", false, "skip Conventional Commits validation")
	f.BoolVar(&opts.amend, "amend", false, "amend the previous commit")
	f.BoolVar(&opts.noTUI, "no-tui", false, "force non-interactive output even on a TTY")
	f.BoolVar(&opts.noInteract, "no-interactive", false, "alias for --no-tui")
	f.StringVar(&opts.typ, "type", "", "commit type (conventional/freeform)")
	f.StringVar(&opts.scope, "scope", "", "commit scope")
	f.StringVar(&opts.body, "body", "", "commit body")
	f.BoolVar(&opts.breaking, "breaking", false, "mark this commit as a breaking change")
	f.StringVar(&opts.breakingReason, "breaking-reason", "", "reason for --breaking (BREAKING CHANGE footer)")
	return cmd
}

// runCommit is the cobra handler body. It is split out from newCommitCmd
// so tests can call it via a constructed cobra command.
func runCommit(cmd *cobra.Command, deps Deps, opts *commitOptions) error {
	// --no-tui is a global flag (persistent on the root); read it
	// the same way init does so it works in tests that bypass
	// cobra's flag inheritance.
	if v, err := cmd.Root().PersistentFlags().GetBool("no-tui"); err == nil {
		opts.noTUI = v
	}

	start := "."
	workTree, err := deps.Discover(start)
	if err != nil {
		return err
	}

	// Load project config for commits.scopes and commits.allow_breaking.
	// Failure to read got.yml is not fatal: a missing config means
	// there are no configured scopes and the user can opt in to
	// breaking changes freely.
	allowBreaking := true
	var allowedScopes []string
	if cfg, err := config.ReadProjectConfig(filepath.Join(workTree, "got.yml")); err == nil {
		allowBreaking = cfg.Commits.AllowBreaking
		allowedScopes = cfg.Commits.Scopes
	}

	a := deps.AdapterFor(workTree)
	out := cmd.OutOrStdout()
	if out == nil {
		out = deps.Stdout
	}
	answers, err := resolveCommitAnswers(cmd, deps, a, workTree, opts, allowBreaking, allowedScopes)
	if err != nil {
		return err
	}

	return applyCommit(cmd.Context(), a, answers, opts, out)
}

// resolveCommitAnswers picks the wizard or the non-interactive path.
// Both paths return the same commitwiz.Answers struct.
func resolveCommitAnswers(cmd *cobra.Command, deps Deps, a git.Adapter, workTree string, opts *commitOptions, allowBreaking bool, allowedScopes []string) (commitwiz.Answers, error) {
	useWizard := !opts.noTUI && !opts.noInteract && opts.message == ""
	if useWizard && deps.IsTerminal != nil && !deps.IsTerminal() {
		useWizard = false
	}
	if useWizard && deps.RunCommitWizard == nil {
		useWizard = false
	}

	// Pre-populated struct shared by both paths.
	pre := commitwiz.PrePopulated{
		Type:           opts.typ,
		Scope:          opts.scope,
		Body:           opts.body,
		Breaking:       opts.breaking,
		BreakingReason: opts.breakingReason,
		StageAll:       opts.all,
		NoVerify:       opts.noVerify,
		AllowBreaking:  allowBreaking,
		AllowedScopes:  allowedScopes,
	}
	if opts.message != "" {
		// Split "subject\n\nbody" if the user passed a multi-line -m.
		pre.Subject = firstLine(opts.message)
		pre.Body = bodyAfterFirstLine(opts.message)
	}

	if !useWizard {
		a := commitwiz.Defaults()
		if pre.Type != "" {
			a.Type = pre.Type
		}
		if pre.Scope != "" {
			a.Scope = pre.Scope
		}
		if pre.Subject != "" {
			a.Subject = pre.Subject
		}
		if pre.Body != "" {
			a.Body = pre.Body
		}
		if pre.Breaking {
			a.Breaking = true
			a.BreakingReason = pre.BreakingReason
		}
		return a, nil
	}
	return deps.RunCommitWizard(currentStagedPaths(cmd, a), pre)
}

// applyCommit runs git commit with the resolved message + options and
// prints the resulting SHA.
func applyCommit(ctx context.Context, a git.Adapter, ans commitwiz.Answers, opts *commitOptions, out io.Writer) error {
	// Stage-all-tracked first if --all is set.
	if opts.all {
		if err := a.StageAllTracked(ctx); err != nil {
			return err
		}
	}
	// Apply any post-wizard staging changes the user made on the
	// stage-review screen.
	if len(ans.StagedAfter) > 0 {
		if err := a.Stage(ctx, ans.StagedAfter); err != nil {
			return err
		}
	}

	msg := ans.Render()
	// Reject empty subjects at the gate: a rendered message of
	// "feat: \n" is technically non-empty, so a plain `msg == ""`
	// check would let it through and produce a bogus commit. The
	// wizard's subject screen also bounces on empty input; this
	// keeps the non-interactive path consistent.
	if strings.TrimSpace(ans.Subject) == "" {
		return gerr.Validation("commit message subject is empty")
	}
	if msg == "" {
		return gerr.Validation("commit message is empty")
	}

	sha, err := a.Commit(ctx, msg, git.CommitOpts{
		Amend:    opts.amend,
		NoVerify: opts.noVerify,
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "[got %s] %s\n", shortSHA(sha), firstLine(msg))
	return nil
}

// currentStagedPaths returns the list of currently-staged paths by
// reading `git status --porcelain` and picking the staged entries.
// It is used by the wizard's stage-review screen and to feed the
// suggester. Best-effort: if status fails we return an empty slice
// and the wizard shows the empty-state message.
func currentStagedPaths(_ *cobra.Command, a git.Adapter) []string {
	s, err := a.Status(context.Background())
	if err != nil {
		return nil
	}
	var staged []string
	for _, e := range s.Entries {
		if e.IsStaged {
			staged = append(staged, e.Path)
		}
	}
	return staged
}

// firstLine returns the first line of s.
func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}

// bodyAfterFirstLine returns everything after the first newline, with
// the leading blank line (if any) preserved for the conventional
// commits body separator.
func bodyAfterFirstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[i+1:]
		}
	}
	return ""
}

// shortSHA returns the first 7 chars of a SHA, or the SHA itself if
// it's already short.
func shortSHA(sha git.SHA) string {
	s := string(sha)
	if len(s) <= 7 {
		return s
	}
	return s[:7]
}
