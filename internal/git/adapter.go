// Package git implements a thin Git adapter that shells out to the `git`
// CLI via os/exec. Every operation is repository-scoped (no global state)
// and can optionally publish events through the provided Event Bus.
//
// Copyright 2026 The GOT Authors. MIT License.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/supunhg/got/internal/events"
)

// ── Domain types ────────────────────────────────────────────────────

// StatusEntry represents a single file in the working tree status.
type StatusEntry struct {
	IndexStatus  string `json:"index_status"`  // staged change (M, A, D, R, C, or space)
	WorktreeStatus string `json:"worktree_status"` // unstaged change (M, A, D, or space)
	Path         string `json:"path"`
	OldPath      string `json:"old_path,omitempty"` // for renames/copies
}

// Status holds the full working tree status.
type Status struct {
	Branch     string        `json:"branch"`
	Clean      bool          `json:"clean"`
	Staged     []StatusEntry `json:"staged"`
	Unstaged   []StatusEntry `json:"unstaged"`
	Untracked  []string      `json:"untracked"`
}

// Commit represents a single commit in the history.
type Commit struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Refs    string `json:"refs,omitempty"` // branch/tag decorations
}

// Branch represents a Git branch.
type Branch struct {
	Name     string `json:"name"`
	Current  bool   `json:"current"`
	Remote   bool   `json:"remote"`
	SHA      string `json:"sha,omitempty"`
	Upstream string `json:"upstream,omitempty"`
}

// Remote represents a Git remote.
type Remote struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	PushURL string `json:"push_url,omitempty"`
}

// GraphNode represents a single node in the commit graph.
type GraphNode struct {
	SHA     string   `json:"sha"`
	Message string   `json:"message"`
	Parents []string `json:"parents"`
	Refs    string   `json:"refs,omitempty"`
}

// PushResult holds the output of a push operation.
type PushResult struct {
	Output string `json:"output"`
}

// PullResult holds the output of a pull operation.
type PullResult struct {
	Output      string `json:"output"`
	FastForward bool   `json:"fast_forward"`
}

// ── GitAdapter interface ────────────────────────────────────────────

// GitAdapter provides repository-scoped Git operations. Every method
// takes a context for cancellation and returns typed Go values — callers
// never parse raw git output.
type GitAdapter interface {
	// OpenRepository re-initializes the adapter for the given repo path.
	// Must be called before any other operation.
	OpenRepository(ctx context.Context, path string) error

	// Root returns the repository root path.
	Root() string

	// Status returns the working tree status.
	GetStatus(ctx context.Context) (*Status, error)

	// Commit creates a commit with the given message (and optional author).
	// Author defaults to the Git config user.name / user.email if empty.
	CreateCommit(ctx context.Context, message, author string) (string, error)

	// GetCommitHistory returns commit history for a branch (or all if empty).
	GetCommitHistory(ctx context.Context, branch string, limit int) ([]Commit, error)

	// Branch operations.
	ListBranches(ctx context.Context) ([]Branch, error)
	CreateBranch(ctx context.Context, name string) error
	CheckoutBranch(ctx context.Context, name string) error
	DeleteBranch(ctx context.Context, name string, force bool) error
	CurrentBranch(ctx context.Context) (string, error)

	// Remote operations.
	GetRemotes(ctx context.Context) ([]Remote, error)
	AddRemote(ctx context.Context, name, url string) error
	RemoveRemote(ctx context.Context, name string) error
	Push(ctx context.Context, remote, branch string, force bool) (*PushResult, error)
	Pull(ctx context.Context, remote, branch string) (*PullResult, error)

	// Graph returns the commit graph structure for visualisation.
	GetGraph(ctx context.Context, branch string, maxCount int) ([]GraphNode, error)
}

// ── ExecAdapter ─────────────────────────────────────────────────────

// ExecAdapter shells out to the `git` CLI. It is repository-scoped:
// the repo path is set at construction or via OpenRepository.
type ExecAdapter struct {
	repoPath string
	bus      *events.Bus
}

// NewExecAdapter creates a Git adapter backed by the `git` CLI. The bus
// may be nil, in which case events are silently dropped.
func NewExecAdapter(bus *events.Bus) *ExecAdapter {
	return &ExecAdapter{bus: bus}
}

// OpenRepository sets the repo path and publishes RepositoryOpened.
func (a *ExecAdapter) OpenRepository(ctx context.Context, path string) error {
	a.repoPath = path

	if a.bus != nil {
		_ = a.bus.Publish(ctx, events.EventRepositoryOpened, events.RepositoryOpenedPayload{
			Path:     path,
			OpenedAt: nowMS(),
		})
	}
	return nil
}

// Root returns the repository root path.
func (a *ExecAdapter) Root() string {
	return a.repoPath
}

// Run executes an arbitrary git command and returns stdout, stderr, and any error.
// This is a public wrapper for callers that need to run raw git commands
// (e.g. `git add -A` before committing).
func (a *ExecAdapter) Run(ctx context.Context, args ...string) (string, string, error) {
	return a.run(ctx, args...)
}

// run executes a git command and returns stdout, stderr, and any error.
func (a *ExecAdapter) run(ctx context.Context, args ...string) (string, string, error) {
	if a.repoPath == "" {
		return "", "", fmt.Errorf("git: no repository path set (call OpenRepository first)")
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = a.repoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Git exits non-zero for many common cases. Return the combined error.
	if err != nil {
		return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()),
			fmt.Errorf("git %s: %w\nstderr: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), nil
}

// nowMS returns the current UTC time in Unix milliseconds.
func nowMS() int64 {
	return time.Now().UnixMilli()
}
