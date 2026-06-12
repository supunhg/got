package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
)

func newRemoteCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Manage set of tracked repositories",
	}
	cmd.AddCommand(newRemoteListCmd(deps))
	cmd.AddCommand(newRemoteAddCmd())
	cmd.AddCommand(newRemoteRemoveCmd())
	cmd.AddCommand(newRemoteFetchCmd())
	return cmd
}

func newRemoteListCmd(deps Deps) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List remotes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemoteList(cmd, deps, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable JSON output")
	return cmd
}

func runRemoteList(cmd *cobra.Command, deps Deps, asJSON bool) error {
	workTree, err := deps.Discover(".")
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)
	remotes, err := a.Remotes(cmd.Context())
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if out == nil {
		out = deps.Stdout
	}
	if asJSON {
		return writeJSON(out, remotes)
	}
	return writeRemoteTable(out, remotes)
}

func writeRemoteTable(w io.Writer, remotes []git.Remote) error {
	if len(remotes) == 0 {
		fmt.Fprintln(w, "(no remotes)")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tFETCH URL\tPUSH URL")
	for _, r := range remotes {
		fetch := r.FetchURL
		if r.FetchSpec != "" {
			fetch = r.FetchSpec
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Name, fetch, r.PushURL)
	}
	return tw.Flush()
}

func newRemoteAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name> <url>",
		Short: "Add a remote",
		Args:  cobra.ExactArgs(2),
		RunE: func(*cobra.Command, []string) error {
			return gerr.Validation("`got remote add` is not yet implemented in v0.1; use `git remote add` for now")
		},
	}
}

func newRemoteRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a remote",
		Args:  cobra.ExactArgs(1),
		RunE: func(*cobra.Command, []string) error {
			return gerr.Validation("`got remote remove` is not yet implemented in v0.1; use `git remote remove` for now")
		},
	}
}

func newRemoteFetchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fetch [name]",
		Short: "Fetch from one or all remotes",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(*cobra.Command, []string) error {
			return gerr.Validation("`got remote fetch` is not yet implemented in v0.1; use `git fetch` for now")
		},
	}
}
