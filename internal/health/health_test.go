package health

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/got-sh/got/internal/git"
)

// fakeAdapter is a minimal in-memory git.Adapter for the
// health package tests. It returns canned branches, remotes,
// status, and log streams.
type fakeAdapter struct {
	branches       []git.Branch
	remoteBranches []git.Branch
	remotes        []git.Remote
	status         git.Status
	commits        []git.Commit
	logErr         error
}

func (f *fakeAdapter) Status(ctx context.Context) (git.Status, error) { return f.status, nil }
func (f *fakeAdapter) Commit(ctx context.Context, msg string, opts git.CommitOpts) (git.SHA, error) {
	return "", nil
}

func (f *fakeAdapter) Branches(ctx context.Context) ([]git.Branch, error) {
	if f.logErr != nil {
		return nil, f.logErr
	}
	return f.branches, nil
}

func (f *fakeAdapter) RemoteBranches(ctx context.Context) ([]git.Branch, error) {
	return f.remoteBranches, nil
}

func (f *fakeAdapter) Remotes(ctx context.Context) ([]git.Remote, error) {
	return f.remotes, nil
}

func (f *fakeAdapter) Checkout(ctx context.Context, ref string, opts git.CheckoutOpts) error {
	return nil
}

func (f *fakeAdapter) Merge(ctx context.Context, ref string, opts git.MergeOpts) error { return nil }

func (f *fakeAdapter) Reset(ctx context.Context, target string, mode git.ResetMode) error {
	return nil
}
func (f *fakeAdapter) Fetch(ctx context.Context, remote string) error { return nil }
func (f *fakeAdapter) Push(ctx context.Context, remote, branch string, opts git.PushOpts) error {
	return nil
}

func (f *fakeAdapter) Log(ctx context.Context, rangeStr string, format git.LogFormat) (io.Reader, error) {
	if f.logErr != nil {
		return nil, f.logErr
	}
	return &commitStream{commits: f.commits}, nil
}
func (f *fakeAdapter) CurrentRef(ctx context.Context) (string, error)    { return "", nil }
func (f *fakeAdapter) Stage(ctx context.Context, paths []string) error   { return nil }
func (f *fakeAdapter) Unstage(ctx context.Context, paths []string) error { return nil }
func (f *fakeAdapter) StageAllTracked(ctx context.Context) error         { return nil }
func (f *fakeAdapter) CreateBranch(ctx context.Context, name, startPoint string) error {
	return nil
}

func (f *fakeAdapter) DeleteBranch(ctx context.Context, name string, force bool) error {
	return nil
}
func (f *fakeAdapter) RemoteAdd(ctx context.Context, name, url string) error { return nil }
func (f *fakeAdapter) RemoteRemove(ctx context.Context, name string, force bool) error {
	return nil
}

func (f *fakeAdapter) RemoteRename(ctx context.Context, oldName, newName string) error {
	return nil
}

func (f *fakeAdapter) RemoteSetURL(ctx context.Context, name, url string, pushURL bool) error {
	return nil
}
func (f *fakeAdapter) RemotePrune(ctx context.Context, name string) error  { return nil }
func (f *fakeAdapter) FetchPrune(ctx context.Context, remote string) error { return nil }
func (f *fakeAdapter) FetchAll(ctx context.Context, prune bool) error      { return nil }
func (f *fakeAdapter) GraphASCII(ctx context.Context, opts git.GraphOpts) (string, error) {
	return "", nil
}

func (f *fakeAdapter) GraphDOT(ctx context.Context, opts git.GraphOpts) (string, error) {
	return "", nil
}

func (f *fakeAdapter) WorktreeList(ctx context.Context) ([]git.Worktree, error) {
	return nil, nil
}

func (f *fakeAdapter) WorktreeAdd(ctx context.Context, path string, opts git.WorktreeAddOpts) error {
	return nil
}

func (f *fakeAdapter) WorktreeRemove(ctx context.Context, path string, force bool) error {
	return nil
}
func (f *fakeAdapter) WorktreeLock(ctx context.Context, path, reason string) error { return nil }
func (f *fakeAdapter) WorktreeUnlock(ctx context.Context, path string) error       { return nil }
func (f *fakeAdapter) WorktreePrune(ctx context.Context) error                     { return nil }

type commitStream struct {
	commits []git.Commit
	pos     int
}

func (s *commitStream) Read(p []byte) (int, error) {
	if s.pos >= len(s.commits) {
		return 0, io.EOF
	}
	c := s.commits[s.pos]
	s.pos++
	line := []byte(fmt.Sprintf(`{"sha":%q,"author":%q,"email":%q,"timestamp":%q}`,
		c.SHA, c.Author, c.Email, c.Timestamp.Format(time.RFC3339)))
	line = append(line, '\n')
	if len(p) < len(line) {
		copy(p, line[:len(p)])
		return len(p), nil
	}
	copy(p, line)
	return len(line), nil
}

