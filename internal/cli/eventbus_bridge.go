package cli

import (
	"context"
	"database/sql"

	"github.com/got-sh/got/internal/eventbus"
)

// sqlRow is the alias used by eventbus.go; importing
// database/sql here keeps the bridge file small and the
// import list of eventbus.go short.
type sqlRow = sql.Row

// busStoreForShowBridge adapts a *eventbus.Bus to the
// (QueryRowContext -> *sql.Row) shape that the CLI uses
// to look up a single event by ID. It is the only place
// outside internal/eventbus that touches the bus's
// underlying store, and it returns ok=false if the bus
// somehow doesn't expose one (defense in depth; the bus
// constructor enforces this).
func busStoreForShowBridge(bus *eventbus.Bus) (interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}, bool) {
	if bus == nil {
		return nil, false
	}
	s := bus.Store()
	if s == nil {
		return nil, false
	}
	return s.DB(), true
}
