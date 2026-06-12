package plugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/got-sh/got/internal/config"
)

const validManifestForInstall = `{"manifest_version":1,"name":"github","version":"1.2.0","min_got":"0.1.0","commands":[{"name":"pr","description":"open a PR"}]}`

// TestInstallFromPath_Success copies a fake plugin binary into a
// fresh .got/plugins/ and checks the destination exists, is
// executable, and carries the manifest name "github".
func TestInstallFromPath_Success(t *testing.T) {
	dir := t.TempDir()
	src := writeFakePlugin(t, dir, "source-github", validManifestForInstall, 0)
	workTree := filepath.Join(dir, "repo")
	if err := os.MkdirAll(workTree, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	inst := NewInstaller(workTree)
	res, err := inst.InstallFromPath(src)
	if err != nil {
		t.Fatalf("InstallFromPath: %v", err)
	}
	if res.Name != "github" {
		t.Errorf("Name = %q, want github", res.Name)
	}
	if res.Source != "path" {
		t.Errorf("Source = %q, want path", res.Source)
	}
	want := filepath.Join(workTree, ".got", "plugins", "got-github")
	if res.Path != want {
		t.Errorf("Path = %q, want %q", res.Path, want)
	}
	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat installed: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("installed binary is not executable: mode=%v", info.Mode())
	}
}

func TestInstallFromPath_RefusesMissing(t *testing.T) {
	inst := NewInstaller(t.TempDir())
	_, err := inst.InstallFromPath("/no/such/file")
	if err == nil {
		t.Fatalf("expected error for missing source, got nil")
	}
}

func TestInstallFromPath_RefusesNonExecutable(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "not-exec")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho hi\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	inst := NewInstaller(t.TempDir())
	_, err := inst.InstallFromPath(src)
	if err == nil {
		t.Fatalf("expected error for non-executable source, got nil")
	}
	if !strings.Contains(err.Error(), "not executable") {
		t.Errorf("expected 'not executable' error, got: %v", err)
	}
}

func TestInstallFromPath_RefusesBadManifest(t *testing.T) {
	dir := t.TempDir()
	// A binary that exits non-zero on --got-plugin-manifest, so
	// the manifest probe fails.
	src := writeFakePlugin(t, dir, "broken", "", 1)
	inst := NewInstaller(t.TempDir())
	_, err := inst.InstallFromPath(src)
	if err == nil {
		t.Fatalf("expected error for broken manifest, got nil")
	}
}

