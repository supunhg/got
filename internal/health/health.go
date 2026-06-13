// Package health implements GOT's repository health engine
// (ARCHITECTURE.md §"Health Engine"). It analyzes a repository
// for common smells: stale branches, merged branches that were
// never deleted, unreachable remotes, large binaries checked
// into Git, missing documentation, excessive branch count, and
// working-tree cleanliness.
//
// The package is read-only and offline-only: no network calls,
// no mutations. It uses the git.Adapter for branch/remote/log
// data and the local file system for binary / documentation
// detection.
//
// A Checker is constructed with New and produces a HealthReport
// via Check. Each check is independent; an error in one check
// does not abort the others. Findings are de-duplicated by ID
// so a single source of badness (e.g. a binary blob in
// db/seed.png) is reported once, not once per check.
package health

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/got-sh/got/internal/git"
)

// HealthReport is the full output of a health check.
type HealthReport struct {
	// RepositoryPath is the absolute path to the work tree that
	// was checked.
	RepositoryPath string `json:"repositoryPath"`
	// GeneratedAt is the time the report was produced.
	GeneratedAt time.Time `json:"generatedAt"`
	// Score is the overall health score on a 0-100 scale.
	// 100 = pristine, 0 = every check failed.
	Score int `json:"score"`
	// Grade is a letter grade: "A" (90+), "B" (80+), "C" (70+),
	// "D" (60+), "F" (below). "N/A" when there are no findings
	// and the score is 100.
	Grade string `json:"grade"`
	// Findings is the list of problems the checker found,
	// ordered by severity (critical → high → medium → low → info)
	// and then by ID for stability.
	Findings []HealthFinding `json:"findings"`
	// Recommendations is the list of actionable next steps
	// the user can take to improve the score, ordered by
	// Priority (1 = most important).
	Recommendations []Recommendation `json:"recommendations"`
	// Counts is a per-severity summary of the findings.
	Counts HealthCounts `json:"counts"`
	// ChecksRun is the list of check IDs that ran. Useful for
	// debugging ("did this report include the binary check?")
	// and for tests that want to assert on coverage.
	ChecksRun []string `json:"checksRun"`
}

// HealthFinding is a single problem the checker found.
type HealthFinding struct {
	// ID is a stable identifier for the finding
	// (e.g. "stale-branches", "unreachable-remotes").
	ID string `json:"id"`
	// Category groups the finding (branches, remotes, files,
	// documentation, cleanliness).
	Category HealthCategory `json:"category"`
	// Severity rates the finding: info, low, medium, high,
	// or critical.
	Severity HealthSeverity `json:"severity"`
	// Title is a one-line description of the problem.
	Title string `json:"title"`
	// Detail expands on the title with specifics (counts,
	// examples, thresholds).
	Detail string `json:"detail"`
	// Affected is the list of items the finding applies to
	// (branch names, file paths, remote names, etc.).
	// Empty when the finding is repo-wide.
	Affected []string `json:"affected,omitempty"`
}

// HealthCategory groups related findings.
type HealthCategory string

const (
	// CategoryBranches covers branch-related issues
	// (stale branches, merged branches, branch count).
	CategoryBranches HealthCategory = "branches"
	// CategoryRemotes covers remote-related issues
	// (unreachable / unused remotes).
	CategoryRemotes HealthCategory = "remotes"
	// CategoryFiles covers file-related issues
	// (large binaries checked into Git).
	CategoryFiles HealthCategory = "files"
	// CategoryDocs covers documentation issues
	// (missing README, LICENSE, CHANGELOG).
	CategoryDocs HealthCategory = "documentation"
	// CategoryCleanliness covers working-tree state
	// (uncommitted changes, untracked files).
	CategoryCleanliness HealthCategory = "cleanliness"
)

// HealthSeverity rates a finding.
type HealthSeverity string

const (
	// SeverityInfo is a neutral observation. Does not lower
	// the score.
	SeverityInfo HealthSeverity = "info"
	// SeverityLow is a minor smell (e.g. one stale branch).
	// Small score impact.
	SeverityLow HealthSeverity = "low"
	// SeverityMedium is a notable problem (e.g. several
	// stale branches, missing CHANGELOG).
	SeverityMedium HealthSeverity = "medium"
	// SeverityHigh is a significant problem (e.g. 50+ stale
	// branches, several unreachable remotes, large binary
	// checked in).
	SeverityHigh HealthSeverity = "high"
	// SeverityCritical is a serious problem (e.g. no README,
	// dozens of large binaries, working tree in a broken
	// state).
	SeverityCritical HealthSeverity = "critical"
)

