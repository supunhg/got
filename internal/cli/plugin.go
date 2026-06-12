package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/plugin"
	"github.com/got-sh/got/internal/repo"
)

// pluginLogger returns deps.Logger or a no-op fallback. Centralized
// so plugin commands don't have to nil-check at every call site.
func pluginLogger(d Deps) *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newPluginCmd builds the `got plugin` subcommand tree per spec §11.
//
//	got plugin list [--json] [--all|--enabled|--disabled]   list discovered plugins
//	got plugin info <name> [--json]                        show a plugin's full manifest
//	got plugin install <src>  [--force]                    install from local path or git URL
//	got plugin enable <name>                                add to got.yml plugins.enabled
//	got plugin disable <name>                               remove from got.yml plugins.enabled
//	got plugin search <query>                               [v0.2+] registry search; stubbed
//
// In addition, any plugins found via discovery are registered as
// `got <plugin-name> <command>` subcommands under root. v0.1 ships
// zero plugins so the registered subcommands return a clear "not
// yet implemented" message; live invocation lands in v0.5.
func newPluginCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage GOT plugins",
		Long: `Discover, inspect, install, and enable/disable external GOT plugins.

A plugin is an executable named got-<name> on $PATH or in
.got/plugins/. When GOT starts, each candidate is invoked with
` + "`--got-plugin-manifest`" + ` and the returned JSON manifest is
parsed and validated. Commands declared in the manifest are
registered as ` + "`got <plugin-name> <command>`" + ` subcommands.

v0.1 ships zero plugins. The interface, discovery, and manifest
protocol are fully implemented; install + enable/disable + per-repo
registry are implemented in this build; live invocation lands in v0.5.`,
	}
	cmd.AddCommand(newPluginListCmd(d))
	cmd.AddCommand(newPluginInfoCmd(d))
	cmd.AddCommand(newPluginInstallCmd(d))
	cmd.AddCommand(newPluginEnableCmd(d))
	cmd.AddCommand(newPluginDisableCmd(d))
	cmd.AddCommand(newPluginSearchCmd())
	return cmd
}

