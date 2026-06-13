package health

import (
	"context"
	"fmt"
)

// checkCleanliness inspects the working tree status and reports
// uncommitted changes and untracked files. The check uses the
// Status method on the git adapter, which returns one
// StatusEntry per dirty file.
//
// The findings are intentionally mild: a working tree with
// uncommitted changes is normal during development. The check
// only flags a *long* uncommitted state (many untracked files,
// suggesting forgotten scratch files) and leaves the rest to
// the user's own awareness.
func checkCleanliness(ctx context.Context, c *Checker) ([]HealthFinding, error) {
	if c.Adapter == nil {
		return nil, nil
	}
	st, err := c.Adapter.Status(ctx)
	if err != nil {
		return nil, err
	}
	var staged, unstaged, untracked []string
	for _, e := range st.Entries {
		switch {
		case e.IsUntracked:
			untracked = append(untracked, e.Path)
		case e.IsStaged:
			staged = append(staged, e.Path)
		default:
			unstaged = append(unstaged, e.Path)
		}
	}
	var findings []HealthFinding
	if len(staged)+len(unstaged) > 0 {
		affected := append(staged, unstaged...)
		findings = append(findings, HealthFinding{
			ID:       "working-tree-dirty",
			Category: CategoryCleanliness,
			Severity: SeverityLow,
			Title:    fmt.Sprintf("%d uncommitted change(s)", len(affected)),
			Detail:   "The working tree has uncommitted changes. Commit them to a feature branch or stash them for later.",
			Affected: affected,
		})
	}
	if len(untracked) > 0 {
		sev := SeverityLow
		switch {
		case len(untracked) >= 20:
			sev = SeverityMedium
		case len(untracked) >= 50:
			sev = SeverityHigh
		}
		findings = append(findings, HealthFinding{
			ID:       "untracked-files",
			Category: CategoryCleanliness,
			Severity: sev,
			Title:    fmt.Sprintf("%d untracked file(s)", len(untracked)),
			Detail:   "Untracked files pile up over time. Either commit them (when they're meant to be in the repo) or add them to .gitignore.",
			Affected: untracked,
		})
	}
	return findings, nil
}

// scanCommitSHAs is a small helper that reads NDJSON-encoded
// commits from r and returns the set of their SHAs. It's
// factored out so the test suite can drive it with a
// strings.Reader without going through the git adapter.
func scanCommitSHAs(r interface{ Read(p []byte) (int, error) }) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	buf := make([]byte, 64*1024)
	var carry []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			carry = append(carry, buf[:n]...)
			// Split on newlines, parse each line.
			for {
				idx := -1
				for i, b := range carry {
					if b == '\n' {
						idx = i
						break
					}
				}
				if idx < 0 {
					break
				}
				line := carry[:idx]
				carry = carry[idx+1:]
				if len(line) == 0 {
					continue
				}
				// Strip optional \r.
				if line[len(line)-1] == '\r' {
					line = line[:len(line)-1]
				}
				sha, ok := extractSHAFromNDJSON(line)
				if ok {
					out[sha] = struct{}{}
				}
			}
		}
		if err != nil {
			break
		}
	}
	// Flush trailing line (no newline).
	if len(carry) > 0 {
		if sha, ok := extractSHAFromNDJSON(carry); ok {
			out[sha] = struct{}{}
		}
	}
	return out, nil
}

// extractSHAFromNDJSON pulls the "sha" field out of one
// NDJSON-encoded commit line. The format is set by the git
// adapter's Log formatter; we only depend on the SHA field.
func extractSHAFromNDJSON(line []byte) (string, bool) {
	key := []byte(`"sha":"`)
	idx := indexBytes(line, key)
	if idx < 0 {
		return "", false
	}
	start := idx + len(key)
	end := start
	for end < len(line) && line[end] != '"' {
		end++
	}
	if end >= len(line) {
		return "", false
	}
	return string(line[start:end]), true
}

// indexBytes is bytes.IndexByte with a small allocation
// tweak: we inline the search to avoid the function call in a
// hot loop. Equivalent to bytes.IndexByte(s, sub[0]) when sub
// is a single byte — but here we want a multi-byte prefix.
func indexBytes(s, sub []byte) int {
	if len(sub) == 0 {
		return 0
	}
	if len(sub) > len(s) {
		return -1
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
