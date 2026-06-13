# GOT Workspace Engine (v0.4)

The Workspace Engine is GOT's mechanism for grouping **logical
repository knowledge** — files you care about, branches you're
working on, decisions you've made, free-form notes — under a
single named aggregate that lives in `.got/got.db` and is
independent of Git worktrees and Git branches.

The engine is one of v0.4's three feature pillars (per
`ARCHITECTURE.md` and `got-spec.md` §12/§21). It is fully
implemented by the `internal/workspace` package and exposed
through the `got workspace` CLI tree.

## Why a Workspace Engine?

Git itself knows about commits, branches, tags, and worktrees.
GOT adds three orthogonal layers of metadata on top:

| Layer            | Engine       | Stored in                  |
|------------------|--------------|----------------------------|
| Workflow         | Workspaces   | `.got/got.db` (SQLite)     |
| Safety           | Snapshots    | `.got/got.db` + `snapshots/`|
| Intelligence     | Health runs  | `.got/got.db` + `health/`   |

A **workspace** is the user-facing unit of "what am I working
on right now?" It binds together the moving parts of a piece of
work — the files that matter, the branches that carry the work,
the decisions that shaped it, the notes that capture context — so
the dashboard, the CLI, and the future plugin ecosystem can all
render the same picture.

## Independence from Git

A workspace does **not** own or modify any Git-level state:

- It does not create or remove branches. The `workspace_branches`
  table records *which* branches the user has tagged as
  relevant, but `git branch` is untouched.
