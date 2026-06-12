package graph

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/got-sh/got/internal/git"
)

// RenderDOTAdapter is the minimal adapter surface RenderDOT needs.
// It is satisfied by *git.ExecAdapter and *git.FakeAdapter (both
// already implement git.Adapter). Pulling it out as an interface
// keeps the test surface narrow.
type RenderDOTAdapter interface {
	GraphDOT(ctx context.Context, opts git.GraphOpts) (string, error)
}

// RenderDOT fetches the DOT representation of the commit graph and
// writes it to w. Returns the error from the adapter or the writer.
func RenderDOT(ctx context.Context, a RenderDOTAdapter, opts git.GraphOpts, w io.Writer) error {
	dot, err := a.GraphDOT(ctx, opts)
	if err != nil {
		return err
	}
	// Ensure the output ends with a newline so `dot` (Graphviz) is
	// happy when piped to it.
	if !strings.HasSuffix(dot, "\n") {
		dot += "\n"
	}
	_, err = io.WriteString(w, dot)
	return err
}

// dotNode describes one node in the DOT digraph, derived from a
// single line of `git log --pretty=%H %P %D` output. It is exported
// so dot_test.go can assert on the parsed structure.
type dotNode struct {
	SHA        string
	Parents    []string
	Decorators []string
}

// dotLabel returns the Graphviz record-form label for n.
func (n dotNode) dotLabel() string {
	short := n.SHA
	if len(short) > 7 {
		short = short[:7]
	}
	if len(n.Decorators) == 0 {
		return fmt.Sprintf("%q", short)
	}
	parts := make([]string, 0, len(n.Decorators))
	for _, d := range n.Decorators {
		parts = append(parts, d)
	}
	return fmt.Sprintf("{ %q | %q }", short, strings.Join(parts, "\\n"))
}

// ParseDotInput parses the raw NUL-separated `git log --pretty=%H
// %P %D` output into a slice of dotNode values. Exported for
// testing.
func ParseDotInput(raw string) []dotNode {
	var out []dotNode
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x00", 3)
		if len(parts) < 2 {
			continue
		}
		n := dotNode{SHA: parts[0]}
		if parts[1] != "" {
			n.Parents = strings.Fields(parts[1])
		}
		if len(parts) == 3 && parts[2] != "" {
			for _, d := range strings.Split(parts[2], ", ") {
				d = strings.TrimSpace(d)
				if d != "" {
					n.Decorators = append(n.Decorators, d)
				}
			}
		}
		out = append(out, n)
	}
	return out
}
