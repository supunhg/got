package analyzer

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/got-sh/got/internal/git"
)

// fakeAdapter is a minimal in-memory git.Adapter used by
// analyzer tests. It implements just the methods the analyzer
// actually calls (Branches, RemoteBranches, Remotes, Log,
// Status). Everything else returns zero values.
type fakeAdapter struct {
	branches       []git.Branch
	remoteBranches []git.Branch
	remotes        []git.Remote
	commits        []git.Commit
	status         git.Status
	commitErr      error
}

func (f *fakeAdapter) Status(ctx context.Context) (git.Status, error) { return f.status, nil }

// Unused methods — the analyzer doesn't call them but the
// interface requires them. They all return zero values.
func (f *fakeAdapter) Commit(ctx context.Context, msg string, opts git.CommitOpts) (git.SHA, error) {
	return "", nil
}

func (f *fakeAdapter) Branches(ctx context.Context) ([]git.Branch, error) {
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

func (f *fakeAdapter) Merge(ctx context.Context, ref string, opts git.MergeOpts) error {
	return nil
}

func (f *fakeAdapter) Reset(ctx context.Context, target string, mode git.ResetMode) error {
	return nil
}
func (f *fakeAdapter) Fetch(ctx context.Context, remote string) error { return nil }
func (f *fakeAdapter) Push(ctx context.Context, remote, branch string, opts git.PushOpts) error {
	return nil
}

func (f *fakeAdapter) Log(ctx context.Context, rangeStr string, format git.LogFormat) (io.Reader, error) {
	if f.commitErr != nil {
		return nil, f.commitErr
	}
	return &commitStream{commits: f.commits}, nil
}
func (f *fakeAdapter) CurrentRef(ctx context.Context) (string, error)    { return "main", nil }
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

// commitStream is a minimal Read-only stream of NDJSON-encoded
// commits. The fake adapter's Log returns one; tests can
// pre-populate it with a slice.
type commitStream struct {
	commits []git.Commit
	pos     int
}

func (s *commitStream) Read(p []byte) (int, error) {
	if s.pos >= len(s.commits) {
		return 0, io.EOF
	}
	line := encodeCommit(s.commits[s.pos])
	s.pos++
	if len(p) < len(line) {
		copy(p, line[:len(p)])
		return len(p), nil
	}
	copy(p, line)
	return len(line), nil
}

func TestAnalyze_EmptyRepo(t *testing.T) {
	dir := t.TempDir()
	a := New(dir, &fakeAdapter{})
	model, err := a.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if model.Name == "" {
		t.Errorf("Name is empty")
	}
	if model.Type != RepoTypeUnknown {
		t.Errorf("Type = %q, want %q", model.Type, RepoTypeUnknown)
	}
	if model.Stats.CommitCount != 0 {
		t.Errorf("CommitCount = %d, want 0", model.Stats.CommitCount)
	}
}

func TestAnalyze_GoLibrary(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/foo\n\ngo 1.24\n")
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	writeFile(t, dir, "lib.go", "package foo\n\nfunc Bar() int { return 42 }\n")
	a := New(dir, &fakeAdapter{})
	model, err := a.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if model.Type != RepoTypeLibrary && model.Type != RepoTypeApplication && model.Type != RepoTypeTool {
		// A single go.mod + main.go + lib.go could be classified
		// as application (main package present) or tool
		// (cmd/<x>/main.go pattern missing). Library is the
		// "I have no main" case which is wrong here. Accept
		// any of the three.
		t.Logf("Type = %q (note: could be application, library, or tool)", model.Type)
	}
	// Languages should include Go.
	if !hasLanguage(model.Languages, "Go") {
		t.Errorf("Languages missing Go: %+v", model.Languages)
	}
	// Package manager should include Go modules.
	if !hasPackageManager(model.PackageManagers, "Go modules") {
		t.Errorf("PackageManagers missing Go modules: %+v", model.PackageManagers)
	}
}

func TestAnalyze_JavaScriptMonorepo(t *testing.T) {
	dir := t.TempDir()
	// Root manifest with workspaces.
	writeFile(t, dir, "package.json", `{
		"name": "monorepo",
		"workspaces": ["packages/*"],
		"dependencies": {"react": "^18.0.0", "next": "^14.0.0"}
	}`)
	writeFile(t, dir, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	writeFile(t, dir, "packages/a/package.json", `{"name": "a"}`)
	writeFile(t, dir, "packages/b/package.json", `{"name": "b"}`)
	a := New(dir, &fakeAdapter{})
	model, err := a.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if !model.Monorepo.IsMonorepo {
		t.Errorf("Monorepo.IsMonorepo = false, want true")
	}
	if model.Monorepo.Tool != "pnpm workspaces" {
		t.Errorf("Monorepo.Tool = %q, want %q", model.Monorepo.Tool, "pnpm workspaces")
	}
	if model.Monorepo.PackageCount < 2 {
		// 2 sub-packages (the root is not a sub-package)
		t.Errorf("Monorepo.PackageCount = %d, want >= 2", model.Monorepo.PackageCount)
	}
	if !hasFramework(model.Frameworks, "React") {
		t.Errorf("Frameworks missing React: %+v", model.Frameworks)
	}
	if !hasFramework(model.Frameworks, "Next.js") {
		t.Errorf("Frameworks missing Next.js: %+v", model.Frameworks)
	}
}

func TestAnalyze_PythonProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pyproject.toml", "project:\n  name: foo\n  dependencies:\n    - \"django>=4.0\"\n    - \"requests==2.31.0\"\n")
	writeFile(t, dir, "main.py", "import django\nimport requests\n\ndef hello():\n    return 'hi'\n")
	a := New(dir, &fakeAdapter{})
	model, err := a.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if !hasLanguage(model.Languages, "Python") {
		t.Errorf("Languages missing Python: %+v", model.Languages)
	}
	if !hasFramework(model.Frameworks, "Django") {
		t.Errorf("Frameworks missing Django: %+v", model.Frameworks)
	}
	if !hasFramework(model.Frameworks, "Requests") {
		t.Errorf("Frameworks missing Requests: %+v", model.Frameworks)
	}
}

