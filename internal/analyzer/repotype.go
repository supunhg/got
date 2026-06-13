package analyzer

import (
	"path/filepath"
	"strings"
)

// classifyRepository decides the high-level repository type
// (application, library, tool, documentation, config, monorepo,
// unknown) and returns a one-line reason that explains the
// classification.
//
// The classifier is a rules engine with the following priority:
//
//  1. If the repo is a monorepo AND the tool is a recognized
//     monorepo orchestrator (Bazel, Nx, Turborepo, Lerna), it's
//     a monorepo.
//  2. If the repo is a documentation site (mkdocs, docusaurus,
//     hugo, jekyll, etc.) or contains only Markdown and no
//     source code, it's documentation.
//  3. If the repo is purely config / infra (terraform, ansible,
//     puppet, helm charts only, k8s manifests only), it's config.
//  4. If the repo declares a CLI entry point (cobra.Command in
//     a main package, a setup.py entry_points=console_scripts,
//     package.json "bin" field), it's a tool.
//  5. If the repo is a library (go.mod with no main package,
//     package.json with no "bin"/"main" and an "exports" or
//     "main" pointing to a module, setup.py/pyproject.toml
//     with no entry_points), it's a library.
//  6. If none of the above, it's an application (or "unknown"
//     when we have no source files at all).
func classifyRepository(dc DetectionContext, model RepositoryModel) (RepositoryType, string) {
	// Priority 1: explicit monorepo orchestrator.
	if model.Monorepo.IsMonorepo {
		switch model.Monorepo.Tool {
		case "Bazel", "Nx", "Turborepo", "Lerna", "pnpm workspaces", "npm workspaces":
			return RepoTypeMonorepo, "monorepo orchestrator: " + model.Monorepo.Tool
		}
	}

	// Priority 2: documentation.
	if isDocSite(dc) {
		return RepoTypeDocumentation, "documentation site (mkdocs/docusaurus/hugo/jekyll/sphinx)"
	}
	if isDocsOnly(dc) {
		return RepoTypeDocumentation, "only Markdown / docs files, no source code"
	}

	// Priority 3: pure config / infra.
	if isConfigOnly(dc, model) {
		return RepoTypeConfig, "infrastructure / config (terraform/ansible/k8s-only)"
	}

	// Priority 4: tool (CLI).
	if isTool(dc) {
		return RepoTypeTool, "has a CLI entry point (cobra / bin / console_scripts)"
	}

	// Priority 5: library.
	if isLibrary(dc) {
		return RepoTypeLibrary, "library (no CLI entry point, exports a module / package)"
	}

	// Priority 6: application.
	if hasAnySource(dc) {
		return RepoTypeApplication, "has source code without a clear CLI or library shape"
	}
	return RepoTypeUnknown, "no source files detected"
}

// isDocSite reports whether the work tree contains a known
// documentation site generator's config.
func isDocSite(dc DetectionContext) bool {
	docConfigs := map[string]bool{
		"mkdocs.yml":           true,
		"docusaurus.config.js": true,
		"docusaurus.config.ts": true,
		"hugo.toml":            true,
		"hugo.yaml":            true,
		"hugo.json":            true,
		"_config.yml":          true, // jekyll
		"conf.py":              true, // sphinx
		"book.toml":            true, // mdbook
		".vitepress/config.js": true,
		".vitepress/config.ts": true,
		"docs/_config.yml":     true,
	}
	for _, f := range dc.Files {
		base := filepath.Base(f)
		if docConfigs[base] {
			return true
		}
		// Sphinx conf.py anywhere in the work tree counts.
		if base == "conf.py" {
			// Heuristic: conf.py is also a common filename in
			// Python projects that aren't Sphinx sites. Look
			// for a sibling index.rst or Makefile with sphinx.
			dir := filepath.Dir(f)
			if dc.fileExists(filepath.ToSlash(filepath.Join(dir, "index.rst"))) {
				return true
			}
		}
	}
	return false
}

