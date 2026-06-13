package analyzer

import (
	"path/filepath"
	"sort"
	"strings"
)

// detectMonorepo looks for the common monorepo signals and
// returns a populated MonorepoInfo. The detector is "is it a
// monorepo" + "what tool, if any, manages the packages".
//
// A repo is a monorepo if any of the following are true:
//
//   - It has a workspaces field in package.json (npm/yarn/pnpm).
//   - It has pnpm-workspace.yaml (pnpm).
//   - It has lerna.json (Lerna).
//   - It has nx.json (Nx).
//   - It has turbo.json (Turborepo).
//   - It has bazel WORKSPACE/BUILD files at the root (Bazel).
//   - It has go.work (Go workspaces).
//   - It has Cargo workspaces (a [workspace] section in Cargo.toml).
//   - It has multiple independent package directories (heuristic:
//     two or more directories with their own package.json /
//     go.mod / Cargo.toml / etc.).
func detectMonorepo(dc DetectionContext) MonorepoInfo {
	info := MonorepoInfo{}

	// Tool detection (each tool is exclusive — the first one we
	// find wins, ordered by specificity).
	for _, f := range dc.Files {
		base := filepath.Base(f)
		switch base {
		case "pnpm-workspace.yaml":
			info.IsMonorepo = true
			info.Tool = "pnpm workspaces"
		case "lerna.json":
			info.IsMonorepo = true
			info.Tool = "Lerna"
		case "nx.json":
			info.IsMonorepo = true
			info.Tool = "Nx"
		case "turbo.json":
			info.IsMonorepo = true
			info.Tool = "Turborepo"
		case "go.work":
			info.IsMonorepo = true
			info.Tool = "Go workspaces"
		case "WORKSPACE", "WORKSPACE.bazel", "MODULE.bazel":
			info.IsMonorepo = true
			info.Tool = "Bazel"
		}
		if info.IsMonorepo {
			break
		}
	}

	// Cargo workspaces: the [workspace] section in Cargo.toml.
	if !info.IsMonorepo {
		for _, f := range dc.Files {
			if filepath.Base(f) == "Cargo.toml" && filepath.Dir(f) == "." {
				data, err := readFile(dc.WorkTree, f)
				if err != nil {
					continue
				}
				if strings.Contains(string(data), "[workspace]") {
					info.IsMonorepo = true
					info.Tool = "Cargo workspaces"
					break
				}
			}
		}
	}

	// npm/yarn/pnpm workspaces (via package.json's "workspaces" key).
	if !info.IsMonorepo {
		for _, f := range dc.Files {
			if filepath.Base(f) == "package.json" && filepath.Dir(f) == "." {
				data, err := readFile(dc.WorkTree, f)
				if err != nil {
					continue
				}
				var pkg struct {
					Workspaces any `json:"workspaces"`
				}
				if err := jsonUnmarshal(data, &pkg); err != nil {
					continue
				}
				if pkg.Workspaces != nil {
					info.IsMonorepo = true
					if info.Tool == "" {
						info.Tool = "npm workspaces"
					}
				}
				break
			}
		}
	}

	// Sub-package detection. A sub-package is any directory that
	// contains its own manifest (package.json, go.mod, Cargo.toml,
	// pubspec.yaml, pyproject.toml). The root manifest is excluded
	// by filepath.Dir check.
	manifests := []string{"package.json", "go.mod", "Cargo.toml", "pubspec.yaml", "pyproject.toml", "pom.xml", "build.gradle", "build.gradle.kts", "Gemfile", "composer.json"}
	packageDirs := make(map[string]bool)
	for _, f := range dc.Files {
		base := filepath.Base(f)
		dir := filepath.Dir(f)
		if dir == "." || dir == "" {
			continue
		}
		for _, m := range manifests {
			if base == m {
				packageDirs[dir] = true
				break
			}
		}
	}

	// Heuristic: two or more top-level subdirs with their own
	// manifest. A single "examples/" or "docs/" subdir with a
	// go.mod doesn't make it a monorepo.
	if !info.IsMonorepo {
		toplevel := make(map[string]bool)
		for dir := range packageDirs {
			top := strings.SplitN(dir, "/", 2)[0]
			toplevel[top] = true
		}
		if len(toplevel) >= 2 {
			info.IsMonorepo = true
			if info.Tool == "" {
				info.Tool = "multiple manifests"
			}
		}
	}

	if len(packageDirs) > 0 {
		info.PackageCount = len(packageDirs)
		dirs := make([]string, 0, len(packageDirs))
		for d := range packageDirs {
			dirs = append(dirs, d)
		}
		sort.Strings(dirs)
		info.Packages = dirs
	}

	return info
}
