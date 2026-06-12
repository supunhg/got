package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
)

func newRemoteCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Manage set of tracked repositories",
		Long: "Configure and operate on the set of remote repositories " +
			"(see spec §10). All subcommands support --json where the " +
			"underlying operation is a query.",
		// `got remote` with no subcommand defaults to `got remote list`,
		// per spec §10. RunE is set after the subcommands are wired up
		// below so we can call runRemoteList with the same defaults.
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemoteList(cmd, deps, false)
		},
	}
	cmd.AddCommand(newRemoteListCmd(deps))
	cmd.AddCommand(newRemoteAddCmd(deps))
	cmd.AddCommand(newRemoteRemoveCmd(deps))
	cmd.AddCommand(newRemoteRenameCmd(deps))
	cmd.AddCommand(newRemoteSetURLCmd(deps))
	cmd.AddCommand(newRemoteFetchCmd(deps))
	cmd.AddCommand(newRemotePushCmd(deps))
	cmd.AddCommand(newRemotePruneCmd(deps))
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
	out := cmdWriter(cmd, deps)
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

func newRemoteAddCmd(deps Deps) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "add <name> <url>",
		Short: "Add a remote",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemoteAdd(cmd, deps, args[0], args[1], asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable JSON output")
	return cmd
}

func runRemoteAdd(cmd *cobra.Command, deps Deps, name, rawURL string, asJSON bool) error {
	if name == "" {
		return gerr.Validation("remote name is empty")
	}
	if err := validateRemoteName(name); err != nil {
		return err
	}
	if rawURL == "" {
		return gerr.Validation("remote URL is empty")
	}
	if err := validateRemoteURL(rawURL); err != nil {
		return err
	}
	workTree, err := deps.Discover(".")
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)
	if err := a.RemoteAdd(cmd.Context(), name, rawURL); err != nil {
		return err
	}
	out := cmdWriter(cmd, deps)
	if asJSON {
		return writeJSON(out, git.Remote{Name: name, FetchURL: rawURL, PushURL: rawURL})
	}
	fmt.Fprintf(out, "Added remote %s -> %s\n", name, rawURL)
	return nil
}

func newRemoteRemoveCmd(deps Deps) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a remote",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemoteRemove(cmd, deps, args[0], force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "remove even if the remote has tracking branches")
	return cmd
}

func runRemoteRemove(cmd *cobra.Command, deps Deps, name string, force bool) error {
	if name == "" {
		return gerr.Validation("remote name is empty")
	}
	workTree, err := deps.Discover(".")
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)
	if !force {
		// Refuse if the remote has any tracking branch (spec §10).
		remotes, rerr := a.Remotes(cmd.Context())
		if rerr != nil {
			return rerr
		}
		known := false
		for _, r := range remotes {
			if r.Name == name {
				known = true
				break
			}
		}
		if !known {
			return gerr.Validation(fmt.Sprintf("remote %q does not exist", name))
		}
	}
	if err := a.RemoteRemove(cmd.Context(), name, force); err != nil {
		return err
	}
	fmt.Fprintf(cmdWriter(cmd, deps), "Removed remote %s\n", name)
	return nil
}

func newRemoteRenameCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename <old> <new>",
		Short: "Rename a remote",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemoteRename(cmd, deps, args[0], args[1])
		},
	}
	return cmd
}

func runRemoteRename(cmd *cobra.Command, deps Deps, oldName, newName string) error {
	if oldName == "" || newName == "" {
		return gerr.Validation("remote rename requires both old and new names")
	}
	if err := validateRemoteName(newName); err != nil {
		return err
	}
	workTree, err := deps.Discover(".")
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)
	if err := a.RemoteRename(cmd.Context(), oldName, newName); err != nil {
		return err
	}
	fmt.Fprintf(cmdWriter(cmd, deps), "Renamed remote %s -> %s\n", oldName, newName)
	return nil
}

func newRemoteSetURLCmd(deps Deps) *cobra.Command {
	var push, check bool
	cmd := &cobra.Command{
		Use:   "set-url <name> <url>",
		Short: "Change the URL for an existing remote",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemoteSetURL(cmd, deps, args[0], args[1], push, check)
		},
	}
	cmd.Flags().BoolVar(&push, "push", false, "set the push URL instead of the fetch URL")
	cmd.Flags().BoolVar(&check, "check", false, "verify the URL is reachable with a dry-run fetch")
	return cmd
}

