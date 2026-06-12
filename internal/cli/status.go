package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/repo"
	"github.com/got-sh/got/internal/store"
)

// statusOptions is the resolved flag set for `got status`. Keeping the
// flags as a small struct makes the runStatus signature stable and
// easy to call from tests.
type statusOptions struct {
	asJSON  bool
	asShort bool
}

// newStatusCmd builds the `got status` subcommand. Flags follow §13
// of got-spec.md: --short for porcelain (one line per entry) and --json
// for machine-readable output. The human-readable (no flag) path adds
// a GOT metadata section underneath the git status.
func newStatusCmd(deps Deps) *cobra.Command {
	opts := &statusOptions{}
	cmd := &cobra.Command{
		Use:   "status [path]",
		Short: "Show the working tree status and GOT metadata",
		Long: `Show the working tree status (from git) and, if .got/ is
initialized, a summary of the GOT metadata store.

The human-readable output (no flag) renders the git status first, then a
GOT section with the schema version, got version, init timestamp, and
row counts for snapshots, decisions, workspaces, and health runs.

Flags:
  --short   porcelain output (one line per file)
  --json    machine-readable JSON (git status + GOT metadata)`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, deps, args, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.asJSON, "json", false, "machine-readable JSON output")
	cmd.Flags().BoolVarP(&opts.asShort, "short", "s", false, "porcelain output (one line per file)")
	return cmd
}

// runStatus is the shared implementation behind the cobra handler and
// the test suite. It is split out so tests can call it directly without
// the cobra flag-parsing layer.
func runStatus(cmd *cobra.Command, deps Deps, args []string, opts *statusOptions) error {
	logger := loggerFor(deps)
	start := "."
	if len(args) > 0 {
		start = args[0]
	}
	logger.Info("status starting", "start", start, "json", opts.asJSON, "short", opts.asShort)
	workTree, err := deps.Discover(start)
	if err != nil {
		return err
	}
	a := deps.AdapterFor(workTree)
	gitStatus, err := a.Status(cmd.Context())
	if err != nil {
		logger.Warn("status failed", "err", err.Error())
		return err
	}
	out := cmd.OutOrStdout()
	if out == nil {
		out = deps.Stdout
	}
	logger.Info("status finished", "branch", gitStatus.Branch, "entries", len(gitStatus.Entries))
	switch {
	case opts.asJSON:
		return writeStatusJSON(out, deps, workTree, gitStatus)
	case opts.asShort:
		return writeStatusShort(out, gitStatus)
	default:
		return writeStatusHuman(out, deps, workTree, gitStatus)
	}
} // statusReport is the JSON shape emitted by `got status --json`. It

// bundles the raw git status with the GOT metadata so downstream tools
// can read either or both in one shot. The GOT block is always
// present: when .got/ is missing it carries Initialized=false and
// NotInitializedReason; downstream tools can rely on the field's
// presence rather than checking for its absence.
type statusReport struct {
	Git *git.Status     `json:"git"`
	GOT *statusGOTBlock `json:"got"`
}

// statusGOTBlock is the GOT-metadata slice of the JSON report. The
// pointer is never nil; the field is not marked omitempty for that
// reason. NotInitializedReason is omitempty so initialized output
// doesn't carry an empty-string noise field.
type statusGOTBlock struct {
	Initialized          bool         `json:"initialized"`
	SchemaVersion        int          `json:"schemaVersion,omitempty"`
	GotVersion           string       `json:"gotVersion,omitempty"`
	InitAtUnixMS         int64        `json:"initAtUnixMs,omitempty"`
	InitUser             string       `json:"initUser,omitempty"`
	Counts               store.Counts `json:"counts"`
	DBPath               string       `json:"dbPath,omitempty"`
	NotInitializedReason string       `json:"notInitializedReason,omitempty"`
}