// Recommendation is a single actionable next step.
type Recommendation struct {
	// Priority is the recommendation's rank (1 = most important).
	Priority int `json:"priority"`
	// Title is a one-line description.
	Title string `json:"title"`
	// Action is a more detailed description of what to do.
	Action string `json:"action"`
	// Command is a `got` or `git` command the user can run
	// (displayed verbatim, not executed). May be empty when
	// the recommendation is a manual change.
	Command string `json:"command,omitempty"`
	// FindingID links the recommendation to the finding(s)
	// it would resolve. Empty when the recommendation is
	// repo-wide.
	FindingID string `json:"findingId,omitempty"`
}

// HealthCounts is the per-severity summary.
type HealthCounts struct {
	Info     int `json:"info"`
	Low      int `json:"low"`
	Medium   int `json:"medium"`
	High     int `json:"high"`
	Critical int `json:"critical"`
}

// Thresholds groups the configurable knobs for the health
// checks. The zero value uses the v0.1 defaults; tests can
// override individual fields to exercise edge cases.
type Thresholds struct {
	// StaleDays is the age in days at which a branch counts
	// as stale. Default: 180.
	StaleDays int
	// MaxBranches is the local branch count above which the
	// "excessive branch count" finding fires. Default: 50.
	MaxBranches int
	// LargeBinaryBytes is the size in bytes above which a file
	// counts as a "large binary" (when also matched by the
	// binary-extension list). Default: 1 MiB.
	LargeBinaryBytes int64
	// MaxLargeBinaries is the count above which the
	// large-binaries finding escalates from "low" to "medium"
	// to "high". Default: 3 / 10 / 30.
	MaxLargeBinaries struct {
		Low    int
		Medium int
		High   int
	}
}

// defaultThresholds returns the v0.1 defaults. The values are
// chosen to be conservative: they should fire on clearly-bad
// repositories and stay silent on the median repo. Tuning
// happens via Thresholds, not by changing these numbers.
func defaultThresholds() Thresholds {
	t := Thresholds{
		StaleDays:        180,
		MaxBranches:      50,
		LargeBinaryBytes: 1 << 20, // 1 MiB
	}
	t.MaxLargeBinaries.Low = 1
	t.MaxLargeBinaries.Medium = 3
	t.MaxLargeBinaries.High = 10
	return t
}

// Checker is the main entry point. Construct one with New,
// optionally override Thresholds, then call Check.
type Checker struct {
	WorkTree   string
	Adapter    git.Adapter
	Thresholds Thresholds
	// Now returns the current time. Tests override this so
	// "stale" checks produce stable results.
	Now func() time.Time
}

// New returns a Checker with the v0.1 default thresholds.
func New(workTree string, adapter git.Adapter) *Checker {
	return &Checker{
		WorkTree:   workTree,
		Adapter:    adapter,
		Thresholds: defaultThresholds(),
		Now:        time.Now,
	}
}

