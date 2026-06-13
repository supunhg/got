// Package analyzer detects the languages, frameworks, package managers,
// CI/CD systems, containerization, monorepo structure, and repository
// type of a Git repository, and computes basic repository statistics.
//
// The analyzer is read-only: it never modifies the working tree, .git/,
// or .got/. It walks the work tree, inspects the configured files, and
// uses the git.Adapter for branch/commit/contributor data.
//
// Plugins can extend detection by implementing the Detector interface
// and passing instances via NewWithDetectors. The core package's
// built-in detectors are always run; user-supplied detectors run
// afterward and may add or refine items.
//
// The analyzer is offline-only: no network calls. It only inspects
// files that are already in the work tree.
package analyzer

import (
	"path/filepath"
	"time"

	"github.com/got-sh/got/internal/git"
)

// RepositoryModel is the complete set of facts the analyzer derives
// about a repository. Every field is computed in a single Analyze
// call; nothing here is cached or persisted.
type RepositoryModel struct {
	// Path is the absolute path to the work tree that was analyzed.
	Path string `json:"path"`
	// Name is the repository's display name. It is taken from
	// got.yml (project.name) when present, otherwise from the
	// work tree's directory name.
	Name string `json:"name"`
	// Description is a free-form description (got.yml project.description
	// or similar). Empty when the project has none.
	Description string `json:"description,omitempty"`
	// DefaultBranch is the project's default branch (got.yml
	// project.default_branch, or "main").
	DefaultBranch string `json:"defaultBranch"`

	// Languages is the list of detected programming languages,
	// sorted by line count descending.
	Languages []LanguageStat `json:"languages"`
	// Frameworks is the list of detected frameworks and libraries.
	Frameworks []Framework `json:"frameworks"`
	// PackageManagers is the list of detected package managers.
	PackageManagers []PackageManager `json:"packageManagers"`
	// CICDSystems is the list of detected CI/CD systems.
	CICDSystems []CICDSystem `json:"cicdSystems"`
	// Containerization summarizes container-related artifacts.
	Containerization Containerization `json:"containerization"`
	// Monorepo describes monorepo structure (workspaces, packages).
	Monorepo MonorepoInfo `json:"monorepo"`
	// Type is the high-level repository classification.
	Type RepositoryType `json:"type"`
	// TypeReason explains why the repository was classified with Type.
	TypeReason string `json:"typeReason,omitempty"`

	// Stats are the repository-level statistics.
	Stats RepositoryStats `json:"stats"`

	// DetectedAt is the time the analysis was performed.
	DetectedAt time.Time `json:"detectedAt"`
}

// LanguageStat is one detected language plus its share of the codebase.
type LanguageStat struct {
	// Name is the human-readable language name (e.g. "Go", "Python").
	Name string `json:"name"`
	// Extensions is the list of file extensions that map to this language.
	Extensions []string `json:"extensions"`
	// FileCount is the number of source files in this language.
	FileCount int `json:"fileCount"`
	// LineCount is the approximate number of lines in those files.
	// For binary files (images, videos, etc.) this is 0.
	LineCount int `json:"lineCount"`
	// Bytes is the total size of those files on disk.
	Bytes int64 `json:"bytes"`
	// Percentage is the language's share of the total source bytes
	// (0-100, rounded to one decimal place).
	Percentage float64 `json:"percentage"`
}

// Framework is a detected framework, library, or tool.
type Framework struct {
	// Name is the framework's display name (e.g. "React", "Django").
	Name string `json:"name"`
	// Category is the high-level category (e.g. "web", "test", "build").
	Category string `json:"category"`
	// Language is the primary language of the framework (e.g. "JavaScript").
	Language string `json:"language,omitempty"`
	// Version is the detected version, when the manifest carries it
	// (e.g. "18.2.0" from package.json). Empty when unknown.
	Version string `json:"version,omitempty"`
	// ConfigFiles is the list of files that triggered detection.
	ConfigFiles []string `json:"configFiles"`
	// Confidence is "high" when the framework is explicitly named in
	// a manifest, "medium" when inferred from a directory or build
	// artifact, "low" when guessed from a single file.
	Confidence string `json:"confidence"`
}

