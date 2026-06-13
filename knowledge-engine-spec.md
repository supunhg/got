# GOT Knowledge Engine — Specification (v0.4)

> **Status:** Design specification — implementation deferred to v0.4.
> This document is forward-looking. Core infrastructure (store, event bus, workspace engine) will be in place by v0.4.
> See `got-spec.md` for the current v0.1 scope and roadmap.

---

## 1. Purpose

Give repositories persistent, queryable memory about decisions, notes, and onboarding
context. Commits store *what* and *when*. The Knowledge Engine stores *why* and helps
new developers understand a codebase faster.

### Guiding principles

1. **Local-first.** Everything works offline. No network calls, no external services.
2. **Thread-safe.** All store operations are safe for concurrent access from multiple
   goroutines (e.g., plugin hooks and CLI commands running alongside each other).
3. **Plugin-compatible.** Core operations publish typed events via the Event Bus so
   plugins can react to knowledge changes.
4. **Git-compatible.** No modifications to `.git/` internals. All knowledge data lives
   in `.got/`.
5. **Every operation is recoverable.** Writes are transactional via SQLite. Malformed
   data does not corrupt the database.

---

## 2. Release timing (resolved)

| Question | Resolution |
|----------|-----------|
| When to implement | **v0.4**, as originally scheduled in `got-spec.md`. This spec documents the design ahead of time so the architecture leaves room. |
| Event Bus | **Design a minimal in-memory bus** as part of this spec. A robust event system is deferred; the v0.4 bus is a simple channel-based pub/sub. |
| ADR storage | **Hybrid: files + SQLite.** Decision bodies are Markdown files in `.got/decisions/` (grep-able, survive DB corruption, follow ADR conventions). Metadata (status, timestamps, links) lives in SQLite for efficient querying. |

---

## 3. Core entities

### 3.1 Decision (ADR-like)

A structured record of an architectural or engineering decision.

| Field | Type | Description |
|-------|------|-------------|
| `id` | ULID | Unique identifier; chronologically sortable |
| `created_at` | INTEGER (unix ms) | Timestamp of creation |
| `updated_at` | INTEGER (unix ms) | Timestamp of last status change |
| `status` | TEXT | One of: `proposed`, `accepted`, `rejected`, `superseded` |
| `title` | TEXT | Short summary (< 80 chars) |
| `context` | TEXT | The problem statement / background (Markdown, < 10 KB) |
| `decision` | TEXT | What was decided (Markdown, < 10 KB) |
| `alternatives` | TEXT | Alternatives considered (Markdown, < 10 KB) |
| `consequences` | TEXT | Positive/negative consequences (Markdown, < 10 KB) |
| `body_path` | TEXT | Path to the full Markdown file, relative to `.got/decisions/` |
| `workspace_id` | TEXT | Optional — scopes the decision to a workspace |
| `supersedes_id` | TEXT | Optional — ULID of the decision this one supersedes |

**Note:** The four body fields (context, decision, alternatives, consequences) are stored
both in SQLite (for fast listing/preview) and in the `.got/decisions/` Markdown file
(authoritative full content). See §8 for the file format.

### 3.2 Note

Freeform Markdown, linked to workspace, branch, or commit.

| Field | Type | Description |
|-------|------|-------------|
| `id` | ULID | Unique identifier |
| `created_at` | INTEGER (unix ms) | Timestamp of creation |
| `updated_at` | INTEGER (unix ms) | Timestamp of last edit |
| `message` | TEXT | Inline Markdown content |
| `workspace_id` | TEXT | Optional — scopes the note to a workspace |
| `branch` | TEXT | Optional — linked branch name |
| `commit_hash` | TEXT | Optional — linked commit SHA |

**Constraint:** Notes are small, inline-only content (< 4 KB). For longer documents,
use a Decision or create a standalone Markdown file in the repo.

### 3.3 OnboardingSession

Tracks what new contributors have been shown / what they still need to explore.

| Field | Type | Description |
|-------|------|-------------|
| `id` | ULID | Unique identifier |
| `created_at` | INTEGER (unix ms) | Session creation time |
| `updated_at` | INTEGER (unix ms) | Last progress update |
| `participant` | TEXT | User identifier (git config user.name or email) |
| `status` | TEXT | One of: `active`, `completed`, `paused` |

