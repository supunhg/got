package eventbus

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	storepkg "github.com/got-sh/got/internal/store"
)

// persist writes e to the events table inside a transaction. It
// is the only place in the bus that touches the events schema
// for writes, which keeps the schema-to-Go mapping in one spot.
//
// The transaction is short and uses INSERT OR REPLACE so that
// re-delivery of an event with the same ID (e.g. a replay
// subscriber re-emitting) is idempotent. This is not the primary
// dedup mechanism — replay subscribers should re-emit using
// PublishRaw with the original ID — but it means a buggy
// subscriber can't double the count.
func (b *Bus) persist(ctx context.Context, e *Event) error {
	if e == nil || e.ID == "" {
		return ErrEmptyID
	}
	body, err := json.Marshal(e.Payload)
	if err != nil {
		return fmt.Errorf("eventbus: marshal payload for %s: %w", e.Topic, err)
	}
	tx, err := b.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("eventbus: begin tx: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT OR REPLACE INTO events(id, topic, created_at, actor, source, payload)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		e.ID, string(e.Topic), e.CreatedAt.UTC().UnixMilli(), e.Actor, e.Source, string(body),
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("eventbus: insert event %s: %w", e.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("eventbus: commit event %s: %w", e.ID, err)
	}
	return nil
}

// readRecentEvents returns the most recent events from the
// durable log, optionally filtered by topic. The order is
// newest-first. The limit caps the number of rows scanned.
func readRecentEvents(ctx context.Context, s *storepkg.Store, topic string, limit int) ([]*Event, error) {
	q := `SELECT id, topic, created_at, COALESCE(actor, ''), COALESCE(source, ''), COALESCE(payload, '{}')
	      FROM events`
	args := []any{}
	if topic != "" {
		q += ` WHERE topic = ?`
		args = append(args, topic)
	}
	q += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("eventbus: query recent: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

// readEventsSince returns events with created_at >= since,
// optionally filtered by topic, in oldest-first order. The
// caller-supplied limit caps the result set; a default of 1000
// is applied by ReplaySince if limit <= 0.
func readEventsSince(ctx context.Context, s *storepkg.Store, topic string, since time.Time, limit int) ([]*Event, error) {
	q := `SELECT id, topic, created_at, COALESCE(actor, ''), COALESCE(source, ''), COALESCE(payload, '{}')
	      FROM events WHERE created_at >= ?`
	args := []any{since.UTC().UnixMilli()}
	if topic != "" {
		q += ` AND topic = ?`
		args = append(args, topic)
	}
	q += ` ORDER BY created_at ASC, id ASC LIMIT ?`
	args = append(args, limit)

	rows, err := s.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("eventbus: query since: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

// scanEvents walks a *sql.Rows produced by one of the read*
// helpers and decodes each row into a *Event. The row layout
// must match the SELECT in readRecentEvents / readEventsSince.
func scanEvents(rows *sql.Rows) ([]*Event, error) {
	out := []*Event{}
	for rows.Next() {
		var (
			e         Event
			createdMS int64
			payload   string
		)
		if err := rows.Scan(&e.ID, &e.Topic, &createdMS, &e.Actor, &e.Source, &payload); err != nil {
			return nil, fmt.Errorf("eventbus: scan event row: %w", err)
		}
		e.CreatedAt = time.UnixMilli(createdMS).UTC()
		if payload != "" {
			_ = json.Unmarshal([]byte(payload), &e.Payload)
		}
		if e.Payload == nil {
			e.Payload = Payload{}
		}
		out = append(out, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
