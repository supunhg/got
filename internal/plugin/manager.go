package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Message is a single NDJSON message on the plugin's stdin/stdout
// protocol. Per spec §11:
//
//	Request:  {"type":"call","id":"...","command":"pr","args":{...}}
//	Response: {"type":"result","id":"...","ok":true,"data":{...}}
//	          {"type":"error","id":"...","code":"...","message":"..."}
//	Event:    {"type":"event","id":"...","event":"...","payload":{...}}
//
// Only the response half is modeled here; the request/event
// variants are documented for plugin authors and tested via
// integration tests in a later milestone.
type Message struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Command string          `json:"command,omitempty"`
	Args    json.RawMessage `json:"args,omitempty"`
	OK      bool            `json:"ok,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	Code    string          `json:"code,omitempty"`
	Message string          `json:"message,omitempty"`
	Event   string          `json:"event,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// InvocationTimeout is the default per-invocation timeout for plugin
// calls. Spec §11 sets the CLI default to 30s, overridable via
// `--plugin-timeout`.
const InvocationTimeout = 30 * time.Second

// Manager is the v0.1 stub for plugin invocation. Per spec §11, the
// real loader (planned for v0.5) launches the plugin as a
// subprocess and exchanges NDJSON messages. v0.1 only supports
// discovery + manifest validation, so Manager.Call returns a clear
// "not yet implemented" error directing users to the spec.
type Manager struct {
	// Timeout caps each call. Defaults to InvocationTimeout.
	Timeout time.Duration
}

// Call would launch the plugin and exchange NDJSON messages. In
// v0.1 it returns an error pointing at the spec.
func (m *Manager) Call(_ context.Context, _ *DiscoveredPlugin, _ string, _ json.RawMessage) (*Message, error) {
	return nil, fmt.Errorf("plugin: live invocation is not yet implemented in v0.1; see got-spec.md §11 (planned for v0.5)")
}
