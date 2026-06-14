// Package store implements the SQLite persistence layer for GOT. It
// provides a minimal migration runner using embedded SQL files and
// sub-store accessors for each domain (knowledge, workspaces, etc.).
//
// Copyright 2026 The GOT Authors. MIT License.
//
// The migration framework is intentionally lightweight — it reads and
// executes .sql files embedded via //go:embed in filename order.
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // register "sqlite" driver for database/sql
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store wraps an *sql.DB connection and provides migration support.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and runs any
// pending migrations. The database is opened in WAL mode with
// synchronous=NORMAL for a good balance of performance and durability.
func Open(path string) (*Store, error) {
	db, err := sql.Open(
		"sqlite", path+
			"?_pragma=journal_mode(WAL)"+
			"&_pragma=synchronous(NORMAL)"+
			"&_pragma=busy_timeout(5000)"+
			"&_pragma=foreign_keys(1)",
	)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: migrate %s: %w", path, err)
	}

	return s, nil
}

// DB returns the underlying *sql.DB so callers can create sub-stores.
func (s *Store) DB() *sql.DB { return s.db }

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// ── Migration runner ────────────────────────────────────────────────

// migrate reads all embedded .sql files from migrations/, sorts them by
// filename, and executes each one that has not yet been applied.
//
// Migration tracking uses a simple meta table:
//
//	CREATE TABLE IF NOT EXISTS schema_migrations (
//	    filename  TEXT PRIMARY KEY,
//	    applied_at INTEGER NOT NULL
//	);
func (s *Store) migrate() error {
	// Ensure the tracking table exists.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename  TEXT PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// Collect .sql files sorted by name.
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, fname := range files {
		// Check if already applied.
		var count int
		if err := s.db.QueryRow(
			"SELECT COUNT(*) FROM schema_migrations WHERE filename = ?", fname,
		).Scan(&count); err != nil {
			return fmt.Errorf("check migration %s: %w", fname, err)
		}
		if count > 0 {
			continue
		}

		// Read and execute.
		content, err := migrationsFS.ReadFile("migrations/" + fname)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", fname, err)
		}

		// SQLite's Exec handles multi-statement strings.
		if _, err := s.db.Exec(string(content)); err != nil {
			return fmt.Errorf("execute migration %s: %w", fname, err)
		}

		// Record.
		if _, err := s.db.Exec(
			"INSERT INTO schema_migrations (filename, applied_at) VALUES (?, ?)",
			fname, nowMS(),
		); err != nil {
			return fmt.Errorf("record migration %s: %w", fname, err)
		}
	}

	return nil
}

// nowMS returns the current UTC time in Unix milliseconds.
func nowMS() int64 {
	return time.Now().UnixMilli()
}
