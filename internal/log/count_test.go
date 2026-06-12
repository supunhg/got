package log

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestCountingWriter_CountsSuccessfulWrites(t *testing.T) {
	var buf bytes.Buffer
	c := NewCountingWriter(&buf)
	for i := 0; i < 5; i++ {
		if _, err := c.Write([]byte("hello\n")); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	if got, want := c.Count(), int64(5); got != want {
		t.Errorf("Count() = %d, want %d", got, want)
	}
	if got := buf.String(); got != "hello\nhello\nhello\nhello\nhello\n" {
		t.Errorf("buffer = %q, want 5x hello\\n", got)
	}
}

func TestCountingWriter_EmptyWriteIsNoCount(t *testing.T) {
	var buf bytes.Buffer
	c := NewCountingWriter(&buf)
	// slog handlers themselves short-circuit zero-byte writes,
	// but if one sneaks through, the counter should not
	// inflate from it.
	if _, err := c.Write(nil); err != nil {
		t.Fatalf("Write(nil): %v", err)
	}
	if _, err := c.Write([]byte{}); err != nil {
		t.Fatalf("Write([]byte{}): %v", err)
	}
	if got := c.Count(); got != 0 {
		t.Errorf("Count() = %d, want 0 (empty writes don't count as records)", got)
	}
}

func TestCountingWriter_NilUnderlyingWriterErrors(t *testing.T) {
	c := NewCountingWriter(nil)
	n, err := c.Write([]byte("x"))
	if n != 0 {
		t.Errorf("Write n = %d, want 0", n)
	}
	if err == nil {
		t.Error("expected error writing through nil underlying writer, got nil")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("error %q should mention 'nil'", err.Error())
	}
}

func TestCountingWriter_NilReceiverErrors(t *testing.T) {
	var c *CountingWriter
	n, err := c.Write([]byte("x"))
	if n != 0 || err == nil {
		t.Errorf("nil-receiver Write = (%d, %v), want (0, error)", n, err)
	}
	if c.Count() != 0 {
		t.Errorf("nil-receiver Count() = %d, want 0", c.Count())
	}
}

func TestCountingWriter_PropagatesUnderlyingError(t *testing.T) {
	// errWriter returns an error on every Write. The
	// CountingWriter should propagate the error and the byte
	// count returned by the underlying writer, and should NOT
	// increment the counter when zero bytes were written.
	ew := &errWriter{err: errors.New("boom")}
	c := NewCountingWriter(ew)
	n, err := c.Write([]byte("hello"))
	if !errors.Is(err, ew.err) {
		t.Errorf("Write err = %v, want %v", err, ew.err)
	}
	if n != 0 {
		t.Errorf("Write n = %d, want 0 (underlying returned 0, err)", n)
	}
	if got := c.Count(); got != 0 {
		t.Errorf("Count() = %d, want 0 (failed writes don't count)", got)
	}
}

func TestCountingWriter_PartialWriteCountsOnce(t *testing.T) {
	// A Write that returns a partial byte count (some bytes
	// written, then error) should count exactly once: the
	// record made it onto the wire even if it was truncated.
	pw := &partialWriter{first: 3, total: 5, err: io.ErrShortWrite}
	c := NewCountingWriter(pw)
	_, _ = c.Write([]byte("hello"))
	if got := c.Count(); got != 1 {
		t.Errorf("Count() = %d, want 1 (partial writes still count as one record)", got)
	}
}

func TestCountingWriter_ConcurrentWritesCount(t *testing.T) {
	// 8 goroutines each write 100 records; the counter must
	// see all 800 (concurrency-safe accounting is the whole
	// point of the mutex). The underlying writer is wrapped
	// in a safeBuffer because bytes.Buffer is NOT
	// concurrency-safe and the race detector would catch
	// concurrent Writes to the buffer itself — that race
	// belongs to the test's chosen writer, not to
	// CountingWriter, so we guard it here.
	buf := &safeBuffer{}
	c := NewCountingWriter(buf)
	var wg sync.WaitGroup
	const goroutines = 8
	const writesPer = 100
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < writesPer; j++ {
				_, _ = c.Write([]byte("x"))
			}
		}()
	}
	wg.Wait()
	if got, want := c.Count(), int64(goroutines*writesPer); got != want {
		t.Errorf("Count() = %d, want %d", got, want)
	}
}

func TestCountingWriter_SatisfiesIOWriter(t *testing.T) {
	// Compile-time assertion is already in count.go, but a
	// runtime check gives a nicer error if someone breaks it.
	var _ io.Writer = NewCountingWriter(&bytes.Buffer{})
}

// errWriter is an io.Writer that always returns its configured
// error and a zero byte count.
type errWriter struct{ err error }

func (e *errWriter) Write(_ []byte) (int, error) { return 0, e.err }

// safeBuffer is a concurrency-safe wrapper around bytes.Buffer.
// It exists so the concurrent-write test can exercise CountingWriter
// without triggering the race detector on the underlying buffer's
// own non-atomic Write method. The mutex is held for the duration
// of Write, which is fine for the test's purpose (proving the
// counter is race-free) and matches the "count records, not
// bytes" semantic the test cares about.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

// partialWriter returns `first` bytes successfully, then reports
// `total-first` as a short write with `err`.
type partialWriter struct {
	first, total int
	err          error
}

func (p *partialWriter) Write(b []byte) (int, error) {
	if len(b) >= p.first {
		return p.first, nil
	}
	return p.first, p.err
}
