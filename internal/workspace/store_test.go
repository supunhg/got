package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	storepkg "github.com/got-sh/got/internal/store"
)

// pinnedTime is the clock the tests use for now(). It is fixed so
// timestamp assertions are stable across runs.
var pinnedTime = time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

// newTestStore builds a fresh workspace.Store backed by a tempdir
// SQLite database. The returned cleanup function closes the
// underlying store; tests should defer it.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := storepkg.Open(filepath.Join(dir, "got.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return NewWithDB(s.DB(), func() time.Time { return pinnedTime })
}

func TestValidName(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"simple", "oauth", true},
		{"with-hyphen", "oauth-refactor", true},
		{"with-underscore", "oauth_flow", true},
		{"with-digits", "v2-oauth", true},
		{"single-letter", "a", true},
		{"empty", "", false},
		{"starts-with-digit", "1abc", false},
		{"uppercase", "OAuth", false},
		{"space", "my workspace", false},
		{"slash", "feature/x", false},
		{"too-long", strings.Repeat("a", 64), false},
		{"max-len-ok", strings.Repeat("a", 63), true},
		{"special-char", "foo@bar", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidName(tc.input); got != tc.want {
				t.Errorf("ValidName(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestStateValid(t *testing.T) {
	for _, s := range []State{StateOpen, StateArchived} {
		if !s.Valid() {
			t.Errorf("%q.Valid() = false, want true", s)
		}
	}
	for _, s := range []State{"", "draft", "deleted"} {
		if s.Valid() {
			t.Errorf("%q.Valid() = true, want false", s)
		}
	}
}

func TestDecisionStatusValid(t *testing.T) {
	for _, s := range []DecisionStatus{DecisionProposed, DecisionAccepted, DecisionRejected, DecisionSuperseded} {
		if !s.Valid() {
			t.Errorf("%q.Valid() = false, want true", s)
		}
	}
	if DecisionStatus("").Valid() {
		t.Error(`"".Valid() = true, want false`)
	}
	if DecisionStatus("pending").Valid() {
		t.Error(`"pending".Valid() = true, want false`)
	}
}

func TestCreateAndGet(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	w := &Workspace{
		Name:        "oauth-refactor",
		Title:       "OAuth Refactor",
		Description: "Implementing OAuth 2.0",
		Color:       "#3b82f6",
	}
	if err := st.Create(ctx, w); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if w.ID == "" {
		t.Error("Create did not assign ID")
	}
	if w.CreatedAt.IsZero() {
		t.Error("Create did not assign CreatedAt")
	}
	if !w.UpdatedAt.Equal(w.CreatedAt) {
		t.Errorf("UpdatedAt = %v, want %v (equal to CreatedAt on create)", w.UpdatedAt, w.CreatedAt)
	}
	if w.State != StateOpen {
		t.Errorf("default State = %q, want open", w.State)
	}

	// Get by ID.
	got, err := st.Get(ctx, w.ID)
	if err != nil {
		t.Fatalf("Get by ID: %v", err)
	}
	if got.Name != w.Name || got.Title != w.Title || got.Description != w.Description {
		t.Errorf("Get by ID returned %+v, want fields to match", got)
	}
	if got.Color != w.Color {
		t.Errorf("Color = %q, want %q", got.Color, w.Color)
	}
	if !got.CreatedAt.Equal(pinnedTime) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, pinnedTime)
	}

	// Get by name.
	got2, err := st.Get(ctx, "oauth-refactor")
	if err != nil {
		t.Fatalf("Get by name: %v", err)
	}
	if got2.ID != w.ID {
		t.Errorf("Get by name returned ID=%q, want %q", got2.ID, w.ID)
	}
}

