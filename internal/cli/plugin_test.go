package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/git"
	"github.com/got-sh/got/internal/plugin"
)

// pluginDepsFor builds a Deps value with a stubbed DiscoverPlugins.
// Mirrors the other CLI test helpers.
func pluginDepsFor(stdout, stderr *bytes.Buffer, a git.Adapter, workTree string, plugins []plugin.DiscoveredPlugin) Deps {
	return Deps{
		AdapterFor: func(string) git.Adapter { return a },
		Discover:   func(string) (string, error) { return workTree, nil },
		IsTerminal: func() bool { return false },
		Now:        func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		User:       func() string { return "alice" },
		GotVersion: "0.1.0-test",
		Stdout:     stdout,
		Stderr:     stderr,
		DiscoverPlugins: func(_ context.Context) ([]plugin.DiscoveredPlugin, error) {
			return plugins, nil
		},
	}
}

func TestPluginCmd_ListEmpty(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := pluginDepsFor(stdout, stderr, &fakeAdapter{}, dir, nil)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"plugin", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("plugin list: %v", err)
	}
	if !strings.Contains(stdout.String(), "(no plugins discovered)") {
		t.Errorf("expected '(no plugins discovered)' line, got:\n%s", stdout.String())
	}
}

func TestPluginCmd_ListTable(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	plugins := []plugin.DiscoveredPlugin{
		{
			Name:     "github",
			Version:  "1.2.0",
			MinGOT:   "0.1.0",
			Path:     "/usr/local/bin/got-github",
			Source:   "PATH",
			Commands: []plugin.ManifestCommand{{Name: "pr", Description: "Open a PR"}},
		},
		{
			Name:    "slack",
			Version: "0.3.1",
			MinGOT:  "0.1.0",
			Path:    "/home/u/.got/plugins/got-slack",
			Source:  "repo",
		},
	}
	deps := pluginDepsFor(stdout, stderr, &fakeAdapter{}, dir, plugins)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"plugin", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("plugin list: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "github") || !strings.Contains(out, "slack") {
		t.Errorf("expected both plugins in table, got:\n%s", out)
	}
	if !strings.Contains(out, "PATH") || !strings.Contains(out, "repo") {
		t.Errorf("expected PATH and repo source columns, got:\n%s", out)
	}
}

func TestPluginCmd_ListJSON(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	plugins := []plugin.DiscoveredPlugin{
		{Name: "github", Version: "1.2.0", MinGOT: "0.1.0", Path: "/usr/local/bin/got-github", Source: "PATH"},
	}
	deps := pluginDepsFor(stdout, stderr, &fakeAdapter{}, dir, plugins)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"plugin", "list", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("plugin list --json: %v", err)
	}
	var got []plugin.DiscoveredPlugin
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nout=%s", err, stdout.String())
	}
	if len(got) != 1 || got[0].Name != "github" {
		t.Errorf("got = %+v, want one plugin named github", got)
	}
}

func TestPluginCmd_Info(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	plugins := []plugin.DiscoveredPlugin{
		{
			Name:     "github",
			Version:  "1.2.0",
			MinGOT:   "0.1.0",
			Path:     "/usr/local/bin/got-github",
			Source:   "PATH",
			Commands: []plugin.ManifestCommand{{Name: "pr", Description: "Open a PR"}},
		},
	}
	deps := pluginDepsFor(stdout, stderr, &fakeAdapter{}, dir, plugins)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"plugin", "info", "github"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("plugin info: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"github", "1.2.0", "0.1.0", "pr", "Open a PR"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestPluginCmd_InfoUnknownFails(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := pluginDepsFor(stdout, stderr, &fakeAdapter{}, dir, nil)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"plugin", "info", "ghost"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for unknown plugin, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestPluginCmd_InstallStub(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := pluginDepsFor(stdout, stderr, &fakeAdapter{}, dir, nil)
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"plugin", "install", "/tmp/foo"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error from install stub, got nil")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("expected 'not yet implemented' error, got: %v", err)
	}
}

func TestPluginCmd_NotInGitRepoFails(t *testing.T) {
	dir := t.TempDir() // not a git repo
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := pluginDepsFor(stdout, stderr, &fakeAdapter{}, "/nope", nil)
	// The default DiscoverPlugins resolves the git repo first; we
	// stub it to return a NotInGitRepo error so runPluginList
	// fails the same way it would in production outside a repo.
	deps.DiscoverPlugins = func(_ context.Context) ([]plugin.DiscoveredPlugin, error) {
		return nil, gerr.NotInGitRepo(".")
	}
	cmd := NewRootCmd(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"plugin", "list"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error outside git repo, got nil")
	}
	if !strings.Contains(err.Error(), "not inside a Git repository") {
		t.Errorf("expected not-in-git-repo error, got: %v", err)
	}
}

func TestRegisterPluginCommands_AddsParentAndChildren(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	plugins := []plugin.DiscoveredPlugin{
		{
			Name:     "github",
			Version:  "1.2.0",
			MinGOT:   "0.1.0",
			Path:     "/usr/local/bin/got-github",
			Source:   "PATH",
			Commands: []plugin.ManifestCommand{{Name: "pr", Description: "Open a PR"}},
		},
	}
	deps := pluginDepsFor(stdout, stderr, &fakeAdapter{}, dir, plugins)
	cmd := NewRootCmd(deps)

	added := registerPluginCommands(cmd, deps)
	if added != 1 {
		t.Fatalf("registerPluginCommands added = %d, want 1", added)
	}
	// Find the `pr` child and invoke its RunE directly. Going
	// through sub.Execute() is fragile because the parent's
	// SilenceErrors/SilenceUsage are inherited from root and can
	// swallow the child error; RunE is the deterministic surface.
	var prCmd *cobra.Command
	for _, sub := range cmd.Commands() {
		if sub.Name() != "github" {
			continue
		}
		for _, child := range sub.Commands() {
			if child.Name() == "pr" {
				prCmd = child
				break
			}
		}
	}
	if prCmd == nil {
		t.Fatalf("pr grandchild not found under github subcommand")
	}
	if prCmd.RunE == nil {
		t.Fatalf("pr grandchild has no RunE")
	}
	err := prCmd.RunE(prCmd, nil)
	if err == nil {
		t.Fatalf("expected error from plugin invocation stub, got nil")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("expected 'not yet implemented' error, got: %v", err)
	}
}

func TestRegisterPluginCommands_NilDiscovererIsNoop(t *testing.T) {
	dir := initGitRepo(t)
	withChdir(t, dir)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	// Deps with NO DiscoverPlugins — registerPluginCommands must
	// short-circuit and not call into it.
	deps := Deps{
		AdapterFor: func(string) git.Adapter { return &fakeAdapter{} },
		Discover:   func(string) (string, error) { return dir, nil },
		IsTerminal: func() bool { return false },
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
		User:       func() string { return "alice" },
		GotVersion: "0.1.0-test",
		Stdout:     stdout,
		Stderr:     stderr,
	}
	cmd := NewRootCmd(deps)
	added := registerPluginCommands(cmd, deps)
	if added != 0 {
		t.Errorf("added = %d, want 0 (nil DiscoverPlugins should be a no-op)", added)
	}
}
