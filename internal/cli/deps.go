package cli

import (
	"io"
	"os"
	"time"

	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/repo"
	"github.com/got-sh/got/internal/store"
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
		Now:      time.Now,
		User:     osUser,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
	}
}
