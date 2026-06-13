package health

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/got-sh/got/internal/git"
)

// checkStaleBranches flags local branches whose last commit is
// older than Thresholds.StaleDays. The default branch (HEAD's
// upstream / project's default branch) is excluded — a stale
// default branch is a different problem the user can address
// via `got branch`.
//
// A branch is also excluded when it is the current branch —
// "your own branch is stale" is rarely actionable.
func checkStaleBranches(ctx context.Context, c *Checker) ([]HealthFinding, error) {
	if c.Adapter == nil {
		return nil, nil
	}
	branches, err := c.Adapter.Branches(ctx)
	if err != nil {
		return nil, err
	}
	if len(branches) == 0 {
		return nil, nil
	}
	defaultBranch := defaultBranchFor(c)
	cutoff := c.Now().AddDate(0, 0, -c.Thresholds.StaleDays)
	var affected []string
	for _, b := range branches {
		if b.IsCurrent {
			continue
		}
		if b.Name == defaultBranch {
			continue
		}
		if b.CommitAt.IsZero() {
			continue
		}
		if b.CommitAt.Before(cutoff) {
			affected = append(affected, b.Name)
		}
	}
	if len(affected) == 0 {
		return nil, nil
	}
	sort.Strings(affected)
	return []HealthFinding{{
		ID:       "stale-branches",
		Category: CategoryBranches,
		Severity: severityForCount(len(affected), 1, 3, 10),
		Title:    fmt.Sprintf("%d stale branch(es)", len(affected)),
		Detail:   fmt.Sprintf("Branches with no commits in the last %d days (excluding %s and the current branch):", c.Thresholds.StaleDays, defaultBranch),
		Affected: affected,
	}}, nil
}

// checkMergedBranches flags local branches whose tip is an
// ancestor of the default branch. Such branches are fully
// merged and can be deleted with `git branch -d`.
//
// We use the default branch as the merge base. If we can't
// determine the default branch we fall back to the first
// branch whose IsCurrent is true (HEAD), then to the first
// non-remote branch with a SHA.
//
// The check does its own ancestor computation by walking the
// Log of the default branch and matching branch tips against
// the resulting commit set. This is O(branches * commits) but
// matches the adapter's existing surface area — adding a new
// `IsAncestor` method on the adapter is a v0.2+ change.
func checkMergedBranches(ctx context.Context, c *Checker) ([]HealthFinding, error) {
	if c.Adapter == nil {
		return nil, nil
	}
	branches, err := c.Adapter.Branches(ctx)
	if err != nil {
		return nil, err
	}
	if len(branches) < 2 {
		return nil, nil
	}
	defaultBranch := defaultBranchFor(c)
	// Build a set of SHAs reachable from the default branch.
	reachable, err := reachableSHAs(ctx, c.Adapter, defaultBranch)
	if err != nil {
		// Fall back: if we can't read the default branch's log
		// (e.g. the branch doesn't exist locally), skip this
		// check rather than fail the whole report.
		return nil, nil
	}
	var affected []string
	for _, b := range branches {
		if b.IsCurrent {
			continue
		}
		if b.Name == defaultBranch {
			continue
		}
		if b.SHA == "" {
			continue
		}
		if _, ok := reachable[b.SHA]; ok {
			affected = append(affected, b.Name)
		}
	}
	if len(affected) == 0 {
		return nil, nil
	}
	sort.Strings(affected)
	return []HealthFinding{{
		ID:       "merged-branches",
		Category: CategoryBranches,
		Severity: severityForCount(len(affected), 1, 5, 15),
		Title:    fmt.Sprintf("%d merged branch(es) not deleted", len(affected)),
		Detail:   fmt.Sprintf("The following branches are fully merged into %s and can be deleted with `git branch -d`:", defaultBranch),
		Affected: affected,
	}}, nil
}

// checkExcessiveBranches fires when the local branch count
// exceeds Thresholds.MaxBranches. The intent is to nudge users
// with truly overgrown branch lists (50+) toward a cleanup
// pass; the check does not produce findings for the median
// repo with 5-20 branches.
func checkExcessiveBranches(ctx context.Context, c *Checker) ([]HealthFinding, error) {
	if c.Adapter == nil {
		return nil, nil
	}
	branches, err := c.Adapter.Branches(ctx)
	if err != nil {
		return nil, err
	}
	if len(branches) <= c.Thresholds.MaxBranches {
		return nil, nil
	}
	return []HealthFinding{{
		ID:       "excessive-branches",
		Category: CategoryBranches,
		Severity: severityForCount(len(branches), c.Thresholds.MaxBranches+1, c.Thresholds.MaxBranches*2, c.Thresholds.MaxBranches*4),
		Title:    fmt.Sprintf("%d local branches", len(branches)),
		Detail:   fmt.Sprintf("This repository has more than %d local branches. Most are likely stale or merged; prune them in batches of 5-10 to keep the review manageable.", c.Thresholds.MaxBranches),
	}}, nil
}

// reachableSHAs returns the set of commit SHAs reachable from
// the given ref. An empty ref means "HEAD". Used by
// checkMergedBranches to determine whether a branch's tip is
// in the default branch's ancestry.
func reachableSHAs(ctx context.Context, a git.Adapter, ref string) (map[string]struct{}, error) {
	if a == nil {
		return nil, fmt.Errorf("nil adapter")
	}
	if ref == "" {
		ref = "HEAD"
	}
	rdr, err := a.Log(ctx, ref, git.LogFormatNDJSON)
	if err != nil {
		return nil, err
	}
	defer func() {
		if c, ok := rdr.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	}()
	_ = rdr // silence unused warning when scanning is inlined below
	return scanCommitSHAs(rdr)
}

// defaultBranchFor returns the project's default branch name.
// The lookup order is:
//
//  1. The current branch (HEAD), when the current branch is
//     one of "main" / "master" / "trunk" / "develop".
//  2. The first local branch whose name is in the canonical set.
//  3. "main" as a last-resort default.
//
// We intentionally do not parse got.yml here — the health
// engine is decoupled from the project config. The result is
// "what is most likely the default branch" rather than "what
// the project declares", which is the right call for a
// generic health check.
func defaultBranchFor(c *Checker) string {
	canonical := map[string]bool{
		"main": true, "master": true, "trunk": true, "develop": true, "default": true,
	}
	// 1. Current branch, if canonical.
	if c.Adapter == nil {
		return "main"
	}
	ctx := context.Background()
	branches, err := c.Adapter.Branches(ctx)
	if err == nil {
		for _, b := range branches {
			if b.IsCurrent && canonical[b.Name] {
				return b.Name
			}
		}
		for _, b := range branches {
			if canonical[b.Name] {
				return b.Name
			}
		}
	}
	return "main"
}

// severityForCount maps a count to a severity using three
// thresholds (low, medium, high). The mapping is intentionally
// simple: the user-facing text includes the count, so the
// severity just needs to communicate "this is big / small".
func severityForCount(n, low, med, high int) HealthSeverity {
	switch {
	case n >= high:
		return SeverityHigh
	case n >= med:
		return SeverityMedium
	case n >= low:
		return SeverityLow
	}
	return SeverityInfo
}

// earliestCommitTime returns the earliest CommitAt across a
// branch list. Used in tests.
func earliestCommitTime(in []git.Branch) time.Time {
	var out time.Time
	for _, b := range in {
		if b.CommitAt.IsZero() {
			continue
		}
		if out.IsZero() || b.CommitAt.Before(out) {
			out = b.CommitAt
		}
	}
	return out
}