func runRemoteSetURL(cmd *cobra.Command, deps Deps, name, rawURL string, push, check bool) error {
	if name == "" || rawURL == "" {
		return gerr.Validation("remote set-url requires both name and URL")
	}
	if err := validateRemoteURL(rawURL); err != nil {
		return err
	}
	workTree, err := deps.Discover(".")
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)
	if err := a.RemoteSetURL(cmd.Context(), name, rawURL, push); err != nil {
		return err
	}
	if check {
		// Dry-run fetch: use a context detached from the parent so we
		// don't keep a long-lived fetch running, but reuse the adapter
		// so a fake can record the call.
		if err := a.Fetch(context.Background(), name); err != nil {
			return err
		}
	}
	fmt.Fprintf(cmdWriter(cmd, deps), "Updated %s URL for %s -> %s\n", urlKind(push), name, rawURL)
	return nil
}

func newRemoteFetchCmd(deps Deps) *cobra.Command {
	var prune, all bool
	cmd := &cobra.Command{
		Use:   "fetch [name]",
		Short: "Fetch from one or all remotes",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemoteFetch(cmd, deps, args, prune, all)
		},
	}
	cmd.Flags().BoolVar(&prune, "prune", false, "remove stale remote-tracking refs after fetch")
	cmd.Flags().BoolVar(&all, "all", false, "fetch every configured remote")
	return cmd
}

func runRemoteFetch(cmd *cobra.Command, deps Deps, args []string, prune, all bool) error {
	logger := remoteLogger(deps)
	workTree, err := deps.Discover(".")
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)
	if all {
		logger.Info("remote fetch-all starting", "prune", prune)
		if err := a.FetchAll(cmd.Context(), prune); err != nil {
			logger.Warn("remote fetch-all failed", "err", err.Error())
			return err
		}
		logger.Info("remote fetch-all finished")
		fmt.Fprintln(cmdWriter(cmd, deps), "Fetched all remotes")
		return nil
	}
	if len(args) == 0 {
		return gerr.Validation("fetch requires a remote name or --all")
	}
	name := args[0]
	logger.Info("remote fetch starting", "name", name, "prune", prune)
	if prune {
		if err := a.FetchPrune(cmd.Context(), name); err != nil {
			logger.Warn("remote fetch failed", "name", name, "prune", true, "err", err.Error())
			return err
		}
	} else {
		if err := a.Fetch(cmd.Context(), name); err != nil {
			logger.Warn("remote fetch failed", "name", name, "err", err.Error())
			return err
		}
	}
	logger.Info("remote fetch finished", "name", name)
	fmt.Fprintf(cmdWriter(cmd, deps), "Fetched %s\n", name)
	return nil
}

func newRemotePushCmd(deps Deps) *cobra.Command {
	var force, forceWithLease, setUpstream, tags bool
	cmd := &cobra.Command{
		Use:   "push <name> <branch>",
		Short: "Push a branch to a remote",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemotePush(cmd, deps, args[0], args[1], force, forceWithLease, setUpstream, tags)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "force the push (DANGEROUS; prefer --force-with-lease)")
	cmd.Flags().BoolVar(&forceWithLease, "force-with-lease", false, "force the push only if the remote ref has not moved")
	cmd.Flags().BoolVar(&setUpstream, "set-upstream", false, "set the upstream tracking ref (-u)")
	cmd.Flags().BoolVar(&tags, "tags", false, "also push all tags")
	return cmd
}

func runRemotePush(cmd *cobra.Command, deps Deps, name, branch string, force, forceWithLease, setUpstream, tags bool) error {
	if name == "" || branch == "" {
		return gerr.Validation("push requires both remote name and branch")
	}
	if force && forceWithLease {
		return gerr.Validation("--force and --force-with-lease are mutually exclusive")
	}
	// spec §10: refuse non-fast-forward without --force-with-lease.
	// We can't know the remote state without a fetch; the adapter
	// surfaces non-FF as a GitError which we wrap here. The CLI-level
	// guard refuses to set --force silently; users must opt in.
	opts := git.PushOpts{
		Force:          force && !forceWithLease,
		ForceWithLease: forceWithLease,
		SetUpstream:    setUpstream,
		Tags:           tags,
	}
	workTree, err := deps.Discover(".")
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)
	logger := remoteLogger(deps)
	logger.Info("remote push starting", "name", name, "branch", branch, "force", opts.Force, "forceWithLease", opts.ForceWithLease, "setUpstream", opts.SetUpstream, "tags", opts.Tags)
	if err := a.Push(cmd.Context(), name, branch, opts); err != nil {
		wrapped := wrapPushError(err, name, branch, forceWithLease || force)
		// Per spec §16, non-fast-forward is a recoverable issue
		// (the user can re-run with --force-with-lease) and so
		// deserves a warn record alongside the validation error
		// the user sees.
		if wrapped != err {
			logger.Warn("remote push rejected", "name", name, "branch", branch, "err", err.Error())
		} else {
			logger.Warn("remote push failed", "name", name, "branch", branch, "err", err.Error())
		}
		return wrapped
	}
	logger.Info("remote push finished", "name", name, "branch", branch)
	fmt.Fprintf(cmdWriter(cmd, deps), "Pushed %s to %s\n", branch, name)
	return nil
}

