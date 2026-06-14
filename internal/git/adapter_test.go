package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newTestRepo creates a temporary Git repository and returns its path.
// It initialises the repo with an initial commit on "main".
func newTestRepo(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "got-git-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}

	// git init
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		t.Fatalf("git init: %v\n%s", err, out)
	}

	// Configure user for commits.
	for _, kv := range [][]string{{"user.name", "GOT Test"}, {"user.email", "test@got.sh"}} {
		cmd = exec.Command("git", "config", kv[0], kv[1])
		cmd.Dir = dir
		_ = cmd.Run()
	}

	// Create initial commit.
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

func TestNewExecAdapter(t *testing.T) {
	adapter := NewExecAdapter(nil)
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
}

func TestOpenRepository(t *testing.T) {
	repoPath := newTestRepo(t)
	defer os.RemoveAll(repoPath)

	ctx := context.Background()
	adapter := NewExecAdapter(nil)

	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}

	if adapter.Root() != repoPath {
		t.Fatalf("Root mismatch: %q vs %q", adapter.Root(), repoPath)
	}
}

func TestGetStatus_Clean(t *testing.T) {
	repoPath := newTestRepo(t)
	defer os.RemoveAll(repoPath)

	ctx := context.Background()
	adapter := NewExecAdapter(nil)
	adapter.OpenRepository(ctx, repoPath)

	status, err := adapter.GetStatus(ctx)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	if status.Branch != "main" {
		t.Fatalf("expected branch 'main', got %q", status.Branch)
	}
	if !status.Clean {
		t.Fatal("expected clean status")
	}
	if len(status.Untracked) != 0 {
		t.Fatalf("expected no untracked files, got %v", status.Untracked)
	}
}

