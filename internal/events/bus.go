// Copyright 2026 Supun Hewagamage. MIT License.
package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// Sentinel errors returned by Bus methods.
var ErrBusClosed = errors.New("events: bus is closed")

// subscription pairs a handler with its unique ID so unsubscribe can find
// and remove it without relying on function pointer comparison (which Go
// does not allow for closures).
type subscription struct {
	id      uint64
	handler Handler
}

// Bus is a minimal in-memory event bus for in-process pub/sub. It is
// thread-safe and intentionally simple:
//   - Dispatch is synchronous — Publish blocks while handlers execute.
//   - Handlers are called in registration order.
//   - A closed bus refuses new Subscribe and Publish calls.
//
// For the v0.4 Knowledge Engine this is sufficient. A future version may
// add async dispatch, timeouts, handler worker pools, or delivery guarantees.
type Bus struct {
	mu     sync.RWMutex
	subs   map[string][]subscription
	nextID uint64
	closed bool

	// Event history for replay and NDJSON streaming
	history       []Event
	historyMu     sync.RWMutex
	maxHistory    int
	subscribers   []chan Event
	subscribersMu sync.RWMutex
}

// New creates and returns an initialized, ready-to-use Bus.
func New() *Bus {
	return &Bus{
		subs:       make(map[string][]subscription),
		maxHistory: 1000, // keep last 1000 events
	}
}

// Subscribe registers handler for the given eventType. It returns an
// unsubscribe function that removes the handler. Calling unsubscribe more
// than once is a no-op.
//
// Subscribe returns ErrBusClosed if the bus has been closed.
func (b *Bus) Subscribe(eventType string, handler Handler) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, ErrBusClosed
	}

	id := b.nextID
	b.nextID++

	b.subs[eventType] = append(b.subs[eventType], subscription{id: id, handler: handler})

	done := false

	unsubscribe := func() {
		if done {
			return
		}
		done = true

		b.mu.Lock()
		defer b.mu.Unlock()

		subs := b.subs[eventType]
		for i, sub := range subs {
			if sub.id == id {
				b.subs[eventType] = append(subs[:i], subs[i+1:]...)
				return
			}
		}
	}

	return unsubscribe, nil
}

// Publish dispatches an event to all handlers registered for eventType.
// Handlers are called synchronously in registration order. If any handler
// returns an error, Publish continues dispatching to remaining handlers
// and returns all errors aggregated via errors.Join.
//
// Publish sets Event.Timestamp to the current time before dispatch.
// Events are stored in history for replay and NDJSON streaming.
//
// Publish returns ErrBusClosed if the bus has been closed.
func (b *Bus) Publish(ctx context.Context, eventType string, payload any) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return ErrBusClosed
	}

	subs := b.subs[eventType]
	if len(subs) == 0 && len(b.subscribers) == 0 {
		return nil
	}

	e := Event{
		Type:      eventType,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	// Store in history
	b.historyMu.Lock()
	b.history = append(b.history, e)
	if len(b.history) > b.maxHistory {
		b.history = b.history[len(b.history)-b.maxHistory:]
	}
	b.historyMu.Unlock()

	// Notify NDJSON subscribers
	b.subscribersMu.RLock()
	for _, ch := range b.subscribers {
		select {
		case ch <- e:
		default:
			// Don't block if subscriber is full
		}
	}
	b.subscribersMu.RUnlock()

	var errs []error
	for _, sub := range subs {
		if err := sub.handler(ctx, e); err != nil {
			errs = append(errs, fmt.Errorf("events: handler for %q: %w", eventType, err))
		}
	}

	return errors.Join(errs...)
}

// Replay returns all events from history, optionally filtered by event type.
// If eventType is empty, all events are returned.
func (b *Bus) Replay(eventType string) []Event {
	b.historyMu.RLock()
	defer b.historyMu.RUnlock()

	if eventType == "" {
		result := make([]Event, len(b.history))
		copy(result, b.history)
		return result
	}

	var result []Event
	for _, e := range b.history {
		if e.Type == eventType {
			result = append(result, e)
		}
	}
	return result
}

// SubscribeNDJSON creates a new NDJSON event stream. It returns a channel
// that receives events and an unsubscribe function. Events are written to
// the provided writer as NDJSON (one JSON object per line).
func (b *Bus) SubscribeNDJSON(writer io.Writer) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, ErrBusClosed
	}

	ch := make(chan Event, 100)
	b.subscribersMu.Lock()
	b.subscribers = append(b.subscribers, ch)
	b.subscribersMu.Unlock()

	// Start goroutine to write events as NDJSON
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range ch {
			data, err := json.Marshal(e)
			if err != nil {
				continue
			}
			data = append(data, '\n')
			_, _ = writer.Write(data)
		}
	}()

	unsubscribe := func() {
		b.subscribersMu.Lock()
		defer b.subscribersMu.Unlock()
		for i, sub := range b.subscribers {
			if sub == ch {
				b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)
				break
			}
		}
		close(ch)
		<-done
	}

	return unsubscribe, nil
}

// Close prevents any further Subscribe or Publish calls and clears all
// registered handlers. It waits for any in-flight Publish calls to complete
// (via the write lock) and then returns.
//
// Calling Close more than once returns ErrBusClosed.
func (b *Bus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return ErrBusClosed
	}

	b.closed = true
	b.subs = make(map[string][]subscription) // clear all handlers
	return nil
}
