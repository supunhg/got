package events

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestBus_PublishSubscribe(t *testing.T) {
	b := New()
	ctx := context.Background()

	var received []Event
	var mu sync.Mutex

	handler := func(_ context.Context, e Event) error {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
		return nil
	}

	_, err := b.Subscribe("test.event", handler)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	if err := b.Publish(ctx, "test.event", "hello"); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	mu.Lock()
	if len(received) != 1 {
		mu.Unlock()
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Type != "test.event" {
		mu.Unlock()
		t.Fatalf("expected type 'test.event', got %q", received[0].Type)
	}
	if got, ok := received[0].Payload.(string); !ok || got != "hello" {
		mu.Unlock()
		t.Fatalf("expected payload 'hello', got %v", received[0].Payload)
	}
	if received[0].Timestamp.IsZero() {
		mu.Unlock()
		t.Fatal("expected non-zero timestamp")
	}
	mu.Unlock()
}

func TestBus_MultipleHandlers(t *testing.T) {
	b := New()
	ctx := context.Background()

	var calls []int
	var mu sync.Mutex

	for i := range 3 {
		i := i
		_, _ = b.Subscribe("test.event", func(_ context.Context, e Event) error {
			mu.Lock()
			calls = append(calls, i)
			mu.Unlock()
			return nil
		})
	}

	if err := b.Publish(ctx, "test.event", nil); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	mu.Lock()
	if len(calls) != 3 {
		mu.Unlock()
		t.Fatalf("expected 3 handler calls, got %d", len(calls))
	}
	// Verify registration order.
	for i, v := range calls {
		if v != i {
			mu.Unlock()
			t.Fatalf("handler %d called out of order at position %d", v, i)
		}
	}
	mu.Unlock()
}

func TestBus_Unsubscribe(t *testing.T) {
	b := New()
	ctx := context.Background()

	var calls int
	var mu sync.Mutex

	h := func(_ context.Context, e Event) error {
		mu.Lock()
		calls++
		mu.Unlock()
		return nil
	}

	unsub, err := b.Subscribe("test.event", h)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// First publish — handler should fire.
	if err := b.Publish(ctx, "test.event", nil); err != nil {
		t.Fatalf("first Publish failed: %v", err)
	}

	unsub()

	// Second publish — handler should NOT fire.
	if err := b.Publish(ctx, "test.event", nil); err != nil {
		t.Fatalf("second Publish failed: %v", err)
	}

	mu.Lock()
	if calls != 1 {
		mu.Unlock()
		t.Fatalf("expected 1 handler call after unsubscribe, got %d", calls)
	}
	mu.Unlock()
}

func TestBus_DoubleUnsubscribeIsNoop(t *testing.T) {
	b := New()

	h := func(_ context.Context, e Event) error { return nil }
	unsub, err := b.Subscribe("test.event", h)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Should not panic or error.
	unsub()
	unsub()
}

func TestBus_PublishNoHandlers(t *testing.T) {
	b := New()
	ctx := context.Background()

	// Publishing to an event type with no handlers should succeed without error.
	if err := b.Publish(ctx, "nonexistent", nil); err != nil {
		t.Fatalf("Publish to unregistered type failed: %v", err)
	}
}

func TestBus_ClosePreventsSubscribe(t *testing.T) {
	b := New()

	if err := b.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	_, err := b.Subscribe("test.event", func(_ context.Context, e Event) error { return nil })
	if !errors.Is(err, ErrBusClosed) {
		t.Fatalf("expected ErrBusClosed, got %v", err)
	}
}

func TestBus_ClosePreventsPublish(t *testing.T) {
	b := New()
	ctx := context.Background()

	if err := b.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	err := b.Publish(ctx, "test.event", nil)
	if !errors.Is(err, ErrBusClosed) {
		t.Fatalf("expected ErrBusClosed, got %v", err)
	}
}

func TestBus_DoubleClose(t *testing.T) {
	b := New()

	if err := b.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}

	err := b.Close()
	if !errors.Is(err, ErrBusClosed) {
		t.Fatalf("expected ErrBusClosed on second Close, got %v", err)
	}
}

func TestBus_HandlerError(t *testing.T) {
	b := New()
	ctx := context.Background()

	handlerErr := errors.New("handler failure")

	_, _ = b.Subscribe("test.event", func(_ context.Context, e Event) error {
		return handlerErr
	})

	err := b.Publish(ctx, "test.event", nil)
	if err == nil {
		t.Fatal("expected error from handler, got nil")
	}
	if !errors.Is(err, handlerErr) {
		t.Fatalf("expected handlerErr in chain, got %v", err)
	}
}

func TestBus_MultipleHandlerErrors(t *testing.T) {
	b := New()
	ctx := context.Background()

	errA := errors.New("error A")
	errB := errors.New("error B")

	_, _ = b.Subscribe("test.event", func(_ context.Context, e Event) error { return errA })
	_, _ = b.Subscribe("test.event", func(_ context.Context, e Event) error { return errB })

	err := b.Publish(ctx, "test.event", nil)
	if err == nil {
		t.Fatal("expected errors from handlers, got nil")
	}
	if !errors.Is(err, errA) {
		t.Fatalf("expected errA in chain, got %v", err)
	}
	if !errors.Is(err, errB) {
		t.Fatalf("expected errB in chain, got %v", err)
	}
}

