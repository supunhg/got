// Package worktree provides the .got/worktrees.json porcelain tracker
// used by `got worktree` (spec §14). Git's own worktree bookkeeping
// (.git/worktrees/<id>/) is the source of truth for what is checked
// out where; this file is a sidecar that adds a few GOT-specific
// fields (last-attached, editor hint, friendly label) so the CLI and
// the future TUI dashboard can render a richer view without re-running
// `git worktree list` on every command.
//
// The file is optional: if it doesn't exist, Read returns an empty
// slice (not an error). Every Write is atomic — write to a temp file
// in the same directory, then rename — so a crash mid-write cannot
// leave a half-truncated file on disk.
package worktree

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/got-sh/got/internal/gerr"
)

// FileName is the basename of the porcelain tracker inside .got/.
// It lives next to got.db so all GOT metadata is colocated.
const FileName = "worktrees.json"

// FileVersion is the integer version of the on-disk schema. Bump
// when making breaking changes to WorktreeRecord so a future loader
// can detect and migrate older files.
const FileVersion = 1

// WorktreeRecord is one entry in the porcelain tracker. It mirrors
// the relevant subset of git.Worktree and adds a few GOT-specific
// fields:
//
//   - Label: a user-friendly alias (e.g. "alice's sandbox"); the
//     CLI uses it as the primary display string when set.
//   - Editor: optional command to launch when the user picks
//     "Open in editor" from the TUI (e.g. "code", "mate -w").
//   - LastAttachedAt: timestamp of the most recent `got worktree
//     attach` against this record, used to sort by recency.
//   - Notes: free-form text the user can attach to a worktree.
//
// The Path field is the key: the CLI looks records up by absolute
// path so two records with the same path (which the on-disk format
// forbids) would conflict on read.
type WorktreeRecord struct {
	// Path is the absolute filesystem path of the worktree.
	Path string `json:"path"`
	// Branch is the short ref name (e.g. "main", "feature/x").
	// Empty for detached worktrees.
	Branch string `json:"branch,omitempty"`
	// HEAD is the full commit SHA the worktree is on.
	HEAD string `json:"head,omitempty"`
	// Label is a user-friendly alias shown by `got worktree list`.
	Label string `json:"label,omitempty"`
	// Editor is an optional command for the TUI's "Open in editor"
	// action. Empty means "use $EDITOR" or skip.
	Editor string `json:"editor,omitempty"`
	// LastAttachedAt is the last time the user ran `got worktree
	// attach` against this record.
	LastAttachedAt time.Time `json:"lastAttachedAt,omitempty"`
	// Notes is free-form text the user attached to the worktree.
	Notes string `json:"notes,omitempty"`
}

// fileBody is the on-disk JSON wrapper. The version field lets a
// future loader refuse to read a file it doesn't understand; the
// records slice is the actual payload.
type fileBody struct {
	Version   int              `json:"version"`
	Records   []WorktreeRecord `json:"records"`
	UpdatedAt time.Time        `json:"updatedAt"`
}

// Store is a thin handle over a single .got/worktrees.json file.
// All methods are safe for concurrent use; the embedded mutex
// serializes the read-modify-write cycle that Update performs.
type Store struct {
	// Path is the absolute path of the on-disk file. Exposed for
	// tests that want to assert the file lives at the right spot.
	Path string

	mu sync.Mutex
}

// NewStore returns a Store backed by the given file path. The file
// need not exist; Read treats a missing file as an empty tracker.
// Write creates the parent directory (MkdirAll, 0o755) if it
// does not exist, so callers can hand Write a fresh .got/ tree
// without pre-creating it.
func NewStore(path string) *Store {
	return &Store{Path: path}
}

// Read returns the full record list. A missing file yields an
// empty slice and a nil error; only parse or permission failures
// return an error. Records are returned in on-disk order (the
// caller is free to re-sort).
func (s *Store) Read() ([]WorktreeRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readLocked()
}

