package eventbus

import (
	"database/sql"

	storepkg "github.com/got-sh/got/internal/store"
)

// Store returns the *storepkg.Store that backs the bus. The
// bridge is used by the CLI's `got event show <id>` command
// to look up a single event by its primary key without
// re-opening the DB. Tests and other callers that build a
// bus with a *sql.DB directly (no *storepkg.Store wrapper)
// should not call Store; the result is a typed nil and the
// caller should use the bus's Replay / ReplaySince methods
// instead.
//
// The method is intentionally read-only: callers must not
// use the returned *storepkg.Store to publish events. The
// bus is the only writer to the events table, and a second
// writer would race with the bus's transactions.
func (b *Bus) Store() *storepkg.Store {
	return b.store
}

// bridgeStore is the small subset of *sql.DB that the CLI
// needs to run a single SELECT for `got event show`. The
// actual *storepkg.Store satisfies this interface; tests
// can pass a real *sql.DB.
type bridgeStore interface {
	QueryRowContext(ctx any, query string, args ...any) *sql.Row
}
