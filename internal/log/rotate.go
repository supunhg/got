package log

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// RotatingFile is an io.Writer that appends to a file on disk and
// rotates the file when its size exceeds a threshold. The
// rotation is size-based (per spec §16): when the file would
// grow past maxBytes, the existing file is renamed to path+".1"
// (overwriting any previous .1) and a fresh file is opened at
// path. The writer is concurrency-safe: a single mutex serializes
// the size check, the rotate, and the write so two goroutines
// can't race on the rotate decision.
//
// The zero value is invalid; construct one via OpenRotatingFile.
//
// Rotation policy: only one backup is kept (<path>.1). Older
// backups are not retained. This is the simplest policy that
// keeps a single recent "what just happened" file around and is
// appropriate for a short-lived CLI invocation. Long-running
// daemons would want a higher rotation count (path.1, path.2,
// ...); that is intentionally not in scope for v0.1.
type RotatingFile struct {
	path     string
	maxBytes int64
	mu       sync.Mutex
	f        *os.File
	bytes    int64
}

// OpenRotatingFile opens (or creates) path for append and returns
// a *RotatingFile that rotates the file when its size exceeds
// maxBytes. A maxBytes of 0 or less disables rotation (the writer
// behaves as a plain append-only file). A maxBytes of 1 or more
// enables rotation; the file is rotated before any write that
// would push its size past the threshold.
//
// The file is created with mode 0600 (user-only read/write) so
// the log file is not world-readable, matching the --log-file
// perms set by cmd/got/main.go for the non-rotating path.
//
// The returned RotatingFile holds an open file descriptor. The
// caller must call Close to release it. For short-lived CLI
// processes the OS reclaims the fd on exit, so the documented
// pattern (matching the non-rotating --log-file path) is to
// intentionally not close it and let the process exit clean it
// up. Close is still exposed for tests and for callers that want
// the explicit lifecycle.
func OpenRotatingFile(path string, maxBytes int64) (*RotatingFile, error) {
	if path == "" {
		return nil, fmt.Errorf("log: RotatingFile path must not be empty")
	}
	if maxBytes < 0 {
		return nil, fmt.Errorf("log: RotatingFile maxBytes must be >= 0 (got %d)", maxBytes)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("log: open rotating file %q: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("log: stat rotating file %q: %w", path, err)
	}
	return &RotatingFile{
		path:     path,
		maxBytes: maxBytes,
		f:        f,
		bytes:    info.Size(),
	}, nil
}

// Write implements io.Writer. It first checks whether adding p
// would push the file past maxBytes; if so, it rotates before
// writing. The rotate decision is made under the mutex so two
// concurrent writers can't both decide to rotate and clobber
// each other.
//
// If maxBytes is 0 (rotation disabled), Write is a passthrough to
// the underlying file.
func (r *RotatingFile) Write(p []byte) (int, error) {
	if r == nil {
		return 0, fmt.Errorf("log: RotatingFile.Write on nil receiver")
	}
	if len(p) == 0 {
		return 0, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return 0, fmt.Errorf("log: RotatingFile %q is closed", r.path)
	}
	// Decide whether to rotate BEFORE writing. The size check
	// uses >= so a write that would land exactly on the boundary
	// triggers rotation (avoiding edge cases where the file
	// sits at maxBytes for the rest of the run).
	if r.maxBytes > 0 && r.bytes+int64(len(p)) > r.maxBytes {
		if err := r.rotateLocked(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.bytes += int64(n)
	return n, err
}

// Close flushes and closes the underlying file. After Close the
// RotatingFile is unusable; further Write calls return an error.
// Close is safe to call multiple times; only the first call
// closes the fd.
func (r *RotatingFile) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}

// Path returns the file path the RotatingFile writes to. Useful
// for tests and for the post-exit summary the CLI may want to
// print when --log-file is set.
func (r *RotatingFile) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}

// rotateLocked performs the rename-and-reopen dance. Caller must
// hold r.mu. The existing fd is closed, the file is renamed to
// path+".1" (overwriting any previous .1), and a fresh fd is
// opened at path.
func (r *RotatingFile) rotateLocked() error {
	if err := r.f.Close(); err != nil {
		return fmt.Errorf("log: close before rotate: %w", err)
	}
	r.f = nil
	backup := r.path + ".1"
	// os.Rename overwrites the destination on POSIX, which is
	// the behavior we want: the previous .1 is replaced.
	if err := os.Rename(r.path, backup); err != nil {
		// If rename fails (e.g. the source doesn't exist for
		// some reason), try to reopen the original file so the
		// writer isn't permanently broken.
		f, openErr := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if openErr != nil {
			return fmt.Errorf("log: rename %s -> %s failed (%v) and reopen failed (%v)", r.path, backup, err, openErr)
		}
		r.f = f
		return fmt.Errorf("log: rename %s -> %s: %w", r.path, backup, err)
	}
	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("log: reopen %s after rotate: %w", r.path, err)
	}
	r.f = f
	r.bytes = 0
	return nil
}

// Ensure RotatingFile satisfies io.Writer at compile time.
var _ io.Writer = (*RotatingFile)(nil)
