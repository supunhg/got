// Copyright 2026 The GOT Authors. MIT License.
package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/supunhg/got/internal/events"
)

// GetStatus parses `git status --porcelain` to produce a structured Status.
func (a *ExecAdapter) GetStatus(ctx context.Context) (*Status, error) {
	// Get current branch.
	branch, _ := a.CurrentBranch(ctx)

	// Get porcelain status.
	stdout, _, err := a.run(ctx, "status", "--porcelain")
	if err != nil {
		// Non-zero exit can happen with untracked files; parse output anyway.
		// git status --porcelain exits 0 unless there's a real error.
		return nil, fmt.Errorf("status: %w", err)
	}

	lines := strings.Split(stdout, "\n")
	status := &Status{
		Branch: branch,
		Clean:  len(lines) == 0 || (len(lines) == 1 && lines[0] == ""),
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if len(line) < 3 {
			continue
		}

		xy := line[:2]
		path := strings.TrimSpace(line[2:])

		indexStatus := string(xy[0])
		worktreeStatus := string(xy[1])

		entry := StatusEntry{
			IndexStatus:    indexStatus,
			WorktreeStatus: worktreeStatus,
			Path:           path,
		}

		switch {
		case indexStatus == "?" && worktreeStatus == "?":
			// Also untracked (some versions)
			path = strings.TrimPrefix(path, "?? ")
			path = strings.TrimSpace(path)
			if path != "" {
				status.Untracked = append(status.Untracked, path)
			}

		case indexStatus != " ":
			// Staged
			// Handle renames: "R  src/old -> src/new"
			if indexStatus == "R" || indexStatus == "C" {
				parts := strings.Split(path, " -> ")
				if len(parts) == 2 {
					entry.OldPath = strings.TrimSpace(parts[0])
					entry.Path = strings.TrimSpace(parts[1])
				}
			}
			status.Staged = append(status.Staged, entry)

		case worktreeStatus != " ":
			// Unstaged
			status.Unstaged = append(status.Unstaged, entry)
		}
	}

	return status, nil
}

// CurrentBranch returns the current branch name, or "HEAD" when detached.
func (a *ExecAdapter) CurrentBranch(ctx context.Context) (string, error) {
	stdout, _, err := a.run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "HEAD", nil // detached HEAD
	}
	return strings.TrimSpace(stdout), nil
}

// ListBranches lists all local branches (and remote if requested).
func (a *ExecAdapter) ListBranches(ctx context.Context) ([]Branch, error) {
	stdout, _, err := a.run(ctx, "branch", "--format", "%(refname:short)|%(HEAD)|%(upstream:short)", "--list")
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}

	lines := strings.Split(stdout, "\n")
	var branches []Branch

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 2 {
			continue
		}

		name := parts[0]
		isCurrent := strings.TrimSpace(parts[1]) == "*"
		upstream := ""
		if len(parts) >= 3 && parts[2] != "" {
			upstream = strings.TrimSpace(parts[2])
		}

		branches = append(branches, Branch{
			Name:     name,
			Current:  isCurrent,
			Remote:   false,
			Upstream: upstream,
		})
	}

	return branches, nil
}

// CreateBranch creates a new branch at HEAD.
func (a *ExecAdapter) CreateBranch(ctx context.Context, name string) error {
	_, stderr, err := a.run(ctx, "branch", name)
	if err != nil {
		return fmt.Errorf("create branch %q: %w\n%s", name, err, stderr)
	}

	if a.bus != nil {
		_ = a.bus.Publish(ctx, events.EventBranchCreated, events.BranchCreatedPayload{
			Name:      name,
			CreatedAt: nowMS(),
		})
	}

	return nil
}

// CheckoutBranch switches to a branch.
func (a *ExecAdapter) CheckoutBranch(ctx context.Context, name string) error {
	prev, _ := a.CurrentBranch(ctx)

	_, stderr, err := a.run(ctx, "checkout", name)
	if err != nil {
		return fmt.Errorf("checkout %q: %w\n%s", name, err, stderr)
	}

	if a.bus != nil {
		_ = a.bus.Publish(ctx, events.EventBranchCheckedOut, events.BranchCheckedOutPayload{
			PreviousBranch: prev,
			NewBranch:      name,
			CheckedOutAt:   nowMS(),
		})
	}

	return nil
}

// DeleteBranch removes a branch. If force is true, uses -D.
func (a *ExecAdapter) DeleteBranch(ctx context.Context, name string, force bool) error {
	args := []string{"branch", "-d"}
	if force {
		args = []string{"branch", "-D"}
	}
	args = append(args, name)

	_, stderr, err := a.run(ctx, args...)
	if err != nil {
		return fmt.Errorf("delete branch %q: %w\n%s", name, err, stderr)
	}

	if a.bus != nil {
		_ = a.bus.Publish(ctx, events.EventBranchDeleted, events.BranchDeletedPayload{
			Name:      name,
			DeletedAt: nowMS(),
		})
	}

	return nil
}

// CreateCommit creates a commit with the given message and optional author.
// Author defaults to Git config if empty.
func (a *ExecAdapter) CreateCommit(ctx context.Context, message, author string) (string, error) {
	args := []string{"commit", "-m", message}
	if author != "" {
		args = append(args, "--author", author)
	}

	_, stderr, err := a.run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("create commit: %w\n%s", err, stderr)
	}

	// Get the commit SHA.
	sha, _, err := a.run(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("create commit: get SHA: %w", err)
	}

	if a.bus != nil {
		branch, _ := a.CurrentBranch(ctx)
		_ = a.bus.Publish(ctx, events.EventCommitCreated, events.CommitCreatedPayload{
			SHA:       sha,
			Message:   message,
			Author:    author,
			Branch:    branch,
			CreatedAt: nowMS(),
		})
	}

	return sha, nil
}

// GetCommitHistory returns commits for a branch (or all if empty).
func (a *ExecAdapter) GetCommitHistory(ctx context.Context, branch string, limit int) ([]Commit, error) {
	args := []string{"log", "--format=%H|%s|%an|%ai|%D", "--max-count=" + fmt.Sprintf("%d", limit)}
	if branch != "" {
		args = append(args, branch)
	}

	stdout, _, err := a.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("commit history: %w", err)
	}

	lines := strings.Split(stdout, "\n")
	var commits []Commit

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 4 {
			continue
		}

		c := Commit{
			SHA:     parts[0],
			Message: parts[1],
			Author:  parts[2],
			Date:    parts[3],
		}
		if len(parts) >= 5 {
			c.Refs = parts[4]
		}

		commits = append(commits, c)
	}

	return commits, nil
}
