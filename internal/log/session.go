package log

import (
	"fmt"
	"os"
)

// SessionLog aggregates the runtime metrics for the on-disk log
// file (per spec §16). It is the type buildLogger returns
// alongside the *slog.Logger when --log-file is set, so main()
// can print a one-line "where did my log go" summary at command
// exit without having to re-stat the file or count records by
// hand. SessionLog is nil when --log-file is not set; methods
// are nil-safe so callers can blindly defer a summary print.
//
// Exactly one of Rotator or PlainFile is non-nil: the rotating
// case wraps the open file descriptor in a RotatingFile (so we
// can ask it for rotation count and current size atomically with
// each Write); the non-rotating case keeps a plain *os.File
// whose size is read via Stat at summary time.
type SessionLog struct {
	// Path is the on-disk file the logger is writing to. It
	// matches the --log-file flag value verbatim.
	Path string

	// Counter counts the records written to the log file.
	// Each slog record is exactly one Write call on the
	// underlying handler's writer, so Counter.Count() equals
	// the number of records in the file (including any that
	// were rotated to .1).
	Counter *CountingWriter

	// Rotator is set when --log-max-size > 0; nil otherwise.
	// Used to read rotation count and current file size.
	Rotator *RotatingFile

	// PlainFile is set when rotation is disabled; nil when
	// Rotator is set. Used to stat the file at summary time.
	PlainFile *os.File
}

// Records returns the number of records written to the log file.
// Nil-safe.
func (s *SessionLog) Records() int64 {
	if s == nil || s.Counter == nil {
		return 0
	}
	return s.Counter.Count()
}

// Bytes returns the current on-disk size of the log file in
// bytes. For the rotating case it reads from the RotatingFile's
// in-memory counter (sized atomically with each Write); for the
// non-rotating case it stats the file fresh. Nil-safe: returns 0
// for a nil receiver or a missing/statable file.
func (s *SessionLog) Bytes() int64 {
	if s == nil {
		return 0
	}
	if s.Rotator != nil {
		return s.Rotator.Size()
	}
	if s.PlainFile != nil {
		info, err := s.PlainFile.Stat()
		if err != nil {
			return 0
		}
		return info.Size()
	}
	if s.Counter != nil {
		return s.Counter.Bytes()
	}
	return 0
}

// Rotations returns the number of times the on-disk log file was
// rotated. Always zero when rotation is disabled or the writer
// has not seen a rotation yet.
func (s *SessionLog) Rotations() int64 {
	if s == nil || s.Rotator == nil {
		return 0
	}
	return s.Rotator.Rotations()
}

// Summary returns a one-line human-readable summary suitable for
// printing to stderr at command exit, e.g.:
//
//	"Session log: 47 records (12.3 KiB) written to /tmp/got.log [1 rotation]"
//	"Session log: 0 records (0 B) written to /tmp/got.log"
//
// The rotation suffix is omitted when rotation is disabled or no
// rotation has happened yet, so the no-rotation common case stays
// short. Nil-safe: returns "" for a nil receiver.
func (s *SessionLog) Summary() string {
	if s == nil {
		return ""
	}
	suffix := ""
	if s.Rotator != nil && s.Rotations() > 0 {
		suffix = fmt.Sprintf(" [%d rotation%s]", s.Rotations(), plural(int64(s.Rotations())))
	}
	return fmt.Sprintf("Session log: %d record%s (%s) written to %s%s",
		s.Records(), plural(s.Records()), HumanBytes(s.Bytes()), s.Path, suffix)
}

func plural(n int64) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// HumanBytes formats a byte count with IEC units. The base is
// 1024 and the suffixes are KiB/MiB/GiB to match the underlying
// math in cmd/got/main.go (which multiplies --log-max-size MB by
// 1024*1024). Below 1 KiB the value is rendered as plain bytes.
func HumanBytes(n int64) string {
	const (
		KiB = 1024
		MiB = 1024 * 1024
		GiB = 1024 * 1024 * 1024
	)
	switch {
	case n < KiB:
		return fmt.Sprintf("%d B", n)
	case n < MiB:
		return fmt.Sprintf("%.1f KiB", float64(n)/KiB)
	case n < GiB:
		return fmt.Sprintf("%.1f MiB", float64(n)/MiB)
	default:
		return fmt.Sprintf("%.2f GiB", float64(n)/GiB)
	}
}
