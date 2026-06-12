// Package git defines the GitAdapter interface and the data types
// returned by adapter methods. The default implementation is in exec.go
// and shells out to the `git` binary. Tests use the in-memory FakeAdapter
// in fake.go.
package git

import (
	"context"
	"io"
	"time"
)

// SHA is a Git commit SHA. The empty string represents an unborn HEAD.
type SHA string

// Status is the parsed result of `git status --porcelain=v2 --branch`.
// The `Entries` slice is empty for a clean working tree.
type Status struct {
	Branch   string        `json:"branch"`
	Detached bool          `json:"detached"`
	Ahead    int           `json:"ahead"`
	Behind   int           `json:"behind"`
	Upstream string        `json:"upstream,omitempty"`
	Entries  []StatusEntry `json:"entries"`
}

// StatusEntry is one file's status. A file may be staged, unstaged, or
// untracked. Renames carry the original path in OriginalPath.
type StatusEntry struct {
	Path         string `json:"path"`
	OriginalPath string `json:"originalPath,omitempty"`
	XY           string `json:"xy"`
	IsStaged     bool   `json:"staged"`
	IsUnstaged   bool   `json:"unstaged"`
	IsUntracked  bool   `json:"untracked"`
	IsRenamed    bool   `json:"renamed"`
}

// Branch is a local or remote-tracking branch. IsCurrent marks the
// branch HEAD currently points at; IsRemote is true for refs under
// refs/remotes/.
type Branch struct {
	Name      string    `json:"name"`
	IsCurrent bool      `json:"current"`
	IsRemote  bool      `json:"remote"`
	Upstream  string    `json:"upstream,omitempty"`
	SHA       string    `json:"sha"`
	CommitAt  time.Time `json:"commitAt"`
}

// Remote is a configured Git remote. FetchURL and PushURL are identical
// unless a separate pushurl is configured.
type Remote struct {
	Name      string `json:"name"`
	FetchURL  string `json:"fetchUrl"`
	PushURL   string `json:"pushUrl"`
	FetchSpec string `json:"fetchSpec,omitempty"`
}

// Commit is one commit in a log range.
type Commit struct {
	SHA       string    `json:"sha"`
	Parents   []string  `json:"parents,omitempty"`
	Author    string    `json:"author"`
	Email     string    `json:"email"`
	Timestamp time.Time `json:"timestamp"`
	Subject   string    `json:"subject"`
	Refs      []string  `json:"refs,omitempty"`
}

// LogFormat selects how the Log reader encodes commits.
type LogFormat string

const (
	// LogFormatNDJSON yields one JSON commit object per line.
	LogFormatNDJSON LogFormat = "ndjson"
)

// CommitOpts controls `git commit`.
type CommitOpts struct {
	Amend      bool
	AllowEmpty bool
	Signoff    bool
	NoVerify   bool
	Author     string
}

// CheckoutOpts controls `git checkout`.
type CheckoutOpts struct {
	Create bool
	Force  bool
	Detach bool
}

// MergeOpts controls `git merge`.
type MergeOpts struct {
	NoFF     bool
	Squash   bool
	NoCommit bool
	Message  string
}

// ResetMode selects the mode of `git reset`.
type ResetMode string

const (
	ResetSoft  ResetMode = "soft"
	ResetMixed ResetMode = "mixed"
	ResetHard  ResetMode = "hard"
)

// PushOpts controls `git push`.
type PushOpts struct {
	Force          bool
	ForceWithLease bool
	SetUpstream    bool
	Tags           bool
}

// Adapter is the abstract Git interface used by the rest of the GOT
// codebase. The exec-based implementation in exec.go is the default; the
// in-memory implementation in fake.go is used by tests.
type Adapter interface {
	Status(ctx context.Context) (Status, error)
	Commit(ctx context.Context, msg string, opts CommitOpts) (SHA, error)
	Branches(ctx context.Context) ([]Branch, error)
	RemoteBranches(ctx context.Context) ([]Branch, error)
	Remotes(ctx context.Context) ([]Remote, error)
	Checkout(ctx context.Context, ref string, opts CheckoutOpts) error
	Merge(ctx context.Context, ref string, opts MergeOpts) error
	Reset(ctx context.Context, target string, mode ResetMode) error
	Fetch(ctx context.Context, remote string) error
	Push(ctx context.Context, remote, branch string, opts PushOpts) error
	Log(ctx context.Context, rangeStr string, format LogFormat) (io.Reader, error)
	CurrentRef(ctx context.Context) (string, error)
}
