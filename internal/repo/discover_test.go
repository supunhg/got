package repo

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/got-sh/got/internal/gerr"
)

func TestDiscover_NotInGitRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := Discover(dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := gerr.ExitCode(err); got != int(gerr.CodeNotInGitRepo) {
		t.Errorf("ExitCode = %d, want %d", got, gerr.CodeNotInGitRepo)
	}
}

func TestDiscover_InRepo(t *testing.T) {
	dir := initRepo(t)
	got, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Errorf("got %q, want %q", got, dir)
	}
}

func TestDiscover_Subdir(t *testing.T) {
	dir := initRepo(t)
	sub := filepath.Join(dir, "a", "b", "c")
	if err := mkdirAll(sub); err != nil {
		t.Fatal(err)
	}
	got, err := Discover(sub)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Errorf("got %q, want %q", got, dir)
	}
}

func TestOpen_HasGOTDir(t *testing.T) {
	dir := initRepo(t)
	if err := mkdirAll(filepath.Join(dir, ".got")); err != nil {
		t.Fatal(err)
	}
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.HasGOTDir() {
		t.Error("HasGOTDir = false, want true")
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmds := [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "--allow-empty", "-m", "initial"},
	}
	for _, args := range cmds {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func mkdirAll(p string) error {
	return exec.Command("mkdir", "-p", p).Run()
}
