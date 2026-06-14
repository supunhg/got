# GOT — Git-native developer operating layer

[![Go Version](https://img.shields.io/badge/Go-1.25-blue)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/got-sh/got)](https://goreportcard.com/report/github.com/got-sh/got)

GOT is a Git-native developer operating layer. It does **not** replace Git — it enhances Git with workflow abstraction, safety mechanisms, repository intelligence, team knowledge management, and interactive developer experiences.

**Git remains the source of truth. GOT metadata lives in `.got/`. Your repository stays 100% usable without GOT.**

---

## Quick Start

```bash
# Install (from source)
git clone https://github.com/got-sh/got.git
cd got
make build
./bin/got --version

# Initialize GOT in a Git repository
cd your-project
got init

# Explore
got status          # working tree status
got graph           # text-based commit graph
got branch          # list branches
got workspace list  # list workspaces
```

---

## Features

### Core Git Commands

| Command | Description |
|---------|-------------|
| `got status` | Working tree status with staged/unstaged/untracked sections, `--json` |
| `got commit` | `-m` required, `--all`, `--allow-empty`, `--auto-link` (links decisions/notes) |
| `got branch` | List (`--json`), create, delete (`-f`), checkout |
| `got graph` | Text-based commit graph with SHAs, messages, refs, merge info. `--branch`, `--max-count` |
| `got remote` | List (`--json`), add, remove, push (`-f`), pull |

### Knowledge Engine

| Command | Description |
|---------|-------------|
| `got decision` | Full ADR CRUD: create, list, show, link (`--auto`, `--branch-link`), supersede, update, delete |
| `got note` | Note CRUD with workspace/branch/commit linking |
| `got search` | Full-text search across decisions (5 fields) + notes with relevance scoring. `--type`, `--workspace`, `--json` |
| `got onboard` | Onboarding session lifecycle: start, list, progress, mark, skip, complete |

### Workspace Engine

| Command | Description |
|---------|-------------|
| `got workspace create` | Create with `--description`, `--tags`, `--create-branch` |
| `got workspace list` | Table output with status, description, tags |
| `got workspace show` | Detailed view with files, branches, decisions, notes, commits, **pull requests, issues** |
| `got workspace status` | Compact summary with counts and Git branch state |
| `got workspace add-file` | Track a file (validates existence on disk or in Git tree) |
| `got workspace add-branch` | Track a branch (validates existence in Git) |
| `got workspace add-note` | Create a note scoped to a workspace |
| `got workspace add-decision` | Link a decision to a workspace |
| `got workspace sync` | Detect stale files/branches and clean up |

### Plugin Runtime

| Command | Description |
|---------|-------------|
| `got plugin install <path>` | Install a plugin from a local directory |
| `got plugin remove <name>` | Uninstall a plugin |
| `got plugin list` | Show installed plugins, enabled/disabled status, version |
| `got plugin enable <name>` | Enable a plugin |
| `got plugin disable <name>` | Disable a plugin |
| `got plugin run <name> <action>` | Manually trigger a plugin action |

Plugins are external scripts that subscribe to events on GOT's event bus. Event data is passed as JSON via stdin. Plugins can also register CLI subcommands.

### GitHub Integration (Built-in Plugin)

| Command | Description |
|---------|-------------|
| `got github auth` | Store GitHub PAT (tries `gh auth token` first), validates via API |
| `got github pr create --title` | Create PR from current branch, auto-includes workspace refs |
| `got github pr list` | List open PRs, filter by `--branch`, `--workspace` |
| `got github pr status <number>` | Detailed PR info: mergeable, reviews, checks |
| `got github issue create --title` | Create issue with `--labels`, `--assignee`, `--workspace` |
| `got github issue list` | List open issues, filter by `--workspace` |
| `got github link <type> <id>` | Manually link workspace to PR or issue |

---

## Architecture

```
cmd/got/main.go          — Entrypoint
internal/
├── cli/                  — Cobra command tree (16 commands)
│   ├── root.go           — Root command, shared bus, plugin loading
│   ├── github.go         — GitHub integration (built-in plugin)
│   ├── plugin.go         — Plugin lifecycle commands
│   ├── plugin_runtime.go — Plugin runtime: load, execute hooks
│   ├── integration.go    — Event-driven integration layer
│   └── ... (workspace, decision, note, commit, branch, etc.)
├── git/                  — Git adapter (os/exec, not libgit2)
│   ├── adapter.go        — GitAdapter interface + ExecAdapter
│   ├── operations.go     — Status, branches, commits
│   └── remote_graph.go   — Remotes, push/pull, graph
├── store/                — SQLite storage via modernc.org/sqlite
│   ├── store.go          — Open/Close with WAL mode, migration runner
│   ├── knowledge.go      — KnowledgeStore (~1700 lines, all CRUD)
│   └── migrations/       — 8 SQL migrations (embed.FS)
├── events/               — In-memory event bus (21 event types)
│   ├── bus.go            — Thread-safe pub/sub
│   └── event.go          — Event types + typed payloads
└── version/              — Build-time version stamping + semver matching
```

### Key Design Decisions

- **Event-driven architecture** — The event bus (`internal/events`) is the backbone for all inter-module communication. Git operations publish events; the workspace engine, plugin hooks, and integration layer subscribe. The bus is shared across all CLI commands via a global instance (`globalBus`).
- **Plugin hooks via subprocess** — Plugins run as isolated subprocesses (`os/exec`). Event data is passed as JSON via stdin. A failing hook never crashes GOT.
- **SQLite with migrations** — Pure Go SQLite (`modernc.org/sqlite`), WAL mode, all migrations embedded. No CGo, no external dependencies.
- **Interface-based Git adapter** — `GitAdapter` interface in `internal/git` lets us mock Git operations in tests and could support a libgit2 backend later.

### Data Flow

```
User types command
       ↓
  Cobra command handler
       ↓
  openKnowledgeStore() → obtains shared globalBus
       ↓
  Store operations publish events (CommitCreated, WorkspaceUpdated, etc.)
       ↓
  Event bus dispatches to subscribers:
    ├── Integration layer → auto-update workspaces
    ├── Plugin hooks      → execute script with JSON stdin
    └── Event logger      → persist to event_log table
```

---

## Current Status

| Layer | Status | Details |
|-------|--------|---------|
| Git Adapter | ✅ Built | Status, commits, branches, remotes, graph — 12 tests |
| Knowledge Engine | ✅ Built | Decisions, notes, search, onboarding — 73 tests |
| Workspace Engine | ✅ Built | 12 subcommands, Git integration, PR/issue display |
| Plugin Runtime v2 | ✅ Built | Install/remove/enable/disable/run, event hooks, sample plugin |
| GitHub Integration | ✅ Built | Auth, PR/issue CRUD, workspace linking |
| TUI Framework | ❌ Not built | No `internal/tui/` directory |
| CI/CD | ❌ Not built | No `.github/workflows/` |

**Test stats:** 9 test files, ~128 tests, all pass with `go test -race ./...`

---

## Development

### Prerequisites

- Go 1.25+
- Git

### Build & Test

```bash
make build        # produces bin/got
make test         # run unit tests
make test-race    # run tests with race detector
make vet          # run go vet
make ci           # fmt-check + lint + vet + test + check-paths
```

### Quick Smoke Test

```bash
make smoke        # build + run got --help
./bin/got version # print version
```

### Adding a Migration

1. Create `internal/store/migrations/000N_description.sql`
2. Add the migration filename to `internal/store/store.go`'s migration order
3. Run `make test` to verify

---

## Documentation

| File | Content |
|------|---------|
| `ARCHITECTURE.md` | High-level architecture overview |
| `ARCHITECTURE_WORKSPACES.md` | Workspace Engine design |
| `ARCHITECTURE_GIT.md` | Git Adapter design |
| `ARCHITECTURE_INTEGRATION.md` | Event-driven integration layer |
| `ARCHITECTURE_PLUGINS.md` | Plugin Runtime v2 design |
| `ARCHITECTURE_GITHUB.md` | GitHub integration design |
| `got-spec.md` | Original v0.1 spec (outdated) |
| `TEMP.md` | Full implementation status report |

---

## Project Structure

```
got/
├── cmd/got/main.go          # Entrypoint
├── internal/                # All Go packages
│   ├── cli/                 # CLI commands (Cobra)
│   ├── git/                 # Git adapter
│   ├── store/               # SQLite storage
│   ├── events/              # Event bus
│   └── version/             # Version stamping
├── testdata/
│   └── hello-plugin/        # Sample plugin
├── Makefile
├── go.mod / go.sum
├── *.md                     # Documentation
└── LICENSE                  # MIT
```

---

## License

MIT — see [LICENSE](LICENSE).

---

*GOT is a Git-native developer operating layer. Git remains the source of truth. Offline-first. Plugin-first.*
