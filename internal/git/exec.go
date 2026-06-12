// exec.go implements Adapter by shelling out to the `git` binary.
package git

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/got-sh/got/internal/gerr"
)

// ExecAdapter implements Adapter by shelling out to `git`. It is the
// default Adapter used by the got CLI.
type ExecAdapter struct {
	// GitPath is the path to the `git` binary. Defaults to "git" (i.e.
	// resolved via $PATH).
	GitPath string
	// WorkTree is the directory the adapter operates on. All `git`
	// invocations run with this as the working directory.
	WorkTree string
	// Env holds additional environment variables to set when running
	// `git`. The ambient process environment is preserved.
	Env []string
}

// NewExecAdapter returns an ExecAdapter for the given work tree.
func NewExecAdapter(workTree string) *ExecAdapter {
	return &ExecAdapter{
		GitPath:  "git",
		WorkTree: workTree,
	}
}

// run executes `git args...` in the work tree, returning stdout, stderr,
// and any error. It respects ctx cancellation by killing the entire
// process group.
func (a *ExecAdapter) run(ctx context.Context, args ...string) (stdout, stderr []byte, err error) {
	cmd := exec.CommandContext(ctx, a.GitPath, args...)
	cmd.Dir = a.WorkTree
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if len(a.Env) > 0 {
		cmd.Env = append(os.Environ(), a.Env...)
	}

	err = cmd.Run()

	// If the context was cancelled, surface that as the error so callers
	// can distinguish cancellation from a real git failure.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), ctxErr
	}
	return stdoutBuf.Bytes(), stderrBuf.Bytes(), err
}

// Status implements Adapter.
func (a *ExecAdapter) Status(ctx context.Context) (Status, error) {
	stdout, _, err := a.run(ctx,
		"status", "--porcelain=v2", "--branch",
		"--untracked-files=normal", "--ignored=no")
	if err != nil {
		return Status{}, gerr.GitError(err, "status")
	}
	return parseStatusPorcelainV2(stdout)
}

// parseStatusPorcelainV2 parses the output of
// `git status --porcelain=v2 --branch`. Format spec:
// https://git-scm.com/docs/git-status#_porcelain_format_version_2
func parseStatusPorcelainV2(out []byte) (Status, error) {
	s := Status{Entries: []StatusEntry{}}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "#"):
			parseStatusHeader(line, &s)
		case strings.HasPrefix(line, "1 "):
			// 1 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <path>
			parts := strings.SplitN(line, " ", 9)
			if len(parts) < 9 {
				return s, fmt.Errorf("malformed ordinary status entry: %q", line)
			}
			xy := parts[1]
			s.Entries = append(s.Entries, StatusEntry{
				Path:       parts[8],
				XY:         xy,
				IsStaged:   isStatusChange(xy[0]),
				IsUnstaged: isStatusChange(xy[1]),
			})
		case strings.HasPrefix(line, "2 "):
			// 2 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <X><score> <path>\t<origPath>
			tab := strings.IndexByte(line, '\t')
			if tab < 0 {
				return s, fmt.Errorf("malformed renamed status entry: %q", line)
			}
			head := line[:tab]
			newPath := line[tab+1:]
			parts := strings.SplitN(head, " ", 9)
			if len(parts) < 9 {
				return s, fmt.Errorf("malformed renamed status entry: %q", line)
			}
			xy := parts[1]
			s.Entries = append(s.Entries, StatusEntry{
				Path:         newPath,
				OriginalPath: parts[8],
				XY:           xy,
				IsStaged:     isStatusChange(xy[0]),
				IsUnstaged:   isStatusChange(xy[1]),
				IsRenamed:    true,
			})
		case strings.HasPrefix(line, "? "):
			s.Entries = append(s.Entries, StatusEntry{
				Path:        strings.TrimPrefix(line, "? "),
				IsUntracked: true,
			})
		case strings.HasPrefix(line, "! "):
			// Ignored entries are skipped in v0.1.
		default:
			// Unknown line types are skipped.
		}
	}
	if err := sc.Err(); err != nil {
		return s, err
	}
	return s, nil
}

