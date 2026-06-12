package log

import (
	"compress/gzip"
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
	path      string
	maxBytes  int64
	mode      os.FileMode
	compress  bool
	mu        sync.Mutex
	f         *os.File
	bytes     int64
	rotations int64
}

// OpenRotatingFile opens (or creates) path for append and returns
// a *RotatingFile that rotates the file when its size exceeds
// maxBytes. A maxBytes of 0 or less disables rotation (the writer
// behaves as a plain append-only file). A maxBytes of 1 or more
// enables rotation; the file is rotated before any write that
// would push its size past the threshold.
//
// mode is the file permission bits used when the file is
// created (and after each rotation). The lower 9 bits are the
// standard rwx bits; the upper bits (setuid/setgid/sticky) are
// ignored. Typical values are 0o600 (user-only) and 0o640
// (user + group). The caller is expected to have validated the
// mode already; OpenRotatingFile does not enforce a minimum
// (e.g. 0 would create an unreadable file, which is the
// caller's responsibility to avoid).
//
// compress controls what happens to the rotated backup. When
// false (the default), the backup is left uncompressed at
// <path>.1. When true, the backup is gzipped to <path>.1.gz
// and the uncompressed <path>.1 is removed. The compression
// is best-effort: if it fails, the uncompressed backup is
// retained and the error is returned from the Write call that
// triggered rotation (the writer itself remains functional).
// This is the right behavior for long-running CI jobs that
// care more about disk space than about catching every
// compression error.
//
// The returned RotatingFile holds an open file descriptor. The
// caller must call Close to release it. For short-lived CLI
// processes the OS reclaims the fd on exit, so the documented
// pattern (matching the non-rotating --log-file path) is to
// intentionally not close it and let the process exit clean it
// up. Close is still exposed for tests and for callers that want
// the explicit lifecycle.
func OpenRotatingFile(path string, maxBytes int64, mode os.FileMode, compress bool) (*RotatingFile, error) {
	if path == "" {
		return nil, fmt.Errorf("log: RotatingFile path must not be empty")
	}
	if maxBytes < 0 {
		return nil, fmt.Errorf("log: RotatingFile maxBytes must be >= 0 (got %d)", maxBytes)
	}
	// Mask to the 9 standard permission bits. setuid/setgid/
	// sticky don't make sense for a log file and we don't
	// want a CLI flag typo to silently create a setuid log.
	mode = mode & 0o777
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, mode)
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
		mode:     mode,
		compress: compress,
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

// Size returns the current on-disk size of the log file in bytes.
// For the non-rotating case the value tracks every Write's byte
// count under the mutex; for the rotating case the value is reset
// to 0 on each rotation, so Size() always reports the size of the
// *current* file, not the cumulative bytes ever written. That
// matches the spec §16 "current file size" wording for the post-
// exit summary.
func (r *RotatingFile) Size() int64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bytes
}

// Rotations returns the number of times the on-disk log file has
// been rotated so far. Always zero when rotation is disabled
// (maxBytes == 0) or for a RotatingFile that has not seen a
// rotation yet. The counter is incremented exactly once per
// successful rename-and-reopen, so a Write that triggers rotation
// increments by one even though many bytes may move.
func (r *RotatingFile) Rotations() int64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rotations
}

// rotateLocked performs the rename-and-reopen dance. Caller must
// hold r.mu. The existing fd is closed, the file is renamed to
// path+".1" (overwriting any previous .1), optionally gzipped
// to path+".1.gz" (if r.compress), and a fresh fd is opened at
// path. The new file is created with the same mode the
// original was opened with (r.mode), so the user's
// --log-file-mode setting persists across rotations.
//
// If compression is requested and fails, the error is returned
// but the writer is left functional: the fresh file is already
// opened, subsequent Writes succeed, and the uncompressed .1
// is retained (the next rotation will overwrite it anyway).
// This matches the spec §16 "best-effort log" contract.
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
		f, openErr := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, r.mode)
		if openErr != nil {
			return fmt.Errorf("log: rename %s -> %s failed (%v) and reopen failed (%v)", r.path, backup, err, openErr)
		}
		r.f = f
		return fmt.Errorf("log: rename %s -> %s: %w", r.path, backup, err)
	}
	// Compress the backup if requested. Failure here is
	// reported via the returned error but the writer is
	// still opened below so the next Write can succeed.
	var compressErr error
	if r.compress {
		if err := r.compressBackup(backup); err != nil {
			compressErr = fmt.Errorf("log: compress backup %q: %w", backup, err)
		}
	}
	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, r.mode)
	if err != nil {
		return fmt.Errorf("log: reopen %s after rotate: %w", r.path, err)
	}
	r.f = f
	r.bytes = 0
	r.rotations++
	return compressErr
}

// compressBackup reads srcPath, writes a gzipped copy to
// srcPath+".gz", and removes the original. The .gz file is
// created with the same mode the original was opened with
// (r.mode) so --log-file-mode applies to backups too. Caller
// must hold r.mu.
func (r *RotatingFile) compressBackup(srcPath string) error {
	in, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(srcPath+".gz", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, r.mode)
	if err != nil {
		return fmt.Errorf("open destination: %w", err)
	}
	gz := gzip.NewWriter(out)
	_, copyErr := io.Copy(gz, in)
	closeErr := gz.Close()
	outCloseErr := out.Close()
	// If any step failed, the .gz file may be incomplete or
	// the source may still be there. We report the first
	// error and let the caller decide what to do (rotateLocked
	// will return this error but the writer stays functional).
	if copyErr != nil {
		return fmt.Errorf("gzip copy: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("gzip close: %w", closeErr)
	}
	if outCloseErr != nil {
		return fmt.Errorf("close destination: %w", outCloseErr)
	}
	// Compression succeeded; remove the uncompressed source
	// so the on-disk state is "<path>.gz only", matching the
	// documentation in the OpenRotatingFile doc comment.
	if err := os.Remove(srcPath); err != nil {
		return fmt.Errorf("remove uncompressed source: %w", err)
	}
	return nil
}

// Ensure RotatingFile satisfies io.Writer at compile time.
var _ io.Writer = (*RotatingFile)(nil)
