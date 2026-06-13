package workspace

import (
	"regexp"
	"time"
)

// State is the lifecycle state of a Workspace. Stored as TEXT in
// SQLite; the value must round-trip through State.Set / State.String
// for the validation in Store.Update and Store.Create to succeed.
type State string

const (
	// StateOpen is the default state. An open workspace is the
	// "live" view: it appears in `got workspace list` by default
	// and is the only state `got workspace add-file` will accept
	// (the CLI refuses to attach files to an archived workspace).
	StateOpen State = "open"
	// StateArchived hides the workspace from the default list and
	// from status counts. An archived workspace is preserved on
	// disk and can be re-opened with `got workspace update
	// <name> --state open`. We do not delete archived workspaces
	// automatically; the user can `got workspace delete` them
	// explicitly.
	StateArchived State = "archived"
)

// Valid reports whether s is a known State. Used by Store.Update to
// reject bad input from the CLI; a bad value is reported as a
// validation error so the user sees a clear message instead of a
// SQLite constraint violation.
func (s State) Valid() bool {
	switch s {
	case StateOpen, StateArchived:
		return true
	default:
		return false
	}
}

// DecisionStatus is the lifecycle of a workspace-scoped decision.
// The vocabulary is intentionally identical to the global ADR
// statuses in got-spec.md §12 (decisions table) so future code can
// share a renderer.
type DecisionStatus string

const (
	// DecisionProposed is the initial state: the user wrote it
	// down but has not committed to it.
	DecisionProposed DecisionStatus = "proposed"
	// DecisionAccepted means the decision is in force.
	DecisionAccepted DecisionStatus = "accepted"
	// DecisionRejected means the decision was considered and
	// turned down. The body is preserved for the historical
	// record ("we considered X and decided against it because
	// Y").
	DecisionRejected DecisionStatus = "rejected"
	// DecisionSuperseded means a newer decision replaces this
	// one. The body is preserved; no automatic link is created
	// to the successor (the user can put it in the body).
	DecisionSuperseded DecisionStatus = "superseded"
)

// Valid reports whether s is a known DecisionStatus.
func (s DecisionStatus) Valid() bool {
	switch s {
	case DecisionProposed, DecisionAccepted, DecisionRejected, DecisionSuperseded:
		return true
	default:
		return false
	}
}

// nameRe validates a workspace name. The constraints are:
//   - 1..63 characters (the SQLite TEXT column has no limit but
//     the CLI surface shouldn't accept unbounded input).
//   - Starts with a lowercase ASCII letter.
//   - Body is lowercase ASCII letters, digits, hyphen, or
//     underscore. The UNIQUE index in the schema does the
//     collision check; this regex is a fast first line of defense
//     so the user gets a clean validation error before we hit
//     the database.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)

// ValidName reports whether name is a syntactically valid workspace
// slug. The check is purely lexical; uniqueness is enforced by the
// SQLite UNIQUE index in Store.Create.
func ValidName(name string) bool {
	return nameRe.MatchString(name)
}

