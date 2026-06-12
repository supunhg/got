package git

import (
	"context"

	"github.com/got-sh/got/internal/gerr"
)

// Stage stages the given paths (relative to the work tree) by
// running `git add <paths>`. An empty path list is a no-op.
//
// Stage is a thin wrapper around the `git add` command; it does not
// validate that the paths exist or are tracked. Callers (typically
// the commit wizard's stage-review screen) are expected to pass
// paths that came from `git status --porcelain`, which always
// names real files.
func (a *ExecAdapter) Stage(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add", "--"}, paths...)
	_, _, err := a.run(ctx, args...)
	if err != nil {
		return gerr.GitError(err, args...)
	}
	return nil
}

// Unstage unstages the given paths by running
// `git reset HEAD -- <paths>`. An empty path list is a no-op.
func (a *ExecAdapter) Unstage(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"reset", "HEAD", "--"}, paths...)
	_, _, err := a.run(ctx, args...)
	if err != nil {
		return gerr.GitError(err, args...)
	}
	return nil
}

// StageAllTracked stages all modifications to tracked files by
// running `git add -u`. This is the implementation behind
// `got commit -a`; it does NOT pick up untracked files (use
// `git add <untracked>` for that). Matches git's -a flag exactly.
func (a *ExecAdapter) StageAllTracked(ctx context.Context) error {
	_, _, err := a.run(ctx, "add", "-u")
	if err != nil {
		return gerr.GitError(err, "add", "-u")
	}
	return nil
}
