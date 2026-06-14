# Plugin Runtime v2

## Overview

The Plugin Runtime v2 provides a safe, local-first extension mechanism for GOT.
Plugins are external code that can:

- Subscribe to any event published on the Event Bus (e.g., `CommitCreated`, `WorkspaceUpdated`)
- Execute shell commands or scripts when events fire (hooks)
- Register new CLI subcommands under `got plugin <name> <command>`
- Declare their capabilities in a manifest

All plugins run in subprocesses via `os/exec` for safety and isolation. No
Go plugin system or shared memory is involved.

## Directory Layout

Installed plugins live in `.got/plugins/<name>/`. The expected directory
structure for a plugin source is:

```
my-plugin/
├── manifest.json         # Required: plugin metadata and configuration
├── hooks/                # Event hook scripts (referenced from manifest)
├── commands/             # CLI command executables (referenced from manifest)
└── ...                   # Any other files the plugin needs
```

## Manifest (`manifest.json`)

```json
{
  "name": "hello-world",
  "version": "1.0.0",
  "description": "A simple GOT plugin",
  "capabilities": ["subscribe-events", "register-commands", "hooks"],
  "events": ["CommitCreated"],
  "hooks": {
    "CommitCreated": "hooks/on-commit.sh"
  },
  "commands": [
    {
      "name": "greet",
      "description": "Print a greeting message",
      "executable": "commands/greet.sh"
    }
  ],
  "requires_got_version": ">=0.1.0"
}
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Unique plugin name (used as CLI identifier and directory name) |
| `version` | string | yes | Semantic version string (e.g. `1.0.0`) |
| `description` | string | no | Human-readable description |
| `capabilities` | []string | no | `subscribe-events`, `register-commands`, `hooks` |
| `events` | []string | no | Event types to subscribe to (e.g. `CommitCreated`) |
| `hooks` | map[string]string | no | Event type → script path (relative to plugin root) |
| `commands` | []Command | no | CLI commands the plugin exposes |
| `requires_got_version` | string | no | Semver constraint (e.g. `>=0.5.0`, `<1.0.0`) |

### Commands

Each command object has:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Command name (used as `got plugin <name> <command>`) |
| `description` | string | no | Help text shown in `got plugin list` |
| `executable` | string | yes | Path to the executable, relative to plugin root |

## Installation Flow

1. User runs `got plugin install <path>`
2. CLI reads and validates `manifest.json` from the source directory
3. Copies the entire source directory to `.got/plugins/<name>/`
4. Registers the plugin in the SQLite `plugins` table
5. Plugin is loaded on next `got` startup (unless `--no-plugins`)

## Startup Flow

1. `NewRootCmd()` creates the root Cobra command and adds all subcommands
2. If `--no-plugins` is NOT set, `PluginRuntime.Load()` is called
3. `Load()` queries all enabled plugins from the store
4. For each plugin:
   - Validates manifest (name, version, GOT version constraint)
   - Subscribes hook scripts to the Event Bus for declared events
   - Registers plugin commands in the runtime's command registry
5. Plugin commands are available under `got plugin <name> <command>`

## Hook Execution

When an event fires on the bus:

1. The runtime serializes the event as JSON: `{"type": "...", "payload": {...}, "timestamp": "..."}`
2. The hook script is executed as a subprocess via `os/exec.CommandContext`
3. The JSON is passed to the script via **stdin**
4. The script has 30 seconds to complete (configurable via `--plugin-timeout`)
5. If the script fails (non-zero exit), the error is logged to stderr but does **not** crash GOT

### Event JSON format passed to hooks

```json
{
  "type": "CommitCreated",
  "payload": {
    "sha": "abc123...",
    "message": "Fix bug in auth",
    "author": "dev@example.com",
    "branch": "fix/auth",
    "created_at": 1700000000000
  },
  "timestamp": "2026-06-14T10:00:00Z"
}
```

## CLI Commands

| Command | Description |
|---------|-------------|
| `got plugin install <path>` | Install a plugin from a local directory |
| `got plugin remove <name>` | Uninstall a plugin |
| `got plugin list` | List installed plugins with version and status |
| `got plugin enable <name>` | Enable a disabled plugin |
| `got plugin disable <name>` | Disable a plugin without removing it |
| `got plugin run <name> <action>` | Manually trigger a plugin hook or command |

## Data Model

### `plugins` table

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT (ULID) | Primary key |
| `name` | TEXT (UNIQUE) | Plugin name from manifest |
| `version` | TEXT | Semver from manifest |
| `description` | TEXT | Human-readable description |
| `path` | TEXT | Absolute path to `.got/plugins/<name>/` |
| `enabled` | INTEGER (0/1) | 1 = enabled (loaded on startup) |
| `manifest_json` | TEXT | Full manifest content as JSON |
| `installed_at` | INTEGER | Unix timestamp in milliseconds |

## Sample Plugin

A complete sample plugin is provided at `testdata/hello-plugin/`:

```
testdata/hello-plugin/
├── manifest.json         # Plugin manifest
├── hooks/
│   └── on-commit.sh      # Hook script for CommitCreated events
└── commands/
    └── greet.sh          # CLI command: "got plugin run hello-world greet"
```

## Security Considerations

- **Subprocess isolation**: All plugin code runs in a separate process with no
  access to GOT's memory space. A crashing plugin cannot crash GOT.
- **No network by default**: Plugins can make network calls via shell commands
  (curl, wget) but the runtime does not provide any networking primitives.
- **Filesystem access**: Plugins have the same filesystem access as the user
  running GOT. Plugin authors should be vetted before installation.
- **Timeout protection**: Hooks are killed after 30 seconds by default to
  prevent runaway scripts.
- **No remote registry**: The current version only supports local path
  installation. A future version may add a registry with signature verification.

## Testing

The test file `internal/cli/plugin_test.go` covers:

- Plugin installation and duplicate detection
- Plugin enable/disable lifecycle
- Plugin manifest parsing
- Plugin runtime hook execution via Event Bus
- Version constraint matching
