package log

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionLog_NilSafe(t *testing.T) {
	var s *SessionLog
	if got := s.Records(); got != 0 {
		t.Errorf("nil Records() = %d, want 0", got)
	}
	if got := s.Bytes(); got != 0 {
		t.Errorf("nil Bytes() = %d, want 0", got)
	}
	if got := s.Rotations(); got != 0 {
		t.Errorf("nil Rotations() = %d, want 0", got)
	}
	if got := s.Summary(); got != "" {
		t.Errorf("nil Summary() = %q, want \"\"", got)
	}
}

func TestSessionLog_RecordsAndBytesViaCounter(t *testing.T) {
	var buf bytes.Buffer
	counter := NewCountingWriter(&buf)
	logger, err := Tee([]io.Writer{counter}, FormatText, "info")
	if err != nil {
		t.Fatalf("Tee: %v", err)
	}
	logger.Info("one")
	logger.Info("two")
	logger.Warn("three")
	s := &SessionLog{Path: "/tmp/x.log", Counter: counter}
	if got, want := s.Records(), int64(3); got != want {
		t.Errorf("Records() = %d, want %d", got, want)
	}
	// Bytes should be the size of the captured log content.
	if got, want := s.Bytes(), int64(buf.Len()); got != want {
		t.Errorf("Bytes() = %d, want %d (buffer length)", got, want)
	}
}

func TestSessionLog_BytesStatsPlainFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write([]byte("hello world")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	s := &SessionLog{Path: path, PlainFile: f}
	if got, want := s.Bytes(), int64(11); got != want {
		t.Errorf("Bytes() = %d, want %d (stat of file)", got, want)
	}
	if got := s.Rotations(); got != 0 {
		t.Errorf("Rotations() = %d, want 0 (no rotator)", got)
	}
}

func TestSessionLog_RotationsDelegatesToRotator(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	rotator, err := OpenRotatingFile(path, 1, 0o600, false)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = rotator.Close() }()
	counter := NewCountingWriter(rotator)
	for i, payload := range []string{"A", "B", "C", "D"} {
		if _, err := counter.Write([]byte(payload)); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	s := &SessionLog{Path: path, Counter: counter, Rotator: rotator}
	if got, want := s.Records(), int64(4); got != want {
		t.Errorf("Records() = %d, want %d", got, want)
	}
	if got, want := s.Rotations(), int64(3); got != want {
		t.Errorf("Rotations() = %d, want %d", got, want)
	}
	if got, want := s.Bytes(), int64(1); got != want {
		t.Errorf("Bytes() = %d, want %d (current file is 1 byte after 4th 1-byte write)", got, want)
	}
}

func TestSessionLog_SummaryNoRotation(t *testing.T) {
	var buf bytes.Buffer
	counter := NewCountingWriter(&buf)
	logger, err := Tee([]io.Writer{counter}, FormatText, "info")
	if err != nil {
		t.Fatalf("Tee: %v", err)
	}
	logger.Info("alpha")
	logger.Info("beta")
	s := &SessionLog{Path: "/tmp/got.log", Counter: counter}
	got := s.Summary()
	want := "Session log: 2 records"
	if !strings.HasPrefix(got, want) {
		t.Errorf("Summary() = %q, want prefix %q", got, want)
	}
	if !strings.Contains(got, "/tmp/got.log") {
		t.Errorf("Summary() = %q, must contain path", got)
	}
	if !strings.Contains(got, "written to") {
		t.Errorf("Summary() = %q, must contain 'written to'", got)
	}
	// No rotation suffix when rotation is disabled.
	if strings.Contains(got, "rotation") {
		t.Errorf("Summary() = %q, must not mention rotation when disabled", got)
	}
}

func TestSessionLog_SummaryWithRotations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	rotator, err := OpenRotatingFile(path, 1, 0o600, false)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = rotator.Close() }()
	counter := NewCountingWriter(rotator)
	for _, payload := range []string{"A", "B", "C", "D"} {
		if _, err := counter.Write([]byte(payload)); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	s := &SessionLog{Path: path, Counter: counter, Rotator: rotator}
	got := s.Summary()
	if !strings.Contains(got, "4 records") {
		t.Errorf("Summary() = %q, want '3 records'", got)
	}
	if !strings.Contains(got, "[3 rotations]") {
		t.Errorf("Summary() = %q, want '[3 rotations]'", got)
	}
	if !strings.Contains(got, path) {
		t.Errorf("Summary() = %q, want path %q", got, path)
	}
}

func TestSessionLog_SummaryWithOneRotation(t *testing.T) {
	// Singular "rotation" when count == 1.
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	rotator, err := OpenRotatingFile(path, 1, 0o600, false)
	if err != nil {
		t.Fatalf("OpenRotatingFile: %v", err)
	}
	defer func() { _ = rotator.Close() }()
	counter := NewCountingWriter(rotator)
	if _, err := counter.Write([]byte("A")); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	if _, err := counter.Write([]byte("B")); err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	s := &SessionLog{Path: path, Counter: counter, Rotator: rotator}
	got := s.Summary()
	if !strings.Contains(got, "[1 rotation]") {
		t.Errorf("Summary() = %q, want '[1 rotation]' (singular)", got)
	}
	if strings.Contains(got, "[1 rotations]") {
		t.Errorf("Summary() = %q, should use singular 'rotation' for count=1", got)
	}
}

func TestSessionLog_SummaryWithOneRecord(t *testing.T) {
	// Singular "record" when count == 1.
	var buf bytes.Buffer
	counter := NewCountingWriter(&buf)
	logger, err := Tee([]io.Writer{counter}, FormatText, "info")
	if err != nil {
		t.Fatalf("Tee: %v", err)
	}
	logger.Info("solo")
	s := &SessionLog{Path: "/tmp/got.log", Counter: counter}
	got := s.Summary()
	if !strings.Contains(got, "1 record ") {
		t.Errorf("Summary() = %q, want '1 record ' (singular, with trailing space)", got)
	}
	if strings.Contains(got, "1 records ") {
		t.Errorf("Summary() = %q, should use singular 'record' for count=1", got)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024 * 1024 * 1024, "1.00 GiB"},
		{2 * 1024 * 1024 * 1024, "2.00 GiB"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := HumanBytes(tc.n); got != tc.want {
				t.Errorf("HumanBytes(%d) = %q, want %q", tc.n, got, tc.want)
			}
		})
	}
}