// writeStatusJSON encodes the combined git + GOT report as indented
// JSON. If the GOT store cannot be opened, the GOT block is still
// emitted with Initialized=false and NotInitializedReason set.
func writeStatusJSON(w io.Writer, deps Deps, workTree string, s git.Status) error {
	report := statusReport{Git: &s, GOT: loadGOTBlock(deps, workTree)}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// loadGOTBlock returns a statusGOTBlock describing the .got/ store in
// workTree. If .got/got.db does not exist or cannot be opened, the
// returned block has Initialized=false and a NotInitializedReason;
// the call never returns an error.
func loadGOTBlock(deps Deps, workTree string) *statusGOTBlock {
	block := &statusGOTBlock{}
	paths := repo.NewPaths(workTree)

	// Probe .got/got.db. We deliberately do not call paths.EnsureGOTDir
	// here — `got status` is a read-only command.
	if deps.StoreFor == nil {
		block.NotInitializedReason = "store factory not configured"
		return block
	}
	s, err := deps.StoreFor(paths.DBFile)
	if err != nil {
		block.NotInitializedReason = err.Error()
		return block
	}
	defer func() { _ = s.Close() }()

	ver, err := s.SchemaVersion()
	if err != nil {
		block.NotInitializedReason = err.Error()
		return block
	}
	gotVer, _ := s.MetaGet("got_version")
	initAt, _ := s.MetaGet("init_at")
	initUser, _ := s.MetaGet("init_user")
	counts, _ := s.Counts()

	block.Initialized = true
	block.SchemaVersion = ver
	block.GotVersion = gotVer
	block.DBPath = paths.DBFile
	block.Counts = counts
	if initAt != "" {
		if ms, perr := strconv.ParseInt(initAt, 10, 64); perr == nil {
			block.InitAtUnixMS = ms
		}
	}
	block.InitUser = initUser
	return block
}

// writeStatusHuman renders the multi-section human-readable status:
// the git status first, then (if initialized) a GOT section.
func writeStatusHuman(w io.Writer, deps Deps, workTree string, s git.Status) error {
	if err := writeGitStatusHuman(w, s); err != nil {
		return err
	}
	block := loadGOTBlock(deps, workTree)
	writeGOTSectionHuman(w, block)
	return nil
}

// writeGitStatusHuman renders the git-only status block: branch,
// ahead/behind, and the staged/unstaged/untracked groups.
func writeGitStatusHuman(w io.Writer, s git.Status) error {
	if s.Detached {
		fmt.Fprintln(w, "HEAD detached")
	} else if s.Branch != "" {
		fmt.Fprintf(w, "On branch %s\n", s.Branch)
	}
	if s.Upstream != "" {
		switch {
		case s.Ahead > 0 && s.Behind > 0:
			fmt.Fprintf(w, "Your branch and %s have diverged (%d ahead, %d behind).\n", s.Upstream, s.Ahead, s.Behind)
		case s.Ahead > 0:
			fmt.Fprintf(w, "Your branch is ahead of %s by %d commit(s).\n", s.Upstream, s.Ahead)
		case s.Behind > 0:
			fmt.Fprintf(w, "Your branch is behind %s by %d commit(s).\n", s.Upstream, s.Behind)
		default:
			fmt.Fprintf(w, "Your branch is up to date with %s.\n", s.Upstream)
		}
	}
	staged, unstaged, untracked := splitStatus(s.Entries)
	if len(staged) > 0 {
		fmt.Fprintln(w, "\nChanges to be committed:")
		for _, e := range staged {
			fmt.Fprintf(w, "  %s\n", statusLabel(e))
		}
	}
	if len(unstaged) > 0 {
		fmt.Fprintln(w, "\nChanges not staged for commit:")
		for _, e := range unstaged {
			fmt.Fprintf(w, "  %s\n", statusLabel(e))
		}
	}
	if len(untracked) > 0 {
		fmt.Fprintln(w, "\nUntracked files:")
		for _, e := range untracked {
			fmt.Fprintf(w, "  %s\n", e.Path)
		}
	}
	if len(s.Entries) == 0 && s.Upstream == "" && !s.Detached {
		fmt.Fprintln(w, "nothing to commit, working tree clean")
	}
	return nil
}

// writeGOTSectionHuman appends the GOT-metadata section to w. It is
// always called, even when .got/ is uninitialized; in that case it
// prints a single hint line so the user knows why the section is
// empty.
func writeGOTSectionHuman(w io.Writer, block *statusGOTBlock) {
	fmt.Fprintln(w, "\nGOT:")
	if block == nil || !block.Initialized {
		if block != nil && block.NotInitializedReason != "" {
			fmt.Fprintf(w, "  not initialized (%s)\n", block.NotInitializedReason)
		} else {
			fmt.Fprintln(w, "  not initialized (run `got init` to set up .got/)")
		}
		return
	}
	if block.GotVersion != "" {
		fmt.Fprintf(w, "  version:     %s\n", block.GotVersion)
	}
	if block.SchemaVersion > 0 {
		fmt.Fprintf(w, "  schema:      v%d\n", block.SchemaVersion)
	}
	if block.InitAtUnixMS > 0 {
		t := time.UnixMilli(block.InitAtUnixMS).UTC()
		fmt.Fprintf(w, "  initialized: %s by %s\n", t.Format("2006-01-02 15:04:05 MST"), block.InitUser)
	}
	fmt.Fprintf(w, "  snapshots:   %d\n", block.Counts.Snapshots)
	fmt.Fprintf(w, "  decisions:   %d\n", block.Counts.Decisions)
	fmt.Fprintf(w, "  workspaces:  %d (%d open)\n", block.Counts.Workspaces, block.Counts.OpenWorkspaces)
	fmt.Fprintf(w, "  health runs: %d\n", block.Counts.HealthRuns)
}

func writeStatusShort(w io.Writer, s git.Status) error {
	for _, e := range s.Entries {
		if e.IsUntracked {
			fmt.Fprintf(w, "?? %s\n", e.Path)
			continue
		}
		fmt.Fprintf(w, "%s %s\n", e.XY, e.Path)
	}
	return nil
}

// statusLabel renders a one-line human description of an entry.
func statusLabel(e git.StatusEntry) string {
	if e.IsRenamed {
		return fmt.Sprintf("renamed: %s -> %s", e.OriginalPath, e.Path)
	}
	// e.XY is two characters: index (staged), worktree (unstaged).
	switch {
	case e.IsStaged && e.IsUnstaged:
		return fmt.Sprintf("%s %s", e.XY, e.Path)
	case e.IsStaged:
		return fmt.Sprintf("%s- %s", string(e.XY[0]), e.Path)
	case e.IsUnstaged:
		return fmt.Sprintf("-%s %s", string(e.XY[1]), e.Path)
	default:
		return "-- " + e.Path
	}
}

func splitStatus(entries []git.StatusEntry) (staged, unstaged, untracked []git.StatusEntry) {
	for _, e := range entries {
		switch {
		case e.IsUntracked:
			untracked = append(untracked, e)
		case e.IsStaged:
			staged = append(staged, e)
		default:
			unstaged = append(unstaged, e)
		}
	}
	return staged, unstaged, untracked
}

// writeJSON encodes v to w as indented JSON.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