func TestBus_Concurrency(t *testing.T) {
	b := New()
	ctx := context.Background()

	var mu sync.Mutex
	var received int

	// Register handler.
	_, _ = b.Subscribe("test.event", func(_ context.Context, e Event) error {
		mu.Lock()
		received++
		mu.Unlock()
		time.Sleep(time.Microsecond) // tiny delay to expose races
		return nil
	})

	var wg sync.WaitGroup

	// Concurrent publishers.
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := b.Publish(ctx, "test.event", "data"); err != nil {
				t.Errorf("concurrent Publish failed: %v", err)
			}
		}()
	}

	// Concurrent subscriber.
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := b.Subscribe("test.event", func(_ context.Context, e Event) error { return nil })
		if err != nil {
			t.Errorf("concurrent Subscribe failed: %v", err)
		}
	}()

	wg.Wait()

	mu.Lock()
	if received != 10 {
		mu.Unlock()
		t.Fatalf("expected 10 handler calls, got %d", received)
	}
	mu.Unlock()
}

func TestBus_CloseWaitsForInFlightPublish(t *testing.T) {
	b := New()
	ctx := context.Background()

	// A slow handler that signals when it starts and blocks until unblocked.
	started := make(chan struct{})
	unblock := make(chan struct{})

	_, _ = b.Subscribe("test.event", func(_ context.Context, e Event) error {
		close(started)
		<-unblock
		return nil
	})

	// Start a publish in the background.
	publishDone := make(chan struct{})
	go func() {
		_ = b.Publish(ctx, "test.event", nil)
		close(publishDone)
	}()

	// Wait for the handler to start.
	<-started

	// Close in the background — it should block until the publish finishes.
	closeDone := make(chan struct{})
	go func() {
		_ = b.Close()
		close(closeDone)
	}()

	// Give Close a moment to try acquiring the write lock.
	time.Sleep(10 * time.Millisecond)

	select {
	case <-closeDone:
		t.Fatal("Close returned before Publish completed")
	default:
	}

	// Unblock the handler.
	close(unblock)

	// Now Close should be able to proceed.
	<-publishDone
	<-closeDone
}

func TestBus_TimestampIsSet(t *testing.T) {
	b := New()
	ctx := context.Background()

	before := time.Now()

	var e Event
	_, _ = b.Subscribe("test.event", func(_ context.Context, ev Event) error {
		e = ev
		return nil
	})

	if err := b.Publish(ctx, "test.event", nil); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	if e.Timestamp.Before(before) {
		t.Fatal("timestamp should not be before publish call")
	}
}

// ── Knowledge Engine event type tests ──────────────────────────────

func TestBus_DecisionCreatedEvent(t *testing.T) {
	b := New()
	ctx := context.Background()

	var payload DecisionCreatedPayload
	_, _ = b.Subscribe(EventDecisionCreated, func(_ context.Context, e Event) error {
		payload = e.Payload.(DecisionCreatedPayload)
		return nil
	})

	expected := DecisionCreatedPayload{
		ID:     "01JQZ3ZABC",
		Title:  "Use SQLite",
		Status: "proposed",
	}

	if err := b.Publish(ctx, EventDecisionCreated, expected); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	if payload.ID != expected.ID || payload.Title != expected.Title || payload.Status != expected.Status {
		t.Fatalf("got %+v, want %+v", payload, expected)
	}
}

func TestBus_NoteAddedEvent(t *testing.T) {
	b := New()
	ctx := context.Background()

	var payload NoteAddedPayload
	_, _ = b.Subscribe(EventNoteAdded, func(_ context.Context, e Event) error {
		payload = e.Payload.(NoteAddedPayload)
		return nil
	})

	expected := NoteAddedPayload{
		ID:         "01JQZ4ZABC",
		Branch:     "main",
		CommitHash: "abc123",
	}

	if err := b.Publish(ctx, EventNoteAdded, expected); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	if payload.ID != expected.ID || payload.Branch != expected.Branch || payload.CommitHash != expected.CommitHash {
		t.Fatalf("got %+v, want %+v", payload, expected)
	}
}

func TestBus_OnboardingStartedEvent(t *testing.T) {
	b := New()
	ctx := context.Background()

	var payload OnboardingStartedPayload
	_, _ = b.Subscribe(EventOnboardingStarted, func(_ context.Context, e Event) error {
		payload = e.Payload.(OnboardingStartedPayload)
		return nil
	})

	expected := OnboardingStartedPayload{
		SessionID:   "01JQZ5ZABC",
		Participant: "alice",
		ItemCount:   42,
	}

	if err := b.Publish(ctx, EventOnboardingStarted, expected); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	if payload.SessionID != expected.SessionID || payload.Participant != expected.Participant || payload.ItemCount != expected.ItemCount {
		t.Fatalf("got %+v, want %+v", payload, expected)
	}
}

// ── Handler unsubscribe after subscribe ────────────────────────────

func TestBus_UnsubscribePreservesOtherHandlers(t *testing.T) {
	b := New()
	ctx := context.Background()

	var calls []string
	var mu sync.Mutex

	h1 := func(_ context.Context, e Event) error {
		mu.Lock()
		calls = append(calls, "h1")
		mu.Unlock()
		return nil
	}
	h2 := func(_ context.Context, e Event) error {
		mu.Lock()
		calls = append(calls, "h2")
		mu.Unlock()
		return nil
	}
	h3 := func(_ context.Context, e Event) error {
		mu.Lock()
		calls = append(calls, "h3")
		mu.Unlock()
		return nil
	}

	unsub1, _ := b.Subscribe("test.event", h1)
	_, _ = b.Subscribe("test.event", h2)
	unsub3, _ := b.Subscribe("test.event", h3)

	// Remove h1 and h3.
	unsub1()
	unsub3()

	if err := b.Publish(ctx, "test.event", nil); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	mu.Lock()
	if len(calls) != 1 || calls[0] != "h2" {
		mu.Unlock()
		t.Fatalf("expected only h2 to be called, got %v", calls)
	}
	mu.Unlock()
}
