// Package log configures a *slog.Logger for the got CLI per spec
// §16. The package is intentionally small: it builds a handler from
// the user-facing --log-level / --log-format flags, picks a default
// level based on the TTY state, and exposes a discard logger for the
// TUI (which must never write to stderr).
//
// The TUI (got tui / interactive dashboards) never writes to
// stderr; if a user wants to see the underlying slog output, they
// run a non-interactive command (got status, got remote fetch, etc.)
// with --log-level=debug. Per the spec, the in-app log view reachable
// via ctrl+l is owned by the dashboard model, not this package.
package log

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// FormatText and FormatJSON name the two output formats. The CLI's
// --log-format flag accepts either (case-insensitive). An empty
// string is treated as FormatText.
const (
	FormatText = "text"
	FormatJSON = "json"
)

// ModeInteractive and ModeNonInteractive label the two startup modes
// the spec distinguishes. DefaultLevel uses them to pick warn vs
// info.
const (
	ModeInteractive    = "interactive"
	ModeNonInteractive = "non-interactive"
)

// New returns a *slog.Logger writing structured records to w.
// format is "text" or "json" (case-insensitive). level is one of
// "debug", "info", "warn", "error" (case-insensitive; "warning" is
// accepted as an alias for "warn"). An empty level is rejected —
// callers should resolve the default via DefaultLevel before
// calling.
func New(w io.Writer, format, level string) (*slog.Logger, error) {
	if w == nil {
		return nil, fmt.Errorf("log: writer must not be nil")
	}
	lvl, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", FormatText:
		h = slog.NewTextHandler(w, opts)
	case FormatJSON:
		h = slog.NewJSONHandler(w, opts)
	default:
		return nil, fmt.Errorf("log: unknown format %q (want text|json)", format)
	}
	return slog.New(h), nil
}

// Tee returns a *slog.Logger that writes every record to every
// writer in writers, using the same format and level for all of
// them. This is the "log to both stderr and a file" case per spec
// §16: when --log-file is set, the user wants a full session log
// on disk while still seeing the chatter live in the terminal.
//
// A nil or empty writers slice is rejected. A single-element slice
// delegates to New (no MultiWriter overhead). A nil element in
// the slice is rejected up front so the failure is loud rather
// than appearing as a broken write later.
func Tee(writers []io.Writer, format, level string) (*slog.Logger, error) {
	if len(writers) == 0 {
		return nil, fmt.Errorf("log: Tee requires at least one writer")
	}
	for i, w := range writers {
		if w == nil {
			return nil, fmt.Errorf("log: Tee writers[%d] is nil", i)
		}
	}
	if len(writers) == 1 {
		return New(writers[0], format, level)
	}
	return New(io.MultiWriter(writers...), format, level)
}

// Discard returns a *slog.Logger that drops every record. It is
// the logger the TUI uses so the dashboard can never write to
// stderr. The slog.NewTextHandler(io.Discard, nil) form uses the
// default LevelInfo, so debug records are silently dropped before
// the writer is even consulted.
func Discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// DefaultLevel returns the spec §16 default log level for the given
// startup mode. Interactive mode (a TTY) defaults to "warn" so the
// user isn't interrupted by chatter; non-interactive mode (no TTY or
// --no-tui) defaults to "info" so CI / scripts see what the command
// did. An empty mode defaults to interactive.
func DefaultLevel(mode string) string {
	switch mode {
	case ModeNonInteractive:
		return "info"
	default:
		return "warn"
	}
}

// ParseLevel maps a human level string to slog.Level. The level is
// case-insensitive; surrounding whitespace is tolerated. An empty
// string returns an error — callers must resolve a default first
// so the spec §16 contract is preserved.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("log: unknown level %q (want debug|info|warn|error)", s)
	}
}