// Check runs every health check and returns the assembled report.
// The function never returns an error for a partial failure —
// each check is independent and any check-level error is
// converted to a Severity=Info finding with the error in Detail.
// A non-nil error is reserved for catastrophic failures (e.g. an
// unreachable work tree).
func (c *Checker) Check(ctx context.Context) (HealthReport, error) {
	if c == nil {
		return HealthReport{}, fmt.Errorf("health: nil receiver")
	}
	if c.WorkTree == "" {
		return HealthReport{}, fmt.Errorf("health: empty work tree path")
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Thresholds.StaleDays == 0 {
		c.Thresholds = defaultThresholds()
	}

	r := HealthReport{
		RepositoryPath: c.WorkTree,
		GeneratedAt:    c.Now().UTC(),
	}

	// Stale branches.
	if findings, err := checkStaleBranches(ctx, c); err == nil {
		r.Findings = append(r.Findings, findings...)
		r.ChecksRun = append(r.ChecksRun, "stale-branches")
	} else {
		r.Findings = append(r.Findings, errFinding("stale-branches", CategoryBranches, err))
	}

	// Merged branches not deleted.
	if findings, err := checkMergedBranches(ctx, c); err == nil {
		r.Findings = append(r.Findings, findings...)
		r.ChecksRun = append(r.ChecksRun, "merged-branches")
	} else {
		r.Findings = append(r.Findings, errFinding("merged-branches", CategoryBranches, err))
	}

	// Excessive branch count.
	if findings, err := checkExcessiveBranches(ctx, c); err == nil {
		r.Findings = append(r.Findings, findings...)
		r.ChecksRun = append(r.ChecksRun, "excessive-branches")
	} else {
		r.Findings = append(r.Findings, errFinding("excessive-branches", CategoryBranches, err))
	}

	// Unreachable remotes.
	if findings, err := checkRemotes(ctx, c); err == nil {
		r.Findings = append(r.Findings, findings...)
		r.ChecksRun = append(r.ChecksRun, "unreachable-remotes")
	} else {
		r.Findings = append(r.Findings, errFinding("unreachable-remotes", CategoryRemotes, err))
	}

	// Large binaries.
	if findings, err := checkLargeBinaries(c); err == nil {
		r.Findings = append(r.Findings, findings...)
		r.ChecksRun = append(r.ChecksRun, "large-binaries")
	} else {
		r.Findings = append(r.Findings, errFinding("large-binaries", CategoryFiles, err))
	}

	// Missing documentation.
	if findings, err := checkDocumentation(c); err == nil {
		r.Findings = append(r.Findings, findings...)
		r.ChecksRun = append(r.ChecksRun, "missing-docs")
	} else {
		r.Findings = append(r.Findings, errFinding("missing-docs", CategoryDocs, err))
	}

	// Working-tree cleanliness.
	if findings, err := checkCleanliness(ctx, c); err == nil {
		r.Findings = append(r.Findings, findings...)
		r.ChecksRun = append(r.ChecksRun, "cleanliness")
	} else {
		r.Findings = append(r.Findings, errFinding("cleanliness", CategoryCleanliness, err))
	}

	// Sort findings: by severity desc, then by ID for stability.
	sortFindings(r.Findings)
	// Count severities.
	for _, f := range r.Findings {
		switch f.Severity {
		case SeverityInfo:
			r.Counts.Info++
		case SeverityLow:
			r.Counts.Low++
		case SeverityMedium:
			r.Counts.Medium++
		case SeverityHigh:
			r.Counts.High++
		case SeverityCritical:
			r.Counts.Critical++
		}
	}
	// Compute score + grade.
	r.Score = scoreFromFindings(r.Findings)
	r.Grade = gradeForScore(r.Score, r.Counts)
	// Build recommendations from findings.
	r.Recommendations = recommendationsFromFindings(r.Findings, c)
	sortRecommendations(r.Recommendations)
	return r, nil
}

// errFinding builds a Severity=Info finding that documents a
// failed check. The finding's title includes "check failed" so
// the user can tell it apart from a real problem.
func errFinding(id string, cat HealthCategory, err error) HealthFinding {
	return HealthFinding{
		ID:       id,
		Category: cat,
		Severity: SeverityInfo,
		Title:    id + " check failed",
		Detail:   "the " + id + " check could not run: " + err.Error(),
	}
}

// sortFindings orders findings by severity (critical first)
// then by ID for stability.
func sortFindings(in []HealthFinding) {
	sort.SliceStable(in, func(i, j int) bool {
		ri := severityRank(in[i].Severity)
		rj := severityRank(in[j].Severity)
		if ri != rj {
			return ri > rj
		}
		return in[i].ID < in[j].ID
	})
}

// severityRank returns a sortable rank (higher = more severe).
func severityRank(s HealthSeverity) int {
	switch s {
	case SeverityCritical:
		return 5
	case SeverityHigh:
		return 4
	case SeverityMedium:
		return 3
	case SeverityLow:
		return 2
	case SeverityInfo:
		return 1
	}
	return 0
}

// scoreFromFindings returns a 0-100 score. The base is 100 and
// each finding deducts points proportional to its severity:
//
//	critical: 25
//	high:     10
//	medium:   4
//	low:      1
//	info:     0
//
// A perfect repo (no findings) scores 100. Severely broken
// repos can drop below 0, which we clamp to 0 in the caller.
func scoreFromFindings(findings []HealthFinding) int {
	score := 100
	for _, f := range findings {
		switch f.Severity {
		case SeverityCritical:
			score -= 25
		case SeverityHigh:
			score -= 10
		case SeverityMedium:
			score -= 4
		case SeverityLow:
			score -= 1
		}
	}
	if score < 0 {
		score = 0
	}
	return score
}

// gradeForScore maps a 0-100 score to a letter grade. We use
// the same thresholds as a typical school report card.
func gradeForScore(score int, counts HealthCounts) string {
	if counts.Critical+counts.High+counts.Medium+counts.Low == 0 {
		return "A+"
	}
	switch {
	case score >= 90:
		return "A"
	case score >= 80:
		return "B"
	case score >= 70:
		return "C"
	case score >= 60:
		return "D"
	}
	return "F"
}

// recommendationsFromFindings turns findings into prioritized
// recommendations. The Priority is a stable rank:
//
//	1  = critical
//	2  = high
//	3  = medium
//	4  = low
//
// Info findings do not produce recommendations.
func recommendationsFromFindings(findings []HealthFinding, c *Checker) []Recommendation {
	out := []Recommendation{}
	for _, f := range findings {
		var rec Recommendation
		rec.FindingID = f.ID
		switch f.Severity {
		case SeverityCritical:
			rec.Priority = 1
		case SeverityHigh:
			rec.Priority = 2
		case SeverityMedium:
			rec.Priority = 3
		case SeverityLow:
			rec.Priority = 4
		default:
			continue
		}
		rec.Title, rec.Action, rec.Command = recommendationFor(f, c)
		if rec.Title == "" {
			continue
		}
		out = append(out, rec)
	}
	return out
}

// recommendationFor returns the title/action/command for a
// single finding. The mapping is hand-written per ID — there
// are only a handful of findings, and the recommendation text
// is the most user-visible part of the report.
func recommendationFor(f HealthFinding, c *Checker) (title, action, command string) {
	switch f.ID {
	case "stale-branches":
		title = "Delete or merge stale branches"
		action = fmt.Sprintf("The following branches have had no commits in %d days and can probably be deleted:", c.Thresholds.StaleDays)
		if n := len(f.Affected); n > 0 && n <= 5 {
			command = "git branch -d " + strings.Join(f.Affected, " ")
		} else if n > 5 {
			command = "git branch | xargs git branch -d  # edit the list first"
		}
		return
	case "merged-branches":
		title = "Delete merged branches"
		action = "The following branches are fully merged into the default branch and can be deleted safely:"
		if n := len(f.Affected); n > 0 && n <= 5 {
			command = "git branch -d " + strings.Join(f.Affected, " ")
		} else if n > 5 {
			command = "git branch --merged main | xargs git branch -d"
		}
		return
	case "excessive-branches":
		title = "Tidy up local branches"
		action = fmt.Sprintf("This repository has more than %d local branches. Most are likely stale or merged; prune them in batches.", c.Thresholds.MaxBranches)
		command = "git branch --list | head -20  # review the list, then delete stale ones"
		return
	case "unreachable-remotes":
		title = "Remove or fix unreachable remotes"
		action = "The following remotes appear to be unused or unreachable. Verify each is still needed, then `git remote remove` the ones that aren't:"
		if n := len(f.Affected); n > 0 && n <= 3 {
			command = "git remote remove " + strings.Join(f.Affected, " ")
		}
		return
	case "large-binaries":
		title = "Move large binaries out of the repo"
		action = "Large binary files inflate the clone size and slow every Git operation. Move them to object storage, an LFS server, or a release artifact:"
		command = "git lfs migrate  # or: remove the file and rewrite history (destructive)"
		return
	case "missing-readme":
		title = "Add a README"
		action = "A README is the front door of your repository. Add a top-level README.md that explains what the project does and how to get started."
		command = "echo '# " + c.WorkTree + "' > README.md"
		return
	case "missing-license":
		title = "Add a LICENSE"
		action = "Open-source projects without a license cannot be legally reused by others. Add a LICENSE file (MIT, Apache-2.0, GPL-3.0, etc.) at the work-tree root."
		return
	case "missing-changelog":
		title = "Add a CHANGELOG"
		action = "A CHANGELOG helps users and contributors understand what changed between releases. Adopt `Keep a Changelog` (https://keepachangelog.com) and start a CHANGELOG.md."
		return
	case "missing-contributing":
		title = "Add a CONTRIBUTING guide"
		action = "A CONTRIBUTING.md tells new contributors how to get started (dev setup, test suite, PR process). It significantly reduces the friction of receiving contributions."
		return
	case "working-tree-dirty":
		title = "Commit or stash working-tree changes"
		action = "Your working tree has uncommitted changes. Either commit them to a feature branch, or stash them for later:"
		command = "git status  # then either `got commit` or `git stash`"
		return
	case "untracked-files":
		title = "Add or .gitignore untracked files"
		action = "Untracked files pile up over time. Either commit them (when they're meant to be in the repo) or add them to .gitignore:"
		command = "git status --short  # review, then either `git add` or edit .gitignore"
		return
	}
	// Default: echo the finding's title as a recommendation.
	return f.Title, f.Detail, ""
}

// sortRecommendations orders recommendations by Priority asc,
// then by Title for stability.
func sortRecommendations(in []Recommendation) {
	sort.SliceStable(in, func(i, j int) bool {
		if in[i].Priority != in[j].Priority {
			return in[i].Priority < in[j].Priority
		}
		return in[i].Title < in[j].Title
	})
}
