package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/got-sh/got/internal/events"
)

// newTestStore creates a temporary SQLite database and returns a
// KnowledgeStore wired to it, plus a cleanup function.
func newTestStore(t *testing.T) (*KnowledgeStore, *events.Bus, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "got-test-knowledge-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}

	dbPath := filepath.Join(dir, "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	bus := events.New()
	ks := NewKnowledgeStore(s.DB(), bus)

	cleanup := func() {
		s.Close()
		os.RemoveAll(dir)
	}

	return ks, bus, cleanup
}

// ptrString is a small helper to get a *string literal.
func ptrString(s string) *string { return &s }

func intPtr(i int) *int { return &i }

func TestCreateDecision(t *testing.T) {
	ks, bus, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	// Subscribe to DecisionCreated.
	var gotEvent events.DecisionCreatedPayload
	_, _ = bus.Subscribe(events.EventDecisionCreated, func(_ context.Context, e events.Event) error {
		gotEvent = e.Payload.(events.DecisionCreatedPayload)
		return nil
	})

	d, err := ks.CreateDecision(ctx, CreateDecisionParams{
		Title:    "Use SQLite for storage",
		Context:  "Need a local database",
		Decision: "Use SQLite with modernc.org",
	})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}

	if d.Title != "Use SQLite for storage" {
		t.Fatalf("expected title 'Use SQLite for storage', got %q", d.Title)
	}
	if d.Status != "proposed" {
		t.Fatalf("expected status 'proposed', got %q", d.Status)
	}
	if d.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if d.BodyPath != "decisions/"+d.ID+".md" {
		t.Fatalf("expected body_path 'decisions/%s.md', got %q", d.ID, d.BodyPath)
	}

	// Verify event was published.
	if gotEvent.ID != d.ID {
		t.Fatalf("event ID mismatch: %q vs %q", gotEvent.ID, d.ID)
	}
	if gotEvent.Title != d.Title {
		t.Fatalf("event title mismatch: %q vs %q", gotEvent.Title, d.Title)
	}
}

func TestCreateDecisionWithSupersedes(t *testing.T) {
	ks, bus, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	// Create initial decision.
	old, err := ks.CreateDecision(ctx, CreateDecisionParams{
		Title: "Old approach",
	})
	if err != nil {
		t.Fatalf("CreateDecision(old): %v", err)
	}

	// Subscribe to DecisionSuperseded.
	var supersededEvent events.DecisionSupersededPayload
	_, _ = bus.Subscribe(events.EventDecisionSuperseded, func(_ context.Context, e events.Event) error {
		supersededEvent = e.Payload.(events.DecisionSupersededPayload)
		return nil
	})

	// Create new decision that supersedes the old one.
	newD, err := ks.CreateDecision(ctx, CreateDecisionParams{
		Title:        "New approach",
		SupersedesID: &old.ID,
	})
	if err != nil {
		t.Fatalf("CreateDecision(new): %v", err)
	}

	// Verify old decision is now superseded.
	oldReloaded, err := ks.GetDecision(ctx, old.ID)
	if err != nil {
		t.Fatalf("GetDecision(old): %v", err)
	}
	if oldReloaded.Status != "superseded" {
		t.Fatalf("expected old decision status 'superseded', got %q", oldReloaded.Status)
	}

	// Verify new decision's supersedes_id.
	if newD.SupersedesID == nil || *newD.SupersedesID != old.ID {
		t.Fatalf("expected new decision to supersede %q, got %v", old.ID, newD.SupersedesID)
	}

	// Verify superseded event.
	if supersededEvent.ID != old.ID {
		t.Fatalf("expected superseded event for %q, got %q", old.ID, supersededEvent.ID)
	}
	if supersededEvent.NewID != newD.ID {
		t.Fatalf("expected superseded event new_id %q, got %q", newD.ID, supersededEvent.NewID)
	}
}

func TestGetDecisionNotFound(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := ks.GetDecision(ctx, "nonexistent")
	if err != ErrDecisionNotFound {
		t.Fatalf("expected ErrDecisionNotFound, got %v", err)
	}
}

