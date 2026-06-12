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

	flag "github.com/spf13/pflag"

	"github.com/got-sh/got/internal/cli"
	"github.com/got-sh/got/internal/gerr"
	gotlog "github.com/got-sh/got/internal/log"
)

func main() {
	logger, logCfgErr := buildLogger(os.Args[1:], os.Stderr)
	if logCfgErr != nil {
		// Bad logging config is a usage error (spec §15: code 2).
		fmt.Fprintln(os.Stderr, "got:", logCfgErr)
		os.Exit(int(gerr.CodeUsage))
	}
	if err := cli.Execute(logger); err != nil {
		fmt.Fprintln(os.Stderr, "got:", gerr.UserMessage(err))
		os.Exit(gerr.ExitCode(err))
	}
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
// in append mode, 0600 perms) in addition to w. The file is closed
// when the process exits; partial writes are acceptable per spec
// §16 (the log is best-effort).
//
// When --log-max-size is also set (in megabytes, > 0), the file
// writer rotates the file when its size exceeds the threshold:
// the existing file is renamed to <log-file>.1 (overwriting any
// previous .1) and a fresh file is opened. Only one backup is
// kept; the v0.1 policy is "what just happened" rather than
// long-term retention. A max size of 0 (the default) disables
// rotation, matching the pre-rotation behavior.
func buildLogger(args []string, w io.Writer) (*slog.Logger, error) {
	fs := flag.NewFlagSet("got-log-init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		level      string
		format     = gotlog.FormatText
		noTUI      bool
		logFile    string
		logMaxSize int64
	)
	fs.StringVar(&level, "log-level", "", "log level: debug|info|warn|error")
	fs.StringVar(&format, "log-format", gotlog.FormatText, "log output format: text|json")
	fs.BoolVar(&noTUI, "no-tui", false, "force plain CLI output even in wizards")
	fs.StringVar(&logFile, "log-file", "", "also append every log record to this file (created if missing)")
	fs.Int64Var(&logMaxSize, "log-max-size", 0, "rotate --log-file when it exceeds this many megabytes (0 disables)")
	_ = fs.Parse(args)

	mode := gotlog.ModeInteractive
	if noTUI {
		mode = gotlog.ModeNonInteractive
	}
	if level == "" {
		level = gotlog.DefaultLevel(mode)
	}
	writers := []io.Writer{w}
	if logFile != "" {
		var f io.Writer
		var ferr error
		if logMaxSize > 0 {
			f, ferr = gotlog.OpenRotatingFile(logFile, logMaxSize*1024*1024)
		} else {
			f, ferr = os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		}
		if ferr != nil {
			return nil, fmt.Errorf("log: open --log-file %q: %w", logFile, ferr)
		}
		// Note: f is intentionally not closed. The process is
		// about to exit and the OS reclaims the fd. Closing
		// here would race with concurrent writes from goroutines
		// spawned by subcommands (e.g. plugin install), so
		// leaving the fd open is the correct choice for a
		// short-lived CLI process.
		writers = append(writers, f)
	}
	// gotlog.New returns *slog.Logger; we keep the import alias
	// short so the cli package can also import it without a clash.
	return gotlog.Tee(writers, format, level)
}
