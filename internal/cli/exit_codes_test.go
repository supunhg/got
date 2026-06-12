// exit_codes_test.go: spec §15 exit-code wiring. These tests drive
// the cobra command tree with stubs that return each kind of gerr
// error and assert that gerr.ExitCode(err) reports the right number.
// They live in their own file so a reader looking for "how do I make
// a new command return the right exit code?" can find the pattern in
// one place.
package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/plugin"
	"github.com/got-sh/got/internal/store"
)

// newCmdWithDiscoverErr builds a root command whose Discover returns
// the given error. This is the cheapest way to exercise "got X in a
// non-git-repo" without a temp dir + real filesystem setup.
func newCmdWithDiscoverErr(stdout, stderr *bytes.Buffer, discoverErr error) *cobra.Command {
	deps := Deps{
		AdapterFor: func(string) git.Adapter { return &fakeAdapter{} },
		Discover:   func(string) (string, error) { return "", discoverErr },
		IsTerminal: func() bool { return false },
		Stdout:     stdout,
		Stderr:     stderr,
	}
	return NewRootCmd(deps)
}

// TestExitCode_NotInGitRepo_AcrossCommands verifies that every
// command which calls deps.Discover (status, commit, branch, remote,
// graph, plugin list) returns the same *gerr.Error with Code 3.
func TestExitCode_NotInGitRepo_AcrossCommands(t *testing.T) {
	cases := [][]string{
		{"status"},
		{"commit", "-m", "test: x"},
		{"branch"},
		{"remote", "list"},
		{"graph", "--no-tui"},
		{"plugin", "list"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			notInRepo := gerr.NotInGitRepo(".")
			// plugin list's flow goes through deps.DiscoverPlugins,
			// which by default calls repo.Discover, so we wire the
			// not-in-repo error into both seams.
			deps := Deps{
				AdapterFor: func(string) git.Adapter { return &fakeAdapter{} },
				Discover:   func(string) (string, error) { return "", notInRepo },
				DiscoverPlugins: func(_ context.Context) ([]plugin.DiscoveredPlugin, error) {
					return nil, notInRepo
				},
				IsTerminal: func() bool { return false },
				Stdout:     stdout,
				Stderr:     stderr,
			}
			cmd := NewRootCmd(deps)
			cmd.SetOut(stdout)
			cmd.SetErr(stderr)
			cmd.SetArgs(args)
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if got := gerr.ExitCode(wrapExecuteError(err)); got != int(gerr.CodeNotInGitRepo) {
				t.Errorf("ExitCode = %d, want %d (CodeNotInGitRepo)", got, gerr.CodeNotInGitRepo)
			}
		})
	}
}

// TestExitCode_Usage_UnknownFlag verifies that `got --bogus-flag`
// returns a gerr.Usage error (code 2) after the wrapExecuteError
// mapping. Cobra's pflag returns *flag.Error, which would otherwise
// map to CodeGeneric (1).
func TestExitCode_Usage_UnknownFlag(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := newCmdWithDiscoverErr(stdout, stderr, nil)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"--bogus-flag"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for unknown flag, got nil")
	}
	wrapped := wrapExecuteError(err)
	if got := gerr.ExitCode(wrapped); got != int(gerr.CodeUsage) {
		t.Errorf("ExitCode = %d, want %d (CodeUsage) for unknown flag; err=%v", got, gerr.CodeUsage, err)
	}
	// The wrapped message should preserve the pflag text.
	if !strings.Contains(wrapped.Error(), "bogus") {
		t.Errorf("wrapped error should mention the flag; got: %v", wrapped)
	}
}

// TestExitCode_Usage_UnknownSubcommand verifies that `got bogus-sub`
// also maps to CodeUsage.
func TestExitCode_Usage_UnknownSubcommand(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := newCmdWithDiscoverErr(stdout, stderr, nil)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"bogus-sub"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for unknown subcommand, got nil")
	}
	wrapped := wrapExecuteError(err)
	if got := gerr.ExitCode(wrapped); got != int(gerr.CodeUsage) {
		t.Errorf("ExitCode = %d, want %d (CodeUsage) for unknown subcommand", got, gerr.CodeUsage)
	}
}

// TestExitCode_NotInitialized_PluginList verifies that the
// "not initialized" path returns Code 4 when a command that needs
// .got/ cannot find it. We exercise this via plugin list, whose
// production DiscoverPlugins call returns gerr.NotInitialized on a
// missing .got/.
func TestExitCode_NotInitialized_PluginList(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := Deps{
		AdapterFor: func(string) git.Adapter { return &fakeAdapter{} },
		Discover:   func(string) (string, error) { return dir, nil },
		Stdout:     stdout,
		Stderr:     stderr,
		DiscoverPlugins: func(_ context.Context) ([]plugin.DiscoveredPlugin, error) {
			return nil, gerr.NotInitialized()
		},
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"plugin", "list"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error when .got/ is missing for plugin list, got nil")
	}
	if got := gerr.ExitCode(err); got != int(gerr.CodeNotInitialized) {
		t.Errorf("ExitCode = %d, want %d (CodeNotInitialized)", got, gerr.CodeNotInitialized)
	}
}

