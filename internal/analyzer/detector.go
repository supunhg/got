package analyzer

import (
	"context"
	"path/filepath"
	"sort"
)

// DetectionKind classifies the type of a detected item. It is used
// to merge items from user-supplied Detector implementations into
// the core model.
type DetectionKind string

const (
	// KindLanguage is a programming language.
	KindLanguage DetectionKind = "language"
	// KindFramework is a framework or library.
	KindFramework DetectionKind = "framework"
	// KindPackageManager is a package manager.
	KindPackageManager DetectionKind = "package-manager"
	// KindCICD is a CI/CD system.
	KindCICD DetectionKind = "cicd"
	// KindMonorepo is a monorepo tool indicator.
	KindMonorepo DetectionKind = "monorepo"
	// KindRepositoryType overrides the inferred repository type.
	KindRepositoryType DetectionKind = "repository-type"
	// KindCustom is for any other signal a plugin wants to surface.
	// The CLI ignores it by default but downstream tools can read it
	// from the JSON output.
	KindCustom DetectionKind = "custom"
)

// DetectedItem is a single finding from a Detector. The fields are
// interpreted based on Kind. For example:
//
//	{Kind: KindLanguage,      Name: "Rust", ...}      adds a language
//	{Kind: KindFramework,     Name: "Rocket", ...}    adds a framework
//	{Kind: KindPackageManager, Name: "cargo", ...}    adds a package manager
//
// Multiple items with the same Kind+Name are deduplicated; the first
// detection wins. A plugin that wants to refine the core model's
// fields (for example, to mark a project as a documentation repo)
// should return a KindRepositoryType item with the desired name.
type DetectedItem struct {
	// Kind categorizes the item.
	Kind DetectionKind `json:"kind"`
	// Name is the item's display name.
	Name string `json:"name"`
	// Category is an optional secondary label (e.g. "web" for frameworks).
	Category string `json:"category,omitempty"`
	// Language is the item's primary language, when relevant
	// (KindFramework / KindPackageManager).
	Language string `json:"language,omitempty"`
	// Version is the item's version, when known.
	Version string `json:"version,omitempty"`
	// Evidence is the list of files or directories that triggered
	// the detection. Used to populate ConfigFiles on the model.
	Evidence []string `json:"evidence,omitempty"`
	// Confidence is "high", "medium", or "low".
	Confidence string `json:"confidence,omitempty"`
}

// Detector is the interface plugins can implement to extend detection.
// Implementations should be deterministic and offline (no network calls).
//
// Detect returns the items the detector found, in the order they were
// discovered. An empty slice means "nothing to add". An error is
// returned only for unrecoverable problems; "I found nothing" is
// not an error.
//
// The Context argument gives access to the work tree, the list of
// files the core walker already enumerated, and any errors the
// walker encountered (e.g. permission-denied subdirectories).
type Detector interface {
	// Name returns the detector's name. Used in logs and in
	// test output to identify which detector produced an item.
	Name() string
	// Detect runs the detector against the supplied context.
	Detect(ctx context.Context, dc DetectionContext) ([]DetectedItem, error)
}

// DetectionContext is the bundle of state passed to a Detector.
// It is provided so detectors can reuse the work the core walker
// has already done (file enumeration, file contents) without
// walking the work tree a second time.
type DetectionContext struct {
	// WorkTree is the absolute path to the repository's work tree.
	WorkTree string `json:"workTree"`
	// Files is the list of files the core walker found, with paths
	// relative to WorkTree. Sorted lexically.
	Files []string `json:"files"`
	// SkippedDirs is the list of directory basenames the walker skipped.
	// Detectors that need to look at a "hidden" directory (e.g. ".github")
	// can use this to know which standard excludes were applied.
	SkippedDirs []string `json:"skippedDirs"`
	// Errors is a list of non-fatal errors the walker encountered
	// (permission denied, broken symlinks, etc.). Detectors can
	// inspect these to decide whether the analysis is incomplete.
	Errors []error `json:"-"`
}

// ReadFile reads a file relative to the work tree, returning its
// contents. The path may be relative (to the work tree) or absolute.
// Returns an error if the file is missing or unreadable.
//
// Detectors should use this helper rather than calling os.ReadFile
// directly so the read goes through the same path-resolution logic
// the rest of the package uses.
func (dc DetectionContext) ReadFile(path string) ([]byte, error) {
	return readFile(dc.WorkTree, path)
}

// fileExists reports whether the file at path (relative to the work
// tree, or absolute) exists. Detectors use this for cheap existence
// checks before reading a manifest.
func (dc DetectionContext) fileExists(path string) bool {
	return fileExists(dc.WorkTree, path)
}

// isDir reports whether path (relative to the work tree, or absolute)
// is a directory.
func (dc DetectionContext) isDir(path string) bool {
	return isDir(dc.WorkTree, path)
}

// basenameOf is a tiny convenience: filepath.Base(path) with an
// empty fallback for paths that are themselves empty.
func basenameOf(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}

// sortedKeys returns the keys of m sorted lexically. Useful for
// deterministic detector output.
func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
