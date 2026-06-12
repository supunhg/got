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
func buildLogger(args []string, w io.Writer) (*slog.Logger, error) {
	fs := flag.NewFlagSet("got-log-init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		level  string
		format = gotlog.FormatText
		noTUI  bool
	)
	fs.StringVar(&level, "log-level", "", "log level: debug|info|warn|error")
	fs.StringVar(&format, "log-format", gotlog.FormatText, "log output format: text|json")
	fs.BoolVar(&noTUI, "no-tui", false, "force plain CLI output even in wizards")
	_ = fs.Parse(args)

	mode := gotlog.ModeInteractive
	if noTUI {
		mode = gotlog.ModeNonInteractive
	}
	if level == "" {
		level = gotlog.DefaultLevel(mode)
	}
	// gotlog.New returns *slog.Logger; we keep the import alias
	// short so the cli package can also import it without a clash.
	return gotlog.New(w, format, level)
}
