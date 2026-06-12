package cli

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/plugin"
	"github.com/got-sh/got/internal/repo"
)

// newPluginCmd builds the `got plugin` subcommand tree per spec §11.
//
//	got plugin list           list discovered plugins (table or --json)
//	got plugin info <name>    show a plugin's full manifest
//	got plugin install <src>  install from local path or git URL [v0.1: stubbed]
//
// In addition, any plugins found via discovery are registered as
// `got <plugin-name> <command>` subcommands under root. v0.1 ships
// zero plugins so the registered subcommands return a clear "not
// yet implemented" message; live invocation lands in v0.5.
func newPluginCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage GOT plugins",
		Long: `Discover, inspect, and (eventually) install external GOT plugins.

A plugin is an executable named got-<name> on $PATH or in
.got/plugins/. When GOT starts, each candidate is invoked with
` + "`--got-plugin-manifest`" + ` and the returned JSON manifest is
parsed and validated. Commands declared in the manifest are
registered as ` + "`got <plugin-name> <command>`" + ` subcommands.

v0.1 ships zero plugins. The interface, discovery, and manifest
protocol are fully implemented so plugin authors can start building
immediately; live invocation lands in v0.5.`,
	}
	cmd.AddCommand(newPluginListCmd(d))
	cmd.AddCommand(newPluginInfoCmd(d))
	cmd.AddCommand(newPluginInstallCmd())
	return cmd
}

// newPluginListCmd builds `got plugin list [--json]`.
func newPluginListCmd(d Deps) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List discovered plugins",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPluginList(cmd.Context(), cmd, d, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable JSON output")
	return cmd
}

func runPluginList(ctx context.Context, cmd *cobra.Command, d Deps, asJSON bool) error {
	plugins, err := d.DiscoverPlugins(ctx)
	if err != nil {
		return err
	}
	out := cmdWriter(cmd, d)
	if asJSON {
		return writeJSON(out, plugins)
	}
	return writePluginTable(out, plugins)
}

func writePluginTable(w io.Writer, plugins []plugin.DiscoveredPlugin) error {
	if len(plugins) == 0 {
		_, err := fmt.Fprintln(w, "(no plugins discovered)")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tVERSION\tMIN GOT\tSOURCE\tPATH")
	for _, p := range plugins {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", p.Name, p.Version, p.MinGOT, p.Source, p.Path)
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

// newPluginInstallCmd is a stub for `got plugin install`. Per spec
// §11 the real installer lands in v0.5; v0.1 only returns a
// "not yet implemented" error so the subcommand tree stays
// discoverable.
func newPluginInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <source>",
		Short: "Install a plugin (not yet implemented in v0.1)",
		Args:  cobra.ExactArgs(1),
		RunE: func(*cobra.Command, []string) error {
			return gerr.Validation("`got plugin install` is not yet implemented in v0.1; see got-spec.md §11 (planned for v0.5)")
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

// repoPluginsDir returns the .got/plugins/ path for the current
// work tree, or "" if we're not in a git repo. Used by the
// default Deps to wire DiscoverPlugins.
func repoPluginsDir() string {
	workTree, err := repo.Discover(".")
	if err != nil {
		return ""
	}
	return workTree + "/.got/plugins"
}