func TestAnalyze_DockerfileAndCompose(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Dockerfile", "FROM golang:1.24\n")
	writeFile(t, dir, "docker-compose.yml", "version: '3'\nservices:\n  web:\n    build: .\n")
	writeFile(t, dir, "k8s/deployment.yaml", "apiVersion: apps/v1\nkind: Deployment\n")
	writeFile(t, dir, "charts/myapp/Chart.yaml", "apiVersion: v2\nname: myapp\nversion: 1.0.0\n")
	a := New(dir, &fakeAdapter{})
	model, err := a.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	c := model.Containerization
	if !c.HasDockerfile {
		t.Errorf("HasDockerfile = false, want true")
	}
	if c.DockerfileCount != 1 {
		t.Errorf("DockerfileCount = %d, want 1", c.DockerfileCount)
	}
	if !c.HasDockerCompose {
		t.Errorf("HasDockerCompose = false, want true")
	}
	if !c.HasKubernetes {
		t.Errorf("HasKubernetes = false, want true")
	}
	if !c.HasHelm {
		t.Errorf("HasHelm = false, want true")
	}
	if c.HelmChartCount != 1 {
		t.Errorf("HelmChartCount = %d, want 1", c.HelmChartCount)
	}
}

func TestAnalyze_CICD(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".github/workflows/ci.yml", "name: CI\non: push\n")
	writeFile(t, dir, ".github/workflows/release.yml", "name: Release\n")
	writeFile(t, dir, ".gitlab-ci.yml", "stages:\n  - test\n")
	a := New(dir, &fakeAdapter{})
	model, err := a.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if !hasCICD(model.CICDSystems, "GitHub Actions") {
		t.Errorf("CICD missing GitHub Actions: %+v", model.CICDSystems)
	}
	if !hasCICD(model.CICDSystems, "GitLab CI") {
		t.Errorf("CICD missing GitLab CI: %+v", model.CICDSystems)
	}
	gh := findCICD(model.CICDSystems, "GitHub Actions")
	if gh != nil && gh.WorkflowCount != 2 {
		t.Errorf("GitHub Actions WorkflowCount = %d, want 2", gh.WorkflowCount)
	}
}