func TestGetStatus_Dirty(t *testing.T) {
	repoPath := newTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create an untracked file.
	if err := os.WriteFile(filepath.Join(repoPath, "new.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx := context.Background()
	adapter := NewExecAdapter(nil)
	adapter.OpenRepository(ctx, repoPath)

	status, err := adapter.GetStatus(ctx)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	if status.Clean {
		t.Fatal("expected dirty status")
	}
	if len(status.Untracked) != 1 || status.Untracked[0] != "new.txt" {
		t.Fatalf("expected untracked ['new.txt'], got %v", status.Untracked)
	}
}

func TestCurrentBranch(t *testing.T) {
	repoPath := newTestRepo(t)
	defer os.RemoveAll(repoPath)

	ctx := context.Background()
	adapter := NewExecAdapter(nil)
	adapter.OpenRepository(ctx, repoPath)

	branch, err := adapter.CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "main" {
		t.Fatalf("expected 'main', got %q", branch)
	}
}

func TestListBranches(t *testing.T) {
	repoPath := newTestRepo(t)
	defer os.RemoveAll(repoPath)

	ctx := context.Background()
	adapter := NewExecAdapter(nil)
	adapter.OpenRepository(ctx, repoPath)

	branches, err := adapter.ListBranches(ctx)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}

	if len(branches) != 1 {
		t.Fatalf("expected 1 branch, got %d", len(branches))
	}
	if branches[0].Name != "main" {
		t.Fatalf("expected 'main', got %q", branches[0].Name)
	}
	if !branches[0].Current {
		t.Fatal("expected 'main' to be current")
	}
}

func TestCreateAndDeleteBranch(t *testing.T) {
	repoPath := newTestRepo(t)
	defer os.RemoveAll(repoPath)

	ctx := context.Background()
	adapter := NewExecAdapter(nil)
	adapter.OpenRepository(ctx, repoPath)

	// Create.
	if err := adapter.CreateBranch(ctx, "feature/test"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	branches, _ := adapter.ListBranches(ctx)
	if len(branches) != 2 {
		t.Fatalf("expected 2 branches after create, got %d", len(branches))
	}

	// Delete.
	if err := adapter.DeleteBranch(ctx, "feature/test", false); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}

	branches, _ = adapter.ListBranches(ctx)
	if len(branches) != 1 {
		t.Fatalf("expected 1 branch after delete, got %d", len(branches))
	}
}

func TestCheckoutBranch(t *testing.T) {
	repoPath := newTestRepo(t)
	defer os.RemoveAll(repoPath)

	ctx := context.Background()
	adapter := NewExecAdapter(nil)
	adapter.OpenRepository(ctx, repoPath)

	if err := adapter.CreateBranch(ctx, "develop"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// Switch to develop.
	if err := adapter.CheckoutBranch(ctx, "develop"); err != nil {
		t.Fatalf("CheckoutBranch: %v", err)
	}

	branch, _ := adapter.CurrentBranch(ctx)
	if branch != "develop" {
		t.Fatalf("expected 'develop', got %q", branch)
	}
}

func TestGetCommitHistory(t *testing.T) {
	repoPath := newTestRepo(t)
	defer os.RemoveAll(repoPath)

	ctx := context.Background()
	adapter := NewExecAdapter(nil)
	adapter.OpenRepository(ctx, repoPath)

	// Make another commit.
	if err := os.WriteFile(filepath.Join(repoPath, "second.txt"), []byte("second"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	exec.Command("git", "-C", repoPath, "add", ".").Run()
	exec.Command("git", "-C", repoPath, "commit", "-m", "Second commit").Run()

	commits, err := adapter.GetCommitHistory(ctx, "", 10)
	if err != nil {
		t.Fatalf("GetCommitHistory: %v", err)
	}

	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}
	if commits[0].Message != "Second commit" {
		t.Fatalf("expected 'Second commit', got %q", commits[0].Message)
	}
	if commits[0].SHA == "" {
		t.Fatal("expected non-empty SHA")
	}
	if commits[0].Author == "" {
		t.Fatal("expected non-empty Author")
	}
}

func TestGetRemotes(t *testing.T) {
	repoPath := newTestRepo(t)
	defer os.RemoveAll(repoPath)

	ctx := context.Background()
	adapter := NewExecAdapter(nil)
	adapter.OpenRepository(ctx, repoPath)

	// Initially no remotes.
	remotes, err := adapter.GetRemotes(ctx)
	if err != nil {
		t.Fatalf("GetRemotes: %v", err)
	}
	if len(remotes) != 0 {
		t.Fatalf("expected 0 remotes, got %d", len(remotes))
	}

	// Add a remote.
	if err := adapter.AddRemote(ctx, "origin", "https://example.com/repo.git"); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}

	remotes, _ = adapter.GetRemotes(ctx)
	if len(remotes) != 1 {
		t.Fatalf("expected 1 remote, got %d", len(remotes))
	}
	if remotes[0].Name != "origin" {
		t.Fatalf("expected 'origin', got %q", remotes[0].Name)
	}
	if remotes[0].URL != "https://example.com/repo.git" {
		t.Fatalf("expected URL, got %q", remotes[0].URL)
	}

	// Remove.
	if err := adapter.RemoveRemote(ctx, "origin"); err != nil {
		t.Fatalf("RemoveRemote: %v", err)
	}

	remotes, _ = adapter.GetRemotes(ctx)
	if len(remotes) != 0 {
		t.Fatalf("expected 0 remotes after remove, got %d", len(remotes))
	}
}

func TestGetGraph(t *testing.T) {
	repoPath := newTestRepo(t)
	defer os.RemoveAll(repoPath)

	ctx := context.Background()
	adapter := NewExecAdapter(nil)
	adapter.OpenRepository(ctx, repoPath)

	// Make another commit so we have a graph with parent relationships.
	if err := os.WriteFile(filepath.Join(repoPath, "second.txt"), []byte("second"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	exec.Command("git", "-C", repoPath, "add", ".").Run()
	exec.Command("git", "-C", repoPath, "commit", "-m", "Second commit").Run()

	nodes, err := adapter.GetGraph(ctx, "", 10)
	if err != nil {
		t.Fatalf("GetGraph: %v", err)
	}

	if len(nodes) != 2 {
		t.Fatalf("expected 2 graph nodes, got %d", len(nodes))
	}
	if len(nodes[0].Parents) != 1 {
		t.Fatalf("expected 1 parent for second commit, got %v", nodes[0].Parents)
	}
	if nodes[0].Parents[0] != nodes[1].SHA {
		t.Fatalf("expected parent SHA %q, got %q", nodes[1].SHA, nodes[0].Parents[0])
	}
}

func TestCreateCommit(t *testing.T) {
	repoPath := newTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Modify a file and stage it so there's something to commit.
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("# Updated\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx := context.Background()
	adapter := NewExecAdapter(nil)
	adapter.OpenRepository(ctx, repoPath)

	// Stage the modified file.
	_, _, err := adapter.Run(ctx, "add", "README.md")
	if err != nil {
		t.Fatalf("stage: %v", err)
	}

	sha, err := adapter.CreateCommit(ctx, "Test commit message", "")
	if err != nil {
		t.Fatalf("CreateCommit: %v", err)
	}
	if sha == "" {
		t.Fatal("expected non-empty SHA")
	}

	// Verify commit exists.
	commits, _ := adapter.GetCommitHistory(ctx, "", 5)
	if len(commits) < 1 {
		t.Fatalf("expected at least 1 commit, got %d", len(commits))
	}
	found := false
	for _, c := range commits {
		if strings.HasPrefix(c.SHA, sha) || strings.HasPrefix(sha, c.SHA) {
			found = true
			if c.Message != "Test commit message" {
				t.Fatalf("expected message 'Test commit message', got %q", c.Message)
			}
			break
		}
	}
	if !found {
		t.Fatalf("commit %q not found in history", sha)
	}
}