var errEOF = fakeEOF{}

type fakeEOF struct{}

func (fakeEOF) Error() string { return "EOF" }

func writeFile(t *testing.T, root, rel string, body []byte) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestCheck_StaleBranches(t *testing.T) {
	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -200) // 200 days ago
	recent := now.AddDate(0, 0, -10)
	dir := t.TempDir()
	a := &fakeAdapter{
		branches: []git.Branch{
			{Name: "main", IsCurrent: true, SHA: "abc", CommitAt: recent},
			{Name: "stale1", SHA: "def", CommitAt: old},
			{Name: "stale2", SHA: "ghi", CommitAt: old},
		},
	}
	c := New(dir, a)
	c.Now = func() time.Time { return now }
	r, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	f := findFinding(r, "stale-branches")
	if f == nil {
		t.Fatalf("stale-branches finding missing: %+v", r.Findings)
	}
	if f.Severity != SeverityLow {
		t.Errorf("Severity = %q, want %q", f.Severity, SeverityLow)
	}
	if len(f.Affected) != 2 {
		t.Errorf("Affected = %v, want 2 entries", f.Affected)
	}
}

func TestCheck_ExcessiveBranches(t *testing.T) {
	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	branches := make([]git.Branch, 60)
	for i := range branches {
		branches[i] = git.Branch{
			Name: fmt.Sprintf("branch-%d", i), SHA: fmt.Sprintf("sha%d", i),
			CommitAt: now,
		}
	}
	branches[0] = git.Branch{Name: "main", IsCurrent: true, SHA: "main-sha", CommitAt: now}
	dir := t.TempDir()
	a := &fakeAdapter{branches: branches}
	c := New(dir, a)
	c.Now = func() time.Time { return now }
	c.Thresholds.MaxBranches = 50
	r, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	f := findFinding(r, "excessive-branches")
	if f == nil {
		t.Fatalf("excessive-branches finding missing: %+v", r.Findings)
	}
}

func TestCheck_MissingDocs(t *testing.T) {
	dir := t.TempDir()
	// No README, no LICENSE.
	a := &fakeAdapter{}
	c := New(dir, a)
	c.Now = func() time.Time { return time.Now() }
	r, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if findFinding(r, "missing-readme") == nil {
		t.Errorf("missing-readme finding missing")
	}
	if findFinding(r, "missing-license") == nil {
		t.Errorf("missing-license finding missing")
	}
	if findFinding(r, "missing-changelog") == nil {
		t.Errorf("missing-changelog finding missing")
	}
	// Add a README; the finding should disappear.
	writeFile(t, dir, "README.md", []byte("# Title\n\nContent.\n"))
	r, err = c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if findFinding(r, "missing-readme") != nil {
		t.Errorf("missing-readme should be gone after adding README.md")
	}
}

func TestCheck_LargeBinaries(t *testing.T) {
	dir := t.TempDir()
	// 2 MiB binary.
	big := make([]byte, 2<<20)
	for i := range big {
		big[i] = 0xFF
	}
	writeFile(t, dir, "seed.png", big)
	writeFile(t, dir, "small.png", []byte{0x89, 'P', 'N', 'G'})
	a := &fakeAdapter{}
	c := New(dir, a)
	c.Now = func() time.Time { return time.Now() }
	r, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	f := findFinding(r, "large-binaries")
	if f == nil {
		t.Fatalf("large-binaries finding missing: %+v", r.Findings)
	}
	if len(f.Affected) != 1 {
		t.Errorf("Affected = %v, want 1 (small.png should be excluded)", f.Affected)
	}
	if f.Affected[0] != "seed.png" {
		t.Errorf("Affected[0] = %q, want seed.png", f.Affected[0])
	}
}

func TestCheck_LargeBinaries_SkipsNodeModules(t *testing.T) {
	dir := t.TempDir()
	big := make([]byte, 2<<20)
	for i := range big {
		big[i] = 0xFF
	}
	writeFile(t, dir, "node_modules/foo/big.png", big)
	a := &fakeAdapter{}
	c := New(dir, a)
	c.Now = func() time.Time { return time.Now() }
	r, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if findFinding(r, "large-binaries") != nil {
		t.Errorf("large-binaries should skip node_modules: %+v", r.Findings)
	}
}

