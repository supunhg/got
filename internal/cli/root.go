// Package cli implements the got command-line interface. It is the only
// package that should depend on Cobra; the rest of the codebase stays
// Cobra-agnostic so domain logic can be unit-tested without spinning up
// a command tree.
package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/version"
)

// NewRootCmd builds the root `got` command with all persistent flags and
// the v0.1 subcommand stubs. It is exposed for tests that want to drive
// the command tree without going through main().
func NewRootCmd() *cobra.Command {
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

	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newDecisionCmd())
	cmd.AddCommand(newNoteCmd())
	cmd.AddCommand(newOnboardCmd())

	return cmd
}

// Execute runs the root command. It is the single entry point used by
// cmd/got/main.go. Errors are written to stderr and returned for tests
// to assert against; main() translates the error into a non-zero exit
// code.
func Execute() error {
	return NewRootCmd().Execute()
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