func (s *Store) readLocked() ([]WorktreeRecord, error) {
	body, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []WorktreeRecord{}, nil
		}
		return nil, gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("reading %s", s.Path))
	}
	if len(body) == 0 {
		return []WorktreeRecord{}, nil
	}
	var b fileBody
	if err := json.Unmarshal(body, &b); err != nil {
		return nil, gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("parsing %s", s.Path))
	}
	if b.Records == nil {
		return []WorktreeRecord{}, nil
	}
	return b.Records, nil
}

// Write replaces the entire record list atomically. The write goes
// to a temp file in the same directory and is then renamed into
// place; readers always see either the old or the new file, never
// a half-written one. Records are written verbatim (the caller is
// responsible for sorting and dedup). The parent directory is
// created (MkdirAll, 0o755) if it does not exist, so callers can
// hand Write a fresh .got/ tree without pre-creating it.
func (s *Store) Write(records []WorktreeRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeLocked(records)
}

func (s *Store) writeLocked(records []WorktreeRecord) error {
	if records == nil {
		records = []WorktreeRecord{}
	}
	body := fileBody{
		Version:   FileVersion,
		Records:   records,
		UpdatedAt: time.Now().UTC(),
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, "encoding worktrees.json")
	}
	data = append(data, '\n')
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("creating %s", dir))
	}
	tmp, err := os.CreateTemp(dir, ".worktrees-*.json.tmp")
	if err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("creating temp file in %s", dir))
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("writing %s", tmpName))
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("closing %s", tmpName))
	}
	if err := os.Rename(tmpName, s.Path); err != nil {
		cleanup()
		return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("renaming %s to %s", tmpName, s.Path))
	}
	return nil
}

// Update applies the given mutation function to the current record
// list and writes the result back. It is the right entry point for
// callers that want read-modify-write semantics without writing
// the boilerplate. The mutation runs under the store's lock so
// concurrent Update callers do not race.
func (s *Store) Update(mutate func(records []WorktreeRecord) []WorktreeRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, err := s.readLocked()
	if err != nil {
		return err
	}
	next := mutate(cur)
	return s.writeLocked(next)
}

// FindByPath returns the record for the given absolute path, or
// (nil, false) if no such record exists. It does not return an
// error on "not found" — only on real I/O failures.
func (s *Store) FindByPath(path string) (*WorktreeRecord, bool, error) {
	records, err := s.Read()
	if err != nil {
		return nil, false, err
	}
	for i := range records {
		if records[i].Path == path {
			rec := records[i]
			return &rec, true, nil
		}
	}
	return nil, false, nil
}

// Upsert inserts or updates a record keyed by Path. The update
// function receives the existing record (or a zero value if none
// exists) and returns the new record to persist. A nil update
// function leaves an existing record unchanged but still inserts
// a new zero record — callers who want to no-op on missing records
// should check the bool return themselves.
func (s *Store) Upsert(path string, update func(existing *WorktreeRecord, found bool) WorktreeRecord) error {
	return s.Update(func(records []WorktreeRecord) []WorktreeRecord {
		for i := range records {
			if records[i].Path == path {
				rec := update(&records[i], true)
				records[i] = rec
				return records
			}
		}
		// Not found: append a fresh record.
		zero := WorktreeRecord{Path: path}
		rec := update(&zero, false)
		return append(records, rec)
	})
}

// Remove deletes the record with the given path. It returns true
// if a record was removed, false if no such record existed. A nil
// error always means the post-state contains no record with that
// path.
func (s *Store) Remove(path string) (bool, error) {
	var removed bool
	err := s.Update(func(records []WorktreeRecord) []WorktreeRecord {
		out := records[:0]
		for _, r := range records {
			if r.Path == path {
				removed = true
				continue
			}
			out = append(out, r)
		}
		return out
	})
	return removed, err
}
