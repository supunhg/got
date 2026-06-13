-- 0004_workspace_commits.sql
--
-- v0.5 Workspace Engine (ARCHITECTURE_WORKSPACES.md).
--
-- The v0.4 migration (0002_workspaces.sql) created workspaces,
-- workspace_files, workspace_branches, workspace_decisions, and
-- workspace_notes, but had no way to pin a Git commit to a
-- workspace. This migration adds workspace_commits.
--
-- A workspace can reference a commit in two ways:
--
--   1. Explicit pinning. The user runs `got workspace add-commit
--      <ws> <sha>` to record a commit as relevant to a
--      workspace. The row's `note` is a free-form annotation
--      (e.g. "this is the commit that landed the PKCE flow").
--
--   2. Implicit discovery. `got workspace status <ws>` runs
--      `git log -- <tracked-files>` to surface recent commits
--      that affect the workspace's tracked files. Those commits
--      are NOT inserted into workspace_commits; they are read
--      straight from git on demand. The table only stores
--      user-pinned commits.
--
-- The primary key is (workspace_id, sha) so re-adding the same
-- commit (e.g. from a CI hook) is idempotent: the existing row's
-- added_at and note are updated.
--
-- The schema deliberately does NOT enforce a foreign key on sha
-- to refs/heads/<branch>. The commit graph moves under the
-- workspace's feet: rebases, force-pushes, and branch deletions
-- all invalidate individual SHAs. The workspace engine stores
-- SHAs as opaque identifiers and tolerates dangling ones
-- gracefully (status shows "commit xyz not in repo" rather
-- than erroring).

CREATE TABLE workspace_commits (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  sha          TEXT NOT NULL,                  -- 40-char hex (or 7+ for short)
  added_at     INTEGER NOT NULL,               -- unix ms
  note         TEXT NOT NULL DEFAULT '',       -- optional annotation
  PRIMARY KEY (workspace_id, sha)
);
CREATE INDEX workspace_commits_workspace_idx ON workspace_commits(workspace_id);
CREATE INDEX workspace_commits_added_at_idx ON workspace_commits(workspace_id, added_at DESC);
