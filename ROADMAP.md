# GOT Roadmap

GOT is a Git-native developer operating layer. This document outlines what's built, what's in progress, and what's planned.

## Current Status (v0.5)

| Feature | Status | Details |
|---|---|---|
| Git Adapter | ✅ Complete | Status, commits, branches, remotes, graph — 12 tests |
| Core CLI | ✅ Complete | `init`, `status`, `commit`, `branch`, `graph`, `remote` |
| Knowledge Engine | ✅ Complete | Decisions, notes, search, onboarding — 73 tests |
| Workspace Engine | ✅ Complete | 12 subcommands, Git integration, PR/issue display |
| Plugin Runtime v2 | ✅ Complete | Install/remove/enable/disable/run, event hooks, sample plugin |
| GitHub Integration | ✅ Complete | Auth, PR/issue CRUD, review, merge, diff, workspace linking |
| Event System | ✅ Complete | 21 event types, thread-safe pub/sub, event logger |
| CI/CD | ✅ Complete | GitHub Actions, GoReleaser, cross-platform builds |
| Shell Completions | ✅ Complete | bash, zsh, fish, powershell via `got completion` |
| Health Check | ✅ Complete | `got health` validates .got/, DB, Git, workspace consistency |

## Completed Milestones

### v0.1 — Foundation
- Git adapter with interface-based design
- SQLite persistence layer with migration framework
- In-memory event bus
- Core CLI commands (`init`, `status`, `commit`, `branch`, `graph`, `remote`)

### v0.4 — Knowledge & Workspaces
- Decision (ADR) CRUD with linking to commits, branches, workspaces
- Notes with workspace/branch/commit scoping
- Full-text search across decisions and notes
- Workspace engine: create, list, show, file/branch tracking, sync
- Onboarding session lifecycle

### v0.5 — Integration & GitHub
- Event-driven integration layer (auto-update workspaces on commit)
- Git-aware workspace validation (files, branches)
- `--auto-link` on commit for decisions and notes
- GitHub built-in plugin: auth, PR create/list/status/review/merge/diff
- GitHub issues: create, list, workspace filtering
- Plugin runtime v2: install, hooks, event bus, subprocess isolation
- PR reviews and merge tracking

## Next Up (v0.6)

### Snapshot Engine
- Recovery points before destructive operations
- `got snapshot create` / `got snapshot restore`
- Automatic snapshots before `reset`, `rebase`, `force push`

### Safe Operations
- `got safe reset` — reset with automatic snapshot
- `got safe rebase` — rebase with recovery point
- `got safe push --force-with-lease` — force push with safety net

## Future (v0.7 — v1.0)

### Health Engine
- Stale branch detection
- Repository analytics (churn, ownership, commit frequency)
- `got health` reports

### Advanced Workspace Features
- Workspace templates
- Cross-workspace search
- Workspace-aware graph view

### Platform Expansion
- GitLab integration plugin
- Bitbucket integration plugin
- CI/CD integrations (GitHub Actions, GitLab CI)

### TUI Dashboard
- Interactive terminal dashboard with Bubbletea
- Status, branches, remotes, graph, plugins tabs

### AI Integration
- Commit message suggestions
- Decision drafting assistance
- Code review suggestions

## Design Principles

These principles guide all design decisions:

1. **Git is the source of truth.** GOT never modifies Git in ways the user didn't ask for.
2. **Metadata is isolated.** Everything lives in `.got/`. The repo stays usable without GOT.
3. **Every operation must be recoverable.** Destructive operations create snapshots first.
4. **Offline-first.** No network calls except those the user initiates.
5. **Plugin-first.** Core features use the same APIs available to plugins.
6. **Interface-based.** Every subsystem has a Go interface for testability and swappability.

---

*For the original v0.1 specification, see `docs/got-spec.md`.*