Progress is tracked in the `onboarding_items` table (§4).

---

## 4. Data model (SQLite)

### 4.1 Migration file

```sql
-- 0004_knowledge.sql (applied at v0.4)

-- ── Decisions ──────────────────────────────────────────────────────
CREATE TABLE decisions (
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
  workspace_id  TEXT REFERENCES workspaces(id) ON DELETE SET NULL,
  supersedes_id TEXT REFERENCES decisions(id) ON DELETE SET NULL
);
CREATE INDEX decisions_created_at_idx ON decisions(created_at DESC);
CREATE INDEX decisions_workspace_idx ON decisions(workspace_id);
CREATE INDEX decisions_status_idx ON decisions(status);

-- ── Decision links (commits, files) ────────────────────────────────
CREATE TABLE decision_links (
  id            TEXT PRIMARY KEY,           -- ULID
  decision_id   TEXT NOT NULL REFERENCES decisions(id) ON DELETE CASCADE,
  link_type     TEXT NOT NULL CHECK (link_type IN ('commit','file','workspace')),
  target        TEXT NOT NULL,              -- SHA (commit), path (file), workspace_id
  line_start    INTEGER,                   -- optional line range start
  line_end      INTEGER,                   -- optional line range end
  branch        TEXT,                      -- optional branch ref
  created_at    INTEGER NOT NULL           -- unix ms
);
CREATE INDEX decision_links_decision_idx ON decision_links(decision_id);
CREATE INDEX decision_links_target_idx ON decision_links(target);

-- ── Notes ──────────────────────────────────────────────────────────
CREATE TABLE notes (
  id            TEXT PRIMARY KEY,           -- ULID
  created_at    INTEGER NOT NULL,           -- unix ms
  updated_at    INTEGER NOT NULL,           -- unix ms
  message       TEXT NOT NULL,
  workspace_id  TEXT REFERENCES workspaces(id) ON DELETE SET NULL,
  branch        TEXT,
  commit_hash   TEXT
);
CREATE INDEX notes_created_at_idx ON notes(created_at DESC);
CREATE INDEX notes_workspace_idx ON notes(workspace_id);

-- ── Onboarding ─────────────────────────────────────────────────────
CREATE TABLE onboarding_sessions (
  id            TEXT PRIMARY KEY,           -- ULID
  created_at    INTEGER NOT NULL,           -- unix ms
  updated_at    INTEGER NOT NULL,           -- unix ms
  participant   TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'active'
                          CHECK (status IN ('active','completed','paused'))
);
CREATE INDEX onboarding_sessions_participant_idx ON onboarding_sessions(participant);

-- Progress: which items have been viewed/marked as covered
CREATE TABLE onboarding_items (
  id              TEXT PRIMARY KEY,         -- ULID
  session_id      TEXT NOT NULL REFERENCES onboarding_sessions(id) ON DELETE CASCADE,
  item_type       TEXT NOT NULL CHECK (item_type IN ('decision','note','file')),
  item_target     TEXT NOT NULL,            -- decision_id / note_id / file path
  covered_at      INTEGER,                 -- unix ms when marked covered, NULL if not yet
  skipped         INTEGER NOT NULL DEFAULT 0  -- boolean: explicitly skipped
);
CREATE INDEX onboarding_items_session_idx ON onboarding_items(session_id);
CREATE UNIQUE INDEX onboarding_items_unique ON onboarding_items(session_id, item_type, item_target);

-- ── Event log (append-only, for plugin consumption) ────────────────
CREATE TABLE event_log (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  event_type    TEXT NOT NULL,
  payload       TEXT NOT NULL,              -- JSON
  created_at    INTEGER NOT NULL            -- unix ms
);
```

### 4.2 Design notes

- All IDs are **ULIDs** — chronologically sortable, URL-safe, no collisions.
- The `event_log` table provides a durable record for plugins that start after an event
  fires. The in-memory Event Bus handles real-time delivery; `event_log` provides
  replay/historical access.
- `ON DELETE SET NULL` on workspace/supersedes references — deleting a workspace or
  superseded decision does not cascade-delete linked decisions.