- It does not create or remove worktrees. The `git worktree`
  porcelain (and GOT's `.got/worktrees.json` sidecar) is a
  separate concern; the two packages share only the SQLite
  database file.
- It does not require any file to exist. `got workspace add-file
  x internal/auth/oauth.go` records the path even if the file
  has not been created yet.

This independence is what makes the engine **offline**, **plugin
compatible**, and **fully testable** — none of the operations
touch the working tree, none shell out to `git`, and none make
network calls. The package can be exercised end-to-end against a
`/tmp/foo/got.db` with no Git binary anywhere on the system.

## Data Model

Five entities, defined in `internal/workspace/types.go`:

| Entity            | Purpose                                     | Storage                          |
|-------------------|---------------------------------------------|----------------------------------|
| `Workspace`       | Root aggregate: name, title, state, color   | `workspaces` (1 row)             |
| `WorkspaceFile`   | Repo-relative path the user wants to track  | `workspace_files` (N rows)       |
| `WorkspaceBranch` | Git branch the user has tagged as relevant  | `workspace_branches` (N rows)    |
| `WorkspaceDecision` | Lightweight ADR scoped to the workspace   | `workspace_decisions` (N rows)   |
| `WorkspaceNote`   | Free-form markdown note                     | `workspace_notes` (N rows)       |

The full schema is in `internal/store/migrations/0002_workspaces.sql`.
`ON DELETE CASCADE` from every child table to `workspaces` makes
`got workspace delete <name>` atomic: the workspace, its files,
its branches, its decisions, and its notes all disappear in a
single transaction.

### Entity details

**Workspace** is keyed by a unique slug `name` (regex
`^[a-z][a-z0-9_-]{0,62}$`) and has an opaque `id` for database
joins. The slug is the user-facing identifier; the ID is an
implementation detail plugins and external scripts should
not depend on. The `state` column is `open` or `archived`; the
default is `open`. The `metadata` column is a JSON blob
intended for plugin extensions (e.g. a code-review plugin could
record the most recent PR number under the workspace).

**WorkspaceFile** is `(workspace_id, path)` keyed. Re-adding the
same path is idempotent: the existing note is overwritten and
`added_at` is bumped. Notes are short, free-form annotations
("touched in PR #42", "needs review", etc.).

**WorkspaceBranch** is `(workspace_id, branch)` keyed. The branch
is recorded by short ref name (e.g. `feature/x`); the workspace
does not own the branch. `last_seen_at` is in the schema for
future staleness detection but is not written by the v0.4 CLI.

**WorkspaceDecision** carries a UUID, a title, a markdown body,
and a status from the same vocabulary as the global ADR table
(`proposed` | `accepted` | `rejected` | `superseded`). The body
is preserved verbatim; the CLI never interprets it. Decisions
are intentionally separate from the global `decisions` table so
workspace-scoped ADRs don't pollute the repo-wide ADR list.

**WorkspaceNote** is the lightest entity: a markdown body and a
`pinned` flag. Pinned notes float to the top of `got workspace
show` and are typically used for "what this workspace is about"
or "current focus / blockers".

## Package API

The `internal/workspace` package exposes a `Store` with
first-class CRUD for every entity:

```go
// Workspace lifecycle
Create(ctx, *Workspace) error
Get(ctx, idOrName string) (*Workspace, error)
List(ctx, ListOptions) ([]*Workspace, error)
Update(ctx, *Workspace) error
Delete(ctx, idOrName string) error    // cascades to children

// Files
AddFile(ctx, workspaceID, path, note string) error
RemoveFile(ctx, workspaceID, path string) error
ListFiles(ctx, workspaceID string) ([]WorkspaceFile, error)

// Branches
AddBranch(ctx, workspaceID, branch string) error
RemoveBranch(ctx, workspaceID, branch string) error
ListBranches(ctx, workspaceID string) ([]WorkspaceBranch, error)

// Decisions (full CRUD)
AddDecision(ctx, *WorkspaceDecision) error
UpdateDecision(ctx, *WorkspaceDecision) error
RemoveDecision(ctx, id string) error
GetDecision(ctx, id string) (*WorkspaceDecision, error)
ListDecisions(ctx, workspaceID string) ([]WorkspaceDecision, error)

// Notes (full CRUD; no Get — List is the canonical view)
AddNote(ctx, *WorkspaceNote) error
UpdateNote(ctx, *WorkspaceNote) error
RemoveNote(ctx, id string) error
ListNotes(ctx, workspaceID string) ([]WorkspaceNote, error)

// Aggregated
Show(ctx, idOrName string) (*showView, error)  // workspace + 4 child lists
```

### Construction

```go
// Production: open the SQLite store, wrap it.
s, err := store.Open(repo.NewPaths(workTree).DBFile)
ws := workspace.New(s)

// Tests: skip the store package, use a raw *sql.DB.
ws := workspace.NewWithDB(db, func() time.Time { return pinnedTime })
```

The `Store` does **not** own the database lifetime; callers open
and close the underlying `*store.Store`. This keeps the package
trivially testable (a `t.TempDir()` + `store.Open` is enough) and
lets the CLI share one DB across many workspace operations
without re-opening.

### Errors

`internal/workspace/errors.go` defines the sentinel errors and
typed error structs the API can return:

- `ErrNotFound` — no workspace matches the id/name.
- `ErrNameTaken` — `Create` saw a UNIQUE collision.
- `ErrEmptyTitle` — `Create` was called with an empty title.
- `*ErrInvalidName` — name doesn't match the slug regex.
- `*ErrInvalidState` — state is not `open` or `archived`.
- `*ErrInvalidDecisionStatus` — status is not one of the four
  known values.

Use `errors.Is(err, workspace.ErrNotFound)` for sentinel checks
and `errors.As(err, &badName)` to read the offending name out of
`*ErrInvalidName`.

## CLI surface

The `got workspace` command tree (in `internal/cli/workspace.go`)
is the primary user-facing layer. Seven subcommands per the
implementation spec:

```
got workspace
got workspace list [--state open|archived] [--all] [--json]
got workspace create <name> [--title T] [--description D] [--color C] [--metadata JSON]
got workspace delete <name>
got workspace show <name> [--json]
got workspace add-file <name> <path> [--note N]
got workspace add-branch <name> <branch>
got workspace add-note <name> [--body B|-] [--stdin] [--pinned]
```

`got workspace` with no args defaults to the same as
`got workspace list` (open workspaces, table output). Pass
`--json` on `list` or `show` for machine-readable output; the
JSON shape is the source of truth for plugin authors and is
covered by `internal/cli/workspace_test.go::TestWorkspaceCmd_ShowJSON`.

The CLI is non-interactive by design. `add-note --body "-"`
(equivalent to `--stdin`) is the only escape hatch for multi-line
content; the wizard-style "open $EDITOR" path is intentionally
deferred so the engine ships in v0.4 without depending on
`$EDITOR` resolution.

## Plugin compatibility

Plugins in v0.1+ are external binaries that communicate with
GOT over NDJSON (got-spec.md §11). The Workspace Engine is
plugin-compatible by design:

1. **Read access**: a plugin can shell out to
   `got workspace list --json` or `got workspace show <name> --json`
   and parse the result. The JSON shape is stable and the
   subject of `TestWorkspaceCmd_ShowJSON` so a schema break is
   caught in CI.
2. **Write access**: a plugin can shell out to any of the
   `got workspace add-*` / `delete` / `create` commands. The
   CLI exits non-zero on validation failure, so a plugin can
   detect a bad workspace name and report it to the user.
3. **Direct DB access**: a plugin can open `.got/got.db`
   directly using the same `modernc.org/sqlite` driver GOT
   uses, and call into `internal/workspace` via a Go plugin
   module. This is the highest-fidelity path and bypasses the
   CLI's validation layer.

The `--metadata` flag on `create` is the one extension point
the engine ships with: a JSON object stored in
`workspaces.metadata` is opaque to the engine but freely
readable by any plugin that knows the workspace's name or ID.

## Schema migration

`internal/store/migrations/0002_workspaces.sql` is the migration
that brings the workspace tables to their v0.4 shape. It drops
the v0.1 forward-compat stubs and recreates them with the
expanded columns, plus adds the three new child tables
(`workspace_branches`, `workspace_decisions`, `workspace_notes`).

The drop+recreate is safe because no v0.1 command ever wrote to
`workspaces` or `workspace_files`; the tables existed for
forward compatibility only. `Store.SchemaVersion` is bumped from
1 to 2 and recorded in the `meta` table on migration completion;
`store_test.go::TestMigrateBodyContainsCreateTable` asserts the
new tables are present after the runner applies 0002.

## Testing strategy

The engine is fully tested across two layers:

- **Package-level** (`internal/workspace/store_test.go`): every
  method has at least one test, the cascading-delete behavior is
  exercised end-to-end, the slug/state/decision-status
  validation is covered by table-driven tests, and
  `TestNewIDFormat` pins the ID format so a future bump doesn't
  silently change the wire format.
- **CLI-level** (`internal/cli/workspace_test.go`): every command
  has a happy-path test, every documented error path
  (validation, not-found, not-in-git-repo, not-initialized) has
  a test, and the JSON output is round-tripped through
  `json.Unmarshal` to prove the shape is stable.

Both test files use a `t.TempDir()`-backed SQLite store; the
tests do not require a Git binary and do not make network calls.
This is what makes the engine "fully testable" in the
implementation brief.

## Out of scope for v0.4

The engine is intentionally narrow. The following are deferred
to a later milestone and are explicitly **not** in this build:

- **Staleness detection.** `last_seen_at` on
  `workspace_branches` is in the schema but no command writes to
  it. v0.5+ will populate it from `git for-each-ref` and surface
  stale branches in `got workspace show`.
- **Cross-workspace queries.** "List all workspaces tracking
  file `internal/auth/oauth.go`" requires a
  `workspace_files.path` index. The data is in the DB; the
  command is not. Add `got workspace where-file <path>` in v0.5.
- **Editor integration.** `got workspace add-note` reads from
  `--body` or `--stdin`. An `--editor` flag that opens `$VISUAL`
  / `$EDITOR` on a tempfile would be a small addition in a
  later milestone.
- **Workspace-to-ADR linking.** The `workspace_decisions` table
  is independent of the global `decisions` table. A future
  feature could promote a workspace decision to a global ADR
  (or vice versa) without a schema change.
- **Workspace TUI tab.** The `got tui` dashboard is the obvious
  place to render a workspace list, but that is owned by the
  `dashwiz` package; the integration is a v0.5 task.

These items are tracked as followups and intentionally kept
out of this PR so the engine ships with a clean, narrow API.
