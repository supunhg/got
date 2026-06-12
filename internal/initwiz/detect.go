// Package initwiz implements the interactive `got init` wizard. The
// wizard drives a Bubbletea model through the screens described in
// got-spec.md §7 (Detected values, commit style, plugins, confirm).
// It is invoked by the got init command in internal/cli when stdout
// is a TTY and --no-tui is not set; otherwise the command falls
// through to a non-interactive path that uses the same Answers
// struct.
package initwiz

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Detected bundles the auto-detected values shown on the wizard's
// first screen. The wizard uses these to pre-populate the Answers
// (the user can change any of them) and to display a "Detected"
// panel for transparency.
type Detected struct {
	// Name is the directory basename of the work tree.
	Name string
	// Branch is the current branch (from git symbolic-ref), or empty
	// if HEAD is detached.
	Branch string
	// Languages is the set of languages inferred from project files
	// (go.mod, package.json, etc.).
	Languages []string
	// Frameworks is the set of frameworks inferred from config files
	// (next.config.js, angular.json, etc.). v0.1 shows these in the
	// Detected panel but does not gate anything on them.
	Frameworks []string
}

// Detect runs the best-effort language/framework detection on root
// (the work tree). It is intentionally cheap: one os.Stat per known
// marker file, no recursion, no parsing.
func Detect(root string) Detected {
	d := Detected{Name: filepath.Base(root)}
	langs := map[string]bool{}
	frameworks := map[string]bool{}

	// Each marker is a tuple of (file or dir, language, framework).
	// The framework is "" when the marker is only a language hint.
	markers := []struct {
		path string
		lang string
		fw   string
	}{
		// Languages.
		{"go.mod", "go", ""},
		{"Cargo.toml", "rust", ""},
		{"package.json", "javascript", ""},
		{"tsconfig.json", "", "typescript"},
		{"pyproject.toml", "python", ""},
		{"requirements.txt", "python", ""},
		{"setup.py", "python", ""},
		{"Pipfile", "python", ""},
		{"Gemfile", "ruby", ""},
		{"composer.json", "php", ""},
		{"pom.xml", "java", ""},
		{"build.gradle", "java", ""},
		{"build.gradle.kts", "kotlin", ""},
		{"mix.exs", "elixir", ""},
		{"pubspec.yaml", "dart", ""},
		{"Package.swift", "swift", ""},
		{"CMakeLists.txt", "c++", ""},
		{"*.csproj", "c#", ""},
		{"go.sum", "", ""}, // counted as Go already via go.mod
		// Frameworks.
		{"next.config.js", "", "next.js"},
		{"next.config.mjs", "", "next.js"},
		{"nuxt.config.js", "", "nuxt"},
		{"nuxt.config.ts", "", "nuxt"},
		{"angular.json", "", "angular"},
		{"vite.config.js", "", "vite"},
		{"vite.config.ts", "", "vite"},
		{"svelte.config.js", "", "svelte"},
		{"vue.config.js", "", "vue"},
		{"tailwind.config.js", "", "tailwindcss"},
		{"tailwind.config.ts", "", "tailwindcss"},
		{"Dockerfile", "", "docker"},
		{"docker-compose.yml", "", "docker"},
		{".eslintrc", "", "eslint"},
		{".eslintrc.js", "", "eslint"},
		{".eslintrc.json", "", "eslint"},
		{".prettierrc", "", "prettier"},
		{".prettierrc.json", "", "prettier"},
	}

	for _, m := range markers {
		// Glob markers (e.g. *.csproj) require a directory walk; the
		// common case is a single file, so we use filepath.Glob at
		// the root. Deeper projects can be added later.
		if strings.Contains(m.path, "*") {
			matches, _ := filepath.Glob(filepath.Join(root, m.path))
			if len(matches) > 0 {
				if m.lang != "" {
					langs[m.lang] = true
				}
				if m.fw != "" {
					frameworks[m.fw] = true
				}
			}
			continue
		}
		if _, err := os.Stat(filepath.Join(root, m.path)); err == nil {
			if m.lang != "" {
				langs[m.lang] = true
			}
			if m.fw != "" {
				frameworks[m.fw] = true
			}
		}
	}

	d.Languages = keysSorted(langs)
	d.Frameworks = keysSorted(frameworks)
	return d
}

// keysSorted returns the keys of m as a sorted slice. Used to make
// Detect's output deterministic.
func keysSorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		if m[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
