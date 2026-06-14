// Package events provides a minimal in-memory event bus for in-process
// pub/sub communication. It is designed for the v0.4 Knowledge Engine and
// is intentionally simple — no persistence, no delivery guarantees beyond
// best-effort synchronous dispatch to subscribers, no wildcard patterns.
package events

import (
	"context"
	"time"
)

// Event is the canonical event envelope published through the Bus.
type Event struct {
	// Type identifies the kind of event (e.g. "DecisionCreated").
	Type string

	// Payload holds the typed event data. See the payload structs in this
	// package for the concrete types published by the Knowledge Engine.
	Payload any

	// Timestamp is the time at which the event was published. It is set
	// automatically by Bus.Publish.
	Timestamp time.Time
}

// Handler receives events published to the bus. Returning an error signals
// that the handler failed to process the event. The Publish caller receives
// all handler errors aggregated.
type Handler func(ctx context.Context, e Event) error

// Event type constants published by the Knowledge Engine. Plugins and
// in-process consumers subscribe to these strings.
const (
	// Decision lifecycle
	EventDecisionCreated    = "DecisionCreated"
	EventDecisionUpdated    = "DecisionUpdated"
	EventDecisionSuperseded = "DecisionSuperseded"
	EventDecisionLinked     = "DecisionLinked"

	// Notes
	EventNoteAdded   = "NoteAdded"
	EventNoteDeleted = "NoteDeleted"

	// Decision lifecycle (continued)
	EventDecisionDeleted = "DecisionDeleted"

	// Git adapter
	EventRepositoryOpened    = "RepositoryOpened"
	EventBranchCreated       = "BranchCreated"
	EventBranchDeleted       = "BranchDeleted"
	EventBranchCheckedOut     = "BranchCheckedOut"
	EventCommitCreated        = "CommitCreated"
	EventRemoteAdded          = "RemoteAdded"
	EventRemoteRemoved        = "RemoteRemoved"
	EventPushCompleted        = "PushCompleted"
	EventPullCompleted        = "PullCompleted"

	// Workspaces
	EventWorkspaceCreated    = "WorkspaceCreated"
	EventWorkspaceUpdated    = "WorkspaceUpdated"
	EventWorkspaceDeleted    = "WorkspaceDeleted"
	EventWorkspaceItemAdded   = "WorkspaceItemAdded"
	EventWorkspaceItemRemoved = "WorkspaceItemRemoved"

	// Onboarding
	EventOnboardingStarted      = "OnboardingStarted"
	EventOnboardingItemCovered  = "OnboardingItemCovered"
	EventOnboardingCompleted    = "OnboardingCompleted"
)

// ── Typed payloads ──────────────────────────────────────────────────
// Each payload struct corresponds to one EventType constant. The structs
// are serialised to JSON when written to the durable event_log table but
// passed as-is through the in-memory Bus.

// DecisionCreatedPayload is published after a decision is created.
type DecisionCreatedPayload struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	WorkspaceID  string `json:"workspace_id,omitempty"`
	SupersedesID string `json:"supersedes_id,omitempty"`
	CreatedAt    int64  `json:"created_at"`
}

// DecisionUpdatedPayload is published after a decision's status or fields change.
type DecisionUpdatedPayload struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Status         string `json:"status"`
	PreviousStatus string `json:"previous_status"`
	WorkspaceID    string `json:"workspace_id,omitempty"`
	UpdatedAt      int64  `json:"updated_at"`
}

// DecisionSupersededPayload is published when one decision supersedes another.
type DecisionSupersededPayload struct {
	ID          string `json:"id"`
	NewID       string `json:"new_id"`
	OldStatus   string `json:"old_status"`
	SupersededAt int64 `json:"superseded_at"`
}

// DecisionLinkedPayload is published when a link (commit, file, workspace) is
// attached to a decision.
type DecisionLinkedPayload struct {
	DecisionID string `json:"decision_id"`
	LinkType   string `json:"link_type"`    // "commit", "file", "workspace"
	Target     string `json:"target"`
	CreatedAt  int64  `json:"created_at"`
}

// DecisionDeletedPayload is published after a decision is hard-deleted.
type DecisionDeletedPayload struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	DeletedAt   int64  `json:"deleted_at"`
}

