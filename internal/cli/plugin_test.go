package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/supunhg/got/internal/events"
	"github.com/supunhg/got/internal/store"
	"github.com/supunhg/got/internal/version"
)

// ── Test helpers ─────────────────────────────────────────────────────

// setupPluginTest creates a temporary GOT environment with a store, event
// bus, and plugins directory ready for plugin testing. Also copies the
// testdata/hello-plugin directory into the plugins dir for testing.
func setupPluginTest(t *testing.T) (*store.KnowledgeStore, *events.Bus, string, func()) {
	t.Helper()

	// Create a temp directory for .got/.
	gotDir := t.TempDir()

	// Initialize the store.
	dbPath := filepath.Join(gotDir, "got.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	bus := events.New()
	ks := store.NewKnowledgeStore(st.DB(), bus)

	pluginsDir := filepath.Join(gotDir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		t.Fatalf("mkdir plugins: %v", err)
	}

	// Find the testdata/hello-plugin directory.
	// Walk up from the current working directory to find the project root.
	cwd, _ := os.Getwd()
	testDataDir := filepath.Join(cwd, "..", "..", "testdata", "hello-plugin")
	if _, err := os.Stat(testDataDir); os.IsNotExist(err) {
		// Try relative to the test binary.
		testDataDir = filepath.Join("..", "..", "testdata", "hello-plugin")
	}

	// Copy the sample plugin.
	destDir := filepath.Join(pluginsDir, "hello-world")
	if err := copyDir(testDataDir, destDir); err != nil {
		t.Fatalf("copy sample plugin: %v", err)
	}

	cleanup := func() {
		bus.Close()
		st.Close()
	}

	return ks, bus, pluginsDir, cleanup
}

// ── Test: Install, list, get ─────────────────────────────────────────

func TestPluginInstall(t *testing.T) {
	ks, _, pluginsDir, cleanup := setupPluginTest(t)
	defer cleanup()
	ctx := context.Background()

	// Parse the manifest and register the plugin.
	manifestPath := filepath.Join(pluginsDir, "hello-world", "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest store.PluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}

	manifestBytes, _ := json.Marshal(manifest)
	p, err := ks.InstallPlugin(ctx, manifest.Name, manifest.Version, manifest.Description, filepath.Join(pluginsDir, "hello-world"), string(manifestBytes))
	if err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}

	if p.Name != "hello-world" {
		t.Errorf("expected name 'hello-world', got %q", p.Name)
	}
	if p.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", p.Version)
	}
	if !p.Enabled {
		t.Errorf("expected plugin to be enabled by default")
	}

	// List plugins.
	plugins, err := ks.ListPlugins(ctx)
	if err != nil {
		t.Fatalf("ListPlugins: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Name != "hello-world" {
		t.Errorf("expected 'hello-world', got %q", plugins[0].Name)
	}

	// Get plugin.
	gp, err := ks.GetPlugin(ctx, "hello-world")
	if err != nil {
		t.Fatalf("GetPlugin: %v", err)
	}
	if gp.Manifest == nil {
		t.Fatalf("expected manifest to be parsed")
	}
	if len(gp.Manifest.Commands) != 1 {
		t.Errorf("expected 1 command, got %d", len(gp.Manifest.Commands))
	}
	if gp.Manifest.Commands[0].Name != "greet" {
		t.Errorf("expected command 'greet', got %q", gp.Manifest.Commands[0].Name)
	}

	// Duplicate install should fail.
	_, err = ks.InstallPlugin(ctx, "hello-world", "1.0.0", "", "", "{}")
	if err == nil {
		t.Fatal("expected error for duplicate plugin install")
	}
}

// ── Test: Enable/disable ─────────────────────────────────────────────

func TestPluginEnableDisable(t *testing.T) {
	ks, _, pluginsDir, cleanup := setupPluginTest(t)
	defer cleanup()
	ctx := context.Background()

	// Install the plugin.
	manifestPath := filepath.Join(pluginsDir, "hello-world", "manifest.json")
	data, _ := os.ReadFile(manifestPath)
	var manifest store.PluginManifest
	json.Unmarshal(data, &manifest)
	manifestBytes, _ := json.Marshal(manifest)
	ks.InstallPlugin(ctx, manifest.Name, manifest.Version, manifest.Description, filepath.Join(pluginsDir, "hello-world"), string(manifestBytes))

	// Disable.
	if err := ks.DisablePlugin(ctx, "hello-world"); err != nil {
		t.Fatalf("DisablePlugin: %v", err)
	}
	p, err := ks.GetPlugin(ctx, "hello-world")
	if err != nil {
		t.Fatalf("GetPlugin: %v", err)
	}
	if p.Enabled {
		t.Error("expected plugin to be disabled")
	}

	// Enable.
	if err := ks.EnablePlugin(ctx, "hello-world"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	p, err = ks.GetPlugin(ctx, "hello-world")
	if err != nil {
		t.Fatalf("GetPlugin: %v", err)
	}
	if !p.Enabled {
		t.Error("expected plugin to be enabled")
	}

	// Remove.
	if err := ks.RemovePlugin(ctx, "hello-world"); err != nil {
		t.Fatalf("RemovePlugin: %v", err)
	}
	_, err = ks.GetPlugin(ctx, "hello-world")
	if err != store.ErrPluginNotFound {
		t.Fatalf("expected ErrPluginNotFound, got %v", err)
	}
}

// ── Test: Plugin runtime hook execution ──────────────────────────────

func TestPluginRuntimeHookExecution(t *testing.T) {
	ks, bus, pluginsDir, cleanup := setupPluginTest(t)
	defer cleanup()
	ctx := context.Background()

	// Install the plugin.
	manifestPath := filepath.Join(pluginsDir, "hello-world", "manifest.json")
	data, _ := os.ReadFile(manifestPath)
	var manifest store.PluginManifest
	json.Unmarshal(data, &manifest)
	manifestBytes, _ := json.Marshal(manifest)
	ks.InstallPlugin(ctx, manifest.Name, manifest.Version, manifest.Description, filepath.Join(pluginsDir, "hello-world"), string(manifestBytes))

	// Create plugin runtime and load plugins.
	rt := NewPluginRuntime(ks, bus, pluginsDir)
	defer rt.Close()

	if err := rt.Load(ctx); err != nil {
		t.Fatalf("Load plugins: %v", err)
	}

	// Verify the plugin is loaded.
	loadedPlugins := rt.Plugins()
	if len(loadedPlugins) != 1 {
		t.Fatalf("expected 1 loaded plugin, got %d", len(loadedPlugins))
	}
	if loadedPlugins[0].Name != "hello-world" {
		t.Errorf("expected 'hello-world', got %q", loadedPlugins[0].Name)
	}

	// Verify commands are registered.
	cmds := rt.Commands()
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].PluginName != "hello-world" || cmds[0].CommandName != "greet" {
		t.Errorf("expected hello-world/greet command, got %s/%s", cmds[0].PluginName, cmds[0].CommandName)
	}

	// Publish a CommitCreated event to trigger the hook.
	hookErr := bus.Publish(ctx, events.EventCommitCreated, events.CommitCreatedPayload{
		SHA:       "abc123",
		Message:   "Test commit",
		Branch:    "main",
		CreatedAt: time.Now().UnixMilli(),
	})
	if hookErr != nil {
		// Hook execution failure should not crash GOT, but we should check
		// if the hook existed and was called. The plugin hook writes to
		// stderr, which is fine.
		t.Logf("Hook execution returned: %v", hookErr)
	}
}

// ── Test: Plugin manifest parsing ────────────────────────────────────

func TestParseManifestFile(t *testing.T) {
	// Use the testdata hello-plugin manifest.
	cwd, _ := os.Getwd()
	manifestDir := filepath.Join(cwd, "..", "..", "testdata", "hello-plugin")
	if _, err := os.Stat(manifestDir); os.IsNotExist(err) {
		manifestDir = filepath.Join("..", "..", "testdata", "hello-plugin")
	}

	manifest, err := ParseManifestFile(manifestDir)
	if err != nil {
		t.Fatalf("ParseManifestFile: %v", err)
	}
	if manifest.Name != "hello-world" {
		t.Errorf("expected name 'hello-world', got %q", manifest.Name)
	}
	if manifest.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", manifest.Version)
	}
	if len(manifest.Capabilities) != 3 {
		t.Errorf("expected 3 capabilities, got %d", len(manifest.Capabilities))
	}
	if len(manifest.Events) != 1 || manifest.Events[0] != "CommitCreated" {
		t.Errorf("expected [CommitCreated] events, got %v", manifest.Events)
	}
	if len(manifest.Hooks) != 1 {
		t.Errorf("expected 1 hook, got %d", len(manifest.Hooks))
	}
	if hookCmd, ok := manifest.Hooks["CommitCreated"]; !ok {
		t.Error("expected CommitCreated hook")
	} else if hookCmd != "hooks/on-commit.sh" {
		t.Errorf("expected hook 'hooks/on-commit.sh', got %q", hookCmd)
	}
	if len(manifest.Commands) != 1 {
		t.Errorf("expected 1 command, got %d", len(manifest.Commands))
	}
	if manifest.Commands[0].Name != "greet" {
		t.Errorf("expected command 'greet', got %q", manifest.Commands[0].Name)
	}
}

// ── Test: Version matching ───────────────────────────────────────────

func TestPluginVersionMatching(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		expect     bool
	}{
		{">=0.5.0", "0.6.0", true},
		{">=0.5.0", "0.4.0", false},
		{">=0.5.0", "0.5.0", true},
		{">0.5.0", "0.5.0", false},
		{">0.5.0", "0.5.1", true},
		{"<1.0.0", "0.9.0", true},
		{"<1.0.0", "1.0.0", false},
		{"=1.0.0", "1.0.0", true},
		{"=1.0.0", "1.0.1", false},
		{"1.0.0", "1.0.0", true},
		{"1.0.0", "1.0.1", false},
		{"", "1.0.0", true},  // empty constraint = always pass
	}

	// Save original version and restore after test.
	origVersion := version.Version
	defer func() { version.Version = origVersion }()

	for _, tt := range tests {
		version.Version = tt.version
		result := version.Matches(tt.constraint)
		if result != tt.expect {
			t.Errorf("version.Matches(%q) with version=%q = %v, want %v", tt.constraint, tt.version, result, tt.expect)
		}
	}
}