- `ON DELETE CASCADE` on decision_links — deleting a decision cleans up its links.
- The `decision.body` fields in SQLite are preview copies. The authoritative content
  is the Markdown file at `body_path`.

---

## 5. `.got/decisions/` file layout

Each decision gets a Markdown file at:

```
.got/decisions/<id>.md
```

Where `<id>` is the ULID with no hyphens (e.g., `01JQZ3ZABC1234567890ABCDEFG.md`).

The file uses a structured MADR-inspired template:

```markdown
# <title>

- **Status:** proposed
- **Created:** 2026-06-13
- **Workspace:** <workspace_name> (optional)

## Context

<context>

## Decision

<decision>

## Alternatives Considered

<alternatives>

## Consequences

<consequences>

## Links

- **Commits:** <sha1>, <sha2>
- **Files:** src/foo.go:42-58, src/bar.go (branch: feature/x)
- **Supersedes:** <superseded_id>
```

The SQLite row mirrors the frontmatter for efficient querying. The file is the
source of truth; on creation, both are written atomically.

---

## 6. CLI surface

### 6.1 Global flags

All `got decision`, `got note`, and `got onboard` commands support:

| Flag | Description |
|------|-------------|
| `--json` | Output in JSON format for machine consumption (list/show commands) |
| `--no-color` | Disable terminal styles |
| `--cwd <path>` | Operate on a different repository |

All list commands additionally support:

| Flag | Description |
|------|-------------|
| `--workspace <name>` | Filter by workspace |
| `--all` | Show all items (lists default to recent 20) |
| `--limit <n>` | Limit results (default 20, 0 for unlimited) |

The `decision list` command also supports:

| Flag | Description |
|------|-------------|
| `--status <s>` | Filter by status: proposed, accepted, rejected, superseded |

### 6.2 `got decision`

```
got decision create              # Interactive guided flow
got decision list                # Filterable table (--status, --workspace, --all, --json)
got decision show <id>           # Full rich view (rendered Markdown)
got decision link <id>           # Attach to commits, files, workspaces
got decision supersede <old> <new>  # Mark <old> as superseded, point to <new>
```

#### `got decision create`

Guided flow (can be overridden with flags for non-interactive use):

| Flag | Description |
|------|-------------|
| `--title <s>` | Decision title |
| `--context <s>` | Context/background |
| `--decision <s>` | The decision |
| `--alternatives <s>` | Alternatives considered |
| `--consequences <s>` | Positive/negative consequences |
| `--workspace <name>` | Scope to a workspace (auto-created if not found) |
| `--supersedes <id>` | ID of a decision this one supersedes |
| `--no-interactive` | Use all flags, do not prompt |

The interactive flow:
1. **Title** — single-line string, ≤ 80 chars
2. **Status** — default: `proposed`
3. **Context** — multi-line (textarea) in the TUI editor; or skip
4. **Decision** — multi-line (textarea)
5. **Alternatives** — multi-line; or skip
6. **Consequences** — multi-line; or skip
7. **Workspace** — optional, autocomplete from existing workspaces
8. **Supersedes** — optional, autocomplete from existing decisions
9. **Links** — optional, prompts to link commits/files
10. **Confirm** — preview of the rendered ADR and file path

On confirm:
- Writes `<id>.md` to `.got/decisions/`
- Inserts row into `decisions` table
- Inserts rows into `decision_links` if any
- Publishes `DecisionCreated` event
- If `--supersedes`, marks old decision as `superseded` and publishes `DecisionSuperseded`

#### `got decision list`

Output (default — table):

```
ID                  STATUS      TITLE                     WORKSPACE    CREATED
01JQZ3ZABC...       proposed    Use SQLite for storage    engine       2026-06-13
01JQZ4ZABC...       accepted    Adopt Bubbletea for TUI   tui          2026-06-12
```

With `--json`:

```json
[
  {
    "id": "01JQZ3ZABC...",
    "status": "proposed",
    "title": "Use SQLite for storage",
    "workspace_id": "01JQZ2ZABC...",
    "workspace_name": "engine",
    "created_at": 1750000000000
  }
]
```

#### `got decision show <id>`

Output: the full decision rendered in-terminal.

