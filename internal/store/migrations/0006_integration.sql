-- 0006_integration.sql (applied at v0.5)
-- Workspace ↔ Git integration schema.
--
-- This migration wires workspaces to real Git state by tracking commit
-- associations and storing the last known commit SHA for fast access.

-- Add last_commit_sha to workspaces for fast access to the most recent commit.
ALTER TABLE workspaces ADD COLUMN last_commit_sha TEXT NOT NULL DEFAULT '';

-- Workspace commits: records commits associated with a workspace.
-- Each entry links a workspace to a commit SHA with a timestamp and
-- optional branch context. This enables workspace commit history viewing
-- even after branches are deleted.
CREATE TABLE IF NOT EXISTS workspace_commits (
  id            TEXT PRIMARY KEY,                  -- ULID
  workspace_id  TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  commit_sha    TEXT NOT NULL,
  branch_name   TEXT NOT NULL DEFAULT '',          -- branch at time of linking
  message       TEXT NOT NULL DEFAULT '',          -- commit message (cached for display)
  created_at    INTEGER NOT NULL,                  -- unix ms (when the link was created, not the commit)
  UNIQUE(workspace_id, commit_sha)
);

CREATE INDEX IF NOT EXISTS workspace_commits_ws_idx ON workspace_commits(workspace_id);
CREATE INDEX IF NOT EXISTS workspace_commits_sha_idx ON workspace_commits(commit_sha);
