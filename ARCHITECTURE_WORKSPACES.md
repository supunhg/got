# Workspace Engine — Architecture

> Design document for the Logical Workspace Engine (v0.4).
> Last updated: 2026-06-14

---

## Overview

Workspaces are **logical groupings** of related artifacts — files, branches,
decisions, notes — into named contexts like `"oauth"`, `"payment-refactor"`,
or `"k8s-migration"`. They are organizational units for **human intent**,
not tied to Git worktrees or branches.

A workspace represents an area of work or concern. It collects everything
relevant to that concern so you can see the full picture in one place.

### Core principles

1. **Not tied to Git.** Workspaces are purely logical. They don't require
   a Git repository to exist, though they will eventually integrate with
   Git data.
2. **Human-readable names.** Workspaces are identified by user-chosen names
   (slugs like `"oauth"` or `"payment-refactor"`), not opaque IDs.
3. **Best-effort references.** Files and branches are stored as string
   references. A future version will connect them to actual Git paths
   and refs.
4. **Graceful cleanup.** Deleting a workspace clears its associations but
   does NOT delete the referenced decisions or notes.

---

## Data Model

### Tables (in `.got/got.db`)

```sql
-- Core workspace record
workspaces (
  id          TEXT PRIMARY KEY,    -- ULID
  name        TEXT NOT NULL UNIQUE,-- human-readable slug
  description TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'active'
              CHECK (status IN ('active', 'archived')),
  tags        TEXT NOT NULL DEFAULT '[]',  -- JSON array
  created_at  INTEGER NOT NULL,    -- unix ms
  updated_at  INTEGER NOT NULL     -- unix ms
)

-- File paths tracked within a workspace
workspace_files (
  id           TEXT PRIMARY KEY,   -- ULID
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  path         TEXT NOT NULL,      -- file path (string ref only)
  created_at   INTEGER NOT NULL,   -- unix ms
  UNIQUE(workspace_id, path)
)

-- Branch names tracked within a workspace
workspace_branches (
  id           TEXT PRIMARY KEY,   -- ULID
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  branch_name  TEXT NOT NULL,      -- branch name (string ref only)
  created_at   INTEGER NOT NULL,   -- unix ms
  UNIQUE(workspace_id, branch_name)
)
```

### Linking to Knowledge Engine entities

Decisions and notes are linked to workspaces via the existing `workspace_id`
column on the `decisions` and `notes` tables. This is a string reference
storing the workspace **name** (not the ULID), making it human-readable
in queries and output.

The workspace engine also has a `workspace_links` table (polymorphic) that
is reserved for future use when linking to entities without a native
`workspace_id` column.

### Migration

The schema is applied by migration `0005_workspaces.sql`, which is embedded
and auto-applied at `store.Open()`. Existing databases with older migrations
will pick up the new tables on next launch.

---

## Event Integration

Every mutating workspace operation publishes an event on the in-process
event bus (`internal/events`). This enables plugins, the event log, and
future subscribers to react to workspace changes.

| Event                    | Published When          | Payload                          |
|--------------------------|--------------------------|----------------------------------|
| `WorkspaceCreated`       | Workspace is created     | ID, Name, Description, Tags, CreatedAt |
| `WorkspaceUpdated`       | Workspace is updated     | ID, Name, Description, Tags, UpdatedAt |
| `WorkspaceDeleted`       | Workspace is deleted     | ID, Name, ItemCount, DeletedAt   |
| `WorkspaceItemAdded`     | File/branch added        | WorkspaceID, ItemType, ItemTarget, CreatedAt |
| `WorkspaceItemRemoved`   | File/branch removed      | WorkspaceID, ItemType, ItemTarget, RemovedAt |

Events are defined in `internal/events/event.go` with typed payload structs
in the `events` package.

---

## CLI Surface

```
got workspace                              # same as list
got workspace create <name> [flags]        # create new workspace
got workspace delete <name>               # delete workspace (clears associations)
got workspace list [flags]                # list all workspaces
got workspace show <name> [flags]          # show workspace details + contents
got workspace add-file <ws> <path>         # track a file path
got workspace add-branch <ws> <branch>     # track a branch name
got workspace add-note <ws> "text"         # create note + link to workspace
got workspace add-decision <ws> <id>       # link existing decision
got workspace remove-file <ws> <path>      # untrack a file
got workspace remove-branch <ws> <branch>  # untrack a branch
got workspace status <ws> [flags]          # summary with item counts
```

### Flags

- `--description` (create): Human-readable description
- `--tags` (create): Repeatable or comma-separated tags
- `--json` (list/show/status): Machine-readable JSON output
- `--no-interactive` (create): Skip prompts, use flags only
- `--force` (delete): Skip confirmation

### Output format

Table output for `list` uses `tabwriter` for aligned columns:
```
NAME                STATUS    DESCRIPTION     TAGS          CREATED
oauth               active    OAuth 2.0 impl  auth,sec      2026-06-14
payment-refactor    archived  Payment system   infra        2026-06-10
```

---

## Store Layer

All workspace operations are methods on `store.KnowledgeStore`, the same
struct that handles decisions, notes, onboarding, and search.

### Key methods

| Method                          | Description                                    |
|---------------------------------|------------------------------------------------|
| `CreateWorkspace`               | Insert workspace, publish event                |
| `GetWorkspace(name)`            | Fetch by name (user-facing identifier)         |
| `GetWorkspaceByID(id)`          | Fetch by ULID                                  |
| `ListWorkspaces`                | All workspaces, ordered by name                |
| `UpdateWorkspace(name, params)` | Update mutable fields, publish event           |
| `DeleteWorkspace(name)`         | Delete workspace, clear associations, publish  |
| `AddWorkspaceFile(ws, path)`    | Track a file path                              |
| `RemoveWorkspaceFile(ws, path)` | Untrack a file path                            |
| `ListWorkspaceFiles(ws)`        | List tracked files                             |
| `AddWorkspaceBranch(ws, name)`  | Track a branch name                            |
| `RemoveWorkspaceBranch(ws, br)` | Untrack a branch name                          |
| `ListWorkspaceBranches(ws)`     | List tracked branches                          |
| `GetWorkspaceStatus(ws)`        | Full summary: metadata + files + branches + decisions + notes |

### Sentinel errors

- `ErrWorkspaceNotFound`: workspace name does not exist
- `ErrDuplicateWorkspace`: workspace name already exists (UNIQUE violation)

---

## Future Git Integration

Once the Git adapter is built (v0.1 scope), workspace integration will add:

1. **`got workspace status` with Git data** — show whether tracked files
   have uncommitted changes, whether tracked branches exist, and whether
   they're ahead/behind.
2. **`got workspace diff`** — show changes across all tracked files.
3. **`got workspace sync`** — ensure all tracked branches exist.
4. **`got workspace summary`** — Git-aware activity report.

The current string-reference design makes this straightforward: the Git
adapter just needs to resolve `workspace_files.path` against the repo
and `workspace_branches.branch_name` against `git branch --list`.

---

## Testing Strategy

| Layer    | Approach                                      |
|----------|-----------------------------------------------|
| Store    | Temp SQLite DB, `NewKnowledgeStore`, assert on returned structs and events |
| CLI      | `testCLIEnv` helper + `newTestCmd` capturing stdout |
| Events   | Subscribe to bus before operation, verify payload |

Tests follow the same patterns as the existing Knowledge Engine tests
in `internal/store/knowledge_test.go` and `internal/cli/decision_test.go`.
