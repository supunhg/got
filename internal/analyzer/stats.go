package analyzer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/got-sh/got/internal/git"
)

// computeStats derives RepositoryStats from the git adapter and
// the file walk. It is the only step of Analyze that talks to git.
//
// Statistics:
//
//   - CommitCount: number of commits reachable from any ref,
//     via `git log --all` parsed NDJSON. We don't have a direct
//     "count" method on the adapter, but Log returns one commit
//     per line so we just count lines.
//
//   - BranchCount / RemoteBranchCount: from adapter.Branches() /
//     RemoteBranches(). Each returns one Branch per ref.
//
//   - ContributorCount: from the same log, counting distinct
//     (email) values.
//
//   - FileCount / LineCount / SizeBytes: from the file walk.
//     Binary files count toward FileCount and SizeBytes only;
//     text files also bump LineCount.
//
//   - FirstCommitAt / LastCommitAt: the earliest and latest
//     commit timestamps in the log. Done in a single pass.
//
// The function is best-effort: a git error (corrupt index,
// unborn HEAD, etc.) is returned, but the caller wraps it in
// TypeReason rather than failing the whole analysis.
func computeStats(ctx context.Context, a git.Adapter, dc DetectionContext) (RepositoryStats, error) {
	out := RepositoryStats{}

	// File-system stats. Walk the file list and accumulate.
	// We re-read file sizes rather than caching the Lstat from
	// the walker (the walker is internal; this keeps the
	// public API simple).
	var totalBytes int64
	var totalLines int
	for _, rel := range dc.Files {
		full, err := resolveUnderRoot(dc.WorkTree, rel)
		if err != nil {
			continue
		}
		size := fileSize(full)
		totalBytes += size
		out.FileCount++
		if isTextFile(full) && !binaryExts[strings.ToLower(extOf(rel))] {
			data, err := readFile(dc.WorkTree, rel)
			if err == nil {
				totalLines += countLines(data)
			}
		}
	}
	out.LineCount = totalLines
	out.SizeBytes = totalBytes

	// Git-derived stats. A nil adapter (test stub) yields zero
	// values without an error.
	if a == nil {
		return out, nil
	}

	// Branches: count local + remote.
	branches, err := a.Branches(ctx)
	if err != nil {
		return out, fmt.Errorf("branches: %w", err)
	}
	out.BranchCount = len(branches)
	remoteBranches, err := a.RemoteBranches(ctx)
	if err == nil {
		out.RemoteBranchCount = len(remoteBranches)
	}

	// Commits + contributors + first/last. A single Log call
	// gets us everything (we want the all-refs graph).
	rdr, err := a.Log(ctx, "", git.LogFormatNDJSON)
	if err != nil {
		// Empty repos (no commits) cause "fatal: bad default
		// upstream" or similar errors. Treat as "no commits".
		if isLikelyEmptyRepoError(err) {
			return out, nil
		}
		return out, fmt.Errorf("log: %w", err)
	}
	defer func() {
		if c, ok := rdr.(io.Closer); ok {
			_ = c.Close()
		}
	}()

	scanner := bufio.NewScanner(rdr)
	// Increase the scanner buffer to handle large commit subjects
	// and refs lines. The default 64KiB is fine for v0.1; bump
	// if a real-world repo hits the limit.
	contributors := make(map[string]struct{})
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var c git.Commit
		if err := json.Unmarshal(line, &c); err != nil {
			// Skip malformed lines rather than aborting
			// the whole count — a single broken entry
			// shouldn't blind us to the rest.
			continue
		}
		out.CommitCount++
		// Contributors are de-duplicated by email (lowercased).
		email := strings.ToLower(strings.TrimSpace(c.Email))
		if email == "" {
			// Fall back to author name when email is missing.
			email = strings.ToLower(strings.TrimSpace(c.Author))
		}
		if email != "" {
			contributors[email] = struct{}{}
		}
		// First / last commit timestamps.
		if !c.Timestamp.IsZero() {
			if out.FirstCommitAt.IsZero() || c.Timestamp.Before(out.FirstCommitAt) {
				out.FirstCommitAt = c.Timestamp
			}
			if c.Timestamp.After(out.LastCommitAt) {
				out.LastCommitAt = c.Timestamp
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return out, fmt.Errorf("scan log: %w", err)
	}
	out.ContributorCount = len(contributors)
	return out, nil
}

// extOf returns the lowercased extension of rel, including the
// leading dot. Returns "" when there is no extension.
func extOf(rel string) string {
	idx := strings.LastIndex(rel, ".")
	if idx < 0 {
		return ""
	}
	// Reject dotfiles with no real extension (".gitignore" has
	// no extension by this rule: the dot is the first char and
	// there's no character after it).
	if idx == 0 {
		return ""
	}
	return strings.ToLower(rel[idx:])
}

// isLikelyEmptyRepoError reports whether err is the kind of
// error git returns for a brand-new repo (no commits) or a
// shallow clone with no reflog. We don't want to surface these
// as fatal — "you have zero commits" is a perfectly valid state.
func isLikelyEmptyRepoError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "bad default upstream"):
		return true
	case strings.Contains(msg, "fatal: ambiguous argument 'HEAD'"):
		return true
	case strings.Contains(msg, "does not have any commits yet"):
		return true
	case strings.Contains(msg, "Your current branch does not have any commits yet"):
		return true
	}
	return false
}

// sortedContributors returns the contributor emails sorted
// alphabetically. Used by tests and downstream tools that want
// a stable list.
func sortedContributors(in map[string]struct{}) []string {
	out := make([]string, 0, len(in))
	for k := range in {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