func TestListDecisions(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	// Create two decisions.
	d1, _ := ks.CreateDecision(ctx, CreateDecisionParams{Title: "First decision"})
	d2, _ := ks.CreateDecision(ctx, CreateDecisionParams{Title: "Second decision"})

	// List all.
	decisions, err := ks.ListAllDecisions(ctx)
	if err != nil {
		t.Fatalf("ListAllDecisions: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(decisions))
	}
	// Most recent first.
	if decisions[0].ID != d2.ID {
		t.Fatalf("expected first item %q (d2), got %q", d2.ID, decisions[0].ID)
	}
	if decisions[1].ID != d1.ID {
		t.Fatalf("expected second item %q (d1), got %q", d1.ID, decisions[1].ID)
	}
}

func TestListDecisionsWithFilter(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	wid := ptrString("ws1")

	ks.CreateDecision(ctx, CreateDecisionParams{Title: "D1", WorkspaceID: wid})
	ks.CreateDecision(ctx, CreateDecisionParams{Title: "D2"})

	// Filter by workspace.
	filtered, err := ks.ListDecisions(ctx, DecisionFilter{WorkspaceID: wid, All: true})
	if err != nil {
		t.Fatalf("ListDecisions with filter: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 decision in workspace, got %d", len(filtered))
	}
	if filtered[0].Title != "D1" {
		t.Fatalf("expected D1, got %q", filtered[0].Title)
	}
}

func TestLinkDecision(t *testing.T) {
	ks, bus, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	d, _ := ks.CreateDecision(ctx, CreateDecisionParams{Title: "Test decision"})

	// Subscribe to DecisionLinked.
	var linkEvent events.DecisionLinkedPayload
	_, _ = bus.Subscribe(events.EventDecisionLinked, func(_ context.Context, e events.Event) error {
		linkEvent = e.Payload.(events.DecisionLinkedPayload)
		return nil
	})

	// Link to a commit.
	err := ks.LinkDecision(ctx, LinkDecisionParams{
		DecisionID: d.ID,
		LinkType:   "commit",
		Target:     "abc123def456",
		Branch:     "main",
	})
	if err != nil {
		t.Fatalf("LinkDecision: %v", err)
	}

	// Verify link was stored.
	links, err := ks.GetDecisionLinks(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetDecisionLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].Target != "abc123def456" {
		t.Fatalf("expected target 'abc123def456', got %q", links[0].Target)
	}

	// Verify event.
	if linkEvent.DecisionID != d.ID {
		t.Fatalf("expected link event decision_id %q, got %q", d.ID, linkEvent.DecisionID)
	}
	if linkEvent.Target != "abc123def456" {
		t.Fatalf("expected link event target 'abc123def456', got %q", linkEvent.Target)
	}
}

func TestLinkDecisionWithLineRange(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	d, _ := ks.CreateDecision(ctx, CreateDecisionParams{Title: "Test"})

	err := ks.LinkDecision(ctx, LinkDecisionParams{
		DecisionID: d.ID,
		LinkType:   "file",
		Target:     "src/main.go",
		LineStart:  intPtr(42),
		LineEnd:    intPtr(58),
	})
	if err != nil {
		t.Fatalf("LinkDecision with line range: %v", err)
	}

	links, _ := ks.GetDecisionLinks(ctx, d.ID)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].LineStart == nil || *links[0].LineStart != 42 {
		t.Fatalf("expected line_start 42, got %v", links[0].LineStart)
	}
	if links[0].LineEnd == nil || *links[0].LineEnd != 58 {
		t.Fatalf("expected line_end 58, got %v", links[0].LineEnd)
	}
}

func TestLinkDecisionInvalidType(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	err := ks.LinkDecision(ctx, LinkDecisionParams{
		DecisionID: "some-id",
		LinkType:   "invalid",
		Target:     "x",
	})
	if err != ErrInvalidLinkType {
		t.Fatalf("expected ErrInvalidLinkType, got %v", err)
	}
}

func TestSupersedeDecision(t *testing.T) {
	ks, bus, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	old, _ := ks.CreateDecision(ctx, CreateDecisionParams{Title: "Old"})
	newD, _ := ks.CreateDecision(ctx, CreateDecisionParams{Title: "New"})

	var supersededEvent events.DecisionSupersededPayload
	_, _ = bus.Subscribe(events.EventDecisionSuperseded, func(_ context.Context, e events.Event) error {
		supersededEvent = e.Payload.(events.DecisionSupersededPayload)
		return nil
	})

	err := ks.SupersedeDecision(ctx, old.ID, newD.ID)
	if err != nil {
		t.Fatalf("SupersedeDecision: %v", err)
	}

	// Verify old is superseded.
	oldReloaded, _ := ks.GetDecision(ctx, old.ID)
	if oldReloaded.Status != "superseded" {
		t.Fatalf("expected 'superseded', got %q", oldReloaded.Status)
	}

	// Verify event.
	if supersededEvent.ID != old.ID {
		t.Fatalf("expected event ID %q, got %q", old.ID, supersededEvent.ID)
	}
	if supersededEvent.NewID != newD.ID {
		t.Fatalf("expected event new_id %q, got %q", newD.ID, supersededEvent.NewID)
	}
}

