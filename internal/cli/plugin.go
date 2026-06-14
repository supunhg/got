// Copyright 2026 Supun Hewagamage. MIT License.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// newPluginCmd builds the `got plugin` command tree.
func newPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage GOT plugins",
		Long: `Manage GOT plugins — install, list, enable, disable, and run plugins.

Plugins extend GOT's functionality by subscribing to events and adding
new CLI commands. They are installed from a local directory containing
a manifest.json file and placed in .got/plugins/<name>/.

Requires GOT to be initialized in a Git repository (.got/ must exist).`,
	}

	cmd.AddCommand(newPluginInstallCmd())
	cmd.AddCommand(newPluginRemoveCmd())
	cmd.AddCommand(newPluginListCmd())
	cmd.AddCommand(newPluginEnableCmd())
	cmd.AddCommand(newPluginDisableCmd())
	cmd.AddCommand(newPluginRunCmd())

	return cmd
}

func newPluginInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <path>",
		Short: "Install a plugin from a local directory",
		Long: `Install a plugin from a local directory.

Copies the plugin directory into .got/plugins/<name>/, validates the
manifest, and registers the plugin in the database.

Example:
  got plugin install ./my-plugin
  got plugin install /path/to/plugins/hello-world`,
		Args: cobra.ExactArgs(1),
		RunE: runPluginInstall,
	}
}

func newPluginRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Uninstall a plugin",
		Long: `Uninstall a plugin by name.

Removes the plugin from the database and deletes its directory from
.got/plugins/<name>/. The plugin will no longer be loaded on startup.

Example:
  got plugin remove hello-world`,
		Args: cobra.ExactArgs(1),
		RunE: runPluginRemove,
	}
}

func newPluginListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		Long:  `List all installed plugins, their version, and enabled/disabled status.`,
		Args:  cobra.NoArgs,
		RunE:  runPluginList,
	}
	return cmd
}

func newPluginEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <name>",
		Short: "Enable a plugin",
		Long:  `Enable a previously disabled plugin. The plugin will be loaded on next startup.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runPluginEnable,
	}
}

func newPluginDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <name>",
		Short: "Disable a plugin",
		Long:  `Disable a plugin without uninstalling it. The plugin will not be loaded on startup.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runPluginDisable,
	}
}

func newPluginRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <name> <action>",
		Short: "Manually run a plugin action or hook",
		Long: `Run a specific plugin action or hook for debugging.

Actions are defined in the plugin manifest.json under "hooks" (event type
keys) or "commands" (command names).

Example:
  got plugin run hello-world greet
  got plugin run my-plugin on-commit`,
		Args: cobra.ExactArgs(2),
		RunE: runPluginRun,
	}
	return cmd
}

// ── Run functions ───────────────────────────────────────────────────

func runPluginInstall(cmd *cobra.Command, args []string) error {
	srcPath := args[0]

	// Resolve to absolute path.
	absSrc, err := filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// Stat the source to confirm it exists.
	srcInfo, err := os.Stat(absSrc)
	if err != nil {
		return fmt.Errorf("access plugin source: %w", err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("plugin source must be a directory: %s", absSrc)
	}

	// Parse the manifest.
	manifest, err := ParseManifestFile(absSrc)
	if err != nil {
		return fmt.Errorf("invalid plugin manifest: %w", err)
	}

	// Open the store.
	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	gotDir, err := findGotDir()
	if err != nil {
		return fmt.Errorf("find .got/: %w", err)
	}
	pluginsDir := filepath.Join(gotDir, PluginDirName)

	// Check if already installed.
	if existing, _ := ks.GetPlugin(cmd.Context(), manifest.Name); existing != nil {
		return fmt.Errorf("plugin %q already installed (use 'got plugin remove %s' first)", manifest.Name, manifest.Name)
	}

	// Copy plugin to .got/plugins/<name>/.
	destDir := filepath.Join(pluginsDir, manifest.Name)
	if err := copyDir(absSrc, destDir); err != nil {
		return fmt.Errorf("copy plugin: %w", err)
	}

	// Serialize manifest for storage.
	manifestBytes, _ := json.Marshal(manifest)

	// Register in DB.
	p, err := ks.InstallPlugin(cmd.Context(), manifest.Name, manifest.Version, manifest.Description, destDir, string(manifestBytes))
	if err != nil {
		// Rollback the copied directory.
		os.RemoveAll(destDir)
		return fmt.Errorf("register plugin: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Installed plugin: %s v%s\n", p.Name, p.Version)
	fmt.Fprintf(cmd.OutOrStdout(), "  Path: %s\n", destDir)
	fmt.Fprintf(cmd.OutOrStdout(), "  Capabilities: %s\n", strings.Join(manifest.Capabilities, ", "))
	if len(manifest.Hooks) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  Event hooks: %d\n", len(manifest.Hooks))
	}
	if len(manifest.Commands) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  Commands: %d\n", len(manifest.Commands))
	}

	return nil
}

func runPluginRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	// Get the plugin to know its path.
	p, err := ks.GetPlugin(cmd.Context(), name)
	if err != nil {
		return fmt.Errorf("get plugin: %w", err)
	}

	// Remove from DB.
	if err := ks.RemovePlugin(cmd.Context(), name); err != nil {
		return fmt.Errorf("remove plugin: %w", err)
	}

	// Remove plugin directory.
	pluginDir := p.Path
	if pluginDir == "" {
		gotDir, _ := findGotDir()
		pluginDir = filepath.Join(gotDir, PluginDirName, name)
	}
	os.RemoveAll(pluginDir)

	fmt.Fprintf(cmd.OutOrStdout(), "Removed plugin: %s\n", name)
	return nil
}

func runPluginList(cmd *cobra.Command, args []string) error {
	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	plugins, err := ks.ListPlugins(cmd.Context())
	if err != nil {
		return fmt.Errorf("list plugins: %w", err)
	}

	if len(plugins) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No plugins installed.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tENABLED\tDESCRIPTION")
	fmt.Fprintln(w, "----\t-------\t-------\t-----------")

	for _, p := range plugins {
		enabled := "yes"
		if !p.Enabled {
			enabled = "no"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Name, p.Version, enabled, p.Description)
	}
	w.Flush()

	return nil
}

func runPluginEnable(cmd *cobra.Command, args []string) error {
	name := args[0]

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	if err := ks.EnablePlugin(cmd.Context(), name); err != nil {
		return fmt.Errorf("enable plugin: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Enabled plugin: %s\n", name)
	return nil
}

func runPluginDisable(cmd *cobra.Command, args []string) error {
	name := args[0]

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	if err := ks.DisablePlugin(cmd.Context(), name); err != nil {
		return fmt.Errorf("disable plugin: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Disabled plugin: %s\n", name)
	return nil
}

func runPluginRun(cmd *cobra.Command, args []string) error {
	name := args[0]
	action := args[1]

	// Use the global runtime if already loaded (e.g., if --no-plugins was
	// NOT set at startup). Otherwise fall back to loading it here.
	rt := globalPluginRuntime

	if rt == nil {
		// No global runtime — load one specifically for this command.
		kc, err := openKnowledgeStore()
		if err != nil {
			return err
		}
		defer kc.Close()
		ks := kc.ks

		gotDir, gotErr := findGotDir()
		if gotErr != nil {
			return fmt.Errorf("find .got/: %w", gotErr)
		}
		pluginsDir := filepath.Join(gotDir, PluginDirName)

		localRT := NewPluginRuntime(ks, kc.bus, pluginsDir)
		defer localRT.Close()

		ctx := cmd.Context()
		if loadErr := localRT.Load(ctx); loadErr != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: plugin load: %v\n", loadErr)
		}

		rt = localRT
	}

	timeout, _ := cmd.Flags().GetDuration("plugin-timeout")

	stdout, stderr, runErr := rt.RunAction(cmd.Context(), name, action, timeout)
	if stdout != "" {
		fmt.Fprint(cmd.OutOrStdout(), stdout)
	}
	if stderr != "" {
		fmt.Fprint(cmd.ErrOrStderr(), stderr)
	}
	if runErr != nil {
		return fmt.Errorf("run plugin: %w", runErr)
	}

	return nil
}

// ── Directory copy helper ───────────────────────────────────────────

// copyDir copies a directory recursively from src to dst.
// Used during plugin installation.
func copyDir(src, dst string) error {
	// Create destination directory.
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("create destination: %w", err)
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, readErr := os.ReadFile(srcPath)
			if readErr != nil {
				return fmt.Errorf("read %s: %w", srcPath, readErr)
			}
			if writeErr := os.WriteFile(dstPath, data, 0o644); writeErr != nil {
				return fmt.Errorf("write %s: %w", dstPath, writeErr)
			}
		}
	}

	return nil
}