- Title rendered bold + colored
- Status badge (green for accepted, yellow for proposed, red for rejected, dim for superseded)
- Context, Decision, Alternatives, Consequences sections with heading rendering
- Links section showing commits, files, and supersedes chain
- Footer with timestamps and file path

With `--json`: same as the SQLite row as a JSON object.

#### `got decision link <id>`

Interactive linking flow (or flags):

| Flag | Description |
|------|-------------|
| `--commit <sha>` | Link to a commit |
| `--file <path>` | Link to a file, optionally `path:line` or `path:start-end` |
| `--branch <name>` | Specify branch context |
| `--workspace <id>` | Link to a different workspace |

#### `got decision supersede <old-id> <new-id>`

- Sets `status = 'superseded'` on `<old-id>`
- Sets `supersedes_id` on `<new-id>` to `<old-id>`
- Updates the `<old-id>.md` file's YAML frontmatter
- Publishes `DecisionSuperseded`

### 6.3 `got note`

```
got note add <message>          # Add a note (inline text only)
got note list                   # Filterable table (--workspace, --all, --json)
got note show <id>              # Basic rendered view (headings, lists, bold/italic)
```

#### `got note add "<message>"`

| Flag | Description |
|------|-------------|
| `--workspace <name>` | Scope to a workspace (auto-created if not found) |
| `--branch <name>` | Attach to a branch |
| `--commit <sha>` | Attach to a commit |

The message is a single string argument. Multi-line notes can be passed with
standard shell quoting:

```sh
got note add "Investigated the Bubbletea rendering pipeline.
Key insight: the viewport component handles scrolling internally."
```

#### `got note list`

Output (table or JSON), same pattern as `decision list`.

#### `got note show <id>`

Output: the note message with basic terminal rendering. Headings rendered bold,
lists indented, code blocks highlighted with a border.

### 6.4 `got onboard`

```
got onboard start               # Begin an onboarding session
got onboard progress            # Show current session status
got onboard mark <type> <id>    # Mark an item as covered
got onboard skip <type> <id>    # Skip an item
got onboard list                # List unwatched items
```

#### `got onboard start`

1. Detects the current participant (from `git config user.name` or `user.email`).
2. If an active session exists for this participant, resumes it.
3. Scans the repository:
   - Lists all decisions with `status` != `rejected` that haven't been seen in this session.
   - Lists all notes that haven't been seen.
   - Identifies files in the repo that are "new" — unwatched. (v0.4: this is a
     best-effort scan of tracked files. A future version could use the analyzer.)
4. Presents the list as a reading queue, grouped by type.
5. Publishes `OnboardingStarted` event.

#### `got onboard progress`

Shows a progress table:

```
Onboarding session for Alice (active)
Started: 2026-06-13

Type           Total    Covered    Skipped    Remaining
Decisions      12       5          1          6
Notes          8        3          0          5
Files          45       12         3          30

Overall: 20 / 65 items covered (31%)
```

With `--json`: returns structured JSON with counts.

#### `got onboard mark <type> <id>`

Marks an onboarding item as covered. `<type>` is one of `decision`, `note`, `file`.
Publishes `OnboardingItemCovered`.

#### `got onboard skip <type> <id>`

Marks an onboarding item as explicitly skipped. The item is not shown again unless
`--all` is used.

#### `got onboard list`

Lists all unwatched (not covered and not skipped) items for the active session.
Same output style as `decision list`.

---

## 7. Event Bus (minimal)

### 7.1 Design

A simple in-memory channel-based pub/sub bus. Part of the `internal/events` package.

```go
// Package events provides a minimal in-memory event bus for in-process
// pub/sub communication. It is designed for the v0.4 Knowledge Engine and
// is intentionally simple — no persistence, no delivery guarantees beyond
// best-effort synchronous dispatch to subscribers, no wildcard patterns.
package events

type Bus struct {
    mu     sync.RWMutex
    subs   map[string][]Handler
}

type Event struct {
    Type      string
    Payload   any
    Timestamp time.Time
}

type Handler func(ctx context.Context, e Event) error

func New() *Bus { ... }
func (b *Bus) Publish(ctx context.Context, eventType string, payload any) error { ... }
func (b *Bus) Subscribe(eventType string, handler Handler) func() { ... }  // returns unsubscribe
func (b *Bus) Close() error { ... }
```

