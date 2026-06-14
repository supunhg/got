package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/supunhg/got/internal/git"
	"github.com/supunhg/got/internal/store"
)

// newTestRepo creates a temporary Git repository and returns its path.
func newTestRepoIntegration(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "got-ws-integration-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}

	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		t.Fatalf("git init: %v\n%s", err, out)
	}

	for _, kv := range [][]string{{"user.name", "GOT Test"}, {"user.email", "test@got.sh"}} {
		cmd = exec.Command("git", "config", kv[0], kv[1])
		cmd.Dir = dir
		_ = cmd.Run()
	}

	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test\n"), 0o644); err != nil {
		os.RemoveAll(dir)
		t.Fatalf("WriteFile: %v", err)
	}

	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = dir
	_ = cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	return dir
}

// setupWorkspaceEnv creates a temp Git repo with a workspace and returns
// the repo path, store, adapter, and cleanup function.
func setupWorkspaceEnv(t *testing.T) (string, *store.KnowledgeStore, *git.ExecAdapter, func()) {
	t.Helper()

	repoPath := newTestRepoIntegration(t)

	// Create .got directory and database.
	gotDir := filepath.Join(repoPath, ".got")
	os.MkdirAll(gotDir, 0o755)
	dbPath := filepath.Join(gotDir, "got.db")

	s, err := store.Open(dbPath)
	if err != nil {
		os.RemoveAll(repoPath)
		t.Fatalf("store.Open: %v", err)
	}

	ks := store.NewKnowledgeStore(s.DB(), nil)

	adapter := git.NewExecAdapter(nil)
	ctx := context.Background()
	adapter.OpenRepository(ctx, repoPath)

	cleanup := func() {
		s.Close()
		os.RemoveAll(repoPath)
	}

	return repoPath, ks, adapter, cleanup
}