func TestCreateRejectsBadInput(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// Nil pointer.
	if err := st.Create(ctx, nil); err == nil {
		t.Error("Create(nil) = nil, want error")
	}
	// Bad name.
	err := st.Create(ctx, &Workspace{Name: "Bad-Name", Title: "X"})
	var badName *ErrInvalidName
	if !errors.As(err, &badName) {
		t.Errorf("Create with bad name returned %v, want ErrInvalidName", err)
	}
	// Empty title.
	err = st.Create(ctx, &Workspace{Name: "ok-name", Title: ""})
	if !errors.Is(err, ErrEmptyTitle) {
		t.Errorf("Create with empty title returned %v, want ErrEmptyTitle", err)
	}
	// Bad state.
	err = st.Create(ctx, &Workspace{Name: "ok-name2", Title: "T", State: "draft"})
	var badState *ErrInvalidState
	if !errors.As(err, &badState) {
		t.Errorf("Create with bad state returned %v, want ErrInvalidState", err)
	}
}

func TestCreateRejectsNameCollision(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	if err := st.Create(ctx, &Workspace{Name: "x", Title: "X"}); err != nil {
		t.Fatalf("Create #1: %v", err)
	}
	err := st.Create(ctx, &Workspace{Name: "x", Title: "X again"})
	if !errors.Is(err, ErrNameTaken) {
		t.Errorf("Create with duplicate name returned %v, want ErrNameTaken", err)
	}
}

func TestListFiltering(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	for _, w := range []*Workspace{
		{Name: "a", Title: "A", State: StateOpen},
		{Name: "b", Title: "B", State: StateOpen},
		{Name: "c", Title: "C", State: StateArchived},
	} {
		if err := st.Create(ctx, w); err != nil {
			t.Fatalf("Create %s: %v", w.Name, err)
		}
	}

	all, err := st.List(ctx, ListOptions{})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("List all = %d, want 3", len(all))
	}
	open, err := st.List(ctx, ListOptions{State: StateOpen})
	if err != nil {
		t.Fatalf("List open: %v", err)
	}
	if len(open) != 2 {
		t.Errorf("List open = %d, want 2", len(open))
	}
	arch, err := st.List(ctx, ListOptions{State: StateArchived})
	if err != nil {
		t.Fatalf("List archived: %v", err)
	}
	if len(arch) != 1 {
		t.Errorf("List archived = %d, want 1", len(arch))
	}
	limited, err := st.List(ctx, ListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("List limited: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("List limited = %d, want 2", len(limited))
	}
}

func TestUpdate(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	w := &Workspace{Name: "x", Title: "X"}
	if err := st.Create(ctx, w); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Move time forward and update.
	now := pinnedTime.Add(time.Hour)
	st.now = func() time.Time { return now }

	w.Title = "X (renamed)"
	w.State = StateArchived
	w.Description = "new desc"
	if err := st.Update(ctx, w); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !w.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", w.UpdatedAt, now)
	}

	got, _ := st.Get(ctx, w.ID)
	if got.Title != "X (renamed)" || got.State != StateArchived || got.Description != "new desc" {
		t.Errorf("Get after Update = %+v, want updated fields", got)
	}
}

func TestUpdateValidation(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	w := &Workspace{Name: "x", Title: "X"}
	_ = st.Create(ctx, w)

	// Missing ID.
	if err := st.Update(ctx, &Workspace{Title: "X"}); err == nil {
		t.Error("Update without ID = nil, want error")
	}
	// Bad name.
	w.Name = "Bad-Name"
	if err := st.Update(ctx, w); err == nil {
		t.Error("Update with bad name = nil, want error")
	}
	// Bad state.
	w.Name = "x"
	w.State = "draft"
	if err := st.Update(ctx, w); err == nil {
		t.Error("Update with bad state = nil, want error")
	}
	// Not found.
	ghost := &Workspace{ID: "0188f3b7c5a0-7f3e9a2b1c4d8e0f", Title: "Y", Name: "y"}
	if err := st.Update(ctx, ghost); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update non-existent = %v, want ErrNotFound", err)
	}
}

