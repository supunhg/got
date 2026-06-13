package eventbus

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	storepkg "github.com/got-sh/got/internal/store"
)

// Subscriber is the callback invoked when an event is published.
// Subscribers must be safe to call from multiple goroutines — the
// bus dispatches each event to all subscribers sequentially on the
// publisher's goroutine. A panicking subscriber is recovered so
// other subscribers still see the event.
//
// The returned boolean is reserved for future use (e.g. a
// subscriber that wants to signal "re-deliver this later"); v0.4
// ignores it.
type Subscriber func(ctx context.Context, e *Event) error

// Bus is the in-process event bus. Construct with New. The bus
// owns a persistence log backed by a *storepkg.Store; once an
// event is published, it is durable in the events table before
// any subscriber is called.
//
// The zero value is not usable.
type Bus struct {
	store *storepkg.Store
	mu    sync.RWMutex
	subs  map[Topic][]*subEntry
	// actor is the OS user captured at New time. Empty if unknown.
	actor string
	// source is the .got/got.db path captured at New time. Empty
	// if the bus was built without one.
	source string
}

// subEntry wraps a Subscriber with a unique registration ID so
// Unsubscribe can find it even if the same callback is registered
// for multiple topics.
type subEntry struct {
	id  uint64
	fn  Subscriber
	ctx context.Context
}

// New constructs a bus backed by the given *storepkg.Store. The
// store must be non-nil (ErrStoreRequired otherwise). The actor and
// source arguments are stamped on every event the bus publishes on
// behalf of callers that leave them blank; pass the empty string
// to skip.
//
// The bus does not take ownership of the store. The caller is
// responsible for closing it. This keeps New side-effect-free
// for tests that share a *storepkg.Store across multiple buses.
func New(s *storepkg.Store, actor, source string) (*Bus, error) {
	if s == nil {
		return nil, ErrStoreRequired
	}
	if actor == "" {
		actor = currentActor()
	}
	return &Bus{
		store:  s,
		subs:   make(map[Topic][]*subEntry),
		actor:  actor,
		source: source,
	}, nil
}

// Subscribe registers fn to receive every event published on
// topic. The returned ID can be passed to Unsubscribe to remove
// the registration. The supplied context bounds the lifetime of
// the subscription: when ctx is cancelled, the bus removes the
// subscription automatically. This makes it easy to write a
// per-command subscriber without leaking goroutines.
//
// Subscribe is safe to call from multiple goroutines.
func (b *Bus) Subscribe(topic Topic, ctx context.Context, fn Subscriber) (uint64, error) {
	if _, err := ParseTopic(string(topic)); err != nil {
		return 0, err
	}
	if fn == nil {
		return 0, ErrNilSubscriber
	}
	if ctx == nil {
		ctx = context.Background()
	}
	id := nextSubID()
	entry := &subEntry{id: id, fn: fn, ctx: ctx}

	b.mu.Lock()
	b.subs[topic] = append(b.subs[topic], entry)
	b.mu.Unlock()

	// Auto-unsubscribe when ctx is cancelled.
	go func() {
		<-ctx.Done()
		_ = b.Unsubscribe(topic, id)
	}()

	return id, nil
}

// Unsubscribe removes the registration identified by id. Returns
// nil even if no such registration exists (idempotent). The
// supplied topic scopes the search so the same id can be reused
// across topics.
func (b *Bus) Unsubscribe(topic Topic, id uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	entries := b.subs[topic]
	for i, e := range entries {
		if e.id == id {
			// Remove in-place: O(n) but n is tiny.
			b.subs[topic] = append(entries[:i], entries[i+1:]...)
			return nil
		}
	}
	return nil
}

// PublishOption configures a single Publish call.
type PublishOption func(*publishOpts)

type publishOpts struct {
	actor   string
	source  string
	payload Payload
	// at overrides the event timestamp. Tests use this to inject
	// deterministic CreatedAt values.
	at time.Time
}

// WithActor overrides the actor stamped on the event.
func WithActor(actor string) PublishOption {
	return func(o *publishOpts) { o.actor = actor }
}

// WithSource overrides the source stamped on the event.
func WithSource(src string) PublishOption {
	return func(o *publishOpts) { o.source = src }
}

// WithPayload sets the payload. If not provided, Publish emits
// an empty payload object.
func WithPayload(p Payload) PublishOption {
	return func(o *publishOpts) { o.payload = p }
}

// WithTimestamp overrides the event timestamp. Production callers
// should leave this at zero so Publish uses time.Now.
func WithTimestamp(t time.Time) PublishOption {
	return func(o *publishOpts) { o.at = t }
}

// Publish synchronously persists e to the events table and then
// dispatches it to every subscriber of e.Topic. The returned
// event is the envelope that was persisted; its ID, CreatedAt,
// Actor, and Source are populated by the bus if the caller left
// them blank.
//
// Publish is safe to call from multiple goroutines.
func (b *Bus) Publish(ctx context.Context, topic Topic, opts ...PublishOption) (*Event, error) {
	t, err := ParseTopic(string(topic))
	if err != nil {
		return nil, err
	}
	o := publishOpts{}
	for _, opt := range opts {
		opt(&o)
	}
	if o.payload == nil {
		o.payload = Payload{}
	}
	if o.actor == "" {
		o.actor = b.actor
	}
	if o.source == "" {
		o.source = b.source
	}
	if o.at.IsZero() {
		o.at = time.Now().UTC()
	}
	e := &Event{
		ID:        newEventID(o.at),
		Topic:     t,
		CreatedAt: o.at,
		Actor:     o.actor,
		Source:    o.source,
		Payload:   o.payload,
	}
	if err := b.persist(ctx, e); err != nil {
		return nil, err
	}
	b.dispatch(ctx, e)
	return e, nil
}

