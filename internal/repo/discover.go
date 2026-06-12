// Package repo locates the Git repository the user is operating on and
// provides a small struct for carrying the work tree + .git directory
// path through the CLI.
package repo

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/got-sh/got/internal/gerr"
)

// Discover walks up from start looking for a directory containing .git.
// .git can be either a directory (normal repo) or a file (worktree /
// submodule pointer). Returns the work tree root.
//
// Returns gerr.NotInGitRepo if no .git is found before reaching the
// filesystem root.
func Discover(start string) (string, error) {
	cur, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(cur, ".git")
		info, err := os.Stat(candidate)
		switch {
		case err == nil:
			// .git can be a directory (normal repo) or a file
			// (worktree / submodule pointer). Both are valid repo
			// markers.
			_ = info
			return cur, nil
		case errors.Is(err, fs.ErrNotExist):
			// Keep walking up.
		default:
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", gerr.NotInGitRepo(start)
		}
		cur = parent
	}
}

// Open discovers the repository at or above start and returns a Repo.
func Open(start string) (*Repo, error) {
	workTree, err := Discover(start)
	if err != nil {
		return nil, err
	}
	return &Repo{
		WorkTree: workTree,
		GitDir:   filepath.Join(workTree, ".git"),
	}, nil
}

// Repo bundles a work tree path with the .git directory location.
type Repo struct {
	WorkTree string
	GitDir   string
}

// HasGOTDir reports whether .got/ exists in the work tree.
func (r *Repo) HasGOTDir() bool {
	info, err := os.Stat(filepath.Join(r.WorkTree, ".got"))
	return err == nil && info.IsDir()
}
