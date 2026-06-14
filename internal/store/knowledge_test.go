package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/supunhg/got/internal/events"
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
	// Both decisions should be present (order may vary when created in
	// the same millisecond due to ULID random component).
	ids := map[string]bool{decisions[0].ID: true, decisions[1].ID: true}
	if !ids[d1.ID] {
		t.Fatalf("expected d1 %q in results, got %v", d1.ID, ids)
	}
	if !ids[d2.ID] {
		t.Fatalf("expected d2 %q in results, got %v", d2.ID, ids)
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

func TestDeleteNote(t *testing.T) {
	ks, bus, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	n, err := ks.CreateNote(ctx, CreateNoteParams{
		Message:    "To be deleted",
		Branch:     "feature/x",
		CommitHash: "abc123",
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	// Subscribe to NoteDeleted.
	var deletedEvent events.NoteDeletedPayload
	_, _ = bus.Subscribe(events.EventNoteDeleted, func(_ context.Context, e events.Event) error {
		deletedEvent = e.Payload.(events.NoteDeletedPayload)
		return nil
	})

	// Delete the note.
	err = ks.DeleteNote(ctx, n.ID)
	if err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}

	// Verify it's gone.
	_, err = ks.GetNote(ctx, n.ID)
	if err != ErrNoteNotFound {
		t.Fatalf("expected ErrNoteNotFound after delete, got %v", err)
	}

	// Verify event.
	if deletedEvent.ID != n.ID {
		t.Fatalf("event ID mismatch: %q vs %q", deletedEvent.ID, n.ID)
	}
	if deletedEvent.Message != "To be deleted" {
		t.Fatalf("event message mismatch: %q", deletedEvent.Message)
	}
	if deletedEvent.Branch != "feature/x" {
		t.Fatalf("event branch mismatch: %q", deletedEvent.Branch)
	}
}

func TestDeleteNoteNonexistent(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	err := ks.DeleteNote(ctx, "nonexistent")
	if err != ErrNoteNotFound {
		t.Fatalf("expected ErrNoteNotFound, got %v", err)
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

// ── Search tests ───────────────────────────────────────────────────

func TestSearchDecisionsAndNotes(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	// Create decisions with searchable content.
	ks.CreateDecision(ctx, CreateDecisionParams{
		Title:    "Use SQLite for storage",
		Context:  "We need a local database for persistence",
		Decision: "Use SQLite with the modernc.org driver",
	})
	ks.CreateDecision(ctx, CreateDecisionParams{
		Title:    "Adopt Bubbletea for TUI",
		Context:  "We need a terminal UI framework",
		Decision: "Use Bubbletea for the interactive CLI",
		Alternatives: "Considered Huh? and survey",
	})

	// Create notes with searchable content.
	ks.CreateNote(ctx, CreateNoteParams{
		Message: "SQLite WAL mode improves concurrent read performance",
	})
	ks.CreateNote(ctx, CreateNoteParams{
		Message: "Bubbletea has excellent testing utilities in the form of the TestModel",
	})

	// Search for SQLite.
	results, err := ks.Search(ctx, SearchParams{Query: "SQLite", Limit: 10})
	if err != nil {
		t.Fatalf("Search('SQLite'): %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for 'SQLite', got %d", len(results))
	}

	// First result should be the decision (matches more fields = higher score).
	if results[0].Type != "decision" {
		t.Fatalf("expected first result type 'decision', got %q", results[0].Type)
	}
	if results[0].Title != "Use SQLite for storage" {
		t.Fatalf("expected title 'Use SQLite for storage', got %q", results[0].Title)
	}
	if results[0].Status != "proposed" {
		t.Fatalf("expected status 'proposed', got %q", results[0].Status)
	}

	// Second result should be the note.
	if results[1].Type != "note" {
		t.Fatalf("expected second result type 'note', got %q", results[1].Type)
	}

	// Search for Bubbletea.
	results, err = ks.Search(ctx, SearchParams{Query: "Bubbletea", Limit: 10})
	if err != nil {
		t.Fatalf("Search('Bubbletea'): %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for 'Bubbletea', got %d", len(results))
	}
}

func TestSearchFilterByType(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateDecision(ctx, CreateDecisionParams{Title: "Database choice"})
	ks.CreateNote(ctx, CreateNoteParams{Message: "Database tuning notes"})

	// Search only decisions.
	decOnly := "decision"
	results, err := ks.Search(ctx, SearchParams{Query: "Database", Type: &decOnly, Limit: 10})
	if err != nil {
		t.Fatalf("Search decisions only: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 decision result, got %d", len(results))
	}
	if results[0].Type != "decision" {
		t.Fatalf("expected type 'decision', got %q", results[0].Type)
	}

	// Search only notes.
	noteOnly := "note"
	results, err = ks.Search(ctx, SearchParams{Query: "Database", Type: &noteOnly, Limit: 10})
	if err != nil {
		t.Fatalf("Search notes only: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 note result, got %d", len(results))
	}
	if results[0].Type != "note" {
		t.Fatalf("expected type 'note', got %q", results[0].Type)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	results, err := ks.Search(ctx, SearchParams{Query: ""})
	if err != nil {
		t.Fatalf("Search empty: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty query, got %d", len(results))
	}
}

func TestSearchWithWorkspaceFilter(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	wid := ptrString("ws1")
	ks.CreateDecision(ctx, CreateDecisionParams{
		Title:       "SQLite in workspace",
		Decision:    "Use SQLite",
		WorkspaceID: wid,
	})
	ks.CreateDecision(ctx, CreateDecisionParams{
		Title:    "SQLite elsewhere",
		Decision: "Use SQLite too",
	})
	ks.CreateNote(ctx, CreateNoteParams{
		Message:     "SQLite note in workspace",
		WorkspaceID: wid,
	})

	// Search with workspace filter.
	results, err := ks.Search(ctx, SearchParams{Query: "SQLite", WorkspaceID: wid, Limit: 10})
	if err != nil {
		t.Fatalf("Search with workspace filter: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results in workspace, got %d", len(results))
	}
	for _, r := range results {
		if r.WorkspaceID == nil || *r.WorkspaceID != "ws1" {
			t.Fatalf("expected result in workspace 'ws1', got %v", r.WorkspaceID)
		}
	}
}

func TestSearchNoMatches(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	results, err := ks.Search(ctx, SearchParams{Query: "zzz_nonexistent_zzz", Limit: 10})
	if err != nil {
		t.Fatalf("Search no match: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

// ── UpdateDecision tests ────────────────────────────────────────────

func TestUpdateDecision(t *testing.T) {
	ks, bus, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	d, err := ks.CreateDecision(ctx, CreateDecisionParams{
		Title:    "Original title",
		Context:  "Original context",
		Decision: "Original decision",
	})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}

	// Subscribe to DecisionUpdated.
	var updatedEvent events.DecisionUpdatedPayload
	_, _ = bus.Subscribe(events.EventDecisionUpdated, func(_ context.Context, e events.Event) error {
		updatedEvent = e.Payload.(events.DecisionUpdatedPayload)
		return nil
	})

	// Update context and decision.
	updated, err := ks.UpdateDecision(ctx, d.ID, UpdateDecisionParams{
		Context:  ptrString("Updated context"),
		Decision: ptrString("Updated decision"),
	})
	if err != nil {
		t.Fatalf("UpdateDecision: %v", err)
	}

	if updated.Title != "Original title" {
		t.Fatalf("expected title unchanged, got %q", updated.Title)
	}
	if updated.Context != "Updated context" {
		t.Fatalf("expected context 'Updated context', got %q", updated.Context)
	}
	if updated.Decision != "Updated decision" {
		t.Fatalf("expected decision 'Updated decision', got %q", updated.Decision)
	}

	// Verify event.
	if updatedEvent.ID != d.ID {
		t.Fatalf("event ID mismatch: %q", updatedEvent.ID)
	}
	if updatedEvent.Title != "Original title" {
		t.Fatalf("expected event title 'Original title', got %q", updatedEvent.Title)
	}
	if updatedEvent.Status != "proposed" {
		t.Fatalf("expected event status 'proposed', got %q", updatedEvent.Status)
	}
	if updatedEvent.PreviousStatus != "proposed" {
		t.Fatalf("expected previous_status 'proposed', got %q", updatedEvent.PreviousStatus)
	}
}

func TestUpdateDecisionNonexistent(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := ks.UpdateDecision(ctx, "nonexistent", UpdateDecisionParams{
		Context: ptrString("anything"),
	})
	if err != ErrDecisionNotFound {
		t.Fatalf("expected ErrDecisionNotFound, got %v", err)
	}
}

func TestUpdateDecisionNoChanges(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	d, _ := ks.CreateDecision(ctx, CreateDecisionParams{Title: "Stable"})

	// Call with no fields set — should return current without error.
	result, err := ks.UpdateDecision(ctx, d.ID, UpdateDecisionParams{})
	if err != nil {
		t.Fatalf("UpdateDecision with no changes: %v", err)
	}
	if result.ID != d.ID {
		t.Fatalf("expected same decision, got %q", result.ID)
	}
}

func TestUpdateDecisionClearWorkspace(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	d, _ := ks.CreateDecision(ctx, CreateDecisionParams{
		Title:       "Workspace decision",
		WorkspaceID: ptrString("ws1"),
	})

	// Clear workspace by setting it to empty string.
	updated, err := ks.UpdateDecision(ctx, d.ID, UpdateDecisionParams{
		WorkspaceID: ptrString(""),
	})
	if err != nil {
		t.Fatalf("UpdateDecision clear workspace: %v", err)
	}

	if updated.WorkspaceID != nil && *updated.WorkspaceID != "" {
		t.Fatalf("expected workspace to be nil/empty, got %v", *updated.WorkspaceID)
	}
}

// ── DeleteDecision tests ──────────────────────────────────────────

func TestDeleteDecision(t *testing.T) {
	ks, bus, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	d, err := ks.CreateDecision(ctx, CreateDecisionParams{
		Title:    "To be deleted",
		Context:  "This decision will be removed",
		Decision: "Delete it",
	})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}

	// Subscribe to DecisionDeleted.
	var deletedEvent events.DecisionDeletedPayload
	_, _ = bus.Subscribe(events.EventDecisionDeleted, func(_ context.Context, e events.Event) error {
		deletedEvent = e.Payload.(events.DecisionDeletedPayload)
		return nil
	})

	// Delete the decision.
	err = ks.DeleteDecision(ctx, d.ID)
	if err != nil {
		t.Fatalf("DeleteDecision: %v", err)
	}

	// Verify it's gone.
	_, err = ks.GetDecision(ctx, d.ID)
	if err != ErrDecisionNotFound {
		t.Fatalf("expected ErrDecisionNotFound after delete, got %v", err)
	}

	// Verify event.
	if deletedEvent.ID != d.ID {
		t.Fatalf("event ID mismatch: %q vs %q", deletedEvent.ID, d.ID)
	}
	if deletedEvent.Title != "To be deleted" {
		t.Fatalf("event title mismatch: %q", deletedEvent.Title)
	}
	if deletedEvent.Status != "proposed" {
		t.Fatalf("event status mismatch: %q", deletedEvent.Status)
	}
}

func TestDeleteDecisionWithLinks(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	d, _ := ks.CreateDecision(ctx, CreateDecisionParams{Title: "Delete me"})

	// Add a link.
	_ = ks.LinkDecision(ctx, LinkDecisionParams{
		DecisionID: d.ID,
		LinkType:   "commit",
		Target:     "abc123",
	})

	// Verify link exists.
	links, _ := ks.GetDecisionLinks(ctx, d.ID)
	if len(links) != 1 {
		t.Fatalf("expected 1 link before delete, got %d", len(links))
	}

	// Delete the decision.
	if err := ks.DeleteDecision(ctx, d.ID); err != nil {
		t.Fatalf("DeleteDecision: %v", err)
	}

	// Verify links are cascade-deleted.
	links, _ = ks.GetDecisionLinks(ctx, d.ID)
	if len(links) != 0 {
		t.Fatalf("expected 0 links after cascade delete, got %d", len(links))
	}
}

func TestDeleteDecisionNonexistent(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	err := ks.DeleteDecision(ctx, "nonexistent")
	if err != ErrDecisionNotFound {
		t.Fatalf("expected ErrDecisionNotFound, got %v", err)
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

// ── Workspace CRUD tests ──────────────────────────────────────────

func TestCreateWorkspace(t *testing.T) {
	ks, bus, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	var createdEvent events.WorkspaceCreatedPayload
	_, _ = bus.Subscribe(events.EventWorkspaceCreated, func(_ context.Context, e events.Event) error {
		createdEvent = e.Payload.(events.WorkspaceCreatedPayload)
		return nil
	})

	w, err := ks.CreateWorkspace(ctx, CreateWorkspaceParams{
		Name:        "oauth",
		Description: "OAuth 2.0 authentication implementation",
		Tags:        []string{"auth", "security"},
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	if w.Name != "oauth" {
		t.Fatalf("expected name 'oauth', got %q", w.Name)
	}
	if w.Description != "OAuth 2.0 authentication implementation" {
		t.Fatalf("description mismatch: %q", w.Description)
	}
	if w.Status != "active" {
		t.Fatalf("expected status 'active', got %q", w.Status)
	}
	if len(w.Tags) != 2 || w.Tags[0] != "auth" {
		t.Fatalf("unexpected tags: %v", w.Tags)
	}

	// Verify event.
	if createdEvent.ID != w.ID {
		t.Fatalf("event ID mismatch: %q vs %q", createdEvent.ID, w.ID)
	}
	if createdEvent.Name != "oauth" {
		t.Fatalf("event name mismatch: %q", createdEvent.Name)
	}
}

func TestCreateWorkspaceDuplicate(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "dupe"})
	if err != nil {
		t.Fatalf("first CreateWorkspace: %v", err)
	}

	_, err = ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "dupe"})
	if err != ErrDuplicateWorkspace {
		t.Fatalf("expected ErrDuplicateWorkspace, got %v", err)
	}
}

func TestCreateWorkspaceEmptyName(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: ""})
	if err == nil {
		t.Fatal("expected error for empty workspace name")
	}
}

func TestGetWorkspace(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	created, err := ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "test-ws"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	fetched, err := ks.GetWorkspace(ctx, "test-ws")
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("ID mismatch: %q vs %q", fetched.ID, created.ID)
	}
	if fetched.Name != "test-ws" {
		t.Fatalf("name mismatch: %q", fetched.Name)
	}
}

func TestGetWorkspaceNotFound(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := ks.GetWorkspace(ctx, "nonexistent")
	if err != ErrWorkspaceNotFound {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestListWorkspaces(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "b-ws"})
	ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "a-ws"})

	workspaces, err := ks.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(workspaces) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(workspaces))
	}
	// Ordered by name ASC.
	if workspaces[0].Name != "a-ws" {
		t.Fatalf("expected first 'a-ws', got %q", workspaces[0].Name)
	}
	if workspaces[1].Name != "b-ws" {
		t.Fatalf("expected second 'b-ws', got %q", workspaces[1].Name)
	}
}