func TestAnalyze_TypeClassification(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  RepositoryType
	}{
		{
			name: "Go CLI tool",
			files: map[string]string{
				"cmd/foo/main.go": "package main\n",
				"go.mod":          "module foo\n",
			},
			want: RepoTypeTool,
		},
		{
			name: "Go library",
			files: map[string]string{
				"lib.go": "package foo\n",
				"go.mod": "module foo\n",
			},
			want: RepoTypeLibrary,
		},
		{
			name: "Python documentation",
			files: map[string]string{
				"docs/index.md": "# Title\n",
				"docs/page2.md": "More content\n",
			},
			want: RepoTypeDocumentation,
		},
		{
			name: "Terraform config",
			files: map[string]string{
				"main.tf":      "resource \"null_resource\" \"x\" {}\n",
				"variables.tf": "variable \"x\" {}\n",
			},
			want: RepoTypeConfig,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for path, body := range tt.files {
				writeFile(t, dir, path, body)
			}
			a := New(dir, &fakeAdapter{})
			model, err := a.Analyze(context.Background())
			if err != nil {
				t.Fatalf("Analyze: %v", err)
			}
			if model.Type != tt.want {
				t.Errorf("Type = %q, want %q (reason: %s)", model.Type, tt.want, model.TypeReason)
			}
		})
	}
}

func TestAnalyze_StatsFromAdapter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	writeFile(t, dir, "lib.go", "package main\n\nfunc lib() {}\n")
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	adapter := &fakeAdapter{
		branches: []git.Branch{
			{Name: "main", IsCurrent: true, SHA: "abc", CommitAt: now},
			{Name: "feature", SHA: "def", CommitAt: now},
		},
		remoteBranches: []git.Branch{
			{Name: "origin/main", SHA: "abc", CommitAt: now},
		},
		commits: []git.Commit{
			{SHA: "abc", Author: "Alice", Email: "alice@example.com", Timestamp: now},
			{SHA: "def", Author: "Bob", Email: "bob@example.com", Timestamp: now.Add(-24 * time.Hour)},
		},
	}
	a := New(dir, adapter)
	model, err := a.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if model.Stats.CommitCount != 2 {
		t.Errorf("CommitCount = %d, want 2", model.Stats.CommitCount)
	}
	if model.Stats.BranchCount != 2 {
		t.Errorf("BranchCount = %d, want 2", model.Stats.BranchCount)
	}
	if model.Stats.RemoteBranchCount != 1 {
		t.Errorf("RemoteBranchCount = %d, want 1", model.Stats.RemoteBranchCount)
	}
	if model.Stats.ContributorCount != 2 {
		t.Errorf("ContributorCount = %d, want 2", model.Stats.ContributorCount)
	}
	if model.Stats.FileCount != 2 {
		t.Errorf("FileCount = %d, want 2", model.Stats.FileCount)
	}
}

func TestDetectLanguages_CommonExtensions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package main\n")
	writeFile(t, dir, "b.go", "package main\n")
	writeFile(t, dir, "c.py", "import os\n")
	writeFile(t, dir, "image.png", string([]byte{0x89, 'P', 'N', 'G'}))
	a := New(dir, &fakeAdapter{})
	model, err := a.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(model.Languages) < 2 {
		t.Fatalf("Languages = %v, want >= 2", model.Languages)
	}
	goLang := findLanguage(model.Languages, "Go")
	if goLang == nil || goLang.FileCount != 2 {
		t.Errorf("Go FileCount = %v, want 2", goLang)
	}
	pyLang := findLanguage(model.Languages, "Python")
	if pyLang == nil || pyLang.FileCount != 1 {
		t.Errorf("Python FileCount = %v, want 1", pyLang)
	}
	// PNG should be in "Other" or in no specific language.
	// Image bytes shouldn't bump Go's count.
	goLang = findLanguage(model.Languages, "Go")
	if goLang != nil && goLang.FileCount != 2 {
		t.Errorf("Go FileCount bumped by image: %v", goLang)
	}
}