func TestDeleteCascades(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	w := &Workspace{Name: "x", Title: "X"}
	if err := st.Create(ctx, w); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Add a file, branch, decision, note.
	if err := st.AddFile(ctx, w.ID, "foo.go", ""); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	if err := st.AddBranch(ctx, w.ID, "feature/x"); err != nil {
		t.Fatalf("AddBranch: %v", err)
	}
	if err := st.AddDecision(ctx, &WorkspaceDecision{WorkspaceID: w.ID, Title: "Use PKCE"}); err != nil {
		t.Fatalf("AddDecision: %v", err)
	}
	if err := st.AddNote(ctx, &WorkspaceNote{WorkspaceID: w.ID, Body: "hello"}); err != nil {
		t.Fatalf("AddNote: %v", err)
	}
	// Delete the workspace.
	if err := st.Delete(ctx, w.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// Every child table should be empty.
	if files, _ := st.ListFiles(ctx, w.ID); len(files) != 0 {
		t.Errorf("ListFiles after Delete = %d, want 0", len(files))
	}
	if branches, _ := st.ListBranches(ctx, w.ID); len(branches) != 0 {
		t.Errorf("ListBranches after Delete = %d, want 0", len(branches))
	}
	if decisions, _ := st.ListDecisions(ctx, w.ID); len(decisions) != 0 {
		t.Errorf("ListDecisions after Delete = %d, want 0", len(decisions))
	}
	if notes, _ := st.ListNotes(ctx, w.ID); len(notes) != 0 {
		t.Errorf("ListNotes after Delete = %d, want 0", len(notes))
	}
	// And the workspace itself is gone.
	if _, err := st.Get(ctx, w.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete = %v, want ErrNotFound", err)
	}
}

func TestDeleteByName(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	w := &Workspace{Name: "x", Title: "X"}
	_ = st.Create(ctx, w)
	if err := st.Delete(ctx, "x"); err != nil {
		t.Fatalf("Delete by name: %v", err)
	}
	if _, err := st.Get(ctx, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete by name = %v, want ErrNotFound", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	st := newTestStore(t)
	if err := st.Delete(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete missing = %v, want ErrNotFound", err)
	}
}

func TestAddFileIsIdempotent(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	w := &Workspace{Name: "x", Title: "X"}
	_ = st.Create(ctx, w)

	// First add.
	if err := st.AddFile(ctx, w.ID, "a.go", "first"); err != nil {
		t.Fatalf("AddFile #1: %v", err)
	}
	// Second add: same path, new note. Should overwrite note and
	// bump added_at.
	now := pinnedTime.Add(2 * time.Hour)
	st.now = func() time.Time { return now }
	if err := st.AddFile(ctx, w.ID, "a.go", "second"); err != nil {
		t.Fatalf("AddFile #2: %v", err)
	}
	files, err := st.ListFiles(ctx, w.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("ListFiles = %d rows, want 1 (idempotent)", len(files))
	}
	if files[0].Note != "second" {
		t.Errorf("Note = %q, want %q (upsert overwrote)", files[0].Note, "second")
	}
	if !files[0].AddedAt.Equal(now) {
		t.Errorf("AddedAt = %v, want %v (upsert bumped)", files[0].AddedAt, now)
	}
}

func TestRemoveFileIsIdempotent(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	w := &Workspace{Name: "x", Title: "X"}
	_ = st.Create(ctx, w)
	if err := st.RemoveFile(ctx, w.ID, "missing.go"); err != nil {
		t.Errorf("RemoveFile missing = %v, want nil (idempotent)", err)
	}
	_ = st.AddFile(ctx, w.ID, "a.go", "")
	if err := st.RemoveFile(ctx, w.ID, "a.go"); err != nil {
		t.Fatalf("RemoveFile: %v", err)
	}
	files, _ := st.ListFiles(ctx, w.ID)
	if len(files) != 0 {
		t.Errorf("ListFiles after RemoveFile = %d, want 0", len(files))
	}
}

func TestBranches(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	w := &Workspace{Name: "x", Title: "X"}
	_ = st.Create(ctx, w)
	for _, b := range []string{"main", "feature/x"} {
		if err := st.AddBranch(ctx, w.ID, b); err != nil {
			t.Fatalf("AddBranch %s: %v", b, err)
		}
	}
	branches, err := st.ListBranches(ctx, w.ID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 2 {
		t.Errorf("ListBranches = %d, want 2", len(branches))
	}
	// Branches should be sorted by name.
	if branches[0].Branch != "feature/x" {
		t.Errorf("branches[0] = %q, want feature/x (alphabetical sort)", branches[0].Branch)
	}
	if err := st.RemoveBranch(ctx, w.ID, "main"); err != nil {
		t.Fatalf("RemoveBranch: %v", err)
	}
	branches, _ = st.ListBranches(ctx, w.ID)
	if len(branches) != 1 || branches[0].Branch != "feature/x" {
		t.Errorf("ListBranches after remove = %+v, want only feature/x", branches)
	}
}

func TestDecisions(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	w := &Workspace{Name: "x", Title: "X"}
	_ = st.Create(ctx, w)

	d := &WorkspaceDecision{WorkspaceID: w.ID, Title: "Use PKCE", Body: "PKCE is the right answer"}
	if err := st.AddDecision(ctx, d); err != nil {
		t.Fatalf("AddDecision: %v", err)
	}
	if d.ID == "" || d.Status != DecisionProposed {
		t.Errorf("AddDecision: ID=%q Status=%q, want non-empty and proposed", d.ID, d.Status)
	}
	if d.CreatedAt.IsZero() {
		t.Error("AddDecision did not set CreatedAt")
	}

	// Get by ID.
	got, err := st.GetDecision(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if got.Title != d.Title || got.Body != d.Body {
		t.Errorf("GetDecision = %+v, want fields to match", got)
	}

	// Update.
	got.Status = DecisionAccepted
	got.Body = "Updated body"
	if err := st.UpdateDecision(ctx, got); err != nil {
		t.Fatalf("UpdateDecision: %v", err)
	}
	got2, _ := st.GetDecision(ctx, d.ID)
	if got2.Status != DecisionAccepted || got2.Body != "Updated body" {
		t.Errorf("after Update: %+v", got2)
	}

	// List.
	decisions, _ := st.ListDecisions(ctx, w.ID)
	if len(decisions) != 1 {
		t.Errorf("ListDecisions = %d, want 1", len(decisions))
	}

	// Remove.
	if err := st.RemoveDecision(ctx, d.ID); err != nil {
		t.Fatalf("RemoveDecision: %v", err)
	}
	if _, err := st.GetDecision(ctx, d.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetDecision after Remove = %v, want ErrNotFound", err)
	}
}

func TestDecisionStatusValidation(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	w := &Workspace{Name: "x", Title: "X"}
	_ = st.Create(ctx, w)

	err := st.AddDecision(ctx, &WorkspaceDecision{WorkspaceID: w.ID, Title: "T", Status: "pending"})
	var badStatus *ErrInvalidDecisionStatus
	if !errors.As(err, &badStatus) {
		t.Errorf("AddDecision with bad status returned %v, want ErrInvalidDecisionStatus", err)
	}

	// Update with bad status: need a valid row first.
	d := &WorkspaceDecision{WorkspaceID: w.ID, Title: "T"}
	_ = st.AddDecision(ctx, d)
	d.Status = "pending"
	if err := st.UpdateDecision(ctx, d); !errors.As(err, &badStatus) {
		t.Errorf("UpdateDecision with bad status returned %v, want ErrInvalidDecisionStatus", err)
	}
}

func TestNotes(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	w := &Workspace{Name: "x", Title: "X"}
	_ = st.Create(ctx, w)

	// Pinned note created later (createdAt = pinnedTime+1h) should
	// still come first because of the pinned-first ordering.
	later := pinnedTime.Add(time.Hour)
	st.now = func() time.Time { return pinnedTime }
	_ = st.AddNote(ctx, &WorkspaceNote{WorkspaceID: w.ID, Body: "older note", Pinned: false})
	st.now = func() time.Time { return later }
	_ = st.AddNote(ctx, &WorkspaceNote{WorkspaceID: w.ID, Body: "newer pinned", Pinned: true})

	notes, err := st.ListNotes(ctx, w.ID)
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("ListNotes = %d, want 2", len(notes))
	}
	if !notes[0].Pinned {
		t.Errorf("notes[0].Pinned = false, want true (pinned first)")
	}
	if notes[0].Body != "newer pinned" {
		t.Errorf("notes[0].Body = %q, want %q", notes[0].Body, "newer pinned")
	}

	// Update pinning.
	notes[1].Pinned = true
	if err := st.UpdateNote(ctx, &notes[1]); err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}
	notes2, _ := st.ListNotes(ctx, w.ID)
	for _, n := range notes2 {
		if !n.Pinned {
			t.Errorf("note %q not pinned after Update", n.Body)
		}
	}

	// Remove.
	if err := st.RemoveNote(ctx, notes[0].ID); err != nil {
		t.Fatalf("RemoveNote: %v", err)
	}
	notes3, _ := st.ListNotes(ctx, w.ID)
	if len(notes3) != 1 {
		t.Errorf("ListNotes after Remove = %d, want 1", len(notes3))
	}
}

func TestNoteValidation(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	w := &Workspace{Name: "x", Title: "X"}
	_ = st.Create(ctx, w)

	if err := st.AddNote(ctx, &WorkspaceNote{WorkspaceID: w.ID, Body: ""}); err == nil {
		t.Error("AddNote empty body = nil, want error")
	}
	if err := st.AddNote(ctx, &WorkspaceNote{WorkspaceID: "", Body: "x"}); err == nil {
		t.Error("AddNote empty workspaceID = nil, want error")
	}
	if err := st.UpdateNote(ctx, &WorkspaceNote{ID: ""}); err == nil {
		t.Error("UpdateNote empty ID = nil, want error")
	}
}

func TestShowAggregates(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	w := &Workspace{Name: "x", Title: "X"}
	_ = st.Create(ctx, w)
	_ = st.AddFile(ctx, w.ID, "a.go", "first")
	_ = st.AddBranch(ctx, w.ID, "main")
	_ = st.AddDecision(ctx, &WorkspaceDecision{WorkspaceID: w.ID, Title: "T"})
	_ = st.AddNote(ctx, &WorkspaceNote{WorkspaceID: w.ID, Body: "hello"})

	view, err := st.Show(ctx, "x")
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if view.Workspace == nil || view.Workspace.Name != "x" {
		t.Errorf("Show.Workspace = %+v, want x", view.Workspace)
	}
	if len(view.Files) != 1 || view.Files[0].Path != "a.go" {
		t.Errorf("Show.Files = %+v, want a.go", view.Files)
	}
	if len(view.Branches) != 1 || view.Branches[0].Branch != "main" {
		t.Errorf("Show.Branches = %+v, want main", view.Branches)
	}
	if len(view.Decisions) != 1 || view.Decisions[0].Title != "T" {
		t.Errorf("Show.Decisions = %+v, want T", view.Decisions)
	}
	if len(view.Notes) != 1 || view.Notes[0].Body != "hello" {
		t.Errorf("Show.Notes = %+v, want hello", view.Notes)
	}
}

func TestShowNotFound(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.Show(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Show missing = %v, want ErrNotFound", err)
	}
}

func TestGetNotFound(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.Get(context.Background(), ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get empty = %v, want ErrNotFound", err)
	}
	if _, err := st.Get(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing = %v, want ErrNotFound", err)
	}
}

func TestNewIDFormat(t *testing.T) {
	// Two IDs from the same millisecond should differ in the
	// random suffix; either way both should look like IDs.
	a := newID()
	b := newID()
	if !looksLikeID(a) || !looksLikeID(b) {
		t.Errorf("newID() returned non-conforming IDs: %q %q", a, b)
	}
	if a == b {
		t.Errorf("newID() returned duplicate IDs in the same call: %q", a)
	}
}
