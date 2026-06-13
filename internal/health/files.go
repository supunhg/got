package health

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// binaryExts is the canonical list of file extensions that
// count as "binary" for the large-binaries check. It overlaps
// with analyzer.binaryExts but is kept independent because the
// health engine doesn't import the analyzer package (and
// shouldn't — health is a lower-level concern than analyzer).
var binaryExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true,
	".ico": true, ".webp": true, ".tiff": true, ".tif": true, ".svg": true,
	".pdf": true,
	".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true,
	".7z": true, ".rar": true,
	".jar": true, ".war": true, ".ear": true, ".aar": true, ".class": true,
	".pyc": true, ".pyo": true, ".pyd": true,
	".so": true, ".dylib": true, ".dll": true, ".exe": true, ".bin": true,
	".o": true, ".a": true, ".lib": true, ".obj": true,
	".deb": true, ".rpm": true, ".dmg": true, ".iso": true, ".img": true,
	".apk": true, ".ipa": true, ".app": true,
	".mp3": true, ".mp4": true, ".m4a": true, ".m4v": true, ".wav": true,
	".ogg": true, ".flac": true, ".webm": true, ".mov": true, ".avi": true,
	".mkv": true, ".wmv": true, ".flv": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
}

// isLikelyBinary reports whether a file's name suggests it is
// binary. The check is a basename / extension lookup; we
// intentionally do not read the file (the analyzer does that
// for the language breakdown — duplicating the read here would
// be wasteful).
func isLikelyBinary(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return binaryExts[ext]
}

// checkLargeBinaries walks the work tree and reports files
// that are both binary (by extension) and large (by bytes).
// The thresholds are tunable via Thresholds.LargeBinaryBytes
// and Thresholds.MaxLargeBinaries.
//
// Build artifacts (.git/, .got/, node_modules/, vendor/, etc.)
// are skipped — the same list the analyzer uses, so a
// "binary" in node_modules/ does not fire the health finding.
func checkLargeBinaries(c *Checker) ([]HealthFinding, error) {
	if c.WorkTree == "" {
		return nil, nil
	}
	abs, err := filepath.Abs(c.WorkTree)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}
	threshold := c.Thresholds.LargeBinaryBytes
	if threshold <= 0 {
		threshold = 1 << 20
	}
	var large []string
	walkErr := filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := filepath.Base(path)
			// Match the analyzer's skip list so both
			// packages see the same "this is build
			// artifact" boundary.
			switch base {
			case ".git", ".got", "node_modules", "vendor", "target", "dist", "build", "out", ".next", ".nuxt", ".svelte-kit", ".cache", ".parcel-cache", ".turbo", ".gradle", "__pycache__", ".pytest_cache", ".mypy_cache", ".ruff_cache", ".tox", ".venv", "venv", "env", ".idea", ".vscode", "coverage", "Godeps", "_obj", "_test", "bin", "pkg":
				return filepath.SkipDir
			}
			return nil
		}
		if !isLikelyBinary(path) {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		if fi.Size() < threshold {
			return nil
		}
		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return nil
		}
		large = append(large, filepath.ToSlash(rel))
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	if len(large) == 0 {
		return nil, nil
	}
	sort.Strings(large)
	t := c.Thresholds.MaxLargeBinaries
	return []HealthFinding{{
		ID:       "large-binaries",
		Category: CategoryFiles,
		Severity: severityForCount(len(large), t.Low, t.Medium, t.High),
		Title:    fmt.Sprintf("%d large binary file(s) in the working tree", len(large)),
		Detail:   fmt.Sprintf("These files are larger than %s and inflate the clone size. Move them to object storage, an LFS server, or release artifacts:", formatBytes(threshold)),
		Affected: large,
	}}, nil
}

// formatBytes renders a byte count as a human-readable string.
// Used in finding detail text; mirrors the format used by
// `du -h` and GitHub.
func formatBytes(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GiB", float64(n)/(1024*1024*1024))
}