// ── Note CRUD tests ────────────────────────────────────────────────

func TestCreateNote(t *testing.T) {
	ks, bus, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	var noteEvent events.NoteAddedPayload
	_, _ = bus.Subscribe(events.EventNoteAdded, func(_ context.Context, e events.Event) error {
		noteEvent = e.Payload.(events.NoteAddedPayload)
		return nil
	})

	n, err := ks.CreateNote(ctx, CreateNoteParams{
		Message:    "Important finding about SQLite journaling",
		Branch:     "feature/knowledge",
		CommitHash: "abc123",
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	if n.Message != "Important finding about SQLite journaling" {
		t.Fatalf("unexpected message: %q", n.Message)
	}
	if n.Branch != "feature/knowledge" {
		t.Fatalf("unexpected branch: %q", n.Branch)
	}

	// Verify event.
	if noteEvent.ID != n.ID {
		t.Fatalf("event ID mismatch: %q vs %q", noteEvent.ID, n.ID)
	}
	if noteEvent.Branch != "feature/knowledge" {
		t.Fatalf("event branch mismatch: %q", noteEvent.Branch)
	}
}

func TestCreateNoteEmptyMessage(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := ks.CreateNote(ctx, CreateNoteParams{Message: ""})
	if err != ErrEmptyMessage {
		t.Fatalf("expected ErrEmptyMessage, got %v", err)
	}

	_, err = ks.CreateNote(ctx, CreateNoteParams{Message: "   "})
	if err != ErrEmptyMessage {
		t.Fatalf("expected ErrEmptyMessage for whitespace, got %v", err)
	}
}

func TestGetNoteNotFound(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := ks.GetNote(ctx, "nonexistent")
	if err != ErrNoteNotFound {
		t.Fatalf("expected ErrNoteNotFound, got %v", err)
	}
}

func TestListNotes(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateNote(ctx, CreateNoteParams{Message: "Note 1"})
	ks.CreateNote(ctx, CreateNoteParams{Message: "Note 2", WorkspaceID: ptrString("ws1")})

	notes, err := ks.ListNotes(ctx, NoteFilter{All: true})
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(notes))
	}

	// Filter by workspace.
	wsNotes, err := ks.ListNotes(ctx, NoteFilter{WorkspaceID: ptrString("ws1"), All: true})
	if err != nil {
		t.Fatalf("ListNotes with workspace filter: %v", err)
	}
	if len(wsNotes) != 1 {
		t.Fatalf("expected 1 note in workspace, got %d", len(wsNotes))
	}
}

func TestListNotesDefaultLimit(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	// Create 25 notes (over the default limit of 20).
	for i := range 25 {
		ks.CreateNote(ctx, CreateNoteParams{
			Message: fmt.Sprintf("Note %d", i),
		})
	}

	// Without All, should return at most 20.
	notes, err := ks.ListNotes(ctx, NoteFilter{})
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(notes) != 20 {
		t.Fatalf("expected 20 notes (default limit), got %d", len(notes))
	}
}

// ── Onboarding CRUD tests ──────────────────────────────────────────

func TestStartOnboarding(t *testing.T) {
	ks, bus, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	// Pre-create some decisions and a note so the session gets seeded.
	ks.CreateDecision(ctx, CreateDecisionParams{Title: "D1"})
	ks.CreateDecision(ctx, CreateDecisionParams{Title: "D2"})
	ks.CreateNote(ctx, CreateNoteParams{Message: "Note 1"})

	var startedEvent events.OnboardingStartedPayload
	_, _ = bus.Subscribe(events.EventOnboardingStarted, func(_ context.Context, e events.Event) error {
		startedEvent = e.Payload.(events.OnboardingStartedPayload)
		return nil
	})

	session, err := ks.StartOnboarding(ctx, "alice")
	if err != nil {
		t.Fatalf("StartOnboarding: %v", err)
	}

	if session.Participant != "alice" {
		t.Fatalf("expected participant 'alice', got %q", session.Participant)
	}
	if session.Status != "active" {
		t.Fatalf("expected status 'active', got %q", session.Status)
	}

	// Verify event.
	if startedEvent.SessionID != session.ID {
		t.Fatalf("event session_id mismatch: %q vs %q", startedEvent.SessionID, session.ID)
	}
	if startedEvent.Participant != "alice" {
		t.Fatalf("event participant mismatch: %q", startedEvent.Participant)
	}

	// Verify items were seeded.
	items, err := ks.ListUnwatchedItems(ctx, session.ID)
	if err != nil {
		t.Fatalf("ListUnwatchedItems: %v", err)
	}
	// 2 decisions + 1 note = 3 items
	if len(items) != 3 {
		t.Fatalf("expected 3 unwatched items, got %d", len(items))
	}
}

