// Copyright 2026 Supun Hewagamage. MIT License.
package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/supunhg/got/internal/git"
	"github.com/supunhg/got/internal/store"
)

// ── Snapshot CLI tests ─────────────────────────────────────────────

func TestSnapshotCreate(t *testing.T) {
	dir, s, cleanup := testCLIEnv(t)
	defer cleanup()

	// Initialize a Git repo so findRepoRoot works.
	initTestGitRepo(t, dir)

	_ = s
	cmd, buf := newTestCmd()
	err := runSnapshotCreate(cmd, "test-snapshot")
	if err != nil {
		t.Fatalf("runSnapshotCreate: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Snapshot created:") {
		t.Fatalf("expected 'Snapshot created:' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "test-snapshot") {
		t.Fatalf("expected 'test-snapshot' in output, got:\n%s", output)
	}
}

func TestSnapshotList_Empty(t *testing.T) {
	_, s, cleanup := testCLIEnv(t)
	defer cleanup()
	_ = s

	cmd, buf := newTestCmd()
	err := runSnapshotList(cmd, false)
	if err != nil {
		t.Fatalf("runSnapshotList: %v", err)
	}

	if !strings.Contains(buf.String(), "No snapshots found") {
		t.Fatalf("expected 'No snapshots found' in output, got:\n%s", buf.String())
	}
}

func TestSnapshotList_WithData(t *testing.T) {
	_, s, cleanup := testCLIEnv(t)
	defer cleanup()
	ctx := context.Background()

	ks := store.NewKnowledgeStore(s.DB(), nil)
	ks.CreateSnapshot(ctx, store.CreateSnapshotParams{Reason: "before-reset", Ref: "main (abc123)"})
	ks.CreateSnapshot(ctx, store.CreateSnapshotParams{Reason: "before-rebase", Ref: "main (def456)"})

	cmd, buf := newTestCmd()
	err := runSnapshotList(cmd, false)
	if err != nil {
		t.Fatalf("runSnapshotList: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "before-reset") {
		t.Fatalf("expected 'before-reset' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "before-rebase") {
		t.Fatalf("expected 'before-rebase' in output, got:\n%s", output)
	}
}

func TestSnapshotList_JSON(t *testing.T) {
	_, s, cleanup := testCLIEnv(t)
	defer cleanup()
	ctx := context.Background()

	ks := store.NewKnowledgeStore(s.DB(), nil)
	ks.CreateSnapshot(ctx, store.CreateSnapshotParams{Reason: "test", Ref: "main"})

	cmd, buf := newTestCmd()
	err := runSnapshotList(cmd, true)
	if err != nil {
		t.Fatalf("runSnapshotList JSON: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "\"reason\"") {
		t.Fatalf("expected JSON output with 'reason' field, got:\n%s", output)
	}
}

func TestSnapshotShow(t *testing.T) {
	_, s, cleanup := testCLIEnv(t)
	defer cleanup()
	ctx := context.Background()

	ks := store.NewKnowledgeStore(s.DB(), nil)
	snap, _ := ks.CreateSnapshot(ctx, store.CreateSnapshotParams{
		Reason: "before-reset",
		Ref:    "main (abc123def456)",
	})

	cmd, buf := newTestCmd()
	err := runSnapshotShow(cmd, snap.ID, false)
	if err != nil {
		t.Fatalf("runSnapshotShow: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "before-reset") {
		t.Fatalf("expected 'before-reset' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "abc123def456") {
		t.Fatalf("expected ref in output, got:\n%s", output)
	}
}

func TestSnapshotShow_NotFound(t *testing.T) {
	_, _, cleanup := testCLIEnv(t)
	defer cleanup()

	cmd, _ := newTestCmd()
	err := runSnapshotShow(cmd, "nonexistent", false)
	if err == nil {
		t.Fatal("expected error for nonexistent snapshot")
	}
}

func TestSnapshotDelete(t *testing.T) {
	_, s, cleanup := testCLIEnv(t)
	defer cleanup()
	ctx := context.Background()

	ks := store.NewKnowledgeStore(s.DB(), nil)
	snap, _ := ks.CreateSnapshot(ctx, store.CreateSnapshotParams{Reason: "delete me", Ref: "main"})

	cmd, buf := newTestCmd()
	err := runSnapshotDelete(cmd, snap.ID)
	if err != nil {
		t.Fatalf("runSnapshotDelete: %v", err)
	}

	if !strings.Contains(buf.String(), "deleted") {
		t.Fatalf("expected 'deleted' in output, got:\n%s", buf.String())
	}

	// Verify it's gone.
	_, err = ks.GetSnapshot(ctx, snap.ID)
	if err != store.ErrSnapshotNotFound {
		t.Fatalf("expected ErrSnapshotNotFound after delete, got %v", err)
	}
}

// ── Health CLI tests ───────────────────────────────────────────────

func TestHealth_WithGotDir(t *testing.T) {
	dir, s, cleanup := testCLIEnv(t)
	defer cleanup()
	_ = s

	// Initialize a Git repo so findRepoRoot works.
	initTestGitRepo(t, dir)

	cmd, buf := newTestCmd()
	err := runHealth(cmd, false)
	if err != nil {
		t.Fatalf("runHealth: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "got-directory") {
		t.Fatalf("expected 'got-directory' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "database") {
		t.Fatalf("expected 'database' in output, got:\n%s", output)
	}
}

func TestHealth_NoGotDir(t *testing.T) {
	dir, err := os.MkdirTemp("", "got-health-no-got-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(dir)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	cmd, buf := newTestCmd()
	err = runHealth(cmd, false)
	if err != nil {
		t.Fatalf("runHealth: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, ".got/ directory not found") {
		t.Fatalf("expected '.got/ directory not found' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "fail") {
		t.Fatalf("expected 'fail' status in output, got:\n%s", output)
	}
}

func TestHealth_JSON(t *testing.T) {
	dir, s, cleanup := testCLIEnv(t)
	defer cleanup()
	_ = s

	initTestGitRepo(t, dir)

	cmd, buf := newTestCmd()
	err := runHealth(cmd, true)
	if err != nil {
		t.Fatalf("runHealth JSON: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "\"name\"") {
		t.Fatalf("expected JSON output with 'name' field, got:\n%s", output)
	}
}

// ── Workspace diff CLI tests ───────────────────────────────────────

func TestWorkspaceDiff_NoFiles(t *testing.T) {
	dir, s, cleanup := testCLIEnv(t)
	defer cleanup()
	ctx := context.Background()

	initTestGitRepo(t, dir)

	ks := store.NewKnowledgeStore(s.DB(), nil)
	ks.CreateWorkspace(ctx, store.CreateWorkspaceParams{Name: "test-ws"})

	cmd, buf := newTestCmd()
	err := runWorkspaceDiff(cmd, "test-ws")
	if err != nil {
		t.Fatalf("runWorkspaceDiff: %v", err)
	}

	if !strings.Contains(buf.String(), "no tracked files") {
		t.Fatalf("expected 'no tracked files' in output, got:\n%s", buf.String())
	}
}

func TestWorkspaceDiff_WorkspaceNotFound(t *testing.T) {
	_, _, cleanup := testCLIEnv(t)
	defer cleanup()

	cmd, _ := newTestCmd()
	err := runWorkspaceDiff(cmd, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent workspace")
	}
}

// ── Completion CLI tests ───────────────────────────────────────────

func TestCompletion_Bash(t *testing.T) {
	root := NewRootCmd()
	_, buf := newTestCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"completion", "bash"})
	err := root.Execute()
	if err != nil {
		t.Fatalf("completion bash: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "bash") {
		t.Fatalf("expected bash completion output, got:\n%s", output[:200])
	}
	if len(output) < 100 {
		t.Fatalf("expected substantial completion output, got %d bytes", len(output))
	}
}

func TestCompletion_Zsh(t *testing.T) {
	root := NewRootCmd()
	_, buf := newTestCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"completion", "zsh"})
	err := root.Execute()
	if err != nil {
		t.Fatalf("completion zsh: %v", err)
	}

	output := buf.String()
	if len(output) < 100 {
		t.Fatalf("expected substantial completion output, got %d bytes", len(output))
	}
}

func TestCompletion_InvalidShell(t *testing.T) {
	root := NewRootCmd()
	root.SetArgs([]string{"completion", "invalid"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for invalid shell")
	}
}

// ── Safe CLI tests ─────────────────────────────────────────────────

func TestSafeReset_InvalidMode(t *testing.T) {
	dir, _, cleanup := testCLIEnv(t)
	defer cleanup()

	initTestGitRepo(t, dir)

	cmd, _ := newTestCmd()
	cmd.Flags().String("mode", "invalid", "")
	err := runSafeReset(cmd, nil)
	if err == nil {
		t.Fatal("expected error for invalid reset mode")
	}
	if !strings.Contains(err.Error(), "invalid reset mode") {
		t.Fatalf("expected 'invalid reset mode' in error, got %q", err.Error())
	}
}

// ── Helpers ────────────────────────────────────────────────────────

// initTestGitRepo initializes a Git repository in the given directory
// with a minimal commit so HEAD exists. If the directory already
// contains a .git directory, it is a no-op.
func initTestGitRepo(t *testing.T, dir string) {
	t.Helper()

	// Check if already a git repo.
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return
	}

	adapter := git.NewExecAdapter(nil)
	adapter.OpenRepository(context.Background(), dir)

	if _, _, err := adapter.Run(context.Background(), "init", "-b", "main"); err != nil {
		t.Fatalf("initTestGitRepo: git init: %v", err)
	}
	adapter.Run(context.Background(), "config", "user.name", "Test")
	adapter.Run(context.Background(), "config", "user.email", "test@test.com")

	// Create an initial commit so HEAD exists.
	readme := filepath.Join(dir, "README.md")
	os.WriteFile(readme, []byte("# Test\n"), 0o644)
	adapter.Run(context.Background(), "add", "README.md")
	if _, _, err := adapter.Run(context.Background(), "commit", "-m", "Initial commit"); err != nil {
		t.Fatalf("initTestGitRepo: initial commit: %v", err)
	}
}
