package eventbus

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	storepkg "github.com/got-sh/got/internal/store"
)

// newTestBus opens a temporary SQLite store and returns a bus
// plus a cleanup function. The store file lives in t.TempDir() so
// it is removed at the end of the test.
func newTestBus(t *testing.T) (*Bus, *storepkg.Store, func()) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "got.db")
	s, err := storepkg.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	bus, err := New(s, "tester", dbPath)
	if err != nil {
		_ = s.Close()
		t.Fatalf("eventbus.New: %v", err)
	}
	return bus, s, func() { _ = s.Close() }
}

func TestNew_RejectsNilStore(t *testing.T) {
	if _, err := New(nil, "", ""); !errors.Is(err, ErrStoreRequired) {
		t.Fatalf("want ErrStoreRequired, got %v", err)
	}
}

func TestParseTopic_AllowsKnown(t *testing.T) {
	for _, name := range AllTopics() {
		if _, err := ParseTopic(string(name)); err != nil {
			t.Fatalf("ParseTopic(%q) error: %v", name, err)
		}
	}
}

func TestParseTopic_RejectsUnknown(t *testing.T) {
	if _, err := ParseTopic("BogusTopic"); !errors.Is(err, ErrUnknownTopic) {
		t.Fatalf("want ErrUnknownTopic, got %v", err)
	}
}

func TestSubscribe_RejectsNilCallback(t *testing.T) {
	bus, _, cleanup := newTestBus(t)
	defer cleanup()
	if _, err := bus.Subscribe(TopicWorkspaceCreated, nil, nil); !errors.Is(err, ErrNilSubscriber) {
		t.Fatalf("want ErrNilSubscriber, got %v", err)
	}
}

func TestSubscribe_RejectsUnknownTopic(t *testing.T) {
	bus, _, cleanup := newTestBus(t)
	defer cleanup()
	if _, err := bus.Subscribe(Topic("Bogus"), nil, func(context.Context, *Event) error { return nil }); !errors.Is(err, ErrUnknownTopic) {
		t.Fatalf("want ErrUnknownTopic, got %v", err)
	}
}

