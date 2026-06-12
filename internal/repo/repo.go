package repo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/got-sh/got/internal/gerr"
)

// Subdirs are the directories that live directly under .got/. v0.1
// only uses got.db and plugins; the rest are reserved per got-spec.md
// §5 and created empty so future versions can drop in features without
// touching the layout.
var Subdirs = []string{
	"snapshots",
	"workspaces",
	"decisions",
	"health",
	"cache",
	"plugins",
}

// Paths bundles the on-disk locations of the .got/ artifacts a command
// might need. All paths are absolute. The zero value is invalid; use
// NewPaths to construct one.
type Paths struct {
	// WorkTree is the absolute path to the repository's work tree.
	WorkTree string
	// GOTDir is the absolute path to .got/ (WorkTree + ".got").
	GOTDir string
	// ConfigFile is .got/config.yaml.
	ConfigFile string
	// DBFile is .got/got.db.
	DBFile string
}

// NewPaths returns the Paths for the given work tree.
func NewPaths(workTree string) Paths {
	got := filepath.Join(workTree, ".got")
	return Paths{
		WorkTree:   workTree,
		GOTDir:     got,
		ConfigFile: filepath.Join(got, "config.yaml"),
		DBFile:     filepath.Join(got, "got.db"),
	}
}

// EnsureGOTDir creates .got/ and all its reserved subdirectories if
// they don't already exist. Permissions are 0o755. It is safe to call
// when .got/ already exists; existing directories are left alone.
//
// Returns nil if .got/ already existed (or was just created) and all
// subdirs are present.
func (p Paths) EnsureGOTDir() error {
	if err := os.MkdirAll(p.GOTDir, 0o755); err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("creating %s", p.GOTDir))
	}
	for _, sub := range Subdirs {
		full := filepath.Join(p.GOTDir, sub)
		if err := os.MkdirAll(full, 0o755); err != nil {
			return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("creating %s", full))
		}
	}
	return nil
}

// EnsureGitignoreEntry ensures the .gitignore at the work tree root
// contains a line that excludes .got/. The line is appended if absent;
// if it's already there (possibly as ".got" or ".got/" or with leading
// whitespace) the file is left untouched.
//
// We deliberately do not write to .git/info/exclude — that file is
// local-only and would not propagate to teammates. See got-spec.md §4.
func (p Paths) EnsureGitignoreEntry() error {
	gi := filepath.Join(p.WorkTree, ".gitignore")
	existing, err := os.ReadFile(gi)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("reading %s", gi))
	}
	if hasGitignoreEntry(existing, ".got") {
		return nil
	}
	// Append. We always end with a single trailing newline; the spec
	// says .got/ is the canonical entry so we add that form.
	var b strings.Builder
	if len(existing) > 0 {
		b.Write(existing)
		// If the file does not end with a newline, add one before the
		// new entry so it is on its own line.
		if !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
	}
	b.WriteString("# GOT metadata (see `got init`)\n")
	b.WriteString(".got/\n")
	if err := os.WriteFile(gi, []byte(b.String()), 0o644); err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("writing %s", gi))
	}
	return nil
}

// hasGitignoreEntry reports whether the .gitignore body already has a
// line that excludes the .got/ directory. Matching is intentionally
// loose so the common forms (".got", ".got/", "  .got  # comment")
// are all recognized as already-present.
func hasGitignoreEntry(body []byte, target string) bool {
	for _, line := range strings.Split(string(body), "\n") {
		// Strip trailing comments and whitespace.
		hash := strings.IndexByte(line, '#')
		if hash >= 0 {
			line = line[:hash]
		}
		line = strings.TrimSpace(line)
		if line == target || line == target+"/" {
			return true
		}
	}
	return false
}
