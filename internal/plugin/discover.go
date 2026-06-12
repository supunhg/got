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
)

// DiscoveredPlugin is a plugin binary that successfully returned a
// valid manifest AND passed the min_got version check.
type DiscoveredPlugin struct {
	// Name from the manifest (e.g. "github").
	Name string
	// Version from the manifest.
	Version string
	// MinGOT from the manifest.
	MinGOT string
	// Path to the plugin binary on disk (absolute or relative to
	// whatever the caller passed in).
	Path string
	// Source is "PATH" for binaries found on $PATH, "repo" for
	// binaries under .got/plugins/.
	Source string
	// Commands is the manifest's command list, copied for the
	// caller's convenience.
	Commands []ManifestCommand
}

// Discoverer finds and validates plugin binaries. The default
// implementation walks $PATH and the repo's .got/plugins/ directory
// (per spec §11) and runs `--got-plugin-manifest` against each
// candidate.
type Discoverer struct {
	// RunningVersion is the semver of the running GOT binary, used
	// for the min_got check. Empty defaults to "0.0.0", which
	// rejects every plugin that declares a min_got (the safe
	// direction).
	RunningVersion string
	// RepoPluginsDir overrides the .got/plugins/ path. Empty means
	// "do not scan the repo plugins directory".
	RepoPluginsDir string
	// PathLookup is the function used to look up `got-*` binaries
	// on $PATH. Defaults to exec.LookPath.
	PathLookup func(name string) (string, error)
	// PathEnvs holds the $PATH-like directories to scan. Empty
	// means "use $PATH from the environment".
	PathEnvs []string
	// ManifestTimeout caps how long a single --got-plugin-manifest
	// invocation is allowed to run. Defaults to 5s.
	ManifestTimeout time.Duration
}

// Discover runs the full discovery pipeline: scan $PATH for
// executables starting with "got-", then scan RepoPluginsDir for
// executables; for each, run `--got-plugin-manifest`, parse the
// JSON, and filter by the min_got check. Invalid plugins (bad
// manifest, wrong version, non-zero exit, timeout) are silently
// skipped: a broken plugin in one slot must not block discovery
// of the others.
func (d *Discoverer) Discover(ctx context.Context) ([]DiscoveredPlugin, error) {
	timeout := d.ManifestTimeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	running := d.RunningVersion
	if running == "" {
		running = "0.0.0"
	}

	seen := make(map[string]bool)
	var found []DiscoveredPlugin

	// 1) $PATH: walk every directory and pick up got-* executables.
	for _, name := range d.candidatePathNames() {
		path, err := d.lookupPath(name)
		if err != nil {
			continue
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		p, err := d.probe(ctx, path, "PATH", timeout, running)
		if err != nil {
			continue
		}
		found = append(found, p)
	}

	// 2) Repo plugins dir.
	if d.RepoPluginsDir != "" {
		entries, err := os.ReadDir(d.RepoPluginsDir)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				full := filepath.Join(d.RepoPluginsDir, e.Name())
				info, err := os.Stat(full)
				if err != nil || info.Mode()&0o111 == 0 {
					continue
				}
				if seen[full] {
					continue
				}
				seen[full] = true
				p, err := d.probe(ctx, full, "repo", timeout, running)
				if err != nil {
					continue
				}
				found = append(found, p)
			}
		}
	}

	return found, nil
}

// candidatePathNames enumerates the basenames of `got-*` executables
// found under every directory in $PATH (or d.PathEnvs).
func (d *Discoverer) candidatePathNames() []string {
	envs := d.PathEnvs
	if len(envs) == 0 {
		envs = filepath.SplitList(os.Getenv("PATH"))
	}
	var out []string
	seen := make(map[string]bool)
	for _, dir := range envs {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
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
			// Skip non-executable files on unix. Windows uses the
			// .exe extension instead; the mode bit test is a no-op
			// there because the .exe bit is set by the linker.
			if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
				continue
			}
			if seen[e.Name()] {
				continue
			}
			seen[e.Name()] = true
			out = append(out, e.Name())
		}
	}
	return out
}

// lookupPath resolves a `got-*` name to a full path.
func (d *Discoverer) lookupPath(name string) (string, error) {
	if d.PathLookup != nil {
		return d.PathLookup(name)
	}
	return exec.LookPath(name)
}

// probe runs `<path> --got-plugin-manifest` and parses the result.
// Returns a non-nil error for any failure (bad exit, bad JSON,
// manifest validation, min_got mismatch); the caller decides
// whether to log + skip or surface to the user.
func (d *Discoverer) probe(ctx context.Context, path, source string, timeout time.Duration, running string) (DiscoveredPlugin, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, path, "--got-plugin-manifest")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return DiscoveredPlugin{}, fmt.Errorf("plugin %s: --got-plugin-manifest failed: %w", path, err)
	}
	m, err := ParseManifest(stdout.Bytes())
	if err != nil {
		return DiscoveredPlugin{}, fmt.Errorf("plugin %s: %w", path, err)
	}
	ok, err := MeetsMinGOT(m.MinGOT, running)
	if err != nil {
		return DiscoveredPlugin{}, fmt.Errorf("plugin %s: %w", path, err)
	}
	if !ok {
		return DiscoveredPlugin{}, fmt.Errorf("plugin %s (%s): requires min_got %s, running %s",
			path, m.Name, m.MinGOT, running)
	}
	return DiscoveredPlugin{
		Name:     m.Name,
		Version:  m.Version,
		MinGOT:   m.MinGOT,
		Path:     path,
		Source:   source,
		Commands: m.Commands,
	}, nil
}
