# Git Adapter & Core Commands Architecture

## Overview

GOT's Git Adapter (`internal/git/`) provides a thin, testable interface over the `git` CLI via `os/exec`. It exposes repository-scoped operations for status, commits, branches, remotes, and commit graphs — all without global state. The adapter is designed to be fully offline and does not depend on any external Go libraries (no go-git).

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    CLI Layer (internal/cli/)                  │
│                                                              │
│  got init    got status    got commit    got branch           │
│  got graph   got remote                                      │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                   Git Adapter (internal/git/)                 │
│                                                              │
│  GitAdapter (interface) ──► ExecAdapter (os/exec)            │
│    │                                                         │
│    ├── OpenRepository / Root                                 │
│    ├── GetStatus / CurrentBranch                             │
│    ├── CreateCommit / GetCommitHistory                       │
│    ├── CreateBranch / CheckoutBranch / DeleteBranch          │
│    ├── ListBranches                                          │
│    ├── GetRemotes / AddRemote / RemoveRemote                 │
│    ├── Push / Pull                                           │
│    ├── GetGraph                                              │
│    └── Run (raw git command passthrough)                     │
└───────────┬─────────────────────────────────────────────────┘
            │
            ▼
┌─────────────────────────────────────────────────────────────┐
│                    Event Bus (internal/events/)               │
│                                                              │
│  RepositoryOpened   BranchCreated   BranchDeleted            │
│  BranchCheckedOut   CommitCreated   RemoteAdded              │
│  RemoteRemoved      PushCompleted   PullCompleted            │
└─────────────────────────────────────────────────────────────┘
```

## Adapter Design

### Interface vs Implementation

The `GitAdapter` interface is defined in `internal/git/adapter.go`. The sole production implementation is `ExecAdapter`, which shells out to the `git` CLI. This design makes it easy to:

- Swap implementations (e.g., a mock for testing)
- Add go-git as a second backend later
- Test the adapter independently with temporary Git repos

### Repository Scope

Each `ExecAdapter` instance is bound to a single repository path. Callers must call `OpenRepository(ctx, path)` before any other operation. This prevents accidental cross-repository operations and keeps adapter instances stateless beyond the repo path.

### Error Handling

- All errors are wrapped with context (operation name, branch name, etc.)
- `stderr` output is included in error messages for debugging
- Non-zero git exits are expected for many common cases (detached HEAD, no commits, etc.) and handled gracefully
- The `--ff-only` flag is attempted first for `Pull()`, falling back to regular pull

### Event Publishing

The adapter accepts an optional `*events.Bus` at construction. When provided, it publishes events for every mutation operation:
- `RepositoryOpened` — adapter initialized for a path
- `BranchCreated` / `BranchDeleted` / `BranchCheckedOut` — branch lifecycle
- `CommitCreated` — new commits
- `RemoteAdded` / `RemoteRemoved` — remote lifecycle
- `PushCompleted` / `PullCompleted` — push/pull results

Events are best-effort (errors are silently dropped via `_`).

## CLI Commands

### `got init`

Initializes GOT metadata in a Git repository. If no Git repository exists, it:
1. Runs `git init -b main`
2. Configures a default user identity (best-effort if global config exists)
3. Creates an initial `README.md` with a project name header
4. Stages and commits the README so HEAD is never unborn

Then creates the `.got/` directory structure, SQLite database, migrations, `got.yml` config, and appends `.got/` to `.gitignore`.

### `got status`

Shows the working tree status (similar to `git status`). Uses `git status --porcelain` for machine-readable parsing. Displays:
- Current branch name
- Staged changes (index status)
- Unstaged changes (worktree status)
- Untracked files

Supports `--json` for machine-readable output.

### `got commit`

Creates a commit with a message. Options:
- `-m "message"` — commit message (required)
- `-a` / `--all` — stage all tracked files before committing
- `--allow-empty` — allow empty commits

Displays the commit SHA after success.

### `got branch`

List, create, delete, and checkout branches.

Subcommands:
- `got branch` — list branches with current marker and upstream info
- `got branch create <name>` — create a branch at HEAD
- `got branch delete <name>` — delete a branch (`-f` for force)
- `got branch checkout <name>` — switch to a branch

Supports `--json` for the list command.

### `got graph`

Displays a text-based commit graph with parent-child relationships. Shows:
- Short SHA (8 chars) and commit message
- Branch/tag decorations
- Merge commits with parent SHAs

Options: `--branch`, `--max-count` (default 20).

### `got remote`

Manage Git remotes.

Subcommands:
- `got remote` — list remotes
- `got remote add <name> <url>` — add a remote
- `got remote remove <name>` — remove a remote
- `got remote push <remote> <branch>` — push (supports `--force`)
- `got remote pull <remote> <branch>` — pull (tries `--ff-only` first)

Supports `--json` for the list command.

## Data Model

```
GitAdapter (repo-scoped)
│
├── Status
│   ├── Branch (string)
│   ├── Clean (bool)
│   ├── Staged []StatusEntry  (IndexStatus, WorktreeStatus, Path, OldPath)
│   ├── Unstaged []StatusEntry
│   └── Untracked []string
│
├── Commit
│   ├── SHA, Message, Author, Date, Refs
│
├── Branch
│   ├── Name, Current, Remote, SHA, Upstream
│
├── Remote
│   ├── Name, URL, PushURL
│
├── GraphNode
│   ├── SHA, Message, Parents []string, Refs
│
└── PushResult / PullResult
        └── Output, FastForward (pull only)