func TestUpdateWorkspace(t *testing.T) {
	ks, bus, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "updatable"})

	var updatedEvent events.WorkspaceUpdatedPayload
	_, _ = bus.Subscribe(events.EventWorkspaceUpdated, func(_ context.Context, e events.Event) error {
		updatedEvent = e.Payload.(events.WorkspaceUpdatedPayload)
		return nil
	})

	updated, err := ks.UpdateWorkspace(ctx, "updatable", UpdateWorkspaceParams{
		Description: ptrString("New description"),
		Tags:        []string{"updated"},
	})
	if err != nil {
		t.Fatalf("UpdateWorkspace: %v", err)
	}

	if updated.Description != "New description" {
		t.Fatalf("description mismatch: %q", updated.Description)
	}
	if len(updated.Tags) != 1 || updated.Tags[0] != "updated" {
		t.Fatalf("tags mismatch: %v", updated.Tags)
	}

	// Verify event.
	if updatedEvent.ID != updated.ID {
		t.Fatalf("event ID mismatch: %q", updatedEvent.ID)
	}
	if updatedEvent.Description != "New description" {
		t.Fatalf("event description mismatch: %q", updatedEvent.Description)
	}
}

func TestDeleteWorkspace(t *testing.T) {
	ks, bus, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "delete-me"})

	var deletedEvent events.WorkspaceDeletedPayload
	_, _ = bus.Subscribe(events.EventWorkspaceDeleted, func(_ context.Context, e events.Event) error {
		deletedEvent = e.Payload.(events.WorkspaceDeletedPayload)
		return nil
	})

	w, err := ks.DeleteWorkspace(ctx, "delete-me")
	if err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}
	if w.Name != "delete-me" {
		t.Fatalf("expected name 'delete-me', got %q", w.Name)
	}

	// Verify it's gone.
	_, err = ks.GetWorkspace(ctx, "delete-me")
	if err != ErrWorkspaceNotFound {
		t.Fatalf("expected ErrWorkspaceNotFound after delete, got %v", err)
	}

	// Verify event.
	if deletedEvent.ID != w.ID {
		t.Fatalf("event ID mismatch: %q", deletedEvent.ID)
	}
	if deletedEvent.Name != "delete-me" {
		t.Fatalf("event name mismatch: %q", deletedEvent.Name)
	}
}