// isStatusChange reports whether a single XY byte from porcelain v2
// represents an actual change. In porcelain v2, "." (not space) is the
// "unchanged" indicator; " " only appears when the corresponding side
// (index or worktree) is absent (which is rare in practice). "?" and "!"
// are reserved for untracked/ignored entries which we handle separately.
func isStatusChange(c byte) bool {
	return c != ' ' && c != '.' && c != '?' && c != '!'
}

// parseStatusHeader parses one of the "# branch.X ..." header lines
// from `git status --porcelain=v2 --branch`.
func parseStatusHeader(line string, s *Status) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return
	}
	switch fields[1] {
	case "branch.head":
		if len(fields) >= 3 {
			if fields[2] == "(detached)" {
				s.Detached = true
			} else {
				s.Branch = fields[2]
			}
		}
	case "branch.upstream":
		if len(fields) >= 3 && fields[2] != "(none)" {
			s.Upstream = fields[2]
		}
	case "branch.ab":
		// "# branch.ab +<ahead> -<behind>"
		if len(fields) >= 4 {
			s.Ahead, _ = strconv.Atoi(strings.TrimPrefix(fields[2], "+"))
			s.Behind, _ = strconv.Atoi(strings.TrimPrefix(fields[3], "-"))
		}
	}
}

// Branches implements Adapter. Returns local branches (refs/heads/).
func (a *ExecAdapter) Branches(ctx context.Context) ([]Branch, error) {
	return a.listRefs(ctx, "refs/heads/", false)
}

// RemoteBranches implements Adapter. Returns remote-tracking branches
// (refs/remotes/). Used by `got branch -r` and `got branch -a`.
func (a *ExecAdapter) RemoteBranches(ctx context.Context) ([]Branch, error) {
	return a.listRefs(ctx, "refs/remotes/", true)
}

// listRefs is the shared implementation behind Branches and
// RemoteBranches. It always passes `--format` so we can parse the
// output with parseBranches.
func (a *ExecAdapter) listRefs(ctx context.Context, pattern string, isRemote bool) ([]Branch, error) {
	stdout, _, err := a.run(ctx,
		"for-each-ref",
		"--format=%(HEAD)%00%(refname:short)%00%(upstream:short)%00%(objectname:short)%00%(committerdate:iso-strict)",
		pattern)
	if err != nil {
		return nil, gerr.GitError(err, "for-each-ref", pattern)
	}
	return parseBranches(stdout, isRemote)
}

// parseBranches parses the NUL-separated for-each-ref output. The
// isRemote flag is set on every branch by the caller (true for refs/
// remotes/, false for refs/heads/).
func parseBranches(out []byte, isRemote bool) ([]Branch, error) {
	var branches []Branch
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x00")
		if len(parts) < 5 {
			return nil, fmt.Errorf("malformed branch line: %q", line)
		}
		b := Branch{
			Name:      parts[1],
			IsCurrent: parts[0] == "*",
			IsRemote:  isRemote,
			Upstream:  parts[2],
			SHA:       parts[3],
		}
		if t, err := time.Parse(time.RFC3339, parts[4]); err == nil {
			b.CommitAt = t
		}
		branches = append(branches, b)
	}
	return branches, sc.Err()
}

// Remotes implements Adapter.
func (a *ExecAdapter) Remotes(ctx context.Context) ([]Remote, error) {
	stdout, _, err := a.run(ctx, "config", "--get-regexp", `^remote\..*\.(url|pushurl|fetch)$`)
	if err != nil {
		// Exit code 1 from `git config` means "no matches", i.e. no
		// remotes are configured. That is not an error.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return []Remote{}, nil
		}
		return nil, gerr.GitError(err, "config", "--get-regexp")
	}
	return parseRemotes(stdout)
}

