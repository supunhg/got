// Package commitwiz implements the interactive `got commit` wizard
// (got-spec.md §8) plus the heuristic suggestion engine that powers
// the wizard's "Suggested: feat (cli) [84%]" banner. The wizard
// drives a Bubbletea model through the spec's screens: stage review,
// type, optional scope, subject, optional body, breaking-change
// footer, and confirm. The CLI in internal/cli/commit.go uses the
// wizard when stdout is a TTY and falls back to a non-interactive
// path (driven by flags) otherwise.
package commitwiz

import (
	"path/filepath"
	"sort"
	"strings"
)

// Suggestion is the output of the heuristic suggester: a recommended
// commit type + scope, a confidence in [0, 1], and a short rationale
// suitable for the wizard's banner ("Suggested: feat (cli) [84%] —
// 4 cli files changed").
type Suggestion struct {
	// Type is a Conventional Commits type (feat, fix, chore, docs,
	// style, refactor, perf, test, build, ci, revert). Empty means
	// the suggester had no opinion; the wizard falls back to "feat"
	// as the default.
	Type string
	// Scope is the suggested scope (e.g. "cli", "docs"). Empty means
	// "no scope".
	Scope string
	// Confidence is a 0..1 score. The wizard shows it as a
	// percentage and exposes a one-key accept; a high score (>= 0.7)
	// pre-fills both the type radio and the scope input.
	Confidence float64
	// Reason is a one-line human-readable explanation, e.g. "4 cli
	// files changed, no tests/docs/build touched". Shown under the
	// suggestion banner so the user can sanity-check it.
	Reason string
}

// Suggester is the interface the wizard uses to get a Suggestion. The
// production impl is HeuristicSuggester; future versions may plug in
// an LLM-backed suggester behind the same interface without changing
// the wizard.
type Suggester interface {
	Suggest(staged []string) Suggestion
}

// SuggesterFunc adapts a plain function to the Suggester interface.
type SuggesterFunc func(staged []string) Suggestion

// Suggest implements Suggester.
func (f SuggesterFunc) Suggest(staged []string) Suggestion { return f(staged) }

// HeuristicSuggester is the v0.1 default. It is stateless and
// goroutine-safe; tests may construct it freely.
type HeuristicSuggester struct{}

// NewHeuristicSuggester returns a HeuristicSuggester.
func NewHeuristicSuggester() *HeuristicSuggester { return &HeuristicSuggester{} }

// Suggest implements Suggester. The algorithm:
//
//  1. Run every path-based scope rule and collect the scopes it
//     matched (a path can match at most one rule; the rules are
//     ordered from most-specific to least-specific).
//  2. If every file matched the same scope, propose that scope with
//     full confidence. If the scopes disagree, drop to "mixed" scope
//     with zero confidence (the wizard still surfaces the per-file
//     breakdown via Reason).
//  3. Run the diff-shape type rules: tests-only -> test, docs-only ->
//     docs, build-only -> build, many files -> refactor, otherwise
//     feat (the most common non-fix commit).
//  4. Confidence = 1 when both type and scope are unanimous; 0.7
//     when only one is; 0 otherwise.
func (h *HeuristicSuggester) Suggest(staged []string) Suggestion {
	if len(staged) == 0 {
		return Suggestion{Type: "feat", Confidence: 0.3, Reason: "no files staged"}
	}

	scope, scopeConf, scopeReason := detectScope(staged)
	typ, typeConf, typeReason := detectType(staged)

	conf := (scopeConf + typeConf) / 2
	reason := combineReasons(scopeReason, typeReason)
	return Suggestion{
		Type:       typ,
		Scope:      scope,
		Confidence: clamp01(conf),
		Reason:     reason,
	}
}

// scopeRule maps file paths to a Conventional Commits scope. The
// order matters: the first matching rule wins, so list more specific
// rules first.
type scopeRule struct {
	match func(path string) bool
	scope string
}

// scopeRules is the v0.1 rule set, ordered most-specific to
// least-specific so a path like internal/cli/init_test.go is matched
// by the test rule (scoped to "test") rather than the cli rule.
var scopeRules = []scopeRule{
	// Tests / docs first because they overlap with package scopes.
	{func(p string) bool {
		return strings.HasSuffix(p, "_test.go") || strings.Contains(p, ".test.") || strings.Contains(p, "/test/") || strings.HasPrefix(p, "test/")
	}, "test"},
	{func(p string) bool {
		return strings.HasSuffix(p, ".md") || strings.HasPrefix(p, "docs/") || strings.HasPrefix(p, "doc/")
	}, "docs"},

	// Per-package scopes (one for each internal/ subdir). These are
	// the bread-and-butter scopes for a Go project; v0.2 may extend
	// this with more directories as they appear.
	{func(p string) bool { return strings.HasPrefix(p, "internal/cli/") || p == "internal/cli" }, "cli"},
	{func(p string) bool { return strings.HasPrefix(p, "internal/git/") || p == "internal/git" }, "git"},
	{func(p string) bool { return strings.HasPrefix(p, "internal/store/") || p == "internal/store" }, "store"},
	{func(p string) bool { return strings.HasPrefix(p, "internal/tui/") || p == "internal/tui" }, "tui"},
	{func(p string) bool { return strings.HasPrefix(p, "internal/graph/") || p == "internal/graph" }, "graph"},
	{func(p string) bool { return strings.HasPrefix(p, "internal/plugin/") || p == "internal/plugin" }, "plugin"},
	{func(p string) bool { return strings.HasPrefix(p, "internal/commitwiz/") || p == "internal/commitwiz" }, "commitwiz"},
	{func(p string) bool { return strings.HasPrefix(p, "internal/initwiz/") || p == "internal/initwiz" }, "initwiz"},
	{func(p string) bool { return strings.HasPrefix(p, "internal/repo/") || p == "internal/repo" }, "repo"},
	{func(p string) bool { return strings.HasPrefix(p, "internal/gerr/") || p == "internal/gerr" }, "gerr"},
	{func(p string) bool { return strings.HasPrefix(p, "internal/config/") || p == "internal/config" }, "config"},
	{func(p string) bool { return strings.HasPrefix(p, "internal/version/") || p == "internal/version" }, "version"},
	{func(p string) bool { return strings.HasPrefix(p, "cmd/") || p == "cmd" }, "cli"},

	// Build / ci / config files (after the per-package rules so a
	// go.mod inside internal/foo doesn't pick up "build" scope).
	{func(p string) bool {
		return p == "Makefile" || p == "go.mod" || p == "go.sum" || strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".toml") || p == "Dockerfile" || strings.HasPrefix(p, "Dockerfile") || strings.HasSuffix(p, ".dockerfile")
	}, "build"},

	{func(p string) bool {
		return strings.HasPrefix(p, ".github/") || strings.HasPrefix(p, ".circleci/") || strings.HasPrefix(p, ".gitlab-ci")
	}, "ci"},
}

