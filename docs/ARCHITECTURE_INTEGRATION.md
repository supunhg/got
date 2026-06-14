# Integration Layer — Architecture

> Event-driven integration between the Git Adapter, Workspace Engine,
> Knowledge Engine, and Event Bus.
> Last updated: 2026-06-14

---

## Overview

The Integration Layer is the "glue" that makes GOT's subsystems feel
cohesive. It uses the in-process Event Bus (`internal/events`) to
automatically react to changes across the Git adapter, Workspace
Engine, and Knowledge Engine.

### How it works

```
┌─────────────────────────────────────────────────────────────────┐
│                        Event Bus                                │
│                                                                  │
│  CommitCreated ──► onCommitCreated() ──► Update workspace       │
│  WorkspaceCreated ──► (optional --create-branch)                 │
│  DecisionCreated ──► (planned: auto-link to pending commit)     │
│  NoteAdded ──► (planned: auto-link to pending commit)           │
└─────────────────────────────────────────────────────────────────┘
```

The `IntegrationService` (`internal/cli/integration.go`) subscribes to
events and performs automatic actions:

1. **CommitCreated**: Checks all workspaces for branches matching the
   commit's branch. If found, records the commit in `workspace_commits`
   and updates `last_commit_sha` on the workspace.

2. **Auto-link on commit** (`--auto-link` flag on `got commit`): Finds
   decisions and notes created since the last commit (without existing
   commit links) and links them to the new commit.

---

## Data Model Additions (Migration 0006)

### `workspace_commits` table

```sql
CREATE TABLE workspace_commits (
  id            TEXT PRIMARY KEY,           -- ULID
  workspace_id  TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  commit_sha    TEXT NOT NULL,
  branch_name   TEXT NOT NULL DEFAULT '',
  message       TEXT NOT NULL DEFAULT '',   -- commit message (cached)
  created_at    INTEGER NOT NULL,            -- when the link was created
  UNIQUE(workspace_id, commit_sha)
);
```

### `last_commit_sha` on `workspaces`

```sql
ALTER TABLE workspaces ADD COLUMN last_commit_sha TEXT NOT NULL DEFAULT '';
```

This column provides fast access to the most recent commit SHA for
display in `got workspace list` and `got workspace status`.

---

## CLI Integration Points

### Workspace Commands (Git-aware)

| Command | Git Integration |
|---------|----------------|
| `got workspace add-file <ws> <path>` | Validates file exists on disk or in Git working tree |
| `got workspace add-branch <ws> <branch>` | Validates branch exists in repository |
| `got workspace show <ws>` | Resolves branch info (exists, ahead/behind, latest commit) |
| `got workspace status <ws>` | Shows last commit SHA |
| `got workspace sync <ws>` | Detects stale files (not on disk) and stale branches (not in repo), optionally removes them |
| `got workspace create <name> --create-branch` | Creates a Git branch with the same name |

### Decision Link Extensions

| Flag | Behavior |
|------|----------|
| `--auto` | Resolves `HEAD` and links to the most recent commit |
| `--branch <name>` | Sets branch context for the link |
| `--branch <name> --branch-link` | Links to all commits on the specified branch |

### Commit Auto-Link

The `--auto-link` flag on `got commit`:

1. Finds all decisions not yet linked to any commit
2. Links them to the new commit if created after the previous commit
3. Updates all notes without a `commit_hash` to reference the new commit
4. Updates workspace activity for workspaces tracking the current branch

### Event-Driven Updates

The `IntegrationService` subscribes to the event bus and automatically:

- On `CommitCreated`: records the commit in any workspace tracking
  the branch, updating `last_commit_sha` and `updated_at`.

---

## Testing Strategy

| Test | Approach |
|------|----------|
| Workspace → Git validation | Create real Git repo with known files/branches, add them to workspace, verify validation passes |
| Workspace sync | Create workspace, delete a file from disk, run sync, verify stale detection |
| Event-driven commit linking | Mock a CommitCreated event, verify workspace gets updated |
| Auto-link | Create decisions/notes, commit with --auto-link, verify links created |
| Decision link --auto | Use Git adapter to create HEAD, link a decision, verify SHA |
| File validation | Try to add non-existent file, verify helpful error message |

---

## Future Plans

### Plugin Runtime

The events emitted by the Git adapter and Knowledge Engine are designed
to be consumed by external plugins:

- On `CommitCreated`, a GitHub plugin could automatically push
- On `BranchCreated`, a CI plugin could trigger a pipeline
- On `DecisionLinked`, a documentation plugin could update ADR pages

### Planned Integration Points

1. **Auto-link decisions on commit** — when a decision record exists and
   a commit is made, auto-link if the commit message references the
   decision ID.

2. **Workspace snapshot on commit** — automatically update workspace
   file hashes/timestamps when a tracked file is part of a commit.

3. **Branch lifecycle events** — when a workspace-tracked branch is
   deleted, offer to remove it from the workspace.
