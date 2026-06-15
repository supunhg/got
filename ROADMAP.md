# GOT Roadmap

GOT is a Git-native developer operating layer. This document outlines what's built, what's in progress, and what's planned.

## Current Status (v1.0.0)

| Feature | Status | Details |
|---|---|---|
| Git Adapter | ✅ Complete | Status, commits, branches, remotes, graph — 12 tests |
| Core CLI | ✅ Complete | `init`, `status`, `commit`, `branch`, `graph`, `remote` |
| Knowledge Engine | ✅ Complete | Decisions, notes, search, onboarding — 73 tests |
| Workspace Engine | ✅ Complete | 12 subcommands, Git integration, PR/issue display |
| Plugin Runtime v2 | ✅ Complete | Install/remove/enable/disable/run, event hooks, sample plugin |
| GitHub Integration | ✅ Complete | Auth, PR/issue CRUD, review, merge, diff, workspace linking |
| Event System | ✅ Complete | 21 event types, thread-safe pub/sub, event logger |
| Snapshot Engine | ✅ Complete | Create/list/show/delete, auto-snapshot before destructive ops |
| Safe Operations | ✅ Complete | `got safe reset/push/rebase` with automatic snapshots |
| CI | ✅ Complete | GitHub Actions (test, vet, lint, format, build) |
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

### v0.6 — Snapshots & Safe Operations
- Snapshot engine: create/list/show/delete recovery points
- Automatic snapshots before destructive operations
- `got safe reset/push/rebase` with safety net
- Shell completions (bash, zsh, fish, powershell)
- Health check command

### v1.0.0 — Stable Release
- All features stable and tested (comprehensive test coverage, race-clean)
- Cross-platform builds (darwin/linux/windows × amd64/arm64)
- GitHub Actions CI (test, vet, lint, format check, build)
- CLI-level tests for all new commands

## Next Up (v1.1)

### Rich TUI
- Interactive terminal dashboard with Bubbletea
- Status, branches, remotes, graph, plugins tabs

### E2E Test Suite
- testscript scenarios for all major workflows
- Integration test coverage

## Completed Milestones

### v1.1 — TUI & E2E Tests
- Interactive TUI dashboard (`got tui`) with Bubbletea: Status, Branches, Remotes, Graph, Plugins tabs
- Vim-style keybindings (h/l tabs, j/k scroll, q quit, r refresh, ? help)
- Tokyo Night dark theme via separate `internal/tui/theme/` package
- E2E test suite using testscript: 4 scenarios (core_workflow, safe_ops, workspace_sync, onboarding)
- All 150+ unit tests pass, E2E tests pass

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