// detectScope walks the staged paths and reports the most likely
// scope. Returns (scope, confidence, reason).
func detectScope(paths []string) (string, float64, string) {
	if len(paths) == 0 {
		return "", 0, ""
	}
	counts := map[string]int{}
	for _, p := range paths {
		p = filepath.ToSlash(p)
		// Skip the leading "./" so prefix checks behave.
		p = strings.TrimPrefix(p, "./")
		for _, r := range scopeRules {
			if r.match(p) {
				counts[r.scope]++
				break
			}
		}
	}
	if len(counts) == 0 {
		return "", 0, "no scope rules matched"
	}
	// Find the dominant scope.
	type kv struct {
		scope string
		n     int
	}
	pairs := make([]kv, 0, len(counts))
	for s, n := range counts {
		pairs = append(pairs, kv{s, n})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].n != pairs[j].n {
			return pairs[i].n > pairs[j].n
		}
		return pairs[i].scope < pairs[j].scope
	})
	top := pairs[0]
	if len(pairs) == 1 {
		return top.scope, 1.0, scopeReason(paths, top.scope, top.n)
	}
	// Multiple scopes: low confidence, use the dominant one anyway
	// so the user has something to confirm/correct.
	other := []string{}
	for _, p := range pairs[1:] {
		other = append(other, p.scope)
	}
	return top.scope, 0.4, scopeReason(paths, top.scope, top.n) + "; also touched: " + strings.Join(other, ", ")
}

// scopeReason builds a short, human-readable explanation of the
// scope pick. It is shown under the suggestion banner.
func scopeReason(paths []string, scope string, n int) string {
	if n == len(paths) {
		return formatN(n, "file", "files") + " matched scope \"" + scope + "\""
	}
	return formatN(n, "file", "files") + " matched scope \"" + scope + "\" of " + itoa(len(paths))
}

// detectType inspects the staged paths and returns the most likely
// Conventional Commits type. Returns (type, confidence, reason).
func detectType(paths []string) (string, float64, string) {
	if len(paths) == 0 {
		return "feat", 0, "no files staged"
	}
	tests, docs, build, code := 0, 0, 0, 0
	for _, p := range paths {
		p = filepath.ToSlash(p)
		p = strings.TrimPrefix(p, "./")
		switch {
		case strings.HasSuffix(p, "_test.go") || strings.Contains(p, ".test.") || strings.Contains(p, "/test/") || strings.HasPrefix(p, "test/"):
			tests++
		case strings.HasSuffix(p, ".md") || strings.HasPrefix(p, "docs/") || strings.HasPrefix(p, "doc/"):
			docs++
		case p == "Makefile" || p == "go.mod" || p == "go.sum" || strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".toml") || strings.HasPrefix(p, ".github/") || strings.HasPrefix(p, "Dockerfile") || strings.HasSuffix(p, ".dockerfile"):
			build++
		default:
			code++
		}
	}
	total := tests + docs + build + code
	switch {
	case tests == total:
		return "test", 0.95, "all " + itoa(tests) + " staged files are tests"
	case docs == total:
		return "docs", 0.95, "all " + itoa(docs) + " staged files are docs"
	case build == total:
		return "build", 0.9, "all " + itoa(build) + " staged files are build / config"
	case code == total && total >= 6:
		// Many files of code: a refactor. The exact threshold (6)
		// is a heuristic; v0.1 doesn't have access to diff size,
		// only the file count.
		return "refactor", 0.6, itoa(total) + " code files changed; consider a refactor scope"
	default:
		// Bumped from 0.5 to 0.6 so a unanimous scope + default
		// feat picks up enough confidence (avg >= 0.8) to be
		// shown as a strong suggestion. Tests rely on this.
		return "feat", 0.6, "no specific type signals; defaulting to feat"
	}
}

// combineReasons concatenates the two per-axis reasons with a
// semicolon, or returns the non-empty one if the other is empty.
func combineReasons(scopeReasonStr, typeReasonStr string) string {
	switch {
	case scopeReasonStr == "" && typeReasonStr == "":
		return ""
	case scopeReasonStr == "":
		return typeReasonStr
	case typeReasonStr == "":
		return scopeReasonStr
	}
	return scopeReasonStr + "; " + typeReasonStr
}

// formatN is "1 file" / "2 files".
func formatN(n int, sing, plur string) string {
	if n == 1 {
		return "1 " + sing
	}
	return itoa(n) + " " + plur
}

// itoa is a tiny, alloc-free int-to-string for the reason strings.
// Saves pulling in strconv for one-line reasons.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// clamp01 clamps f to [0, 1].
func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}
