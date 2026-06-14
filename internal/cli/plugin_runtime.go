// Copyright 2026 Supun Hewagamage. MIT License.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/supunhg/got/internal/events"
	"github.com/supunhg/got/internal/store"
	"github.com/supunhg/got/internal/version"
)

const (
	// PluginDirName is the subdirectory inside .got/ where plugins are stored.
	PluginDirName = "plugins"

	// DefaultManifestFile is the default manifest filename to look for.
	DefaultManifestFile = "manifest.json"
)

// PluginCommand describes a command registered by a plugin.
type PluginCommand struct {
	PluginName  string
	CommandName string
	Description string
	ExecPath    string // absolute path to the executable
}

// PluginRuntime manages plugin lifecycle: loading, hook subscriptions, and
// command registration. It operates on the .got/plugins/<name>/ directory
// structure and the plugins table in the SQLite store.
type PluginRuntime struct {
	ks       *store.KnowledgeStore
	bus      *events.Bus
	plugins  []store.Plugin
	commands []PluginCommand
	// unsubscribers are the bus unsubscribe functions for each plugin's hooks.
	unsubscribers []func()
	pluginsDir    string
}

// NewPluginRuntime creates a PluginRuntime for the given store, bus, and
// plugins directory. It does not load plugins — call Load() to do that.
func NewPluginRuntime(ks *store.KnowledgeStore, bus *events.Bus, pluginsDir string) *PluginRuntime {
	return &PluginRuntime{
		ks:            ks,
		bus:           bus,
		plugins:       nil,
		commands:      nil,
		unsubscribers: nil,
		pluginsDir:    pluginsDir,
	}
}

// Load reads all installed and enabled plugins from the store, validates
// their manifests, and subscribes their event hooks to the bus. It returns
// any validation errors but always loads all valid plugins (best-effort).
func (pr *PluginRuntime) Load(ctx context.Context) error {
	pr.Close() // unsubscribe any previous hooks

	dbPlugins, err := pr.ks.ListPlugins(ctx)
	if err != nil {
		return fmt.Errorf("plugin runtime: list plugins: %w", err)
	}

	var loadErrors []string

	for _, p := range dbPlugins {
		if !p.Enabled {
			continue
		}

		manifest := p.Manifest
		if manifest == nil {
			loadErrors = append(loadErrors, fmt.Sprintf("%s: nil manifest", p.Name))
			continue
		}

		// Validate required fields.
		if manifest.Name == "" {
			loadErrors = append(loadErrors, fmt.Sprintf("%s: manifest missing 'name'", p.Name))
			continue
		}
		if manifest.Version == "" {
			loadErrors = append(loadErrors, fmt.Sprintf("%s: manifest missing 'version'", p.Name))
			continue
		}

		// Check GOT version compatibility.
		if manifest.RequiresGotVersion != "" {
			if !version.Matches(manifest.RequiresGotVersion) {
				loadErrors = append(loadErrors, fmt.Sprintf(
					"%s: requires GOT version %s (current: %s)",
					p.Name, manifest.RequiresGotVersion, version.String(),
				))
				continue
			}
		}

		// Subscribe event hooks.
		for eventType, scriptPath := range manifest.Hooks {
			et := eventType  // capture
			sp := scriptPath // capture
			pName := p.Name  // capture
			pPath := p.Path  // capture

			unsub, subErr := pr.bus.Subscribe(et, func(ctx context.Context, e events.Event) error {
				return pr.executeHook(ctx, pName, pPath, sp, e)
			})
			if subErr != nil {
				loadErrors = append(loadErrors, fmt.Sprintf("%s: subscribe %s: %v", p.Name, et, subErr))
				continue
			}
			pr.unsubscribers = append(pr.unsubscribers, unsub)
		}

		// Register plugin commands.
		for _, cmd := range manifest.Commands {
			execPath := cmd.Executable
			if !filepath.IsAbs(execPath) {
				execPath = filepath.Join(p.Path, cmd.Executable)
			}
			pr.commands = append(pr.commands, PluginCommand{
				PluginName:  p.Name,
				CommandName: cmd.Name,
				Description: cmd.Description,
				ExecPath:    execPath,
			})
		}

		pr.plugins = append(pr.plugins, p)
	}

	if len(loadErrors) > 0 {
		return fmt.Errorf("plugin runtime: %s", strings.Join(loadErrors, "; "))
	}

	return nil
}

// Commands returns the list of plugin-registered commands.
func (pr *PluginRuntime) Commands() []PluginCommand {
	return pr.commands
}

