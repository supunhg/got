// Command got is a Git-native developer operating layer.
//
// See ARCHITECTURE.md for the high-level design and got-spec.md for the
// binding v0.1 specification.
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"

	flag "github.com/spf13/pflag"

	"github.com/got-sh/got/internal/cli"
	"github.com/got-sh/got/internal/gerr"
	gotlog "github.com/got-sh/got/internal/log"
)

func main() {
	// run() returns the exit code so the deferred session-log
	// summary print can fire on every path (success, subcommand
	// error, panic-recovered). os.Exit bypasses defers, so we
	// route through a helper: main() calls os.Exit(run()), and
	// the defer in run() runs when run() returns, before
	// main() forwards the code to os.Exit. This is the standard
	// pattern for "do something at exit, then exit with a
	// computed code".
	os.Exit(run())
}

// run is main's body. It is split out so the deferred
// session-log summary (registered here) fires on both the
// success and the error path; if main() called os.Exit
// directly, defers would be skipped and the summary would
// never reach the user. Returns the process exit code per
// spec §15.
func run() int {
	logger, sessionLog, logCfgErr := buildLogger(os.Args[1:], os.Stderr)
	if logCfgErr != nil {
		// Bad logging config is a usage error (spec §15: code 2).
		fmt.Fprintln(os.Stderr, "got:", logCfgErr)
		return int(gerr.CodeUsage)
	}
	// Print the session-log summary at exit, after the command
	// has run, so the user can find the captured trace even if
	// the subcommand errored. The defer fires on both the
	// success and the error path; printing to stderr keeps the
	// summary out of stdout (which most subcommands use for
	// their own data output).
	defer func() {
		if sessionLog != nil {
			fmt.Fprintln(os.Stderr, sessionLog.Summary())
		}
	}()
	if err := cli.Execute(logger); err != nil {
		fmt.Fprintln(os.Stderr, "got:", gerr.UserMessage(err))
		return gerr.ExitCode(err)
	}
	return 0
}

// buildLogger parses the two logging-related persistent flags
// (--log-level, --log-format) plus --no-tui from args and constructs
// the spec §16 *slog.Logger. We do this in main() instead of inside
// cli.Execute() so the logger exists before the command tree runs
// (PersistentPreRunE would also work but mixes flag parsing with
// logger construction, which is harder to test in isolation).
//
// The same flag definitions also live in cli.NewRootCmd — Cobra is
// the source of truth for help text and validation. We intentionally
// do NOT call into the cobra command from here: a malformed flag
// would surface twice (once here, once in cobra). Instead, we use
// pflag directly with ContinueOnError + an io.Discard writer so
// any parse problem is silently ignored here and re-surfaced by
// cobra with the proper usage text.
//
// When --log-file is set, the logger is teed to that file (created
// in append mode, perms from --log-file-mode) in addition to w.
// The file is closed when the process exits; partial writes are
// acceptable per spec §16 (the log is best-effort).
//
// When --log-max-size is also set (in megabytes, > 0), the file
// writer rotates the file when its size exceeds the threshold:
// the existing file is renamed to <log-file>.1 (overwriting any
// previous .1) and a fresh file is opened. Only one backup is
// kept; the v0.1 policy is "what just happened" rather than
// long-term retention. A max size of 0 (the default) disables
// rotation, matching the pre-rotation behavior. The fresh file
// is created with the same --log-file-mode perms.
//
// The file writer is wrapped in a *CountingWriter so the returned
// SessionLog can report how many records were written to the file
// at command exit. The wrapper sits OUTSIDE the rotation logic
// (counter → RotatingFile → os.File) so the count reflects every
// record across all rotations, not just the current file.
//
// The returned SessionLog is nil when --log-file is not set; main()
// uses that to decide whether to print the post-exit summary line.
func buildLogger(args []string, w io.Writer) (*slog.Logger, *gotlog.SessionLog, error) {
	fs := flag.NewFlagSet("got-log-init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		level       string
		format      = gotlog.FormatText
		noTUI       bool
		logFile     string
		logMaxSize  int64
		logFileMode string
	)
	fs.StringVar(&level, "log-level", "", "log level: debug|info|warn|error")
	fs.StringVar(&format, "log-format", gotlog.FormatText, "log output format: text|json")
	fs.BoolVar(&noTUI, "no-tui", false, "force plain CLI output even in wizards")
	fs.StringVar(&logFile, "log-file", "", "also append every log record to this file (created if missing)")
	fs.Int64Var(&logMaxSize, "log-max-size", 0, "rotate --log-file when it exceeds this many megabytes (0 disables)")
	fs.StringVar(&logFileMode, "log-file-mode", "0600", "permissions for --log-file (octal, e.g. 0600, 0640)")
	_ = fs.Parse(args)

	// Parse --log-file-mode as octal. Using base 8 (not 0) so
	// "0600" works but "384" is rejected as not-a-valid-octal:
	// users who want decimal can write "0o600" or convert.
	// The string is captured here (not in the pflag Var) so a
	// bad value doesn't surface twice; cobra will re-parse
	// the flag and surface the error with proper usage.
	modeUint, modeErr := strconv.ParseUint(logFileMode, 8, 32)
	if modeErr != nil {
		return nil, nil, fmt.Errorf("log: invalid --log-file-mode %q (want octal, e.g. 0600, 0640): %w", logFileMode, modeErr)
	}

	mode := gotlog.ModeInteractive
	if noTUI {
		mode = gotlog.ModeNonInteractive
	}
	if level == "" {
		level = gotlog.DefaultLevel(mode)
	}
	writers := []io.Writer{w}
	var sessionLog *gotlog.SessionLog
	if logFile != "" {
		// --log-file-mode was validated above; cast the parsed
		// uint32 to os.FileMode here. OpenRotatingFile masks
		// to the lower 9 bits; we do the same for the
		// non-rotating path to keep behavior consistent.
		fileMode := os.FileMode(modeUint) & 0o777
		var fileWriter io.Writer
		var rotator *gotlog.RotatingFile
		var plainFile *os.File
		var ferr error
		if logMaxSize > 0 {
			rotator, ferr = gotlog.OpenRotatingFile(logFile, logMaxSize*1024*1024, fileMode)
			fileWriter = rotator
		} else {
			plainFile, ferr = os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, fileMode)
			fileWriter = plainFile
		}
		if ferr != nil {
			return nil, nil, fmt.Errorf("log: open --log-file %q: %w", logFile, ferr)
		}
		// Note: the file is intentionally not closed. The
		// process is about to exit and the OS reclaims the fd.
		// Closing here would race with concurrent writes from
		// goroutines spawned by subcommands (e.g. plugin
		// install), so leaving the fd open is the correct
		// choice for a short-lived CLI process.
		//
		// Wrap the file writer in a CountingWriter so the
		// session-log summary at command exit can report
		// record count. The wrapper sits ABOVE the rotation
		// logic (counter sees every record, including ones
		// that have been rotated to .1).
		counter := gotlog.NewCountingWriter(fileWriter)
		writers = append(writers, counter)
		sessionLog = &gotlog.SessionLog{
			Path:      logFile,
			Counter:   counter,
			Rotator:   rotator,
			PlainFile: plainFile,
		}
	}
	// gotlog.New returns *slog.Logger; we keep the import alias
	// short so the cli package can also import it without a clash.
	logger, err := gotlog.Tee(writers, format, level)
	if err != nil {
		return nil, nil, err
	}
	return logger, sessionLog, nil
}