// TestExitCode_Validation_AlreadyInitialized verifies that
// `got init --no-tui` against an already-initialized repo returns
// Code 5 (gerr.Validation).
func TestExitCode_Validation_AlreadyInitialized(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	// Pretend .got/ + got.yml already exist by writing a stub.
	if err := os.MkdirAll(filepath.Join(dir, ".got"), 0o755); err != nil {
		t.Fatalf("mkdir .got: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "got.yml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatalf("write got.yml: %v", err)
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := Deps{
		AdapterFor: func(string) git.Adapter { return a },
		Discover:   func(string) (string, error) { return dir, nil },
		StoreFor:   store.Open,
		IsTerminal: func() bool { return false },
		Now:        func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		User:       func() string { return "alice" },
		GotVersion: "0.1.0-test",
		Stdout:     stdout,
		Stderr:     stderr,
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init", "--no-tui"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for re-init, got nil")
	}
	if got := gerr.ExitCode(err); got != int(gerr.CodeValidation) {
		t.Errorf("ExitCode = %d, want %d (CodeValidation); err=%v", got, gerr.CodeValidation, err)
	}
}

// TestExitCode_Plugin_PluginError verifies that a plugin exec
// failure routes through gerr.PluginError and returns Code 64.
func TestExitCode_Plugin_PluginError(t *testing.T) {
	// A plugin binary that exits non-zero on --got-plugin-manifest.
	dir := initGitRepo(t)
	withChdir(t, dir)
	bad := filepath.Join(t.TempDir(), "got-bad")
	if err := os.WriteFile(bad, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write bad: %v", err)
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := Deps{
		AdapterFor: func(string) git.Adapter { return a },
		Discover:   func(string) (string, error) { return dir, nil },
		IsTerminal: func() bool { return false },
		GotVersion: "0.1.0-test",
		Stdout:     stdout,
		Stderr:     stderr,
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"plugin", "install", bad})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for bad plugin, got nil")
	}
	if got := gerr.ExitCode(err); got != int(gerr.CodePlugin) {
		t.Errorf("ExitCode = %d, want %d (CodePlugin)", got, gerr.CodePlugin)
	}
}

// TestExitCode_PermissionDenied_ReadOnlyDir verifies that
// gerr.PermissionDenied (code 1) is returned when an install is
// attempted into a read-only directory. We use os.Chmod to make
// the destination unwritable, then restore it for cleanup.
func TestExitCode_PermissionDenied_ReadOnlyDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission denied is bypassed")
	}
	dir := initGitRepo(t)
	withChdir(t, dir)
	pluginsDir := filepath.Join(dir, ".got", "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Make the plugins dir read-only. Restoring at the end is
	// safe even on test failure because t.TempDir cleanup will
	// blow away the whole tree.
	if err := os.Chmod(pluginsDir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(pluginsDir, 0o755) })

	src := writeFakePluginBinary(t, t.TempDir(), "got-rw")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := &fakeAdapter{}
	deps := Deps{
		AdapterFor: func(string) git.Adapter { return a },
		Discover:   func(string) (string, error) { return dir, nil },
		IsTerminal: func() bool { return false },
		GotVersion: "0.1.0-test",
		Stdout:     stdout,
		Stderr:     stderr,
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"plugin", "install", src})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error installing into read-only dir, got nil")
	}
	// PermissionDenied returns CodeGeneric (1) per spec §15.
	if got := gerr.ExitCode(err); got != int(gerr.CodeGeneric) {
		t.Errorf("ExitCode = %d, want %d (CodeGeneric) for permission denied; err=%v",
			got, gerr.CodeGeneric, err)
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error should mention 'permission denied'; got: %v", err)
	}
}

// TestWrapExecuteError_PassThrough verifies that pre-typed gerr
// errors are not double-wrapped.
func TestWrapExecuteError_PassThrough(t *testing.T) {
	orig := gerr.NotInGitRepo(".")
	got := wrapExecuteError(orig)
	if got != orig {
		t.Errorf("expected pass-through (same pointer); got %v", got)
	}
	if !errors.Is(got, orig) {
		t.Errorf("expected errors.Is to find orig")
	}
}

// TestWrapExecuteError_NilIsNil verifies the no-error case.
func TestWrapExecuteError_NilIsNil(t *testing.T) {
	if got := wrapExecuteError(nil); got != nil {
		t.Errorf("wrapExecuteError(nil) = %v, want nil", got)
	}
}
