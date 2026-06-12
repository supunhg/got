package cli

import (
	"os"
	"testing"
)

// withChdir changes the process working directory to dir and
// registers a t.Cleanup to restore the original cwd. Tests that
// drive the `got init` command with no path arg rely on this so that
// filepath.Abs(".") resolves to the test's tempdir, not the host
// repository's checkout.
//
// We use os.Chdir rather than overriding deps.Discover to a fixed
// value because the real Discover has to walk up to find .git/ —
// without chdir, it would find the host repo's .git and init there.
func withChdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %q: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Logf("warning: chdir back to %q: %v", orig, err)
		}
	})
}