func TestDeleteWorkspaceClearsAssociations(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "ws1"})

	// Create a decision and note scoped to the workspace.
	d, _ := ks.CreateDecision(ctx, CreateDecisionParams{
		Title:       "WS decision",
		WorkspaceID: ptrString("ws1"),
	})
	n, _ := ks.CreateNote(ctx, CreateNoteParams{
		Message:     "WS note",
		WorkspaceID: ptrString("ws1"),
	})

	// Delete the workspace.
	ks.DeleteWorkspace(ctx, "ws1")

	// Verify decision and note still exist but workspace_id is cleared.
	dReloaded, _ := ks.GetDecision(ctx, d.ID)
	if dReloaded.WorkspaceID != nil {
		t.Fatalf("expected decision workspace_id to be cleared, got %v", *dReloaded.WorkspaceID)
	}
	nReloaded, _ := ks.GetNote(ctx, n.ID)
	if nReloaded.WorkspaceID != nil {
		t.Fatalf("expected note workspace_id to be cleared, got %v", *nReloaded.WorkspaceID)
	}
}

func TestDeleteWorkspaceNotFound(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := ks.DeleteWorkspace(ctx, "nonexistent")
	if err != ErrWorkspaceNotFound {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

// ── Workspace files tests ──────────────────────────────────────────

func TestAddWorkspaceFile(t *testing.T) {
	ks, bus, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "ws"})

	var itemEvent events.WorkspaceItemAddedPayload
	_, _ = bus.Subscribe(events.EventWorkspaceItemAdded, func(_ context.Context, e events.Event) error {
		itemEvent = e.Payload.(events.WorkspaceItemAddedPayload)
		return nil
	})

	f, err := ks.AddWorkspaceFile(ctx, "ws", "src/main.go")
	if err != nil {
		t.Fatalf("AddWorkspaceFile: %v", err)
	}
	if f.Path != "src/main.go" {
		t.Fatalf("expected path 'src/main.go', got %q", f.Path)
	}

	// Verify event.
	if itemEvent.ItemType != "file" || itemEvent.ItemTarget != "src/main.go" {
		t.Fatalf("event mismatch: type=%q target=%q", itemEvent.ItemType, itemEvent.ItemTarget)
	}

	// List and verify.
	files, _ := ks.ListWorkspaceFiles(ctx, "ws")
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != "src/main.go" {
		t.Fatalf("expected path 'src/main.go', got %q", files[0].Path)
	}
}

