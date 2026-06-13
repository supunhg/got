-- 0003_events.sql
--
-- v0.5+ Event Bus (docs/EVENT_BUS.md).
--
-- The events table is the durable replay log for the
-- internal/eventbus package. Every Publish call that has a
-- Persister attached writes one row here. The table is small
-- (one row per event, payload as JSON) and indexed for the
-- two read patterns the CLI cares about: list by topic, and
-- list by recency.
--
-- Topic is a dotted string like "commit.created" (the
-- internal/eventbus.Topic type). It is stored as TEXT, not as
-- an integer ID, so plugin authors can grep the table or
-- filter on it without a join. The on-disk JSON payloads are
-- stable and versioned: any breaking change to the wire shape
-- (e.g. renaming a Topic constant) requires a follow-up
-- migration plus a note in docs/EVENT_BUS.md.

CREATE TABLE events (
  id          TEXT PRIMARY KEY,                -- ULID-like (eventbus.newEventID)
  topic       TEXT NOT NULL,                   -- e.g. "CommitCreated" (PascalCase)
  created_at  INTEGER NOT NULL,                -- unix ms
  actor       TEXT NOT NULL DEFAULT '',        -- user who triggered the event
  source      TEXT NOT NULL DEFAULT '',        -- .got/got.db path (multi-worktree disambiguation)
  payload     TEXT NOT NULL DEFAULT '{}',      -- JSON blob (eventbus.Payload)
  metadata    TEXT NOT NULL DEFAULT '{}'       -- JSON blob for plugin extensions
);
CREATE INDEX events_topic_idx ON events(topic);
CREATE INDEX events_created_at_idx ON events(created_at DESC);