// PackageManager is a detected package manager.
type PackageManager struct {
	// Name is the package manager (e.g. "npm", "go modules", "pip").
	Name string `json:"name"`
	// Ecosystem is the language ecosystem (e.g. "JavaScript", "Go", "Python").
	Ecosystem string `json:"ecosystem"`
	// Manifest is the primary manifest file (e.g. "package.json", "go.mod").
	Manifest string `json:"manifest"`
	// Lockfile is the lockfile, when one exists (e.g. "package-lock.json").
	// Empty when no lockfile is present.
	Lockfile string `json:"lockfile,omitempty"`
	// IsFrozen reports whether the lockfile is checked in.
	IsFrozen bool `json:"isFrozen"`
}

// CICDSystem is a detected CI/CD system.
type CICDSystem struct {
	// Name is the human-readable system name (e.g. "GitHub Actions").
	Name string `json:"name"`
	// Provider is the provider identifier (e.g. "github", "gitlab").
	Provider string `json:"provider"`
	// ConfigFiles is the list of CI/CD config files.
	ConfigFiles []string `json:"configFiles"`
	// WorkflowCount is the number of workflows/jobs/pipelines
	// declared (best-effort count of files in the config directory).
	WorkflowCount int `json:"workflowCount"`
}

// Containerization summarizes container-related artifacts.
type Containerization struct {
	// HasDockerfile is true when one or more Dockerfiles are present.
	HasDockerfile bool `json:"hasDockerfile"`
	// DockerfileCount is the number of Dockerfiles found.
	DockerfileCount int `json:"dockerfileCount"`
	// HasDockerCompose is true when docker-compose.yml/.yaml is present.
	HasDockerCompose bool `json:"hasDockerCompose"`
	// ComposeFileCount is the number of docker-compose files.
	ComposeFileCount int `json:"composeFileCount"`
	// HasKubernetes is true when Kubernetes manifests are present.
	HasKubernetes bool `json:"hasKubernetes"`
	// K8sManifestCount is the number of K8s YAML files.
	K8sManifestCount int `json:"k8sManifestCount"`
	// HasHelm is true when Helm charts are present.
	HasHelm bool `json:"hasHelm"`
	// HelmChartCount is the number of Helm charts (Chart.yaml files).
	HelmChartCount int `json:"helmChartCount"`
}

// MonorepoInfo describes monorepo structure.
type MonorepoInfo struct {
	// IsMonorepo is true when the repository is a monorepo
	// (multiple packages/workspaces inside one repo).
	IsMonorepo bool `json:"isMonorepo"`
	// Tool is the monorepo tool, when one is detected
	// (e.g. "npm workspaces", "lerna", "nx", "turborepo", "pnpm").
	Tool string `json:"tool,omitempty"`
	// PackageCount is the number of sub-packages detected.
	PackageCount int `json:"packageCount"`
	// Packages is the list of sub-package paths (relative to the work tree).
	Packages []string `json:"packages,omitempty"`
}

// RepositoryType is the high-level classification of a repository.
type RepositoryType string

const (
	// RepoTypeApplication is a deployable application (web app,
	// service, CLI with a runnable artifact).
	RepoTypeApplication RepositoryType = "application"
	// RepoTypeLibrary is a reusable library or SDK.
	RepoTypeLibrary RepositoryType = "library"
	// RepoTypeTool is a developer tool (CLI, build tool, formatter).
	RepoTypeTool RepositoryType = "tool"
	// RepoTypeDocumentation is a documentation-only repository.
	RepoTypeDocumentation RepositoryType = "documentation"
	// RepoTypeConfig is a configuration / infrastructure-only repo
	// (Terraform, Ansible, dotfiles, etc.).
	RepoTypeConfig RepositoryType = "config"
	// RepoTypeMonorepo is a multi-package repository (set when
	// Monorepo.IsMonorepo is true and no more specific type fits).
	RepoTypeMonorepo RepositoryType = "monorepo"
	// RepoTypeUnknown is used when the analyzer cannot classify
	// the repository with confidence.
	RepoTypeUnknown RepositoryType = "unknown"
)