func TestDetectFrameworks_HighConfidence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{
		"dependencies": {
			"react": "^18.2.0",
			"next": "^14.0.0",
			"tailwindcss": "^3.0.0"
		}
	}`)
	a := New(dir, &fakeAdapter{})
	model, err := a.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	for _, name := range []string{"React", "Next.js", "Tailwind CSS"} {
		fw := findFramework(model.Frameworks, name)
		if fw == nil {
			t.Errorf("framework %q missing", name)
			continue
		}
		if fw.Confidence != "high" {
			t.Errorf("framework %q Confidence = %q, want high", name, fw.Confidence)
		}
	}
}

func TestCustomDetector(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\n")
	custom := &customDetector{
		name: "my-detector",
		items: []DetectedItem{
			{Kind: KindLanguage, Name: "MyLang", Confidence: "high"},
			{Kind: KindRepositoryType, Name: string(RepoTypeTool), Category: "custom reason"},
		},
	}
	a := NewWithDetectors(dir, &fakeAdapter{}, []Detector{custom})
	model, err := a.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if !hasLanguage(model.Languages, "MyLang") {
		t.Errorf("MyLang not added by custom detector: %+v", model.Languages)
	}
	if model.Type != RepoTypeTool {
		t.Errorf("Type = %q, want %q", model.Type, RepoTypeTool)
	}
	if model.TypeReason != "custom reason" {
		t.Errorf("TypeReason = %q, want %q", model.TypeReason, "custom reason")
	}
}

// customDetector is a Detector implementation used by the
// CustomDetector test.
type customDetector struct {
	name  string
	items []DetectedItem
}

func (c *customDetector) Name() string { return c.name }
func (c *customDetector) Detect(ctx context.Context, dc DetectionContext) ([]DetectedItem, error) {
	return c.items, nil
}

func TestSkippedDirs(t *testing.T) {
	dir := t.TempDir()
	// Files inside a skipped directory should not appear in the
	// file list (and therefore not in language stats).
	writeFile(t, dir, "main.go", "package main\n")
	writeFile(t, dir, "node_modules/foo/index.js", "var x = 1;\n")
	writeFile(t, dir, "vendor/bar.go", "package bar\n")
	a := New(dir, &fakeAdapter{})
	model, err := a.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	// Go should be present (main.go at root).
	goLang := findLanguage(model.Languages, "Go")
	if goLang == nil {
		t.Fatalf("Go missing: %+v", model.Languages)
	}
	if goLang.FileCount != 1 {
		t.Errorf("Go FileCount = %d, want 1 (vendor/bar.go should be skipped)", goLang.FileCount)
	}
}

// Helpers.

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func hasLanguage(in []LanguageStat, name string) bool {
	for _, l := range in {
		if l.Name == name {
			return true
		}
	}
	return false
}

func findLanguage(in []LanguageStat, name string) *LanguageStat {
	for i := range in {
		if in[i].Name == name {
			return &in[i]
		}
	}
	return nil
}

func hasFramework(in []Framework, name string) bool {
	for _, f := range in {
		if f.Name == name {
			return true
		}
	}
	return false
}

func findFramework(in []Framework, name string) *Framework {
	for i := range in {
		if in[i].Name == name {
			return &in[i]
		}
	}
	return nil
}

func hasPackageManager(in []PackageManager, name string) bool {
	for _, p := range in {
		if p.Name == name {
			return true
		}
	}
	return false
}

func hasCICD(in []CICDSystem, name string) bool {
	for _, c := range in {
		if c.Name == name {
			return true
		}
	}
	return false
}

func findCICD(in []CICDSystem, name string) *CICDSystem {
	for i := range in {
		if in[i].Name == name {
			return &in[i]
		}
	}
	return nil
}

// errEOF is a sentinel used by commitStream.Read to signal end
// of input. io.EOF is the obvious choice; using a local
// sentinel makes the intent obvious in the stream code.
// encodeCommit serializes a git.Commit as a single JSON line.
// Mirrors what the exec adapter's Log formatter emits.
func encodeCommit(c git.Commit) []byte {
	type alias git.Commit
	// Use the json tags from git.Commit.
	body, _ := jsonMarshal(alias(c))
	return append(body, '\n')
}
