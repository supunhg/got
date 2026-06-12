package log

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRotatingFile_AppendsToExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	// Pre-populate the file with some content so the writer's
	// size accounting has to honor the existing bytes.
	if err := os.WriteFile(path, []byte("pre-existing\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r, err := OpenRotatingFile(path, 0, 0o600) // 0 = no rotation
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = r.Close() }()
	if _, err := r.Write([]byte("hello\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "pre-existing\nhello\n" {
		t.Errorf("file content = %q, want %q", got, "pre-existing\nhello\n")
	}
}

func TestRotatingFile_RotatesWhenSizeExceeded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	// maxBytes is small enough that the second write triggers
	// rotation. The size check is "bytes + len(p) > maxBytes"
	// so we want the first write to fit and the second to push
	// past.
	r, err := OpenRotatingFile(path, 5, 0o600)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = r.Close() }()

	// First write: 5 bytes, exactly at the threshold (allowed).
	if _, err := r.Write([]byte("12345")); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	// Second write: 1 byte, would push size to 6 > 5, so rotate.
	if _, err := r.Write([]byte("X")); err != nil {
		t.Fatalf("Write 2: %v", err)
	}

	// Current file should contain only the second write.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "X" {
		t.Errorf("current file = %q, want %q", got, "X")
	}
	// Backup should contain the first write.
	backup, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("ReadFile backup: %v", err)
	}
	if string(backup) != "12345" {
		t.Errorf("backup file = %q, want %q", backup, "12345")
	}
}

func TestRotatingFile_MultipleRotationsOverwriteBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	r, err := OpenRotatingFile(path, 1, 0o600)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = r.Close() }()

	// Three rotations: each write pushes the size past the
	// threshold so the previous content is moved to .1.
	for i, payload := range []string{"AAA", "BBB", "CCC"} {
		if _, err := r.Write([]byte(payload)); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	// After all three writes, the current file holds only the
	// last one and the .1 holds the second-to-last (the first
	// was overwritten by the second rotation, the second by the
	// third).
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "CCC" {
		t.Errorf("current file = %q, want %q", got, "CCC")
	}
	backup, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("ReadFile backup: %v", err)
	}
	if string(backup) != "BBB" {
		t.Errorf("backup file = %q, want %q (AAA was rotated over by the second rotation)", backup, "BBB")
	}
}

func TestRotatingFile_EmptyWriteIsNoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	r, err := OpenRotatingFile(path, 100, 0o600)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = r.Close() }()
	n, err := r.Write(nil)
	if n != 0 || err != nil {
		t.Errorf("Write(nil) = (%d, %v), want (0, nil)", n, err)
	}
	n, err = r.Write([]byte{})
	if n != 0 || err != nil {
		t.Errorf("Write([]byte{}) = (%d, %v), want (0, nil)", n, err)
	}
}

func TestRotatingFile_ConcurrentWritesDoNotPanic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	// Small maxBytes forces many rotations under concurrent
	// writers; the mutex guarantees exactly-one rotation per
	// threshold crossing.
	r, err := OpenRotatingFile(path, 64, 0o600)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = r.Close() }()

	var wg sync.WaitGroup
	const goroutines = 8
	const writesPer = 50
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPer; j++ {
				_, _ = r.Write([]byte("x"))
			}
		}(i)
	}
	wg.Wait()
	// We can't assert exact file content (concurrent writes
	// interleave at the OS level) but the writer must not have
	// panicked and the file must exist.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("current file missing after concurrent writes: %v", err)
	}
}

func TestRotatingFile_CloseMakesWriteError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	r, err := OpenRotatingFile(path, 0, 0o600)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err = r.Write([]byte("x"))
	if err == nil {
		t.Error("expected error writing to closed RotatingFile, got nil")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("expected 'closed' in error, got: %v", err)
	}
}

func TestRotatingFile_CloseIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	r, err := OpenRotatingFile(path, 0, 0o600)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Errorf("second Close: %v (want nil for idempotent close)", err)
	}
}

func TestRotatingFile_PathReturnsConfiguredPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	r, err := OpenRotatingFile(path, 0, 0o600)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = r.Close() }()
	if r.Path() != path {
		t.Errorf("Path() = %q, want %q", r.Path(), path)
	}
}

func TestOpenRotatingFile_RejectsEmptyPath(t *testing.T) {
	if _, err := OpenRotatingFile("", 100, 0o600); err == nil {
		t.Error("expected error for empty path")
	}
}

func TestOpenRotatingFile_RejectsNegativeMaxBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if _, err := OpenRotatingFile(path, -1, 0o600); err == nil {
		t.Error("expected error for negative maxBytes")
	}
}

func TestRotatingFile_RotationsStartsAtZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	r, err := OpenRotatingFile(path, 0, 0o600)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = r.Close() }()
	if got := r.Rotations(); got != 0 {
		t.Errorf("Rotations() = %d, want 0 (no rotations yet)", got)
	}
}