// newPluginListCmd builds `got plugin list [--json] [--all|--enabled|--disabled]`.
func newPluginListCmd(d Deps) *cobra.Command {
	var (
		asJSON  bool
		showAll bool
		showEna bool
		showDis bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List discovered plugins",
		Long: `List discovered plugins.

By default only enabled plugins are shown. Pass --all to also show
plugins that are installed but not enabled, --enabled to force
enabled-only, or --disabled to show only disabled. The ENABLED
column reflects got.yml's plugins.enabled list.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPluginList(cmd.Context(), cmd, d, asJSON, parseListFilter(showAll, showEna, showDis))
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable JSON output")
	cmd.Flags().BoolVar(&showAll, "all", false, "show enabled and disabled plugins")
	cmd.Flags().BoolVar(&showEna, "enabled", false, "show only enabled plugins")
	cmd.Flags().BoolVar(&showDis, "disabled", false, "show only disabled plugins")
	cmd.MarkFlagsMutuallyExclusive("all", "enabled", "disabled")
	return cmd
}

// listFilter encodes the --all/--enabled/--disabled selection. The
// default (all false) is "enabled only" — the conservative view that
// matches what the rest of GOT (and the dashboard's Plugins tab) sees
// at startup.
type listFilter int

const (
	filterEnabled listFilter = iota
	filterAll
	filterDisabled
)

// parseListFilter resolves the mutually-exclusive --all/--enabled/--disabled
// flags into a listFilter. The default is "enabled only" — the conservative
// view that matches what the rest of GOT (and the dashboard's Plugins tab)
// sees at startup. The flags are marked mutually exclusive by
// MarkFlagsMutuallyExclusive in the command definition.
func parseListFilter(showAll, showEna, showDis bool) listFilter {
	switch {
	case showAll:
		return filterAll
	case showDis:
		return filterDisabled
	case showEna:
		return filterEnabled
	default:
		return filterEnabled
	}
}

func runPluginList(ctx context.Context, cmd *cobra.Command, d Deps, asJSON bool, filter listFilter) error {
	plugins, err := d.DiscoverPlugins(ctx)
	if err != nil {
		return err
	}
	enabled, err := loadEnabledSet()
	if err != nil {
		return err
	}
	view := filterPlugins(plugins, enabled, filter)
	out := cmdWriter(cmd, d)
	if asJSON {
		return writeJSON(out, view)
	}
	return writePluginTable(out, view, enabled)
}

// filterPlugins applies the list filter to the discovered list,
// decorating each entry with its enabled status.
func filterPlugins(in []plugin.DiscoveredPlugin, enabled map[string]bool, f listFilter) []plugin.DiscoveredPlugin {
	out := make([]plugin.DiscoveredPlugin, 0, len(in))
	for _, p := range in {
		isOn := enabled[p.Name]
		switch f {
		case filterAll:
			out = append(out, p)
		case filterEnabled:
			if isOn {
				out = append(out, p)
			}
		case filterDisabled:
			if !isOn {
				out = append(out, p)
			}
		}
	}
	return out
}

func writePluginTable(w io.Writer, plugins []plugin.DiscoveredPlugin, enabled map[string]bool) error {
	if len(plugins) == 0 {
		_, err := fmt.Fprintln(w, "(no plugins discovered)")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tVERSION\tMIN GOT\tSOURCE\tPATH\tENABLED")
	for _, p := range plugins {
		marker := "no"
		if enabled[p.Name] {
			marker = "yes"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", p.Name, p.Version, p.MinGOT, p.Source, p.Path, marker)
	}
	return tw.Flush()
}

// newPluginInfoCmd builds `got plugin info <name> [--json]`.
func newPluginInfoCmd(d Deps) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "info <name>",
		Short: "Show a plugin's full manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginInfo(cmd.Context(), cmd, d, args[0], asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable JSON output")
	return cmd
}

func runPluginInfo(ctx context.Context, cmd *cobra.Command, d Deps, name string, asJSON bool) error {
	plugins, err := d.DiscoverPlugins(ctx)
	if err != nil {
		return err
	}
	for _, p := range plugins {
		if p.Name == name {
			out := cmdWriter(cmd, d)
			if asJSON {
				return writeJSON(out, p)
			}
			return writePluginInfo(out, p)
		}
	}
	return gerr.Validation(fmt.Sprintf("plugin %q not found", name))
}

func writePluginInfo(w io.Writer, p plugin.DiscoveredPlugin) error {
	_, _ = fmt.Fprintf(w, "Name:     %s\n", p.Name)
	_, _ = fmt.Fprintf(w, "Version:  %s\n", p.Version)
	_, _ = fmt.Fprintf(w, "Min GOT:  %s\n", p.MinGOT)
	_, _ = fmt.Fprintf(w, "Source:   %s\n", p.Source)
	_, _ = fmt.Fprintf(w, "Path:     %s\n", p.Path)
	if len(p.Commands) == 0 {
		_, _ = fmt.Fprintln(w, "Commands: (none declared)")
		return nil
	}
	_, _ = fmt.Fprintln(w, "Commands:")
	for _, c := range p.Commands {
		_, _ = fmt.Fprintf(w, "  %s\t%s\n", c.Name, c.Description)
	}
	return nil
}

// newPluginInstallCmd builds `got plugin install <source> [--force]`.
// The source is either a local file path to a plugin binary or a
// git URL (http(s)://, git://, ssh://, or user@host:path).
func newPluginInstallCmd(d Deps) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "install <source>",
		Short: "Install a plugin from a local path or git URL",
		Long: `Install a plugin into the current repo's .got/plugins/ directory.

The source is either a local path to a plugin binary or a git URL
(http(s)://, git://, ssh://, or user@host:path). The destination
filename is derived from the source's manifest: got-<name>. Pass
--force to overwrite an existing binary at the destination.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginInstall(cmd.Context(), cmd, d, args[0], force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing binary at the destination")
	return cmd
}

func runPluginInstall(_ context.Context, cmd *cobra.Command, d Deps, source string, force bool) error {
	logger := pluginLogger(d)
	logger.Info("plugin install starting", "source", source, "force", force)
	workTree, err := d.Discover(".")
	if err != nil {
		return err
	}
	inst := plugin.NewInstaller(workTree)
	inst.Overwrite = force
	res, err := installFromSource(inst, source)
	if err != nil {
		logger.Warn("plugin install failed", "source", source, "err", err.Error())
		return err
	}
	logger.Info("plugin install finished", "name", res.Name, "path", res.Path, "version", res.Version)
	out := cmdWriter(cmd, d)
	_, _ = fmt.Fprintf(out, "[got] installed plugin %s (from %s) at %s\n", res.Name, res.Source, res.Path)
	return nil
}

// installFromSource dispatches on whether the source looks like a
// local path or a git URL. It is a small wrapper so runPluginInstall
// stays readable and so tests can target either branch directly.
func installFromSource(inst *plugin.Installer, source string) (plugin.InstallResult, error) {
	if pluginSourceIsGitURL(source) {
		return inst.InstallFromGit(source)
	}
	return inst.InstallFromPath(source)
}

// pluginSourceIsGitURL mirrors plugin.LooksLikeGitURL for the CLI's
// dispatch needs. We deliberately duplicate the check here (rather
// than exporting it) so the dispatch stays snappy and so the rule is
// a single source of truth in the plugin package; the package's own
// InstallFromGit does the final say.
func pluginSourceIsGitURL(s string) bool {
	return plugin.LooksLikeGitURL(s)
}

// newPluginEnableCmd builds `got plugin enable <name>`. The name
// must be discoverable (it has to exist as either a PATH binary or
// a .got/plugins/ binary) so the registry doesn't end up with
// dangling entries that can never be invoked.
func newPluginEnableCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable <name>",
		Short: "Enable a discovered plugin in got.yml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginEnable(cmd.Context(), cmd, d, args[0])
		},
	}
	return cmd
}

