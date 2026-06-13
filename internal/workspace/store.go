// Package workspace — Store implementation.
//
// The Store is a thin layer over the *sql.DB handle that the
// internal/store package already manages. It does NOT own the
// database lifetime: callers open and close the underlying
// *store.Store and pass the *sql.DB in via New or NewWithDB.
// This keeps the package trivially testable (a tempdir + store.Open
// is enough) and lets the CLI share one DB across many workspace
// operations without re-opening.
package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	storepkg "github.com/got-sh/got/internal/store"
)

// Store provides CRUD over the workspace tables in the GOT
// metadata DB. The zero value is not usable; construct with New
// or NewWithDB.
type Store struct {
	// db is the underlying *sql.DB handle.
	db *sql.DB
	// now returns the current time; injected so tests can pin
	// timestamps. Defaults to time.Now at construction.
	now func() time.Time
	// closer, when set, closes the underlying *storepkg.Store
	// on Store.Close. It is set by New (which takes the
	// *storepkg.Store directly) and left nil by NewWithDB
	// (test-only constructor). NewWithDB callers manage DB
	// lifetime themselves.
	closer func() error
}

// New wraps an already-open *store.Store. The returned Store
// takes ownership of the *storepkg.Store: calling Store.Close
// closes the *storepkg.Store too. This is the right model for
// the CLI's `openWorkspaceStore` helper, which opens the DB
// once per command and expects to close it on return.
func New(s *storepkg.Store) *Store {
	ws := NewWithDB(s.DB(), time.Now)
	ws.closer = s.Close
	return ws
}

// NewWithDB is the lower-level constructor for tests and
// synthetic *sql.DB handles (e.g. in-memory SQLite). The
// `now` argument is required; pass time.Now for production.
// Store.Close on a NewWithDB-built Store is a no-op because
// the caller manages DB lifetime.
func NewWithDB(db *sql.DB, now func() time.Time) *Store {
	return &Store{db: db, now: now}
}

// Close releases any resources owned by the Store. For a
// Store built with New, Close closes the underlying
// *storepkg.Store. For a Store built with NewWithDB, Close is
// a no-op. The method is safe to call on a nil receiver and
// returns the first non-nil error (typically from the
// underlying *storepkg.Store close).
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	if s.closer == nil {
		return nil
	}
	err := s.closer()
	s.closer = nil
	return err
}

// ---------------------------------------------------------------------------
// Workspace CRUD
// ---------------------------------------------------------------------------

// Create inserts w as a new workspace. If w.ID is empty, a fresh
// ULID-like ID is generated. If w.State is empty, it defaults to
// StateOpen. If w.CreatedAt / w.UpdatedAt are zero, they default
// to the injected clock. The name must satisfy ValidName and must
// not collide with an existing workspace.
func (s *Store) Create(ctx context.Context, w *Workspace) error {
	if w == nil {
		return errors.New("workspace: nil workspace")
	}
	if !ValidName(w.Name) {
		return &ErrInvalidName{Name: w.Name}
	}
	if w.Title == "" {
		return ErrEmptyTitle
	}
	if w.ID == "" {
		w.ID = newID()
	}
	if w.State == "" {
		w.State = StateOpen
	} else if !w.State.Valid() {
		return &ErrInvalidState{State: w.State}
	}
	now := s.now()
	if w.CreatedAt.IsZero() {
		w.CreatedAt = now
	}
	if w.UpdatedAt.IsZero() {
		w.UpdatedAt = now
	}
	meta, err := encodeMetadata(w.Metadata)
	if err != nil {
		return fmt.Errorf("workspace: encode metadata: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO workspaces(id, name, title, description, color, state, created_at, updated_at, metadata)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.Title, w.Description, w.Color, string(w.State),
		w.CreatedAt.UnixMilli(), w.UpdatedAt.UnixMilli(), meta)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrNameTaken
		}
		return fmt.Errorf("workspace: insert: %w", err)
	}
	return nil
}