func parseRemotes(out []byte) ([]Remote, error) {
	remotes := map[string]*Remote{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		// Lines look like: "remote.<name>.url https://..."
		sp := strings.SplitN(line, " ", 2)
		if len(sp) != 2 {
			continue
		}
		key, value := sp[0], sp[1]
		parts := strings.Split(key, ".")
		if len(parts) != 3 {
			continue
		}
		name, subkey := parts[1], parts[2]
		r, ok := remotes[name]
		if !ok {
			r = &Remote{Name: name}
			remotes[name] = r
		}
		switch subkey {
		case "url":
			if r.FetchURL == "" {
				r.FetchURL = value
			}
			if r.PushURL == "" {
				r.PushURL = value
			}
		case "pushurl":
			r.PushURL = value
		case "fetch":
			r.FetchSpec = value
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	out2 := make([]Remote, 0, len(remotes))
	for _, r := range remotes {
		out2 = append(out2, *r)
	}
	return out2, nil
}

// Log implements Adapter. The returned reader yields one JSON object per
// commit when format == LogFormatNDJSON.
func (a *ExecAdapter) Log(ctx context.Context, rangeStr string, format LogFormat) (io.Reader, error) {
	if format != LogFormatNDJSON {
		return nil, gerr.Validation("unsupported log format: " + string(format))
	}
	const pretty = "%H%x00%P%x00%an%x00%ae%x00%at%x00%s%x00%D"
	args := []string{"log", "--pretty=format:" + pretty, "--no-color"}
	if rangeStr != "" {
		args = append(args, rangeStr)
	}
	stdout, _, err := a.run(ctx, args...)
	if err != nil {
		return nil, gerr.GitError(err, "log")
	}
	return encodeCommitsNDJSON(stdout)
}

func encodeCommitsNDJSON(raw []byte) (io.Reader, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x00")
		if len(parts) < 7 {
			continue
		}
		c := Commit{
			SHA:     parts[0],
			Author:  parts[2],
			Email:   parts[3],
			Subject: parts[6],
		}
		if parts[1] != "" {
			c.Parents = strings.Fields(parts[1])
		}
		if ts, err := strconv.ParseInt(parts[4], 10, 64); err == nil {
			c.Timestamp = time.Unix(ts, 0).UTC()
		}
		if parts[5] != "" {
			for _, ref := range strings.Split(parts[5], ", ") {
				ref = strings.TrimSpace(ref)
				if ref != "" {
					c.Refs = append(c.Refs, ref)
				}
			}
		}
		if err := enc.Encode(c); err != nil {
			return nil, err
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return &buf, nil
}

// CurrentRef implements Adapter. Returns the current branch name, or the
// short SHA if HEAD is detached.
func (a *ExecAdapter) CurrentRef(ctx context.Context) (string, error) {
	stdout, _, err := a.run(ctx, "symbolic-ref", "--short", "HEAD")
	if err == nil {
		return strings.TrimSpace(string(stdout)), nil
	}
	// If the context was cancelled, surface that rather than masking
	// it with a generic git error from the fallback call.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	stdout, _, err = a.run(ctx, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", gerr.GitError(err, "rev-parse", "HEAD")
	}
	return strings.TrimSpace(string(stdout)), nil
}

// Stubs for the rest of the interface. v0.1 doesn't need them; v0.2 will
// fill them in (and the snapshot engine will start using them).

func (a *ExecAdapter) Commit(ctx context.Context, msg string, opts CommitOpts) (SHA, error) {
	if msg == "" {
		return "", gerr.Validation("commit message is empty")
	}
	args := []string{"commit"}
	if opts.Amend {
		args = append(args, "--amend")
	}
	if opts.AllowEmpty {
		args = append(args, "--allow-empty")
	}
	if opts.Signoff {
		args = append(args, "--signoff")
	}
	if opts.NoVerify {
		args = append(args, "--no-verify")
	}
	if opts.Author != "" {
		args = append(args, "--author="+opts.Author)
	}
	// Use -F - and pipe the message on stdin for multi-line; use
	// -m for single-line so we don't depend on a tty for stdin.
	multi := strings.Contains(msg, "\n")
	if multi {
		args = append(args, "-F", "-")
	} else {
		args = append(args, "-m", msg)
	}

	cmd := exec.CommandContext(ctx, a.GitPath, args...)
	cmd.Dir = a.WorkTree
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if multi {
		cmd.Stdin = strings.NewReader(msg)
	}
	if len(a.Env) > 0 {
		cmd.Env = append(os.Environ(), a.Env...)
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", gerr.GitError(err, args...)
	}
	// Resolve the new SHA via `git rev-parse HEAD`. Slightly racy in
	// the sense that the work tree could have changed in between,
	// but for a CLI invocation this is the standard pattern.
	shaOut, _, err := a.run(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", gerr.GitError(err, "rev-parse", "HEAD")
	}
	return SHA(strings.TrimSpace(string(shaOut))), nil
}

// Checkout implements Adapter. `git checkout` switches the working
// tree to the given ref. Create=true maps to `git checkout -b`
// (create the branch and switch to it in one step). Detach=true
// maps to `--detach` (used for one-off inspections of a SHA).
func (a *ExecAdapter) Checkout(ctx context.Context, ref string, opts CheckoutOpts) error {
	if ref == "" {
		return gerr.Validation("checkout ref is empty")
	}
	args := []string{"checkout"}
	if opts.Create {
		args = append(args, "-b")
	}
	if opts.Force {
		args = append(args, "--force")
	}
	if opts.Detach {
		args = append(args, "--detach")
	}
	args = append(args, ref)
	_, stderr, err := a.run(ctx, args...)
	if err != nil {
		return gerr.GitError(err, args...)
	}
	_ = stderr
	return nil
}

// CreateBranch implements Adapter.
func (a *ExecAdapter) CreateBranch(ctx context.Context, name, startPoint string) error {
	if name == "" {
		return gerr.Validation("branch name is empty")
	}
	if strings.ContainsAny(name, " \t\n~^:?*[\\") {
		return gerr.Validation(fmt.Sprintf("invalid branch name %q (contains forbidden characters)", name))
	}
	args := []string{"branch", name}
	if startPoint != "" {
		args = append(args, startPoint)
	}
	_, _, err := a.run(ctx, args...)
	if err != nil {
		return gerr.GitError(err, args...)
	}
	return nil
}

// DeleteBranch implements Adapter.
func (a *ExecAdapter) DeleteBranch(ctx context.Context, name string, force bool) error {
	if name == "" {
		return gerr.Validation("branch name is empty")
	}
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, stderr, err := a.run(ctx, "branch", flag, name)
	if err != nil {
		// git branch -d exits with 1 and a helpful stderr message
		// when the branch is not fully merged; surface that as a
		// GitError so callers can match on it.
		return gerr.GitError(err, "branch", flag, name)
	}
	_ = stderr
	return nil
}

func (a *ExecAdapter) Merge(_ context.Context, _ string, _ MergeOpts) error {
	return gerr.Validation("`got merge` is not yet implemented in v0.1")
}

func (a *ExecAdapter) Reset(_ context.Context, _ string, _ ResetMode) error {
	return gerr.Validation("`got reset` is not yet implemented in v0.1")
}

// Fetch implements Adapter. An empty remote name fetches the default
// remote's tracking branches.
func (a *ExecAdapter) Fetch(ctx context.Context, remote string) error {
	args := []string{"fetch"}
	if remote != "" {
		args = append(args, remote)
	}
	_, _, err := a.run(ctx, args...)
	if err != nil {
		return gerr.GitError(err, args...)
	}
	return nil
}

// FetchPrune implements Adapter. Like Fetch but passes --prune so
// stale remote-tracking refs are removed as part of the fetch.
func (a *ExecAdapter) FetchPrune(ctx context.Context, remote string) error {
	args := []string{"fetch", "--prune"}
	if remote != "" {
		args = append(args, remote)
	}
	_, _, err := a.run(ctx, args...)
	if err != nil {
		return gerr.GitError(err, args...)
	}
	return nil
}

// FetchAll implements Adapter. Runs `git fetch --all` (and
// optionally `--prune`).
func (a *ExecAdapter) FetchAll(ctx context.Context, prune bool) error {
	args := []string{"fetch", "--all"}
	if prune {
		args = append(args, "--prune")
	}
	_, _, err := a.run(ctx, args...)
	if err != nil {
		return gerr.GitError(err, args...)
	}
	return nil
}

// Push implements Adapter. The CLI layer is responsible for refusing
// non-fast-forward pushes unless ForceWithLease or Force is set; the
// adapter just translates the request.
func (a *ExecAdapter) Push(ctx context.Context, remote, branch string, opts PushOpts) error {
	if remote == "" {
		return gerr.Validation("push remote is empty")
	}
	if branch == "" {
		return gerr.Validation("push branch is empty")
	}
	args := []string{"push"}
	if opts.ForceWithLease {
		args = append(args, "--force-with-lease")
	} else if opts.Force {
		args = append(args, "--force")
	}
	if opts.SetUpstream {
		args = append(args, "--set-upstream")
	}
	if opts.Tags {
		args = append(args, "--tags")
	}
	args = append(args, remote, branch)
	_, _, err := a.run(ctx, args...)
	if err != nil {
		return gerr.GitError(err, args...)
	}
	return nil
}

// RemoteAdd implements Adapter.
func (a *ExecAdapter) RemoteAdd(ctx context.Context, name, url string) error {
	if name == "" {
		return gerr.Validation("remote name is empty")
	}
	if url == "" {
		return gerr.Validation("remote URL is empty")
	}
	_, _, err := a.run(ctx, "remote", "add", name, url)
	if err != nil {
		return gerr.GitError(err, "remote", "add", name, url)
	}
	return nil
}

// RemoteRemove implements Adapter. The force flag is a CLI-level
// guard (refuse to remove a remote with tracking branches unless
// --force); `git remote remove` itself has no --force flag.
func (a *ExecAdapter) RemoteRemove(ctx context.Context, name string, force bool) error {
	if name == "" {
		return gerr.Validation("remote name is empty")
	}
	_ = force
	_, _, err := a.run(ctx, "remote", "remove", name)
	if err != nil {
		return gerr.GitError(err, "remote", "remove", name)
	}
	return nil
}

// RemoteRename implements Adapter.
func (a *ExecAdapter) RemoteRename(ctx context.Context, oldName, newName string) error {
	if oldName == "" || newName == "" {
		return gerr.Validation("remote rename requires both old and new names")
	}
	_, _, err := a.run(ctx, "remote", "rename", oldName, newName)
	if err != nil {
		return gerr.GitError(err, "remote", "rename", oldName, newName)
	}
	return nil
}

// RemoteSetURL implements Adapter. When pushURL is true the push URL
// is updated (`git remote set-url --push`); otherwise the fetch URL
// is updated.
func (a *ExecAdapter) RemoteSetURL(ctx context.Context, name, url string, pushURL bool) error {
	if name == "" || url == "" {
		return gerr.Validation("remote set-url requires both name and URL")
	}
	args := []string{"remote", "set-url"}
	if pushURL {
		args = append(args, "--push")
	}
	args = append(args, name, url)
	_, _, err := a.run(ctx, args...)
	if err != nil {
		return gerr.GitError(err, args...)
	}
	return nil
}

// RemotePrune implements Adapter.
func (a *ExecAdapter) RemotePrune(ctx context.Context, name string) error {
	if name == "" {
		return gerr.Validation("remote name is empty")
	}
	_, _, err := a.run(ctx, "remote", "prune", name)
	if err != nil {
		return gerr.GitError(err, "remote", "prune", name)
	}
	return nil
}

// GraphASCII implements Adapter. Runs `git log --graph --decorate
// --oneline` (and --all when opts.All is true) with the given
// filters, and returns the raw output. The graph glyphs (|, \, /,
// *, _, =) are part of the output verbatim; callers are expected
// to wrap them in styles.
func (a *ExecAdapter) GraphASCII(ctx context.Context, opts GraphOpts) (string, error) {
	args := []string{"log", "--graph", "--decorate", "--oneline", "--no-color"}
	if opts.All {
		args = append(args, "--all")
	}
	if opts.MaxCount > 0 {
		args = append(args, "-n", strconv.Itoa(opts.MaxCount))
	} else if opts.MaxCount == 0 {
		// Spec §9 default: 200 commits.
		args = append(args, "-n", "200")
	}
	if opts.Since != "" {
		args = append(args, "--since="+opts.Since)
	}
	if opts.Until != "" {
		args = append(args, "--until="+opts.Until)
	}
	if opts.Author != "" {
		args = append(args, "--author="+opts.Author)
	}
	if opts.Grep != "" {
		args = append(args, "--grep="+opts.Grep)
	}
	stdout, _, err := a.run(ctx, args...)
	if err != nil {
		// git log returns non-zero with no error message when there
		// are simply no commits; treat that as an empty graph, not
		// a GitError.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 128 {
			return "", nil
		}
		return "", gerr.GitError(err, args...)
	}
	return strings.TrimRight(string(stdout), "\n"), nil
}

// GraphDOT implements Adapter. Walks `git log --pretty=%H %P %d`
// filtered by opts and emits a Graphviz DOT digraph. Each commit
// is a node labelled with its short SHA; parent edges are
// directed; decorations (branches, tags, HEAD) are emitted as
// record fields on the node label.
func (a *ExecAdapter) GraphDOT(ctx context.Context, opts GraphOpts) (string, error) {
	args := []string{"log", "--pretty=format:%H%x00%P%x00%D", "--no-color"}
	if opts.All {
		args = append(args, "--all")
	}
	if opts.MaxCount > 0 {
		args = append(args, "-n", strconv.Itoa(opts.MaxCount))
	} else if opts.MaxCount == 0 {
		args = append(args, "-n", "200")
	}
	if opts.Since != "" {
		args = append(args, "--since="+opts.Since)
	}
	if opts.Until != "" {
		args = append(args, "--until="+opts.Until)
	}
	if opts.Author != "" {
		args = append(args, "--author="+opts.Author)
	}
	if opts.Grep != "" {
		args = append(args, "--grep="+opts.Grep)
	}
	stdout, _, err := a.run(ctx, args...)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 128 {
			return renderEmptyDOT(), nil
		}
		return "", gerr.GitError(err, args...)
	}
	return renderDOT(string(stdout))
}

