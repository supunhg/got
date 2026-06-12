package git

import (
	"context"
	"os/exec"
	"testing"
)

// runGit is a small helper that runs `git args...` in dir. It fails the
// test on any non-zero exit.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

// setupRepo creates a fresh git repo with an initial commit.
func setupRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "commit", "--allow-empty", "-m", "initial")
	return dir
}

func TestExecAdapter_StatusCleanRepo(t *testing.T) {
	dir := setupRepo(t)
	a := NewExecAdapter(dir)
	s, err := a.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Branch != "main" {
		t.Errorf("Branch = %q, want main", s.Branch)
	}
	if len(s.Entries) != 0 {
		t.Errorf("Entries = %v, want empty", s.Entries)
	}
}

func TestExecAdapter_StatusWithChanges(t *testing.T) {
	dir := setupRepo(t)
	// Create a new file (untracked) and modify a tracked file.
	if err := writeFile(dir, "tracked.txt", "v1\n"); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "tracked.txt")
	runGit(t, dir, "commit", "-m", "add tracked")
	if err := writeFile(dir, "tracked.txt", "v2\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(dir, "untracked.txt", "new\n"); err != nil {
		t.Fatal(err)
	}

	a := NewExecAdapter(dir)
	s, err := a.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Branch != "main" {
		t.Errorf("Branch = %q, want main", s.Branch)
	}
	if len(s.Entries) != 2 {
		t.Fatalf("Entries = %v, want 2 (one unstaged, one untracked)", s.Entries)
	}
	seen := map[string]bool{}
	for _, e := range s.Entries {
		seen[e.Path] = true
		if e.Path == "tracked.txt" && !(e.IsUnstaged && !e.IsStaged) {
			t.Errorf("tracked.txt: IsUnstaged=%v IsStaged=%v, want IsUnstaged=true IsStaged=false", e.IsUnstaged, e.IsStaged)
		}
		if e.Path == "untracked.txt" && !e.IsUntracked {
			t.Errorf("untracked.txt: IsUntracked=%v, want true", e.IsUntracked)
		}
	}
	if !seen["tracked.txt"] || !seen["untracked.txt"] {
		t.Errorf("expected both files in entries, got %v", seen)
	}
}

func TestExecAdapter_Branches(t *testing.T) {
	dir := setupRepo(t)
	runGit(t, dir, "checkout", "-b", "feature")
	a := NewExecAdapter(dir)
	branches, err := a.Branches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(branches) != 2 {
		t.Fatalf("branches = %v, want 2", branches)
	}
	names := map[string]bool{}
	for _, b := range branches {
		names[b.Name] = true
	}
	if !names["main"] || !names["feature"] {
		t.Errorf("branches = %v, want main and feature", branches)
	}
	// Exactly one branch should be current.
	current := 0
	for _, b := range branches {
		if b.IsCurrent {
			current++
		}
	}
	if current != 1 {
		t.Errorf("got %d current branches, want 1", current)
	}
}

func TestExecAdapter_RemotesNone(t *testing.T) {
	dir := setupRepo(t)
	a := NewExecAdapter(dir)
	remotes, err := a.Remotes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(remotes) != 0 {
		t.Errorf("remotes = %v, want empty", remotes)
	}
}

func TestExecAdapter_RemotesWithOrigin(t *testing.T) {
	dir := setupRepo(t)
	runGit(t, dir, "remote", "add", "origin", "https://example.com/foo.git")
	a := NewExecAdapter(dir)
	remotes, err := a.Remotes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(remotes) != 1 {
		t.Fatalf("remotes = %v, want 1", remotes)
	}
	r := remotes[0]
	if r.Name != "origin" {
		t.Errorf("Name = %q, want origin", r.Name)
	}
	if r.FetchURL != "https://example.com/foo.git" {
		t.Errorf("FetchURL = %q", r.FetchURL)
	}
	if r.PushURL != "https://example.com/foo.git" {
		t.Errorf("PushURL = %q", r.PushURL)
	}
}

func TestExecAdapter_CurrentRef(t *testing.T) {
	dir := setupRepo(t)
	a := NewExecAdapter(dir)
	ref, err := a.CurrentRef(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ref != "main" {
		t.Errorf("CurrentRef = %q, want main", ref)
	}
}

func TestExecAdapter_RemoteBranches(t *testing.T) {
	dir := setupRepo(t)
	// Simulate a remote-tracking branch by writing a ref under
	// refs/remotes/origin/main. This avoids needing a real network
	// fetch in unit tests.
	runGit(t, dir, "remote", "add", "origin", "https://example.com/foo.git")
	runGit(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")

	a := NewExecAdapter(dir)
	got, err := a.RemoteBranches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("RemoteBranches = %v, want 1", got)
	}
	b := got[0]
	if b.Name != "origin/main" {
		t.Errorf("Name = %q, want origin/main", b.Name)
	}
	if !b.IsRemote {
		t.Error("IsRemote = false, want true")
	}
	if b.IsCurrent {
		t.Error("IsCurrent = true on a remote-tracking branch; want false")
	}
}
