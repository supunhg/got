// Package version exposes the GOT binary's build-time identity. The
// Version, Commit, and Date variables are intended to be overridden at
// build time via:
//
//	-ldflags "-X github.com/got-sh/got/internal/version.Version=... \
//	          -X github.com/got-sh/got/internal/version.Commit=... \
//	          -X github.com/got-sh/got/internal/version.Date=..."
package version

import "fmt"

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
