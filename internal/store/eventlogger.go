// Copyright 2026 Supun Hewagamage. MIT License.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/supunhg/got/internal/events"
)

// EventLogger subscribes to all events on a Bus and durably writes them
// to the event_log table. It is the bridge between the in-memory event
// bus and the persistent audit trail that external plugins can poll.
//
// Use NewEventLogger to create and start logging. Call Close to
// unsubscribe all handlers.
type EventLogger struct {
	db           *sql.DB
	bus          *events.Bus
	unsubscribes []func()
}

// NewEventLogger creates an EventLogger, subscribes to every event type
// the Knowledge Engine publishes, and returns the ready-to-use logger.
// The logger is tied to the lifetime of the caller — Close when done.
func NewEventLogger(db *sql.DB, bus *events.Bus) *EventLogger {
	el := &EventLogger{db: db, bus: bus}
	el.subscribe()
	return el
}

// allEventTypes lists every event type the Knowledge Engine publishes.
// Add new event constants here when they are defined.
var allEventTypes = []string{
	events.EventDecisionCreated,
	events.EventDecisionUpdated,
	events.EventDecisionSuperseded,
	events.EventDecisionLinked,
	events.EventNoteAdded,
	events.EventOnboardingStarted,
	events.EventOnboardingItemCovered,
	events.EventOnboardingCompleted,
}

// subscribe registers a handler for each event type. Errors from
// Subscribe are ignored (the bus is always open when this is called).
func (el *EventLogger) subscribe() {
	for _, et := range allEventTypes {
		// Capture eventType in the loop iteration (Go ≥1.22 does this
		// automatically, but we keep the pattern explicit).
		eventType := et
		unsub, err := el.bus.Subscribe(eventType, func(ctx context.Context, e events.Event) error {
			return el.logEvent(ctx, e)
		})
		if err == nil {
			el.unsubscribes = append(el.unsubscribes, unsub)
		}
	}
}

// logEvent marshals the event payload to JSON and inserts a row into the
// event_log table. Handler errors are returned so the Bus can surface
// them to the publisher (via errors.Join).
func (el *EventLogger) logEvent(ctx context.Context, e events.Event) error {
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return fmt.Errorf("eventlogger: marshal payload for %q: %w", e.Type, err)
	}

	_, err = el.db.ExecContext(ctx,
		"INSERT INTO event_log (event_type, payload, created_at) VALUES (?, ?, ?)",
		e.Type, string(payload), nowMS())
	if err != nil {
		return fmt.Errorf("eventlogger: insert event_log for %q: %w", e.Type, err)
	}

	return nil
}

// Close unsubscribes all registered event handlers. After Close returns
// the EventLogger will not write any more rows to the event_log table
// (though existing rows remain). Idempotent.
func (el *EventLogger) Close() {
	for _, unsub := range el.unsubscribes {
		unsub()
	}
	el.unsubscribes = nil
}