func TestPublish_PersistsBeforeDispatch(t *testing.T) {
	bus, s, cleanup := newTestBus(t)
	defer cleanup()

	var got *Event
	subCtx, subCancel := context.WithCancel(context.Background())
	defer subCancel()
	if _, err := bus.Subscribe(TopicWorkspaceCreated, subCtx, func(_ context.Context, e *Event) error {
		got = e
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	e, err := bus.Publish(context.Background(), TopicWorkspaceCreated,
		WithPayload(Payload{"name": "alpha", "title": "Alpha"}),
		WithTimestamp(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)),
	)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if e.ID == "" {
		t.Fatal("Publish returned empty ID")
	}
	if e.Topic != TopicWorkspaceCreated {
		t.Fatalf("topic mismatch: %q", e.Topic)
	}

	// Subscriber must have been called.
	if got == nil {
		t.Fatal("subscriber was not called")
	}
	if got.ID != e.ID {
		t.Fatalf("subscriber saw id %q, want %q", got.ID, e.ID)
	}
	if got.Payload["name"] != "alpha" {
		t.Fatalf("subscriber payload missing name: %+v", got.Payload)
	}

	// The event must be durable.
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE id = ?`, e.ID).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 row in events, got %d", n)
	}
}

func TestPublish_RejectsUnknownTopic(t *testing.T) {
	bus, _, cleanup := newTestBus(t)
	defer cleanup()
	if _, err := bus.Publish(context.Background(), Topic("Bogus")); !errors.Is(err, ErrUnknownTopic) {
		t.Fatalf("want ErrUnknownTopic, got %v", err)
	}
}

func TestPublish_DefaultsActorAndSource(t *testing.T) {
	bus, _, cleanup := newTestBus(t)
	defer cleanup()
	e, err := bus.Publish(context.Background(), TopicCommitCreated)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if e.Actor == "" {
		t.Fatal("actor should default to current user (test env), got empty")
	}
	if e.Source == "" {
		t.Fatal("source should default to the .got/got.db path, got empty")
	}
}

func TestPublishRaw_RejectsEmptyID(t *testing.T) {
	bus, _, cleanup := newTestBus(t)
	defer cleanup()
	if err := bus.PublishRaw(context.Background(), &Event{Topic: TopicCommitCreated}); !errors.Is(err, ErrEmptyID) {
		t.Fatalf("want ErrEmptyID, got %v", err)
	}
}

func TestPublishRaw_RejectsUnknownTopic(t *testing.T) {
	bus, _, cleanup := newTestBus(t)
	defer cleanup()
	if err := bus.PublishRaw(context.Background(), &Event{ID: "x", Topic: Topic("Bogus")}); !errors.Is(err, ErrUnknownTopic) {
		t.Fatalf("want ErrUnknownTopic, got %v", err)
	}
}

func TestUnsubscribe_RemovesRegistration(t *testing.T) {
	bus, _, cleanup := newTestBus(t)
	defer cleanup()
	var calls int32
	id, err := bus.Subscribe(TopicCommitCreated, nil, func(context.Context, *Event) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if _, err := bus.Publish(context.Background(), TopicCommitCreated); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := bus.Unsubscribe(TopicCommitCreated, id); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	if _, err := bus.Publish(context.Background(), TopicCommitCreated); err != nil {
		t.Fatalf("Publish 2: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("subscriber called %d times, want 1", got)
	}
	// Double unsubscribe is a no-op.
	if err := bus.Unsubscribe(TopicCommitCreated, id); err != nil {
		t.Fatalf("Unsubscribe again: %v", err)
	}
}

func TestSubscribe_ContextCancelRemovesEntry(t *testing.T) {
	bus, _, cleanup := newTestBus(t)
	defer cleanup()
	var calls int32
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := bus.Subscribe(TopicBranchCreated, ctx, func(context.Context, *Event) error {
		atomic.AddInt32(&calls, 1)
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	cancel()
	// Give the auto-unsubscribe goroutine a moment to run.
	time.Sleep(10 * time.Millisecond)
	if _, err := bus.Publish(context.Background(), TopicBranchCreated); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("subscriber called %d times after cancel, want 0", got)
	}
}

func TestPublish_ContinuesAfterSubscriberPanic(t *testing.T) {
	bus, _, cleanup := newTestBus(t)
	defer cleanup()
	var after int32
	if _, err := bus.Subscribe(TopicCommitCreated, nil, func(context.Context, *Event) error {
		panic("boom")
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if _, err := bus.Subscribe(TopicCommitCreated, nil, func(context.Context, *Event) error {
		atomic.AddInt32(&after, 1)
		return nil
	}); err != nil {
		t.Fatalf("Subscribe 2: %v", err)
	}
	if _, err := bus.Publish(context.Background(), TopicCommitCreated); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if got := atomic.LoadInt32(&after); got != 1 {
		t.Fatalf("second subscriber called %d times, want 1", got)
	}
}

func TestReplay_NewestFirst(t *testing.T) {
	bus, _, cleanup := newTestBus(t)
	defer cleanup()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		_, err := bus.Publish(context.Background(), TopicCommitCreated,
			WithTimestamp(now.Add(time.Duration(i)*time.Second)),
		)
		if err != nil {
			t.Fatalf("Publish %d: %v", i, err)
		}
	}
	events, err := bus.Replay(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("want 3 events, got %d", len(events))
	}
	if !events[0].CreatedAt.After(events[1].CreatedAt) {
		t.Fatalf("Replay not newest-first: %+v", events)
	}
}

func TestReplay_FilterByTopic(t *testing.T) {
	bus, _, cleanup := newTestBus(t)
	defer cleanup()
	if _, err := bus.Publish(context.Background(), TopicCommitCreated); err != nil {
		t.Fatalf("Publish CommitCreated: %v", err)
	}
	if _, err := bus.Publish(context.Background(), TopicBranchCreated); err != nil {
		t.Fatalf("Publish BranchCreated: %v", err)
	}
	events, err := bus.Replay(context.Background(), TopicCommitCreated, 10)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].Topic != TopicCommitCreated {
		t.Fatalf("filter leaked: %+v", events[0])
	}
}

func TestReplay_AppliesDefaultLimit(t *testing.T) {
	bus, _, cleanup := newTestBus(t)
	defer cleanup()
	for i := 0; i < 150; i++ {
		_, err := bus.Publish(context.Background(), TopicCommitCreated)
		if err != nil {
			t.Fatalf("Publish %d: %v", i, err)
		}
	}
	events, err := bus.Replay(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(events) != 100 {
		t.Fatalf("default limit not applied: got %d", len(events))
	}
}

func TestReplay_RejectsUnknownTopic(t *testing.T) {
	bus, _, cleanup := newTestBus(t)
	defer cleanup()
	if _, err := bus.Replay(context.Background(), Topic("Bogus"), 10); !errors.Is(err, ErrUnknownTopic) {
		t.Fatalf("want ErrUnknownTopic, got %v", err)
	}
}

func TestReplaySince_OldestFirst(t *testing.T) {
	bus, _, cleanup := newTestBus(t)
	defer cleanup()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		_, err := bus.Publish(context.Background(), TopicCommitCreated,
			WithTimestamp(base.Add(time.Duration(i)*time.Hour)),
		)
		if err != nil {
			t.Fatalf("Publish %d: %v", i, err)
		}
	}
	events, err := bus.ReplaySince(context.Background(), "", base.Add(2*time.Hour), 0)
	if err != nil {
		t.Fatalf("ReplaySince: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("want 3 events, got %d", len(events))
	}
	if !events[0].CreatedAt.Before(events[1].CreatedAt) {
		t.Fatalf("ReplaySince not oldest-first: %+v", events)
	}
}

func TestCounts_AggregatesPerTopic(t *testing.T) {
	bus, _, cleanup := newTestBus(t)
	defer cleanup()
	if _, err := bus.Publish(context.Background(), TopicCommitCreated); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if _, err := bus.Publish(context.Background(), TopicCommitCreated); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if _, err := bus.Publish(context.Background(), TopicBranchCreated); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	counts, err := bus.Counts(context.Background())
	if err != nil {
		t.Fatalf("Counts: %v", err)
	}
	if counts[TopicCommitCreated] != 2 {
		t.Fatalf("CommitCreated: want 2, got %d", counts[TopicCommitCreated])
	}
	if counts[TopicBranchCreated] != 1 {
		t.Fatalf("BranchCreated: want 1, got %d", counts[TopicBranchCreated])
	}
}

func TestPublish_ConcurrentSafe(t *testing.T) {
	bus, _, cleanup := newTestBus(t)
	defer cleanup()
	var (
		mu     sync.Mutex
		seen   = map[string]int{}
	)
	if _, err := bus.Subscribe(TopicCommitCreated, nil, func(_ context.Context, e *Event) error {
		mu.Lock()
		seen[e.ID]++
		mu.Unlock()
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	const N = 50
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := bus.Publish(context.Background(), TopicCommitCreated); err != nil {
				t.Errorf("Publish: %v", err)
			}
		}()
	}
	wg.Wait()
	if len(seen) != N {
		t.Fatalf("subscriber saw %d unique events, want %d", len(seen), N)
	}
}

func TestNewEventID_IsUniqueAndSortable(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	seen := map[string]bool{}
	prev := ""
	for i := 0; i < 200; i++ {
		id := newEventID(base.Add(time.Duration(i) * time.Millisecond))
		if seen[id] {
			t.Fatalf("duplicate id: %q", id)
		}
		seen[id] = true
		if prev != "" && id <= prev {
			t.Fatalf("ids not strictly increasing: %q <= %q", id, prev)
		}
		prev = id
	}
}

func TestMarshalPayload_HandlesStructs(t *testing.T) {
	type evt struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	p, err := MarshalPayload(evt{Name: "alpha", N: 7})
	if err != nil {
		t.Fatalf("MarshalPayload: %v", err)
	}
	if p["name"] != "alpha" || p["n"].(float64) != 7 {
		t.Fatalf("payload mismatch: %+v", p)
	}
}

func TestMarshalPayload_HandlesNil(t *testing.T) {
	p, err := MarshalPayload(nil)
	if err != nil {
		t.Fatalf("MarshalPayload(nil): %v", err)
	}
	if len(p) != 0 {
		t.Fatalf("nil payload should be empty, got %+v", p)
	}
}
