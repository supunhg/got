package health

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// docFileRules is the list of documentation files the health
// engine expects. Each rule has a set of "any-of" basenames —
// e.g. README can be "README.md", "README.rst", "README.txt",
// or plain "README" (case-insensitive).
//
// The list is intentionally conservative: a v0.1 health report
// shouldn't ding a project for not having a CONTRIBUTING.md
// for *every* missing-docs category; we only fire on the
// highest-signal files (README, LICENSE, CHANGELOG,
// CONTRIBUTING).
var docFileRules = []struct {
	ID        string
	Title     string
	Detail    string
	Severity  HealthSeverity
	Basenames []string
	// Dir restricts the search to a particular directory
	// (relative to the work tree). Empty means "anywhere".
	Dir string
}{
	{
		ID:       "missing-readme",
		Title:    "No README found",
		Detail:   "A top-level README explains what the project does and how to get started. Most open-source repositories are expected to have one.",
		Severity: SeverityCritical,
		Basenames: []string{
			"README", "README.md", "README.rst", "README.txt", "README.adoc",
			"readme", "readme.md",
		},
	},
	{
		ID:       "missing-license",
		Title:    "No LICENSE found",
		Detail:   "Without a license, others cannot legally reuse your code. Add a LICENSE file at the work-tree root (MIT, Apache-2.0, GPL-3.0, etc.).",
		Severity: SeverityHigh,
		Basenames: []string{
			"LICENSE", "LICENSE.md", "LICENSE.txt", "License", "license", "license.md",
			"COPYING", "copying",
		},
	},
	{
		ID:       "missing-changelog",
		Title:    "No CHANGELOG found",
		Detail:   "A CHANGELOG helps users understand what changed between releases. Start a CHANGELOG.md (Keep a Changelog format recommended).",
		Severity: SeverityMedium,
		Basenames: []string{
			"CHANGELOG", "CHANGELOG.md", "CHANGELOG.rst", "CHANGELOG.txt",
			"HISTORY", "HISTORY.md", "NEWS", "NEWS.md",
			"changelog", "changelog.md", "history.md", "news.md",
		},
	},
	{
		ID:       "missing-contributing",
		Title:    "No CONTRIBUTING guide",
		Detail:   "A CONTRIBUTING.md tells new contributors how to get started. It significantly reduces the friction of receiving contributions.",
		Severity: SeverityLow,
		Basenames: []string{
			"CONTRIBUTING", "CONTRIBUTING.md", "CONTRIBUTING.rst",
			"contributing", "contributing.md",
		},
	},
}

// checkDocumentation scans the work tree for the four canonical
// documentation files. Each missing file becomes a finding.
// The doc-engine does not recurse: docs/CHANGELOG.md does NOT
// count as "CHANGELOG at the work tree root", because v0.1
// users expect a top-level file.
func checkDocumentation(c *Checker) ([]HealthFinding, error) {
	if c.WorkTree == "" {
		return nil, nil
	}
	// Build a basename → bool index for O(1) lookup.
	// We do the indexing by walking the work tree once.
	index := make(map[string]bool)
	walkErr := filepath.WalkDir(c.WorkTree, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Match analyzer's skip list at the top level
			// only — the docs engine is meant to find
			// files at the work-tree root. We don't want
			// .git/CONTRIBUTING.md to count.
			if path != c.WorkTree {
				base := filepath.Base(path)
				switch base {
				case ".git", ".got", "node_modules", "vendor", "target", "dist", "build", "out", "coverage", ".idea", ".vscode":
					return filepath.SkipDir
				}
			}
			return nil
		}
		// Only index the top level + a few well-known
		// subdirectories (docs/, .github/).
		rel, err := filepath.Rel(c.WorkTree, path)
		if err != nil {
			return nil
		}
		depth := strings.Count(filepath.ToSlash(rel), "/")
		if depth > 1 {
			return nil
		}
		base := filepath.Base(path)
		index[strings.ToLower(base)] = true
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	var findings []HealthFinding
	for _, rule := range docFileRules {
		found := false
		for _, name := range rule.Basenames {
			if index[strings.ToLower(name)] {
				found = true
				break
			}
		}
		if found {
			continue
		}
		findings = append(findings, HealthFinding{
			ID:       rule.ID,
			Category: CategoryDocs,
			Severity: rule.Severity,
			Title:    rule.Title,
			Detail:   rule.Detail,
		})
	}
	sortFindings(findings)
	return findings, nil
}
