package repo

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestNewPaths(t *testing.T) {
	p := NewPaths("/tmp/repo")
	if p.WorkTree != "/tmp/repo" {
		t.Errorf("WorkTree = %q", p.WorkTree)
	}
	if p.GOTDir != "/tmp/repo/.got" {
		t.Errorf("GOTDir = %q", p.GOTDir)
	}
	if p.ConfigFile != "/tmp/repo/.got/config.yaml" {
		t.Errorf("ConfigFile = %q", p.ConfigFile)
	}
	if p.DBFile != "/tmp/repo/.got/got.db" {
		t.Errorf("DBFile = %q", p.DBFile)
	}
}

func TestEnsureGOTDirCreatesAllSubdirs(t *testing.T) {
	dir := t.TempDir()
	p := NewPaths(dir)

	if err := p.EnsureGOTDir(); err != nil {
		t.Fatalf("EnsureGOTDir: %v", err)
	}

	// .got/ must exist.
	info, err := os.Stat(p.GOTDir)
	if err != nil {
		t.Fatalf("stat .got: %v", err)
	}
	if !info.IsDir() {
		t.Errorf(".got is not a directory")
	}

	// Every reserved subdir must exist.
	got := []string{}
	for _, e := range listDir(t, p.GOTDir) {
		got = append(got, e)
	}
	sort.Strings(got)
	want := append([]string{}, Subdirs...)
	sort.Strings(want)
	if !equal(got, want) {
		t.Errorf(".got subdirs = %v, want %v", got, want)
	}
}

func TestEnsureGOTDirIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	p := NewPaths(dir)
	for i := 0; i < 3; i++ {
		if err := p.EnsureGOTDir(); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	// Put a sentinel file inside plugins/ and make sure it survives.
	sentinel := filepath.Join(p.GOTDir, "plugins", "sentinel")
	if err := os.WriteFile(sentinel, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := p.EnsureGOTDir(); err != nil {
		t.Fatalf("EnsureGOTDir after sentinel: %v", err)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Errorf("sentinel was removed: %v", err)
	}
}

func TestEnsureGitignoreEntryFromScratch(t *testing.T) {
	dir := t.TempDir()
	p := NewPaths(dir)
	if err := p.EnsureGitignoreEntry(); err != nil {
		t.Fatalf("EnsureGitignoreEntry: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if string(body) != "# GOT metadata (see `got init`)\n.got/\n" {
		t.Errorf("gitignore = %q", body)
	}
}

func TestEnsureGitignoreEntryPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	original := "node_modules/\n*.log\n"
	if err := os.WriteFile(gi, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewPaths(dir)
	if err := p.EnsureGitignoreEntry(); err != nil {
		t.Fatalf("EnsureGitignoreEntry: %v", err)
	}
	got, err := os.ReadFile(gi)
	if err != nil {
		t.Fatal(err)
	}
	want := "node_modules/\n*.log\n# GOT metadata (see `got init`)\n.got/\n"
	if string(got) != want {
		t.Errorf("gitignore = %q, want %q", got, want)
	}
}

func TestEnsureGitignoreEntryIdempotent(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	// Pre-populate with .got/ in three common forms.
	original := ".got\n  .got/  # existing\n\t.got/\n"
	if err := os.WriteFile(gi, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewPaths(dir)
	if err := p.EnsureGitignoreEntry(); err != nil {
		t.Fatalf("EnsureGitignoreEntry: %v", err)
	}
	got, err := os.ReadFile(gi)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("gitignore changed:\n got: %q\nwant: %q", got, original)
	}
}

func TestEnsureGitignoreEntryNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gi, []byte("node_modules/"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewPaths(dir)
	if err := p.EnsureGitignoreEntry(); err != nil {
		t.Fatalf("EnsureGitignoreEntry: %v", err)
	}
	got, err := os.ReadFile(gi)
	if err != nil {
		t.Fatal(err)
	}
	want := "node_modules/\n# GOT metadata (see `got init`)\n.got/\n"
	if string(got) != want {
		t.Errorf("gitignore = %q, want %q", got, want)
	}
}

// listDir returns the names of entries in dir that are directories.
func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir %q: %v", dir, err)
	}
	out := []string{}
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
