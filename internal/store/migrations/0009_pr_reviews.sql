-- 0009_pr_reviews.sql (applied at v0.7+)
-- PR review tracking: reviews, merge metadata on pull_requests.

CREATE TABLE IF NOT EXISTS pr_reviews (
  id            TEXT PRIMARY KEY,              -- ULID
  pr_number     INTEGER NOT NULL,              -- FK to pull_requests.number
  reviewer      TEXT NOT NULL,                 -- GitHub login of reviewer
  state         TEXT NOT NULL,                 -- APPROVED, CHANGES_REQUESTED, COMMENTED
  body          TEXT NOT NULL DEFAULT '',
  workspace_id  TEXT,                          -- optional link to workspaces.name
  submitted_at  INTEGER NOT NULL               -- unix ms
);

CREATE INDEX IF NOT EXISTS pr_reviews_pr_number_idx ON pr_reviews(pr_number);
CREATE INDEX IF NOT EXISTS pr_reviews_workspace_idx ON pr_reviews(workspace_id);

-- Add merge tracking columns to pull_requests.
-- These are added with ALTER TABLE IF NOT EXISTS style — SQLite doesn't
-- support IF NOT EXISTS for ALTER TABLE, so we use a safe workaround:
-- we try the ALTER and ignore the error if the column already exists.
-- Since this is run inside a migration (which is idempotent), duplicates
-- are harmless.
ALTER TABLE pull_requests ADD COLUMN merge_commit_sha TEXT NOT NULL DEFAULT '';
ALTER TABLE pull_requests ADD COLUMN merged_at INTEGER;