func TestStartOnboardingResumesActive(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	s1, _ := ks.StartOnboarding(ctx, "bob")
	s2, _ := ks.StartOnboarding(ctx, "bob")

	// Should return the same active session.
	if s1.ID != s2.ID {
		t.Fatalf("expected same session ID on resume, got %q vs %q", s1.ID, s2.ID)
	}
}

func TestMarkOnboardingItem(t *testing.T) {
	ks, bus, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateDecision(ctx, CreateDecisionParams{Title: "D1"})
	session, _ := ks.StartOnboarding(ctx, "charlie")

	items, _ := ks.ListUnwatchedItems(ctx, session.ID)
	if len(items) == 0 {
		t.Fatal("expected at least one unwatched item")
	}

	var coveredEvent events.OnboardingItemCoveredPayload
	_, _ = bus.Subscribe(events.EventOnboardingItemCovered, func(_ context.Context, e events.Event) error {
		coveredEvent = e.Payload.(events.OnboardingItemCoveredPayload)
		return nil
	})

	// Mark the first item as covered.
	err := ks.MarkOnboardingItem(ctx, session.ID, items[0].ItemType, items[0].ItemTarget)
	if err != nil {
		t.Fatalf("MarkOnboardingItem: %v", err)
	}

	// Verify event.
	if coveredEvent.SessionID != session.ID {
		t.Fatalf("event session_id mismatch: %q", coveredEvent.SessionID)
	}
	if coveredEvent.ItemTarget != items[0].ItemTarget {
		t.Fatalf("event item_target mismatch: %q vs %q", coveredEvent.ItemTarget, items[0].ItemTarget)
	}

	// Re-count unwatched.
	remaining, _ := ks.ListUnwatchedItems(ctx, session.ID)
	if len(remaining) != len(items)-1 {
		t.Fatalf("expected %d remaining, got %d", len(items)-1, len(remaining))
	}
}

func TestSkipOnboardingItem(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateDecision(ctx, CreateDecisionParams{Title: "D1"})
	session, _ := ks.StartOnboarding(ctx, "dave")

	items, _ := ks.ListUnwatchedItems(ctx, session.ID)
	if len(items) == 0 {
		t.Fatal("expected at least one unwatched item")
	}

	err := ks.SkipOnboardingItem(ctx, session.ID, items[0].ItemType, items[0].ItemTarget)
	if err != nil {
		t.Fatalf("SkipOnboardingItem: %v", err)
	}

	remaining, _ := ks.ListUnwatchedItems(ctx, session.ID)
	if len(remaining) != len(items)-1 {
		t.Fatalf("expected %d remaining after skip, got %d", len(items)-1, len(remaining))
	}
}

func TestOnboardingProgress(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateDecision(ctx, CreateDecisionParams{Title: "D1"})
	ks.CreateDecision(ctx, CreateDecisionParams{Title: "D2"})
	session, _ := ks.StartOnboarding(ctx, "eve")

	progress, err := ks.GetOnboardingProgress(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetOnboardingProgress: %v", err)
	}

	if progress.Session.ID != session.ID {
		t.Fatalf("progress session ID mismatch")
	}
	if progress.TotalItems != 2 {
		t.Fatalf("expected 2 total items, got %d", progress.TotalItems)
	}
	if progress.Covered != 0 {
		t.Fatalf("expected 0 covered, got %d", progress.Covered)
	}
	if progress.Remaining != 2 {
		t.Fatalf("expected 2 remaining, got %d", progress.Remaining)
	}

	// Mark one as covered and check again.
	items, _ := ks.ListUnwatchedItems(ctx, session.ID)
	ks.MarkOnboardingItem(ctx, session.ID, items[0].ItemType, items[0].ItemTarget)

	progress, _ = ks.GetOnboardingProgress(ctx, session.ID)
	if progress.Covered != 1 {
		t.Fatalf("expected 1 covered, got %d", progress.Covered)
	}
	if progress.Remaining != 1 {
		t.Fatalf("expected 1 remaining, got %d", progress.Remaining)
	}
}

