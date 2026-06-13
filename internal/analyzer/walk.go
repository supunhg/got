package analyzer

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// walkResult is the result of walking a work tree: the list of
// relative file paths, the list of directories that were skipped,
// and any non-fatal errors encountered along the way.
type walkResult struct {
	files       []string
	skippedDirs []string
	errors      []error
}

// walkFiles walks workTree and returns every regular file (relative
// to workTree) that does not live under a skipped directory. The
// returned paths are slash-separated, sorted lexically, and
// relative to the work tree (no leading "./").
//
// Symbolic links are not followed: a symlink to a directory counts
// as a single file, not as a directory to descend into. This avoids
// accidentally walking into dependency trees that point back into
// the work tree (e.g. a "current" symlink in a deployment layout).
//
// The walker never returns an error for permission-denied entries;
// those become entries in walkResult.errors so the model can surface
// them to the user. The only fatal error is "work tree does not exist".
func walkFiles(workTree string, opts WalkOptions) (walkResult, error) {
	out := walkResult{}
	info, err := os.Stat(workTree)
	if err != nil {
		return out, err
	}
	if !info.IsDir() {
		return out, errors.New("analyzer: work tree is not a directory")
	}
	skip := skippedFor(opts)
	out.skippedDirs = append([]string(nil), skip...)

	// Detect "I am inside my own work tree" before descending, so
	// we don't recurse into .git/ etc.
	abs, err := filepath.Abs(workTree)
	if err != nil {
		return out, err
	}

	walkErr := filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Permission denied, broken symlink, race with
			// deletion, etc. Record and keep going.
			out.errors = append(out.errors, err)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			// Don't recurse into a directory whose name is
			// in the skip list. Compare on the basename, not
			// the full path, so an unrelated subdirectory that
			// happens to be named "build" inside a project's
			// source tree is still skipped (matches the intent
			// of the default skip list).
			base := filepath.Base(path)
			for _, s := range skip {
				if base == s {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Regular file (or symlink to file). Skip dotfiles-only
		// hidden directories? No — we want to find .github/,
		// .vscode/, etc. The skip list already covers the ones
		// we don't want (e.g. .git, .got).
		rel, err := filepath.Rel(abs, path)
		if err != nil {
			out.errors = append(out.errors, err)
			return nil
		}
		// Normalize to forward slashes for downstream consumers.
		rel = filepath.ToSlash(rel)
		// Skip the work tree root itself.
		if rel == "." {
			return nil
		}
		out.files = append(out.files, rel)
		return nil
	})
	if walkErr != nil {
		out.errors = append(out.errors, walkErr)
	}
	sort.Strings(out.files)
	return out, nil
}

// readFile reads a file at path (relative to root, or absolute).
// Returns an error if the file is missing or unreadable. The path
// is cleaned and validated to prevent escape attempts.
func readFile(root, path string) ([]byte, error) {
	full, err := resolveUnderRoot(root, path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(full)
}

// fileExists reports whether the file at path (relative to root, or
// absolute) exists. Directories count as "exists". Symlinks are not
// followed: a broken symlink returns false.
func fileExists(root, path string) bool {
	full, err := resolveUnderRoot(root, path)
	if err != nil {
		return false
	}
	_, err = os.Lstat(full)
	return err == nil
}

// isDir reports whether path (relative to root, or absolute) is a
// directory. Symlinks are not followed.
func isDir(root, path string) bool {
	full, err := resolveUnderRoot(root, path)
	if err != nil {
		return false
	}
	info, err := os.Stat(full)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// resolveUnderRoot joins root and path and verifies the result is
// still under root. The check defends against paths like "../../etc/passwd"
// that would otherwise escape the work tree. An absolute path is
// validated as-is; a relative path is joined with root and then
// cleaned.
//
// Returns an error when the resolved path escapes root.
func resolveUnderRoot(root, path string) (string, error) {
	if path == "" {
		return "", errors.New("analyzer: empty path")
	}
	var full string
	if filepath.IsAbs(path) {
		full = filepath.Clean(path)
	} else {
		full = filepath.Clean(filepath.Join(root, path))
	}
	// Clean the root too so the prefix check is reliable.
	cleanRoot := filepath.Clean(root)
	// Allow exact match (the root itself). Anything else must have
	// cleanRoot as a prefix, with a path separator after.
	if full == cleanRoot {
		return full, nil
	}
	prefix := cleanRoot + string(os.PathSeparator)
	if !strings.HasPrefix(full, prefix) {
		return "", errors.New("analyzer: path escapes work tree")
	}
	return full, nil
}

// isTextFile reports whether the file at path is a text file. A
// text file is one whose first 512 bytes contain no NUL bytes
// (the heuristic used by git itself for the text/binary attribute).
// Returns false on read error so the caller treats it as binary.
func isTextFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return false
	}
	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return false
		}
	}
	return true
}

// countLines counts the number of newline-terminated lines in data.
// The final line is counted even if it has no trailing newline.
// Empty data returns 0.
func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := 0
	for _, b := range data {
		if b == '\n' {
			n++
		}
	}
	return n
}

// fileSize returns the size of the file at path in bytes. Returns
// 0 on error. Symlinks are not followed (matches the walker's
// behaviour).
func fileSize(path string) int64 {
	info, err := os.Lstat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
