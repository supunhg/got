package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/plugin"
)

// pluginLoggerDepsFor builds a Deps with a slog Logger that writes
// to the provided buffer at LevelInfo. The shared depsWithLogger
// helper from testhelpers_test.go provides the adapter / discover
// / IsTerminal / Now / User / GotVersion / stdout / stderr
// scaffolding; pluginLoggerDepsFor layers DiscoverPlugins on top
// so the plugin subcommand tree has something to enumerate.
func pluginLoggerDepsFor(logBuf, stdout, stderr *bytes.Buffer, a git.Adapter, workTree string, plugins []plugin.DiscoveredPlugin) Deps {
	d := depsWithLogger(logBuf, stdout, stderr, a, workTree)
	d.DiscoverPlugins = func(_ context.Context) ([]plugin.DiscoveredPlugin, error) {
		return plugins, nil
	}
	return d
}

func TestPluginCmd_Install_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	src := writeFakePluginBinary(t, t.TempDir(), "got-github")
	deps := pluginLoggerDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir, nil)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"plugin", "install", src})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("plugin install: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="plugin install starting"`) {
		t.Errorf("expected 'plugin install starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="plugin install finished"`) {
		t.Errorf("expected 'plugin install finished' log, got:\n%s", out)
	}
	if !strings.Contains(out, "name=github") {
		t.Errorf("expected 'name=github' in finished log, got:\n%s", out)
	}
}

func TestPluginCmd_Install_FailureEmitsWarnLog(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Source does not exist — should fail with a validation error
	// and the log should have a "plugin install failed" warn line.
	deps := pluginLoggerDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir, nil)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"plugin", "install", "/no/such/file"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error for missing source, got nil")
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="plugin install starting"`) {
		t.Errorf("expected 'plugin install starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `level=WARN`) {
		t.Errorf("expected a WARN level record, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="plugin install failed"`) {
		t.Errorf("expected 'plugin install failed' log, got:\n%s", out)
	}
}

func TestPluginCmd_Enable_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	plugins := []plugin.DiscoveredPlugin{
		{Name: "github", Version: "1.2.0", MinGOT: "0.1.0", Path: "/usr/local/bin/got-github", Source: "PATH"},
	}
	deps := pluginLoggerDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir, plugins)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"plugin", "enable", "github"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("plugin enable: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="plugin enable starting"`) {
		t.Errorf("expected 'plugin enable starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="plugin enable finished"`) {
		t.Errorf("expected 'plugin enable finished' log, got:\n%s", out)
	}
}

func TestPluginCmd_Enable_UnknownEmitsWarnLog(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	deps := pluginLoggerDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir, nil)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"plugin", "enable", "ghost"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error for unknown plugin, got nil")
	}
	out := logBuf.String()
	if !strings.Contains(out, `level=WARN`) {
		t.Errorf("expected a WARN record, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="plugin enable failed: not discovered"`) {
		t.Errorf("expected 'plugin enable failed: not discovered' log, got:\n%s", out)
	}
}

func TestPluginCmd_Disable_EmitsStartedFinishedLogs(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	logBuf := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Disable an already-unknown plugin is a no-op success; we
	// should still see the started+finished records because spec
	// §16 wants the high-level operation to be observable in the
	// log even when it does nothing.
	deps := pluginLoggerDepsFor(logBuf, stdout, stderr, &fakeAdapter{}, dir, nil)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"plugin", "disable", "ghost"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("plugin disable: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, `msg="plugin disable starting"`) {
		t.Errorf("expected 'plugin disable starting' log, got:\n%s", out)
	}
	if !strings.Contains(out, `msg="plugin disable finished"`) {
		t.Errorf("expected 'plugin disable finished' log, got:\n%s", out)
	}
}

func TestPluginLogger_NilDepsLoggerIsNoop(t *testing.T) {
	// The pluginLogger helper must always return a non-nil
	// *slog.Logger so call sites can chain .Info / .Warn
	// without nil-checking. Use a Deps without a Logger.
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := pluginDepsFor(stdout, stderr, &fakeAdapter{}, dir, nil)
	// Explicitly nil the logger (it's already nil since
	// pluginDepsFor doesn't set it; this documents the test
	// intent).
	deps.Logger = nil
	if got := pluginLogger(deps); got == nil {
		t.Fatalf("pluginLogger(nil-Deps) returned nil; want a non-nil discard logger")
	}
}

// fakeAdapter from exit_codes_test.go / existing tests is reused.
// The import of gerr below keeps go vet happy for the unused-but-
// reachable error reference in the failure path.
var _ = gerr.Validation
