package eventbus

import "errors"

// Sentinel errors. Subscribers and the CLI use errors.Is to
// distinguish bus-internal failures from publisher errors.
var (
	// ErrUnknownTopic is returned by Publish / Subscribe when the
	// topic is not in the closed allow-list. The CLI maps this to
	// exit code 64 (usage error).
	ErrUnknownTopic = errors.New("eventbus: unknown topic")

	// ErrNilSubscriber is returned by Subscribe when the supplied
	// callback is nil. We reject nil explicitly so a typo doesn't
	// silently no-op.
	ErrNilSubscriber = errors.New("eventbus: nil subscriber")

	// ErrEmptyID is returned by Persist / Envelope when the event
	// has an empty ID. This is a programmer error — newEventID
	// always sets a non-empty ID.
	ErrEmptyID = errors.New("eventbus: empty event id")

	// ErrStoreRequired is returned by the constructors when a nil
	// *store.Store is passed. The bus needs the store for
	// persistence and replay; it cannot operate without one.
	ErrStoreRequired = errors.New("eventbus: store is required")
)
