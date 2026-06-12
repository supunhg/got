package cli

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/got-sh/got/internal/branchwiz"
	"github.com/got-sh/got/internal/commitwiz"
	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/graphwiz"
	"github.com/got-sh/got/internal/initwiz"
	"github.com/got-sh/got/internal/plugin"
	"github.com/got-sh/got/internal/repo"
	"github.com/got-sh/got/internal/store"
	"github.com/got-sh/got/internal/tui"
	"github.com/got-sh/got/internal/version"
)

// Deps bundles the runtime dependencies the CLI needs. Tests pass a
// custom Deps with a fake adapter and an in-memory discoverer; production
// uses defaultDeps().
type Deps struct {
	// AdapterFor returns a git.Adapter for the given work tree. It is
	// called once per command invocation so it can allocate fresh
	// state per command if needed.
	AdapterFor func(workTree string) git.Adapter
	// Discover returns the work tree root for the given start directory.
	Discover func(start string) (string, error)
	// StoreFor returns an open store.Store for the given .got/ DB path.
	// It is responsible for running migrations on Open.
	StoreFor func(dbPath string) (*store.Store, error)
	// RunWizard starts the interactive init wizard and blocks until
	// the user confirms or cancels. Tests stub this to return canned
	// Answers without a real terminal.
	RunWizard func(detected initwiz.Detected, pre initwiz.PrePopulated, theme tui.Theme) (initwiz.Answers, error)
	// RunCommitWizard starts the interactive commit wizard and blocks
	// until the user confirms or cancels. Tests stub this to return
	// canned Answers without a real terminal.
	RunCommitWizard func(staged []string, pre commitwiz.PrePopulated) (commitwiz.Answers, error)
	// RunBranchWizard starts the interactive branch wizard and blocks
	// until the user confirms or cancels. Tests stub this to return
	// canned Answers without a real terminal.
	RunBranchWizard func(branches []git.Branch, pre branchwiz.PrePopulated, theme tui.Theme) (branchwiz.Answers, error)
	// RunGraphWizard starts the interactive commit-graph pager and
	// blocks until the user quits. Tests stub this to skip the real
	// Bubbletea program; production delegates to graphwiz.Run.
	RunGraphWizard func(ctx context.Context, content string, theme tui.Theme) error
	// DiscoverPlugins runs the plugin discovery pipeline (spec §11)
	// and returns the list of valid plugins. Tests stub this to
	// return canned results without scanning $PATH or .got/plugins/.
	// When nil, auto-registration of `got <name> <command>` is
	// skipped entirely (so tests that don't set it don't pay the
	// discovery cost).
	DiscoverPlugins func(ctx context.Context) ([]plugin.DiscoveredPlugin, error)
	// IsTerminal reports whether stdout is a TTY. When false, the
	// init command skips the wizard and uses defaults from flags.
	IsTerminal func() bool
	// Now returns the current time. Tests override this so they can
	// assert on timestamps without sleeping.
	Now func() time.Time
	// User returns the username to record in init_user. Tests may
	// override to avoid leaking the developer's $USER into snapshots.
	User func() string
	// GotVersion is the GOT version stamped into meta and into
	// .got/config.yaml. It is filled in by defaultDeps() from
	// internal/version, but tests may stub it.
	GotVersion string
	// Stdout and Stderr are where commands write their output.
	Stdout io.Writer
	Stderr io.Writer
}

// defaultDeps returns the production Deps: a real ExecAdapter factory
// and the real repo.Discover.
func defaultDeps() Deps {
	return Deps{
		AdapterFor: func(workTree string) git.Adapter {
			return git.NewExecAdapter(workTree)
		},
		Discover: repo.Discover,
		StoreFor: store.Open,
		RunWizard: func(d initwiz.Detected, pre initwiz.PrePopulated, theme tui.Theme) (initwiz.Answers, error) {
			return initwiz.Run(d, pre, theme)
		},
		RunCommitWizard: func(staged []string, pre commitwiz.PrePopulated) (commitwiz.Answers, error) {
			return commitwiz.Run(staged, pre, commitwiz.NewHeuristicSuggester(), tui.NewTheme())
		},
		RunBranchWizard: func(branches []git.Branch, pre branchwiz.PrePopulated, theme tui.Theme) (branchwiz.Answers, error) {
			return branchwiz.Run(branches, pre, theme)
		},
		RunGraphWizard: func(ctx context.Context, content string, theme tui.Theme) error {
			return graphwiz.Run(ctx, content, theme)
		},
		DiscoverPlugins: func(ctx context.Context) ([]plugin.DiscoveredPlugin, error) {
			d := &plugin.Discoverer{
				RunningVersion: version.Version,
			}
			if workTree, err := repo.Discover("."); err == nil {
				d.RepoPluginsDir = workTree + "/.got/plugins"
			}
			return d.Discover(ctx)
		},
		IsTerminal: defaultIsTerminal,
		Now:        time.Now,
		User:       osUser,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
	}
}
