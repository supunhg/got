package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/store"
)

// testCLIEnv sets up a temporary directory with an initialised .got/
// SQLite database. It changes the working directory to the temp dir so
// that openKnowledgeStore / findGotDir can find .got/.
//
// Returns the temp dir path, the store (for seeding data), a cleanup
// function that restores the original working directory.
func testCLIEnv(t *testing.T) (string, *store.Store, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "got-cli-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}

	// Create .got/ directory.
	gotDir := filepath.Join(dir, ".got")
	if err := os.MkdirAll(gotDir, 0755); err != nil {
		os.RemoveAll(dir)
		t.Fatalf("MkdirAll .got: %v", err)
	}

	// Open the database — runs migrations and creates got.db.
	dbPath := filepath.Join(gotDir, "got.db")
	s, err := store.Open(dbPath)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("store.Open: %v", err)
	}

	// Save original working directory and chdir to temp dir.
	origDir, err := os.Getwd()
	if err != nil {
		s.Close()
		os.RemoveAll(dir)
		t.Fatalf("Getwd: %v", err)
	}

	if err := os.Chdir(dir); err != nil {
		s.Close()
		os.RemoveAll(dir)
		t.Fatalf("Chdir: %v", err)
	}

	cleanup := func() {
		os.Chdir(origDir)
		s.Close()
		os.RemoveAll(dir)
	}

	return dir, s, cleanup
}

// newTestCmd creates a cobra.Command configured to capture stdout for
// testing. It sets SilenceUsage and SilenceErrors so that validation
// failures don't print usage text.
func newTestCmd() (*cobra.Command, *bytes.Buffer) {
	buf := new(bytes.Buffer)
	cmd := &cobra.Command{
		Use:           "test",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.SetOut(buf)
	return cmd, buf
}

// ── Tests ───────────────────────────────────────────────────────────

// TestDecisionDelete_Success verifies that deleting an existing decision
// succeeds, prints the expected output, and cleans up the body file.
func TestDecisionDelete_Success(t *testing.T) {
	dir, s, cleanup := testCLIEnv(t)
	defer cleanup()
	ctx := context.Background()

	ks := store.NewKnowledgeStore(s.DB(), nil)

	// Create a decision to delete.
	d, err := ks.CreateDecision(ctx, store.CreateDecisionParams{
		Title:    "Delete this decision",
		Context:  "Test context",
		Decision: "Delete it",
	})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}

	// Create the body file on disk so we can verify cleanup.
	bodyPath := filepath.Join(dir, ".got", d.BodyPath)
	if err := os.MkdirAll(filepath.Dir(bodyPath), 0755); err != nil {
		t.Fatalf("MkdirAll body dir: %v", err)
	}
	if err := os.WriteFile(bodyPath, []byte("# "+d.Title+"\n\nBody content."), 0644); err != nil {
		t.Fatalf("WriteFile body: %v", err)
	}

	cmd, buf := newTestCmd()
	err = runDecisionDelete(cmd, d.ID)
	if err != nil {
		t.Fatalf("runDecisionDelete: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Decision deleted:") {
		t.Fatalf("expected 'Decision deleted:' in output, got:\n%s", output)
	}
	if !strings.Contains(output, d.ID) {
		t.Fatalf("expected decision ID %q in output, got:\n%s", d.ID, output)
	}
	if !strings.Contains(output, "Delete this decision") {
		t.Fatalf("expected title in output, got:\n%s", output)
	}

	// Verify the decision is actually gone from the DB.
	_, err = ks.GetDecision(ctx, d.ID)
	if err != store.ErrDecisionNotFound {
		t.Fatalf("expected ErrDecisionNotFound after delete, got %v", err)
	}

	// Verify the body file was removed from disk.
	if _, err := os.Stat(bodyPath); err == nil {
		t.Fatalf("expected body file %s to be removed, but it still exists", bodyPath)
	}
}

// TestDecisionDelete_MissingBodyFile verifies that deleting a decision
// succeeds even when the body file doesn't exist on disk.
func TestDecisionDelete_MissingBodyFile(t *testing.T) {
	_, s, cleanup := testCLIEnv(t)
	defer cleanup()
	ctx := context.Background()

	ks := store.NewKnowledgeStore(s.DB(), nil)

	d, err := ks.CreateDecision(ctx, store.CreateDecisionParams{
		Title: "No body file",
	})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}

	cmd, buf := newTestCmd()
	err = runDecisionDelete(cmd, d.ID)
	if err != nil {
		t.Fatalf("runDecisionDelete (missing body file): %v", err)
	}

	if !strings.Contains(buf.String(), "Decision deleted:") {
		t.Fatalf("expected 'Decision deleted:' in output, got:\n%s", buf.String())
	}
}

// TestDecisionDelete_Nonexistent verifies that deleting a non-existent
// ID returns an error.

// TestDecisionDelete_Nonexistent verifies that deleting a non-existent
// ID returns an error.
func TestDecisionDelete_Nonexistent(t *testing.T) {
	_, _, cleanup := testCLIEnv(t)
	defer cleanup()

	cmd, _ := newTestCmd()
	err := runDecisionDelete(cmd, "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for nonexistent decision, got nil")
	}
	if !strings.Contains(err.Error(), "decision not found") {
		t.Fatalf("expected 'decision not found' in error, got %q", err.Error())
	}
}

// TestDecisionDelete_NoGotDir verifies that running delete outside a
// GOT-initialised directory returns the appropriate error.
func TestDecisionDelete_NoGotDir(t *testing.T) {
	// Create a temp dir without .got/ and chdir into it.
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
	err = runDecisionDelete(cmd, "any-id")
	if err == nil {
		t.Fatal("expected error outside .got/, got nil")
	}
	if !strings.Contains(err.Error(), "GOT not initialized") {
		t.Fatalf("expected 'GOT not initialized' in error, got %q", err.Error())
	}
}
