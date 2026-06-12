package commitwiz

import (
	"strings"
	"testing"
)

func TestSuggest_NoFiles(t *testing.T) {
	got := NewHeuristicSuggester().Suggest(nil)
	if got.Type != "feat" {
		t.Errorf("Type = %q, want feat (default)", got.Type)
	}
	if got.Confidence < 0.2 || got.Confidence > 0.5 {
		t.Errorf("Confidence = %v, want low default (0.2..0.5)", got.Confidence)
	}
	if !strings.Contains(got.Reason, "no files") {
		t.Errorf("Reason = %q, want it to mention 'no files'", got.Reason)
	}
}

func TestSuggest_AllCLIFiles(t *testing.T) {
	got := NewHeuristicSuggester().Suggest([]string{
		"internal/cli/init.go",
		"internal/cli/status.go",
		"internal/cli/commit.go",
		"internal/cli/deps.go",
	})
	if got.Scope != "cli" {
		t.Errorf("Scope = %q, want cli", got.Scope)
	}
	if got.Confidence < 0.8 {
		t.Errorf("Confidence = %v, want >= 0.8 (unanimous scope+type)", got.Confidence)
	}
	if !strings.Contains(got.Reason, "cli") {
		t.Errorf("Reason = %q, want it to mention 'cli'", got.Reason)
	}
}

func TestSuggest_AllTests_PicksTestType(t *testing.T) {
	got := NewHeuristicSuggester().Suggest([]string{
		"internal/cli/init_test.go",
		"internal/store/store_test.go",
	})
	if got.Type != "test" {
		t.Errorf("Type = %q, want test (all files are tests)", got.Type)
	}
	if got.Confidence < 0.8 {
		t.Errorf("Confidence = %v, want >= 0.8 (unanimous test type)", got.Confidence)
	}
	// The test rule beats the package rule, so each test file
	// matches "test" scope, which is unanimous.
	if got.Scope != "test" {
		t.Errorf("Scope = %q, want test (test files match the test rule first)", got.Scope)
	}
}

func TestSuggest_AllDocs_PicksDocsType(t *testing.T) {
	got := NewHeuristicSuggester().Suggest([]string{
		"docs/architecture.md",
		"README.md",
	})
	if got.Type != "docs" {
		t.Errorf("Type = %q, want docs", got.Type)
	}
	if got.Scope != "docs" {
		t.Errorf("Scope = %q, want docs", got.Scope)
	}
}

func TestSuggest_AllBuildFiles_PicksBuildType(t *testing.T) {
	got := NewHeuristicSuggester().Suggest([]string{
		"go.mod",
		"go.sum",
		"Makefile",
		".github/workflows/ci.yml",
	})
	// .github/ matches the "ci" scope rule; Makefile/go.mod match
	// "build". The dominant scope is whichever has more matches.
	// We accept either, but the type should be "build" since
	// every file is in the build/ci categories.
	if got.Type != "build" {
		t.Errorf("Type = %q, want build", got.Type)
	}
	if got.Scope != "build" && got.Scope != "ci" {
		t.Errorf("Scope = %q, want build or ci", got.Scope)
	}
}

func TestSuggest_ManyCodeFiles_PicksRefactor(t *testing.T) {
	paths := []string{
		"internal/cli/a.go", "internal/cli/b.go", "internal/cli/c.go",
		"internal/cli/d.go", "internal/cli/e.go", "internal/cli/f.go",
	}
	got := NewHeuristicSuggester().Suggest(paths)
	if got.Type != "refactor" {
		t.Errorf("Type = %q, want refactor (>= 6 code files)", got.Type)
	}
	if got.Scope != "cli" {
		t.Errorf("Scope = %q, want cli", got.Scope)
	}
}

func TestSuggest_MixedScopes_HasLowerConfidence(t *testing.T) {
	got := NewHeuristicSuggester().Suggest([]string{
		"internal/cli/init.go",
		"internal/git/adapter.go",
		"internal/store/store.go",
	})
	if got.Scope == "" {
		t.Errorf("Scope is empty; expected a dominant scope")
	}
	if got.Confidence >= 0.7 {
		t.Errorf("Confidence = %v, want < 0.7 (mixed scopes)", got.Confidence)
	}
	if !strings.Contains(got.Reason, "also touched") {
		t.Errorf("Reason = %q, want it to mention 'also touched'", got.Reason)
	}
}

func TestSuggest_DefaultsToFeatForUnknown(t *testing.T) {
	got := NewHeuristicSuggester().Suggest([]string{
		"some/random/path/foo.go",
		"another/random/bar.go",
	})
	if got.Type != "feat" {
		t.Errorf("Type = %q, want feat (no specific type signals)", got.Type)
	}
}

func TestSuggest_RemovesLeadingDotSlashSlash(t *testing.T) {
	// `git status --porcelain` may produce paths with "./" prefixes.
	// The suggester should treat "./internal/cli/x.go" the same as
	// "internal/cli/x.go".
	got := NewHeuristicSuggester().Suggest([]string{"./internal/cli/init.go"})
	if got.Scope != "cli" {
		t.Errorf("Scope = %q, want cli (leading ./ should be stripped)", got.Scope)
	}
}

func TestSuggest_ConfidenceIsClampedTo01(t *testing.T) {
	// Sanity: the Confidence value should always be in [0, 1] for
	// any input. We don't expect this to trip on the happy path,
	// but the clamp is a one-liner; if it ever goes wrong we want
	// the test to scream.
	for _, paths := range [][]string{
		nil,
		{"a.go"},
		{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go"},
	} {
		got := NewHeuristicSuggester().Suggest(paths)
		if got.Confidence < 0 || got.Confidence > 1 {
			t.Errorf("Confidence = %v out of [0,1] for paths=%v", got.Confidence, paths)
		}
	}
}

func TestSuggesterFunc(t *testing.T) {
	fn := SuggesterFunc(func(staged []string) Suggestion {
		return Suggestion{Type: "fix", Scope: "x", Confidence: 0.5, Reason: "stub"}
	})
	got := fn.Suggest([]string{"a.go"})
	if got.Type != "fix" || got.Scope != "x" || got.Confidence != 0.5 {
		t.Errorf("SuggesterFunc.Suggest = %+v", got)
	}
}
