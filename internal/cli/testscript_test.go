// Copyright 2026 Supun Hewagamage. MIT License.
package cli

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestScript(t *testing.T) {
	gotBin := buildGot(t)

	testscript.Run(t, testscript.Params{
		Dir: filepath.Join("testdata", "testscript"),
		Setup: func(env *testscript.Env) error {
			env.Vars = append(env.Vars,
				"GOT_BIN="+gotBin,
			)
			return nil
		},
	})
}

func buildGot(t *testing.T) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "got")
	// Find repo root by walking up from this test file.
	repoRoot := filepath.Join("..", "..")
	cmd := exec.Command("go", "build", "-o", dst, "./cmd/got")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build got binary: %v\n%s", err, out)
	}
	return dst
}
