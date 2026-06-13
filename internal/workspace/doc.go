// Package workspace implements the v0.4 Workspace Engine: a logical
// grouping of repository knowledge that lives in .got/got.db (the same
// SQLite store the rest of GOT uses) and is fully independent of
// Git worktrees and Git branches.
//
// A Workspace is a named aggregate that groups together:
//
//   - files (repo-relative paths) the user wants to keep an eye on
//   - branches (logical refs) the workspace is being developed on
//   - decisions (lightweight ADRs scoped to this workspace)
//   - notes (free-form markdown the user attaches to the workspace)
//
// The engine is offline-only (no network calls), plugin-compatible
// (any external plugin can read the JSON output of `got workspace
// list --json` / `got workspace show --json` or shell out to the CLI),
// and 100% testable (the Store takes a *store.Store and uses its
// *sql.DB handle, so tests can use a tempdir-backed DB with no
// network, no TTY, and no real Git repository).
//
// The package is intentionally small: 5 entity types (Workspace,
// WorkspaceFile, WorkspaceBranch, WorkspaceDecision, WorkspaceNote)
// and one Store that owns CRUD for all of them. The CLI surface
// (`internal/cli/workspace.go`) is the primary user-facing layer;
// the package API is the seam tests and plugins use.
//
// # Schema
//
// The schema is defined in internal/store/migrations/0002_workspaces.sql
// and includes ON DELETE CASCADE from every child table to
// workspaces, so deleting a workspace is a single transaction that
// removes files, branches, decisions, and notes atomically.
//
// # Independence from worktrees and branches
//
// A workspace is identified by a stable slug (`name`) and an opaque
// ID; it does not reference any Git-level worktree or branch. The
// `workspace_branches` table records *which* branches the user has
// tagged as relevant to a given workspace, but the workspace does
// not own or modify those branches. Deleting a workspace leaves
// the branches alone; the next `git branch` shows them unchanged.
package workspace
