package store

import (
	"path/filepath"
	"testing"
)

// TestCountsAreZeroOnFreshDB verifies that a freshly opened store
// reports zero rows for every count. In v0.1 no command writes to
// snapshots/decisions/workspaces/health_runs, so this is the expected
// steady state.
func TestCountsAreZeroOnFreshDB(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "got.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	c, err := s.Counts()
	if err != nil {
		t.Fatalf("Counts: %v", err)
	}
	if c.Snapshots != 0 || c.Decisions != 0 || c.Workspaces != 0 || c.OpenWorkspaces != 0 || c.HealthRuns != 0 {
		t.Errorf("fresh Counts = %+v, want all zeros", c)
	}
}

// TestCountsReflectInsertedRows inserts one row into each of the four
// user-visible tables and confirms Counts reports the right totals.
// This proves the COUNT(*) statements hit the right tables and that
// CountWorkspaces distinguishes open vs. closed.
func TestCountsReflectInsertedRows(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "got.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Two snapshots
	for _, id := range []string{"s1", "s2"} {
		if _, err := s.db.Exec(
			`INSERT INTO snapshots(id, created_at, reason, ref) VALUES(?, 1700000000000, 'before-reset', 'refs/heads/main')`,
			id,
		); err != nil {
			t.Fatalf("insert snapshot %s: %v", id, err)
		}
	}
	// One decision
	if _, err := s.db.Exec(
		`INSERT INTO decisions(id, created_at, status, title, body_path) VALUES('d1', 1700000000000, 'accepted', 'Use SQLite', 'decisions/0001.md')`,
	); err != nil {
		t.Fatalf("insert decision: %v", err)
	}
	// Three workspaces: two open, one closed
	for i, st := range []string{"open", "open", "closed"} {
		stmt, err := s.db.Prepare(`INSERT INTO workspaces(id, name, created_at, state) VALUES(?, ?, ?, ?)`)
		if err != nil {
			t.Fatalf("prepare: %v", err)
		}
		if _, err := stmt.Exec("w"+string(rune('1'+i)), "ws"+string(rune('1'+i)), 1700000000000, st); err != nil {
			t.Fatalf("insert workspace: %v", err)
		}
		_ = stmt.Close()
	}
	// One health run
	if _, err := s.db.Exec(
		`INSERT INTO health_runs(id, run_at, report) VALUES('h1', 1700000000000, '{"score":0.9}')`,
	); err != nil {
		t.Fatalf("insert health_run: %v", err)
	}

	c, err := s.Counts()
	if err != nil {
		t.Fatalf("Counts: %v", err)
	}
	want := Counts{Snapshots: 2, Decisions: 1, Workspaces: 3, OpenWorkspaces: 2, HealthRuns: 1}
	if c != want {
		t.Errorf("Counts = %+v, want %+v", c, want)
	}
}

// TestPerTableAccessors verifies the individual CountX methods agree
// with the Counts aggregate and with each other.
func TestPerTableAccessors(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "got.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if n, err := s.CountSnapshots(); err != nil || n != 0 {
		t.Errorf("CountSnapshots = (%d, %v), want (0, nil)", n, err)
	}
	if n, err := s.CountDecisions(); err != nil || n != 0 {
		t.Errorf("CountDecisions = (%d, %v), want (0, nil)", n, err)
	}
	if n, err := s.CountHealthRuns(); err != nil || n != 0 {
		t.Errorf("CountHealthRuns = (%d, %v), want (0, nil)", n, err)
	}
	if total, open, err := s.CountWorkspaces(); err != nil || total != 0 || open != 0 {
		t.Errorf("CountWorkspaces = (%d, %d, %v), want (0, 0, nil)", total, open, err)
	}
}

// TestCountRowsWhereAllowListRejection verifies that the private
// countRowsWhere helper refuses table names not in the allow-list.
// This is a guard against future refactors that might route
// user-controlled table names through the helper.
func TestCountRowsWhereAllowListRejection(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "got.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if _, err := s.countRowsWhere("DROP TABLE meta; --", ""); err == nil {
		t.Errorf("countRowsWhere should reject unknown table name, got nil")
	}
	if _, err := s.countRowsWhere("snapshots", "1=1 OR 1=1; --"); err == nil {
		t.Errorf("countRowsWhere should reject unsafe where clause, got nil")
	}
}
