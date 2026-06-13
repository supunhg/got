package store

import (
	"database/sql"
	"fmt"
)

// Counts is an aggregate snapshot of the row counts across the
// user-visible tables in the GOT metadata DB. It is used by `got status`
// to render a one-line summary; the per-table accessors below are
// exposed so other commands (e.g. future `got doctor`) can use the same
// primitives.
//
// Counts of zero are normal in v0.1: snapshots, decisions, workspaces,
// and health_runs are forward-compat tables that no v0.1 command writes
// to. A non-zero count for one of those tables means the user has been
// poking at the DB directly or is running a development build that
// writes to them.
//
// The Workspace* fields reflect the v0.4 Workspace Engine (added in
// migration 0002). The legacy Workspaces/OpenWorkspaces pair counts the
// parent `workspaces` table rows; the per-child-table counts
// (WorkspaceFiles, WorkspaceBranches, WorkspaceDecisions,
// WorkspaceNotes) are summed across all workspaces and are useful
// for the `got status` one-liner.
type Counts struct {
	Snapshots          int `json:"snapshots"`
	Decisions          int `json:"decisions"`
	Workspaces         int `json:"workspaces"`
	OpenWorkspaces     int `json:"openWorkspaces"`
	WorkspaceFiles     int `json:"workspaceFiles"`
	WorkspaceBranches  int `json:"workspaceBranches"`
	WorkspaceDecisions int `json:"workspaceDecisions"`
	WorkspaceNotes     int `json:"workspaceNotes"`
	HealthRuns         int `json:"healthRuns"`
	Events             int `json:"events"`
	WorkspaceCommits   int `json:"workspaceCommits"`
}

// Counts returns a single snapshot of all row counts. It runs the
// COUNT(*) queries in sequence on a single connection — cheap because
// v0.1's tables are tiny — and returns an aggregate. The return value
// is suitable for direct inclusion in `got status --json` output.
func (s *Store) Counts() (Counts, error) {
	c := Counts{}
	var err error
	if c.Snapshots, err = s.CountSnapshots(); err != nil {
		return c, err
	}
	if c.Decisions, err = s.CountDecisions(); err != nil {
		return c, err
	}
	if c.Workspaces, c.OpenWorkspaces, err = s.CountWorkspaces(); err != nil {
		return c, err
	}
	if c.WorkspaceFiles, err = s.CountWorkspaceFiles(); err != nil {
		return c, err
	}
	if c.WorkspaceBranches, err = s.CountWorkspaceBranches(); err != nil {
		return c, err
	}
	if c.WorkspaceDecisions, err = s.CountWorkspaceDecisions(); err != nil {
		return c, err
	}
	if c.WorkspaceNotes, err = s.CountWorkspaceNotes(); err != nil {
		return c, err
	}
	if c.HealthRuns, err = s.CountHealthRuns(); err != nil {
		return c, err
	}
	if c.Events, err = s.CountEvents(); err != nil {
		return c, err
	}
	if c.WorkspaceCommits, err = s.CountWorkspaceCommits(); err != nil {
		return c, err
	}
	return c, nil
}

// CountSnapshots returns the number of rows in the snapshots table.
// Snapshots are reserved for v0.2; in v0.1 the count is always zero
// unless the user has been poking at the DB.
func (s *Store) CountSnapshots() (int, error) {
	return s.countRows("snapshots")
}

// CountDecisions returns the number of rows in the decisions table.
// ADRs land in v0.4; the count is always zero in v0.1.
func (s *Store) CountDecisions() (int, error) {
	return s.countRows("decisions")
}

// CountWorkspaces returns the total number of workspace rows and the
// number whose state is 'open'. Workspaces land in v0.4; both counts
// are zero in v0.1.
func (s *Store) CountWorkspaces() (total, open int, err error) {
	open, err = s.countRowsWhere("workspaces", "state = 'open'")
	if err != nil {
		return 0, 0, err
	}
	total, err = s.countRows("workspaces")
	if err != nil {
		return 0, 0, err
	}
	return total, open, nil
}

// CountHealthRuns returns the number of rows in the health_runs table.
// The health engine lands in v0.3; the count is always zero in v0.1.
func (s *Store) CountHealthRuns() (int, error) {
	return s.countRows("health_runs")
}

// CountWorkspaceFiles returns the total number of workspace_files rows
// across all workspaces. Added in migration 0002 (v0.4 Workspace
// Engine). The per-workspace count is exposed via
// internal/workspace.Store.ListFiles.
func (s *Store) CountWorkspaceFiles() (int, error) {
	return s.countRows("workspace_files")
}

