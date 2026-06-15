// Copyright 2026 Supun Hewagamage. MIT License.
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/supunhg/got/internal/events"
	"github.com/supunhg/got/internal/store"
)

// PluginRegistry represents a plugin registry entry.
type PluginRegistry struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
	URL         string `json:"url"`
	Downloads   int    `json:"downloads"`
}

// DefaultRegistryURL is the default plugin registry URL.
const DefaultRegistryURL = "https://registry.got.sh/plugins"

func newPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage GOT plugins",
		Long:  `Install, remove, enable, disable, and run GOT plugins.`,
	}

	cmd.AddCommand(newPluginInstallCmd())
	cmd.AddCommand(newPluginRemoveCmd())
	cmd.AddCommand(newPluginListCmd())
	cmd.AddCommand(newPluginEnableCmd())
	cmd.AddCommand(newPluginDisableCmd())
	cmd.AddCommand(newPluginRunCmd())
	cmd.AddCommand(newPluginSearchCmd())

	return cmd
}

func newPluginInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <path-or-url>",
		Short: "Install a plugin from a local directory or URL",
		Long:  `Install a GOT plugin from a local directory containing a manifest.json.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginInstall(cmd, args[0])
		},
	}
}

func newPluginRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginRemove(cmd, args[0])
		},
	}
}

func newPluginListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPluginList(cmd)
		},
	}
}

func newPluginEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <name>",
		Short: "Enable a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginEnable(cmd, args[0])
		},
	}
}

func newPluginDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <name>",
		Short: "Disable a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginDisable(cmd, args[0])
		},
	}
}

func newPluginRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <plugin> <command> [args...]",
		Short: "Run a plugin command",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginRun(cmd, args[0], args[1], args[2:])
		},
	}
}

func newPluginSearchCmd() *cobra.Command {
	var registryURL string

	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search for plugins in the registry",
		Long:  `Search for available plugins in the GOT plugin registry.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			return runPluginSearch(cmd, query, registryURL)
		},
	}

	cmd.Flags().StringVar(&registryURL, "registry", DefaultRegistryURL, "Plugin registry URL")

	return cmd
}

func runPluginInstall(cmd *cobra.Command, source string) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("plugin install: %w", err)
	}

	gotDir := filepath.Join(repoPath, ".got")
	pluginsDir := filepath.Join(gotDir, PluginDirName)

	// Ensure plugins directory exists
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return fmt.Errorf("plugin install: create plugins dir: %w", err)
	}

	// Check if source is a URL
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		// Download and extract plugin from URL
		fmt.Fprintf(cmd.OutOrStdout(), "Downloading plugin from %s...\n", source)
		// TODO: Implement plugin download and extraction
		return fmt.Errorf("plugin install: URL installation not yet implemented")
	}

	// Local directory installation
	manifest, err := ParseManifestFile(source)
	if err != nil {
		return fmt.Errorf("plugin install: %w", err)
	}

	// Copy plugin to plugins directory
	pluginDir := filepath.Join(pluginsDir, manifest.Name)
	if err := copyDir(source, pluginDir); err != nil {
		return fmt.Errorf("plugin install: copy plugin: %w", err)
	}

	// Register in database
	gotDBPath := filepath.Join(gotDir, "got.db")
	s, err := store.Open(gotDBPath)
	if err != nil {
		return fmt.Errorf("plugin install: open store: %w", err)
	}
	defer s.Close()

	bus := globalBus
	if bus == nil {
		bus = newBackgroundBus()
	}
	ks := store.NewKnowledgeStore(s.DB(), bus)

	// Convert manifest to JSON
	manifestJSON := fmt.Sprintf(`{"name":%q,"version":%q,"description":%q}`,
		manifest.Name, manifest.Version, manifest.Description)

	if _, err := ks.InstallPlugin(ctx, manifest.Name, manifest.Version, manifest.Description, pluginDir, manifestJSON); err != nil {
		return fmt.Errorf("plugin install: register: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Installed plugin %s v%s\n", manifest.Name, manifest.Version)
	return nil
}

