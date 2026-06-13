// Package store provides the SQLite-backed metadata store for GOT. The
// store lives at .got/got.db in the user's repository, runs in WAL mode
// with synchronous=NORMAL (per got-spec.md §12), and is created by
// `got init` (step 3 of got-spec.md §24). The schema is forward-compat:
// tables that v0.1 doesn't fully use are still created so v0.2+ can
// start writing to them without schema migrations.
//
// Migrations are embedded at compile time and applied in lexicographic
// order on Open. The migration runner is small enough that pulling in
// github.com/golang-migrate/migrate/v4 would be overkill, and rolling
// our own keeps the dependency surface CGo-free (modernc.org/sqlite is
// the only driver).
package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver; registers itself as "sqlite"

	"github.com/got-sh/got/internal/gerr"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// SchemaVersion is the highest migration version this binary knows about.
// It is updated whenever a new migration file is added to migrations/.
// The migration runner compares this value to the schema_version row in
// the meta table to decide what to apply.
//
// Bump history:
//   - 1: initial schema (got-spec.md §12)
//   - 2: Workspace Engine (ARCHITECTURE.md "Workspace Engine"):
//     drops the v0.1 stubs `workspaces` + `workspace_files` and
//     recreates them with the v0.4 shape; adds `workspace_branches`,
//     `workspace_decisions`, and `workspace_notes`. See
//     migrations/0002_workspaces.sql for the full diff.
//   - 3: Event Bus (docs/EVENT_BUS.md): adds the `events` table as
//     the durable replay log for the internal/eventbus package.
//     See migrations/0003_events.sql.
//   - 4: Workspace Engine v0.5 (ARCHITECTURE_WORKSPACES.md):
//     adds the `workspace_commits` table so a workspace can
//     pin a Git commit (independently of branches/files).
//     See migrations/0004_workspace_commits.sql.
const SchemaVersion = 4

// Store is an open SQLite database with migrations applied. Always
// close it with Close when done.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path, applies any
// pending migrations, and primes the meta table with schema_version and
// got_version. The returned Store must be closed with Close.
//
// The driver is configured for WAL mode and synchronous=NORMAL per
// got-spec.md §12. Foreign keys are enabled so the workspace_files FK
// actually fires. A short busy timeout keeps concurrent writes from
// failing on contention.
func Open(path string) (*Store, error) {
	// modernc.org/sqlite supports ?_pragma=... query params on the
	// DSN. journal_mode=WAL and synchronous=NORMAL are the spec's
	// recommended settings. busy_timeout is in milliseconds.
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("opening sqlite database at %q", path))
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("pinging sqlite database at %q", path))
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database handle. Safe to call multiple times; only
// the first call closes the underlying *sql.DB.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

// DB returns the underlying *sql.DB for callers that need to run their
// own queries (e.g. tests, future snapshot/decision/health modules).
// Returns nil after Close.
func (s *Store) DB() *sql.DB { return s.db }

// migrate runs any pending migrations in lexicographic order. It is
// idempotent: running it twice in a row is a no-op. The schema_version
// row in the meta table is the source of truth for what's been applied.
//
// Each migration runs inside a single transaction; on any error the
// transaction is rolled back and the function returns it. The meta
// table itself is created in a bootstrap step so it exists even on a
// fresh database before the first migration runs.
func (s *Store) migrate() error {
	// Bootstrap: ensure meta exists and read the current version.
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`); err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, "creating meta table")
	}
	current, err := s.metaInt("schema_version")
	if err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, "reading embedded migrations")
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		ver, err := migrationVersion(name)
		if err != nil {
			return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("parsing migration name %q", name))
		}
		if ver <= current {
			continue
		}
		body, err := fs.ReadFile(migrationsFS, "migrations/"+name)
		if err != nil {
			return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("reading migration %q", name))
		}
		if err := s.applyMigration(ver, string(body)); err != nil {
			return err
		}
	}
	return nil
}

// applyMigration runs one migration file in a transaction. golang-migrate
// convention is to split on the literal "-- +migrate StatementBegin" /
// "-- +migrate StatementEnd" markers; we don't have those, so we just
// run the whole body as a single Exec. For v0.1's single migration that
// is sufficient. If we add a migration with multiple statements later we
// can introduce the split.
func (s *Store) applyMigration(version int, body string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("begin tx for migration %d", version))
	}
	if _, err := tx.Exec(body); err != nil {
		_ = tx.Rollback()
		return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("applying migration %d", version))
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO meta(key, value) VALUES('schema_version', ?)`, fmt.Sprintf("%d", version)); err != nil {
		_ = tx.Rollback()
		return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("recording schema_version %d", version))
	}
	if err := tx.Commit(); err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("commit migration %d", version))
	}
	return nil
}

// migrationVersion parses a migration filename like "0007_foo.sql" and
// returns 7.
func migrationVersion(name string) (int, error) {
	idx := strings.IndexByte(name, '_')
	if idx <= 0 {
		return 0, fmt.Errorf("migration filename %q missing NNNN_ prefix", name)
	}
	n := 0
	for _, c := range name[:idx] {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("migration filename %q has non-digit prefix", name)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// MetaGet returns the value for key, or ("", nil) if the key is absent.
// Returns the error only for underlying database failures.
func (s *Store) MetaGet(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("meta get %q", key))
	}
	return v, nil
}

// MetaSet upserts the (key, value) pair.
func (s *Store) MetaSet(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO meta(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("meta set %q", key))
	}
	return nil
}

// MetaDelete removes the (key, value) pair. Returns nil even if the key
// is absent.
func (s *Store) MetaDelete(key string) error {
	_, err := s.db.Exec(`DELETE FROM meta WHERE key = ?`, key)
	if err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("meta delete %q", key))
	}
	return nil
}

// SchemaVersion returns the highest migration version currently applied
// to the database, or 0 if the database is fresh.
func (s *Store) SchemaVersion() (int, error) {
	return s.metaInt("schema_version")
}

// metaInt is a small helper that reads a meta value and parses it as
// an int. Returns 0 if the row is absent or unparseable.
func (s *Store) metaInt(key string) (int, error) {
	v, err := s.MetaGet(key)
	if err != nil {
		return 0, err
	}
	if v == "" {
		return 0, nil
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return 0, nil
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// TouchInitMeta primes the meta table with got_version, init_at, and
// init_user. Called by `got init` after migrations run. It is a no-op
// if all three keys are already set; the caller can pass force=true to
// overwrite them (used by `got init --force`).
func (s *Store) TouchInitMeta(gotVersion, initUser string, at time.Time, force bool) error {
	if !force {
		// If init_at is set and we're not forcing, leave it alone so
		// future opens don't bump the timestamp.
		if existing, _ := s.MetaGet("init_at"); existing != "" {
			return nil
		}
	}
	if err := s.MetaSet("got_version", gotVersion); err != nil {
		return err
	}
	if err := s.MetaSet("init_at", fmt.Sprintf("%d", at.UnixMilli())); err != nil {
		return err
	}
	if err := s.MetaSet("init_user", initUser); err != nil {
		return err
	}
	return nil
}
