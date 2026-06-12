package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestOpenCreatesDBAndRunsMigrations verifies that Open creates the
// .db file, applies the embedded migrations, and records the right
// schema_version.
func TestOpenCreatesDBAndRunsMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "got.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected db file at %q: %v", dbPath, err)
	}

	got, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if got != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", got, SchemaVersion)
	}

	// Every table from §12 should exist. We probe sqlite_master.
	expected := []string{"meta", "snapshots", "decisions", "workspaces", "workspace_files", "health_runs", "cache_kv"}
	for _, table := range expected {
		var name string
		err := s.db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing: %v", table, err)
		}
	}
}

// TestMigrateIsIdempotent verifies that opening the same DB file twice
// in a row does not re-apply the migrations or change schema_version.
func TestMigrateIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "got.db")

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open #1: %v", err)
	}
	first, err := s1.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion #1: %v", err)
	}
	// Insert a marker row that should survive a second open.
	if err := s1.MetaSet("marker", "hello"); err != nil {
		t.Fatalf("MetaSet: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close #1: %v", err)
	}

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open #2: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })
	second, err := s2.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion #2: %v", err)
	}
	if first != second {
		t.Errorf("schema_version changed: %d -> %d", first, second)
	}
	v, err := s2.MetaGet("marker")
	if err != nil {
		t.Fatalf("MetaGet marker: %v", err)
	}
	if v != "hello" {
		t.Errorf("marker = %q, want %q", v, "hello")
	}
}

// TestMetaRoundtrip exercises MetaSet, MetaGet, MetaDelete, and the
// "absent" path for MetaGet.
func TestMetaRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "got.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if v, err := s.MetaGet("absent"); err != nil || v != "" {
		t.Errorf("MetaGet absent = (%q, %v), want (\"\", nil)", v, err)
	}

	if err := s.MetaSet("k", "v1"); err != nil {
		t.Fatalf("MetaSet k=v1: %v", err)
	}
	if v, err := s.MetaGet("k"); err != nil || v != "v1" {
		t.Errorf("MetaGet k = (%q, %v), want (\"v1\", nil)", v, err)
	}

	// Upsert: setting the same key overwrites the value.
	if err := s.MetaSet("k", "v2"); err != nil {
		t.Fatalf("MetaSet k=v2: %v", err)
	}
	if v, _ := s.MetaGet("k"); v != "v2" {
		t.Errorf("after upsert MetaGet k = %q, want \"v2\"", v)
	}

	if err := s.MetaDelete("k"); err != nil {
		t.Fatalf("MetaDelete: %v", err)
	}
	if v, _ := s.MetaGet("k"); v != "" {
		t.Errorf("after delete MetaGet k = %q, want \"\"", v)
	}
	// Delete of missing key is a no-op, not an error.
	if err := s.MetaDelete("nope"); err != nil {
		t.Errorf("MetaDelete missing: %v", err)
	}
}

// TestTouchInitMetaIsIdempotent verifies the "don't bump init_at on
// re-open" semantics.
func TestTouchInitMetaIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "got.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	at1 := time.Unix(1_700_000_000, 0).UTC()
	if err := s.TouchInitMeta("0.1.0", "alice", at1, false); err != nil {
		t.Fatalf("TouchInitMeta #1: %v", err)
	}
	if v, _ := s.MetaGet("init_at"); v != "1700000000000" {
		t.Errorf("init_at = %q, want 1700000000000", v)
	}

	// Second call without force should leave init_at alone.
	at2 := time.Unix(1_800_000_000, 0).UTC()
	if err := s.TouchInitMeta("0.2.0", "bob", at2, false); err != nil {
		t.Fatalf("TouchInitMeta #2: %v", err)
	}
	if v, _ := s.MetaGet("init_at"); v != "1700000000000" {
		t.Errorf("init_at bumped on second call: %q", v)
	}
	if v, _ := s.MetaGet("got_version"); v != "0.1.0" {
		t.Errorf("got_version changed: %q", v)
	}
	if v, _ := s.MetaGet("init_user"); v != "alice" {
		t.Errorf("init_user changed: %q", v)
	}

	// force=true overwrites everything.
	if err := s.TouchInitMeta("0.2.0", "bob", at2, true); err != nil {
		t.Fatalf("TouchInitMeta #3: %v", err)
	}
	if v, _ := s.MetaGet("init_at"); v != "1800000000000" {
		t.Errorf("force did not overwrite init_at: %q", v)
	}
	if v, _ := s.MetaGet("init_user"); v != "bob" {
		t.Errorf("force did not overwrite init_user: %q", v)
	}
}

// TestSnapshotFKOnDeleteCascade verifies the workspace_files FK and the
// ON DELETE CASCADE behavior wired in by the foreign_keys pragma. This
// also indirectly proves the pragma is applied at Open time.
func TestSnapshotFKOnDeleteCascade(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "got.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// workspaces is "used in v0.4" per §12, but its schema is created
	// now, so we can exercise the FK. Insert a workspace and a file.
	if _, err := s.db.Exec(
		`INSERT INTO workspaces(id, name, created_at, state) VALUES('w1', 'demo', 1700000000000, 'open')`,
	); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	if _, err := s.db.Exec(
		`INSERT INTO workspace_files(workspace_id, path) VALUES('w1', 'a/b/c.go')`,
	); err != nil {
		t.Fatalf("insert workspace_file: %v", err)
	}
	// Deleting the workspace should cascade and remove the file row.
	if _, err := s.db.Exec(`DELETE FROM workspaces WHERE id = 'w1'`); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM workspace_files WHERE workspace_id='w1'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("cascade did not remove file row: count=%d", n)
	}
}

// TestCloseNilSafe verifies Close is safe to call on a Store whose
// internal db is already closed. Cobra shutdown paths sometimes double-
// close.
func TestCloseNilSafe(t *testing.T) {
	s := &Store{}
	if err := s.Close(); err != nil {
		t.Errorf("Close on empty store: %v", err)
	}
}

// TestMigrationVersionParser checks the helper that parses
// "NNNN_foo.sql" filenames.
func TestMigrationVersionParser(t *testing.T) {
	cases := map[string]struct {
		in      string
		want    int
		wantErr bool
	}{
		"leading zeros": {"0001_init.sql", 1, false},
		"double digit":  {"0042_something.sql", 42, false},
		"no underscore": {"123.sql", 0, true},
		"non-digit":     {"abcd_init.sql", 0, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := migrationVersion(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Errorf("migrationVersion(%q) = %d, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("migrationVersion(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("migrationVersion(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// TestMigrateBodyContainsCreateTable does a sanity check that the
// embedded migration actually has the SQL we expect, so a missing
// file in the embed would fail loudly here.
func TestMigrateBodyContainsCreateTable(t *testing.T) {
	body, err := fsReadFile("migrations/0001_init.sql")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS meta",
		"CREATE TABLE IF NOT EXISTS snapshots",
		"CREATE TABLE IF NOT EXISTS decisions",
		"CREATE TABLE IF NOT EXISTS workspaces",
		"CREATE TABLE IF NOT EXISTS workspace_files",
		"CREATE TABLE IF NOT EXISTS health_runs",
		"CREATE TABLE IF NOT EXISTS cache_kv",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("migration missing %q", want)
		}
	}
}