```

## Testing

The adapter is fully testable with temporary Git repositories:

1. `newTestRepo(t)` helper creates a temp dir, runs `git init`, configures identity, creates an initial commit
2. Each test creates its own repo via `defer os.RemoveAll(repoPath)`
3. Tests cover: clean/dirty status, branch CRUD, checkout, commit history, remotes, graph, and commit creation
4. No global state — tests are fully isolated

## Future Integration

### Workspace Integration (v0.5)

The Git adapter now powers real workspace ↔ Git validation:

- **`got workspace add-file`** — validates the path exists on disk or in the Git working tree via `GetStatus()`
- **`got workspace add-branch`** — validates the branch exists via `ListBranches()`
- **`got workspace show`** — resolves branch info (exists, clean/dirty, ahead/behind, latest commit) by calling `ListBranches()`, `GetStatus()`, and `rev-list --left-right --count`
- **`got workspace status`** — shows current Git branch and clean/dirty status using `CurrentBranch()` and `GetStatus()`
- **`got workspace sync`** — compares tracked files/branches against real Git state, detects stale items
- **`got commit --auto-link`** — after creating a commit, auto-links unlinked decisions/notes and updates workspace activity via `AddWorkspaceCommit()`
- **Event-driven updates** — the `IntegrationService` subscribes to `CommitCreated` events and automatically records commits in workspaces tracking that branch

### GitHub / Remote Integration

Future network features can build on the remote operations:

- `got pr create` — push branch + create PR via GitHub CLI or API
- `got pr list` — list open PRs
- `got sync` — pull + push in sequence with workspace metadata sync

### Plugin System

Events emitted by the Git adapter can trigger plugin hooks:

- On `CommitCreated`, run a post-commit hook (e.g., update linked decisions)
- On `BranchCheckedOut`, update workspace status
- On `PushCompleted`, trigger deployment or CI

## File Layout

```
internal/git/
  adapter.go          GitAdapter interface, ExecAdapter, domain types
  operations.go       GetStatus, CurrentBranch, branch CRUD, commit history
  remote_graph.go     GetRemotes, AddRemote, RemoveRemote, Push, Pull, GetGraph
  adapter_test.go     Tests for all adapter operations

internal/cli/
  init.go             got init (with git repo initialization)
  status.go           got status
  commit.go           got commit
  branch.go           got branch
  graph.go            got graph
  remote.go           got remote

internal/events/
  event.go            Git event constants and payloads
```