func TestInstallFromPath_RefusesExistingWithoutOverwrite(t *testing.T) {
	dir := t.TempDir()
	src := writeFakePlugin(t, dir, "src", validManifestForInstall, 0)
	workTree := filepath.Join(dir, "repo")
	if err := os.MkdirAll(workTree, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	plugins := filepath.Join(workTree, ".got", "plugins")
	if err := os.MkdirAll(plugins, 0o755); err != nil {
		t.Fatalf("mkdir plugins: %v", err)
	}
	// Drop a pre-existing binary at the destination.
	if err := os.WriteFile(filepath.Join(plugins, "got-github"), []byte("old"), 0o755); err != nil {
		t.Fatalf("write pre-existing: %v", err)
	}
	inst := NewInstaller(workTree)
	_, err := inst.InstallFromPath(src)
	if err == nil {
		t.Fatalf("expected error for existing destination, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' error, got: %v", err)
	}
}

func TestInstallFromPath_Overwrite(t *testing.T) {
	dir := t.TempDir()
	src := writeFakePlugin(t, dir, "src", validManifestForInstall, 0)
	workTree := filepath.Join(dir, "repo")
	if err := os.MkdirAll(workTree, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	plugins := filepath.Join(workTree, ".got", "plugins")
	if err := os.MkdirAll(plugins, 0o755); err != nil {
		t.Fatalf("mkdir plugins: %v", err)
	}
	if err := os.WriteFile(filepath.Join(plugins, "got-github"), []byte("old"), 0o755); err != nil {
		t.Fatalf("write pre-existing: %v", err)
	}
	inst := NewInstaller(workTree)
	inst.Overwrite = true
	if _, err := inst.InstallFromPath(src); err != nil {
		t.Fatalf("InstallFromPath with overwrite: %v", err)
	}
}

func TestInstallFromGit_RefusesBadURL(t *testing.T) {
	inst := NewInstaller(t.TempDir())
	_, err := inst.InstallFromGit("not a url")
	if err == nil {
		t.Fatalf("expected error for bad URL, got nil")
	}
	if !strings.Contains(err.Error(), "does not look like a git URL") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInstallFromGit_SuccessEnd2End(t *testing.T) {
	// Build a fake plugin source dir containing a got-* executable
	// and a manifest. Then stub RunCmd to skip the real `git
	// clone` and just point it at the source dir.
	srcDir := t.TempDir()
	writeFakePlugin(t, srcDir, "got-github", validManifestForInstall, 0)
	workTree := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(workTree, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	inst := NewInstaller(workTree)
	// RunCmd stub: `git clone` would populate dst; emulate by
	// copying the source dir into the temp dst path passed as the
	// last arg.
	inst.RunCmd = func(_ context.Context, name string, args ...string) error {
		isGit := name == "git" || strings.HasSuffix(name, "/git")
		if isGit && len(args) >= 2 && args[0] == "clone" {
			dst := args[len(args)-1]
			return copyDir(srcDir, dst)
		}
		return nil
	}
	res, err := inst.InstallFromGit("https://example.com/fake.git")
	if err != nil {
		t.Fatalf("InstallFromGit: %v", err)
	}
	if res.Name != "github" {
		t.Errorf("Name = %q, want github", res.Name)
	}
	want := filepath.Join(workTree, ".got", "plugins", "got-github")
	if res.Path != want {
		t.Errorf("Path = %q, want %q", res.Path, want)
	}
}

func TestInstallFromGit_ClonedRepoHasNoBinary(t *testing.T) {
	// Source dir with NO got-* binary at the root.
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	workTree := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(workTree, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	inst := NewInstaller(workTree)
	inst.RunCmd = func(_ context.Context, name string, args ...string) error {
		isGit := name == "git" || strings.HasSuffix(name, "/git")
		if isGit && len(args) >= 2 && args[0] == "clone" {
			dst := args[len(args)-1]
			return copyDir(srcDir, dst)
		}
		return nil
	}
	_, err := inst.InstallFromGit("https://example.com/fake.git")
	if err == nil {
		t.Fatalf("expected error when clone has no got-* binary, got nil")
	}
	if !strings.Contains(err.Error(), "no executable got-* binary") {
		t.Errorf("expected 'no executable got-*' error, got: %v", err)
	}
}

func TestEnable_AddsToYAML(t *testing.T) {
	dir := t.TempDir()
	inst := NewInstaller(dir)
	cfg, err := inst.Enable("github")
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if len(cfg.Plugins.Enabled) != 1 || cfg.Plugins.Enabled[0] != "github" {
		t.Errorf("cfg.Plugins.Enabled = %v, want [github]", cfg.Plugins.Enabled)
	}
	// File should now exist on disk.
	body, err := os.ReadFile(inst.ProjectConfigPath)
	if err != nil {
		t.Fatalf("read got.yml: %v", err)
	}
	if !strings.Contains(string(body), "github") {
		t.Errorf("got.yml body missing 'github': %s", body)
	}
}

func TestEnable_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	inst := NewInstaller(dir)
	if _, err := inst.Enable("github"); err != nil {
		t.Fatalf("first Enable: %v", err)
	}
	cfg, err := inst.Enable("github")
	if err != nil {
		t.Fatalf("second Enable: %v", err)
	}
	if len(cfg.Plugins.Enabled) != 1 {
		t.Errorf("expected 1 entry after idempotent enable, got %d: %v", len(cfg.Plugins.Enabled), cfg.Plugins.Enabled)
	}
}

func TestDisable_RemovesFromYAML(t *testing.T) {
	dir := t.TempDir()
	inst := NewInstaller(dir)
	if _, err := inst.Enable("github"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if _, err := inst.Enable("slack"); err != nil {
		t.Fatalf("Enable slack: %v", err)
	}
	cfg, err := inst.Disable("github")
	if err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if len(cfg.Plugins.Enabled) != 1 || cfg.Plugins.Enabled[0] != "slack" {
		t.Errorf("cfg.Plugins.Enabled = %v, want [slack]", cfg.Plugins.Enabled)
	}
}

func TestDisable_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	inst := NewInstaller(dir)
	cfg, err := inst.Disable("ghost")
	if err != nil {
		t.Fatalf("Disable of un-enabled plugin: %v", err)
	}
	if len(cfg.Plugins.Enabled) != 0 {
		t.Errorf("expected empty enabled list, got %v", cfg.Plugins.Enabled)
	}
}

func TestDisable_PreservesExistingYAML(t *testing.T) {
	// Pre-existing got.yml with other fields populated must roundtrip.
	dir := t.TempDir()
	gotYml := filepath.Join(dir, "got.yml")
	original := config.DefaultProjectConfig()
	original.Project.Name = "myrepo"
	original.Project.DefaultBranch = "main"
	original.Plugins.Enabled = []string{"github"}
	body, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(gotYml, body, 0o644); err != nil {
		t.Fatalf("write got.yml: %v", err)
	}
	inst := NewInstaller(dir)
	if _, err := inst.Disable("github"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	got, err := config.ReadProjectConfig(gotYml)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Project.Name != "myrepo" {
		t.Errorf("Project.Name = %q, want myrepo (Disable clobbered other fields)", got.Project.Name)
	}
}

func TestIsEnabled(t *testing.T) {
	dir := t.TempDir()
	inst := NewInstaller(dir)
	on, err := inst.IsEnabled("github")
	if err != nil {
		t.Fatalf("IsEnabled (no got.yml): %v", err)
	}
	if on {
		t.Errorf("expected false when got.yml missing, got true")
	}
	if _, err := inst.Enable("github"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	on, err = inst.IsEnabled("github")
	if err != nil {
		t.Fatalf("IsEnabled (post-Enable): %v", err)
	}
	if !on {
		t.Errorf("expected true after Enable, got false")
	}
}

func TestEnabledSet(t *testing.T) {
	dir := t.TempDir()
	inst := NewInstaller(dir)
	for _, n := range []string{"github", "slack"} {
		if _, err := inst.Enable(n); err != nil {
			t.Fatalf("Enable %s: %v", n, err)
		}
	}
	set, err := inst.EnabledSet()
	if err != nil {
		t.Fatalf("EnabledSet: %v", err)
	}
	if !set["github"] || !set["slack"] || set["ghost"] {
		t.Errorf("set = %v, want github+slack true", set)
	}
}

func TestLooksLikeGitURL(t *testing.T) {
	good := []string{
		"https://example.com/repo.git",
		"http://example.com/repo",
		"git://github.com/foo/bar.git",
		"ssh://git@example.com/repo.git",
		"git+https://example.com/repo.git",
		"file:///tmp/repo",
		"git@github.com:foo/bar.git",
		"git@host:path/to/repo",
		"/local/path/repo.git",
	}
	bad := []string{
		"",
		"not a url",
		"github.com/foo/bar", // missing scheme and user@
		"foo",                // too short
	}
	for _, g := range good {
		if !LooksLikeGitURL(g) {
			t.Errorf("LooksLikeGitURL(%q) = false, want true", g)
		}
	}
	for _, b := range bad {
		if LooksLikeGitURL(b) {
			t.Errorf("LooksLikeGitURL(%q) = true, want false", b)
		}
	}
}

func TestEmptyNameRejected(t *testing.T) {
	inst := NewInstaller(t.TempDir())
	if _, err := inst.Enable(""); err == nil {
		t.Errorf("expected error for empty enable name")
	}
	if _, err := inst.Disable(""); err == nil {
		t.Errorf("expected error for empty disable name")
	}
}

// copyDir recursively copies src to dst (which must not exist).
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, info.Mode().Perm())
	})
}
