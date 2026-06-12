// log_test.go: tests for the slog-backed logging helpers per
// spec §16. Each test exercises a single contract: New builds the
// right handler, ParseLevel maps the documented strings, and
// DefaultLevel matches the interactive / non-interactive split.
package log

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestNew_TextHandler_RecordsAtLevel(t *testing.T) {
	var buf bytes.Buffer
	logger, err := New(&buf, FormatText, "info")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.Info("hello", "k", "v")
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("expected 'hello' in output, got: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "k=v") {
		t.Errorf("expected attribute k=v in text output, got: %q", buf.String())
	}
}

func TestNew_JSONHandler_RecordsJSON(t *testing.T) {
	var buf bytes.Buffer
	logger, err := New(&buf, FormatJSON, "info")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.Info("hello", "k", "v")
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\nout=%s", err, buf.String())
	}
	if got["msg"] != "hello" {
		t.Errorf("msg = %v, want hello", got["msg"])
	}
	if got["k"] != "v" {
		t.Errorf("k = %v, want v", got["k"])
	}
	if got["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", got["level"])
	}
}

func TestNew_LevelFiltersRecords(t *testing.T) {
	cases := []struct {
		level   string
		debugOK bool
		infoOK  bool
		warnOK  bool
		errOK   bool
	}{
		{"debug", true, true, true, true},
		{"info", false, true, true, true},
		{"warn", false, false, true, true},
		{"error", false, false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.level, func(t *testing.T) {
			var buf bytes.Buffer
			logger, err := New(&buf, FormatText, tc.level)
			if err != nil {
				t.Fatalf("New(%q): %v", tc.level, err)
			}
			logger.Debug("d")
			logger.Info("i")
			logger.Warn("w")
			logger.Error("e")
			out := buf.String()
			if got := strings.Contains(out, "msg=d"); got != tc.debugOK {
				t.Errorf("debug visible = %v, want %v", got, tc.debugOK)
			}
			if got := strings.Contains(out, "msg=i"); got != tc.infoOK {
				t.Errorf("info visible = %v, want %v", got, tc.infoOK)
			}
			if got := strings.Contains(out, "msg=w"); got != tc.warnOK {
				t.Errorf("warn visible = %v, want %v", got, tc.warnOK)
			}
			if got := strings.Contains(out, "msg=e"); got != tc.errOK {
				t.Errorf("error visible = %v, want %v", got, tc.errOK)
			}
		})
	}
}

func TestNew_UnknownFormat(t *testing.T) {
	if _, err := New(&bytes.Buffer{}, "yaml", "info"); err == nil {
		t.Error("expected error for unknown format")
	}
}

func TestNew_NilWriter(t *testing.T) {
	if _, err := New(nil, FormatText, "info"); err == nil {
		t.Error("expected error for nil writer")
	}
}

func TestNew_EmptyLevelIsError(t *testing.T) {
	if _, err := New(&bytes.Buffer{}, FormatText, ""); err == nil {
		t.Error("expected error for empty level (callers should resolve DefaultLevel first)")
	}
}

func TestNew_FormatCaseInsensitive(t *testing.T) {
	for _, f := range []string{"TEXT", "Text", "text", "  text  "} {
		if _, err := New(&bytes.Buffer{}, f, "info"); err != nil {
			t.Errorf("format %q: %v", f, err)
		}
	}
}

func TestNew_LevelCaseInsensitive(t *testing.T) {
	for _, l := range []string{"DEBUG", "Info", "WARN", "warning", "  error  "} {
		if _, err := New(&bytes.Buffer{}, FormatText, l); err != nil {
			t.Errorf("level %q: %v", l, err)
		}
	}
}

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
		err  bool
	}{
		{"debug", slog.LevelDebug, false},
		{"info", slog.LevelInfo, false},
		{"warn", slog.LevelWarn, false},
		{"warning", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"  INFO  ", slog.LevelInfo, false},
		{"", 0, true},
		{"trace", 0, true},
		{"verbose", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseLevel(tc.in)
			if tc.err {
				if err == nil {
					t.Errorf("expected error for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseLevel(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestDefaultLevel(t *testing.T) {
	cases := []struct {
		mode string
		want string
	}{
		{ModeInteractive, "warn"},
		{ModeNonInteractive, "info"},
		{"", "warn"}, // empty defaults to interactive
		{"unknown", "warn"},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			if got := DefaultLevel(tc.mode); got != tc.want {
				t.Errorf("DefaultLevel(%q) = %q, want %q", tc.mode, got, tc.want)
			}
		})
	}
}

func TestDiscard_DropsAllRecords(t *testing.T) {
	logger := Discard()
	// The four levels should all be safe to call and produce no
	// observable effect.
	logger.Debug("d", "k", "v")
	logger.Info("i", "k", "v")
	logger.Warn("w", "k", "v")
	logger.Error("e", "k", "v")
	// Nothing to assert on output (it's io.Discard), but at least
	// confirm the logger is non-nil and the four calls don't panic.
	if logger == nil {
		t.Error("Discard() returned nil")
	}
}

func TestTee_WritesToAllWriters(t *testing.T) {
	var a, b bytes.Buffer
	logger, err := Tee([]io.Writer{&a, &b}, FormatText, "info")
	if err != nil {
		t.Fatalf("Tee: %v", err)
	}
	logger.Info("hello", "k", "v")
	for label, buf := range map[string]*bytes.Buffer{"a": &a, "b": &b} {
		if !strings.Contains(buf.String(), "hello") {
			t.Errorf("writer %s missing 'hello': %q", label, buf.String())
		}
		if !strings.Contains(buf.String(), "k=v") {
			t.Errorf("writer %s missing k=v: %q", label, buf.String())
		}
	}
}

func TestTee_SingleWriterDelegatesToNew(t *testing.T) {
	var buf bytes.Buffer
	logger, err := Tee([]io.Writer{&buf}, FormatText, "info")
	if err != nil {
		t.Fatalf("Tee: %v", err)
	}
	logger.Info("solo")
	if !strings.Contains(buf.String(), "solo") {
		t.Errorf("expected 'solo' in single-writer output, got: %q", buf.String())
	}
}

func TestTee_RejectsEmptyWriters(t *testing.T) {
	if _, err := Tee(nil, FormatText, "info"); err == nil {
		t.Error("expected error for nil writers slice")
	}
	if _, err := Tee([]io.Writer{}, FormatText, "info"); err == nil {
		t.Error("expected error for empty writers slice")
	}
}

func TestTee_RejectsNilWriterInSlice(t *testing.T) {
	var a bytes.Buffer
	if _, err := Tee([]io.Writer{&a, nil}, FormatText, "info"); err == nil {
		t.Error("expected error when writers slice contains nil")
	}
}

func TestTee_PropagatesUnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	if _, err := Tee([]io.Writer{&buf}, "yaml", "info"); err == nil {
		t.Error("expected error for unknown format")
	}
}

func TestTee_PropagatesUnknownLevel(t *testing.T) {
	var buf bytes.Buffer
	if _, err := Tee([]io.Writer{&buf}, FormatText, "trace"); err == nil {
		t.Error("expected error for unknown level")
	}
}
