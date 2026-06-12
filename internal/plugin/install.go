package plugin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/got-sh/got/internal/config"
	"github.com/got-sh/got/internal/gerr"
)

// InstallResult is what the Installer hands back after a successful
// install: the final on-disk path under the repo's .got/plugins/
// dir, the plugin's manifest name, and where the source came from
// ("path" or "git").
type InstallResult struct {
	// Name is the plugin's manifest name (e.g. "github"), which is
	// also the suffix of the destination binary (e.g. "got-github").
	Name string
	// Path is the absolute path to the installed binary.
	Path string
	// Source is "path" for InstallFromPath and "git" for
	// InstallFromGit. Useful in CLI output.
	Source string
	// Version is the manifest version, when one was parseable. May
	// be empty for installs that were not able to probe the
	// manifest (rare; we always probe when possible).
	Version string
}

// Installer handles plugin install / enable / disable for a single
// repo. The zero value is invalid; construct one via NewInstaller so
// the repo paths and config path are resolved once.
//
// All operations are best-effort safe: enabling an already-enabled
// plugin is a no-op, disabling a non-enabled plugin is a no-op, and
// install refuses to overwrite an existing binary unless Overwrite is
// set.
type Installer struct {
	// WorkTree is the absolute path to the repo's work tree. The
	// Installer writes binaries under WorkTree/.got/plugins/ and
	// reads/writes WorkTree/got.yml.
	WorkTree string
	// RepoPluginsDir is the absolute path to .got/plugins/ inside
	// the work tree. It is computed from WorkTree when NewInstaller
	// is used.
	RepoPluginsDir string
	// ProjectConfigPath is the absolute path to got.yml. Defaults
	// to WorkTree/got.yml.
	ProjectConfigPath string
	// Overwrite allows install to clobber an existing binary at
	// the destination path. Defaults to false; when false, install
	// returns an error if the destination already exists.
	Overwrite bool
	// GitTimeout caps a single `git clone` invocation. Defaults
	// to 60s.
	GitTimeout time.Duration
	// LookPath is exec.LookPath (overridable for tests).
	LookPath func(name string) (string, error)
	// RunCmd is the function used to run external commands
	// (currently just `git clone`). Defaults to a real exec; tests
	// can stub it.
	RunCmd func(ctx context.Context, name string, args ...string) error
}

// NewInstaller builds an Installer rooted at the given work tree.
// It is the only constructor callers should use; it fills in the
// derived paths and the default function pointers.
func NewInstaller(workTree string) *Installer {
	plugins := filepath.Join(workTree, ".got", "plugins")
	return &Installer{
		WorkTree:          workTree,
		RepoPluginsDir:    plugins,
		ProjectConfigPath: filepath.Join(workTree, "got.yml"),
		GitTimeout:        60 * time.Second,
		LookPath:          exec.LookPath,
		RunCmd:            defaultRunCmd,
	}
}

// defaultRunCmd runs an external command with the given args. It
// captures stdout/stderr into a discard buffer (we never use them)
// and returns the error from Run.
func defaultRunCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	return cmd.Run()
}

