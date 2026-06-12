// Package plugin implements the GOT plugin system per got-spec.md §11.
//
// v0.1 ships zero plugins. The package is fully wired so plugin
// authors can start building immediately: discovery (PATH + repo),
// manifest parsing + validation, and command registration. The
// live NDJSON invocation protocol is stubbed and lands in v0.5.
package plugin

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ManifestVersion is the integer version of the manifest schema this
// build of GOT understands. Per spec §11, manifests with any other
// version are refused: the contract between GOT and plugin authors
// is stable across the v0.1 minor series, and a breaking manifest
// change requires bumping this integer.
const ManifestVersion = 1

// Manifest is the JSON document a plugin binary prints in response to
// `--got-plugin-manifest`. The schema is locked in spec §11; see
// ParseManifest for the validation rules.
type Manifest struct {
	// ManifestVersion is the integer schema version. Must equal
	// ManifestVersion (1) for v0.1.
	ManifestVersion int `json:"manifest_version"`
	// Name is the plugin's short name (e.g. "github"). Used as the
	// Cobra subcommand name (`got <name> <command>`) and as the
	// prefix for the binary (`got-<name>`).
	Name string `json:"name"`
	// Version is the plugin's own semver string. Informational; GOT
	// does not compare plugin versions against each other.
	Version string `json:"version"`
	// MinGOT is the minimum GOT version (semver) the plugin
	// requires. GOT refuses to load a plugin whose min_got is
	// higher than the running binary's version.
	MinGOT string `json:"min_got"`
	// Commands is the list of subcommands the plugin provides.
	// Each is registered under the plugin's parent command.
	Commands []ManifestCommand `json:"commands"`
}

// ManifestCommand describes one subcommand the plugin provides.
// Args is intentionally a raw JSON list so the v0.1 help text can
// echo the plugin's own arg descriptors verbatim; GOT does not
// interpret the contents.
type ManifestCommand struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Args        []ManifestArg `json:"args,omitempty"`
}

// ManifestArg is one positional or flag argument descriptor. Plugins
// can describe their own CLI surface here; GOT surfaces the raw
// values in the help text.
type ManifestArg struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// ParseManifest decodes raw JSON bytes and validates the
// manifest_version field plus the required string fields. The
// min_got semver check is done separately (after we know the
// running version) by MeetsMinGOT, so the parser stays pure.
func ParseManifest(raw []byte) (Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, fmt.Errorf("plugin: invalid manifest JSON: %w", err)
	}
	if m.ManifestVersion == 0 {
		return m, fmt.Errorf("plugin: manifest is missing manifest_version")
	}
	if m.ManifestVersion != ManifestVersion {
		return m, fmt.Errorf("plugin: manifest_version %d is not supported (this build of GOT only understands %d)",
			m.ManifestVersion, ManifestVersion)
	}
	if strings.TrimSpace(m.Name) == "" {
		return m, fmt.Errorf("plugin: manifest is missing name")
	}
	if strings.TrimSpace(m.MinGOT) == "" {
		return m, fmt.Errorf("plugin: manifest is missing min_got")
	}
	if len(m.Commands) == 0 {
		return m, fmt.Errorf("plugin: manifest has no commands")
	}
	for i, c := range m.Commands {
		if strings.TrimSpace(c.Name) == "" {
			return m, fmt.Errorf("plugin: command #%d is missing name", i)
		}
	}
	return m, nil
}

// MeetsMinGOT reports whether the running version (a semver string
// like "0.1.0") satisfies the plugin's min_got requirement.
//
// Special cases:
//   - Empty min_got is treated as "no constraint" (returns true) so
//     old manifests that predate the field keep loading.
//   - Running versions that fail semver parsing (e.g. "dev" in
//     local builds) are treated as "satisfies everything" so plugin
//     development works in `go run` / unbuilt-binary workflows.
func MeetsMinGOT(manifestMin, running string) (bool, error) {
	if manifestMin == "" {
		return true, nil
	}
	a, err := parseSemver(manifestMin)
	if err != nil {
		return false, fmt.Errorf("plugin: invalid min_got %q: %w", manifestMin, err)
	}
	b, err := parseSemver(running)
	if err != nil {
		// "dev" or other non-semver running version: assume the
		// plugin's constraint is met. This is the right direction
		// for development (false negatives block plugin authors
		// from testing locally).
		return true, nil
	}
	return compareSemver(b, a) >= 0, nil
}

// parseSemver parses a "MAJOR.MINOR.PATCH" string into [3]int.
// Pre-release / build metadata (e.g. "0.1.0-rc1") is rejected: GOT
// only ever ships the three-integer form internally, and plugin
// authors who want a pre-release can encode it in the plugin's
// own Version field.
func parseSemver(s string) ([3]int, error) {
	var out [3]int
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return out, fmt.Errorf("expected MAJOR.MINOR.PATCH, got %q", s)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, fmt.Errorf("non-numeric version component %q", p)
		}
		if n < 0 {
			return out, fmt.Errorf("negative version component %d", n)
		}
		out[i] = n
	}
	return out, nil
}

// compareSemver returns -1 if a<b, 0 if a==b, +1 if a>b.
func compareSemver(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
