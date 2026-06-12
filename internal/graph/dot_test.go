package graph

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/got-sh/got/internal/git"
)

func TestParseDotInput_LinearHistory(t *testing.T) {
	raw := "aaaaaaa\x00\x00\n" +
		"bbbbbbb\x00aaaaaaa\x00\n" +
		"ccccccc\x00bbbbbbb\x00HEAD -> main, origin/main\n"
	nodes := ParseDotInput(raw)
	if len(nodes) != 3 {
		t.Fatalf("nodes = %d, want 3", len(nodes))
	}
	if nodes[0].SHA != "aaaaaaa" {
		t.Errorf("nodes[0].SHA = %q, want aaaaaaa", nodes[0].SHA)
	}
	if len(nodes[0].Parents) != 0 {
		t.Errorf("nodes[0].Parents = %v, want empty", nodes[0].Parents)
	}
	if len(nodes[1].Parents) != 1 || nodes[1].Parents[0] != "aaaaaaa" {
		t.Errorf("nodes[1].Parents = %v, want [aaaaaaa]", nodes[1].Parents)
	}
	if len(nodes[2].Decorators) != 2 {
		t.Errorf("nodes[2].Decorators = %v, want 2 entries", nodes[2].Decorators)
	}
}

func TestParseDotInput_MergeCommit(t *testing.T) {
	raw := "ccccccc\x00aaaaaaa bbbbbbb\x00HEAD -> main\n"
	nodes := ParseDotInput(raw)
	if len(nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(nodes))
	}
	if len(nodes[0].Parents) != 2 {
		t.Errorf("Parents = %v, want 2", nodes[0].Parents)
	}
}

func TestDotNode_dotLabel_NoDecorations(t *testing.T) {
	n := dotNode{SHA: "abcdef1234567"}
	l := n.dotLabel()
	if l != `"abcdef1"` {
		t.Errorf("label = %q, want quoted 7-char SHA", l)
	}
}

func TestDotNode_dotLabel_WithDecorations(t *testing.T) {
	n := dotNode{SHA: "abcdef1234567", Decorators: []string{"HEAD -> main", "origin/main"}}
	l := n.dotLabel()
	if !strings.HasPrefix(l, "{") || !strings.HasSuffix(l, "}") {
		t.Errorf("label = %q, want record-form { ... }", l)
	}
	if !strings.Contains(l, "HEAD -> main") {
		t.Errorf("label missing HEAD -> main: %s", l)
	}
}

// fakeAdapter is a minimal RenderDOTAdapter for testing.
type fakeAdapter struct {
	dot   string
	calls int
}

func (f *fakeAdapter) GraphDOT(_ context.Context, _ git.GraphOpts) (string, error) {
	f.calls++
	return f.dot, nil
}

func TestRenderDOT_WritesAdapterOutput(t *testing.T) {
	a := &fakeAdapter{dot: "digraph g {}\n"}
	var w strings.Builder
	if err := RenderDOT(context.Background(), a, git.GraphOpts{}, &w); err != nil {
		t.Fatalf("RenderDOT: %v", err)
	}
	if w.String() != "digraph g {}\n" {
		t.Errorf("output = %q, want 'digraph g {}\\n'", w.String())
	}
	if a.calls != 1 {
		t.Errorf("calls = %d, want 1", a.calls)
	}
}

func TestRenderDOT_EnsuresTrailingNewline(t *testing.T) {
	a := &fakeAdapter{dot: "digraph g {}"} // no trailing newline
	var w strings.Builder
	if err := RenderDOT(context.Background(), a, git.GraphOpts{}, &w); err != nil {
		t.Fatalf("RenderDOT: %v", err)
	}
	if !strings.HasSuffix(w.String(), "\n") {
		t.Errorf("output = %q, want trailing newline", w.String())
	}
}

func TestRenderDOT_AdapterErrorPropagates(t *testing.T) {
	a := &errorAdapter{err: errors.New("boom")}
	err := RenderDOT(context.Background(), a, git.GraphOpts{}, io.Discard)
	if err == nil || err.Error() != "boom" {
		t.Errorf("err = %v, want boom", err)
	}
}

type errorAdapter struct{ err error }

func (e *errorAdapter) GraphDOT(_ context.Context, _ git.GraphOpts) (string, error) {
	return "", e.err
}
