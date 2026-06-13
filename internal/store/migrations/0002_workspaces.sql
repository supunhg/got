-- 0002_workspaces.sql
--
-- v0.4 Workspace Engine (got-spec.md §12, ARCHITECTURE.md).
--
-- The v0.1 schema in 0001_init.sql created forward-compat stubs
-- (`workspaces` with just id/name/created_at/state, and
-- `workspace_files` with just workspace_id/path) so the tables
-- existed in the DB but were never written to. This migration drops
-- those stubs and recreates them with the v0.4 shape: more columns
-- on the workspace, additional tables for branches/decisions/notes,
-- and the indexes the workspace CLI needs.
--
-- Both the stub and the final shape use TEXT primary keys for
-- workspace IDs, so the on-disk row format is identical for any
-- future code that wants to read the table. The drop+recreate is
-- safe because no v0.1 command ever wrote to either table; v0.1
-- ships no workspace commands.
--
-- ON DELETE CASCADE on every child table means `got workspace delete
-- <name>` removes the workspace's files/branches/decisions/notes in
-- one transaction; the CLI never has to do that itself.

DROP TABLE IF EXISTS workspace_files;
DROP TABLE IF EXISTS workspaces;

CREATE TABLE workspaces (
  id          TEXT PRIMARY KEY,                -- ULID-like (time + random)
  name        TEXT NOT NULL UNIQUE,            -- unique slug (e.g. "oauth-refactor")
  title       TEXT NOT NULL,                   -- human-readable display name
  description TEXT NOT NULL DEFAULT '',        -- one-line summary
  color       TEXT NOT NULL DEFAULT '',        -- optional label color (hex)
  state       TEXT NOT NULL DEFAULT 'open',    -- open|archived
  created_at  INTEGER NOT NULL,                -- unix ms
  updated_at  INTEGER NOT NULL,                -- unix ms
  metadata    TEXT NOT NULL DEFAULT '{}'       -- JSON for plugin extensions
);
CREATE INDEX workspaces_state_idx ON workspaces(state);
CREATE INDEX workspaces_name_idx ON workspaces(name);
CREATE INDEX workspaces_created_at_idx ON workspaces(created_at DESC);

CREATE TABLE workspace_files (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  path         TEXT NOT NULL,                  -- repo-relative path
  added_at     INTEGER NOT NULL,
  note         TEXT NOT NULL DEFAULT '',       -- optional note
  PRIMARY KEY (workspace_id, path)
);
CREATE INDEX workspace_files_workspace_idx ON workspace_files(workspace_id);

CREATE TABLE workspace_branches (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  branch       TEXT NOT NULL,                  -- branch name
  added_at     INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (workspace_id, branch)
);
CREATE INDEX workspace_branches_workspace_idx ON workspace_branches(workspace_id);
CREATE INDEX workspace_branches_branch_idx ON workspace_branches(branch);

CREATE TABLE workspace_decisions (
  id           TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  title        TEXT NOT NULL,
  body         TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'proposed', -- proposed|accepted|rejected|superseded
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL,
  metadata     TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX workspace_decisions_workspace_idx ON workspace_decisions(workspace_id);
CREATE INDEX workspace_decisions_status_idx ON workspace_decisions(status);
CREATE INDEX workspace_decisions_created_at_idx ON workspace_decisions(created_at DESC);

CREATE TABLE workspace_notes (
  id           TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  body         TEXT NOT NULL,
  pinned       INTEGER NOT NULL DEFAULT 0,
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL
);
CREATE INDEX workspace_notes_workspace_idx ON workspace_notes(workspace_id);
CREATE INDEX workspace_notes_pinned_idx ON workspace_notes(workspace_id, pinned);
CREATE INDEX workspace_notes_created_at_idx ON workspace_notes(workspace_id, created_at DESC);