// InstallFromPath copies an executable at srcPath into the repo's
// .got/plugins/ directory, renaming it to got-<manifest-name> based
// on the source binary's manifest. The destination is set executable.
//
// Refuses if:
//   - srcPath does not exist or is not a regular file
//   - srcPath's manifest is invalid (so we cannot derive the
//     destination name)
//   - the destination already exists and Overwrite is false
//   - WorkTree / RepoPluginsDir is empty (Installer was zero-initialized)
func (i *Installer) InstallFromPath(srcPath string) (InstallResult, error) {
	if i.WorkTree == "" {
		return InstallResult{}, gerr.Validation("plugin: installer has no work tree")
	}
	if i.RepoPluginsDir == "" {
		return InstallResult{}, gerr.Validation("plugin: installer has no repo plugins dir")
	}
	if srcPath == "" {
		return InstallResult{}, gerr.Validation("plugin: install source path is empty")
	}
	info, err := os.Stat(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return InstallResult{}, gerr.Validation(fmt.Sprintf("plugin: install source %s does not exist", srcPath))
		}
		if os.IsPermission(err) {
			return InstallResult{}, gerr.PermissionDenied(srcPath)
		}
		return InstallResult{}, gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("stat %s", srcPath))
	}
	if info.IsDir() {
		return InstallResult{}, gerr.Validation(fmt.Sprintf("plugin: install source %s is a directory, expected a file", srcPath))
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		return InstallResult{}, gerr.Validation(fmt.Sprintf("plugin: install source %s is not executable", srcPath))
	}

	// Probe the source to read its manifest; this is what tells us
	// the plugin's name. We use a short timeout so a broken binary
	// cannot stall the install.
	m, err := probeManifest(srcPath, 5*time.Second)
	if err != nil {
		return InstallResult{}, err
	}
	if m.Name == "" {
		return InstallResult{}, gerr.Validation(fmt.Sprintf("plugin: %s returned an empty manifest name", srcPath))
	}

	destName := "got-" + m.Name
	dest := filepath.Join(i.RepoPluginsDir, destName)

	if _, err := os.Stat(dest); err == nil {
		if !i.Overwrite {
			return InstallResult{}, gerr.Validation(fmt.Sprintf("plugin: %s already exists in .got/plugins/ (use --force to overwrite)", destName))
		}
	} else if !os.IsNotExist(err) {
		if os.IsPermission(err) {
			return InstallResult{}, gerr.PermissionDenied(dest)
		}
		return InstallResult{}, gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("stat %s", dest))
	}

	if err := os.MkdirAll(i.RepoPluginsDir, 0o755); err != nil {
		if os.IsPermission(err) {
			return InstallResult{}, gerr.PermissionDenied(i.RepoPluginsDir)
		}
		return InstallResult{}, gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("mkdir %s", i.RepoPluginsDir))
	}
	if err := copyFile(srcPath, dest, 0o755); err != nil {
		return InstallResult{}, gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("copy %s -> %s", srcPath, dest))
	}
	return InstallResult{
		Name:    m.Name,
		Path:    dest,
		Source:  "path",
		Version: m.Version,
	}, nil
}

// InstallFromGit shallow-clones the given git URL into a temp dir,
// scans the resulting tree for an executable named got-*, picks the
// first one, and copies it into the repo's .got/plugins/ dir (renaming
// it to match the manifest name as for InstallFromPath).
//
// A plugin git repo is expected to contain either:
//   - a pre-built binary named got-<name> at the repo root, or
//   - a single executable file anywhere in the tree whose manifest
//     declares name=<name>.
//
// The first executable got-* found at the repo root wins. If no
// such binary is present, the install fails with a clear error
// suggesting the user build the plugin first.
//
// Refuses if the URL is not a syntactically plausible git URL
// (must start with http://, https://, git://, ssh://, or be a
// path-like form containing a colon, like user@host:path).
func (i *Installer) InstallFromGit(gitURL string) (InstallResult, error) {
	if i.WorkTree == "" {
		return InstallResult{}, gerr.Validation("plugin: installer has no work tree")
	}
	if i.RepoPluginsDir == "" {
		return InstallResult{}, gerr.Validation("plugin: installer has no repo plugins dir")
	}
	if !LooksLikeGitURL(gitURL) {
		return InstallResult{}, gerr.Validation(fmt.Sprintf("plugin: %q does not look like a git URL (expected http(s)://, git://, ssh://, or user@host:path)", gitURL))
	}

	timeout := i.GitTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Look up git on PATH so we get a clear error early if it's
	// not installed.
	gitBin := "git"
	if i.LookPath != nil {
		if resolved, err := i.LookPath(gitBin); err == nil {
			gitBin = resolved
		}
	}

	tmp, err := os.MkdirTemp("", "got-plugin-clone-")
	if err != nil {
		if os.IsPermission(err) {
			return InstallResult{}, gerr.PermissionDenied(os.TempDir())
		}
		return InstallResult{}, gerr.Wrap(gerr.CodeGeneric, err, "create temp dir")
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	run := i.RunCmd
	if run == nil {
		run = defaultRunCmd
	}
	if err := run(ctx, gitBin, "clone", "--depth", "1", gitURL, tmp); err != nil {
		return InstallResult{}, gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("git clone %s", gitURL))
	}

	// Find the first got-* executable under the clone root.
	src, err := findExecutablePlugin(tmp)
	if err != nil {
		return InstallResult{}, err
	}
	// Reuse the path installer; it handles manifest probing, name
	// derivation, and copy.
	return i.InstallFromPath(src)
}