**Behavior:**
- `Publish` dispatches to all registered handlers synchronously. A slow handler blocks
  the publisher. (This is intentional for v0.4 — it keeps the design simple. A future
  version can add async dispatch, timeouts, or a worker pool.)
- Handlers are called in registration order.
- `Subscribe` returns an `unsubscribe` function (a no-op if called more than once).
- `Close` drains all subscribers, waits for in-flight handlers (up to a timeout), and
  prevents further publishes.
- Thread-safe via `sync.RWMutex`.

### 7.2 Knowledge Engine events

The Knowledge Engine publishes these event types:

| Event | Payload | When |
|-------|---------|------|
| `DecisionCreated` | `{id, title, status, workspace_id, supersedes_id, created_at}` | After a decision is created |
| `DecisionUpdated` | `{id, title, status, previous_status, workspace_id, updated_at}` | After a decision status or field is updated |
| `DecisionSuperseded` | `{id, new_id, old_status, superseded_at}` | After a decision is superseded |
| `DecisionLinked` | `{decision_id, link_type, target, created_at}` | After a link is added |
| `NoteAdded` | `{id, workspace_id, branch, commit_hash, created_at}` | After a note is added |
| `OnboardingStarted` | `{session_id, participant, item_count, created_at}` | After onboarding starts |
| `OnboardingItemCovered` | `{session_id, item_type, item_target, covered_at}` | When an item is marked covered |
| `OnboardingCompleted` | `{session_id, participant, total_items, completed_at}` | When all items are covered or session ends |

### 7.3 Durable event log

Every event published through the bus is also appended to the `event_log` table
(see §4.1). This allows:
1. **Late-arriving subscribers** — plugins that start after an event occurred can
   replay past events from the log.
2. **Debugging** — `got knowledge events` (or similar diagnostic command) can dump
   the recent event history.
3. **Crash recovery** — the log is durable; the in-memory bus is best-effort.

An internal `EventLogger` subscribes to all events at bus construction time and
writes them to the SQLite `event_log` table.

---

## 8. Integration points

### 8.1 Workspace integration

- Decisions and notes can optionally be scoped to a workspace via `workspace_id`.
- When a `--workspace <name>` flag is provided and no workspace with that name exists,
   the workspace is auto-created. The auto-created workspace uses:
   ```sql
   INSERT INTO workspaces (id, name, created_at, state)
   VALUES (ulid(), <name>, now(), 'open');
   ```
- Filtering by workspace is supported on all list commands.

### 8.2 Git adapter integration

The Knowledge Engine needs limited read access to Git for:
- Resolving `--commit <sha>` links (validating the SHA exists)
- `got onboard start` — listing tracked files for scan purposes
- `got knowledge sync` — reconciling commit links after rebase/amend (see §10)

This should use the existing `GitAdapter` interface. No new methods are required
for v0.4; `Log` and `CurrentRef` suffice.

### 8.3 Plugin consumption

Plugins can:
1. Subscribe to in-memory events via `events.Bus.Subscribe()` if they are
   in-process (not supported in v0.4 for external plugins — see below).
2. Read events from `event_log` table directly (any plugin language with SQLite
   access).
3. Be notified via the NDJSON plugin protocol (§11 of got-spec.md) — a future
   v1.0 enhancement.

For v0.4, the primary integration path for external plugins is polling `event_log`.

---

## 9. `got knowledge sync` command (maintenance)

A maintenance command that reconciles knowledge links after Git operations.

```
got knowledge sync --commits    # Reconcile commit links (after rebase/amend)
```

Behavior:

1. **Commit link reconciliation** — walks all `decision_links` with
   `link_type = 'commit'`. For each SHA that no longer exists in the current
   branch history:
   - Checks the reflog for the old SHA → new SHA mapping.
   - Updates the link target to the new SHA.
   - Logs changes to stderr (or JSON with `--json`).
2. **File path verification** — checks that linked file paths still exist.
   Warns about broken links via stderr; does not auto-remove.

Future v0.5+ enhancements could add:
- `--workspaces` — sync workspace file lists with actual filesystem
- `--stale-decisions` — flag decisions that reference deleted workspaces
- `--auto-clean` — remove broken file links automatically

---

