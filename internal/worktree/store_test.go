package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStore_ReadMissingYieldsEmpty(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, FileName))
	records, err := s.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("got %d records, want 0 (missing file should be empty)", len(records))
	}
}

func TestStore_WriteThenReadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, FileName))
	now := time.Now().UTC().Truncate(time.Second)
	in := []WorktreeRecord{
		{Path: "/tmp/a", Branch: "main", HEAD: "abc1234", Label: "alpha", LastAttachedAt: now},
		{Path: "/tmp/b", Branch: "feature/x", HEAD: "def5678", Editor: "code"},
	}
	if err := s.Write(in); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out, err := s.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("got %d records, want %d", len(out), len(in))
	}
	for i := range in {
		if out[i].Path != in[i].Path || out[i].Branch != in[i].Branch ||
			out[i].HEAD != in[i].HEAD || out[i].Label != in[i].Label ||
			out[i].Editor != in[i].Editor || !out[i].LastAttachedAt.Equal(in[i].LastAttachedAt) {
			t.Errorf("record %d mismatch: got %+v, want %+v", i, out[i], in[i])
		}
	}
}

func TestStore_AtomicityNoTempFileLeftover(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, FileName))
	if err := s.Write([]WorktreeRecord{{Path: "/tmp/a"}}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("temp file left over: %s", e.Name())
		}
		if e.Name() != FileName {
			t.Errorf("unexpected file: %s", e.Name())
		}
	}
}

func TestStore_FindByPath(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, FileName))
	in := []WorktreeRecord{
		{Path: "/tmp/a", Label: "alpha"},
		{Path: "/tmp/b", Label: "beta"},
	}
	if err := s.Write(in); err != nil {
		t.Fatalf("Write: %v", err)
	}
	rec, found, err := s.FindByPath("/tmp/b")
	if err != nil {
		t.Fatalf("FindByPath: %v", err)
	}
	if !found {
		t.Fatalf("expected to find /tmp/b")
	}
	if rec == nil || rec.Label != "beta" {
		t.Errorf("got %+v, want label=beta", rec)
	}
	_, found, err = s.FindByPath("/tmp/nope")
	if err != nil {
		t.Fatalf("FindByPath: %v", err)
	}
	if found {
		t.Errorf("expected not found for /tmp/nope")
	}
}

func TestStore_UpsertInsert(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, FileName))
	if err := s.Upsert("/tmp/a", func(_ *WorktreeRecord, found bool) WorktreeRecord {
		if found {
			t.Errorf("expected found=false on insert")
		}
		return WorktreeRecord{Path: "/tmp/a", Label: "new"}
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	records, _ := s.Read()
	if len(records) != 1 || records[0].Label != "new" {
		t.Errorf("got %+v, want one record with label=new", records)
	}
}

func TestStore_UpsertUpdate(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, FileName))
	if err := s.Write([]WorktreeRecord{{Path: "/tmp/a", Label: "old"}}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := s.Upsert("/tmp/a", func(existing *WorktreeRecord, found bool) WorktreeRecord {
		if !found {
			t.Errorf("expected found=true on update")
		}
		existing.Label = "updated"
		return *existing
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	records, _ := s.Read()
	if len(records) != 1 || records[0].Label != "updated" {
		t.Errorf("got %+v, want one record with label=updated", records)
	}
}

func TestStore_Remove(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, FileName))
	if err := s.Write([]WorktreeRecord{
		{Path: "/tmp/a"},
		{Path: "/tmp/b"},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	removed, err := s.Remove("/tmp/a")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !removed {
		t.Errorf("Remove returned removed=false, want true")
	}
	records, _ := s.Read()
	if len(records) != 1 || records[0].Path != "/tmp/b" {
		t.Errorf("got %+v, want one record at /tmp/b", records)
	}
	// Removing a non-existent path is not an error.
	removed, err = s.Remove("/tmp/nope")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if removed {
		t.Errorf("Remove on missing path returned removed=true, want false")
	}
}

func TestStore_FileVersionAndPrettyPrinted(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, FileName))
	if err := s.Write([]WorktreeRecord{{Path: "/tmp/a"}}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	body, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(body), "\"version\": "+itoa(FileVersion)) {
		t.Errorf("expected version %d in body, got:\n%s", FileVersion, body)
	}
	if !strings.Contains(string(body), "\n  \"records\":") {
		t.Errorf("expected pretty-printed (indented) records, got:\n%s", body)
	}
	if !strings.HasSuffix(string(body), "\n") {
		t.Errorf("expected trailing newline, got:\n%q", body)
	}
}

// itoa is a tiny local helper so the test file doesn't pull in
// strconv just for one constant.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := false
	if n < 0 {
		negative = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
