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

## v1.0

* Stable plugin API
* Advanced TUI
* Cross-platform distribution
* Production-ready release