// TestWorkspaceCommitShowsInStatus tests that creating a workspace,
// adding a real branch, making a commit, and adding the workspace commit
// results in the workspace status showing that commit.
func TestWorkspaceCommitShowsInStatus(t *testing.T) {
	repoPath, ks, adapter, cleanup := setupWorkspaceEnv(t)
	defer cleanup()
	ctx := context.Background()

	// Create a workspace.
	ws, err := ks.CreateWorkspace(ctx, store.CreateWorkspaceParams{
		Name:        "oauth",
		Description: "OAuth 2.0 implementation",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	// Create a branch via Git adapter.
	if err := adapter.CreateBranch(ctx, "feat/oauth"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// Add the branch to the workspace.
	_, err = ks.AddWorkspaceBranch(ctx, ws.Name, "feat/oauth")
	if err != nil {
		t.Fatalf("AddWorkspaceBranch: %v", err)
	}

	// Checkout the branch, make a change, and commit.
	if err := adapter.CheckoutBranch(ctx, "feat/oauth"); err != nil {
		t.Fatalf("CheckoutBranch: %v", err)
	}

	// Create a file and commit.
	if err := os.WriteFile(filepath.Join(repoPath, "oauth.go"), []byte("package oauth\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	adapter.Run(ctx, "add", "oauth.go")
	sha, err := adapter.CreateCommit(ctx, "feat: add OAuth 2.0 flow", "")
	if err != nil {
		t.Fatalf("CreateCommit: %v", err)
	}

	// Record the commit in the workspace.
	_, err = ks.AddWorkspaceCommit(ctx, store.AddWorkspaceCommitParams{
		WorkspaceName: ws.Name,
		CommitSHA:     sha,
		BranchName:    "feat/oauth",
		Message:       "feat: add OAuth 2.0 flow",
	})
	if err != nil {
		t.Fatalf("AddWorkspaceCommit: %v", err)
	}

	// Verify workspace status shows the commit.
	status, err := ks.GetWorkspaceStatus(ctx, ws.Name)
	if err != nil {
		t.Fatalf("GetWorkspaceStatus: %v", err)
	}

	if len(status.Commits) != 1 {
		t.Fatalf("expected 1 commit in workspace status, got %d", len(status.Commits))
	}
	if status.Commits[0].CommitSHA != sha {
		t.Fatalf("expected commit SHA %q, got %q", sha, status.Commits[0].CommitSHA)
	}
	if status.Commits[0].BranchName != "feat/oauth" {
		t.Fatalf("expected branch 'feat/oauth', got %q", status.Commits[0].BranchName)
	}
	if status.Commits[0].Message != "feat: add OAuth 2.0 flow" {
		t.Fatalf("expected message 'feat: add OAuth 2.0 flow', got %q", status.Commits[0].Message)
	}

	// Verify last_commit_sha was updated.
	if status.Workspace.LastCommitSHA != sha {
		t.Fatalf("expected last_commit_sha %q, got %q", sha, status.Workspace.LastCommitSHA)
	}

	// Verify the branch appears in tracked branches.
	if len(status.Branches) != 1 || status.Branches[0].BranchName != "feat/oauth" {
		t.Fatalf("expected tracked branch 'feat/oauth', got %v", status.Branches)
	}
}

// TestWorkspaceAddBranchValidates test that add-branch validates the branch exists.
func TestWorkspaceAddBranchValidates(t *testing.T) {
	repoPath, ks, adapter, cleanup := setupWorkspaceEnv(t)
	defer cleanup()
	ctx := context.Background()

	// validateBranchName needs to find a Git repo (via findRepoRoot),
	// so change to the repo directory.
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(repoPath)

	_, err := ks.CreateWorkspace(ctx, store.CreateWorkspaceParams{Name: "ws"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	// Store-level AddWorkspaceBranch has no validation.
	_, err = ks.AddWorkspaceBranch(ctx, "ws", "nonexistent-branch")
	if err != nil {
		t.Fatalf("AddWorkspaceBranch should succeed for store (no validation): %v", err)
	}

	// At the CLI level, validateBranchName checks against real Git branches.
	branchErr := validateBranchName("nonexistent-branch")
	if branchErr == nil {
		t.Fatal("expected error for non-existent branch")
	}
	if !strings.Contains(branchErr.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", branchErr)
	}

	// Create the branch via adapter and verify validation passes.
	if err := adapter.CreateBranch(ctx, "real-branch"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	if err := validateBranchName("real-branch"); err != nil {
		t.Fatalf("expected no error for real branch, got: %v", err)
	}
}

// TestWorkspaceAddFileValidatesNonExistent tests that add-file with a
// non-existent file fails with a helpful error.
func TestWorkspaceAddFileValidatesNonExistent(t *testing.T) {
	repoPath, _, _, cleanup := setupWorkspaceEnv(t)
	defer cleanup()

	// Save and restore the working directory.
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(repoPath)

	// validateFilePath should fail for a non-existent file.
	err := validateFilePath("nonexistent-file.go")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected 'does not exist' error, got: %v", err)
	}

	// Create a real file and verify validation passes.
	if err := os.WriteFile(filepath.Join(repoPath, "real-file.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err = validateFilePath("real-file.go")
	if err != nil {
		t.Fatalf("expected no error for existing file, got: %v", err)
	}
}

func TestEventDrivenWorkspaceUpdate(t *testing.T) {
	repoPath, ks, adapter, cleanup := setupWorkspaceEnv(t)
	defer cleanup()
	ctx := context.Background()

	ws, err := ks.CreateWorkspace(ctx, store.CreateWorkspaceParams{Name: "ws"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	_, err = ks.AddWorkspaceBranch(ctx, ws.Name, "main")
	if err != nil {
		t.Fatalf("AddWorkspaceBranch: %v", err)
	}

	// Make a commit on main.
	if err := os.WriteFile(filepath.Join(repoPath, "feature.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	adapter.Run(ctx, "add", "feature.go")
	sha, err := adapter.CreateCommit(ctx, "feat: add feature", "")
	if err != nil {
		t.Fatalf("CreateCommit: %v", err)
	}

	// Simulate the event-driven flow: record the commit in the workspace.
	_, err = ks.AddWorkspaceCommit(ctx, store.AddWorkspaceCommitParams{
		WorkspaceName: ws.Name,
		CommitSHA:     sha,
		BranchName:    "main",
		Message:       "feat: add feature",
	})
	if err != nil {
		t.Fatalf("AddWorkspaceCommit: %v", err)
	}

	// Verify workspace was updated.
	status, err := ks.GetWorkspaceStatus(ctx, ws.Name)
	if err != nil {
		t.Fatalf("GetWorkspaceStatus: %v", err)
	}

	if status.Workspace.LastCommitSHA != sha {
		t.Fatalf("expected last_commit_sha %q, got %q", sha, status.Workspace.LastCommitSHA)
	}
	if len(status.Commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(status.Commits))
	}
}
