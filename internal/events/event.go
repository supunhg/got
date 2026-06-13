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
	EventNoteAdded = "NoteAdded"

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

// OnboardingCompletedPayload is published when an onboarding session completes.
type OnboardingCompletedPayload struct {
	SessionID   string `json:"session_id"`
	Participant string `json:"participant"`
	TotalItems  int    `json:"total_items"`
	CompletedAt int64  `json:"completed_at"`
}