func TestRemoveWorkspaceFile(t *testing.T) {
	ks, bus, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "ws"})
	ks.AddWorkspaceFile(ctx, "ws", "src/main.go")

	var removedEvent events.WorkspaceItemRemovedPayload
	_, _ = bus.Subscribe(events.EventWorkspaceItemRemoved, func(_ context.Context, e events.Event) error {
		removedEvent = e.Payload.(events.WorkspaceItemRemovedPayload)
		return nil
	})

	err := ks.RemoveWorkspaceFile(ctx, "ws", "src/main.go")
	if err != nil {
		t.Fatalf("RemoveWorkspaceFile: %v", err)
	}

	// Verify event.
	if removedEvent.ItemType != "file" || removedEvent.ItemTarget != "src/main.go" {
		t.Fatalf("event mismatch: type=%q target=%q", removedEvent.ItemType, removedEvent.ItemTarget)
	}

	// Verify removal.
	files, _ := ks.ListWorkspaceFiles(ctx, "ws")
	if len(files) != 0 {
		t.Fatalf("expected 0 files after removal, got %d", len(files))
	}
}

func TestRemoveWorkspaceFileNotFound(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "ws"})

	err := ks.RemoveWorkspaceFile(ctx, "ws", "nonexistent.go")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

// ── Workspace branches tests ────────────────────────────────────────

