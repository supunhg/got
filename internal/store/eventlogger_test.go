package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/supunhg/got/internal/events"
)

func TestEventLogger_LogsDecisionCreated(t *testing.T) {
	el, bus, db, cleanup := newTestEventLogger(t)
	defer cleanup()

	ctx := context.Background()
	bus.Publish(ctx, events.EventDecisionCreated, events.DecisionCreatedPayload{
		ID:     "01JQZ3ZABC",
		Title:  "Test decision",
		Status: "proposed",
	})

	var eventType, payload string
	var createdAt int64
	err := db.QueryRow("SELECT event_type, payload, created_at FROM event_log").Scan(&eventType, &payload, &createdAt)
	if err != nil {
		t.Fatalf("query event_log: %v", err)
	}

	if eventType != events.EventDecisionCreated {
		t.Fatalf("expected event_type %q, got %q", events.EventDecisionCreated, eventType)
	}

	var decoded events.DecisionCreatedPayload
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if decoded.ID != "01JQZ3ZABC" {
		t.Fatalf("expected ID '01JQZ3ZABC', got %q", decoded.ID)
	}
	if decoded.Title != "Test decision" {
		t.Fatalf("expected Title 'Test decision', got %q", decoded.Title)
	}
	if createdAt == 0 {
		t.Fatal("expected non-zero created_at")
	}

	el.Close()
}

func TestEventLogger_LogsAllEventTypes(t *testing.T) {
	el, bus, db, cleanup := newTestEventLogger(t)
	defer cleanup()
	ctx := context.Background()

	// Publish one of each event type.
	bus.Publish(ctx, events.EventDecisionCreated, events.DecisionCreatedPayload{ID: "d1", Title: "D1"})
	bus.Publish(ctx, events.EventDecisionUpdated, events.DecisionUpdatedPayload{ID: "d1", Status: "accepted", PreviousStatus: "proposed"})
	bus.Publish(ctx, events.EventDecisionSuperseded, events.DecisionSupersededPayload{ID: "d1", NewID: "d2", OldStatus: "accepted"})
	bus.Publish(ctx, events.EventDecisionLinked, events.DecisionLinkedPayload{DecisionID: "d1", LinkType: "commit", Target: "abc123"})
	bus.Publish(ctx, events.EventNoteAdded, events.NoteAddedPayload{ID: "n1"})
	bus.Publish(ctx, events.EventOnboardingStarted, events.OnboardingStartedPayload{SessionID: "s1", Participant: "alice", ItemCount: 5})
	bus.Publish(ctx, events.EventOnboardingItemCovered, events.OnboardingItemCoveredPayload{SessionID: "s1", ItemType: "decision", ItemTarget: "d1"})
	bus.Publish(ctx, events.EventOnboardingCompleted, events.OnboardingCompletedPayload{SessionID: "s1", Participant: "alice", TotalItems: 5})

	// Count rows.
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM event_log").Scan(&count); err != nil {
		t.Fatalf("count event_log: %v", err)
	}
	if count != 8 {
		t.Fatalf("expected 8 event_log rows, got %d", count)
	}

	// Verify event types are distinct and correct.
	rows, err := db.Query("SELECT event_type FROM event_log ORDER BY id")
	if err != nil {
		t.Fatalf("query event_log: %v", err)
	}
	defer rows.Close()

	var types []string
	for rows.Next() {
		var et string
		if err := rows.Scan(&et); err != nil {
			t.Fatalf("scan event type: %v", err)
		}
		types = append(types, et)
	}

	expected := []string{
		events.EventDecisionCreated,
		events.EventDecisionUpdated,
		events.EventDecisionSuperseded,
		events.EventDecisionLinked,
		events.EventNoteAdded,
		events.EventOnboardingStarted,
		events.EventOnboardingItemCovered,
		events.EventOnboardingCompleted,
	}
	for i, et := range types {
		if et != expected[i] {
			t.Fatalf("index %d: expected event type %q, got %q", i, expected[i], et)
		}
	}

	el.Close()
}

func TestEventLogger_CloseStopsLogging(t *testing.T) {
	el, bus, db, cleanup := newTestEventLogger(t)
	defer cleanup()
	ctx := context.Background()

	// Publish before close.
	bus.Publish(ctx, events.EventDecisionCreated, events.DecisionCreatedPayload{ID: "before"})

	// Close.
	el.Close()

	// Publish after close.
	bus.Publish(ctx, events.EventDecisionCreated, events.DecisionCreatedPayload{ID: "after"})

	// Should only have 1 row.
	var count int
	db.QueryRow("SELECT COUNT(*) FROM event_log").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 row after close, got %d", count)
	}
}

func TestEventLogger_HandlerErrorDoesNotPanic(t *testing.T) {
	// If the DB is closed, logEvent will return an error. The bus should
	// propagate it through Publish; no panic should occur.
	dir, _ := os.MkdirTemp("", "got-test-eventlogger-err-*")
	defer os.RemoveAll(dir)

	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	db := s.DB()

	bus := events.New()
	_ = NewEventLogger(db, bus)

	// Close the DB to force an error on the next write.
	s.Close()

	ctx := context.Background()
	err = bus.Publish(ctx, events.EventDecisionCreated, events.DecisionCreatedPayload{ID: "fail"})
	if err == nil {
		t.Fatal("expected error from EventLogger after DB closed, got nil")
	}
}

// ── Helpers ─────────────────────────────────────────────────────────

// newTestEventLogger creates a temp SQLite DB, wires an EventLogger to
// it, and returns (el, bus, db, cleanup). Tests log events by publishing
// on the returned bus and then querying the returned db.
func newTestEventLogger(t *testing.T) (*EventLogger, *events.Bus, *sql.DB, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "got-test-eventlogger-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}

	dbPath := filepath.Join(dir, "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	bus := events.New()
	el := NewEventLogger(s.DB(), bus)

	cleanup := func() {
		el.Close()
		s.Close()
		os.RemoveAll(dir)
	}

	return el, bus, s.DB(), cleanup
}
