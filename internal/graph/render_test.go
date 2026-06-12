package graph

import (
	"strings"
	"testing"

	"github.com/got-sh/got/internal/tui"
)

func TestParseLine_BasicCommit(t *testing.T) {
	l := parseLine("* abc1234 (HEAD -> main) First commit")
	if l.Prefix != "" {
		t.Errorf("Prefix = %q, want empty", l.Prefix)
	}
	if l.SHA != "abc1234" {
		t.Errorf("SHA = %q, want abc1234", l.SHA)
	}
	if len(l.Decos) != 1 {
		t.Fatalf("Decos = %d, want 1", len(l.Decos))
	}
	if l.Decos[0].Kind != DecorationCurrentBranch {
		t.Errorf("Kind = %v, want DecorationCurrentBranch", l.Decos[0].Kind)
	}
	if l.Subject != "First commit" {
		t.Errorf("Subject = %q, want 'First commit'", l.Subject)
	}
}

func TestParseLine_BranchWithConnector(t *testing.T) {
	l := parseLine("| * def5678 (origin/main) Second commit")
	if l.Prefix != "| " {
		t.Errorf("Prefix = %q, want '| '", l.Prefix)
	}
	if l.SHA != "def5678" {
		t.Errorf("SHA = %q, want def5678", l.SHA)
	}
	if l.Subject != "Second commit" {
		t.Errorf("Subject = %q, want 'Second commit'", l.Subject)
	}
}

func TestParseLine_MergeConnectors(t *testing.T) {
	// Pure graph line, no commit.
	l := parseLine("|/")
	if l.SHA != "" {
		t.Errorf("SHA = %q, want empty for pure graph line", l.SHA)
	}
	if l.Prefix != "|/" {
		t.Errorf("Prefix = %q, want '|/'", l.Prefix)
	}
}

func TestParseLine_TagDecoration(t *testing.T) {
	l := parseLine("* abc1234 (tag: v1.0) Initial commit")
	if len(l.Decos) != 1 {
		t.Fatalf("Decos = %d, want 1", len(l.Decos))
	}
	if l.Decos[0].Kind != DecorationTag {
		t.Errorf("Kind = %v, want DecorationTag", l.Decos[0].Kind)
	}
	if l.Decos[0].Text != "v1.0" {
		t.Errorf("Text = %q, want v1.0", l.Decos[0].Text)
	}
}

func TestParseLine_MultipleDecorations(t *testing.T) {
	l := parseLine("* abc1234 (HEAD -> main, origin/main) Merge")
	if len(l.Decos) != 2 {
		t.Fatalf("Decos = %d, want 2", len(l.Decos))
	}
	if l.Decos[0].Kind != DecorationCurrentBranch {
		t.Errorf("Decos[0].Kind = %v, want DecorationCurrentBranch", l.Decos[0].Kind)
	}
	if l.Decos[1].Kind != DecorationBranchRemote {
		t.Errorf("Decos[1].Kind = %v, want DecorationBranchRemote", l.Decos[1].Kind)
	}
}

func TestParseLine_LocalBranch(t *testing.T) {
	// Use a branch name without '/' since the parser's heuristic
	// classifies anything with '/' as remote (matching git's own
	// default colour rules). Local branches CAN contain '/', but
	// the common case (main, develop, feature-x) doesn't.
	l := parseLine("* abc1234 (main) Add feature")
	if len(l.Decos) != 1 {
		t.Fatalf("Decos = %d, want 1", len(l.Decos))
	}
	if l.Decos[0].Kind != DecorationBranchLocal {
		t.Errorf("Kind = %v, want DecorationBranchLocal", l.Decos[0].Kind)
	}
}

func TestClassify_HeadOnly(t *testing.T) {
	d := classify("HEAD")
	if d.Kind != DecorationHEAD {
		t.Errorf("Kind = %v, want DecorationHEAD", d.Kind)
	}
}

func TestRender_NoColorPreservesText(t *testing.T) {
	in := "* abc1234 (HEAD -> main) First commit"
	out := Render(in, tui.NoColorTheme())
	// No-color theme must keep the SHA and subject visible verbatim
	// (no ANSI escape codes).
	if !strings.Contains(out, "abc1234") {
		t.Errorf("output missing SHA:\n%s", out)
	}
	if !strings.Contains(out, "First commit") {
		t.Errorf("output missing subject:\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("output has ANSI escapes despite no-color theme:\n%s", out)
	}
}

// TestRender_ColorThemeProducesStyledText is the
// environment-independent counterpart to the "with color" check:
// the colored theme must keep the SHA and subject visible (the
// ANSI codes are present, but a bare `strings.Contains` for `\x1b`
// would be flaky in non-TTY test runners where lipgloss strips
// colors — see lipgloss.NewRenderer.SetColorProfile).
func TestRender_ColorThemeProducesStyledText(t *testing.T) {
	in := "* abc1234 (HEAD -> main) First commit"
	out := Render(in, tui.NewTheme())
	if !strings.Contains(out, "abc1234") {
		t.Errorf("output missing SHA:\n%s", out)
	}
	if !strings.Contains(out, "First commit") {
		t.Errorf("output missing subject:\n%s", out)
	}
}

func TestRender_HandlesEmptyLines(t *testing.T) {
	in := "* abc1234 (HEAD -> main) First\n\n* def5678 Second"
	out := Render(in, tui.NoColorTheme())
	if !strings.Contains(out, "abc1234") {
		t.Errorf("missing abc1234:\n%s", out)
	}
	if !strings.Contains(out, "def5678") {
		t.Errorf("missing def5678:\n%s", out)
	}
}

func TestRender_HandlesPureGraphLines(t *testing.T) {
	in := "|/\n* abc1234 (HEAD -> main) First"
	out := Render(in, tui.NoColorTheme())
	if !strings.Contains(out, "|/") {
		t.Errorf("missing graph line:\n%s", out)
	}
	if !strings.Contains(out, "abc1234") {
		t.Errorf("missing commit:\n%s", out)
	}
}
