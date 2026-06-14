-- Snapshots table for safe operations recovery points.
-- Stores the state before destructive Git operations (reset, rebase, force push)
-- so users can recover if something goes wrong.

CREATE TABLE IF NOT EXISTS snapshots (
  id          TEXT PRIMARY KEY,
  created_at  INTEGER NOT NULL,
  reason      TEXT NOT NULL,
  ref         TEXT NOT NULL,
  reflog_sel  TEXT,
  stash_ref   TEXT,
  metadata    TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS snapshots_created_at_idx ON snapshots(created_at DESC);