func runPluginEnable(ctx context.Context, cmd *cobra.Command, d Deps, name string) error {
	logger := pluginLogger(d)
	logger.Info("plugin enable starting", "name", name)
	plugins, err := d.DiscoverPlugins(ctx)
	if err != nil {
		return err
	}
	if !pluginNameDiscovered(plugins, name) {
		logger.Warn("plugin enable failed: not discovered", "name", name)
		return gerr.Validation(fmt.Sprintf("plugin %q is not discovered (install it first, or check $PATH / .got/plugins/)", name))
	}
	workTree, err := d.Discover(".")
	if err != nil {
		return err
	}
	inst := plugin.NewInstaller(workTree)
	if _, err := inst.Enable(name); err != nil {
		logger.Warn("plugin enable failed", "name", name, "err", err.Error())
		return err
	}
	logger.Info("plugin enable finished", "name", name)
	out := cmdWriter(cmd, d)
	_, _ = fmt.Fprintf(out, "[got] enabled plugin %s\n", name)
	return nil
}

// newPluginDisableCmd builds `got plugin disable <name>`. Disabling
// a plugin that is not currently enabled is a no-op (idempotent).
func newPluginDisableCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable <name>",
		Short: "Disable a plugin in got.yml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginDisable(cmd.Context(), cmd, d, args[0])
		},
	}
	return cmd
}

func runPluginDisable(ctx context.Context, cmd *cobra.Command, d Deps, name string) error {
	logger := pluginLogger(d)
	logger.Info("plugin disable starting", "name", name)
	workTree, err := d.Discover(".")
	if err != nil {
		return err
	}
	inst := plugin.NewInstaller(workTree)
	if _, err := inst.Disable(name); err != nil {
		logger.Warn("plugin disable failed", "name", name, "err", err.Error())
		return err
	}
	logger.Info("plugin disable finished", "name", name)
	out := cmdWriter(cmd, d)
	_, _ = fmt.Fprintf(out, "[got] disabled plugin %s\n", name)
	return nil
}

// newPluginSearchCmd is a friendly stub for `got plugin search`.
// The spec's plugin registry lands in v0.2+; for v0.1 we register
// the subcommand so `got plugin --help` advertises it but the body
// tells the user to come back later.
func newPluginSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search the plugin registry (not yet implemented in v0.1)",
		Args:  cobra.ExactArgs(1),
		RunE: func(*cobra.Command, []string) error {
			return gerr.Validation("`got plugin search` is not yet implemented in v0.1; see got-spec.md §11 (planned for v0.2+)")
		},
	}
}

// registerPluginCommands adds each discovered plugin as a
// `got <plugin-name> <command>` subcommand of root. v0.1 only adds
// stub subcommands that print a "not yet implemented" message —
// live invocation lands in v0.5. Returns the number of parent
// commands added so tests can assert on it.
//
// The function is best-effort: a discovery error is swallowed (the
// plugin subcommand tree + `got plugin list` are the authoritative
// diagnostic surfaces).
func registerPluginCommands(root *cobra.Command, d Deps) int {
	if d.DiscoverPlugins == nil {
		return 0
	}
	plugins, err := d.DiscoverPlugins(context.Background())
	if err != nil {
		return 0
	}
	added := 0
	for _, p := range plugins {
		parent := &cobra.Command{
			Use:   p.Name,
			Short: fmt.Sprintf("Commands from the %s plugin", p.Name),
		}
		// Even when the manifest has no commands, we still register
		// the parent so `got <name> --help` is discoverable and
		// says "no commands" instead of failing with "unknown
		// command".
		for _, c := range p.Commands {
			c := c // capture for closure
			parent.AddCommand(&cobra.Command{
				Use:   c.Name,
				Short: c.Description,
				RunE: func(*cobra.Command, []string) error {
					return gerr.Validation("plugin invocation is not yet implemented in v0.1; see got-spec.md §11 (planned for v0.5)")
				},
			})
		}
		root.AddCommand(parent)
		added++
	}
	return added
}

// loadEnabledSet reads got.yml's plugins.enabled list as a set. A
// missing or unparseable got.yml is treated as the empty set so
// `got plugin list` keeps working in fresh repos.
func loadEnabledSet() (map[string]bool, error) {
	workTree, err := repo.Discover(".")
	if err != nil {
		// Not in a git repo: discovery itself fails; let the
		// upstream caller surface that error.
		return map[string]bool{}, err
	}
	return plugin.NewInstaller(workTree).EnabledSet()
}

// pluginNameDiscovered reports whether name is in the list of
// discovered plugins (PATH + .got/plugins/ + repo plugins).
func pluginNameDiscovered(in []plugin.DiscoveredPlugin, name string) bool {
	for _, p := range in {
		if p.Name == name {
			return true
		}
	}
	return false
}