// isDocsOnly reports whether the work tree contains only docs
// files (Markdown, reST, AsciiDoc) and no source code.
func isDocsOnly(dc DetectionContext) bool {
	hasSource := false
	hasDocs := false
	for _, f := range dc.Files {
		ext := strings.ToLower(filepath.Ext(f))
		switch ext {
		case ".md", ".mdx", ".markdown", ".rst", ".adoc", ".asciidoc", ".txt":
			hasDocs = true
		case ".go", ".py", ".js", ".ts", ".jsx", ".tsx", ".java", ".kt", ".rb", ".php", ".rs", ".c", ".cpp", ".h", ".hpp", ".cs", ".swift", ".m", ".mm", ".scala", ".dart", ".ex", ".exs", ".hs", ".ml", ".lua", ".r", ".pl", ".sh", ".bash", ".zsh", ".ps1", ".vim", ".el":
			hasSource = true
		}
	}
	return hasDocs && !hasSource
}

// isConfigOnly reports whether the work tree is infrastructure /
// config only (terraform, ansible, kubernetes-only, helm-only).
func isConfigOnly(dc DetectionContext, model RepositoryModel) bool {
	hasSource := false
	hasInfra := false
	for _, f := range dc.Files {
		ext := strings.ToLower(filepath.Ext(f))
		base := filepath.Base(f)
		if base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile.") {
			continue // a Dockerfile alone doesn't make it config-only
		}
		switch ext {
		case ".tf", ".tfvars", ".hcl", ".nomad":
			hasInfra = true
		case ".yaml", ".yml":
			// k8s-only: many YAMLs in a k8s directory.
			dir := filepath.ToSlash(filepath.Dir(f))
			if isK8sDirectory(dir) {
				hasInfra = true
			}
		case ".go", ".py", ".js", ".ts", ".java", ".rb", ".rs", ".php":
			hasSource = true
		}
		// Ansible playbook.
		if base == "playbook.yml" || base == "playbook.yaml" || base == "ansible.cfg" {
			hasInfra = true
		}
		// Puppet.
		if base == "site.pp" {
			hasInfra = true
		}
	}
	// Helm-only: any charts at all, no source.
	if !hasSource && model.Containerization.HasHelm {
		return true
	}
	return hasInfra && !hasSource
}

// isTool reports whether the work tree contains a CLI entry point.
func isTool(dc DetectionContext) bool {
	// Go: a main package. Heuristic: there's a directory named
	// "cmd/<name>" at the work tree root, OR a "main.go" at the
	// root.
	for _, f := range dc.Files {
		base := filepath.Base(f)
		dir := filepath.ToSlash(filepath.Dir(f))
		if base == "main.go" && (dir == "." || strings.HasPrefix(dir, "cmd/")) {
			return true
		}
		// "cmd/<toolname>/main.go" pattern
		if strings.HasPrefix(dir, "cmd/") && strings.HasSuffix(dir, "/main") && base == "main.go" {
			return true
		}
	}
	// JS: package.json with a "bin" field that has a value
	// (either string or object). Tools declare a bin; libraries
	// declare an "exports" or "main" only.
	for _, f := range dc.Files {
		if filepath.Base(f) == "package.json" && filepath.Dir(f) == "." {
			data, err := readFile(dc.WorkTree, f)
			if err != nil {
				continue
			}
			var pkg struct {
				Bin any `json:"bin"`
			}
			if err := jsonUnmarshal(data, &pkg); err != nil {
				continue
			}
			if pkg.Bin != nil {
				return true
			}
		}
	}
	// Python: setup.py or pyproject.toml with [project.scripts]
	// / [tool.poetry.scripts] / console_scripts.
	for _, f := range dc.Files {
		base := filepath.Base(f)
		if base == "setup.py" {
			data, err := readFile(dc.WorkTree, f)
			if err == nil {
				body := string(data)
				if strings.Contains(body, "entry_points") && strings.Contains(body, "console_scripts") {
					return true
				}
			}
		}
		if base == "pyproject.toml" {
			data, err := readFile(dc.WorkTree, f)
			if err == nil {
				body := string(data)
				if strings.Contains(body, "[project.scripts]") || strings.Contains(body, "[tool.poetry.scripts]") {
					return true
				}
			}
		}
	}
	// Rust: a [[bin]] section in Cargo.toml.
	for _, f := range dc.Files {
		if filepath.Base(f) == "Cargo.toml" && filepath.Dir(f) == "." {
			data, err := readFile(dc.WorkTree, f)
			if err == nil {
				if strings.Contains(string(data), "[[bin]]") {
					return true
				}
			}
		}
	}
	return false
}

