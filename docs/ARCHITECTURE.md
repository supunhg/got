# GOT Architecture

## Vision

GOT is a Git-native developer operating layer.

GOT does not replace Git.

GOT enhances Git through:

* Workflow abstraction
* Safety mechanisms
* Repository intelligence
* Team knowledge management
* Interactive developer experiences

Git remains the source of truth.

GOT maintains 100% Git compatibility.

---

# High-Level Architecture

┌───────────────────────────┐
│         GOT CLI           │
└─────────────┬─────────────┘
│
┌─────────────▼─────────────┐
│      Command Router       │
└─────────────┬─────────────┘
│
┌────────────┼────────────┐
│            │            │
▼            ▼            ▼

Git Layer   Metadata     TUI Layer
Layer

│            │            │

▼            ▼            ▼

Git Repo   .got/       Interactive
Storage     Workflows

---

# Core Principles

1. Git is never modified.

2. GOT metadata is isolated.

3. Repository remains usable without GOT.

4. Every operation must be recoverable.

5. Offline-first.

6. Plugin-first architecture.

---

# Repository Structure

.git/

.got/
├── config.yaml
├── snapshots/
├── workspaces/
├── decisions/
├── health/
├── cache/
└── plugins/

got.yml

---

# Modules

## Git Adapter

Responsibilities:

* Execute Git operations
* Read repository state
* Parse refs
* Parse commits
* Parse branches
* Parse remotes

Interface:

GitAdapter

Methods:

* Status()
* Commit()
* Branches()
* Remotes()
* Checkout()
* Merge()
* Reset()
* Fetch()
* Push()

---

## Repository Analyzer

Responsibilities:

* Detect repository structure
* Detect languages
* Detect frameworks
* Build repository graph
* Compute statistics

Outputs:

RepositoryModel

---

## Workspace Engine

Provides logical work tracking.

Example:

Workspace:
OAuth Authentication

Tracks:

* related files
* branches
* commits
* decisions

Independent from Git branches.

---

## Snapshot Engine

Creates recovery points.

Operations protected:

* reset
* clean
* rebase
* force push

Snapshots stored in:

.got/snapshots

Supports rollback.

---

## Knowledge Engine

Stores:

* ADRs
* notes
* architecture references
* onboarding guides

Integrated into CLI.

Commands:

got decision
got notes
got onboard

---

## Health Engine

Analyzes:

* stale branches
* abandoned remotes
* file churn
* commit frequency
* ownership patterns

Outputs repository diagnostics.

---

## Graph Engine

Builds:

* commit graph
* branch graph
* workspace graph

Provides data for:

got graph

---

## TUI Framework

Built with Bubble Tea.

Views:

* Dashboard
* Branch Explorer
* Remote Manager
* Commit Wizard
* Repository Health
* Workspace Manager

---

## Plugin System

Plugin API:

type Plugin interface {
Name() string
Version() string
Register(router Router)
}

Plugins loaded from:

.got/plugins

Future capabilities:

* GitHub integration
* GitLab integration
* CI integrations
* Deployment workflows

---

# Development Roadmap

## Integration Layer

The Integration Layer (`internal/cli/integration.go`) ties the Git Adapter,
Workspace Engine, Knowledge Engine, and Event Bus together through
event-driven automatic actions.

### Event-driven flows

* `CommitCreated` → auto-update workspace `last_commit_sha` and
  `workspace_commits` for any workspace tracking the commit's branch
* `got commit --auto-link` → link unlinked decisions/notes to new commit
* `got decision link --auto` → resolve HEAD and link to most recent commit

### Git-aware workspaces

* `got workspace add-file` validates file exists (on disk or Git tree)
* `got workspace add-branch` validates branch exists in Git
* `got workspace show` resolves live branch info (exists, ahead/behind,
  latest commit)
* `got workspace sync` detects stale files/branches and cleans up
* `got workspace create --create-branch` creates a Git branch

### Data model (migration 0006)

* `workspace_commits` table linking workspace_id → commit_sha
* `last_commit_sha` column on workspaces for fast access

See `ARCHITECTURE_INTEGRATION.md` for full details.

## v0.1

* Git adapter
* Init wizard
* Commit wizard
* Branch graph
* Remote manager

## v0.2

* Snapshot engine
* Safe reset
* Safe rebase
* Recovery manager

## v0.3

* Health engine
* Repository analyzer
* Repository dashboard

## v0.4

* Workspaces
* Knowledge engine
* ADR support

## v0.5

* Git adapter + core Git commands
* Workspace ↔ Git integration
* Event-driven integration layer
* Decision auto-link and commit linking

## v1.0

* Stable plugin API
* Advanced TUI
* Cross-platform distribution
* Production-ready release
