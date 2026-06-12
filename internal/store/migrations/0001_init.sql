-- GOT initial schema (got-spec.md §12).
--
-- Every table that v0.1 doesn't fully use is created here with its final
-- shape so that v0.2/v0.3/v0.4 can drop code (not migration) work to
-- start writing to these tables. Code that has no interface in v0.1
-- simply has no callers.
--
-- All CREATE statements use IF NOT EXISTS so the migration is
-- idempotent. The migration runner in store.go records schema_version
-- in the meta table and skips already-applied migrations on re-open.

CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS snapshots (  -- USED IN v0.2
  id          TEXT PRIMARY KEY,        -- ULID
  created_at  INTEGER NOT NULL,        -- unix ms
  reason      TEXT NOT NULL,           -- e.g. "before-reset", "before-rebase"
  ref         TEXT NOT NULL,           -- "refs/heads/feature/x" or "detached@abc123"
  reflog_sel  TEXT,                    -- optional reflog selector
  stash_ref   TEXT,                    -- optional git stash ref
  metadata    TEXT NOT NULL DEFAULT '{}'  -- JSON
);
CREATE INDEX IF NOT EXISTS snapshots_created_at_idx ON snapshots(created_at DESC);

CREATE TABLE IF NOT EXISTS decisions (  -- USED IN v0.4
  id         TEXT PRIMARY KEY,
  created_at INTEGER NOT NULL,
  status     TEXT NOT NULL,           -- proposed|accepted|rejected|superseded
  title      TEXT NOT NULL,
  body_path  TEXT NOT NULL            -- relative to .got/decisions/
);

CREATE TABLE IF NOT EXISTS workspaces (  -- USED IN v0.4
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  created_at  INTEGER NOT NULL,
  state       TEXT NOT NULL DEFAULT 'open'  -- open|closed
);
CREATE TABLE IF NOT EXISTS workspace_files (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  path         TEXT NOT NULL,
  PRIMARY KEY (workspace_id, path)
);

CREATE TABLE IF NOT EXISTS health_runs (  -- USED IN v0.3
  id         TEXT PRIMARY KEY,
  run_at     INTEGER NOT NULL,
  report     TEXT NOT NULL           -- JSON
);

CREATE TABLE IF NOT EXISTS cache_kv (  -- generic key/value cache
  key        TEXT PRIMARY KEY,
  value      BLOB NOT NULL,
  updated_at INTEGER NOT NULL
);
