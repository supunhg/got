// Package version exposes the GOT binary's build-time identity. The
// Version, Commit, and Date variables are intended to be overridden at
// build time via:
//
//	-ldflags "-X github.com/got-sh/got/internal/version.Version=... \
//	          -X github.com/got-sh/got/internal/version.Commit=... \
//	          -X github.com/got-sh/got/internal/version.Date=..."
package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is the GOT semantic version (e.g. "0.1.0"). Defaults to "dev" for
// builds that did not inject a value.
var Version = "dev"

// Commit is the short git SHA the binary was built from, or "none" if the
// build did not inject one.
var Commit = "none"

// Date is the RFC3339 build timestamp, or "unknown" if the build did not
// inject one.
var Date = "unknown"

// String returns a human-readable version string of the form:
//
//	got 0.1.0 (commit abcdef1, built 2026-06-12T10:00:00Z)
//
// It is the single source of truth for what `--version` and `got version`
// print. Both call sites route through here so the output stays in sync.
func String() string {
	return fmt.Sprintf("got %s (commit %s, built %s)", Version, Commit, Date)
}

// Matches checks whether the current version satisfies the given semver
// constraint. The constraint format is simple: ">=1.0.0", ">0.5.0",
// "1.0.0" (exact match). Returns true if version is "dev" (development
// builds accept any constraint).
func Matches(constraint string) bool {
	if Version == "dev" {
		return true
	}

	constraint = strings.TrimSpace(constraint)
	if constraint == "" {
		return true
	}

	// Parse current version.
	curMajor, curMinor, curPatch := parseSemver(Version)

	// Check for operators.
	if strings.HasPrefix(constraint, ">=") {
		maj, min, patch := parseSemver(constraint[2:])
		return compareSemver(curMajor, curMinor, curPatch, maj, min, patch) >= 0
	}
	if strings.HasPrefix(constraint, ">") {
		maj, min, patch := parseSemver(constraint[1:])
		return compareSemver(curMajor, curMinor, curPatch, maj, min, patch) > 0
	}
	if strings.HasPrefix(constraint, "<=") {
		maj, min, patch := parseSemver(constraint[2:])
		return compareSemver(curMajor, curMinor, curPatch, maj, min, patch) <= 0
	}
	if strings.HasPrefix(constraint, "<") {
		maj, min, patch := parseSemver(constraint[1:])
		return compareSemver(curMajor, curMinor, curPatch, maj, min, patch) < 0
	}
	if strings.HasPrefix(constraint, "=") {
		maj, min, patch := parseSemver(constraint[1:])
		return compareSemver(curMajor, curMinor, curPatch, maj, min, patch) == 0
	}

	// Exact match.
	maj, min, patch := parseSemver(constraint)
	return compareSemver(curMajor, curMinor, curPatch, maj, min, patch) == 0
}

// parseSemver extracts major.minor.patch from a version string.
// Returns 0,0,0 on error.
func parseSemver(v string) (int, int, int) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return 0, 0, 0
	}
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	patch := 0
	if len(parts) > 2 {
		// Strip any suffix like "-beta1" or "+build"
		patchStr := strings.SplitN(parts[2], "-", 2)[0]
		patchStr = strings.SplitN(patchStr, "+", 2)[0]
		patch, _ = strconv.Atoi(patchStr)
	}
	return major, minor, patch
}

// compareSemver returns -1 if a < b, 0 if a == b, 1 if a > b.
func compareSemver(amaj, amin, apatch, bmaj, bmin, bpatch int) int {
	switch {
	case amaj != bmaj:
		if amaj < bmaj {
			return -1
		}
		return 1
	case amin != bmin:
		if amin < bmin {
			return -1
		}
		return 1
	case apatch != bpatch:
		if apatch < bpatch {
			return -1
		}
		return 1
	default:
		return 0
	}
}