func TestAddWorkspaceBranch(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "ws"})

	b, err := ks.AddWorkspaceBranch(ctx, "ws", "feat/oauth")
	if err != nil {
		t.Fatalf("AddWorkspaceBranch: %v", err)
	}
	if b.BranchName != "feat/oauth" {
		t.Fatalf("expected branch 'feat/oauth', got %q", b.BranchName)
	}

	branches, _ := ks.ListWorkspaceBranches(ctx, "ws")
	if len(branches) != 1 {
		t.Fatalf("expected 1 branch, got %d", len(branches))
	}
}

func TestRemoveWorkspaceBranch(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "ws"})
	ks.AddWorkspaceBranch(ctx, "ws", "feat/oauth")

	err := ks.RemoveWorkspaceBranch(ctx, "ws", "feat/oauth")
	if err != nil {
		t.Fatalf("RemoveWorkspaceBranch: %v", err)
	}

	branches, _ := ks.ListWorkspaceBranches(ctx, "ws")
	if len(branches) != 0 {
		t.Fatalf("expected 0 branches after removal, got %d", len(branches))
	}
}

func TestRemoveWorkspaceBranchNotFound(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "ws"})

	err := ks.RemoveWorkspaceBranch(ctx, "ws", "nonexistent-branch")
	if err == nil {
		t.Fatal("expected error for nonexistent branch")
	}
}

