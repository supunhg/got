package cli

import (
	"io"
	"os"

	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/repo"
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
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
	}
}