func TestCompleteOnboarding(t *testing.T) {
	ks, bus, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	session, _ := ks.StartOnboarding(ctx, "frank")

	var completedEvent events.OnboardingCompletedPayload
	_, _ = bus.Subscribe(events.EventOnboardingCompleted, func(_ context.Context, e events.Event) error {
		completedEvent = e.Payload.(events.OnboardingCompletedPayload)
		return nil
	})

	err := ks.CompleteOnboarding(ctx, session.ID)
	if err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}

	// Verify session is completed.
	reloaded, _ := ks.GetOnboardingSession(ctx, session.ID)
	if reloaded.Status != "completed" {
		t.Fatalf("expected 'completed', got %q", reloaded.Status)
	}

	// Verify event.
	if completedEvent.SessionID != session.ID {
		t.Fatalf("event session_id mismatch: %q", completedEvent.SessionID)
	}
	if completedEvent.Participant != "frank" {
		t.Fatalf("event participant mismatch: %q", completedEvent.Participant)
	}

	// Double completion should error.
	err = ks.CompleteOnboarding(ctx, session.ID)
	if err != ErrSessionAlreadyComplete {
		t.Fatalf("expected ErrSessionAlreadyComplete, got %v", err)
	}
}

// ── Event bus integration (nil bus) ────────────────────────────────

func TestKnowledgeStoreNilBus(t *testing.T) {
	dir, _ := os.MkdirTemp("", "got-test-nilbus-*")
	defer os.RemoveAll(dir)

	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// Create store with nil bus — should not panic.
	ks := NewKnowledgeStore(s.DB(), nil)
	ctx := context.Background()

	d, err := ks.CreateDecision(ctx, CreateDecisionParams{Title: "No bus"})
	if err != nil {
		t.Fatalf("CreateDecision with nil bus: %v", err)
	}
	if d.Title != "No bus" {
		t.Fatalf("unexpected title: %q", d.Title)
	}

	n, err := ks.CreateNote(ctx, CreateNoteParams{Message: "Works without bus"})
	if err != nil {
		t.Fatalf("CreateNote with nil bus: %v", err)
	}
	if n.Message != "Works without bus" {
		t.Fatalf("unexpected message: %q", n.Message)
	}

	session, err := ks.StartOnboarding(ctx, "headless")
	if err != nil {
		t.Fatalf("StartOnboarding with nil bus: %v", err)
	}
	if session.Status != "active" {
		t.Fatalf("unexpected status: %q", session.Status)
	}
}

// ── Edge cases ─────────────────────────────────────────────────────

func TestCreateDecisionEmptyTitle(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := ks.CreateDecision(ctx, CreateDecisionParams{Title: ""})
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestSupersedeNonexistentDecision(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	err := ks.SupersedeDecision(ctx, "nonexistent", "alsononexistent")
	if err != ErrDecisionNotFound {
		t.Fatalf("expected ErrDecisionNotFound, got %v", err)
	}
}

func TestGetOnboardingSessionNotFound(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := ks.GetOnboardingSession(ctx, "nonexistent")
	if err != ErrOnboardingNotFound {
		t.Fatalf("expected ErrOnboardingNotFound, got %v", err)
	}
}

func TestDecisionFields(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	d, err := ks.CreateDecision(ctx, CreateDecisionParams{
		Title:        "Full decision",
		Context:      "Background context",
		Decision:     "The actual decision",
		Alternatives: "Alternative approaches",
		Consequences: "What happens next",
		WorkspaceID:  ptrString("ws1"),
	})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}

	if d.Context != "Background context" {
		t.Fatalf("context mismatch: %q", d.Context)
	}
	if d.Decision != "The actual decision" {
		t.Fatalf("decision field mismatch: %q", d.Decision)
	}
	if d.Alternatives != "Alternative approaches" {
		t.Fatalf("alternatives mismatch: %q", d.Alternatives)
	}
	if d.Consequences != "What happens next" {
		t.Fatalf("consequences mismatch: %q", d.Consequences)
	}
	if d.WorkspaceID == nil || *d.WorkspaceID != "ws1" {
		t.Fatalf("workspace_id mismatch: %v", d.WorkspaceID)
	}
}

func TestListDecisionsWithLimit(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	for i := range 10 {
		title := fmt.Sprintf("Decision %d", i)
		ks.CreateDecision(ctx, CreateDecisionParams{Title: title})
	}

	// Custom limit of 3.
	decisions, err := ks.ListDecisions(ctx, DecisionFilter{Limit: 3})
	if err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(decisions) != 3 {
		t.Fatalf("expected 3 decisions, got %d", len(decisions))
	}
}

func TestULIDGeneration(t *testing.T) {
	// Verify ULIDs are unique and sortable.
	ids := make(map[string]bool)
	for range 1000 {
		id := newULID()
		if len(id) != 26 {
			t.Fatalf("expected ULID length 26, got %d for %q", len(id), id)
		}
		if ids[id] {
			t.Fatalf("duplicate ULID: %q", id)
		}
		ids[id] = true
	}
}