// Get fetches a workspace by ID or by name. The disambiguation
// uses looksLikeID: hex-y strings go to the ID path, everything
// else (including short slugs) goes to the name path. This avoids
// a wasted round trip when the caller already knows which they
// have.
func (s *Store) Get(ctx context.Context, idOrName string) (*Workspace, error) {
	if idOrName == "" {
		return nil, ErrNotFound
	}
	var (
		row *sql.Row
	)
	if looksLikeID(idOrName) {
		row = s.db.QueryRowContext(ctx, selectWorkspaceByID, idOrName)
		w, err := scanWorkspace(row)
		if err == nil {
			return w, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		// Fall through to name lookup; the ID didn't match.
	}
	row = s.db.QueryRowContext(ctx, selectWorkspaceByName, idOrName)
	return scanWorkspace(row)
}

// List returns workspaces in the order specified by opts (created_at
// DESC by default, optionally filtered by state, optionally capped
// at opts.Limit). The returned slice is never nil (use len() to
// detect "no rows").
func (s *Store) List(ctx context.Context, opts ListOptions) ([]*Workspace, error) {
	q := selectWorkspaceBase
	args := []any{}
	if opts.State != "" {
		q += " WHERE state = ?"
		args = append(args, string(opts.State))
	}
	q += " ORDER BY created_at DESC, id DESC"
	if opts.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, opts.Limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("workspace: list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []*Workspace{}
	for rows.Next() {
		w, err := scanWorkspaceFromRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workspace: list rows: %w", err)
	}
	return out, nil
}

// Update modifies an existing workspace in place. The Name and
// State are validated; CreatedAt is preserved. UpdatedAt is set
// to the injected clock. To add or remove files/branches/decisions/
// notes, use the dedicated Add* / Remove* methods; this method
// only updates the workspace row itself.
func (s *Store) Update(ctx context.Context, w *Workspace) error {
	if w == nil {
		return errors.New("workspace: nil workspace")
	}
	if w.ID == "" {
		return errors.New("workspace: update requires ID")
	}
	if w.Name != "" && !ValidName(w.Name) {
		return &ErrInvalidName{Name: w.Name}
	}
	if w.State != "" && !w.State.Valid() {
		return &ErrInvalidState{State: w.State}
	}
	w.UpdatedAt = s.now()
	meta, err := encodeMetadata(w.Metadata)
	if err != nil {
		return fmt.Errorf("workspace: encode metadata: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE workspaces SET
			name        = COALESCE(NULLIF(?, ''), name),
			title       = COALESCE(NULLIF(?, ''), title),
			description = ?,
			color       = ?,
			state       = COALESCE(NULLIF(?, ''), state),
			updated_at  = ?,
			metadata    = ?
		WHERE id = ?`,
		w.Name, w.Title, w.Description, w.Color, string(w.State),
		w.UpdatedAt.UnixMilli(), meta, w.ID)
	if err != nil {
		return fmt.Errorf("workspace: update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("workspace: update rows: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a workspace by ID or name. ON DELETE CASCADE
// removes all child rows (files, branches, decisions, notes) in
// the same transaction; the caller does not need to call the
// per-entity Remove* methods first.
func (s *Store) Delete(ctx context.Context, idOrName string) error {
	if idOrName == "" {
		return ErrNotFound
	}
	var res sql.Result
	var err error
	if looksLikeID(idOrName) {
		res, err = s.db.ExecContext(ctx, `DELETE FROM workspaces WHERE id = ?`, idOrName)
		if err != nil {
			return fmt.Errorf("workspace: delete by id: %w", err)
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			return nil
		}
		// Fall through to name lookup.
	}
	res, err = s.db.ExecContext(ctx, `DELETE FROM workspaces WHERE name = ?`, idOrName)
	if err != nil {
		return fmt.Errorf("workspace: delete by name: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// File operations
// ---------------------------------------------------------------------------

// AddFile records path under workspaceID. If the row already exists
// (same workspace + path), the existing note is overwritten and
// added_at is bumped to now. Note that this is idempotent: a
// second AddFile for the same path is a no-op content-wise.
func (s *Store) AddFile(ctx context.Context, workspaceID, path, note string) error {
	if workspaceID == "" || path == "" {
		return errors.New("workspace: AddFile requires workspaceID and path")
	}
	now := s.now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO workspace_files(workspace_id, path, added_at, note)
		VALUES(?, ?, ?, ?)
		ON CONFLICT(workspace_id, path) DO UPDATE SET
			added_at = excluded.added_at,
			note     = excluded.note`,
		workspaceID, path, now.UnixMilli(), note)
	if err != nil {
		return fmt.Errorf("workspace: add file: %w", err)
	}
	return nil
}

// RemoveFile deletes the (workspace, path) row. Returns nil even
// if the row did not exist (idempotent).
func (s *Store) RemoveFile(ctx context.Context, workspaceID, path string) error {
	if workspaceID == "" || path == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM workspace_files WHERE workspace_id = ? AND path = ?`,
		workspaceID, path)
	if err != nil {
		return fmt.Errorf("workspace: remove file: %w", err)
	}
	return nil
}

// ListFiles returns every file tracked under workspaceID, ordered
// by path. The returned slice is never nil.
func (s *Store) ListFiles(ctx context.Context, workspaceID string) ([]WorkspaceFile, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT workspace_id, path, added_at, note FROM workspace_files WHERE workspace_id = ? ORDER BY path`,
		workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workspace: list files: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []WorkspaceFile{}
	for rows.Next() {
		var (
			wf      WorkspaceFile
			addedAt int64
		)
		if err := rows.Scan(&wf.WorkspaceID, &wf.Path, &addedAt, &wf.Note); err != nil {
			return nil, fmt.Errorf("workspace: scan file: %w", err)
		}
		wf.AddedAt = unixMilliToTime(addedAt)
		out = append(out, wf)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Branch operations
// ---------------------------------------------------------------------------

// AddBranch records branch under workspaceID. Idempotent: a second
// call for the same branch updates added_at.
func (s *Store) AddBranch(ctx context.Context, workspaceID, branch string) error {
	if workspaceID == "" || branch == "" {
		return errors.New("workspace: AddBranch requires workspaceID and branch")
	}
	now := s.now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO workspace_branches(workspace_id, branch, added_at)
		VALUES(?, ?, ?)
		ON CONFLICT(workspace_id, branch) DO UPDATE SET
			added_at = excluded.added_at`,
		workspaceID, branch, now.UnixMilli())
	if err != nil {
		return fmt.Errorf("workspace: add branch: %w", err)
	}
	return nil
}

// RemoveBranch deletes the (workspace, branch) row. Idempotent.
func (s *Store) RemoveBranch(ctx context.Context, workspaceID, branch string) error {
	if workspaceID == "" || branch == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM workspace_branches WHERE workspace_id = ? AND branch = ?`,
		workspaceID, branch)
	if err != nil {
		return fmt.Errorf("workspace: remove branch: %w", err)
	}
	return nil
}

// ListBranches returns every branch tracked under workspaceID,
// ordered by branch name. The returned slice is never nil.
func (s *Store) ListBranches(ctx context.Context, workspaceID string) ([]WorkspaceBranch, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT workspace_id, branch, added_at, last_seen_at FROM workspace_branches WHERE workspace_id = ? ORDER BY branch`,
		workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workspace: list branches: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []WorkspaceBranch{}
	for rows.Next() {
		var (
			wb          WorkspaceBranch
			addedAt     int64
			lastSeenAt  int64
		)
		if err := rows.Scan(&wb.WorkspaceID, &wb.Branch, &addedAt, &lastSeenAt); err != nil {
			return nil, fmt.Errorf("workspace: scan branch: %w", err)
		}
		wb.AddedAt = unixMilliToTime(addedAt)
		if lastSeenAt > 0 {
			wb.LastSeenAt = unixMilliToTime(lastSeenAt)
		}
		out = append(out, wb)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Decision CRUD
// ---------------------------------------------------------------------------

// AddDecision inserts a new decision. If d.ID is empty, a fresh
// ID is generated. The status is validated; an empty status
// defaults to DecisionProposed. CreatedAt / UpdatedAt default to
// the injected clock when zero.
func (s *Store) AddDecision(ctx context.Context, d *WorkspaceDecision) error {
	if d == nil {
		return errors.New("workspace: nil decision")
	}
	if d.WorkspaceID == "" || d.Title == "" {
		return errors.New("workspace: decision requires workspaceID and title")
	}
	if d.Status == "" {
		d.Status = DecisionProposed
	} else if !d.Status.Valid() {
		return &ErrInvalidDecisionStatus{Status: d.Status}
	}
	if d.ID == "" {
		d.ID = newID()
	}
	now := s.now()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = d.CreatedAt
	}
	meta, err := encodeMetadata(d.Metadata)
	if err != nil {
		return fmt.Errorf("workspace: encode decision metadata: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO workspace_decisions(id, workspace_id, title, body, status, created_at, updated_at, metadata)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.WorkspaceID, d.Title, d.Body, string(d.Status),
		d.CreatedAt.UnixMilli(), d.UpdatedAt.UnixMilli(), meta)
	if err != nil {
		return fmt.Errorf("workspace: insert decision: %w", err)
	}
	return nil
}

// UpdateDecision modifies an existing decision. The title and
// body are updated as given (empty body is allowed: it clears the
// body). Status is validated; empty means "leave alone". UpdatedAt
// is bumped to the injected clock. Returns ErrNotFound if no
// decision matches d.ID.
func (s *Store) UpdateDecision(ctx context.Context, d *WorkspaceDecision) error {
	if d == nil {
		return errors.New("workspace: nil decision")
	}
	if d.ID == "" {
		return errors.New("workspace: UpdateDecision requires ID")
	}
	if d.Status != "" && !d.Status.Valid() {
		return &ErrInvalidDecisionStatus{Status: d.Status}
	}
	d.UpdatedAt = s.now()
	res, err := s.db.ExecContext(ctx, `
		UPDATE workspace_decisions SET
			title      = ?,
			body       = ?,
			status     = COALESCE(NULLIF(?, ''), status),
			updated_at = ?
		WHERE id = ?`,
		d.Title, d.Body, string(d.Status), d.UpdatedAt.UnixMilli(), d.ID)
	if err != nil {
		return fmt.Errorf("workspace: update decision: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveDecision deletes a decision by ID. Idempotent.
func (s *Store) RemoveDecision(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM workspace_decisions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("workspace: remove decision: %w", err)
	}
	return nil
}

// GetDecision fetches a decision by ID. Returns ErrNotFound if no
// row matches.
func (s *Store) GetDecision(ctx context.Context, id string) (*WorkspaceDecision, error) {
	if id == "" {
		return nil, ErrNotFound
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, title, body, status, created_at, updated_at, metadata
		FROM workspace_decisions WHERE id = ?`, id)
	return scanDecision(row)
}

// ListDecisions returns every decision under workspaceID, ordered
// by created_at DESC. The returned slice is never nil.
func (s *Store) ListDecisions(ctx context.Context, workspaceID string) ([]WorkspaceDecision, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workspace_id, title, body, status, created_at, updated_at, metadata
		FROM workspace_decisions WHERE workspace_id = ?
		ORDER BY created_at DESC, id DESC`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workspace: list decisions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []WorkspaceDecision{}
	for rows.Next() {
		d, err := scanDecisionFromRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Note CRUD
// ---------------------------------------------------------------------------

// AddNote inserts a new note. If n.ID is empty, a fresh ID is
// generated. CreatedAt / UpdatedAt default to the injected clock
// when zero.
func (s *Store) AddNote(ctx context.Context, n *WorkspaceNote) error {
	if n == nil {
		return errors.New("workspace: nil note")
	}
	if n.WorkspaceID == "" || n.Body == "" {
		return errors.New("workspace: note requires workspaceID and body")
	}
	if n.ID == "" {
		n.ID = newID()
	}
	now := s.now()
	if n.CreatedAt.IsZero() {
		n.CreatedAt = now
	}
	if n.UpdatedAt.IsZero() {
		n.UpdatedAt = n.CreatedAt
	}
	pinned := 0
	if n.Pinned {
		pinned = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO workspace_notes(id, workspace_id, body, pinned, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?)`,
		n.ID, n.WorkspaceID, n.Body, pinned,
		n.CreatedAt.UnixMilli(), n.UpdatedAt.UnixMilli())
	if err != nil {
		return fmt.Errorf("workspace: insert note: %w", err)
	}
	return nil
}

// UpdateNote modifies an existing note. Body, Pinned, and
// UpdatedAt (injected clock) are written. Returns ErrNotFound if
// no row matches n.ID. n.Pinned is set to the value that was
// persisted (so callers can rely on the struct mirroring the row
// after the call).
func (s *Store) UpdateNote(ctx context.Context, n *WorkspaceNote) error {
	if n == nil {
		return errors.New("workspace: nil note")
	}
	if n.ID == "" {
		return errors.New("workspace: UpdateNote requires ID")
	}
	n.UpdatedAt = s.now()
	pinned := 0
	if n.Pinned {
		pinned = 1
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE workspace_notes SET
			body       = ?,
			pinned     = ?,
			updated_at = ?
		WHERE id = ?`,
		n.Body, pinned, n.UpdatedAt.UnixMilli(), n.ID)
	if err != nil {
		return fmt.Errorf("workspace: update note: %w", err)
	}
	n.Pinned = pinned == 1
	num, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("workspace: update note rows: %w", err)
	}
	if num == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveNote deletes a note by ID. Idempotent (a missing ID
// returns nil, not an error). The row is identified by the
// note's primary key; the workspace_id is not consulted, so a
// caller can detach a note without first looking up its
// workspace.
func (s *Store) RemoveNote(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM workspace_notes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("workspace: remove note: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Aggregated view
// ---------------------------------------------------------------------------

// Show returns the workspace aggregate with its child entities
// in a single call. Used by `got workspace show --json` so a
// plugin or external script can render the full picture in one
// subprocess call.
func (s *Store) Show(ctx context.Context, idOrName string) (*ShowView, error) {
	w, err := s.Get(ctx, idOrName)
	if err != nil {
		return nil, err
	}
	files, err := s.ListFiles(ctx, w.ID)
	if err != nil {
		return nil, err
	}
	branches, err := s.ListBranches(ctx, w.ID)
	if err != nil {
		return nil, err
	}
	decisions, err := s.ListDecisions(ctx, w.ID)
	if err != nil {
		return nil, err
	}
	notes, err := s.ListNotes(ctx, w.ID)
	if err != nil {
		return nil, err
	}
	return &ShowView{
		Workspace: w,
		Files:     files,
		Branches:  branches,
		Decisions: decisions,
		Notes:     notes,
	}, nil
}

// CountsByWorkspace returns per-workspace child counts for a
// batch of workspaces in a single round trip (one COUNT query
// per child table, GROUP BY workspace_id). The returned struct
// is keyed by workspace ID; workspaces with no children are
// simply absent from the maps. An empty input list returns
// empty maps without hitting the database. Used by
// `got workspace list` to render the FILES/BRANCHES/
// DECISIONS/NOTES columns in the table view.
func (s *Store) CountsByWorkspace(ctx context.Context, ws []*Workspace) (CountsByWorkspace, error) {
	ids := make([]string, 0, len(ws))
	for _, w := range ws {
		if w != nil {
			ids = append(ids, w.ID)
		}
	}
	out := CountsByWorkspace{
		Files:     make(map[string]int, len(ids)),
		Branches:  make(map[string]int, len(ids)),
		Decisions: make(map[string]int, len(ids)),
		Notes:     make(map[string]int, len(ids)),
	}
	if len(ids) == 0 {
		return out, nil
	}
	var err error
	if out.Files, err = s.countByWorkspace(ctx, "workspace_files", ids); err != nil {
		return out, err
	}
	if out.Branches, err = s.countByWorkspace(ctx, "workspace_branches", ids); err != nil {
		return out, err
	}
	if out.Decisions, err = s.countByWorkspace(ctx, "workspace_decisions", ids); err != nil {
		return out, err
	}
	if out.Notes, err = s.countByWorkspace(ctx, "workspace_notes", ids); err != nil {
		return out, err
	}
	return out, nil
}

// countByWorkspace runs a single SELECT workspace_id, COUNT(*)
// FROM <table> WHERE workspace_id IN (...) GROUP BY
// workspace_id query and returns a map of workspace_id to
// count. The table name is validated against a hard-coded
// allow-list (only the four child tables) and the placeholders
// are built dynamically from the ids slice. Each id is a
// separate parameter so the helper is safe to call with
// untrusted input as long as the table is from the allow-list.
func (s *Store) countByWorkspace(ctx context.Context, table string, ids []string) (map[string]int, error) {
	if !isAllowedChildTable(table) {
		return nil, fmt.Errorf("workspace: countByWorkspace: table %q is not in the allow-list", table)
	}
	out := make(map[string]int, len(ids))
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(
		"SELECT workspace_id, COUNT(*) FROM %s WHERE workspace_id IN (%s) GROUP BY workspace_id",
		table, strings.Join(placeholders, ","))
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("workspace: count by workspace on %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("workspace: scan count: %w", err)
		}
		out[id] = n
	}
	return out, rows.Err()
}

// isAllowedChildTable is the allow-list for the table name
// passed to countByWorkspace. The string concatenation in the
// query is otherwise an injection vector; this switch
// guarantees only the four workspace child tables can be
// referenced. Mirrors the isAllowedCountTable helper in
// internal/store/counts.go and serves the same defense-in-depth
// purpose: the call sites pass hard-coded literals, but a
// future refactor that wires table to user input fails loudly
// here instead of silently exposing every table in the schema.
func isAllowedChildTable(name string) bool {
	switch name {
	case "workspace_files", "workspace_branches", "workspace_decisions", "workspace_notes":
		return true
	default:
		return false
	}
}

// ListNotes returns every note under workspaceID. Pinned notes
// come first (most-recent-pinned first), then the rest in reverse
// chronological order. The returned slice is never nil.
func (s *Store) ListNotes(ctx context.Context, workspaceID string) ([]WorkspaceNote, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workspace_id, body, pinned, created_at, updated_at
		FROM workspace_notes WHERE workspace_id = ?
		ORDER BY pinned DESC, created_at DESC, id DESC`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workspace: list notes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []WorkspaceNote{}
	for rows.Next() {
		var (
			n         WorkspaceNote
			pinned    int
			createdAt int64
			updatedAt int64
		)
		if err := rows.Scan(&n.ID, &n.WorkspaceID, &n.Body, &pinned, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("workspace: scan note: %w", err)
		}
		n.Pinned = pinned != 0
		n.CreatedAt = unixMilliToTime(createdAt)
		n.UpdatedAt = unixMilliToTime(updatedAt)
		out = append(out, n)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Query constants and scan helpers
// ---------------------------------------------------------------------------

const (
	// selectWorkspaceByID / selectWorkspaceByName are the two
	// single-row queries used by Get. They are package-private
	// constants so a typo in one place (vs. a duplicated string
	// at every call site) fails to compile.
	selectWorkspaceByID = `SELECT id, name, title, description, color, state, created_at, updated_at, metadata
		FROM workspaces WHERE id = ?`
	selectWorkspaceByName = `SELECT id, name, title, description, color, state, created_at, updated_at, metadata
		FROM workspaces WHERE name = ?`
	// selectWorkspaceBase is the prefix shared by every List
	// query; opts append " WHERE ..." / " ORDER BY ..." / " LIMIT ?"
	// to it.
	selectWorkspaceBase = `SELECT id, name, title, description, color, state, created_at, updated_at, metadata
		FROM workspaces`
)

func scanWorkspace(row *sql.Row) (*Workspace, error) {
	var (
		w          Workspace
		state      string
		createdAt  int64
		updatedAt  int64
		metadata   string
	)
	err := row.Scan(&w.ID, &w.Name, &w.Title, &w.Description, &w.Color, &state, &createdAt, &updatedAt, &metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("workspace: scan: %w", err)
	}
	w.State = State(state)
	w.CreatedAt = unixMilliToTime(createdAt)
	w.UpdatedAt = unixMilliToTime(updatedAt)
	if metadata != "" && metadata != "{}" {
		if err := json.Unmarshal([]byte(metadata), &w.Metadata); err != nil {
			return nil, fmt.Errorf("workspace: decode metadata: %w", err)
		}
	}
	return &w, nil
}

func scanWorkspaceFromRows(rows *sql.Rows) (*Workspace, error) {
	var (
		w          Workspace
		state      string
		createdAt  int64
		updatedAt  int64
		metadata   string
	)
	if err := rows.Scan(&w.ID, &w.Name, &w.Title, &w.Description, &w.Color, &state, &createdAt, &updatedAt, &metadata); err != nil {
		return nil, fmt.Errorf("workspace: scan row: %w", err)
	}
	w.State = State(state)
	w.CreatedAt = unixMilliToTime(createdAt)
	w.UpdatedAt = unixMilliToTime(updatedAt)
	if metadata != "" && metadata != "{}" {
		if err := json.Unmarshal([]byte(metadata), &w.Metadata); err != nil {
			return nil, fmt.Errorf("workspace: decode metadata: %w", err)
		}
	}
	return &w, nil
}

func scanDecision(row *sql.Row) (*WorkspaceDecision, error) {
	var (
		d         WorkspaceDecision
		status    string
		createdAt int64
		updatedAt int64
		metadata  string
	)
	err := row.Scan(&d.ID, &d.WorkspaceID, &d.Title, &d.Body, &status, &createdAt, &updatedAt, &metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("workspace: scan decision: %w", err)
	}
	d.Status = DecisionStatus(status)
	d.CreatedAt = unixMilliToTime(createdAt)
	d.UpdatedAt = unixMilliToTime(updatedAt)
	if metadata != "" && metadata != "{}" {
		if err := json.Unmarshal([]byte(metadata), &d.Metadata); err != nil {
			return nil, fmt.Errorf("workspace: decode decision metadata: %w", err)
		}
	}
	return &d, nil
}

func scanDecisionFromRows(rows *sql.Rows) (*WorkspaceDecision, error) {
	var (
		d         WorkspaceDecision
		status    string
		createdAt int64
		updatedAt int64
		metadata  string
	)
	if err := rows.Scan(&d.ID, &d.WorkspaceID, &d.Title, &d.Body, &status, &createdAt, &updatedAt, &metadata); err != nil {
		return nil, fmt.Errorf("workspace: scan decision row: %w", err)
	}
	d.Status = DecisionStatus(status)
	d.CreatedAt = unixMilliToTime(createdAt)
	d.UpdatedAt = unixMilliToTime(updatedAt)
	if metadata != "" && metadata != "{}" {
		if err := json.Unmarshal([]byte(metadata), &d.Metadata); err != nil {
			return nil, fmt.Errorf("workspace: decode decision metadata: %w", err)
		}
	}
	return &d, nil
}

// unixMilliToTime converts a millisecond Unix timestamp to a UTC
// time.Time. Zero stays zero (so LastSeenAt stays zero when the
// row was inserted with the default 0).
func unixMilliToTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

// encodeMetadata marshals m to the JSON shape stored in the
// metadata column. A nil map becomes "{}" so the column always
// has a valid JSON value (the schema defaults to "{}" too, so
// the two stay in sync).
func encodeMetadata(m map[string]any) (string, error) {
	if m == nil {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// isUniqueViolation reports whether err is a SQLite UNIQUE
// constraint violation. modernc.org/sqlite returns the string
// "constraint failed: UNIQUE constraint failed: ..." in the
// error message; we match on the prefix so the dependency on
// the specific error type stays loose.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed")
}
