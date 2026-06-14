-- 0008_github.sql (applied at v0.7)
-- GitHub integration schema: tracks pull requests, issues, and auth configuration.

CREATE TABLE IF NOT EXISTS github_config (
  id          TEXT PRIMARY KEY,                -- 'default' singleton row
  token       TEXT NOT NULL DEFAULT '',
  owner       TEXT NOT NULL DEFAULT '',
  repo        TEXT NOT NULL DEFAULT '',
  base_branch TEXT NOT NULL DEFAULT 'main',
  updated_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS pull_requests (
  id            TEXT PRIMARY KEY,              -- ULID
  number        INTEGER NOT NULL,
  title         TEXT NOT NULL,
  state         TEXT NOT NULL DEFAULT 'open',  -- open, closed, merged
  branch        TEXT NOT NULL,                 -- head branch
  base          TEXT NOT NULL DEFAULT 'main',  -- target branch
  url           TEXT NOT NULL DEFAULT '',
  workspace_id  TEXT,                          -- optional FK to workspaces.name
  created_at    INTEGER NOT NULL,              -- unix ms
  updated_at    INTEGER NOT NULL,              -- unix ms
  UNIQUE(number)
);

CREATE INDEX IF NOT EXISTS pull_requests_branch_idx ON pull_requests(branch);
CREATE INDEX IF NOT EXISTS pull_requests_workspace_idx ON pull_requests(workspace_id);

CREATE TABLE IF NOT EXISTS issues (
  id            TEXT PRIMARY KEY,              -- ULID
  number        INTEGER NOT NULL,
  title         TEXT NOT NULL,
  state         TEXT NOT NULL DEFAULT 'open',
  labels        TEXT NOT NULL DEFAULT '[]',    -- JSON array of label names
  url           TEXT NOT NULL DEFAULT '',
  workspace_id  TEXT,                          -- optional FK to workspaces.name
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL,
  UNIQUE(number)
);

CREATE INDEX IF NOT EXISTS issues_workspace_idx ON issues(workspace_id);
