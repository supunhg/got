package cli

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/supunhg/got/internal/store"
)

// ── Tests ───────────────────────────────────────────────────────────

// TestNoteDelete_Success verifies that deleting an existing note
// succeeds and prints the expected output.
func TestNoteDelete_Success(t *testing.T) {
	_, s, cleanup := testCLIEnv(t)
	defer cleanup()
	ctx := context.Background()

	ks := store.NewKnowledgeStore(s.DB(), nil)

	// Create a note to delete.
	n, err := ks.CreateNote(ctx, store.CreateNoteParams{
		Message:    "Delete this note",
		Branch:     "feature/x",
		CommitHash: "abc123",
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	cmd, buf := newTestCmd()
	err = runNoteDelete(cmd, n.ID)
	if err != nil {
		t.Fatalf("runNoteDelete: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Note deleted:") {
		t.Fatalf("expected 'Note deleted:' in output, got:\n%s", output)
	}
	if !strings.Contains(output, n.ID) {
		t.Fatalf("expected note ID %q in output, got:\n%s", n.ID, output)
	}
	if !strings.Contains(output, "Delete this note") {
		t.Fatalf("expected message in output, got:\n%s", output)
	}

	// Verify the note is actually gone from the DB.
	_, err = ks.GetNote(ctx, n.ID)
	if err != store.ErrNoteNotFound {
		t.Fatalf("expected ErrNoteNotFound after delete, got %v", err)
	}
}

// TestNoteDelete_Nonexistent verifies that deleting a non-existent
// ID returns an error.
func TestNoteDelete_Nonexistent(t *testing.T) {
	_, _, cleanup := testCLIEnv(t)
	defer cleanup()

	cmd, _ := newTestCmd()
	err := runNoteDelete(cmd, "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for nonexistent note, got nil")
	}
	if !strings.Contains(err.Error(), "note not found") {
		t.Fatalf("expected 'note not found' in error, got %q", err.Error())
	}
}

// TestNoteDelete_NoGotDir verifies that running delete outside a
// GOT-initialized directory returns the appropriate error.
func TestNoteDelete_NoGotDir(t *testing.T) {
	dir, err := os.MkdirTemp("", "got-cli-no-got-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(dir)

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(origDir)

	cmd, _ := newTestCmd()
	err = runNoteDelete(cmd, "any-id")
	if err == nil {
		t.Fatal("expected error outside .got/, got nil")
	}
	if !strings.Contains(err.Error(), "GOT not initialized") {
		t.Fatalf("expected 'GOT not initialized' in error, got %q", err.Error())
	}
}