func TestCheck_UnreachableRemotes(t *testing.T) {
	dir := t.TempDir()
	a := &fakeAdapter{
		remotes: []git.Remote{
			{Name: "origin", FetchURL: "https://github.com/foo/bar.git"},
			{Name: "stale", FetchURL: "https://example.com/missing.git"},
		},
		remoteBranches: []git.Branch{
			{Name: "origin/main", SHA: "abc"},
		},
	}
	c := New(dir, a)
	c.Now = func() time.Time { return time.Now() }
	r, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	f := findFinding(r, "unreachable-remotes")
	if f == nil {
		t.Fatalf("unreachable-remotes finding missing: %+v", r.Findings)
	}
	if len(f.Affected) != 1 || f.Affected[0] != "stale" {
		t.Errorf("Affected = %v, want [stale]", f.Affected)
	}
}

func TestCheck_MalformedRemoteURL(t *testing.T) {
	dir := t.TempDir()
	a := &fakeAdapter{
		remotes: []git.Remote{
			{Name: "origin", FetchURL: "https://github.com/foo/bar.git"},
			{Name: "broken", FetchURL: "not a url with spaces and stuff"},
		},
		remoteBranches: []git.Branch{
			{Name: "origin/main", SHA: "abc"},
		},
	}
	c := New(dir, a)
	c.Now = func() time.Time { return time.Now() }
	r, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	f := findFinding(r, "malformed-remote-urls")
	if f == nil {
		t.Fatalf("malformed-remote-urls finding missing: %+v", r.Findings)
	}
}

func TestCheck_WorkingTreeDirty(t *testing.T) {
	dir := t.TempDir()
	a := &fakeAdapter{
		status: git.Status{
			Branch: "main",
			Entries: []git.StatusEntry{
				{Path: "staged.go", XY: "M ", IsStaged: true},
				{Path: "untracked.txt", XY: "??", IsUntracked: true},
			},
		},
	}
	c := New(dir, a)
	c.Now = func() time.Time { return time.Now() }
	r, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if findFinding(r, "working-tree-dirty") == nil {
		t.Errorf("working-tree-dirty finding missing")
	}
	if findFinding(r, "untracked-files") == nil {
		t.Errorf("untracked-files finding missing")
	}
}

func TestCheck_RecommendationsForFindings(t *testing.T) {
	dir := t.TempDir()
	// No README → produces a "missing-readme" critical finding
	// and a recommendation.
	a := &fakeAdapter{}
	c := New(dir, a)
	c.Now = func() time.Time { return time.Now() }
	r, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(r.Recommendations) == 0 {
		t.Fatalf("no recommendations produced")
	}
	// First recommendation should be the README one (Priority=1).
	if !strings.Contains(r.Recommendations[0].Title, "README") {
		t.Errorf("first recommendation = %q, want one mentioning README", r.Recommendations[0].Title)
	}
}

func TestCheck_PerfectScore(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", []byte("# Title\n"))
	writeFile(t, dir, "LICENSE", []byte("MIT License\n"))
	writeFile(t, dir, "CHANGELOG.md", []byte("# Changelog\n"))
	writeFile(t, dir, "CONTRIBUTING.md", []byte("# Contributing\n"))
	a := &fakeAdapter{
		branches: []git.Branch{
			{Name: "main", IsCurrent: true, SHA: "abc", CommitAt: time.Now()},
		},
		status: git.Status{Branch: "main"},
	}
	c := New(dir, a)
	c.Now = func() time.Time { return time.Now() }
	r, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if r.Score != 100 {
		t.Errorf("Score = %d, want 100 (findings: %+v)", r.Score, r.Findings)
	}
	if r.Grade != "A+" {
		t.Errorf("Grade = %q, want A+", r.Grade)
	}
}

func TestCheck_ScoreFromFindings(t *testing.T) {
	tests := []struct {
		name     string
		findings []HealthFinding
		minScore int
		maxScore int
	}{
		{
			name:     "no findings",
			findings: nil,
			minScore: 100, maxScore: 100,
		},
		{
			name:     "one low",
			findings: []HealthFinding{{Severity: SeverityLow}},
			minScore: 99, maxScore: 99,
		},
		{
			name:     "one critical",
			findings: []HealthFinding{{Severity: SeverityCritical}},
			minScore: 75, maxScore: 75,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := scoreFromFindings(tt.findings)
			if s < tt.minScore || s > tt.maxScore {
				t.Errorf("score = %d, want in [%d,%d]", s, tt.minScore, tt.maxScore)
			}
		})
	}
}

func findFinding(r HealthReport, id string) *HealthFinding {
	for i := range r.Findings {
		if r.Findings[i].ID == id {
			return &r.Findings[i]
		}
	}
	return nil
}