// PublishRaw is like Publish but takes a fully-formed event. It
// is used by replay callers that are re-emitting events from the
// log. The event's ID, CreatedAt, Actor, and Source are taken
// as-is.
func (b *Bus) PublishRaw(ctx context.Context, e *Event) error {
	if e == nil {
		return fmt.Errorf("eventbus: nil event")
	}
	if e.ID == "" {
		return ErrEmptyID
	}
	if _, err := ParseTopic(string(e.Topic)); err != nil {
		return err
	}
	if err := b.persist(ctx, e); err != nil {
		return err
	}
	b.dispatch(ctx, e)
	return nil
}

// Replay returns the most recent events from the log, optionally
// filtered by topic and bounded by limit. The order is
// newest-first (most recent CreatedAt first). If limit is <= 0,
// the cap is 100; if topic is empty, events from all topics are
// returned.
//
// This is the public surface used by `got event list` and by any
// plugin bootstrap that wants to catch up on missed events.
func (b *Bus) Replay(ctx context.Context, topic Topic, limit int) ([]*Event, error) {
	if topic != "" {
		if _, err := ParseTopic(string(topic)); err != nil {
			return nil, err
		}
	}
	if limit <= 0 {
		limit = 100
	}
	return readRecentEvents(ctx, b.store, string(topic), limit)
}

// ReplaySince returns events whose CreatedAt is at or after since,
// optionally filtered by topic, in oldest-first order. The cap is
// max(limit, 1000) to keep the response bounded. Subscribers can
// use this to catch up after a process restart.
func (b *Bus) ReplaySince(ctx context.Context, topic Topic, since time.Time, limit int) ([]*Event, error) {
	if topic != "" {
		if _, err := ParseTopic(string(topic)); err != nil {
			return nil, err
		}
	}
	if limit <= 0 {
		limit = 1000
	}
	return readEventsSince(ctx, b.store, string(topic), since, limit)
}

// Counts returns a map of topic -> event count from the durable
// log. Used by `got status` and the `got event list` summary.
func (b *Bus) Counts(ctx context.Context) (map[Topic]int, error) {
	rows, err := b.store.DB().QueryContext(ctx,
		`SELECT topic, COUNT(*) FROM events GROUP BY topic`)
	if err != nil {
		return nil, fmt.Errorf("eventbus: count by topic: %w", err)
	}
	defer rows.Close()
	out := map[Topic]int{}
	for rows.Next() {
		var t string
		var n int
		if err := rows.Scan(&t, &n); err != nil {
			return nil, err
		}
		out[Topic(t)] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// dispatch synchronously invokes every registered subscriber for
// e.Topic. A panicking subscriber is recovered and logged (we
// have no logger in the bus layer, so we just swallow it after
// writing a line to stderr — this is best-effort, not a primary
// observability path). The dispatch order is registration
// order, which makes tests deterministic.
func (b *Bus) dispatch(ctx context.Context, e *Event) {
	b.mu.RLock()
	entries := make([]*subEntry, len(b.subs[e.Topic]))
	copy(entries, b.subs[e.Topic])
	b.mu.RUnlock()

	for _, entry := range entries {
		// Skip entries whose context is already cancelled.
		if entry.ctx.Err() != nil {
			continue
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(os.Stderr, "eventbus: subscriber panic on %s: %v\n", e.Topic, r)
				}
			}()
			_ = entry.fn(ctx, e)
		}()
	}
}

// subIDSeq is a process-local counter for subscription IDs.
// uint64 is overkill for the number of subscribers a single
// process is likely to register, but it sidesteps any wraparound
// risk.
var subIDSeq uint64

func nextSubID() uint64 {
	subIDSeq++
	return subIDSeq
}

// newEventID builds a sortable, URL-safe event ID. The format is
// the same 8-byte time prefix + 8 random bytes as the workspace
// engine uses, hex-encoded, separated by a dash. The time prefix
// is the unix millisecond at the time of publish; the random
// suffix avoids collisions for events that share a millisecond
// (Publish is called from many places).
func newEventID(at time.Time) string {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	ts := uint64(at.UTC().UnixMilli())
	tsHex := fmt.Sprintf("%013x", ts)
	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		// crypto/rand is essentially never unavailable, but if
		// it is, fall back to time-based entropy. The ID will
		// still be unique within a single process.
		for i := range rnd {
			rnd[i] = byte(ts >> (uint(i) * 8))
		}
	}
	return tsHex + "-" + hex.EncodeToString(rnd[:])
}

// currentActor returns the OS user, best-effort. On any error
// (e.g. $USER unset on Linux) it returns the empty string.
func currentActor() string {
	for _, env := range []string{"GOT_ACTOR", "USER", "USERNAME", "LOGNAME"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return ""
}

// sortByTimeDesc sorts a slice of *Event in newest-first order
// (most recent CreatedAt first). It is used by Replay to ensure
// a stable ordering even when two events share a timestamp.
func sortByTimeDesc(es []*Event) {
	sort.SliceStable(es, func(i, j int) bool {
		if es[i].CreatedAt.Equal(es[j].CreatedAt) {
			return es[i].ID > es[j].ID
		}
		return es[i].CreatedAt.After(es[j].CreatedAt)
	})
}