// isLibrary reports whether the work tree is shaped like a
// library (exports a module / package, no CLI).
func isLibrary(dc DetectionContext) bool {
	if isTool(dc) {
		return false // a tool with no entry point is contradictory
	}
	// Go: go.mod exists and there is no main package.
	for _, f := range dc.Files {
		if filepath.Base(f) == "go.mod" && filepath.Dir(f) == "." {
			return true
		}
	}
	// JS: package.json with "main" / "exports" / "module" and
	// no "bin". (No "bin" is implied by the isTool check above.)
	for _, f := range dc.Files {
		if filepath.Base(f) == "package.json" && filepath.Dir(f) == "." {
			data, err := readFile(dc.WorkTree, f)
			if err != nil {
				continue
			}
			var pkg struct {
				Main    string `json:"main"`
				Module  string `json:"module"`
				Exports any    `json:"exports"`
			}
			if err := jsonUnmarshal(data, &pkg); err != nil {
				continue
			}
			if pkg.Main != "" || pkg.Module != "" || pkg.Exports != nil {
				return true
			}
		}
	}
	// Python: setup.py / pyproject.toml with a library mode
	// (no entry_points, has packages= or [tool.setuptools.packages.find]).
	for _, f := range dc.Files {
		base := filepath.Base(f)
		if base == "setup.py" {
			data, err := readFile(dc.WorkTree, f)
			if err == nil {
				body := string(data)
				if !strings.Contains(body, "console_scripts") && strings.Contains(body, "packages=") {
					return true
				}
			}
		}
	}
	// Rust: Cargo.toml with [lib] (and not [[bin]]).
	for _, f := range dc.Files {
		if filepath.Base(f) == "Cargo.toml" && filepath.Dir(f) == "." {
			data, err := readFile(dc.WorkTree, f)
			if err == nil {
				body := string(data)
				if strings.Contains(body, "[lib]") && !strings.Contains(body, "[[bin]]") {
					return true
				}
			}
		}
	}
	// Ruby: gemspec at the root.
	for _, f := range dc.Files {
		if filepath.Ext(f) == ".gemspec" && filepath.Dir(f) == "." {
			return true
		}
	}
	return false
}

// hasAnySource reports whether the work tree has any source code
// (Go, Python, JS, TS, Java, etc.).
func hasAnySource(dc DetectionContext) bool {
	for _, f := range dc.Files {
		ext := strings.ToLower(filepath.Ext(f))
		switch ext {
		case ".go", ".py", ".js", ".ts", ".jsx", ".tsx", ".java", ".kt",
			".rb", ".php", ".rs", ".c", ".cpp", ".h", ".hpp", ".cs",
			".swift", ".m", ".mm", ".scala", ".dart", ".ex", ".exs",
			".hs", ".ml", ".lua", ".r", ".pl", ".sh", ".bash", ".zsh",
			".ps1", ".vim", ".el", ".jl", ".zig", ".nim", ".cr", ".d",
			".v", ".sv", ".vhd", ".vhdl", ".f", ".f90", ".pas", ".pp",
			".ada", ".adb":
			return true
		}
	}
	return false
}
