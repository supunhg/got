// Command got is a Git-native developer operating layer.
//
// See ARCHITECTURE.md for the high-level design and got-spec.md for the
// binding v0.1 specification.
package main

import (
	"fmt"
	"os"

	"github.com/got-sh/got/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "got:", err)
		os.Exit(1)
	}
}
