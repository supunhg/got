package eventbus

import (
	"encoding/json"
	"fmt"
	"time"
)

// Topic is the named channel an event flows on. Topics are short
// stable strings (PascalCase verb-past-tense) so that JSON logs are
// readable and so a typo in a subscriber registration is easy to spot
// in code review. The set of known topics is intentionally small; new
// ones are added by extending this list and bumping the docs.
type Topic string

// Known topics. The list is closed: every event published on the bus
// must use one of these values. The CLI rejects unknown topics at
// publish time (defense in depth) and the tests assert that
// internal callers never publish a raw string.
//
// v0.4 wires WorkspaceCreated and WorkspaceUpdated; the rest are
// reserved for v0.5+ commands. They are listed here so plugins
// written against the v0.4 bus can rely on the contract.
const (
	TopicRepositoryOpened   Topic = "RepositoryOpened"
	TopicCommitCreated      Topic = "CommitCreated"
	TopicCommitAmended      Topic = "CommitAmended"
	TopicBranchCreated      Topic = "BranchCreated"
	TopicBranchDeleted      Topic = "BranchDeleted"
	TopicMergeCompleted     Topic = "MergeCompleted"
	TopicPushCompleted      Topic = "PushCompleted"
	TopicWorkspaceCreated   Topic = "WorkspaceCreated"
	TopicWorkspaceUpdated   Topic = "WorkspaceUpdated"
	TopicSnapshotCreated    Topic = "SnapshotCreated"
)

// knownTopics is the closed allow-list used by ParseTopic. It is
// derived from the const block above so the two cannot drift.
var knownTopics = map[Topic]struct{}{
	TopicRepositoryOpened: {},
	TopicCommitCreated:    {},
	TopicCommitAmended:    {},
	TopicBranchCreated:    {},
	TopicBranchDeleted:    {},
	TopicMergeCompleted:   {},
	TopicPushCompleted:    {},
	TopicWorkspaceCreated: {},
	TopicWorkspaceUpdated: {},
	TopicSnapshotCreated:  {},
}

// ParseTopic validates that s is a known topic name. It is the
// single entry point used by both the publisher and the CLI to
// reject typos and unknown topics with a clear error. The
// returned error wraps ErrUnknownTopic so callers can branch
// on it with errors.Is.
func ParseTopic(s string) (Topic, error) {
	t := Topic(s)
	if _, ok := knownTopics[t]; !ok {
		return "", fmt.Errorf("%w: %q (allowed: %v)", ErrUnknownTopic, s, allTopicNames())
	}
	return t, nil
}

// AllTopics returns the known topics in a stable order (the same
// order they are declared in this file). Used by CLI help text and
// by the `got event list` topic filter.
func AllTopics() []Topic {
	out := make([]Topic, 0, len(knownTopics))
	for t := range knownTopics {
		out = append(out, t)
	}
	// Stable order: walk the const block in declaration order via
	// the ordered slice below.
	return orderTopics(out)
}

// allTopicNames is the string-slice form of AllTopics for error
// messages. Kept private because callers should use AllTopics().
func allTopicNames() []string {
	out := make([]string, 0, len(knownTopics))
	for _, t := range orderTopics(nil) {
		out = append(out, string(t))
	}
	return out
}

// orderedTopicList mirrors the declaration order in the const block.
// The first entries are the topics the v0.4 bus actually emits; the
// later entries are reserved for future versions. Keeping this in
// declaration order means CLI help text is stable across builds.
var orderedTopicList = []Topic{
	TopicRepositoryOpened,
	TopicCommitCreated,
	TopicCommitAmended,
	TopicBranchCreated,
	TopicBranchDeleted,
	TopicMergeCompleted,
	TopicPushCompleted,
	TopicWorkspaceCreated,
	TopicWorkspaceUpdated,
	TopicSnapshotCreated,
}

// orderTopics returns topics in declaration order. If in is nil
// (the signal from allTopicNames), it returns the full ordered list
// deduplicated; if in is non-nil, it filters to those that appear in
// the ordered list, preserving order.
func orderTopics(in []Topic) []Topic {
	seen := map[Topic]bool{}
	out := make([]Topic, 0, len(orderedTopicList))
	for _, t := range orderedTopicList {
		if in != nil {
			if !containsTopic(in, t) {
				continue
			}
		}
		if seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func containsTopic(haystack []Topic, needle Topic) bool {
	for _, t := range haystack {
		if t == needle {
			return true
		}
	}
	return false
}

// Payload is the JSON body of an event. It is intentionally
// permissive: any structure that the publisher wants to send
// (CommitCreated may carry a sha and a subject, WorkspaceCreated
// may carry a name and a title, etc.) is allowed. Subscribers should
// type-switch on Topic to interpret the payload.
//
// The envelope itself is *Event; the payload is whatever is in
// Event.Payload. The CLI renders it as a raw JSON object.
type Payload map[string]any

// Event is the envelope carried on the bus and stored in the
// SQLite events table. Fields are stable: ID is the ULID-like
// event ID (used as a dedup key by replay clients), Topic is the
// topic name, CreatedAt is when the publisher called Publish, Actor
// is the OS user (best-effort, empty if unknown), Payload is the
// JSON object, and Source is the .got/got.db file the event came
// from (so multi-worktree processes can tell events apart).
type Event struct {
	ID        string    `json:"id"`
	Topic     Topic     `json:"topic"`
	CreatedAt time.Time `json:"createdAt"`
	Actor     string    `json:"actor,omitempty"`
	Source    string    `json:"source,omitempty"`
	Payload   Payload   `json:"payload"`
}

// MarshalPayload is a small helper for publishers that have a
// strongly-typed struct they want to embed. It JSON-encodes v and
// unmarshals it back into a Payload, which preserves numeric types
// and nested objects more faithfully than `map[string]any{v}`
// would. Returns an error if v is not JSON-encodable.
func MarshalPayload(v any) (Payload, error) {
	if v == nil {
		return Payload{}, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("eventbus: marshal payload: %w", err)
	}
	if len(b) == 0 || string(b) == "null" {
		return Payload{}, nil
	}
	var p Payload
	if err := json.Unmarshal(b, &p); err != nil {
		// Fall back to a stringified form so the event is still
		// persisted; subscribers can re-parse by topic.
		return Payload{"_raw": string(b)}, nil
	}
	return p, nil
}