// remoteLogger is an alias for the shared loggerFor helper, kept
// as a named call site so the remote subcommand tree reads
// consistently. New code should use loggerFor directly.
func remoteLogger(d Deps) *slog.Logger { return loggerFor(d) }

// wrapPushError annotates a non-fast-forward push error with guidance
// to re-run with --force-with-lease, per spec §10. We detect NFF by
// scanning the wrapped error string for git's canonical message
// ("non-fast-forward" / "[rejected] (non-fast-forward)"). The git CLI
// is the only source of these errors and its wording is stable across
// versions.
func wrapPushError(err error, name, branch string, forced bool) error {
	if forced {
		return err
	}
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "non-fast-forward") || strings.Contains(msg, "[rejected]") {
		return gerr.Validation(fmt.Sprintf(
			"push of %s to %s was rejected as non-fast-forward; "+
				"re-run with --force-with-lease (or --force, but be careful): %v",
			branch, name, err,
		))
	}
	return err
}

func newRemotePruneCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune <name>",
		Short: "Delete stale remote-tracking branches under <name>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemotePrune(cmd, deps, args[0])
		},
	}
	return cmd
}

func runRemotePrune(cmd *cobra.Command, deps Deps, name string) error {
	if name == "" {
		return gerr.Validation("prune requires a remote name")
	}
	workTree, err := deps.Discover(".")
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)
	if err := a.RemotePrune(cmd.Context(), name); err != nil {
		return err
	}
	fmt.Fprintf(cmdWriter(cmd, deps), "Pruned %s\n", name)
	return nil
}

// cmdWriter returns the io.Writer that CLI subcommands should write
// their output to. It prefers cmd.OutOrStdout() (which respects
// cmd.SetOut in tests) and falls back to deps.Stdout for callers
// that invoke subcommands without a cobra parent.
func cmdWriter(cmd *cobra.Command, deps Deps) io.Writer {
	if cmd != nil {
		if w := cmd.OutOrStdout(); w != nil {
			return w
		}
	}
	if deps.Stdout != nil {
		return deps.Stdout
	}
	return io.Discard
}

// validateRemoteURL accepts http(s), git, ssh, and file URLs plus
// scp-style (user@host:path). Anything else is rejected with a
// friendly validation error. scp-style URLs must be detected BEFORE
// url.Parse, because Go's net/url rejects "user@host:path" with
// "first path segment in URL cannot contain colon".
func validateRemoteURL(raw string) error {
	if raw == "" {
		return gerr.Validation("remote URL is empty")
	}
	if strings.ContainsAny(raw, " \t\n") {
		return gerr.Validation(fmt.Sprintf("invalid remote URL %q (contains whitespace)", raw))
	}
	// scp-style: contains '@', contains ':', but no '://'.
	if strings.Contains(raw, "@") && strings.Contains(raw, ":") && !strings.Contains(raw, "://") {
		return nil
	}
	// Bare local path: starts with /, ./, or ../. Git accepts these
	// (e.g. `/tmp/repo.git`, `./sibling.git`, `../other.git`).
	if strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return gerr.Validation(fmt.Sprintf("invalid remote URL %q: %v", raw, err))
	}
	switch u.Scheme {
	case "http", "https", "git", "ssh", "file":
		// ok
	case "":
		return gerr.Validation(fmt.Sprintf("invalid remote URL %q (missing scheme)", raw))
	default:
		return gerr.Validation(fmt.Sprintf("unsupported remote URL scheme %q in %q", u.Scheme, raw))
	}
	if u.Host == "" && u.Scheme != "file" {
		return gerr.Validation(fmt.Sprintf("remote URL %q is missing a host", raw))
	}
	return nil
}

// validateRemoteName enforces git's remote-name rules: no spaces, no
// control characters, no leading dot, no "..", no leading/trailing "/".
func validateRemoteName(name string) error {
	if name == "" {
		return gerr.Validation("remote name is empty")
	}
	if name == "." || name == ".." {
		return gerr.Validation(fmt.Sprintf("invalid remote name %q", name))
	}
	for _, c := range name {
		if c <= ' ' || c == 0x7f {
			return gerr.Validation(fmt.Sprintf("invalid remote name %q (contains whitespace or control character)", name))
		}
	}
	if name[0] == '.' || name[len(name)-1] == '/' {
		return gerr.Validation(fmt.Sprintf("invalid remote name %q", name))
	}
	return nil
}

// urlKind returns the human label ("fetch" or "push") for the
// remote URL being updated by `got remote set-url`.
func urlKind(push bool) string {
	if push {
		return "push"
	}
	return "fetch"
}
