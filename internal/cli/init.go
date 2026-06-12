package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/config"
	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/repo"
)

// initOptions holds the resolved values for `got init`. Fields are
// populated by newInitCmd's flag parsing and then passed into runInit.
type initOptions struct {
	// target is the work tree directory in which to initialize. The
	// dir is expected to already be (or be inside) a Git repo.
	target string
	// here forces target to the resolved cwd. The flag is the same
	// as passing no path arg.
	here bool
	// name overrides project.name.
	name string
	// branch overrides project.default_branch.
	branch string
	// style overrides commits.style.
	style string
	// customTemplate is used when style=custom.
	customTemplate string
	// force allows overwriting an existing .got/config.yaml.
	force bool
	// noInteractive is implicit in v0.1 — there is no wizard yet.
	// We keep the flag for forward compatibility with step 4 (§24).
	noInteractive bool
}

// newInitCmd builds the `got init` subcommand.
func newInitCmd(d Deps) *cobra.Command {
	opts := &initOptions{}
	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Initialize GOT in a repository",
		Long: `Initialize GOT in a Git repository.

Creates the .got/ directory tree, writes got.yml and .got/config.yaml,
appends .got/ to .gitignore, and opens the SQLite store at .got/got.db
with all migrations applied. The interactive wizard is not implemented
in v0.1; defaults are used and may be overridden with --name, --branch,
--style, and --custom-template.

If [path] is omitted, the current directory is used.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				opts.target = args[0]
			}
			return runInit(cmd, d, opts)
		},
	}
	pf := cmd.Flags()
	pf.BoolVar(&opts.here, "here", false, "use the current directory")
	pf.StringVar(&opts.name, "name", "", "override the detected project name")
	pf.StringVar(&opts.branch, "branch", "", "override the detected default branch")
	pf.StringVar(&opts.style, "style", "", "commit style: conventional|freeform|custom")
	pf.StringVar(&opts.customTemplate, "custom-template", "", "path to a custom commit template (style=custom)")
	pf.BoolVar(&opts.force, "force", false, "overwrite an existing .got/config.yaml")
	pf.BoolVar(&opts.noInteractive, "no-interactive", true, "use defaults instead of prompting (always true in v0.1)")

	return cmd
}

// runInit performs the full init flow. It is split out from the cobra
// handler so it can be exercised from tests without going through the
// command tree.
func runInit(cmd *cobra.Command, deps Deps, opts *initOptions) error {
	out := cmd.OutOrStdout()
	if deps.Stdout != nil {
		out = deps.Stdout
	}

	// 1. Resolve target. --here is the same as no path; path arg wins
	//    over --here if both are set.
	target := opts.target
	if target == "" || opts.here {
		target = "."
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return gerr.Wrap(gerr.CodeUsage, err, fmt.Sprintf("resolving %q", target))
	}

	// 2. Discover the git repo. We require a git repo to init — the
	//    spec leaves the `git init && got init` path to the user.
	workTree, err := deps.Discover(abs)
	if err != nil {
		// If deps.Discover is the production repo.Discover, the error
		// is already a *gerr.Error with a friendly message. Pass it
		// through unchanged.
		return err
	}

	// If the user passed an explicit path that is not the resolved
	// work tree (i.e. they asked to init some other work tree), bail.
	// The spec says `got init` is "from inside a Git repo"; we don't
	// want to init a child of a git repo against the parent's config.
	if opts.target != "" && !opts.here && abs != workTree {
		return gerr.Validation(
			fmt.Sprintf("path %q resolves to work tree %q; pass the work tree root, not a subdirectory", abs, workTree),
		)
	}

	paths := repo.NewPaths(workTree)

	// 3. Refuse if .got/ already has the pieces we'd write, unless
	//    --force is set. We check got.yml and the DB file.
	exists, err := config.FileExists(filepath.Join(workTree, "got.yml"))
	if err != nil {
		return err
	}
	dbExists, err := config.FileExists(paths.DBFile)
	if err != nil {
		return err
	}
	if (exists || dbExists) && !opts.force {
		return gerr.Validation(
			fmt.Sprintf("GOT is already initialized in %s (pass --force to overwrite)", workTree),
		)
	}

	// 4. Build the project config from flags + detected values.
	cfg := config.DefaultProjectConfig()
	if opts.name != "" {
		cfg.Project.Name = opts.name
	} else {
		cfg.Project.Name = filepath.Base(workTree)
	}
	if opts.branch != "" {
		cfg.Project.DefaultBranch = opts.branch
	}
	if opts.style != "" {
		if err := config.ValidateCommitStyle(opts.style); err != nil {
			return gerr.Validation(err.Error())
		}
		cfg.Commits.Style = opts.style
	}
	if opts.customTemplate != "" {
		cfg.Commits.CustomTemplate = opts.customTemplate
	}
	if opts.style == "" && opts.customTemplate != "" {
		// User supplied --custom-template without --style=custom; default
		// the style to custom.
		cfg.Commits.Style = "custom"
	}

	// 5. Build the internal config from the GOT version. NewRootCmd
	// always fills Deps.GotVersion from version.String() when empty,
	// so this is never blank in production; the nil-guard exists
	// purely to keep runInit safe to call directly from tests.
	gotVersion := deps.GotVersion
	if gotVersion == "" {
		gotVersion = "dev"
	}
	internalCfg := config.DefaultInternalConfig(gotVersion)

	// 6. Create .got/ tree.
	if err := paths.EnsureGOTDir(); err != nil {
		return err
	}

	// 7. Write .got/config.yaml. The spec (§7 step 4) lists this
	//    before got.yml, so we follow that order.
	if err := config.WriteInternalConfig(paths.ConfigFile, internalCfg); err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, "writing .got/config.yaml")
	}

	// 8. Write got.yml.
	if err := config.WriteProjectConfig(filepath.Join(workTree, "got.yml"), cfg); err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, "writing got.yml")
	}

	// 9. Append .got/ to .gitignore (idempotent).
	if err := paths.EnsureGitignoreEntry(); err != nil {
		return err
	}

	// 10. Open the SQLite store and run migrations. The store creates
	//     the file on first open and runs all pending migrations.
	if deps.StoreFor == nil {
		return gerr.Validation("internal: Deps.StoreFor is nil")
	}
	st, err := deps.StoreFor(paths.DBFile)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	// 11. Prime init meta. --force bumps init_at; without --force we
	//     leave any pre-existing init_at in place.
	nowFn := deps.Now
	if nowFn == nil {
		nowFn = timeNow
	}
	userFn := deps.User
	if userFn == nil {
		userFn = osUser
	}
	if err := st.TouchInitMeta(gotVersion, userFn(), nowFn(), opts.force); err != nil {
		return err
	}

	// 12. Friendly success message.
	fmt.Fprintf(out, "Initialized GOT in %s\n", workTree)
	fmt.Fprintf(out, "  - %s\n", filepath.Join(workTree, "got.yml"))
	fmt.Fprintf(out, "  - %s\n", paths.ConfigFile)
	fmt.Fprintf(out, "  - %s\n", paths.DBFile)
	fmt.Fprintln(out, "Next: try `got status`.")
	return nil
}

// timeNow is the time-source used when Deps.Now is nil. It is a
// package-level variable (not a constant) so tests can override it via
// the Deps plumbing. Declared in its own file to keep init.go focused
// on the cobra command.