// Workspace is the root aggregate. The slug (Name) is the
// user-facing identifier; the ID is the opaque primary key the
// database uses internally. Plugins and external scripts should
// match on Name (which is unique within a repo) and treat ID as
// implementation detail.
type Workspace struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Color       string         `json:"color,omitempty"`
	State       State          `json:"state"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// WorkspaceFile records that the user wants the named repo-relative
// path tracked under the given workspace. `Note` is an optional
// free-form annotation (e.g. "touched in PR #42"). The combination
// (workspace_id, path) is the primary key, so adding the same
// path twice is idempotent (the second AddFile updates the
// existing row's added_at and note).
type WorkspaceFile struct {
	WorkspaceID string    `json:"workspaceId"`
	Path        string    `json:"path"`
	AddedAt     time.Time `json:"addedAt"`
	Note        string    `json:"note,omitempty"`
}

// WorkspaceBranch records that the user has tagged the named Git
// branch as relevant to the given workspace. Branches are recorded
// as the short ref name (e.g. "feature/x", "main"); the workspace
// does not own the branch. LastSeenAt is exposed for future health
// / staleness detection — the CLI does not write to it in v0.4,
// but the field is in the schema so v0.5+ commands can without a
// migration.
type WorkspaceBranch struct {
	WorkspaceID string    `json:"workspaceId"`
	Branch      string    `json:"branch"`
	AddedAt     time.Time `json:"addedAt"`
	LastSeenAt  time.Time `json:"lastSeenAt,omitempty"`
}

// WorkspaceDecision is a lightweight ADR scoped to a single
// workspace. The body is markdown; the CLI never interprets it.
// Status defaults to "proposed"; the user promotes it to
// "accepted" / "rejected" / "superseded" via Workspace.Update.
type WorkspaceDecision struct {
	ID          string         `json:"id"`
	WorkspaceID string         `json:"workspaceId"`
	Title       string         `json:"title"`
	Body        string         `json:"body,omitempty"`
	Status      DecisionStatus `json:"status"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// WorkspaceNote is a free-form markdown note attached to a
// workspace. Pinned notes appear at the top of the workspace's
// show output and are typically used for "what this workspace is
// about" / "current focus" / "blockers". Notes are not
// versioned; updating a note overwrites the previous body.
type WorkspaceNote struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	Body        string    `json:"body"`
	Pinned      bool      `json:"pinned"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// ListOptions controls the filter and ordering of Store.List.
// The zero value lists all workspaces sorted by created_at DESC,
// which is what `got workspace list` shows by default.
type ListOptions struct {
	// State, when non-empty, restricts the list to workspaces
	// in that state. The CLI uses StateOpen for the default
	// view and StateArchived for `--archived`.
	State State
	// Limit caps the number of returned rows; 0 means "no
	// limit". Set when the CLI wants a one-line summary.
	Limit int
}

// ShowView is the JSON shape of `got workspace show --json`. It
// bundles the workspace aggregate with its files/branches/
// decisions/notes so a single subprocess call is enough for a
// plugin or external script to render the workspace. The CLI
// renders the same shape as a human-readable multi-section
// block when --json is not set.
//
// ShowView is exported so plugin authors and external tools
// (e.g. scripts that shell out to `got workspace show --json`)
// can decode the output with json.Unmarshal without a
// package-local mirror struct.
type ShowView struct {
	Workspace *Workspace          `json:"workspace"`
	Files     []WorkspaceFile     `json:"files"`
	Branches  []WorkspaceBranch   `json:"branches"`
	Decisions []WorkspaceDecision `json:"decisions"`
	Notes     []WorkspaceNote     `json:"notes"`
}

// CountsByWorkspace is the per-workspace breakdown of child
// table counts. Each map is keyed by workspace ID; missing keys
// mean zero rows. The map keys are stable across calls (always
// workspace IDs, never names) so the caller can stitch the
// result into a rowset by index. Used by `got workspace list`
// to render the FILES/BRANCHES/DECISIONS/NOTES columns in one
// batched query per child table (4 queries for the whole list,
// not per-row).
type CountsByWorkspace struct {
	Files     map[string]int `json:"files"`
	Branches  map[string]int `json:"branches"`
	Decisions map[string]int `json:"decisions"`
	Notes     map[string]int `json:"notes"`
}

// GetID returns the workspace's primary key. Defined as a method
// (not a field accessor) so *workspace.Workspace satisfies the
// publishWorkspaceEvent source interface in internal/cli without
// forcing the CLI to depend on a private mirror type.
func (w *Workspace) GetID() string { return w.ID }

// GetName returns the workspace's unique slug.
func (w *Workspace) GetName() string { return w.Name }

// GetTitle returns the human-readable title.
func (w *Workspace) GetTitle() string { return w.Title }

// GetState returns the current state as a string.
func (w *Workspace) GetState() string { return string(w.State) }