// Enable adds the named plugin to got.yml's plugins.enabled list. If
// the plugin is already enabled, this is a no-op. If got.yml does
// not exist, an empty ProjectConfig is created and written. The
// returned ProjectConfig is the post-mutation value (useful for tests
// and for printing the new state to the user).
func (i *Installer) Enable(name string) (config.ProjectConfig, error) {
	if name == "" {
		return config.ProjectConfig{}, gerr.Validation("plugin: enable name is empty")
	}
	if i.ProjectConfigPath == "" {
		return config.ProjectConfig{}, gerr.Validation("plugin: installer has no project config path")
	}
	cfg, err := readOrDefaultProjectConfig(i.ProjectConfigPath)
	if err != nil {
		return config.ProjectConfig{}, err
	}
	for _, e := range cfg.Plugins.Enabled {
		if e == name {
			return cfg, nil // already enabled; no-op
		}
	}
	cfg.Plugins.Enabled = append(cfg.Plugins.Enabled, name)
	if err := writeProjectConfig(i.ProjectConfigPath, cfg); err != nil {
		return config.ProjectConfig{}, err
	}
	return cfg, nil
}

// Disable removes the named plugin from got.yml's plugins.enabled
// list. If the plugin is not currently enabled, this is a no-op
// (returns the unchanged config and a nil error). If got.yml does
// not exist, the empty default config is returned unchanged.
func (i *Installer) Disable(name string) (config.ProjectConfig, error) {
	if name == "" {
		return config.ProjectConfig{}, gerr.Validation("plugin: disable name is empty")
	}
	if i.ProjectConfigPath == "" {
		return config.ProjectConfig{}, gerr.Validation("plugin: installer has no project config path")
	}
	cfg, err := readOrDefaultProjectConfig(i.ProjectConfigPath)
	if err != nil {
		return config.ProjectConfig{}, err
	}
	out := cfg.Plugins.Enabled[:0]
	found := false
	for _, e := range cfg.Plugins.Enabled {
		if e == name {
			found = true
			continue
		}
		out = append(out, e)
	}
	if !found {
		return cfg, nil // not enabled; no-op
	}
	cfg.Plugins.Enabled = out
	if err := writeProjectConfig(i.ProjectConfigPath, cfg); err != nil {
		return config.ProjectConfig{}, err
	}
	return cfg, nil
}

// IsEnabled reports whether name is in got.yml's plugins.enabled list.
// A missing or unparseable got.yml is treated as "nothing enabled".
func (i *Installer) IsEnabled(name string) (bool, error) {
	if i.ProjectConfigPath == "" {
		return false, nil
	}
	cfg, err := readOrDefaultProjectConfig(i.ProjectConfigPath)
	if err != nil {
		return false, err
	}
	for _, e := range cfg.Plugins.Enabled {
		if e == name {
			return true, nil
		}
	}
	return false, nil
}

// EnabledSet returns the set of enabled plugin names from got.yml.
// A missing or unparseable got.yml is treated as the empty set.
func (i *Installer) EnabledSet() (map[string]bool, error) {
	out := make(map[string]bool)
	if i.ProjectConfigPath == "" {
		return out, nil
	}
	cfg, err := readOrDefaultProjectConfig(i.ProjectConfigPath)
	if err != nil {
		return nil, err
	}
	for _, e := range cfg.Plugins.Enabled {
		out[e] = true
	}
	return out, nil
}

