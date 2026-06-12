// Package cli implements the got command-line interface. It is the only
// package that should depend on Cobra; the rest of the codebase stays
// Cobra-agnostic so domain logic can be unit-tested without spinning up
// a command tree.
package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/version"
)

// loggerFor returns deps.Logger or a no-op fallback. Centralized so
// command files don't have to nil-check the logger at every call
// site and so all commands behave consistently when Logger is
// unset (tests, TUI subcommands, plugin stubs).
func loggerFor(d Deps) *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

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
	if deps.RunCommitWizard != nil {
		d.RunCommitWizard = deps.RunCommitWizard
	}
	if deps.RunBranchWizard != nil {
		d.RunBranchWizard = deps.RunBranchWizard
	}
	if deps.RunGraphWizard != nil {
		d.RunGraphWizard = deps.RunGraphWizard
	}
	if deps.RunWorktreeWizard != nil {
		d.RunWorktreeWizard = deps.RunWorktreeWizard
	}
	if deps.RunDashboardWizard != nil {
		d.RunDashboardWizard = deps.RunDashboardWizard
	}
	if deps.DiscoverPlugins != nil {
		d.DiscoverPlugins = deps.DiscoverPlugins
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
	if deps.Logger != nil {
		d.Logger = deps.Logger
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
	pf.String("log-level", "warn", "log level: debug|info|warn|error (overrides the spec §16 default)")
	pf.String("log-format", "text", "log output format: text|json")
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
	cmd.AddCommand(newGraphCmd(d))
	cmd.AddCommand(newPluginCmd(d))
	cmd.AddCommand(newWorktreeCmd(d))
	cmd.AddCommand(newTUICmd(d))

	// Auto-register discovered plugins as `got <plugin-name>
	// <command>` subcommands per spec §11. v0.1 only registers
	// stubs; live invocation lands in v0.5.
	registerPluginCommands(cmd, d)

	return cmd
}

// Execute runs the root command with the default dependencies and
// the provided logger. It is the single entry point used by
// cmd/got/main.go. Errors are written to stderr via gerr.UserMessage
// and the exit code is set per got-spec.md §15.
//
// Cobra's command tree returns pflag errors (unknown flag, bad flag
// value, etc.) as plain *flag.Error values, which gerr.ExitCode would
// map to CodeGeneric (1). For spec §15 compliance, those are usage
// errors and should map to CodeUsage (2): we wrap them here so the
// exit code is right. Any other non-gerr error is wrapped as a
// generic gerr.Error so the main entry point can rely on errors.As.
//
// logger may be nil, in which case slog.Default() is used. Per
// spec §16, the TUI commands install a discard logger so the
// dashboard never writes to stderr.
func Execute(logger *slog.Logger) error {
	// Thread the logger into the git adapter factory so spec §16
	// "raw git invocations and exit codes" debug records are
	// emitted by ExecAdapter.run. defaultDepsWithLogger stores
	// the logger on Deps AND captures it in the AdapterFor
	// closure; passing nil here is safe and equivalent to
	// defaultDeps().
	deps := defaultDepsWithLogger(logger)
	err := NewRootCmd(deps).Execute()
	return wrapExecuteError(err)
}

// wrapExecuteError maps Cobra's low-level flag errors to typed gerr
// values so the exit-code scheme in spec §15 fires correctly. It is
// split out from Execute so tests can exercise the mapping directly.
//
// Implementation note: Cobra does not export stable sentinels for the
// "you typed the wrong thing" cases (unknown subcommand, unknown
// flag, bad arg, etc.) — some are *pflag.Error (unexported type in
// some versions), some are cobra.Err*, some are plain fmt.Errorf.
// We use stable string prefixes on the error message instead. The
// strings come from Cobra's own user-facing templates in command.go
// and have been stable since Cobra v1.0.
func wrapExecuteError(err error) error {
	if err == nil {
		return nil
	}
	// Already a typed gerr error: pass through unchanged so
	// errors.As, errors.Is, and the original Code/Hint/Cause are
	// preserved.
	if _, ok := err.(*gerr.Error); ok {
		return err
	}
	// Cobra prints help itself when --help / -h is passed; the
	// returned error is the cobra.ErrHelp sentinel (or a
	// fmt.Errorf wrapping it). Either way, no error to surface.
	if strings.Contains(err.Error(), "help requested") {
		return nil
	}
	if isCobraUsageError(err) {
		return gerr.Usage(err.Error())
	}
	// Anything else: return as-is. gerr.ExitCode falls back to
	// CodeGeneric (1) for non-*gerr.Error values, and
	// gerr.UserMessage falls back to err.Error(). Wrapping with
	// gerr.Wrap(CodeGeneric, err, err.Error()) would render as
	// "msg: msg" because gerr.Error.Error() stringifies both the
	// Message and the Cause; passing through unchanged is
	// strictly cleaner.
	return err
}

// isCobraUsageError returns true for errors that Cobra produces for
// bad CLI input. These are the "you typed the wrong thing" cases
// that spec §15 wants to map to CodeUsage (2). The strings are
// stable across Cobra versions because they originate from the
// user-facing templates in command.go and pflag's error helpers.
func isCobraUsageError(err error) bool {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "unknown command "):
		// Cobra v1.6+: "unknown command %q for %q"
		return true
	case strings.HasPrefix(msg, "unknown flag: "):
		// pflag: bad --foo
		return true
	case strings.HasPrefix(msg, "invalid argument "):
		// pflag: bad value for a flag
		return true
	case strings.HasPrefix(msg, "bad flag syntax: "):
		// pflag: malformed -foo=bar
		return true
	case strings.HasPrefix(msg, "flag needs an argument: "):
		// pflag: --foo with no value
		return true
	case strings.HasPrefix(msg, "invalid value "):
		// pflag: --foo=badvalue
		return true
	case strings.HasPrefix(msg, "required flag(s) "):
		// pflag: missing required flag
		return true
	}
	return false
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
