package health

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// checkRemotes reports three classes of remote problems:
//
//  1. The remote's URL is malformed (e.g. starts with a
//     whitespace character, has unbalanced brackets, etc.).
//     This is almost always a copy-paste mistake.
//
//  2. The remote has no tracking branches — it was added
//     (`git remote add`) but never fetched. Often the URL
//     turned out to be unreachable and the user never noticed.
//
//  3. The remote points to a URL scheme that the user almost
//     certainly no longer uses (e.g. `git://` to a service
//     that has been HTTPS-only for years).
//
// The check is offline-only: it does not attempt to contact
// the remote. Network probes (e.g. `git ls-remote`) are out
// of scope for v0.1 per the offline-first design.
func checkRemotes(ctx context.Context, c *Checker) ([]HealthFinding, error) {
	if c.Adapter == nil {
		return nil, nil
	}
	remotes, err := c.Adapter.Remotes(ctx)
	if err != nil {
		return nil, err
	}
	remoteBranches, _ := c.Adapter.RemoteBranches(ctx)
	tracked := make(map[string]bool, len(remoteBranches))
	for _, b := range remoteBranches {
		// Remote branch names look like "origin/main"; the
		// prefix is the remote name. We consider a remote
		// "tracked" when it has at least one branch.
		slash := strings.IndexByte(b.Name, '/')
		if slash < 0 {
			continue
		}
		tracked[b.Name[:slash]] = true
	}

	var findings []HealthFinding
	var unused []string
	var malformed []string
	for _, r := range remotes {
		if !tracked[r.Name] {
			unused = append(unused, r.Name)
		}
		if !looksLikeURL(r.FetchURL) {
			malformed = append(malformed, r.Name+": "+r.FetchURL)
		}
	}
	if len(unused) > 0 {
		sort.Strings(unused)
		findings = append(findings, HealthFinding{
			ID:       "unreachable-remotes",
			Category: CategoryRemotes,
			Severity: severityForCount(len(unused), 1, 3, 5),
			Title:    fmt.Sprintf("%d unused remote(s)", len(unused)),
			Detail:   "These remotes are configured but have no tracking branches — they have never been fetched. Either fetch them once (`git fetch <name>`) or remove them with `git remote remove`:",
			Affected: unused,
		})
	}
	if len(malformed) > 0 {
		sort.Strings(malformed)
		findings = append(findings, HealthFinding{
			ID:       "malformed-remote-urls",
			Category: CategoryRemotes,
			Severity: SeverityHigh,
			Title:    fmt.Sprintf("%d remote(s) with malformed URLs", len(malformed)),
			Detail:   "These remotes have URLs that do not parse as a valid git URL. They will fail on every fetch / push:",
			Affected: malformed,
		})
	}
	return findings, nil
}

// looksLikeURL is a deliberately lenient check: it just
// verifies the URL is non-empty, has a recognized scheme, and
// has no whitespace. Anything more strict would generate false
// positives for unusual-but-valid URLs (e.g. SSH paths with
// unusual port numbers).
func looksLikeURL(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.ContainsAny(s, " \t\n\r") {
		return false
	}
	recognized := []string{
		"https://", "http://", "ssh://", "git://", "file://", "ftp://", "ftps://",
	}
	for _, prefix := range recognized {
		if strings.HasPrefix(s, prefix) {
			return len(s) > len(prefix)
		}
	}
	// SSH "user@host:path" form (no scheme).
	if strings.Contains(s, "@") && strings.Contains(s, ":") && !strings.Contains(s, "://") {
		// Looks like an SSH URL. Validate the user@host part.
		at := strings.IndexByte(s, '@')
		colon := strings.IndexByte(s, ':')
		if at > 0 && colon > at {
			return true
		}
	}
	// Local path form: /abs/path or relative/path.
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return true
	}
	return false
}