// NoteDeletedPayload is published after a note is hard-deleted.
type NoteDeletedPayload struct {
	ID          string `json:"id"`
	Message     string `json:"message"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Branch      string `json:"branch,omitempty"`
	DeletedAt   int64  `json:"deleted_at"`
}

// NoteAddedPayload is published after a note is created.
type NoteAddedPayload struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Branch      string `json:"branch,omitempty"`
	CommitHash  string `json:"commit_hash,omitempty"`
	CreatedAt   int64  `json:"created_at"`
}

// OnboardingStartedPayload is published when an onboarding session begins.
type OnboardingStartedPayload struct {
	SessionID   string `json:"session_id"`
	Participant string `json:"participant"`
	ItemCount   int    `json:"item_count"`
	CreatedAt   int64  `json:"created_at"`
}

// OnboardingItemCoveredPayload is published when an onboarding item is marked covered.
type OnboardingItemCoveredPayload struct {
	SessionID  string `json:"session_id"`
	ItemType   string `json:"item_type"`   // "decision", "note", "file"
	ItemTarget string `json:"item_target"`
	CoveredAt  int64  `json:"covered_at"`
}

// ── Workspace payloads ─────────────────────────────────────────

// ── Git adapter payloads ────────────────────────────────────────

// RepositoryOpenedPayload is published when a repository is opened.
type RepositoryOpenedPayload struct {
	Path      string `json:"path"`
	OpenedAt  int64  `json:"opened_at"`
}

// BranchCreatedPayload is published when a branch is created.
type BranchCreatedPayload struct {
	Name      string `json:"name"`
	Ref       string `json:"ref,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// BranchDeletedPayload is published when a branch is deleted.
type BranchDeletedPayload struct {
	Name      string `json:"name"`
	DeletedAt int64  `json:"deleted_at"`
}

// BranchCheckedOutPayload is published when a branch is checked out.
type BranchCheckedOutPayload struct {
	PreviousBranch string `json:"previous_branch,omitempty"`
	NewBranch      string `json:"new_branch"`
	CheckedOutAt   int64  `json:"checked_out_at"`
}

// CommitCreatedPayload is published when a commit is created.
type CommitCreatedPayload struct {
	SHA       string `json:"sha"`
	Message   string `json:"message"`
	Author    string `json:"author"`
	Branch    string `json:"branch,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// RemoteAddedPayload is published when a remote is added.
type RemoteAddedPayload struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	AddedAt   int64  `json:"added_at"`
}

// RemoteRemovedPayload is published when a remote is removed.
type RemoteRemovedPayload struct {
	Name      string `json:"name"`
	RemovedAt int64  `json:"removed_at"`
}

// PushCompletedPayload is published after a push completes.
type PushCompletedPayload struct {
	Remote    string `json:"remote"`
	Branch    string `json:"branch"`
	Force     bool   `json:"force"`
	CompletedAt int64 `json:"completed_at"`
}

// PullCompletedPayload is published after a pull completes.
type PullCompletedPayload struct {
	Remote      string `json:"remote"`
	Branch      string `json:"branch"`
	FastForward bool   `json:"fast_forward"`
	CompletedAt int64  `json:"completed_at"`
}

// WorkspaceCreatedPayload is published when a workspace is created.
type WorkspaceCreatedPayload struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	CreatedAt   int64    `json:"created_at"`
}

// WorkspaceUpdatedPayload is published when a workspace is updated.
type WorkspaceUpdatedPayload struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	UpdatedAt   int64    `json:"updated_at"`
}

// WorkspaceDeletedPayload is published when a workspace is deleted.
type WorkspaceDeletedPayload struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ItemCount   int    `json:"item_count"`
	DeletedAt   int64  `json:"deleted_at"`
}

// WorkspaceItemAddedPayload is published when an item (file, branch,
// decision, note) is added to a workspace.
type WorkspaceItemAddedPayload struct {
	WorkspaceID string `json:"workspace_id"`
	ItemType    string `json:"item_type"`   // "file", "branch", "decision", "note"
	ItemTarget  string `json:"item_target"` // path, branch name, decision ID, note ID
	CreatedAt   int64  `json:"created_at"`
}

// WorkspaceItemRemovedPayload is published when an item is removed from
// a workspace.
type WorkspaceItemRemovedPayload struct {
	WorkspaceID string `json:"workspace_id"`
	ItemType    string `json:"item_type"`
	ItemTarget  string `json:"item_target"`
	RemovedAt   int64  `json:"removed_at"`
}

// OnboardingCompletedPayload is published when an onboarding session completes.
type OnboardingCompletedPayload struct {
	SessionID   string `json:"session_id"`
	Participant string `json:"participant"`
	TotalItems  int    `json:"total_items"`
	CompletedAt int64  `json:"completed_at"`
}
