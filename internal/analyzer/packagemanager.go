package analyzer

import (
	"encoding/json"
	"path/filepath"
	"sort"
)

// packageManager is the internal record used while building the
// model's PackageManagers slice. We use an ordered list (not a
// map) so the output order is stable across runs.
type packageManager struct {
	Name      string
	Ecosystem string
	Manifest  string
	Lockfile  string
	IsFrozen  bool
}

// packageManagerRules is the manifest → package-manager table.
// The detector scans every file in dc.Files against the basename
// of the manifest; the rule is keyed on the manifest filename
// (e.g. "go.mod", "package.json", "requirements.txt").
type packageManagerRule struct {
	Manifest  string
	Lockfiles []string // any of these means the lockfile is checked in
	Name      string
	Ecosystem string
}

var packageManagerRules = []packageManagerRule{
	{Manifest: "go.mod", Lockfiles: []string{"go.sum"}, Name: "Go modules", Ecosystem: "Go"},
	{Manifest: "package.json", Lockfiles: []string{"package-lock.json", "yarn.lock", "pnpm-lock.yaml", "bun.lockb"}, Name: "npm", Ecosystem: "JavaScript"},
	{Manifest: "package.json", Lockfiles: []string{"yarn.lock"}, Name: "Yarn", Ecosystem: "JavaScript"},
	{Manifest: "package.json", Lockfiles: []string{"pnpm-lock.yaml"}, Name: "pnpm", Ecosystem: "JavaScript"},
	{Manifest: "package.json", Lockfiles: []string{"bun.lockb"}, Name: "Bun", Ecosystem: "JavaScript"},
	{Manifest: "requirements.txt", Lockfiles: []string{"requirements.lock"}, Name: "pip", Ecosystem: "Python"},
	{Manifest: "pyproject.toml", Lockfiles: []string{"poetry.lock", "uv.lock", "pdm.lock", "Pipfile.lock"}, Name: "pip", Ecosystem: "Python"},
	{Manifest: "pyproject.toml", Lockfiles: []string{"poetry.lock"}, Name: "Poetry", Ecosystem: "Python"},
	{Manifest: "pyproject.toml", Lockfiles: []string{"uv.lock"}, Name: "uv", Ecosystem: "Python"},
	{Manifest: "pyproject.toml", Lockfiles: []string{"pdm.lock"}, Name: "PDM", Ecosystem: "Python"},
	{Manifest: "Pipfile", Lockfiles: []string{"Pipfile.lock"}, Name: "Pipenv", Ecosystem: "Python"},
	{Manifest: "Cargo.toml", Lockfiles: []string{"Cargo.lock"}, Name: "Cargo", Ecosystem: "Rust"},
	{Manifest: "pom.xml", Lockfiles: []string{}, Name: "Maven", Ecosystem: "Java"},
	{Manifest: "build.gradle", Lockfiles: []string{}, Name: "Gradle", Ecosystem: "Java"},
	{Manifest: "build.gradle.kts", Lockfiles: []string{}, Name: "Gradle", Ecosystem: "Kotlin"},
	{Manifest: "Gemfile", Lockfiles: []string{"Gemfile.lock"}, Name: "Bundler", Ecosystem: "Ruby"},
	{Manifest: "composer.json", Lockfiles: []string{"composer.lock"}, Name: "Composer", Ecosystem: "PHP"},
	{Manifest: "pubspec.yaml", Lockfiles: []string{"pubspec.lock"}, Name: "pub", Ecosystem: "Dart"},
	{Manifest: "Package.swift", Lockfiles: []string{"Package.resolved"}, Name: "SwiftPM", Ecosystem: "Swift"},
	{Manifest: "Project.toml", Lockfiles: []string{"Manifest.toml"}, Name: "Pkg", Ecosystem: "Julia"},
	{Manifest: "mix.exs", Lockfiles: []string{"mix.lock"}, Name: "Mix", Ecosystem: "Elixir"},
	{Manifest: "rebar.config", Lockfiles: []string{"rebar.lock"}, Name: "rebar3", Ecosystem: "Erlang"},
	{Manifest: "cabal.project", Lockfiles: []string{"cabal.project.freeze"}, Name: "Cabal", Ecosystem: "Haskell"},
	{Manifest: "stack.yaml", Lockfiles: []string{"stack.yaml.lock"}, Name: "Stack", Ecosystem: "Haskell"},
	{Manifest: "Deno.json", Lockfiles: []string{"deno.lock"}, Name: "Deno", Ecosystem: "TypeScript"},
	{Manifest: "deno.json", Lockfiles: []string{"deno.lock"}, Name: "Deno", Ecosystem: "TypeScript"},
	{Manifest: "deno.jsonc", Lockfiles: []string{"deno.lock"}, Name: "Deno", Ecosystem: "TypeScript"},
	{Manifest: "paket.dependencies", Lockfiles: []string{"paket.lock"}, Name: "Paket", Ecosystem: ".NET"},
	{Manifest: "*.csproj", Lockfiles: []string{"packages.lock.json"}, Name: "NuGet", Ecosystem: ".NET"},
}