// renderEmptyDOT returns a placeholder DOT graph for an empty repo
// (no commits, or no commits matching the filter).
func renderEmptyDOT() string {
	return "digraph got_commit_graph {\n" +
		"  // no commits in this repository (or no commits match the filter)\n" +
		"  empty [label=\"(no commits)\"];\n" +
		"}\n"
}

// renderDOT parses the output of `git log --pretty=%H %P %d` and
// builds a DOT digraph. Each input line is three NUL-separated
// fields: full SHA, space-separated parent SHAs, comma-separated
// decoration labels (e.g. "HEAD -> main, origin/main, tag: v1.0").
func renderDOT(raw string) (string, error) {
	var buf bytes.Buffer
	buf.WriteString("digraph got_commit_graph {\n")
	buf.WriteString("  node [shape=record, fontname=\"monospace\"];\n")
	buf.WriteString("  rankdir=BT;\n")
	sc := bufio.NewScanner(strings.NewReader(raw))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	seen := make(map[string]bool)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x00", 3)
		if len(parts) < 2 {
			continue
		}
		sha := parts[0]
		parents := strings.Fields(parts[1])
		deco := ""
		if len(parts) == 3 {
			deco = parts[2]
		}
		if seen[sha] {
			continue
		}
		seen[sha] = true
		label := buildDOTNodeLabel(sha, deco)
		fmt.Fprintf(&buf, "  %q [label=%s];\n", shortSHA(sha), label)
		for _, p := range parents {
			if p == "" {
				continue
			}
			fmt.Fprintf(&buf, "  %q -> %q;\n", shortSHA(sha), shortSHA(p))
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	buf.WriteString("}\n")
	return buf.String(), nil
}

// buildDOTNodeLabel returns a DOT label for a commit node. The
// shape is `{ <short-sha> | <decorations> }` so Graphviz renders
// the decorations in a second row.
func buildDOTNodeLabel(sha, deco string) string {
	short := shortSHA(sha)
	if deco == "" {
		return fmt.Sprintf("%q", short)
	}
	// Comma-separate the decoration parts into a single label cell.
	cleaned := strings.TrimSpace(deco)
	parts := strings.Split(cleaned, ", ")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	joined := strings.Join(parts, "\\n")
	return fmt.Sprintf("{ %q | %q }", short, joined)
}

// shortSHA returns the first 7 hex characters of a full SHA. Git
// itself uses 7 by default for short SHAs unless core.abbrev is
// configured; we hard-code 7 because DOT consumers (Graphviz, etc.)
// benefit from stable labels.
func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
}