// Plugins returns the currently loaded plugins.
func (pr *PluginRuntime) Plugins() []store.Plugin {
	return pr.plugins
}

// ExecuteCommand runs a plugin command with the given arguments.
func (pr *PluginRuntime) ExecuteCommand(ctx context.Context, pluginName, commandName string, args []string, timeout time.Duration) (string, string, error) {
	for _, c := range pr.commands {
		if c.PluginName == pluginName && c.CommandName == commandName {
			return pr.runExec(ctx, c.ExecPath, args, timeout)
		}
	}
	return "", "", fmt.Errorf("plugin command %s/%s not found", pluginName, commandName)
}

// RunAction manually triggers a plugin's action (from hooks or a named action).
// It looks for an executable at hooks/<actionName> (or the literal path from
// the hook's entry). Returns stdout, stderr, error.
func (pr *PluginRuntime) RunAction(ctx context.Context, pluginName, action string, timeout time.Duration) (string, string, error) {
	plugin, err := pr.findPlugin(pluginName)
	if err != nil {
		return "", "", err
	}

	manifest := plugin.Manifest
	if manifest == nil {
		return "", "", fmt.Errorf("plugin %s has no manifest loaded", pluginName)
	}

	// Check if there's a hook for this action name (event type).
	if scriptPath, ok := manifest.Hooks[action]; ok {
		execPath := scriptPath
		if !filepath.IsAbs(execPath) {
			execPath = filepath.Join(plugin.Path, execPath)
		}
		return pr.runExec(ctx, execPath, nil, timeout)
	}

	// Check if there's a commands entry.
	for _, cmd := range manifest.Commands {
		if cmd.Name == action {
			execPath := cmd.Executable
			if !filepath.IsAbs(execPath) {
				execPath = filepath.Join(plugin.Path, execPath)
			}
			return pr.runExec(ctx, execPath, nil, timeout)
		}
	}

	return "", "", fmt.Errorf("plugin %s has no action or hook named %q", pluginName, action)
}

// Close unsubscribes all plugin event hooks and clears internal state.
func (pr *PluginRuntime) Close() {
	for _, unsub := range pr.unsubscribers {
		unsub()
	}
	pr.unsubscribers = nil
	pr.plugins = nil
	pr.commands = nil
}

// ── internal helpers ────────────────────────────────────────────────

// executeHook runs a plugin hook script with the event data passed as JSON
// via stdin. The hook runs in a subprocess for isolation.
func (pr *PluginRuntime) executeHook(ctx context.Context, pluginName, pluginPath, scriptPath string, e events.Event) error {
	execPath := scriptPath
	if !filepath.IsAbs(execPath) {
		execPath = filepath.Join(pluginPath, execPath)
	}

	// Serialize event data as JSON for stdin.
	payload := map[string]any{
		"type":      e.Type,
		"payload":   e.Payload,
		"timestamp": e.Timestamp,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("plugin %s: marshal event: %w", pluginName, err)
	}

	// Execute the hook script with a default 30s timeout.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, execPath)
	cmd.Dir = pluginPath
	cmd.Stdin = bytes.NewReader(data)

	// Collect but don't propagate stderr (hooks log there, it's fine).
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, runErr := cmd.Output()
	if runErr != nil {
		// Non-zero exit is logged but doesn't crash GOT.
		_, _ = fmt.Fprintf(os.Stderr, "plugin %s hook %s: %v\n  stderr: %s\n  stdout: %s\n",
			pluginName, scriptPath, runErr, stderr.String(), string(output))
		return fmt.Errorf("plugin %s hook %s: %w", pluginName, scriptPath, runErr)
	}

	return nil
}

// runExec runs an arbitrary executable with the given args and timeout.
func (pr *PluginRuntime) runExec(ctx context.Context, execPath string, args []string, timeout time.Duration) (string, string, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, execPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// findPlugin looks up a loaded plugin by name.
func (pr *PluginRuntime) findPlugin(name string) (*store.Plugin, error) {
	for _, p := range pr.plugins {
		if p.Name == name {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("plugin %q not found (not loaded or not enabled)", name)
}

// ── Manifest parsing ────────────────────────────────────────────────

// ParseManifestFile reads and parses a manifest.json from the given directory.
func ParseManifestFile(pluginDir string) (*store.PluginManifest, error) {
	manifestPath := filepath.Join(pluginDir, DefaultManifestFile)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var manifest store.PluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	if manifest.Name == "" {
		return nil, fmt.Errorf("manifest missing required field: name")
	}
	if manifest.Version == "" {
		return nil, fmt.Errorf("manifest missing required field: version")
	}

	return &manifest, nil
}