// ── Workspace status tests ─────────────────────────────────────────

func TestGetWorkspaceStatus(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "full-ws"})
	ks.AddWorkspaceFile(ctx, "full-ws", "src/main.go")
	ks.AddWorkspaceBranch(ctx, "full-ws", "main")
	ks.CreateDecision(ctx, CreateDecisionParams{
		Title:       "WS decision",
		WorkspaceID: ptrString("full-ws"),
	})
	ks.CreateNote(ctx, CreateNoteParams{
		Message:     "WS note",
		WorkspaceID: ptrString("full-ws"),
	})

	status, err := ks.GetWorkspaceStatus(ctx, "full-ws")
	if err != nil {
		t.Fatalf("GetWorkspaceStatus: %v", err)
	}

	if status.Workspace.Name != "full-ws" {
		t.Fatalf("workspace name mismatch: %q", status.Workspace.Name)
	}
	if len(status.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(status.Files))
	}
	if len(status.Branches) != 1 {
		t.Fatalf("expected 1 branch, got %d", len(status.Branches))
	}
	if len(status.Decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(status.Decisions))
	}
	if len(status.Notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(status.Notes))
	}
	if status.ItemCount != 4 {
		t.Fatalf("expected 4 total items, got %d", status.ItemCount)
	}
	if status.LastActivity == 0 {
		t.Fatalf("expected non-zero last activity")
	}
}

func TestGetWorkspaceStatusEmpty(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "empty-ws"})

	status, err := ks.GetWorkspaceStatus(ctx, "empty-ws")
	if err != nil {
		t.Fatalf("GetWorkspaceStatus: %v", err)
	}

	if status.ItemCount != 0 {
		t.Fatalf("expected 0 items for empty workspace, got %d", status.ItemCount)
	}
	if len(status.Files) != 0 || len(status.Branches) != 0 {
		t.Fatalf("expected empty files/branches, got files=%d branches=%d",
			len(status.Files), len(status.Branches))
	}
}

func TestGetWorkspaceStatusNotFound(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := ks.GetWorkspaceStatus(ctx, "nonexistent")
	if err != ErrWorkspaceNotFound {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

// ── Workspace commits tests ──────────────────────────────────────

func TestAddWorkspaceCommit(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "ws"})

	c, err := ks.AddWorkspaceCommit(ctx, AddWorkspaceCommitParams{
		WorkspaceName: "ws",
		CommitSHA:     "abc123def456abc123def456abc123def456abc1",
		BranchName:    "main",
		Message:       "feat: implement OAuth",
	})
	if err != nil {
		t.Fatalf("AddWorkspaceCommit: %v", err)
	}

	if c.CommitSHA != "abc123def456abc123def456abc123def456abc1" {
		t.Fatalf("commit SHA mismatch: %q", c.CommitSHA)
	}
	if c.BranchName != "main" {
		t.Fatalf("branch name mismatch: %q", c.BranchName)
	}
	if c.Message != "feat: implement OAuth" {
		t.Fatalf("message mismatch: %q", c.Message)
	}

	// Verify last_commit_sha was updated.
	w, _ := ks.GetWorkspace(ctx, "ws")
	if w.LastCommitSHA != "abc123def456abc123def456abc123def456abc1" {
		t.Fatalf("expected last_commit_sha to be set, got %q", w.LastCommitSHA)
	}

	// Verify duplicate is ignored (UNIQUE constraint).
	_, err = ks.AddWorkspaceCommit(ctx, AddWorkspaceCommitParams{
		WorkspaceName: "ws",
		CommitSHA:     "abc123def456abc123def456abc123def456abc1",
		BranchName:    "main",
		Message:       "duplicate",
	})
	if err != nil {
		t.Fatalf("AddWorkspaceCommit duplicate: %v", err)
	}
}

