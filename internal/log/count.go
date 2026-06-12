package log

import (
	"fmt"
	"io"
	"sync"
)

// CountingWriter wraps an io.Writer and counts the number of
// successful Write calls. A "record" in slog maps to exactly one
// Write call on the underlying handler's writer, so the count
// equals the number of log records written through this writer.
//
// The counter is concurrency-safe so it can be used from the
// Tee's MultiWriter without serializing the logger itself.
// CountingWriter is the building block for the post-exit "session
// log" summary: buildLogger wraps the on-disk file writer with one
// of these so main() can print "47 records written to /tmp/got.log"
// at command exit.
type CountingWriter struct {
	mu    sync.Mutex
	n     int64
	bytes int64
	w     io.Writer
}

// NewCountingWriter returns a CountingWriter wrapping w. A nil w
// is allowed for symmetry with the other writers in this package
// (Tee rejects nil elements up front, so the CLI's --log-file path
// never produces a nil here); if a nil w somehow reaches Write, the
// call returns an error rather than panicking.
func NewCountingWriter(w io.Writer) *CountingWriter {
	return &CountingWriter{w: w}
}

// Write writes p to the underlying writer and, on any non-zero
// successful byte count, increments the record counter. A zero-byte
// write (len(p) == 0) is treated as a no-op by the slog handlers
// themselves, so we don't count those.
func (c *CountingWriter) Write(p []byte) (int, error) {
	if c == nil {
		return 0, fmt.Errorf("log: CountingWriter.Write on nil receiver")
	}
	if c.w == nil {
		return 0, fmt.Errorf("log: CountingWriter underlying writer is nil")
	}
	n, err := c.w.Write(p)
	if n > 0 {
		c.mu.Lock()
		c.n++
		c.bytes += int64(n)
		c.mu.Unlock()
	}
	return n, err
}

// Count returns the number of successful (non-zero byte) Write
// calls observed so far. Safe to call concurrently with Write.
func (c *CountingWriter) Count() int64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// Ensure CountingWriter satisfies io.Writer at compile time.
// Bytes returns the total number of bytes successfully
// written through this CountingWriter. Mirrors Count() but
// reports bytes instead of records. Safe to call concurrently
// with Write.
func (c *CountingWriter) Bytes() int64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bytes
}

var _ io.Writer = (*CountingWriter)(nil)
