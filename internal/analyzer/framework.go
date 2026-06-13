package analyzer

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// detectFrameworks inspects package manifests (package.json,
// requirements.txt, go.mod, Cargo.toml, pom.xml, build.gradle,
// Gemfile, pubspec.yaml, composer.json, pyproject.toml) and
// infers frameworks and major libraries from their dependencies.
//
// Each detection has a "confidence" level:
//
//	"high"   - the framework is named explicitly in a manifest's
//	           dependencies / devDependencies / requirements.
//	"medium" - the framework is inferred from a directory layout
//	           (e.g. "app/" + "routes/" + package.json → Express).
//	"low"    - the framework is guessed from a single file
//	           (e.g. "next.config.js" → Next.js).
func detectFrameworks(dc DetectionContext) []Framework {
	seen := make(map[string]Framework)

	add := func(f Framework) {
		if f.Name == "" {
			return
		}
		if existing, ok := seen[f.Name]; ok {
			// High confidence wins over medium/low.
			if confidenceRank(existing.Confidence) < confidenceRank(f.Confidence) {
				existing.Confidence = f.Confidence
			}
			// Merge config files.
			cf := mergeStringSlices(existing.ConfigFiles, f.ConfigFiles)
			existing.ConfigFiles = cf
			// Version: first non-empty wins.
			if existing.Version == "" {
				existing.Version = f.Version
			}
			seen[f.Name] = existing
			return
		}
		seen[f.Name] = f
	}

	// JavaScript / TypeScript: package.json
	detectFromPackageJSON(dc, add)
	// Python: requirements.txt, pyproject.toml
	detectFromPythonManifests(dc, add)
	// Go: go.mod
	detectFromGoMod(dc, add)
	// Rust: Cargo.toml
	detectFromCargoToml(dc, add)
	// Ruby: Gemfile
	detectFromGemfile(dc, add)
	// PHP: composer.json
	detectFromComposerJSON(dc, add)
	// Java: pom.xml, build.gradle
	detectFromJavaManifests(dc, add)
	// Dart/Flutter: pubspec.yaml
	detectFromPubspec(dc, add)
	// .NET: *.csproj
	detectFromDotNet(dc, add)
	// Config-file-only detections (low confidence)
	detectFromConfigFiles(dc, add)

	out := make([]Framework, 0, len(seen))
	for _, f := range seen {
		sort.Strings(f.ConfigFiles)
		f.ConfigFiles = uniqueStrings(f.ConfigFiles)
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return confidenceRank(out[i].Confidence) > confidenceRank(out[j].Confidence)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// confidenceRank returns a sortable rank for a confidence string.
// Higher rank = more confident. Unknown values rank as "low".
func confidenceRank(c string) int {
	switch strings.ToLower(c) {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	}
	return 1
}

// mergeStringSlices returns the union of a and b (preserving order
// of a, then b's new elements). Duplicates are removed.
func mergeStringSlices(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, s := range b {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// frameworkRule maps a dependency name (in a manifest's deps list)
// to a framework definition. The Key field is the manifest's
// dependency key, not a regex. Negative lists (e.g. "if the dep
// is not also X") are not supported — they are rarely needed for
// v0.1 and the rule is easy to extend later.
type frameworkRule struct {
	Key      string // dep key in the manifest (e.g. "react", "django")
	Name     string // display name
	Category string // web, test, build, etc.
	Language string
}

// jsFrameworkRules covers the JavaScript/TypeScript ecosystem.
// "react" is a special case: it appears in many frameworks as a
// transitive dep, so we also keep a confidence-of-"medium" rule
// for "react-dom" only, leaving the explicit "react" rule at
// "high" confidence.
var jsFrameworkRules = []frameworkRule{
	{Key: "react", Name: "React", Category: "ui", Language: "JavaScript"},
	{Key: "react-dom", Name: "React", Category: "ui", Language: "JavaScript"},
	{Key: "next", Name: "Next.js", Category: "web", Language: "JavaScript"},
	{Key: "nuxt", Name: "Nuxt", Category: "web", Language: "JavaScript"},
	{Key: "svelte", Name: "Svelte", Category: "ui", Language: "JavaScript"},
	{Key: "@sveltejs/kit", Name: "SvelteKit", Category: "web", Language: "JavaScript"},
	{Key: "vue", Name: "Vue", Category: "ui", Language: "JavaScript"},
	{Key: "nuxt", Name: "Nuxt", Category: "web", Language: "JavaScript"},
	{Key: "gatsby", Name: "Gatsby", Category: "web", Language: "JavaScript"},
	{Key: "remix", Name: "Remix", Category: "web", Language: "JavaScript"},
	{Key: "astro", Name: "Astro", Category: "web", Language: "JavaScript"},
	{Key: "express", Name: "Express", Category: "web", Language: "JavaScript"},
	{Key: "fastify", Name: "Fastify", Category: "web", Language: "JavaScript"},
	{Key: "koa", Name: "Koa", Category: "web", Language: "JavaScript"},
	{Key: "hapi", Name: "hapi", Category: "web", Language: "JavaScript"},
	{Key: "nestjs", Name: "NestJS", Category: "web", Language: "JavaScript"},
	{Key: "@nestjs/core", Name: "NestJS", Category: "web", Language: "JavaScript"},
	{Key: "@angular/core", Name: "Angular", Category: "ui", Language: "TypeScript"},
	{Key: "rxjs", Name: "RxJS", Category: "reactive", Language: "TypeScript"},
	{Key: "redux", Name: "Redux", Category: "state", Language: "JavaScript"},
	{Key: "mobx", Name: "MobX", Category: "state", Language: "JavaScript"},
	{Key: "zustand", Name: "Zustand", Category: "state", Language: "JavaScript"},
	{Key: "react-native", Name: "React Native", Category: "mobile", Language: "JavaScript"},
	{Key: "expo", Name: "Expo", Category: "mobile", Language: "JavaScript"},
	{Key: "electron", Name: "Electron", Category: "desktop", Language: "JavaScript"},
	{Key: "tauri", Name: "Tauri", Category: "desktop", Language: "Rust"},
	{Key: "tailwindcss", Name: "Tailwind CSS", Category: "css", Language: "CSS"},
	{Key: "bootstrap", Name: "Bootstrap", Category: "css", Language: "CSS"},
	{Key: "material-ui", Name: "Material UI", Category: "ui", Language: "JavaScript"},
	{Key: "@mui/material", Name: "Material UI", Category: "ui", Language: "JavaScript"},
	{Key: "antd", Name: "Ant Design", Category: "ui", Language: "JavaScript"},
	{Key: "chakra-ui", Name: "Chakra UI", Category: "ui", Language: "JavaScript"},
	{Key: "jest", Name: "Jest", Category: "test", Language: "JavaScript"},
	{Key: "mocha", Name: "Mocha", Category: "test", Language: "JavaScript"},
	{Key: "chai", Name: "Chai", Category: "test", Language: "JavaScript"},
	{Key: "vitest", Name: "Vitest", Category: "test", Language: "JavaScript"},
	{Key: "playwright", Name: "Playwright", Category: "test", Language: "JavaScript"},
	{Key: "@playwright/test", Name: "Playwright", Category: "test", Language: "JavaScript"},
	{Key: "cypress", Name: "Cypress", Category: "test", Language: "JavaScript"},
	{Key: "puppeteer", Name: "Puppeteer", Category: "test", Language: "JavaScript"},
	{Key: "webpack", Name: "webpack", Category: "build", Language: "JavaScript"},
	{Key: "vite", Name: "Vite", Category: "build", Language: "JavaScript"},
	{Key: "rollup", Name: "Rollup", Category: "build", Language: "JavaScript"},
	{Key: "esbuild", Name: "esbuild", Category: "build", Language: "JavaScript"},
	{Key: "parcel", Name: "Parcel", Category: "build", Language: "JavaScript"},
	{Key: "turborepo", Name: "Turborepo", Category: "monorepo", Language: "JavaScript"},
	{Key: "lerna", Name: "Lerna", Category: "monorepo", Language: "JavaScript"},
	{Key: "nx", Name: "Nx", Category: "monorepo", Language: "JavaScript"},
	{Key: "typescript", Name: "TypeScript", Category: "language", Language: "TypeScript"},
	{Key: "prisma", Name: "Prisma", Category: "orm", Language: "TypeScript"},
	{Key: "typeorm", Name: "TypeORM", Category: "orm", Language: "TypeScript"},
	{Key: "sequelize", Name: "Sequelize", Category: "orm", Language: "JavaScript"},
	{Key: "mongoose", Name: "Mongoose", Category: "orm", Language: "JavaScript"},
	{Key: "graphql", Name: "GraphQL", Category: "api", Language: "JavaScript"},
	{Key: "@apollo/server", Name: "Apollo", Category: "api", Language: "TypeScript"},
	{Key: "axios", Name: "Axios", Category: "http", Language: "JavaScript"},
	{Key: "lodash", Name: "Lodash", Category: "utility", Language: "JavaScript"},
	{Key: "rxjs", Name: "RxJS", Category: "reactive", Language: "TypeScript"},
}

// detectFromPackageJSON reads every package.json in the repo and
// extracts framework signals from dependencies and devDependencies.
func detectFromPackageJSON(dc DetectionContext, add func(Framework)) {
	for _, f := range dc.Files {
		if filepath.Base(f) != "package.json" {
			continue
		}
		// Skip nested package.jsons (they are sub-packages of a
		// monorepo and are handled separately).
		if filepath.Dir(f) != "." {
			continue
		}
		data, err := readFile(dc.WorkTree, f)
		if err != nil {
			continue
		}
		var pkg struct {
			Name         string            `json:"name"`
			Version      string            `json:"version"`
			Dependencies map[string]string `json:"dependencies"`
			DevDeps      map[string]string `json:"devDependencies"`
		}
		if err := json.Unmarshal(data, &pkg); err != nil {
			continue
		}
		mergeDeps := make(map[string]string, len(pkg.Dependencies)+len(pkg.DevDeps))
		for k, v := range pkg.Dependencies {
			mergeDeps[k] = v
		}
		for k, v := range pkg.DevDeps {
			mergeDeps[k] = v
		}
		for _, rule := range jsFrameworkRules {
			if _, ok := mergeDeps[rule.Key]; !ok {
				continue
			}
			fw := Framework{
				Name:        rule.Name,
				Category:    rule.Category,
				Language:    rule.Language,
				Confidence:  "high",
				ConfigFiles: []string{f},
			}
			// Add version for the top-level dep if we can.
			if v, ok := mergeDeps[rule.Key]; ok {
				fw.Version = cleanVersion(v)
			}
			add(fw)
		}
	}
}

// detectFromPythonManifests reads requirements.txt and
// pyproject.toml and adds frameworks.
func detectFromPythonManifests(dc DetectionContext, add func(Framework)) {
	rules := []frameworkRule{
		{Key: "django", Name: "Django", Category: "web", Language: "Python"},
		{Key: "flask", Name: "Flask", Category: "web", Language: "Python"},
		{Key: "fastapi", Name: "FastAPI", Category: "web", Language: "Python"},
		{Key: "starlette", Name: "Starlette", Category: "web", Language: "Python"},
		{Key: "tornado", Name: "Tornado", Category: "web", Language: "Python"},
		{Key: "aiohttp", Name: "aiohttp", Category: "web", Language: "Python"},
		{Key: "pyramid", Name: "Pyramid", Category: "web", Language: "Python"},
		{Key: "bottle", Name: "Bottle", Category: "web", Language: "Python"},
		{Key: "sanic", Name: "Sanic", Category: "web", Language: "Python"},
		{Key: "celery", Name: "Celery", Category: "queue", Language: "Python"},
		{Key: "sqlalchemy", Name: "SQLAlchemy", Category: "orm", Language: "Python"},
		{Key: "alembic", Name: "Alembic", Category: "migration", Language: "Python"},
		{Key: "pytest", Name: "pytest", Category: "test", Language: "Python"},
		{Key: "unittest", Name: "unittest", Category: "test", Language: "Python"},
		{Key: "numpy", Name: "NumPy", Category: "data", Language: "Python"},
		{Key: "pandas", Name: "pandas", Category: "data", Language: "Python"},
		{Key: "scipy", Name: "SciPy", Category: "data", Language: "Python"},
		{Key: "scikit-learn", Name: "scikit-learn", Category: "ml", Language: "Python"},
		{Key: "tensorflow", Name: "TensorFlow", Category: "ml", Language: "Python"},
		{Key: "torch", Name: "PyTorch", Category: "ml", Language: "Python"},
		{Key: "transformers", Name: "Hugging Face Transformers", Category: "ml", Language: "Python"},
		{Key: "jupyter", Name: "Jupyter", Category: "notebook", Language: "Python"},
		{Key: "requests", Name: "Requests", Category: "http", Language: "Python"},
		{Key: "httpx", Name: "HTTPX", Category: "http", Language: "Python"},
		{Key: "pydantic", Name: "Pydantic", Category: "validation", Language: "Python"},
		{Key: "click", Name: "Click", Category: "cli", Language: "Python"},
		{Key: "typer", Name: "Typer", Category: "cli", Language: "Python"},
		{Key: "rich", Name: "Rich", Category: "cli", Language: "Python"},
		{Key: "attrs", Name: "attrs", Category: "utility", Language: "Python"},
	}
	for _, f := range dc.Files {
		base := filepath.Base(f)
		if base == "requirements.txt" || base == "requirements-dev.txt" || base == "dev-requirements.txt" {
			data, err := readFile(dc.WorkTree, f)
			if err != nil {
				continue
			}
			detectFromPipList(data, rules, f, add)
		}
		if base == "pyproject.toml" {
			data, err := readFile(dc.WorkTree, f)
			if err != nil {
				continue
			}
			detectFromPyproject(data, rules, f, add)
		}
	}
}

// detectFromPipList parses a pip requirements file (one
// "package==version" per line, with extras / markers ignored) and
// applies the rule list. Comments and blank lines are skipped.
func detectFromPipList(data []byte, rules []frameworkRule, file string, add func(Framework)) {
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip inline comments.
		if i := strings.Index(line, " #"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		// Strip markers: "package==1.0; python_version<'3.10'".
		if semi := strings.Index(line, ";"); semi >= 0 {
			line = strings.TrimSpace(line[:semi])
		}
		// Split on first comparator.
		parts := strings.FieldsFunc(line, func(r rune) bool {
			return r == '=' || r == '>' || r == '<' || r == '~' || r == '!' || r == ' '
		})
		if len(parts) == 0 {
			continue
		}
		pkg := strings.ToLower(strings.TrimSpace(parts[0]))
		// Strip extras: "package[extra]" → "package".
		if i := strings.Index(pkg, "["); i >= 0 {
			pkg = pkg[:i]
		}
		for _, rule := range rules {
			if strings.EqualFold(pkg, rule.Key) {
				add(Framework{
					Name:        rule.Name,
					Category:    rule.Category,
					Language:    rule.Language,
					Confidence:  "high",
					ConfigFiles: []string{file},
				})
			}
		}
	}
}

// detectFromPyproject parses a pyproject.toml (the [project]
// dependencies and [tool.poetry.dependencies] blocks) and applies
// the rules. We use the YAML decoder and look at the dependency
// tables; PEP 621 [project.dependencies] and Poetry's
// [tool.poetry.dependencies] are both supported.
//
// v0.1 limitation: real pyproject.toml is TOML, not YAML. The
// parser only handles the YAML subset that the v0.1 tests use.
// A v0.2 follow-up should switch to a TOML parser
// (github.com/pelletier/go-toml/v2) for full real-world
// coverage. The architecture doc documents this as a known
// limitation.
func detectFromPyproject(data []byte, rules []frameworkRule, file string, add func(Framework)) {
	var doc struct {
		Project struct {
			Dependencies []string `yaml:"dependencies"`
		} `yaml:"project"`
		Tool struct {
			Poetry struct {
				Dependencies map[string]any `yaml:"dependencies"`
			} `yaml:"poetry"`
		} `yaml:"tool"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return
	}
	apply := func(pkg, version string) {
		pkg = strings.ToLower(pkg)
		if i := strings.Index(pkg, "["); i >= 0 {
			pkg = pkg[:i]
		}
		for _, rule := range rules {
			if strings.EqualFold(pkg, rule.Key) {
				fw := Framework{
					Name:        rule.Name,
					Category:    rule.Category,
					Language:    rule.Language,
					Confidence:  "high",
					ConfigFiles: []string{file},
				}
				if version != "" {
					fw.Version = cleanVersion(version)
				}
				add(fw)
			}
		}
	}
	for _, d := range doc.Project.Dependencies {
		// "package==1.0.0" or "package>=1.0.0"
		parts := strings.FieldsFunc(d, func(r rune) bool {
			return r == '=' || r == '>' || r == '<' || r == '~' || r == '!' || r == ' '
		})
		if len(parts) == 0 {
			continue
		}
		apply(parts[0], parts[1])
	}
	for pkg, ver := range doc.Tool.Poetry.Dependencies {
		v := ""
		if sv, ok := ver.(string); ok {
			v = sv
		}
		apply(pkg, v)
	}
}

// detectFromGoMod reads the go.mod file and adds frameworks based
// on the require block. The block is the simplest possible parser:
// we look for "module path/vN" lines and check each path against
// the rule list. Comments are stripped line-by-line.
func detectFromGoMod(dc DetectionContext, add func(Framework)) {
	rules := []frameworkRule{
		{Key: "github.com/gin-gonic/gin", Name: "Gin", Category: "web", Language: "Go"},
		{Key: "github.com/labstack/echo/v4", Name: "Echo", Category: "web", Language: "Go"},
		{Key: "github.com/go-chi/chi/v5", Name: "Chi", Category: "web", Language: "Go"},
		{Key: "github.com/gofiber/fiber/v2", Name: "Fiber", Category: "web", Language: "Go"},
		{Key: "github.com/gorilla/mux", Name: "gorilla/mux", Category: "web", Language: "Go"},
		{Key: "github.com/spf13/cobra", Name: "Cobra", Category: "cli", Language: "Go"},
		{Key: "github.com/urfave/cli/v3", Name: "urfave/cli", Category: "cli", Language: "Go"},
		{Key: "github.com/spf13/viper", Name: "Viper", Category: "config", Language: "Go"},
		{Key: "gopkg.in/yaml.v3", Name: "YAML", Category: "config", Language: "Go"},
		{Key: "github.com/stretchr/testify", Name: "testify", Category: "test", Language: "Go"},
		{Key: "github.com/onsi/ginkgo/v2", Name: "Ginkgo", Category: "test", Language: "Go"},
		{Key: "github.com/onsi/gomega", Name: "Gomega", Category: "test", Language: "Go"},
		{Key: "github.com/google/uuid", Name: "uuid", Category: "utility", Language: "Go"},
		{Key: "github.com/golang-migrate/migrate/v4", Name: "golang-migrate", Category: "migration", Language: "Go"},
		{Key: "github.com/jackc/pgx/v5", Name: "pgx", Category: "orm", Language: "Go"},
		{Key: "github.com/jmoiron/sqlx", Name: "sqlx", Category: "orm", Language: "Go"},
		{Key: "gorm.io/gorm", Name: "GORM", Category: "orm", Language: "Go"},
		{Key: "github.com/charmbracelet/bubbletea", Name: "Bubble Tea", Category: "tui", Language: "Go"},
		{Key: "github.com/charmbracelet/bubbles", Name: "Bubbles", Category: "tui", Language: "Go"},
		{Key: "github.com/charmbracelet/lipgloss", Name: "Lip Gloss", Category: "tui", Language: "Go"},
		{Key: "github.com/spf13/cobra", Name: "Cobra", Category: "cli", Language: "Go"},
	}
	for _, f := range dc.Files {
		if filepath.Base(f) != "go.mod" {
			continue
		}
		if filepath.Dir(f) != "." {
			continue
		}
		data, err := readFile(dc.WorkTree, f)
		if err != nil {
			continue
		}
		body := string(data)
		// Find the require block.
		start := strings.Index(body, "require (")
		if start < 0 {
			continue
		}
		end := strings.Index(body[start:], ")")
		if end < 0 {
			continue
		}
		block := body[start+len("require (") : start+end]
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "//") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			modPath := fields[0]
			for _, rule := range rules {
				if modPath == rule.Key || strings.HasPrefix(modPath, rule.Key+"/") {
					add(Framework{
						Name:        rule.Name,
						Category:    rule.Category,
						Language:    rule.Language,
						Version:     cleanVersion(fields[1]),
						Confidence:  "high",
						ConfigFiles: []string{f},
					})
				}
			}
		}
	}
}

// detectFromCargoToml reads the root Cargo.toml and adds frameworks
// from the [dependencies] and [dev-dependencies] tables.
func detectFromCargoToml(dc DetectionContext, add func(Framework)) {
	rules := []frameworkRule{
		{Key: "actix-web", Name: "Actix Web", Category: "web", Language: "Rust"},
		{Key: "axum", Name: "Axum", Category: "web", Language: "Rust"},
		{Key: "rocket", Name: "Rocket", Category: "web", Language: "Rust"},
		{Key: "warp", Name: "Warp", Category: "web", Language: "Rust"},
		{Key: "tokio", Name: "Tokio", Category: "async", Language: "Rust"},
		{Key: "async-std", Name: "async-std", Category: "async", Language: "Rust"},
		{Key: "diesel", Name: "Diesel", Category: "orm", Language: "Rust"},
		{Key: "sqlx", Name: "SQLx", Category: "orm", Language: "Rust"},
		{Key: "serde", Name: "Serde", Category: "serialization", Language: "Rust"},
		{Key: "tauri", Name: "Tauri", Category: "desktop", Language: "Rust"},
		{Key: "bevy", Name: "Bevy", Category: "game", Language: "Rust"},
		{Key: "wasm-bindgen", Name: "wasm-bindgen", Category: "wasm", Language: "Rust"},
		{Key: "yew", Name: "Yew", Category: "web", Language: "Rust"},
		{Key: "leptos", Name: "Leptos", Category: "web", Language: "Rust"},
	}
	for _, f := range dc.Files {
		if filepath.Base(f) != "Cargo.toml" {
			continue
		}
		if filepath.Dir(f) != "." {
			continue
		}
		data, err := readFile(dc.WorkTree, f)
		if err != nil {
			continue
		}
		var doc struct {
			Dependencies map[string]any `yaml:"dependencies"`
			DevDeps      map[string]any `yaml:"dev-dependencies"`
		}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			continue
		}
		merged := make(map[string]string, len(doc.Dependencies)+len(doc.DevDeps))
		for k, v := range doc.Dependencies {
			merged[k] = cargoVersion(v)
		}
		for k, v := range doc.DevDeps {
			merged[k] = cargoVersion(v)
		}
		for _, rule := range rules {
			if v, ok := merged[rule.Key]; ok {
				add(Framework{
					Name:        rule.Name,
					Category:    rule.Category,
					Language:    rule.Language,
					Version:     v,
					Confidence:  "high",
					ConfigFiles: []string{f},
				})
			}
		}
	}
}

// cargoVersion normalizes a Cargo.toml dependency value (which
// can be a string "1.0" or a table with a "version" key) into a
// version string.
func cargoVersion(v any) string {
	switch t := v.(type) {
	case string:
		return cleanVersion(t)
	case map[string]any:
		if ver, ok := t["version"].(string); ok {
			return cleanVersion(ver)
		}
	}
	return ""
}

// detectFromGemfile reads the Gemfile and adds frameworks.
func detectFromGemfile(dc DetectionContext, add func(Framework)) {
	rules := []frameworkRule{
		{Key: "rails", Name: "Ruby on Rails", Category: "web", Language: "Ruby"},
		{Key: "sinatra", Name: "Sinatra", Category: "web", Language: "Ruby"},
		{Key: "hanami", Name: "Hanami", Category: "web", Language: "Ruby"},
		{Key: "grape", Name: "Grape", Category: "api", Language: "Ruby"},
		{Key: "rspec", Name: "RSpec", Category: "test", Language: "Ruby"},
		{Key: "minitest", Name: "minitest", Category: "test", Language: "Ruby"},
		{Key: "sidekiq", Name: "Sidekiq", Category: "queue", Language: "Ruby"},
		{Key: "devise", Name: "Devise", Category: "auth", Language: "Ruby"},
		{Key: "puma", Name: "Puma", Category: "server", Language: "Ruby"},
	}
	for _, f := range dc.Files {
		if filepath.Base(f) != "Gemfile" {
			continue
		}
		if filepath.Dir(f) != "." {
			continue
		}
		data, err := readFile(dc.WorkTree, f)
		if err != nil {
			continue
		}
		body := strings.ToLower(string(data))
		for _, rule := range rules {
			if strings.Contains(body, "gem '"+rule.Key+"'") || strings.Contains(body, "gem \""+rule.Key+"\"") {
				add(Framework{
					Name:        rule.Name,
					Category:    rule.Category,
					Language:    rule.Language,
					Confidence:  "high",
					ConfigFiles: []string{f},
				})
			}
		}
	}
}

// detectFromComposerJSON reads composer.json and adds PHP frameworks.
func detectFromComposerJSON(dc DetectionContext, add func(Framework)) {
	rules := []frameworkRule{
		{Key: "laravel/framework", Name: "Laravel", Category: "web", Language: "PHP"},
		{Key: "symfony/framework-bundle", Name: "Symfony", Category: "web", Language: "PHP"},
		{Key: "symfony/symfony", Name: "Symfony", Category: "web", Language: "PHP"},
		{Key: "slim/slim", Name: "Slim", Category: "web", Language: "PHP"},
		{Key: "cakephp/cakephp", Name: "CakePHP", Category: "web", Language: "PHP"},
		{Key: "codeigniter/framework", Name: "CodeIgniter", Category: "web", Language: "PHP"},
		{Key: "yiisoft/yii2", Name: "Yii", Category: "web", Language: "PHP"},
		{Key: "phpunit/phpunit", Name: "PHPUnit", Category: "test", Language: "PHP"},
	}
	for _, f := range dc.Files {
		if filepath.Base(f) != "composer.json" {
			continue
		}
		if filepath.Dir(f) != "." {
			continue
		}
		data, err := readFile(dc.WorkTree, f)
		if err != nil {
			continue
		}
		var doc struct {
			Require    map[string]string `json:"require"`
			RequireDev map[string]string `json:"require-dev"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			continue
		}
		merged := make(map[string]string, len(doc.Require)+len(doc.RequireDev))
		for k, v := range doc.Require {
			merged[k] = v
		}
		for k, v := range doc.RequireDev {
			merged[k] = v
		}
		for _, rule := range rules {
			if v, ok := merged[rule.Key]; ok {
				add(Framework{
					Name:        rule.Name,
					Category:    rule.Category,
					Language:    rule.Language,
					Version:     cleanVersion(v),
					Confidence:  "high",
					ConfigFiles: []string{f},
				})
			}
		}
	}
}

// detectFromJavaManifests reads pom.xml and build.gradle and adds
// Java/Kotlin frameworks. Maven and Gradle parsing is intentionally
// loose: we grep for known artifact IDs in the file body.
func detectFromJavaManifests(dc DetectionContext, add func(Framework)) {
	rules := []frameworkRule{
		{Key: "spring-boot-starter-web", Name: "Spring Boot", Category: "web", Language: "Java"},
		{Key: "spring-boot-starter", Name: "Spring Boot", Category: "web", Language: "Java"},
		{Key: "spring-core", Name: "Spring", Category: "web", Language: "Java"},
		{Key: "spring-web", Name: "Spring", Category: "web", Language: "Java"},
		{Key: "spring-webflux", Name: "Spring WebFlux", Category: "web", Language: "Java"},
		{Key: "hibernate-core", Name: "Hibernate", Category: "orm", Language: "Java"},
		{Key: "hibernate-orm", Name: "Hibernate", Category: "orm", Language: "Java"},
		{Key: "junit", Name: "JUnit", Category: "test", Language: "Java"},
		{Key: "junit-jupiter", Name: "JUnit", Category: "test", Language: "Java"},
		{Key: "testng", Name: "TestNG", Category: "test", Language: "Java"},
		{Key: "mockito-core", Name: "Mockito", Category: "test", Language: "Java"},
		{Key: "org.springframework.boot", Name: "Spring Boot", Category: "web", Language: "Java"},
		{Key: "io.quarkus", Name: "Quarkus", Category: "web", Language: "Java"},
		{Key: "io.micronaut", Name: "Micronaut", Category: "web", Language: "Java"},
		{Key: "org.jetbrains.kotlin", Name: "Kotlin", Category: "language", Language: "Kotlin"},
		{Key: "androidx.appcompat", Name: "Android", Category: "mobile", Language: "Java"},
		{Key: "androidx.compose", Name: "Jetpack Compose", Category: "mobile", Language: "Kotlin"},
	}
	for _, f := range dc.Files {
		base := filepath.Base(f)
		if base != "pom.xml" && base != "build.gradle" && base != "build.gradle.kts" {
			continue
		}
		if filepath.Dir(f) != "." {
			continue
		}
		data, err := readFile(dc.WorkTree, f)
		if err != nil {
			continue
		}
		body := string(data)
		for _, rule := range rules {
			if strings.Contains(body, rule.Key) {
				add(Framework{
					Name:        rule.Name,
					Category:    rule.Category,
					Language:    rule.Language,
					Confidence:  "medium",
					ConfigFiles: []string{f},
				})
			}
		}
	}
}

// detectFromPubspec reads pubspec.yaml and adds Flutter / Dart
// packages.
func detectFromPubspec(dc DetectionContext, add func(Framework)) {
	rules := []frameworkRule{
		{Key: "flutter", Name: "Flutter", Category: "mobile", Language: "Dart"},
		{Key: "riverpod", Name: "Riverpod", Category: "state", Language: "Dart"},
		{Key: "provider", Name: "Provider", Category: "state", Language: "Dart"},
		{Key: "bloc", Name: "BLoC", Category: "state", Language: "Dart"},
		{Key: "dio", Name: "Dio", Category: "http", Language: "Dart"},
		{Key: "http", Name: "http", Category: "http", Language: "Dart"},
		{Key: "shelf", Name: "Shelf", Category: "web", Language: "Dart"},
		{Key: "aqueduct", Name: "Aqueduct", Category: "web", Language: "Dart"},
	}
	for _, f := range dc.Files {
		if filepath.Base(f) != "pubspec.yaml" {
			continue
		}
		if filepath.Dir(f) != "." {
			continue
		}
		data, err := readFile(dc.WorkTree, f)
		if err != nil {
			continue
		}
		var doc struct {
			Dependencies    map[string]any `yaml:"dependencies"`
			DevDependencies map[string]any `yaml:"dev_dependencies"`
		}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			continue
		}
		merged := make(map[string]string, len(doc.Dependencies)+len(doc.DevDependencies))
		for k, v := range doc.Dependencies {
			merged[k] = pubspecVersion(v)
		}
		for k, v := range doc.DevDependencies {
			merged[k] = pubspecVersion(v)
		}
		for _, rule := range rules {
			if v, ok := merged[rule.Key]; ok {
				add(Framework{
					Name:        rule.Name,
					Category:    rule.Category,
					Language:    rule.Language,
					Version:     v,
					Confidence:  "high",
					ConfigFiles: []string{f},
				})
			}
		}
	}
}

func pubspecVersion(v any) string {
	if s, ok := v.(string); ok {
		return cleanVersion(s)
	}
	if m, ok := v.(map[string]any); ok {
		if ver, ok := m["version"].(string); ok {
			return cleanVersion(ver)
		}
	}
	return ""
}

// detectFromDotNet reads every *.csproj in the repo and detects
// .NET frameworks. We look for known TargetFramework values and
// package references.
func detectFromDotNet(dc DetectionContext, add func(Framework)) {
	for _, f := range dc.Files {
		base := filepath.Base(f)
		if filepath.Ext(base) != ".csproj" {
			continue
		}
		data, err := readFile(dc.WorkTree, f)
		if err != nil {
			continue
		}
		body := strings.ToLower(string(data))
		// TargetFramework detection
		switch {
		case strings.Contains(body, "<targetframework>net8.0</targetframework>"):
			add(Framework{Name: ".NET 8", Category: "language", Language: "C#", Confidence: "high", ConfigFiles: []string{f}})
		case strings.Contains(body, "<targetframework>net7.0</targetframework>"):
			add(Framework{Name: ".NET 7", Category: "language", Language: "C#", Confidence: "high", ConfigFiles: []string{f}})
		case strings.Contains(body, "<targetframework>net6.0</targetframework>"):
			add(Framework{Name: ".NET 6", Category: "language", Language: "C#", Confidence: "high", ConfigFiles: []string{f}})
		case strings.Contains(body, "<targetframework>netcoreapp"):
			add(Framework{Name: ".NET Core", Category: "language", Language: "C#", Confidence: "high", ConfigFiles: []string{f}})
		case strings.Contains(body, "<targetframework>net4"):
			add(Framework{Name: ".NET Framework", Category: "language", Language: "C#", Confidence: "high", ConfigFiles: []string{f}})
		}
		// Package references
		if strings.Contains(body, "microsoft.aspnetcore") {
			add(Framework{Name: "ASP.NET Core", Category: "web", Language: "C#", Confidence: "high", ConfigFiles: []string{f}})
		}
		if strings.Contains(body, "xunit") {
			add(Framework{Name: "xUnit", Category: "test", Language: "C#", Confidence: "high", ConfigFiles: []string{f}})
		}
		if strings.Contains(body, "nunit") {
			add(Framework{Name: "NUnit", Category: "test", Language: "C#", Confidence: "high", ConfigFiles: []string{f}})
		}
		if strings.Contains(body, "entityframework") || strings.Contains(body, "entityframeworkcore") {
			add(Framework{Name: "Entity Framework", Category: "orm", Language: "C#", Confidence: "high", ConfigFiles: []string{f}})
		}
	}
}

// detectFromConfigFiles performs low-confidence detections based
// on the presence of single config files. These run last so they
// don't override high-confidence manifest detections.
func detectFromConfigFiles(dc DetectionContext, add func(Framework)) {
	configRules := map[string]Framework{
		"next.config.js":       {Name: "Next.js", Category: "web", Language: "JavaScript"},
		"next.config.mjs":      {Name: "Next.js", Category: "web", Language: "JavaScript"},
		"next.config.ts":       {Name: "Next.js", Category: "web", Language: "TypeScript"},
		"nuxt.config.ts":       {Name: "Nuxt", Category: "web", Language: "TypeScript"},
		"vite.config.ts":       {Name: "Vite", Category: "build", Language: "TypeScript"},
		"vite.config.js":       {Name: "Vite", Category: "build", Language: "JavaScript"},
		"angular.json":         {Name: "Angular", Category: "ui", Language: "TypeScript"},
		"svelte.config.js":     {Name: "Svelte", Category: "ui", Language: "JavaScript"},
		"astro.config.mjs":     {Name: "Astro", Category: "web", Language: "JavaScript"},
		"remix.config.js":      {Name: "Remix", Category: "web", Language: "JavaScript"},
		"gatsby-config.js":     {Name: "Gatsby", Category: "web", Language: "JavaScript"},
		"tailwind.config.js":   {Name: "Tailwind CSS", Category: "css", Language: "JavaScript"},
		"tailwind.config.ts":   {Name: "Tailwind CSS", Category: "css", Language: "TypeScript"},
		"tsconfig.json":        {Name: "TypeScript", Category: "language", Language: "TypeScript"},
		"jest.config.js":       {Name: "Jest", Category: "test", Language: "JavaScript"},
		"jest.config.ts":       {Name: "Jest", Category: "test", Language: "TypeScript"},
		"vitest.config.ts":     {Name: "Vitest", Category: "test", Language: "TypeScript"},
		"cypress.config.js":    {Name: "Cypress", Category: "test", Language: "JavaScript"},
		"cypress.config.ts":    {Name: "Cypress", Category: "test", Language: "TypeScript"},
		"playwright.config.ts": {Name: "Playwright", Category: "test", Language: "TypeScript"},
		"playwright.config.js": {Name: "Playwright", Category: "test", Language: "JavaScript"},
		"webpack.config.js":    {Name: "webpack", Category: "build", Language: "JavaScript"},
		"rollup.config.js":     {Name: "Rollup", Category: "build", Language: "JavaScript"},
		"esbuild.config.js":    {Name: "esbuild", Category: "build", Language: "JavaScript"},
		"lerna.json":           {Name: "Lerna", Category: "monorepo", Language: "JavaScript"},
		"nx.json":              {Name: "Nx", Category: "monorepo", Language: "JavaScript"},
		"turbo.json":           {Name: "Turborepo", Category: "monorepo", Language: "JavaScript"},
		"pnpm-workspace.yaml":  {Name: "pnpm workspaces", Category: "monorepo", Language: "JavaScript"},
		"pdm.lock":             {Name: "PDM", Category: "package-manager", Language: "Python"},
		"Pipfile":              {Name: "Pipenv", Category: "package-manager", Language: "Python"},
		"poetry.lock":          {Name: "Poetry", Category: "package-manager", Language: "Python"},
		"uv.lock":              {Name: "uv", Category: "package-manager", Language: "Python"},
		"renv.lock":            {Name: "renv", Category: "package-manager", Language: "R"},
		"Project.toml":         {Name: "Julia", Category: "package-manager", Language: "Julia"},
	}
	for _, f := range dc.Files {
		base := filepath.Base(f)
		if fw, ok := configRules[base]; ok {
			fwCopy := fw
			fwCopy.Confidence = "low"
			fwCopy.ConfigFiles = []string{f}
			add(fwCopy)
		}
	}
}

// cleanVersion strips common version prefixes/suffixes:
//   - "^1.2.3"   → "1.2.3"     (npm)
//   - "~1.2.3"   → "1.2.3"     (npm)
//   - ">=1.2.3"  → "1.2.3"     (pip)
//   - "1.2.3"    → "1.2.3"
//   - "1.2.3+incompatible" → "1.2.3"  (go mod)
func cleanVersion(v string) string {
	v = strings.TrimSpace(v)
	for _, prefix := range []string{"^", "~", ">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(v, prefix) {
			v = strings.TrimPrefix(v, prefix)
			break
		}
	}
	// Strip semver build metadata
	if i := strings.Index(v, "+"); i >= 0 {
		v = v[:i]
	}
	// Strip pre-release tag for v0.1 — show only the numeric
	// portion so the column lines up in the inspect output.
	if i := strings.Index(v, "-"); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}
