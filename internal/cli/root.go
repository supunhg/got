// Package cli implements the got command-line interface. It is the only
// package that should depend on Cobra; the rest of the codebase stays
// Cobra-agnostic so domain logic can be unit-tested without spinning up
// a command tree.
package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/version"
)

// NewRootCmd builds the root `got` command with all persistent flags and
// the v0.1 subcommand stubs. It is exposed for tests that want to drive
// the command tree without going through main(). Nil fields in deps are
// filled in from defaultDeps().
func NewRootCmd(deps Deps) *cobra.Command {
	d := defaultDeps()
	if deps.AdapterFor != nil {
		d.AdapterFor = deps.AdapterFor
	}
	if deps.Discover != nil {
		d.Discover = deps.Discover
	}
	if deps.StoreFor != nil {
		d.StoreFor = deps.StoreFor
	}
	if deps.RunWizard != nil {
		d.RunWizard = deps.RunWizard
	}
	if deps.IsTerminal != nil {
		d.IsTerminal = deps.IsTerminal
	}
	if deps.Now != nil {
		d.Now = deps.Now
	}
	if deps.User != nil {
		d.User = deps.User
	}
	if deps.GotVersion != "" {
		d.GotVersion = deps.GotVersion
	} else {
		// Default to the running binary's version string.
		d.GotVersion = version.String()
	}
	if deps.Stdout != nil {
		d.Stdout = deps.Stdout
	}
	if deps.Stderr != nil {
		d.Stderr = deps.Stderr
	}

	cmd := &cobra.Command{
		Use:   "got",
		Short: "Git-native developer operating layer",
		Long: `GOT is a Git-native developer operating layer.

GOT does not replace Git. It enhances Git with workflow abstraction,
safety mechanisms, repository intelligence, team knowledge management,
and interactive developer experiences.

Git remains the source of truth; GOT metadata lives in .got/.`,
		Version:       version.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// Use the version package's full string verbatim. The default Cobra
	// template would prepend "<name> version ", which is redundant given
	// that the string already starts with "got ".
	cmd.SetVersionTemplate("{{.Version}}\n")

	// Persistent flags available on every subcommand. The set matches
	// §13 of got-spec.md. The defaults match the spec too.
	pf := cmd.PersistentFlags()
	pf.String("cwd", "", "operate on a different directory")
	pf.Bool("no-color", false, "disable lip gloss styles")
	pf.Bool("no-tui", false, "force plain CLI output even in wizards (CI-friendly)")
	pf.String("log-level", "warn", "log level: debug|info|warn|error")
	pf.Duration("plugin-timeout", 30*time.Second, "plugin invocation timeout")

	// Default the context so subcommands can read cmd.Context().
	cmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if cmd.Context() == nil {
			cmd.SetContext(context.Background())
		}
		return nil
	}

	// Subcommands. v0.1 has: version, init, status, commit, branch, remote, tui (stub).
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newInitCmd(d))
	cmd.AddCommand(newStatusCmd(d))
	cmd.AddCommand(newCommitCmd(d))
	cmd.AddCommand(newBranchCmd(d))
	cmd.AddCommand(newRemoteCmd(d))
	cmd.AddCommand(newTUIStubCmd())

	return cmd
}

// Execute runs the root command with the default dependencies. It is
// the single entry point used by cmd/got/main.go. Errors are written
// to stderr via gerr.UserMessage and the exit code is set per
// got-spec.md §15.
func Execute() error {
	return NewRootCmd(defaultDeps()).Execute()
}

// newVersionCmd returns the `got version` subcommand. It prints the same
// string as `--version` so scripts can capture either form.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the got version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), version.String())
		},
	}
}

// newTUIStubCmd is a placeholder for the dashboard TUI. Step 9 of §24
// will replace it with the real dashboard.
func newTUIStubCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Open the dashboard TUI (not yet implemented in v0.1)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return gerr.Validation("`got tui` is not yet implemented in v0.1; it lands in step 9 of got-spec.md §24")
		},
	}
}