// detectPackageManagers scans dc.Files for known manifest files
// and pairs them with their lockfiles. The output is sorted by
// (Ecosystem, Name) for stable rendering.
//
// A manifest may match multiple rules (e.g. pyproject.toml with
// both poetry.lock and uv.lock — but only one will be the active
// lockfile in any given repo; we pick the first one found).
func detectPackageManagers(dc DetectionContext) []PackageManager {
	if len(dc.Files) == 0 {
		return nil
	}
	// Build a basename index for fast lockfile lookup.
	basenames := make(map[string]bool, len(dc.Files))
	for _, f := range dc.Files {
		basenames[filepath.Base(f)] = true
	}

	seen := make(map[string]packageManager)
	record := func(pm packageManager) {
		// De-duplicate by (Ecosystem, Name). Multiple manifests
		// for the same ecosystem+name (e.g. requirements.txt and
		// pyproject.toml both producing "pip") collapse to one
		// entry; we keep the first one's manifest.
		key := pm.Ecosystem + "/" + pm.Name
		if existing, ok := seen[key]; ok {
			// Upgrade the lockfile if a later detection found one.
			if existing.Lockfile == "" && pm.Lockfile != "" {
				existing.Lockfile = pm.Lockfile
				existing.IsFrozen = pm.IsFrozen
			}
			seen[key] = existing
			return
		}
		seen[key] = pm
	}

	// First pass: walk every manifest rule and see if the
	// corresponding manifest file exists in the work tree.
	for _, f := range dc.Files {
		base := filepath.Base(f)
		for _, rule := range packageManagerRules {
			if rule.Manifest == base {
				// Special-case for *.csproj — match the
				// glob directly.
				pm := packageManager{
					Name:      rule.Name,
					Ecosystem: rule.Ecosystem,
					Manifest:  f,
				}
				// Find the first matching lockfile.
				for _, lf := range rule.Lockfiles {
					if basenames[lf] {
						pm.Lockfile = lf
						pm.IsFrozen = true
						break
					}
				}
				record(pm)
			}
		}
	}

	// Second pass: look for package managers whose ONLY signal is
	// the lockfile (no manifest at the work tree root — common in
	// vendored / extracted archives).
	lockfileOnly := map[string]packageManager{
		"yarn.lock":         {Name: "Yarn", Ecosystem: "JavaScript", Lockfile: "yarn.lock", IsFrozen: true},
		"package-lock.json": {Name: "npm", Ecosystem: "JavaScript", Lockfile: "package-lock.json", IsFrozen: true},
		"pnpm-lock.yaml":    {Name: "pnpm", Ecosystem: "JavaScript", Lockfile: "pnpm-lock.yaml", IsFrozen: true},
		"bun.lockb":         {Name: "Bun", Ecosystem: "JavaScript", Lockfile: "bun.lockb", IsFrozen: true},
		"go.sum":            {Name: "Go modules", Ecosystem: "Go", Lockfile: "go.sum", IsFrozen: true},
		"Gemfile.lock":      {Name: "Bundler", Ecosystem: "Ruby", Lockfile: "Gemfile.lock", IsFrozen: true},
		"composer.lock":     {Name: "Composer", Ecosystem: "PHP", Lockfile: "composer.lock", IsFrozen: true},
		"pubspec.lock":      {Name: "pub", Ecosystem: "Dart", Lockfile: "pubspec.lock", IsFrozen: true},
		"Cargo.lock":        {Name: "Cargo", Ecosystem: "Rust", Lockfile: "Cargo.lock", IsFrozen: true},
		"poetry.lock":       {Name: "Poetry", Ecosystem: "Python", Lockfile: "poetry.lock", IsFrozen: true},
		"Pipfile.lock":      {Name: "Pipenv", Ecosystem: "Python", Lockfile: "Pipfile.lock", IsFrozen: true},
		"uv.lock":           {Name: "uv", Ecosystem: "Python", Lockfile: "uv.lock", IsFrozen: true},
		"pdm.lock":          {Name: "PDM", Ecosystem: "Python", Lockfile: "pdm.lock", IsFrozen: true},
	}
	for base := range basenames {
		if pm, ok := lockfileOnly[base]; ok {
			pm.Manifest = "" // unknown; user has only the lockfile
			record(pm)
		}
	}

	// Special case: an empty pyproject.toml with no [project]
	// table is not a package manager; skip records whose only
	// signal is an empty manifest. (Already handled by the
	// basenames check above.)

	// Convert to the public type and sort.
	out := make([]PackageManager, 0, len(seen))
	for _, pm := range seen {
		out = append(out, PackageManager{
			Name:      pm.Name,
			Ecosystem: pm.Ecosystem,
			Manifest:  pm.Manifest,
			Lockfile:  pm.Lockfile,
			IsFrozen:  pm.IsFrozen,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Ecosystem != out[j].Ecosystem {
			return out[i].Ecosystem < out[j].Ecosystem
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// readRootPackageJSON reads the top-level package.json and
// returns its "packageManager" field (which is the official way
// to declare a package manager + version in a project). It also
// returns the package.json's full contents so callers can
// inspect other fields. Returns ok=false when the file is
// missing or unparseable.
func readRootPackageJSON(dc DetectionContext) (data []byte, ok bool) {
	for _, f := range dc.Files {
		if filepath.Base(f) != "package.json" {
			continue
		}
		if filepath.Dir(f) != "." {
			continue
		}
		d, err := readFile(dc.WorkTree, f)
		if err != nil {
			return nil, false
		}
		return d, true
	}
	return nil, false
}

// packageManagerFromNpmField reads the "packageManager" field
// from a package.json. It is the official way to declare
// "yarn@1.22.19" or "pnpm@8.0.0" without a lockfile. Returns ""
// when the field is absent or invalid.
func packageManagerFromNpmField(dc DetectionContext) string {
	data, ok := readRootPackageJSON(dc)
	if !ok {
		return ""
	}
	var pkg struct {
		PackageManager string `json:"packageManager"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}
	return pkg.PackageManager
}
