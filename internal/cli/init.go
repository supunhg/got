package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/config"
	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/initwiz"
	"github.com/got-sh/got/internal/repo"
	"github.com/got-sh/got/internal/tui"
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
	// noTUI forces the non-interactive path even when stdout is a TTY.
	noTUI bool
	// noInteractive is the --no-interactive flag. v0.1 has no wizard
	// TTY check, so the flag is now equivalent to --no-tui; we keep
	// it for forward compatibility with §7's "use defaults instead of
	// prompting" semantics.
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
with all migrations applied. By default an interactive wizard walks
through the detected values, commit style, plugins, and a final
confirm screen; pass --no-tui to use defaults without prompting.

If [path] is omitted, the current directory is used.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				opts.target = args[0]
			}
			// --no-tui is a global flag; read it from the
			// persistent flags so callers can pass it before or
			// after the subcommand.
			if v, err := cmd.Flags().GetBool("no-tui"); err == nil {
				opts.noTUI = v
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
	pf.BoolVar(&opts.noInteractive, "no-interactive", false, "use defaults instead of prompting (alias for --no-tui)")

	return cmd
}

// runInit performs the full init flow: discover, ask (wizard or
// defaults), then write. It is split out from the cobra handler so
// it can be exercised from tests without going through the command
// tree.
func runInit(cmd *cobra.Command, deps Deps, opts *initOptions) error {
	// 1. Resolve target. --here is the same as no path; path arg
	//    wins over --here if both are set.
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
	// work tree, bail. The spec says `got init` is "from inside a
	// Git repo"; we don't want to init a child of a git repo
	// against the parent's config.
	if opts.target != "" && !opts.here && abs != workTree {
		return gerr.Validation(
			fmt.Sprintf("path %q resolves to work tree %q; pass the work tree root, not a subdirectory", abs, workTree),
		)
	}

	// 3. Refuse if .got/ already has the pieces we'd write, unless
	//    --force is set.
	exists, err := config.FileExists(filepath.Join(workTree, "got.yml"))
	if err != nil {
		return err
	}
	dbExists, err := config.FileExists(filepath.Join(workTree, ".got", "got.db"))
	if err != nil {
		return err
	}
	if (exists || dbExists) && !opts.force {
		return gerr.Validation(
			fmt.Sprintf("GOT is already initialized in %s (pass --force to overwrite)", workTree),
		)
	}

	// 4. Build Answers — either by running the wizard, or by
	//    composing flags + detected defaults. Both paths produce
	//    the same struct so the apply step is identical.
	detected := initwiz.Detect(workTree)
	pre := initwiz.PrePopulated{
		Name:           opts.name,
		DefaultBranch:  opts.branch,
		CommitStyle:    opts.style,
		CustomTemplate: opts.customTemplate,
	}
	answers, err := resolveAnswers(deps, opts, detected, pre)
	if err != nil {
		return err
	}

	return applyInit(deps, workTree, answers, opts.force)
}

// resolveAnswers picks the wizard or the non-interactive defaults
// path based on the flags and the TTY status. Both paths return the
// same initwiz.Answers struct.
func resolveAnswers(deps Deps, opts *initOptions, detected initwiz.Detected, pre initwiz.PrePopulated) (initwiz.Answers, error) {
	useWizard := !opts.noTUI && !opts.noInteractive
	if useWizard && deps.IsTerminal != nil && !deps.IsTerminal() {
		useWizard = false
	}
	if useWizard {
		if deps.RunWizard == nil {
			// Defensive: a stub Deps without RunWizard still works.
			useWizard = false
		}
	}
	if !useWizard {
		// Non-interactive: build answers from flags + defaults.
		a := initwiz.Defaults(detected)
		if pre.Name != "" {
			a.Name = pre.Name
		}
		if pre.DefaultBranch != "" {
			a.DefaultBranch = pre.DefaultBranch
		}
		if pre.CommitStyle != "" {
			if err := config.ValidateCommitStyle(pre.CommitStyle); err != nil {
				return initwiz.Answers{}, gerr.Validation(err.Error())
			}
			a.CommitStyle = pre.CommitStyle
		}
		if pre.CustomTemplate != "" {
			a.CustomTemplate = pre.CustomTemplate
			// Custom template without an explicit --style implies custom.
			if pre.CommitStyle == "" {
				a.CommitStyle = "custom"
			}
		}
		return a, nil
	}
	return deps.RunWizard(detected, pre, tui.NewTheme())
}

// applyInit writes the .got/ tree, configs, gitignore entry, and
// primes the SQLite meta. It is the shared tail of the wizard and
// non-interactive paths.
func applyInit(deps Deps, workTree string, a initwiz.Answers, force bool) error {
	paths := repo.NewPaths(workTree)

	// Build the project + internal configs.
	cfg := config.DefaultProjectConfig()
	cfg.Project.Name = a.Name
	cfg.Project.DefaultBranch = a.DefaultBranch
	cfg.Commits.Style = a.CommitStyle
	cfg.Commits.CustomTemplate = a.CustomTemplate
	cfg.Plugins.Enabled = a.Plugins

	gotVersion := deps.GotVersion
	if gotVersion == "" {
		gotVersion = "dev"
	}
	internalCfg := config.DefaultInternalConfig(gotVersion)

	out := deps.Stdout

	// 1. Create .got/ tree.
	if err := paths.EnsureGOTDir(); err != nil {
		return err
	}
	// 2. .got/config.yaml.
	if err := config.WriteInternalConfig(paths.ConfigFile, internalCfg); err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, "writing .got/config.yaml")
	}
	// 3. got.yml.
	if err := config.WriteProjectConfig(filepath.Join(workTree, "got.yml"), cfg); err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, "writing got.yml")
	}
	// 4. .gitignore.
	if err := paths.EnsureGitignoreEntry(); err != nil {
		return err
	}
	// 5. SQLite store + migrations.
	if deps.StoreFor == nil {
		return gerr.Validation("internal: Deps.StoreFor is nil")
	}
	st, err := deps.StoreFor(paths.DBFile)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	// 6. Prime init meta.
	nowFn := deps.Now
	if nowFn == nil {
		nowFn = timeNow
	}
	userFn := deps.User
	if userFn == nil {
		userFn = osUser
	}
	if err := st.TouchInitMeta(gotVersion, userFn(), nowFn(), force); err != nil {
		return err
	}

	// 7. Friendly success message.
	fmt.Fprintf(out, "Initialized GOT in %s\n", workTree)
	fmt.Fprintf(out, "  - %s\n", paths.ConfigFile)
	fmt.Fprintf(out, "  - %s\n", filepath.Join(workTree, "got.yml"))
	fmt.Fprintf(out, "  - %s\n", paths.DBFile)
	fmt.Fprintln(out, "Next: try `got status`.")
	return nil
}