// RepositoryStats are the basic numerical facts about a repository.
type RepositoryStats struct {
	// CommitCount is the number of commits reachable from any ref.
	CommitCount int `json:"commitCount"`
	// BranchCount is the number of local branches.
	BranchCount int `json:"branchCount"`
	// RemoteBranchCount is the number of remote-tracking branches.
	RemoteBranchCount int `json:"remoteBranchCount"`
	// ContributorCount is the number of distinct commit authors
	// (by email; authors with the same email are merged).
	ContributorCount int `json:"contributorCount"`
	// FileCount is the number of files in the work tree (excluding
	// build artifacts, dependencies, and VCS internals).
	FileCount int `json:"fileCount"`
	// LineCount is the total number of lines across all source files.
	LineCount int `json:"lineCount"`
	// SizeBytes is the total size of the work tree in bytes.
	SizeBytes int64 `json:"sizeBytes"`
	// FirstCommitAt is the time of the oldest commit (zero when
	// the repository has no commits).
	FirstCommitAt time.Time `json:"firstCommitAt"`
	// LastCommitAt is the time of the newest commit (zero when
	// the repository has no commits).
	LastCommitAt time.Time `json:"lastCommitAt"`
}

// Analyzer is the main entry point. Construct one with New, then
// call Analyze to produce a RepositoryModel.
type Analyzer struct {
	workTree string
	adapter  git.Adapter

	// detectors is the list of additional detectors to run after
	// the built-ins. nil means "use the built-ins only".
	detectors []Detector
}

// New returns an Analyzer rooted at workTree. The adapter is used
// for git-derived statistics (commit count, branch count, etc.).
func New(workTree string, adapter git.Adapter) *Analyzer {
	return &Analyzer{
		workTree: workTree,
		adapter:  adapter,
	}
}

// NewWithDetectors returns an Analyzer with custom detectors
// appended to the built-in detection pipeline. The built-ins
// always run first; user detectors run in the order supplied
// and may add or refine items.
func NewWithDetectors(workTree string, adapter git.Adapter, detectors []Detector) *Analyzer {
	return &Analyzer{
		workTree:  workTree,
		adapter:   adapter,
		detectors: append([]Detector(nil), detectors...),
	}
}

// WorkTree returns the work tree path the analyzer was constructed for.
func (a *Analyzer) WorkTree() string { return a.workTree }

// WalkOptions configures which directories the file walker skips.
// The zero value uses defaultSkippedDirs. Use it to add or remove
// paths without changing the package-level default.
type WalkOptions struct {
	// ExtraSkipped is appended to the default skip list.
	ExtraSkipped []string
	// ReplaceSkipped, when non-nil, replaces the default skip list.
	ReplaceSkipped []string
}

// defaultSkippedDirs is the set of directories the file walker skips
// during analysis. These are typically build artifacts, dependency
// caches, and VCS internals that would otherwise dominate the file
// count and distort language statistics.
//
// The list is intentionally permissive: it is fine to analyze a
// node_modules directory if the user really wants to, but doing
// so by default would make every JS project look like 99% JS and
// drown out smaller source languages.
var defaultSkippedDirs = []string{
	".git",
	".got",
	"node_modules",
	"vendor",
	"target",
	"dist",
	"build",
	"out",
	".next",
	".nuxt",
	".svelte-kit",
	".cache",
	".parcel-cache",
	".turbo",
	".gradle",
	"__pycache__",
	".pytest_cache",
	".mypy_cache",
	".ruff_cache",
	".tox",
	".venv",
	"venv",
	"env",
	".idea",
	".vscode",
	"coverage",
	"Godeps",
	"_obj",
	"_test",
	"bin", // GOPATH bin; not user code
	"pkg", // GOPATH pkg; not user code
}

// skippedFor returns the effective skip list for the given options.
func skippedFor(opts WalkOptions) []string {
	if opts.ReplaceSkipped != nil {
		out := make([]string, 0, len(opts.ReplaceSkipped)+len(opts.ExtraSkipped))
		out = append(out, opts.ReplaceSkipped...)
		out = append(out, opts.ExtraSkipped...)
		return out
	}
	out := make([]string, 0, len(defaultSkippedDirs)+len(opts.ExtraSkipped))
	out = append(out, defaultSkippedDirs...)
	out = append(out, opts.ExtraSkipped...)
	return out
}

// isSkippedDir reports whether dir's final component is in the
// skip list. The comparison is exact (not prefix-based) so a file
// called "build.go" in the work tree root is not skipped.
func isSkippedDir(dir string, skip []string) bool {
	base := filepath.Base(dir)
	if base == "." || base == "/" || base == "" {
		return false
	}
	for _, s := range skip {
		if base == s {
			return true
		}
	}
	return false
}