// CountWorkspaceBranches returns the total number of workspace_branches
// rows. Added in migration 0002.
func (s *Store) CountWorkspaceBranches() (int, error) {
	return s.countRows("workspace_branches")
}

// CountWorkspaceDecisions returns the total number of
// workspace_decisions rows. Added in migration 0002. This is
// intentionally separate from CountDecisions (the global ADR table
// from got-spec.md §12) so callers can distinguish per-workspace
// decisions from repo-wide ADRs.
func (s *Store) CountWorkspaceDecisions() (int, error) {
	return s.countRows("workspace_decisions")
}

// CountWorkspaceNotes returns the total number of workspace_notes rows.
// Added in migration 0002.
func (s *Store) CountWorkspaceNotes() (int, error) {
	return s.countRows("workspace_notes")
}

// CountEvents returns the total number of events rows. Added in
// migration 0003 (v0.4 Event Bus). The events table is the durable
// replay log for the internal/eventbus package; `got status` shows
// this count so users can see when the log is non-empty.
func (s *Store) CountEvents() (int, error) {
	return s.countRows("events")
}

// CountWorkspaceCommits returns the total number of workspace_commits
// rows. Added in migration 0004 (v0.5 Workspace Engine). Pinned
// commits per workspace; surfaced by `got status` so users can see
// when any workspace has been pinned to a commit.
func (s *Store) CountWorkspaceCommits() (int, error) {
	return s.countRows("workspace_commits")
}

// countRows returns COUNT(*) from the named table. The table name is
// taken from a hard-coded set of call sites (CountSnapshots,
// CountDecisions, CountHealthRuns) so there is no SQL-injection risk.
// We keep the helper private; callers must go through the typed
// CountX accessors above.
func (s *Store) countRows(table string) (int, error) {
	return s.countRowsWhere(table, "")
}

// countRowsWhere returns COUNT(*) from the named table, optionally
// filtered by a where clause supplied by the caller. The where clause
// is hard-coded in the typed CountX accessors above; the table name is
// also from a fixed set. The two strings are concatenated into a
// fmt.Sprintf format string so a malicious table name cannot smuggle
// SQL past it: the table name is validated against an allow-list in
// the caller.
func (s *Store) countRowsWhere(table, where string) (int, error) {
	if !isAllowedCountTable(table) {
		return 0, fmt.Errorf("store.countRowsWhere: table %q is not in the allow-list", table)
	}
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	if where != "" {
		// where clauses come from our own hard-coded call sites, never
		// from user input. Validate they only contain safe characters
		// (letters, digits, spaces, =, ', _) just in case.
		if !isSafeWhere(where) {
			return 0, fmt.Errorf("store.countRowsWhere: unsafe where clause %q", where)
		}
		q += " WHERE " + where
	}
	var n int
	if err := s.db.QueryRow(q).Scan(&n); err != nil {
		return 0, wrapCountErr(table, err)
	}
	return n, nil
}

// allowedCountTables is the set of table names that countRowsWhere
// will accept. Keeping this list explicit prevents accidental or
// malicious use of the helper against arbitrary tables.
//
// Threat model: this is a defense-in-depth check, not a security
// boundary. The call sites (CountSnapshots, CountDecisions, etc.) all
// pass hard-coded string literals; the helper is not exported and
// never receives user input. The allow-list is here so a future
// refactor that wires the helper up to (say) a config-driven table
// name fails loudly instead of silently exposing every table in the
// schema to a possibly-malformed input.
var allowedCountTables = map[string]bool{
	"snapshots":            true,
	"decisions":            true,
	"workspaces":           true,
	"workspace_files":      true,
	"workspace_branches":   true,
	"workspace_decisions":  true,
	"workspace_notes":      true,
	"health_runs":          true,
	"events":               true,
	"cache_kv":             true,
	"meta":                 true,
}

// isAllowedCountTable reports whether name is in allowedCountTables.
func isAllowedCountTable(name string) bool {
	return allowedCountTables[name]
}

// isSafeWhere does a very small sanity check on hard-coded where
// clauses: letters, digits, spaces, =, ', _, and . (for column
// qualifiers). Anything else is rejected.
func isSafeWhere(s string) bool {
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == ' ' || c == '=' || c == '\'' || c == '_' || c == '.' || c == '-':
		default:
			return false
		}
	}
	return true
}

// wrapCountErr wraps a count-query error with a table-qualified
// message. We don't use gerr.Wrap here to keep this file dependency-
// light; the callers (Counts, CountX) are wrapped by the CLI layer
// when they fail.
func wrapCountErr(table string, err error) error {
	if err == sql.ErrNoRows {
		// COUNT(*) never returns ErrNoRows in practice, but guard anyway.
		return nil
	}
	return fmt.Errorf("count %s: %w", table, err)
}