func TestListWorkspaceCommits(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "ws"})

	ks.AddWorkspaceCommit(ctx, AddWorkspaceCommitParams{
		WorkspaceName: "ws",
		CommitSHA:     "aaa",
		BranchName:    "main",
		Message:       "first",
	})
	ks.AddWorkspaceCommit(ctx, AddWorkspaceCommitParams{
		WorkspaceName: "ws",
		CommitSHA:     "bbb",
		BranchName:    "main",
		Message:       "second",
	})

	commits, err := ks.ListWorkspaceCommits(ctx, "ws", 10)
	if err != nil {
		t.Fatalf("ListWorkspaceCommits: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}
	// Most recent first.
	if commits[0].CommitSHA != "bbb" {
		t.Fatalf("expected most recent 'bbb' first, got %q", commits[0].CommitSHA)
	}
}

func TestAddWorkspaceCommitNotFound(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := ks.AddWorkspaceCommit(ctx, AddWorkspaceCommitParams{
		WorkspaceName: "nonexistent",
		CommitSHA:     "abc",
	})
	if err != ErrWorkspaceNotFound {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestUpdateNoteCommitHash(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	n, _ := ks.CreateNote(ctx, CreateNoteParams{
		Message: "Test note",
	})

	// Update the commit hash.
	err := ks.UpdateNoteCommitHash(ctx, n.ID, "abc123def456")
	if err != nil {
		t.Fatalf("UpdateNoteCommitHash: %v", err)
	}

	// Verify the commit hash was set (note.id unchanged).
	nReloaded, _ := ks.GetNote(ctx, n.ID)
	if nReloaded.ID != n.ID {
		t.Fatalf("note ID changed: %q vs %q", nReloaded.ID, n.ID)
	}
	if nReloaded.CommitHash != "abc123def456" {
		t.Fatalf("expected commit_hash 'abc123def456', got %q", nReloaded.CommitHash)
	}
}

func TestUpdateNoteCommitHashNotFound(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	err := ks.UpdateNoteCommitHash(ctx, "nonexistent", "abc123")
	if err != ErrNoteNotFound {
		t.Fatalf("expected ErrNoteNotFound, got %v", err)
	}
}

func TestWorkspaceStatusWithCommits(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "ws"})
	ks.AddWorkspaceBranch(ctx, "ws", "main")

	// Add two commits.
	ks.AddWorkspaceCommit(ctx, AddWorkspaceCommitParams{
		WorkspaceName: "ws",
		CommitSHA:     "aaa",
		BranchName:    "main",
		Message:       "commit one",
	})
	ks.AddWorkspaceCommit(ctx, AddWorkspaceCommitParams{
		WorkspaceName: "ws",
		CommitSHA:     "bbb",
		BranchName:    "main",
		Message:       "commit two",
	})

	status, err := ks.GetWorkspaceStatus(ctx, "ws")
	if err != nil {
		t.Fatalf("GetWorkspaceStatus: %v", err)
	}

	if len(status.Commits) != 2 {
		t.Fatalf("expected 2 commits in status, got %d", len(status.Commits))
	}
	// Item count should include commits (1 branch + 2 commits = 3).
	if status.ItemCount != 3 {
		t.Fatalf("expected item count 3 (1 branch + 2 commits), got %d", status.ItemCount)
	}
	if status.Workspace.LastCommitSHA != "bbb" {
		t.Fatalf("expected last_commit_sha 'bbb', got %q", status.Workspace.LastCommitSHA)
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
