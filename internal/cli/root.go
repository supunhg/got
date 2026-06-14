// Package cli implements the got command-line interface. It is the only
// package that should depend on Cobra; the rest of the codebase stays
// Cobra-agnostic so domain logic can be unit-tested without spinning up
// a command tree.
//
// Copyright 2026 The GOT Authors. MIT License.
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/supunhg/got/internal/events"
	"github.com/supunhg/got/internal/store"
	"github.com/supunhg/got/internal/version"
)

// Global shared bus and plugin runtime for event-driven plugins.
//
// The bus is created once and shared across all CLI commands so that
// plugin hook subscribers can receive events published by any command.
// The plugin runtime is loaded on startup (unless --no-plugins) and
// stored here for reuse (e.g., by `got plugin run`).
var (
	globalBus             *events.Bus
	globalPluginRuntime   *PluginRuntime
	globalPluginRuntimeMu sync.Once
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
	// §13 of the spec. The defaults match the spec too.
	pf := cmd.PersistentFlags()
	pf.String("cwd", "", "operate on a different directory")
	pf.Bool("no-color", false, "disable lip gloss styles")
	pf.Bool("no-tui", false, "force plain CLI output even in wizards (CI-friendly)")
	pf.String("log-level", "warn", "log level: debug|info|warn|error")
	pf.Duration("plugin-timeout", 30*time.Second, "plugin invocation timeout")
	pf.Bool("no-plugins", false, "skip loading plugins on startup")

	// PersistentPreRunE runs before every command. We use it to lazily
	// create the shared event bus and load plugins once on the first
	// command invocation, unless --no-plugins is set.
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Only load once.
		var loadErr error
		globalPluginRuntimeMu.Do(func() {
			// Create the shared bus once.
			globalBus = events.New()

			noPlugins, _ := cmd.Flags().GetBool("no-plugins")
			if noPlugins {
				return
			}

			// Try to find .got/ — if not initialized yet, skip silently
			// (the user might be running `got init` or `got version`).
			runtime, err := loadPlugins(globalBus)
			if err != nil {
				loadErr = err
				return
			}
			globalPluginRuntime = runtime
		})
		return loadErr
	}

	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newDecisionCmd())
	cmd.AddCommand(newNoteCmd())
	cmd.AddCommand(newOnboardCmd())
	cmd.AddCommand(newSearchCmd())
	cmd.AddCommand(newWorkspaceCmd())
	cmd.AddCommand(newPluginCmd())
	cmd.AddCommand(newGitHubCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newCommitCmd())
	cmd.AddCommand(newBranchCmd())
	cmd.AddCommand(newGraphCmd())
	cmd.AddCommand(newRemoteCmd())
	cmd.AddCommand(newCompletionCmd())
	cmd.AddCommand(newHealthCmd())
	cmd.AddCommand(newSnapshotCmd())
	cmd.AddCommand(newSafeCmd())

	return cmd
}

// Execute runs the root command. It is the single entry point used by
// cmd/got/main.go. Errors are written to stderr and returned for tests
// to assert against; main() translates the error into a non-zero exit
// code.
func Execute() error {
	return NewRootCmd().Execute()
}

// loadPlugins opens the GOT store and loads all enabled plugins onto
// the given bus. Returns nil, nil if GOT is not initialized.
func loadPlugins(bus *events.Bus) (*PluginRuntime, error) {
	gotDir, err := findGotDir()
	if err != nil {
		return nil, nil
	}

	dbPath := filepath.Join(gotDir, "got.db")
	if _, err := os.Stat(dbPath); err != nil {
		return nil, nil
	}

	s, err := store.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open store for plugins: %w", err)
	}

	// Use the shared bus for the runtime's store too, so events published
	// by plugin operations go through the same bus.
	ks := store.NewKnowledgeStore(s.DB(), bus)
	pluginsDir := filepath.Join(gotDir, PluginDirName)

	rt := NewPluginRuntime(ks, bus, pluginsDir)
	if err := rt.Load(bgContext); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: plugin load: %v\n", err)
	}

	return rt, nil
}

// bgContext returns a background context for plugin loading.
var bgContext = context.Background()

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
