package analyzer

import (
	"path/filepath"
	"sort"
)

// detectCICD scans for CI/CD configuration files and reports the
// system + provider + workflow count.
//
// The detection is filename-based for the most part: the world's
// CI/CD systems all use a small set of filenames in well-known
// locations. We avoid parsing the file contents because the
// schemas differ wildly and a v0.1 inspect just needs the
// "what system is this?" signal.
func detectCICD(dc DetectionContext) []CICDSystem {
	type rule struct {
		// dir is the directory pattern to look in ("" = anywhere).
		dir string
		// name is the file basename.
		name string
		// system is the human-readable name.
		system string
		// provider is the provider identifier.
		provider string
	}
	rules := []rule{
		{dir: ".github/workflows", name: "*.yml", system: "GitHub Actions", provider: "github"},
		{dir: ".github/workflows", name: "*.yaml", system: "GitHub Actions", provider: "github"},
		{dir: ".gitlab", name: "*.yml", system: "GitLab CI", provider: "gitlab"},
		{dir: ".circleci", name: "config.yml", system: "CircleCI", provider: "circleci"},
		{dir: ".circleci", name: "config.yaml", system: "CircleCI", provider: "circleci"},
		{dir: ".travis.yml", name: "", system: "Travis CI", provider: "travis"},
		{dir: ".gitlab-ci.yml", name: "", system: "GitLab CI", provider: "gitlab"},
		{dir: ".gitlab-ci.yaml", name: "", system: "GitLab CI", provider: "gitlab"},
		{dir: ".drone.yml", name: "", system: "Drone CI", provider: "drone"},
		{dir: ".drone.yaml", name: "", system: "Drone CI", provider: "drone"},
		{dir: "azure-pipelines.yml", name: "", system: "Azure Pipelines", provider: "azure"},
		{dir: "azure-pipelines.yaml", name: "", system: "Azure Pipelines", provider: "azure"},
		{dir: "bitbucket-pipelines.yml", name: "", system: "Bitbucket Pipelines", provider: "bitbucket"},
		{dir: "buildkite.yml", name: "", system: "Buildkite", provider: "buildkite"},
		{dir: "appveyor.yml", name: "", system: "AppVeyor", provider: "appveyor"},
		{dir: "Jenkinsfile", name: "", system: "Jenkins", provider: "jenkins"},
		{dir: ".woodpecker.yml", name: "", system: "Woodpecker CI", provider: "woodpecker"},
		{dir: "tekton", name: "*.yaml", system: "Tekton", provider: "tekton"},
		{dir: "tekton", name: "*.yml", system: "Tekton", provider: "tekton"},
	}

	// Group by system so we can build one CICDSystem per provider.
	systemFiles := make(map[string]*cicdAccum)
	for _, f := range dc.Files {
		dir := filepath.ToSlash(filepath.Dir(f))
		base := filepath.Base(f)
		for _, r := range rules {
			match := false
			if r.dir != "" {
				if r.name == "" {
					// r.dir is a full file path (root-level
					// file like ".travis.yml"). Match by
					// comparing the full path.
					match = f == r.dir
				} else {
					// r.dir is a directory, r.name is a
					// glob pattern. Match when the file
					// lives in r.dir and its basename
					// matches the glob.
					if dir == r.dir || hasPathPrefix(dir, r.dir+"/") {
						if hasGlobMatch(base, r.name) {
							match = true
						}
					}
				}
			} else {
				// r.dir is empty: match anywhere by name.
				if r.name == "" {
					match = false
				} else if hasGlobMatch(base, r.name) {
					match = true
				}
			}
			if !match {
				continue
			}
			if systemFiles[r.system] == nil {
				systemFiles[r.system] = &cicdAccum{
					Name:     r.system,
					Provider: r.provider,
				}
			}
			acc := systemFiles[r.system]
			acc.Files = append(acc.Files, f)
		}
	}

	out := make([]CICDSystem, 0, len(systemFiles))
	for _, acc := range systemFiles {
		sort.Strings(acc.Files)
		acc.Files = uniqueStrings(acc.Files)
		out = append(out, CICDSystem{
			Name:          acc.Name,
			Provider:      acc.Provider,
			ConfigFiles:   acc.Files,
			WorkflowCount: len(acc.Files),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

type cicdAccum struct {
	Name     string
	Provider string
	Files    []string
}

// hasGlobMatch reports whether s matches a simple glob pattern
// with a single "*" wildcard. Patterns with ** or character
// classes are not supported; this is enough for "*.yml" /
// "*.yaml".
func hasGlobMatch(s, pattern string) bool {
	if pattern == "*" {
		return true
	}
	star := -1
	for i, r := range pattern {
		if r == '*' {
			star = i
			break
		}
	}
	if star < 0 {
		return s == pattern
	}
	prefix := pattern[:star]
	suffix := pattern[star+1:]
	if len(s) < len(prefix)+len(suffix) {
		return false
	}
	if !hasPrefix(s, prefix) {
		return false
	}
	return hasSuffix(s, suffix)
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func hasPathPrefix(s, prefix string) bool {
	return hasPrefix(s, prefix)
}