func TestRotatingFile_RotationsCount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	r, err := OpenRotatingFile(path, 1, 0o600)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = r.Close() }()
	// Three writes, each crosses the threshold, so three rotations.
	// With 1-byte writes and maxBytes=1, the first write
	// lands at exactly the threshold (no rotation), and each
	// of the next three writes triggers a rotation. Total: 3.
	for i, payload := range []string{"A", "B", "C", "D"} {
		if _, err := r.Write([]byte(payload)); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	if got, want := r.Rotations(), int64(3); got != want {
		t.Errorf("Rotations() = %d, want %d", got, want)
	}
}

func TestRotatingFile_SizeTracksBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	// Pre-populate so the writer's accounting has to honor the
	// existing bytes.
	if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r, err := OpenRotatingFile(path, 0, 0o600) // 0 = no rotation
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = r.Close() }()
	if got, want := r.Size(), int64(5); got != want {
		t.Errorf("Size() after seed = %d, want %d", got, want)
	}
	if _, err := r.Write([]byte("678")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got, want := r.Size(), int64(8); got != want {
		t.Errorf("Size() after Write = %d, want %d", got, want)
	}
}

func TestRotatingFile_SizeResetsAfterRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	r, err := OpenRotatingFile(path, 5, 0o600)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = r.Close() }()
	// First write: 5 bytes, exactly at the threshold.
	if _, err := r.Write([]byte("12345")); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	if got, want := r.Size(), int64(5); got != want {
		t.Errorf("Size() pre-rotation = %d, want %d", got, want)
	}
	// Second write: 1 byte, would push size to 6 > 5, rotates.
	if _, err := r.Write([]byte("X")); err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	// After rotation, current file holds only "X" (1 byte).
	if got, want := r.Size(), int64(1); got != want {
		t.Errorf("Size() post-rotation = %d, want %d", got, want)
	}
	if got, want := r.Rotations(), int64(1); got != want {
		t.Errorf("Rotations() = %d, want %d", got, want)
	}
}

func TestRotatingFile_NilReceiverReturnsZero(t *testing.T) {
	var r *RotatingFile
	if got := r.Size(); got != 0 {
		t.Errorf("nil Size() = %d, want 0", got)
	}
	if got := r.Rotations(); got != 0 {
		t.Errorf("nil Rotations() = %d, want 0", got)
	}
	if got := r.Path(); got != "" {
		t.Errorf("nil Path() = %q, want \"\"", got)
	}
}

func TestRotatingFile_IntegratesWithTee(t *testing.T) {
	// End-to-end: the rotating file is used as one of the
	// writers in a Tee logger, records show up in both the
	// rotating file and a buffer.
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	r, err := OpenRotatingFile(path, 50, 0o600)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = r.Close() }()
	var buf bytes.Buffer
	logger, err := Tee([]io.Writer{r, &buf}, FormatText, "info")
	if err != nil {
		t.Fatalf("Tee: %v", err)
	}
	logger.Info("hello", "k", "v")
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("buffer missing 'hello': %q", buf.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(got), "hello") {
		t.Errorf("rotating file missing 'hello': %q", got)
	}
}

func TestRotatingFile_CreatesFileWithCustomMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	r, err := OpenRotatingFile(path, 0, 0o640)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = r.Close() }()
	if _, err := r.Write([]byte("hello\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Errorf("file perms = %o, want 0640", got)
	}
}

func TestRotatingFile_DefaultModeIs600(t *testing.T) {
	// Sanity check: passing 0o600 (the default cmd/got/main.go
	// uses) creates a 0600 file.
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	r, err := OpenRotatingFile(path, 0, 0o600)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = r.Close() }()
	if _, err := r.Write([]byte("x")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("file perms = %o, want 0600", got)
	}
}

func TestRotatingFile_RotationPreservesMode(t *testing.T) {
	// When the file rotates, the fresh file should be created
	// with the same mode the original was opened with.
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	r, err := OpenRotatingFile(path, 3, 0o640)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = r.Close() }()
	// 0+3=3 > 3? No. 3+3=6 > 3? Yes. Rotate.
	if _, err := r.Write([]byte("AAA")); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	if _, err := r.Write([]byte("BBB")); err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat current: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Errorf("post-rotation file perms = %o, want 0640", got)
	}
	backupInfo, err := os.Stat(path + ".1")
	if err != nil {
		t.Fatalf("Stat backup: %v", err)
	}
	if got := backupInfo.Mode().Perm(); got != 0o640 {
		t.Errorf("backup file perms = %o, want 0640 (rename preserves perms)", got)
	}
}

func TestOpenRotatingFile_MasksNonPermissionBits(t *testing.T) {
	// The setuid/setgid/sticky bits should be stripped from
	// the mode passed to OpenRotatingFile so a flag typo
	// cannot accidentally create a setuid log file.
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	// 0o7700 = setuid+setgid+sticky + 0700. Should be masked to 0o700.
	r, err := OpenRotatingFile(path, 0, 0o7700)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = r.Close() }()
	if _, err := r.Write([]byte("x")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("file perms = %o, want 0700 (upper bits masked)", got)
	}
}
