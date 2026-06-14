# GOT — Git-Native Developer Operating Layer

[![Go Version](https://img.shields.io/badge/Go-1.25-blue)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![CI](https://github.com/supunhg/got/actions/workflows/ci.yml/badge.svg)](https://github.com/supunhg/got/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/supunhg/got)](https://goreportcard.com/report/github.com/supunhg/got)

**GOT enhances Git with workspace management, decision tracking, team knowledge, and GitHub integration — without replacing Git.**

Your repository stays 100% usable without GOT. All metadata lives in `.got/`. Git remains the source of truth.

---

## Quick Start

### Install

```bash
# With Go
go install github.com/supunhg/got/cmd/got@latest

# From source
git clone https://github.com/supunhg/got.git
cd got
make build
# binary is at ./bin/got
```

### Get Started

```bash
# Initialize GOT in any Git repository
cd your-project
got init

# Core workflow
got status                              # working tree overview
got commit -m "feat: add auth module"   # commit with validation
got workspace create oauth              # organize work by context
got decision create "Use JWT tokens"    # record architectural decisions
```

---

## Why GOT?

Git is excellent at tracking file changes. But day-to-day development involves much more: tracking *why* decisions were made, managing context across branches, onboarding teammates, and coordinating with GitHub.

GOT adds these capabilities as a thin layer on top of Git:

| With Git alone | With GOT |
|---|---|
| `git log` to see what changed | `got workspace show` to see everything about a feature: files, branches, decisions, PRs |
| Scattered notes about architecture decisions | `got decision create` for structured, searchable ADRs |
| "Read the code" onboarding | `got onboard start` with guided, skippable steps |
| Switch between `git` and `gh` CLI | `got github pr create` links PRs to workspaces automatically |
| Hope nobody force-pushes | Safe operations with automatic recovery snapshots |

---

## Features

### Workspace Engine

Group related files, branches, decisions, and notes into logical contexts.

```bash
got workspace create oauth --description "OAuth 2.0 implementation" --tags auth,security
got workspace add-file oauth src/auth/oauth.go
got workspace add-branch oauth feat/oauth2-refresh
got workspace show oauth          # see everything in one place
got workspace sync oauth          # detect stale files/branches
```

### Knowledge Engine

Searchable architectural decisions, notes, and onboarding guides.

```bash
got decision create "Use PostgreSQL for audit log" --status accepted
got decision list
got decision link <id> --auto     # link to current commit
got note add "Need to review rate limiting before launch"
got search "oauth" --type decision
```

### Onboarding

```bash
got onboard start                 # begin guided onboarding session
got onboard progress              # see what's done, what's next
got onboard mark step-3           # mark a step complete
```

### GitHub Integration

Manage PRs and issues without leaving your terminal. Automatically links to workspaces.

```bash
got github auth                   # authenticate (uses gh CLI token if available)
got github pr create --title "Add OAuth support"
got github pr status 42           # reviews, merge status, checks
got github pr merge 42 --method squash --delete-branch
got github issue list --workspace oauth
```

### Core Git Commands (Enhanced)

```bash
got graph                         # text-based commit graph
got branch                        # list with upstream info
got remote list                   # remotes with push/pull
got status --json                 # machine-readable output
```

### Plugin System

Extend GOT with external scripts that subscribe to events.

```bash
got plugin install ./my-plugin    # install from local directory
got plugin list                   # show installed plugins
got plugin run hello-world greet  # run a plugin command
```

Plugins subscribe to events like `CommitCreated`, `WorkspaceUpdated`, and run as isolated subprocesses.

---

## Philosophy

1. **Git is the source of truth.** GOT never modifies Git in ways you didn't ask for.
2. **Metadata is isolated.** Everything lives in `.got/`. Add it to `.gitignore` and Git stays clean.
3. **Offline-first.** No network calls except those you initiate (`git push`, `got github`).
4. **Plugin-first.** Core features use the same event bus and plugin API available to extensions.
5. **Recoverable.** Destructive operations create automatic snapshots for safety.

---

## Architecture

```
cmd/got/                  Entrypoint
internal/
├── cli/                  Cobra command tree (16+ commands)
├── git/                  Git adapter (os/exec, no libgit2)
├── store/                SQLite storage (modernc.org/sqlite, no CGo)
├── events/               In-memory event bus (21 event types)
└── version/              Build-time version stamping
```

**Key design decisions:**

- **Event-driven architecture** — the event bus connects all modules. Git operations publish events; workspace engine, plugins, and integration layer subscribe.
- **Plugin hooks via subprocess** — plugins run as isolated processes with JSON over stdin/stdout. A failing plugin never crashes GOT.
- **SQLite with migrations** — pure Go, WAL mode, zero external dependencies.
- **Interface-based Git adapter** — mockable in tests, swappable to libgit2 later.

---

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for development setup, code conventions, and how to submit changes.

See [`ROADMAP.md`](ROADMAP.md) for the project roadmap.

---

## Development

### Prerequisites

- Go 1.25+
- Git
- (Optional) [golangci-lint](https://golangci-lint.run/) for linting

### Build & Test

```bash
make build          # compile to bin/got
make test           # run unit tests
make test-race      # tests with race detector
make ci             # full CI gate: fmt-check + lint + vet + test + check-paths
```

### Shell Completions

```bash
got completion bash > /etc/bash_completion.d/got
got completion zsh > "${fpath[1]}/_got"
got completion fish > ~/.config/fish/completions/got.fish
got completion powershell > got.ps1
```

---

## License

MIT — see [LICENSE](LICENSE).

---

*GOT is a Git-native developer operating layer. Git remains the source of truth. Offline-first. Plugin-first.*
