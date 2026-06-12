package cli

import (
	"os"

	"github.com/mattn/go-isatty"
)

// defaultIsTerminal reports whether os.Stdout is a TTY. Used by the
// init command to decide between the wizard and the non-interactive
// defaults path. Wrapped in a function so tests can override Deps.
// IsTerminal.
func defaultIsTerminal() bool {
	return isatty.IsTerminal(os.Stdout.Fd())
}
