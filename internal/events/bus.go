package events

import (
	"context"
	"errors"
	"fmt"
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
	mu      sync.RWMutex
	subs    map[string][]subscription
	nextID  uint64
	closed  bool
}

// New creates and returns an initialised, ready-to-use Bus.
func New() *Bus {
	return &Bus{
		subs: make(map[string][]subscription),
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
//
// Publish returns ErrBusClosed if the bus has been closed.
func (b *Bus) Publish(ctx context.Context, eventType string, payload any) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return ErrBusClosed
	}

	subs := b.subs[eventType]
	if len(subs) == 0 {
		return nil
	}

	e := Event{
		Type:      eventType,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	var errs []error
	for _, sub := range subs {
		if err := sub.handler(ctx, e); err != nil {
			errs = append(errs, fmt.Errorf("events: handler for %q: %w", eventType, err))
		}
	}

	return errors.Join(errs...)
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
