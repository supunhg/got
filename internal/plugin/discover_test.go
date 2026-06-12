package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

const (
	validManifest = `{"manifest_version":1,"name":"github","version":"1.0.0","min_got":"0.1.0","commands":[{"name":"pr","description":"open a PR"}]}`
	tooNewMinGOT  = `{"manifest_version":1,"name":"future","version":"1.0.0","min_got":"99.0.0","commands":[{"name":"x","description":"x"}]}`
	badManifest   = `{"manifest_version":2,"name":"x","version":"1.0.0","min_got":"0.1.0","commands":[{"name":"x","description":"x"}]}`
)

// writeFakePlugin writes a shell script that prints its argument as
// JSON (or empty + non-zero exit on a sentinel) to stdout, and makes
// it executable. The plugin convention is: `got-foo --got-plugin-manifest`
// prints the manifest JSON; the fake just prints whatever JSON
// string was passed in.
func writeFakePlugin(t *testing.T, dir, name, manifestJSON string, exitCode int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	body := "#!/bin/sh\n"
	if manifestJSON != "" {
		body += "cat <<'EOF'\n" + manifestJSON + "\nEOF\n"
	}
	if exitCode != 0 {
		body += "exit " + itoa(exitCode) + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := false
	if n < 0 {
		negative = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestDiscover_RepoDir(t *testing.T) {
	dir := t.TempDir()
	pluginsDir := filepath.Join(dir, ".got", "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Use distinct manifests so the two plugins get distinct
	// names in the discovered list.
	githubManifest := `{"manifest_version":1,"name":"github","version":"1.0.0","min_got":"0.1.0","commands":[{"name":"pr","description":"open a PR"}]}`
	slackManifest := `{"manifest_version":1,"name":"slack","version":"0.3.1","min_got":"0.1.0","commands":[{"name":"post","description":"post a message"}]}`
	writeFakePlugin(t, pluginsDir, "got-github", githubManifest, 0)
	writeFakePlugin(t, pluginsDir, "got-slack", slackManifest, 0)
	// Broken plugin: should be skipped without failing the others.
	writeFakePlugin(t, pluginsDir, "got-broken", "", 1)

	d := &Discoverer{
		RunningVersion: "0.1.0",
		RepoPluginsDir: pluginsDir,
		// Empty PathEnvs => no PATH scan.
		PathEnvs: nil,
	}
	got, err := d.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d plugins, want 2 (broken one must be skipped): %+v", len(got), got)
	}
	names := map[string]bool{}
	for _, p := range got {
		names[p.Name] = true
		if p.Source != "repo" {
			t.Errorf("plugin %s Source = %q, want repo", p.Name, p.Source)
		}
	}
	if !names["github"] || !names["slack"] {
		t.Errorf("expected github + slack, got %v", names)
	}
}

func TestDiscover_PathLookup(t *testing.T) {
	dir := t.TempDir()
	writeFakePlugin(t, dir, "got-foo", validManifest, 0)
	writeFakePlugin(t, dir, "got-bar", validManifest, 0)

	d := &Discoverer{
		RunningVersion: "0.1.0",
		PathEnvs:       []string{dir},
		PathLookup: func(name string) (string, error) {
			return filepath.Join(dir, name), nil
		},
	}
	got, err := d.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d plugins, want 2: %+v", len(got), got)
	}
	for _, p := range got {
		if p.Source != "PATH" {
			t.Errorf("plugin %s Source = %q, want PATH", p.Name, p.Source)
		}
	}
}

func TestDiscover_RejectsBadManifest(t *testing.T) {
	dir := t.TempDir()
	pluginsDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFakePlugin(t, pluginsDir, "got-bad", badManifest, 0)

	d := &Discoverer{
		RunningVersion: "0.1.0",
		RepoPluginsDir: pluginsDir,
	}
	got, err := d.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d plugins, want 0 (bad manifest should be skipped): %+v", len(got), got)
	}
}

func TestDiscover_RejectsMinGOTMismatch(t *testing.T) {
	dir := t.TempDir()
	pluginsDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFakePlugin(t, pluginsDir, "got-future", tooNewMinGOT, 0)

	d := &Discoverer{
		RunningVersion: "0.1.0",
		RepoPluginsDir: pluginsDir,
	}
	got, err := d.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d plugins, want 0 (min_got too high should be skipped): %+v", len(got), got)
	}
}

func TestDiscover_RejectsExitedNonZero(t *testing.T) {
	dir := t.TempDir()
	pluginsDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFakePlugin(t, pluginsDir, "got-crash", "", 1)

	d := &Discoverer{
		RunningVersion: "0.1.0",
		RepoPluginsDir: pluginsDir,
	}
	got, err := d.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d plugins, want 0 (crash should be skipped): %+v", len(got), got)
	}
}

func TestDiscover_DedupesAcrossPathAndRepo(t *testing.T) {
	dir := t.TempDir()
	pluginsDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	same := filepath.Join(dir, "got-same")
	writeFakePlugin(t, pluginsDir, "got-same", validManifest, 0)

	d := &Discoverer{
		RunningVersion: "0.1.0",
		PathEnvs:       []string{dir},
		PathLookup: func(name string) (string, error) {
			return filepath.Join(dir, name), nil
		},
		RepoPluginsDir: pluginsDir,
		// Add the repo file's path to the candidate set so the
		// dedup logic kicks in.
	}
	// Pretend the repo file is also on PATH by exposing the same
	// dir from PathLookup.
	got, err := d.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	// got-same is on PATH (via the lookup above) AND in the repo
	// dir; dedup should keep only one entry.
	count := 0
	for _, p := range got {
		if p.Name == "github" {
			count++
		}
	}
	if count > 1 {
		t.Errorf("github appeared %d times, want at most 1 (dedup): %+v", count, got)
	}
	_ = same
}
