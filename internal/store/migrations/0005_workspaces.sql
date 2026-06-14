-- 0005_workspaces.sql (applied at v0.4)
-- Workspace Engine schema: workspaces, files, branches.

-- Workspaces are logical groupings of related artifacts (files, branches,
-- decisions, notes) into named contexts like "oauth" or "payment-refactor".
-- They are NOT tied to Git worktrees or branches.

CREATE TABLE IF NOT EXISTS workspaces (
  id            TEXT PRIMARY KEY,                  -- ULID
  name          TEXT NOT NULL UNIQUE,              -- human-readable CLI identifier
  description   TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL DEFAULT 'active'
                          CHECK (status IN ('active', 'archived')),
  tags          TEXT NOT NULL DEFAULT '[]',        -- JSON array of strings
  created_at    INTEGER NOT NULL,                  -- unix ms
  updated_at    INTEGER NOT NULL                   -- unix ms
);

CREATE INDEX IF NOT EXISTS workspaces_name_idx ON workspaces(name);
CREATE INDEX IF NOT EXISTS workspaces_status_idx ON workspaces(status);

-- Files tracked within a workspace (just string paths; no Git integration yet).
CREATE TABLE IF NOT EXISTS workspace_files (
  id            TEXT PRIMARY KEY,                  -- ULID
  workspace_id  TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  path          TEXT NOT NULL,
  created_at    INTEGER NOT NULL,                  -- unix ms
  UNIQUE(workspace_id, path)
);

CREATE INDEX IF NOT EXISTS workspace_files_ws_idx ON workspace_files(workspace_id);

-- Branches tracked within a workspace (just string names; no Git integration yet).
CREATE TABLE IF NOT EXISTS workspace_branches (
  id            TEXT PRIMARY KEY,                  -- ULID
  workspace_id  TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  branch_name   TEXT NOT NULL,
  created_at    INTEGER NOT NULL,                  -- unix ms
  UNIQUE(workspace_id, branch_name)
);

CREATE INDEX IF NOT EXISTS workspace_branches_ws_idx ON workspace_branches(workspace_id);