// probeManifest runs `--got-plugin-manifest` against path with a
// short timeout and parses the result.
func probeManifest(path string, timeout time.Duration) (Manifest, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--got-plugin-manifest")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Manifest{}, gerr.PluginError("", err, fmt.Sprintf("--got-plugin-manifest failed for %s", path))
	}
	m, err := ParseManifest(stdout.Bytes())
	if err != nil {
		return Manifest{}, gerr.PluginError("", err, fmt.Sprintf("invalid manifest from %s", path))
	}
	return m, nil
}

// copyFile copies src to dst with the given mode. It does not
// preserve extended attributes; that is fine for plugin binaries.
// On permission errors it returns *gerr.Error with the dst path
// included so the CLI can surface a precise "permission denied"
// message and a hint at the fix.
func copyFile(src, dst string, mode os.FileMode) error {
	body, err := os.ReadFile(src)
	if err != nil {
		if os.IsPermission(err) {
			return gerr.PermissionDenied(src)
		}
		return err
	}
	if err := os.WriteFile(dst, body, mode); err != nil {
		if os.IsPermission(err) {
			return gerr.PermissionDenied(dst)
		}
		return err
	}
	return nil
}

// LooksLikeGitURL does a tiny syntactic check so callers can fail
// fast on obviously bad input (e.g. `--install foo` where foo is a
// flag accidentally typed as a URL). It is intentionally
// permissive: real validation happens when `git clone` runs.
//
// Exported because the CLI uses it to decide which installer
// branch (path vs git) to dispatch to before doing any work.
func LooksLikeGitURL(s string) bool {
	if s == "" {
		return false
	}
	prefixes := []string{"http://", "https://", "git://", "ssh://", "git+", "file://"}
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	// SCP-like form: user@host:path
	if strings.Contains(s, "@") && strings.Contains(s, ":") {
		return true
	}
	// Bare /absolute path with .git suffix is also accepted so
	// `got plugin install ./my-plugin` from a built tree works.
	if strings.HasSuffix(s, ".git") {
		return true
	}
	return false
}

// findExecutablePlugin walks the root of dir and returns the path
// of the first executable file whose name starts with "got-". Only
// the root is scanned: nested binaries could be from subprojects
// or build artifacts, and picking one of those silently would mask
// the right answer (which is "ask the user to build the plugin
// first"). If no executable got-* file is found at the root, an
// error is returned suggesting the plugin be built first.
func findExecutablePlugin(dir string) (string, error) {
	// 1) Repo root: any executable got-* file.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("read %s", dir))
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasPrefix(e.Name(), "got-") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
			continue
		}
		return filepath.Join(dir, e.Name()), nil
	}
	return "", gerr.Validation(fmt.Sprintf("plugin: no executable got-* binary found at the root of the cloned repo (build the plugin first, then commit the binary)"))
}

// readOrDefaultProjectConfig reads got.yml and falls back to a
// fresh default ProjectConfig when the file is missing. A
// non-empty parse error is propagated.
func readOrDefaultProjectConfig(path string) (config.ProjectConfig, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config.DefaultProjectConfig(), nil
		}
		return config.ProjectConfig{}, gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("read %s", path))
	}
	var cfg config.ProjectConfig
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		return config.ProjectConfig{}, gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("parse %s", path))
	}
	return cfg, nil
}

// writeProjectConfig marshals cfg to YAML and writes it to path,
// creating parent dirs as needed.
func writeProjectConfig(path string, cfg config.ProjectConfig) error {
	body, err := yaml.Marshal(cfg)
	if err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, "marshal project config")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("mkdir %s", filepath.Dir(path)))
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return gerr.Wrap(gerr.CodeGeneric, err, fmt.Sprintf("write %s", path))
	}
	return nil
}