## 10. Edge cases & error handling

| Scenario | Behavior |
|----------|----------|
| `got decision create` in a non-GOT repo | Fails with "GOT not initialized" |
| `got decision show <id>` with invalid ID | Fails with "decision not found" |
| `got note add ""` (empty message) | Fails with "note message cannot be empty" |
| `got onboard start` with no existing session | Creates new session |
| `got onboard start` with active session | Resumes existing session |
| `got decision link --commit <invalid>` | Fails with "commit not found" |
| `got decision create --supersedes <missing>` | Fails with "superseded decision not found" |
| Decision body file is missing (.md deleted) | Warning on `show`, "file not found — showing from DB cache" |
| Two concurrent `got onboard start` calls | Each creates a separate session (ULID ensures no conflict) |
| Database locking | SQLite WAL mode handles concurrent readers; writers queue |

---

## 11. Testing strategy

| Test type | Tool | What to test |
|-----------|------|-------------|
| Store unit tests | `testing` + `testify` | All CRUD operations on decisions, notes, onboarding tables |
| CLI integration | `testscript` | Full command flows: create → list → show → link → supersede |
| Event bus | `testing` with goroutines | Publish/subscribe/unsubscribe, concurrency, handler errors |
| Onboarding scan | Mock file system | Auto-detection logic, progress tracking |
| Knowledge sync | `testscript` with real git | Commit link reconciliation after rebase |

### 11.1 testscript scenarios to commit

1. `got decision create --no-interactive --title "Use SQLite"` — creates and persists
2. `got decision list` — shows the created decision
3. `got decision show <id>` — displays full content
4. `got decision link <id> --commit HEAD` — links to current HEAD
5. `got decision supersede <old> <new>` — marks old as superseded
6. `got note add "log message"` — creates a note
7. `got note list --json` — machine-readable output
8. `got onboard start` — creates/resumes session
9. `got onboard mark decision <id>` — marks item covered
10. `got onboard progress` — shows progress percentages

---

## 12. ARCHITECTURE_KNOWLEDGE.md

The architecture document for the Knowledge Engine must cover:

1. **Data model** — entity relationships, SQL schema, file layout
2. **Command flows** — lifecycle of create, list, show, link, supersede, sync
3. **Event hooks** — which events are published, what consumes them, event_log table
4. **File layout** — `.got/decisions/` file naming, template, relationship to SQLite
5. **Future AI integration points** — where LLM summarization/generation could slot in:
   - `DecisionSuggester` interface (alternative to heuristic approaches)
   - Auto-summarization of linked decisions into onboarding reading
   - Natural language query interface for knowledge search

---

## 13. What v0.4 does NOT include

- AI summarization or generation (reserved for v0.5+)
- GitHub / PR integrations (reserved for plugin ecosystem)
- Any network features — everything is local-first
- Rich TUI for knowledge browsing (just CLI for v0.4; TUI tab deferred)
- Full-text search across decision bodies (use grep/fzf for now)
- Decision editing (update/deprecate handled via supersede chain)
- Note editing (v0.4: create + delete; no update)
- Event replay/subscription for external NDJSON plugins (deferred to v1.0)

---

## 14. Migration from v0.3

When v0.4 lands, the migration is:

1. Run `0004_knowledge.sql` via the existing migration framework.
2. Create `.got/decisions/` directory (exists already as a placeholder from v0.1).
3. No data migration needed — no decisions, notes, or onboarding data exists before v0.4.

---

## 15. Future (v0.5+) directions

After v0.4, the natural next steps (in priority order):

1. **Plugin Runtime v2** — enables external plugins to subscribe to events
   (NDJSON-based event stream) and register custom knowledge commands.
2. **Knowledge TUI tab** — add a "Knowledge" tab to `got tui` dashboard showing
   recent decisions, notes, and onboarding progress.
3. **Full-text search** — integrate SQLite FTS5 for searching decision bodies and notes.
4. **Decision editing** — `got decision update` for amending context/decision fields.
5. **AI summarization** — optional LLM-backed generation of decision summaries,
   onboarding reading lists, and cross-decision relationship detection.
6. **Template customization** — allow users to define custom ADR templates via `got.yml`.

---

*End of Knowledge Engine specification (v0.4).*
