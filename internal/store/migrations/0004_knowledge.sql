-- 0004_knowledge.sql (applied at v0.4)
-- Knowledge Engine schema: decisions, notes, onboarding, event log.

-- ── Decisions ──────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS decisions (
  id            TEXT PRIMARY KEY,           -- ULID
  created_at    INTEGER NOT NULL,           -- unix ms
  updated_at    INTEGER NOT NULL,           -- unix ms
  status        TEXT NOT NULL DEFAULT 'proposed'
                          CHECK (status IN ('proposed','accepted','rejected','superseded')),
  title         TEXT NOT NULL,
  context       TEXT NOT NULL DEFAULT '',
  decision      TEXT NOT NULL DEFAULT '',
  alternatives  TEXT NOT NULL DEFAULT '',
  consequences  TEXT NOT NULL DEFAULT '',
  body_path     TEXT NOT NULL,              -- relative to .got/decisions/
  workspace_id  TEXT,                       -- REFERENCES workspaces(id) ON DELETE SET NULL
  supersedes_id TEXT                        -- REFERENCES decisions(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS decisions_created_at_idx ON decisions(created_at DESC);
CREATE INDEX IF NOT EXISTS decisions_workspace_idx ON decisions(workspace_id);
CREATE INDEX IF NOT EXISTS decisions_status_idx ON decisions(status);

-- ── Decision links (commits, files) ────────────────────────────────
CREATE TABLE IF NOT EXISTS decision_links (
  id            TEXT PRIMARY KEY,           -- ULID
  decision_id   TEXT NOT NULL REFERENCES decisions(id) ON DELETE CASCADE,
  link_type     TEXT NOT NULL CHECK (link_type IN ('commit','file','workspace')),
  target        TEXT NOT NULL,              -- SHA (commit), path (file), workspace_id
  line_start    INTEGER,                   -- optional line range start
  line_end      INTEGER,                   -- optional line range end
  branch        TEXT,                      -- optional branch ref
  created_at    INTEGER NOT NULL            -- unix ms
);

CREATE INDEX IF NOT EXISTS decision_links_decision_idx ON decision_links(decision_id);
CREATE INDEX IF NOT EXISTS decision_links_target_idx ON decision_links(target);

-- ── Notes ──────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS notes (
  id            TEXT PRIMARY KEY,           -- ULID
  created_at    INTEGER NOT NULL,           -- unix ms
  updated_at    INTEGER NOT NULL,           -- unix ms
  message       TEXT NOT NULL,
  workspace_id  TEXT,                       -- REFERENCES workspaces(id) ON DELETE SET NULL
  branch        TEXT,
  commit_hash   TEXT
);

CREATE INDEX IF NOT EXISTS notes_created_at_idx ON notes(created_at DESC);
CREATE INDEX IF NOT EXISTS notes_workspace_idx ON notes(workspace_id);

-- ── Onboarding ─────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS onboarding_sessions (
  id            TEXT PRIMARY KEY,           -- ULID
  created_at    INTEGER NOT NULL,           -- unix ms
  updated_at    INTEGER NOT NULL,           -- unix ms
  participant   TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'active'
                          CHECK (status IN ('active','completed','paused'))
);

CREATE INDEX IF NOT EXISTS onboarding_sessions_participant_idx ON onboarding_sessions(participant);

-- Progress: which items have been viewed/marked as covered
CREATE TABLE IF NOT EXISTS onboarding_items (
  id              TEXT PRIMARY KEY,         -- ULID
  session_id      TEXT NOT NULL REFERENCES onboarding_sessions(id) ON DELETE CASCADE,
  item_type       TEXT NOT NULL CHECK (item_type IN ('decision','note','file')),
  item_target     TEXT NOT NULL,            -- decision_id / note_id / file path
  covered_at      INTEGER,                 -- unix ms when marked covered, NULL if not yet
  skipped         INTEGER NOT NULL DEFAULT 0  -- boolean: explicitly skipped
);

CREATE INDEX IF NOT EXISTS onboarding_items_session_idx ON onboarding_items(session_id);
CREATE UNIQUE INDEX IF NOT EXISTS onboarding_items_unique ON onboarding_items(session_id, item_type, item_target);

-- ── Event log (append-only, for plugin consumption) ────────────────
CREATE TABLE IF NOT EXISTS event_log (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  event_type    TEXT NOT NULL,
  payload       TEXT NOT NULL,              -- JSON
  created_at    INTEGER NOT NULL           -- unix ms
);