func runPluginRemove(cmd *cobra.Command, name string) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("plugin remove: %w", err)
	}

	gotDir := filepath.Join(repoPath, ".got")
	gotDBPath := filepath.Join(gotDir, "got.db")

	s, err := store.Open(gotDBPath)
	if err != nil {
		return fmt.Errorf("plugin remove: open store: %w", err)
	}
	defer s.Close()

	bus := globalBus
	if bus == nil {
		bus = newBackgroundBus()
	}
	ks := store.NewKnowledgeStore(s.DB(), bus)

	if err := ks.RemovePlugin(ctx, name); err != nil {
		return fmt.Errorf("plugin remove: %w", err)
	}

	// Remove from disk
	pluginDir := filepath.Join(gotDir, PluginDirName, name)
	_ = os.RemoveAll(pluginDir)

	fmt.Fprintf(cmd.OutOrStdout(), "Removed plugin %s\n", name)
	return nil
}

func runPluginList(cmd *cobra.Command) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("plugin list: %w", err)
	}

	gotDir := filepath.Join(repoPath, ".got")
	gotDBPath := filepath.Join(gotDir, "got.db")

	s, err := store.Open(gotDBPath)
	if err != nil {
		return fmt.Errorf("plugin list: open store: %w", err)
	}
	defer s.Close()

	bus := globalBus
	if bus == nil {
		bus = newBackgroundBus()
	}
	ks := store.NewKnowledgeStore(s.DB(), bus)

	plugins, err := ks.ListPlugins(ctx)
	if err != nil {
		return fmt.Errorf("plugin list: %w", err)
	}

	if len(plugins) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No plugins installed.")
		return nil
	}

	for _, p := range plugins {
		status := "enabled"
		if !p.Enabled {
			status = "disabled"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", p.Name, p.Version, status)
	}

	return nil
}

func runPluginEnable(cmd *cobra.Command, name string) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("plugin enable: %w", err)
	}

	gotDir := filepath.Join(repoPath, ".got")
	gotDBPath := filepath.Join(gotDir, "got.db")

	s, err := store.Open(gotDBPath)
	if err != nil {
		return fmt.Errorf("plugin enable: open store: %w", err)
	}
	defer s.Close()

	bus := globalBus
	if bus == nil {
		bus = newBackgroundBus()
	}
	ks := store.NewKnowledgeStore(s.DB(), bus)

	if err := ks.EnablePlugin(ctx, name); err != nil {
		return fmt.Errorf("plugin enable: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Enabled plugin %s\n", name)
	return nil
}

func runPluginDisable(cmd *cobra.Command, name string) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("plugin disable: %w", err)
	}

	gotDir := filepath.Join(repoPath, ".got")
	gotDBPath := filepath.Join(gotDir, "got.db")

	s, err := store.Open(gotDBPath)
	if err != nil {
		return fmt.Errorf("plugin disable: open store: %w", err)
	}
	defer s.Close()

	bus := globalBus
	if bus == nil {
		bus = newBackgroundBus()
	}
	ks := store.NewKnowledgeStore(s.DB(), bus)

	if err := ks.DisablePlugin(ctx, name); err != nil {
		return fmt.Errorf("plugin disable: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Disabled plugin %s\n", name)
	return nil
}

func runPluginRun(cmd *cobra.Command, pluginName, commandName string, args []string) error {
	if globalPluginRuntime == nil {
		return fmt.Errorf("plugin run: plugins not loaded (run inside a GOT repository)")
	}

	ctx := context.Background()
	stdout, stderr, err := globalPluginRuntime.ExecuteCommand(ctx, pluginName, commandName, args, 30*time.Second)
	if err != nil {
		return fmt.Errorf("plugin run: %w", err)
	}

	if stdout != "" {
		fmt.Fprint(cmd.OutOrStdout(), stdout)
	}
	if stderr != "" {
		fmt.Fprint(cmd.ErrOrStderr(), stderr)
	}

	return nil
}

func runPluginSearch(cmd *cobra.Command, query, registryURL string) error {
	fmt.Fprintf(cmd.OutOrStdout(), "Searching for plugins at %s...\n\n", registryURL)

	// TODO: Implement actual registry search
	// For now, show a placeholder
	fmt.Fprintln(cmd.OutOrStdout(), "Plugin registry coming soon!")
	fmt.Fprintln(cmd.OutOrStdout(), "\nFor now, install plugins from local directories:")
	fmt.Fprintln(cmd.OutOrStdout(), "  got plugin install ./my-plugin")

	return nil
}

// newBackgroundBus creates a new event bus for operations outside the CLI context.
func newBackgroundBus() *events.Bus {
	return events.New()
}

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Copy file
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(dstPath, data, info.Mode())
	})
}
