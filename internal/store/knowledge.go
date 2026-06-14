package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/got-sh/got/internal/events"
)

// ── Domain types ────────────────────────────────────────────────────

// Decision represents an architectural decision record (ADR).
type Decision struct {
	ID            string  `json:"id"`
	CreatedAt     int64   `json:"created_at"`
	UpdatedAt     int64   `json:"updated_at"`
	Status        string  `json:"status"`
	Title         string  `json:"title"`
	Context       string  `json:"context"`
	Decision      string  `json:"decision"`
	Alternatives  string  `json:"alternatives"`
	Consequences  string  `json:"consequences"`
	BodyPath      string  `json:"body_path"`
	WorkspaceID   *string `json:"workspace_id,omitempty"`
	SupersedesID  *string `json:"supersedes_id,omitempty"`
}

// CreateDecisionParams holds the user-supplied fields for creating a decision.
type CreateDecisionParams struct {
	Title        string
	Context      string
	Decision     string
	Alternatives string
	Consequences string
	WorkspaceID  *string
	SupersedesID *string
}

// DecisionFilter specifies how to filter/list decisions.
type DecisionFilter struct {
	WorkspaceID *string
	Status      *string
	Limit       int  // 0 means use default (20)
	All         bool
}

// DecisionLink represents a link between a decision and a commit, file, or workspace.
type DecisionLink struct {
	ID          string `json:"id"`
	DecisionID  string `json:"decision_id"`
	LinkType    string `json:"link_type"`
	Target      string `json:"target"`
	LineStart   *int   `json:"line_start,omitempty"`
	LineEnd     *int   `json:"line_end,omitempty"`
	Branch      string `json:"branch,omitempty"`
	CreatedAt   int64  `json:"created_at"`
}

// LinkDecisionParams holds the fields for linking a decision.
type LinkDecisionParams struct {
	DecisionID string
	LinkType   string // "commit", "file", "workspace"
	Target     string
	LineStart  *int
	LineEnd    *int
	Branch     string
}

// Note represents a freeform knowledge note.
type Note struct {
	ID          string  `json:"id"`
	CreatedAt   int64   `json:"created_at"`
	UpdatedAt   int64   `json:"updated_at"`
	Message     string  `json:"message"`
	WorkspaceID *string `json:"workspace_id,omitempty"`
	Branch      string  `json:"branch,omitempty"`
	CommitHash  string  `json:"commit_hash,omitempty"`
}

// CreateNoteParams holds fields for creating a note.
type CreateNoteParams struct {
	Message     string
	WorkspaceID *string
	Branch      string
	CommitHash  string
}

// NoteFilter specifies how to filter notes.
type NoteFilter struct {
	WorkspaceID *string
	Limit       int
	All         bool
}

// OnboardingSession represents an onboarding session for a contributor.
type OnboardingSession struct {
	ID          string `json:"id"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
	Participant string `json:"participant"`
	Status      string `json:"status"` // active | completed | paused
}

// OnboardingItem represents a single item (decision, note, file) in an onboarding session.
type OnboardingItem struct {
	ID          string  `json:"id"`
	SessionID   string  `json:"session_id"`
	ItemType    string  `json:"item_type"`
	ItemTarget  string  `json:"item_target"`
	CoveredAt   *int64  `json:"covered_at,omitempty"`
	Skipped     bool    `json:"skipped"`
}

// SearchParams specifies a full-text search across decisions and notes.
type SearchParams struct {
	Query       string  // search term
	Type        *string // "decision", "note", or nil for both
	WorkspaceID *string // optional workspace filter
	Limit       int     // 0 means default (20)
}

// SearchResult is a single match returned by the Search method.
type SearchResult struct {
	Type        string  `json:"type"`         // "decision" or "note"
	ID          string  `json:"id"`
	Title       string  `json:"title"`        // decision title or note message preview
	Status      string  `json:"status,omitempty"` // decision status (empty for notes)
	WorkspaceID *string `json:"workspace_id,omitempty"`
	CreatedAt   int64   `json:"created_at"`
	Score       int     `json:"score"`        // number of fields matched (crude relevance)
}

// OnboardingProgress summarises the scanning progress for a session.
type OnboardingProgress struct {
	Session    OnboardingSession          `json:"session"`
	ByType     map[string]TypeProgress    `json:"by_type"`
	TotalItems int                        `json:"total_items"`
	Covered    int                        `json:"covered"`
	Skipped    int                        `json:"skipped"`
	Remaining  int                        `json:"remaining"`
}

// TypeProgress shows coverage counts for one item type.
type TypeProgress struct {
	Total    int `json:"total"`
	Covered  int `json:"covered"`
	Skipped  int `json:"skipped"`
	Remaining int `json:"remaining"`
}

// ── Workspace domain types ───────────────────────────────────────────

// Workspace represents a logical grouping of related artifacts.
type Workspace struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	Status        string   `json:"status"`          // active | archived
	Tags          []string `json:"tags,omitempty"`   // JSON array stored in DB
	LastCommitSHA string   `json:"last_commit_sha,omitempty"` // most recent commit SHA
	CreatedAt     int64    `json:"created_at"`
	UpdatedAt     int64    `json:"updated_at"`
}

// WorkspaceCommit represents a commit recorded as activity in a workspace.
type WorkspaceCommit struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	CommitSHA   string `json:"commit_sha"`
	BranchName  string `json:"branch_name,omitempty"`
	Message     string `json:"message,omitempty"`
	CreatedAt   int64  `json:"created_at"`
}

// WorkspaceFile represents a file tracked in a workspace.
type WorkspaceFile struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Path        string `json:"path"`
	CreatedAt   int64  `json:"created_at"`
}

// WorkspaceBranch represents a branch tracked in a workspace.
type WorkspaceBranch struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	BranchName  string `json:"branch_name"`
	CreatedAt   int64  `json:"created_at"`
}

// WorkspaceStatus holds a summary of a workspace's contents — metadata,
// tracked files, tracked branches, linked decisions, linked notes, linked
// commits, linked pull requests, linked issues, item count, and last
// activity timestamp.
type WorkspaceStatus struct {
	Workspace    Workspace          `json:"workspace"`
	Files        []WorkspaceFile   `json:"files"`
	Branches     []WorkspaceBranch `json:"branches"`
	Decisions    []Decision         `json:"decisions"`
	Notes        []Note             `json:"notes"`
	Commits      []WorkspaceCommit  `json:"commits,omitempty"`
	PullRequests []PullRequest     `json:"pull_requests,omitempty"`
	Issues       []Issue           `json:"issues,omitempty"`
	ItemCount    int               `json:"item_count"`
	LastActivity int64              `json:"last_activity"`
}

// CreateWorkspaceParams holds fields for creating a workspace.
type CreateWorkspaceParams struct {
	Name        string
	Description string
	Tags        []string
}

// AddWorkspaceCommitParams holds fields for adding a commit to a workspace.
type AddWorkspaceCommitParams struct {
	WorkspaceName string
	CommitSHA     string
	BranchName    string
	Message       string
}

// UpdateWorkspaceParams holds fields for updating a workspace.
type UpdateWorkspaceParams struct {
	Name        *string
	Description *string
	Status      *string // active | archived
	Tags        []string // nil = no change, empty = clear
}

// ── GitHub domain types ──────────────────────────────────────────

// PullRequest represents a GitHub pull request tracked in GOT.
type PullRequest struct {
	ID          string `json:"id"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	State       string `json:"state"`              // open, closed, merged
	Branch      string `json:"branch"`             // head branch
	Base        string `json:"base"`                // target branch
	URL         string `json:"url,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	MergeCommitSHA string `json:"merge_commit_sha,omitempty"`
	MergedAt    int64  `json:"merged_at,omitempty"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// Issue represents a GitHub issue tracked in GOT.
type Issue struct {
	ID          string   `json:"id"`
	Number      int      `json:"number"`
	Title       string   `json:"title"`
	State       string   `json:"state"`          // open, closed
	Labels      []string `json:"labels,omitempty"`
	URL         string   `json:"url,omitempty"`
	WorkspaceID string   `json:"workspace_id,omitempty"`
	CreatedAt   int64    `json:"created_at"`
	UpdatedAt   int64    `json:"updated_at"`
}

// GitHubConfig stores the GitHub integration configuration.
type GitHubConfig struct {
	Token      string `json:"token,omitempty"`
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	BaseBranch string `json:"base_branch"`
	UpdatedAt  int64  `json:"updated_at"`
}

// CreatePullRequestParams holds fields for recording a PR in GOT.
type CreatePullRequestParams struct {
	Number      int
	Title       string
	State       string
	Branch      string
	Base        string
	URL         string
	WorkspaceID string
}

// PRReview represents a GitHub pull request review recorded in GOT.
type PRReview struct {
	ID           string `json:"id"`
	PRNumber     int    `json:"pr_number"`
	Reviewer     string `json:"reviewer"`
	State        string `json:"state"`       // APPROVED, CHANGES_REQUESTED, COMMENTED
	Body         string `json:"body,omitempty"`
	WorkspaceID  string `json:"workspace_id,omitempty"`
	SubmittedAt  int64  `json:"submitted_at"`
}

// CreateReviewParams holds fields for recording a review in GOT.
type CreateReviewParams struct {
	PRNumber     int
	Reviewer     string
	State        string
	Body         string
	WorkspaceID  string
}

// UpdatePullRequestMergeParams holds fields for updating a PR's merge state.
type UpdatePullRequestMergeParams struct {
	Number         int
	MergeCommitSHA string
}

// CreateIssueParams holds fields for recording an issue in GOT.
type CreateIssueParams struct {
	Number      int
	Title       string
	State       string
	Labels      []string
	URL         string
	WorkspaceID string
}

// ── Plugin domain types ───────────────────────────────────────────

// PluginManifest represents the contents of a plugin manifest.json file.
type PluginManifest struct {
	Name             string                  `json:"name"`
	Version          string                  `json:"version"`
	Description      string                  `json:"description,omitempty"`
	Capabilities     []string                `json:"capabilities,omitempty"`
	Events           []string                `json:"events,omitempty"`
	Hooks            map[string]string       `json:"hooks,omitempty"`     // event type -> command/script
	Commands         []PluginManifestCommand `json:"commands,omitempty"`
	RequiresGotVersion string                `json:"requires_got_version,omitempty"`
}

// PluginManifestCommand describes a command the plugin exposes.
type PluginManifestCommand struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Executable  string `json:"executable"` // relative path from plugin root, or "commands/<name>"
}

// Plugin represents an installed plugin in the database.
type Plugin struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Version      string         `json:"version"`
	Description  string         `json:"description,omitempty"`
	Path         string         `json:"path"`
	Enabled      bool           `json:"enabled"`
	ManifestJSON string         `json:"manifest_json,omitempty"`
	Manifest     *PluginManifest `json:"manifest,omitempty"` // parsed from manifest_json on read
	InstalledAt  int64          `json:"installed_at"`
}

// ── Sentinel errors ─────────────────────────────────────────────────

var (
	ErrDecisionNotFound     = fmt.Errorf("decision not found")
	ErrNoteNotFound         = fmt.Errorf("note not found")
	ErrOnboardingNotFound   = fmt.Errorf("onboarding session not found")
	ErrEmptyMessage         = fmt.Errorf("note message cannot be empty")
	ErrInvalidStatus        = fmt.Errorf("invalid decision status")
	ErrInvalidLinkType      = fmt.Errorf("link type must be one of: commit, file, workspace")
	ErrDecisionAlreadyLinked = fmt.Errorf("this link already exists")
	ErrSessionAlreadyComplete = fmt.Errorf("session is already completed")
	ErrWorkspaceNotFound     = fmt.Errorf("workspace not found")
	ErrDuplicateWorkspace    = fmt.Errorf("workspace with this name already exists")
	ErrPluginNotFound        = fmt.Errorf("plugin not found")
	ErrDuplicatePlugin       = fmt.Errorf("plugin with this name already exists")
)

// ── KnowledgeStore ──────────────────────────────────────────────────

// KnowledgeStore provides CRUD operations for the Knowledge Engine
// entities (decisions, notes, onboarding sessions). Every mutating
// operation publishes a corresponding event on the provided Bus.
type KnowledgeStore struct {
	db  *sql.DB
	bus *events.Bus
}

// NewKnowledgeStore creates a KnowledgeStore backed by the given
// database and event bus. The bus may be nil, in which case events
// are silently dropped.
func NewKnowledgeStore(db *sql.DB, bus *events.Bus) *KnowledgeStore {
	return &KnowledgeStore{db: db, bus: bus}
}

// ── Decision CRUD ──────────────────────────────────────────────────

// CreateDecision inserts a new decision and publishes DecisionCreated.
// If SupersedesID is set, the referenced decision is marked superseded
// and a DecisionSuperseded event is also published.
func (ks *KnowledgeStore) CreateDecision(ctx context.Context, params CreateDecisionParams) (*Decision, error) {
	now := nowMS()
	id := newULID()

	bodyPath := fmt.Sprintf("decisions/%s.md", id)

	if params.Title == "" {
		return nil, fmt.Errorf("decision title is required")
	}

	d := &Decision{
		ID:           id,
		CreatedAt:    now,
		UpdatedAt:    now,
		Status:       "proposed",
		Title:        params.Title,
		Context:      params.Context,
		Decision:     params.Decision,
		Alternatives: params.Alternatives,
		Consequences: params.Consequences,
		BodyPath:     bodyPath,
		WorkspaceID:  params.WorkspaceID,
		SupersedesID: params.SupersedesID,
	}

	_, err := ks.db.ExecContext(ctx, `
		INSERT INTO decisions (id, created_at, updated_at, status, title,
		                       context, decision, alternatives, consequences,
		                       body_path, workspace_id, supersedes_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.CreatedAt, d.UpdatedAt, d.Status, d.Title,
		d.Context, d.Decision, d.Alternatives, d.Consequences,
		d.BodyPath, d.WorkspaceID, d.SupersedesID,
	)
	if err != nil {
		return nil, fmt.Errorf("insert decision: %w", err)
	}

	// Publish DecisionCreated.
	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventDecisionCreated, events.DecisionCreatedPayload{
			ID:           d.ID,
			Title:        d.Title,
			Status:       d.Status,
			WorkspaceID:  coalesceStr(d.WorkspaceID),
			SupersedesID: coalesceStr(d.SupersedesID),
			CreatedAt:    d.CreatedAt,
		})
	}

	// If this decision supersedes another, mark the old one.
	if params.SupersedesID != nil && *params.SupersedesID != "" {
		if err := ks.supersede(ctx, *params.SupersedesID, id, now); err != nil {
			return nil, fmt.Errorf("supersede previous decision: %w", err)
		}
	}

	return d, nil
}

// GetDecision retrieves a single decision by ID.
func (ks *KnowledgeStore) GetDecision(ctx context.Context, id string) (*Decision, error) {
	d := &Decision{}
	var workspaceID, supersedesID sql.NullString

	err := ks.db.QueryRowContext(ctx, `
		SELECT id, created_at, updated_at, status, title,
		       context, decision, alternatives, consequences,
		       body_path, workspace_id, supersedes_id
		FROM decisions WHERE id = ?`, id,
	).Scan(
		&d.ID, &d.CreatedAt, &d.UpdatedAt, &d.Status, &d.Title,
		&d.Context, &d.Decision, &d.Alternatives, &d.Consequences,
		&d.BodyPath, &workspaceID, &supersedesID,
	)
	if err == sql.ErrNoRows {
		return nil, ErrDecisionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get decision: %w", err)
	}

	if workspaceID.Valid {
		d.WorkspaceID = &workspaceID.String
	}
	if supersedesID.Valid {
		d.SupersedesID = &supersedesID.String
	}

	return d, nil
}

// ListDecisions returns decisions matching the given filter, ordered by
// created_at descending. Default limit is 20 unless filter.All is true
// or filter.Limit is set.
func (ks *KnowledgeStore) ListDecisions(ctx context.Context, filter DecisionFilter) ([]Decision, error) {
	if filter.Limit <= 0 {
		filter.Limit = 20
	}

	var conditions []string
	var args []any

	if filter.WorkspaceID != nil && *filter.WorkspaceID != "" {
		conditions = append(conditions, "workspace_id = ?")
		args = append(args, *filter.WorkspaceID)
	}
	if filter.Status != nil && *filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, *filter.Status)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT id, created_at, updated_at, status, title,
		       context, decision, alternatives, consequences,
		       body_path, workspace_id, supersedes_id
		FROM decisions %s
		ORDER BY created_at DESC`, where)

	if !filter.All {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	rows, err := ks.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list decisions: %w", err)
	}
	defer rows.Close()

	return scanDecisions(rows)
}

// ListAllDecisions returns all decisions with no limit.
func (ks *KnowledgeStore) ListAllDecisions(ctx context.Context) ([]Decision, error) {
	return ks.ListDecisions(ctx, DecisionFilter{All: true})
}

// LinkDecision attaches a link (commit, file, or workspace) to a decision.
// Publishes a DecisionLinked event.
func (ks *KnowledgeStore) LinkDecision(ctx context.Context, params LinkDecisionParams) error {
	// Validate link type.
	switch params.LinkType {
	case "commit", "file", "workspace", "branch":
	default:
		return ErrInvalidLinkType
	}

	// Verify decision exists.
	var count int
	if err := ks.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM decisions WHERE id = ?", params.DecisionID,
	).Scan(&count); err != nil {
		return fmt.Errorf("check decision: %w", err)
	}
	if count == 0 {
		return ErrDecisionNotFound
	}

	now := nowMS()
	id := newULID()

	_, err := ks.db.ExecContext(ctx, `
		INSERT INTO decision_links (id, decision_id, link_type, target,
		                            line_start, line_end, branch, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, params.DecisionID, params.LinkType, params.Target,
		params.LineStart, params.LineEnd, params.Branch, now,
	)
	if err != nil {
		// SQLite constraint violation — likely a duplicate or FK issue.
		return fmt.Errorf("insert decision link: %w", err)
	}

	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventDecisionLinked, events.DecisionLinkedPayload{
			DecisionID: params.DecisionID,
			LinkType:   params.LinkType,
			Target:     params.Target,
			CreatedAt:  now,
		})
	}

	return nil
}

// GetDecisionLinks returns all links for a decision.
func (ks *KnowledgeStore) GetDecisionLinks(ctx context.Context, decisionID string) ([]DecisionLink, error) {
	rows, err := ks.db.QueryContext(ctx, `
		SELECT id, decision_id, link_type, target,
		       line_start, line_end, COALESCE(branch, ''), created_at
		FROM decision_links
		WHERE decision_id = ?
		ORDER BY created_at ASC`, decisionID,
	)
	if err != nil {
		return nil, fmt.Errorf("get decision links: %w", err)
	}
	defer rows.Close()

	var links []DecisionLink
	for rows.Next() {
		var l DecisionLink
		if err := rows.Scan(&l.ID, &l.DecisionID, &l.LinkType, &l.Target,
			&l.LineStart, &l.LineEnd, &l.Branch, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan decision link: %w", err)
		}
		links = append(links, l)
	}
	return links, rows.Err()
}

// SupersedeDecision marks oldID as superseded by newID. Both decisions
// must exist. Publishes a DecisionSuperseded event.
func (ks *KnowledgeStore) SupersedeDecision(ctx context.Context, oldID, newID string) error {
	// Verify both decisions exist.
	var count int
	if err := ks.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM decisions WHERE id = ?", oldID,
	).Scan(&count); err != nil {
		return fmt.Errorf("check old decision: %w", err)
	}
	if count == 0 {
		return ErrDecisionNotFound
	}
	if err := ks.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM decisions WHERE id = ?", newID,
	).Scan(&count); err != nil {
		return fmt.Errorf("check new decision: %w", err)
	}
	if count == 0 {
		return ErrDecisionNotFound
	}

	now := nowMS()
	return ks.supersede(ctx, oldID, newID, now)
}

// supersede performs the actual status update. Called by both CreateDecision
// (when supersedes_id is set) and SupersedeDecision (explicit command).
func (ks *KnowledgeStore) supersede(ctx context.Context, oldID, newID string, now int64) error {
	result, err := ks.db.ExecContext(ctx, `
		UPDATE decisions
		SET status = 'superseded', updated_at = ?
		WHERE id = ? AND status != 'superseded'`,
		now, oldID,
	)
	if err != nil {
		return fmt.Errorf("update old decision status: %w", err)
	}
	rows, _ := result.RowsAffected()

	// Set the supersedes_id on the new decision if not already set.
	if newID != "" {
		_, _ = ks.db.ExecContext(ctx, `
			UPDATE decisions SET supersedes_id = ?, updated_at = ?
			WHERE id = ? AND supersedes_id IS NULL`,
			oldID, now, newID,
		)
	}

	if ks.bus != nil && rows > 0 {
		_ = ks.bus.Publish(ctx, events.EventDecisionSuperseded, events.DecisionSupersededPayload{
			ID:           oldID,
			NewID:        newID,
			OldStatus:    "proposed", // best-effort; we don't fetch the old value here
			SupersededAt: now,
		})
	}

	return nil
}

// UpdateDecisionParams holds the fields that can be updated on an existing decision.
type UpdateDecisionParams struct {
	Title        *string // nil = don't update
	Context      *string
	Decision     *string
	Alternatives *string
	Consequences *string
	WorkspaceID  *string // empty string = clear, nil = don't update
}

// UpdateDecision updates the mutable body fields of an existing decision.
// Only non-nil fields are applied. Publishes a DecisionUpdated event.
func (ks *KnowledgeStore) UpdateDecision(ctx context.Context, id string, params UpdateDecisionParams) (*Decision, error) {
	// Fetch existing first so we can compute the diff and return the updated row.
	current, err := ks.GetDecision(ctx, id)
	if err != nil {
		return nil, err // ErrDecisionNotFound or wrapped
	}

	now := nowMS()
	sets := []string{"updated_at = ?"}
	args := []any{now}

	addField := func(ptr *string, column string) {
		if ptr != nil {
			sets = append(sets, fmt.Sprintf("%s = ?", column))
			args = append(args, *ptr)
		}
	}

	addField(params.Title, "title")
	addField(params.Context, "context")
	addField(params.Decision, "decision")
	addField(params.Alternatives, "alternatives")
	addField(params.Consequences, "consequences")

	if params.WorkspaceID != nil {
		sets = append(sets, "workspace_id = ?")
		if *params.WorkspaceID == "" {
			args = append(args, nil)
		} else {
			args = append(args, *params.WorkspaceID)
		}
	}

	if len(sets) == 1 {
		// Only updated_at was set — no fields to change.
		return current, nil
	}

	args = append(args, id)
	query := fmt.Sprintf(`UPDATE decisions SET %s WHERE id = ?`, strings.Join(sets, ", "))

	if _, err := ks.db.ExecContext(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("update decision: %w", err)
	}

	// Re-fetch to return the updated row.
	updated, err := ks.GetDecision(ctx, id)
	if err != nil {
		return nil, err
	}

	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventDecisionUpdated, events.DecisionUpdatedPayload{
			ID:             updated.ID,
			Title:          updated.Title,
			Status:         updated.Status,
			PreviousStatus: current.Status,
			WorkspaceID:    coalesceStr(updated.WorkspaceID),
			UpdatedAt:      now,
		})
	}

	return updated, nil
}

// DeleteDecision hard-deletes a decision and its cascade (links,
// onboarding items). Publishes a DecisionDeleted event.
func (ks *KnowledgeStore) DeleteDecision(ctx context.Context, id string) error {
	// Fetch first so we can publish the event with the decision's details.
	d, err := ks.GetDecision(ctx, id)
	if err != nil {
		return err // ErrDecisionNotFound or wrapped
	}

	now := nowMS()

	result, err := ks.db.ExecContext(ctx, `DELETE FROM decisions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete decision: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrDecisionNotFound
	}

	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventDecisionDeleted, events.DecisionDeletedPayload{
			ID:          d.ID,
			Title:       d.Title,
			Status:      d.Status,
			WorkspaceID: coalesceStr(d.WorkspaceID),
			DeletedAt:   now,
		})
	}

	return nil
}

// ── Note CRUD ───────────────────────────────────────────────────────

// CreateNote inserts a new note and publishes NoteAdded.
func (ks *KnowledgeStore) CreateNote(ctx context.Context, params CreateNoteParams) (*Note, error) {
	if strings.TrimSpace(params.Message) == "" {
		return nil, ErrEmptyMessage
	}

	now := nowMS()
	id := newULID()

	n := &Note{
		ID:          id,
		CreatedAt:   now,
		UpdatedAt:   now,
		Message:     params.Message,
		WorkspaceID: params.WorkspaceID,
		Branch:      params.Branch,
		CommitHash:  params.CommitHash,
	}

	_, err := ks.db.ExecContext(ctx, `
		INSERT INTO notes (id, created_at, updated_at, message,
		                   workspace_id, branch, commit_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.CreatedAt, n.UpdatedAt, n.Message,
		n.WorkspaceID, n.Branch, n.CommitHash,
	)
	if err != nil {
		return nil, fmt.Errorf("insert note: %w", err)
	}

	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventNoteAdded, events.NoteAddedPayload{
			ID:          n.ID,
			WorkspaceID: coalesceStr(n.WorkspaceID),
			Branch:      n.Branch,
			CommitHash:  n.CommitHash,
			CreatedAt:   n.CreatedAt,
		})
	}

	return n, nil
}

// GetNote retrieves a single note by ID.
func (ks *KnowledgeStore) GetNote(ctx context.Context, id string) (*Note, error) {
	n := &Note{}
	var workspaceID sql.NullString

	err := ks.db.QueryRowContext(ctx, `
		SELECT id, created_at, updated_at, message,
		       workspace_id, COALESCE(branch, ''), COALESCE(commit_hash, '')
		FROM notes WHERE id = ?`, id,
	).Scan(&n.ID, &n.CreatedAt, &n.UpdatedAt, &n.Message,
		&workspaceID, &n.Branch, &n.CommitHash)
	if err == sql.ErrNoRows {
		return nil, ErrNoteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get note: %w", err)
	}

	if workspaceID.Valid {
		n.WorkspaceID = &workspaceID.String
	}

	return n, nil
}

// DeleteNote hard-deletes a note from the database. Publishes a
// NoteDeleted event.
func (ks *KnowledgeStore) DeleteNote(ctx context.Context, id string) error {
	// Fetch first so we can publish the event with the note's details.
	n, err := ks.GetNote(ctx, id)
	if err != nil {
		return err // ErrNoteNotFound or wrapped
	}

	now := nowMS()

	result, err := ks.db.ExecContext(ctx, `DELETE FROM notes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete note: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNoteNotFound
	}

	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventNoteDeleted, events.NoteDeletedPayload{
			ID:          n.ID,
			Message:     n.Message,
			WorkspaceID: coalesceStr(n.WorkspaceID),
			Branch:      n.Branch,
			DeletedAt:   now,
		})
	}

	return nil
}

// UpdateNoteCommitHash sets the commit_hash field on an existing note
// without changing its ID or any other fields. Returns ErrNoteNotFound
// if the note does not exist.
func (ks *KnowledgeStore) UpdateNoteCommitHash(ctx context.Context, id, commitHash string) error {
	result, err := ks.db.ExecContext(ctx, `
		UPDATE notes SET commit_hash = ?, updated_at = ? WHERE id = ?`,
		commitHash, nowMS(), id)
	if err != nil {
		return fmt.Errorf("update note commit_hash: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNoteNotFound
	}
	return nil
}

// ListNotes returns notes matching the given filter, ordered by
// created_at descending. Default limit is 20.
func (ks *KnowledgeStore) ListNotes(ctx context.Context, filter NoteFilter) ([]Note, error) {
	if filter.Limit <= 0 {
		filter.Limit = 20
	}

	var conditions []string
	var args []any

	if filter.WorkspaceID != nil && *filter.WorkspaceID != "" {
		conditions = append(conditions, "workspace_id = ?")
		args = append(args, *filter.WorkspaceID)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT id, created_at, updated_at, message,
		       workspace_id, COALESCE(branch, ''), COALESCE(commit_hash, '')
		FROM notes %s
		ORDER BY created_at DESC`, where)

	if !filter.All {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	rows, err := ks.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	defer rows.Close()

	var notes []Note
	for rows.Next() {
		var n Note
		var workspaceID sql.NullString
		if err := rows.Scan(&n.ID, &n.CreatedAt, &n.UpdatedAt, &n.Message,
			&workspaceID, &n.Branch, &n.CommitHash); err != nil {
			return nil, fmt.Errorf("scan note: %w", err)
		}
		if workspaceID.Valid {
			n.WorkspaceID = &workspaceID.String
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// ── Search ──────────────────────────────────────────────────────────

// Search performs a full-text LIKE search across decisions (title,
// context, decision, alternatives, consequences) and notes (message).
// Returns results ordered by relevance (score) then recency.
func (ks *KnowledgeStore) Search(ctx context.Context, params SearchParams) ([]SearchResult, error) {
	q := strings.TrimSpace(params.Query)
	if q == "" {
		return []SearchResult{}, nil
	}

	if params.Limit <= 0 {
		params.Limit = 20
	}

	pattern := "%" + q + "%"

	var conditions []string
	var args []any	// ── Decisions sub-query ──────────────────────────────────────
		if params.Type == nil || *params.Type == "decision" {
			decScore := `(CASE WHEN title LIKE ? THEN 1 ELSE 0 END +
			              CASE WHEN context LIKE ? THEN 1 ELSE 0 END +
			              CASE WHEN decision LIKE ? THEN 1 ELSE 0 END +
			              CASE WHEN alternatives LIKE ? THEN 1 ELSE 0 END +
			              CASE WHEN consequences LIKE ? THEN 1 ELSE 0 END)`
			decWhereCond := `(title LIKE ? OR context LIKE ? OR decision LIKE ? OR alternatives LIKE ? OR consequences LIKE ?)`

			decWhere := "WHERE " + decWhereCond
			// Score `?`s first, then WHERE `?`s — args must match this order.
			decScoreArgs := []any{pattern, pattern, pattern, pattern, pattern}
			decWhereArgs := []any{pattern, pattern, pattern, pattern, pattern}

			if params.WorkspaceID != nil && *params.WorkspaceID != "" {
				decWhere += " AND workspace_id = ?"
				decWhereArgs = append(decWhereArgs, *params.WorkspaceID)
			}

			conditions = append(conditions, fmt.Sprintf(`
SELECT 'decision' AS type, id, title, status, workspace_id, created_at, %s AS score
FROM decisions %s`, decScore, decWhere))
			args = append(args, decScoreArgs...)  // score ?s first
			args = append(args, decWhereArgs...)  // then WHERE ?s
		}

		// ── Notes sub-query ──────────────────────────────────────────
		if params.Type == nil || *params.Type == "note" {
			noteScore := `CASE WHEN message LIKE ? THEN 1 ELSE 0 END`
			noteCond := `message LIKE ?`

			noteWhere := "WHERE " + noteCond
			noteScoreArgs := []any{pattern}
			noteWhereArgs := []any{pattern}

			if params.WorkspaceID != nil && *params.WorkspaceID != "" {
				noteWhere += " AND workspace_id = ?"
				noteWhereArgs = append(noteWhereArgs, *params.WorkspaceID)
			}

			conditions = append(conditions, fmt.Sprintf(`
SELECT 'note' AS type, id, message AS title, '' AS status, workspace_id, created_at, %s AS score
FROM notes %s`, noteScore, noteWhere))
			args = append(args, noteScoreArgs...)  // score ?s first
			args = append(args, noteWhereArgs...)  // then WHERE ?s
	}

	if len(conditions) == 0 {
		return []SearchResult{}, nil
	}

	query := strings.Join(conditions, " UNION ALL ")
	query += " ORDER BY score DESC, created_at DESC"
	query += fmt.Sprintf(" LIMIT %d", params.Limit)

	rows, err := ks.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var workspaceID sql.NullString
		if err := rows.Scan(&r.Type, &r.ID, &r.Title, &r.Status,
			&workspaceID, &r.CreatedAt, &r.Score); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		if workspaceID.Valid {
			r.WorkspaceID = &workspaceID.String
		}
		results = append(results, r)
	}

	return results, rows.Err()
}

// ── Onboarding CRUD ────────────────────────────────────────────────

// StartOnboarding creates a new onboarding session (or resumes an active
// one for the same participant). Publishes OnboardingStarted.
//
// In v0.4 this creates the session record and scans existing decisions
// and notes as onboarding items. File scanning is deferred to the CLI
// layer which has access to the Git adapter.
func (ks *KnowledgeStore) StartOnboarding(ctx context.Context, participant string) (*OnboardingSession, error) {
	// Check for existing active session.
	existing := &OnboardingSession{}
	err := ks.db.QueryRowContext(ctx, `
		SELECT id, created_at, updated_at, participant, status
		FROM onboarding_sessions
		WHERE participant = ? AND status = 'active'
		ORDER BY created_at DESC LIMIT 1`, participant,
	).Scan(&existing.ID, &existing.CreatedAt, &existing.UpdatedAt,
		&existing.Participant, &existing.Status)
	if err == nil {
		// Resume existing session.
		if ks.bus != nil {
			_ = ks.bus.Publish(ctx, events.EventOnboardingStarted, events.OnboardingStartedPayload{
				SessionID:   existing.ID,
				Participant: existing.Participant,
				ItemCount:   0, // CLI will compute actual count
				CreatedAt:   nowMS(),
			})
		}
		return existing, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("check existing session: %w", err)
	}

	// Create new session.
	now := nowMS()
	id := newULID()

	s := &OnboardingSession{
		ID:          id,
		CreatedAt:   now,
		UpdatedAt:   now,
		Participant: participant,
		Status:      "active",
	}

	_, err = ks.db.ExecContext(ctx, `
		INSERT INTO onboarding_sessions (id, created_at, updated_at, participant, status)
		VALUES (?, ?, ?, ?, ?)`,
		s.ID, s.CreatedAt, s.UpdatedAt, s.Participant, s.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("insert onboarding session: %w", err)
	}

	// Seed the session with existing decisions (excluding rejected).
	seedRows, err := ks.db.QueryContext(ctx, `
		SELECT id FROM decisions WHERE status != 'rejected'`)
	if err != nil {
		return nil, fmt.Errorf("seed decisions: %w", err)
	}
	defer seedRows.Close()

	for seedRows.Next() {
		var decID string
		if err := seedRows.Scan(&decID); err != nil {
			return nil, fmt.Errorf("scan decision ID: %w", err)
		}
		itemID := newULID()
		_, _ = ks.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO onboarding_items (id, session_id, item_type, item_target)
			VALUES (?, ?, 'decision', ?)`, itemID, s.ID, decID)
	}

	// Seed with existing notes.
	seedRows2, err := ks.db.QueryContext(ctx, `SELECT id FROM notes`)
	if err != nil {
		return nil, fmt.Errorf("seed notes: %w", err)
	}
	defer seedRows2.Close()

	for seedRows2.Next() {
		var noteID string
		if err := seedRows2.Scan(&noteID); err != nil {
			return nil, fmt.Errorf("scan note ID: %w", err)
		}
		itemID := newULID()
		_, _ = ks.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO onboarding_items (id, session_id, item_type, item_target)
			VALUES (?, ?, 'note', ?)`, itemID, s.ID, noteID)
	}

	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventOnboardingStarted, events.OnboardingStartedPayload{
			SessionID:   s.ID,
			Participant: s.Participant,
			ItemCount:   0,
			CreatedAt:   now,
		})
	}

	return s, nil
}

// GetOnboardingSession retrieves a session by ID.
func (ks *KnowledgeStore) GetOnboardingSession(ctx context.Context, sessionID string) (*OnboardingSession, error) {
	s := &OnboardingSession{}
	err := ks.db.QueryRowContext(ctx, `
		SELECT id, created_at, updated_at, participant, status
		FROM onboarding_sessions WHERE id = ?`, sessionID,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt, &s.Participant, &s.Status)
	if err == sql.ErrNoRows {
		return nil, ErrOnboardingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get onboarding session: %w", err)
	}
	return s, nil
}

// MarkOnboardingItem marks an item as covered. Publishes OnboardingItemCovered.
func (ks *KnowledgeStore) MarkOnboardingItem(ctx context.Context, sessionID, itemType, itemTarget string) error {
	return ks.setItemCovered(ctx, sessionID, itemType, itemTarget, true)
}

// SkipOnboardingItem marks an item as explicitly skipped.
func (ks *KnowledgeStore) SkipOnboardingItem(ctx context.Context, sessionID, itemType, itemTarget string) error {
	return ks.setItemCovered(ctx, sessionID, itemType, itemTarget, false)
}

func (ks *KnowledgeStore) setItemCovered(ctx context.Context, sessionID, itemType, itemTarget string, covered bool) error {
	now := nowMS()

	var err error
	if covered {
		_, err = ks.db.ExecContext(ctx, `
			UPDATE onboarding_items
			SET covered_at = ?, skipped = 0
			WHERE session_id = ? AND item_type = ? AND item_target = ?`,
			now, sessionID, itemType, itemTarget)
	} else {
		_, err = ks.db.ExecContext(ctx, `
			UPDATE onboarding_items
			SET skipped = 1, covered_at = NULL
			WHERE session_id = ? AND item_type = ? AND item_target = ?`,
			sessionID, itemType, itemTarget)
	}
	if err != nil {
		return fmt.Errorf("update onboarding item: %w", err)
	}

	if ks.bus != nil && covered {
		_ = ks.bus.Publish(ctx, events.EventOnboardingItemCovered, events.OnboardingItemCoveredPayload{
			SessionID:  sessionID,
			ItemType:   itemType,
			ItemTarget: itemTarget,
			CoveredAt:  now,
		})
	}

	return nil
}

// GetOnboardingProgress returns a structured progress report for a session.
func (ks *KnowledgeStore) GetOnboardingProgress(ctx context.Context, sessionID string) (*OnboardingProgress, error) {
	session, err := ks.GetOnboardingSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	rows, err := ks.db.QueryContext(ctx, `
		SELECT item_type,
		       COUNT(*) AS total,
		       SUM(CASE WHEN covered_at IS NOT NULL THEN 1 ELSE 0 END) AS covered,
		       SUM(skipped) AS skipped
		FROM onboarding_items
		WHERE session_id = ?
		GROUP BY item_type`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query progress: %w", err)
	}
	defer rows.Close()

	prog := &OnboardingProgress{
		Session: *session,
		ByType:  make(map[string]TypeProgress),
	}

	for rows.Next() {
		var tp TypeProgress
		var itemType string
		if err := rows.Scan(&itemType, &tp.Total, &tp.Covered, &tp.Skipped); err != nil {
			return nil, fmt.Errorf("scan progress row: %w", err)
		}
		tp.Remaining = tp.Total - tp.Covered - tp.Skipped
		prog.ByType[itemType] = tp
		prog.TotalItems += tp.Total
		prog.Covered += tp.Covered
		prog.Skipped += tp.Skipped
	}
	prog.Remaining = prog.TotalItems - prog.Covered - prog.Skipped

	return prog, rows.Err()
}

// ListUnwatchedItems returns all onboarding items for a session that are
// not covered and not skipped.
func (ks *KnowledgeStore) ListUnwatchedItems(ctx context.Context, sessionID string) ([]OnboardingItem, error) {
	rows, err := ks.db.QueryContext(ctx, `
		SELECT id, session_id, item_type, item_target, covered_at, skipped
		FROM onboarding_items
		WHERE session_id = ? AND covered_at IS NULL AND skipped = 0
		ORDER BY item_type, id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list unwatched items: %w", err)
	}
	defer rows.Close()

	return scanOnboardingItems(rows)
}

// CompleteOnboarding marks a session as completed. Publishes OnboardingCompleted.
func (ks *KnowledgeStore) CompleteOnboarding(ctx context.Context, sessionID string) error {
	session, err := ks.GetOnboardingSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.Status == "completed" {
		return ErrSessionAlreadyComplete
	}

	now := nowMS()
	_, err = ks.db.ExecContext(ctx, `
		UPDATE onboarding_sessions SET status = 'completed', updated_at = ?
		WHERE id = ?`, now, sessionID)
	if err != nil {
		return fmt.Errorf("complete session: %w", err)
	}

	if ks.bus != nil {
		prog, _ := ks.GetOnboardingProgress(ctx, sessionID)
		total := 0
		if prog != nil {
			total = prog.TotalItems
		}
		_ = ks.bus.Publish(ctx, events.EventOnboardingCompleted, events.OnboardingCompletedPayload{
			SessionID:   sessionID,
			Participant: session.Participant,
			TotalItems:  total,
			CompletedAt: now,
		})
	}

	return nil
}

// ── Workspace CRUD ────────────────────────────────────────────────

// CreateWorkspace inserts a new workspace and publishes WorkspaceCreated.
func (ks *KnowledgeStore) CreateWorkspace(ctx context.Context, params CreateWorkspaceParams) (*Workspace, error) {
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return nil, fmt.Errorf("workspace name is required")
	}

	now := nowMS()
	id := newULID()

	// Encoded tags as JSON array.
	tagsJSON := "[]"
	if len(params.Tags) > 0 {
		tj, err := json.Marshal(params.Tags)
		if err != nil {
			return nil, fmt.Errorf("marshal tags: %w", err)
		}
		tagsJSON = string(tj)
	}

	w := &Workspace{
		ID:          id,
		Name:        name,
		Description: params.Description,
		Status:      "active",
		Tags:        params.Tags,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err := ks.db.ExecContext(ctx, `
		INSERT INTO workspaces (id, name, description, status, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.Description, w.Status, tagsJSON, w.CreatedAt, w.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return nil, ErrDuplicateWorkspace
		}
		return nil, fmt.Errorf("insert workspace: %w", err)
	}

	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventWorkspaceCreated, events.WorkspaceCreatedPayload{
			ID:          w.ID,
			Name:        w.Name,
			Description: w.Description,
			Tags:        w.Tags,
			CreatedAt:   w.CreatedAt,
		})
	}

	return w, nil
}

// GetWorkspace retrieves a workspace by name (the user-facing identifier).
func (ks *KnowledgeStore) GetWorkspace(ctx context.Context, name string) (*Workspace, error) {
	return ks.getWorkspaceBy(ctx, "name", name)
}

// GetWorkspaceByID retrieves a workspace by ULID.
func (ks *KnowledgeStore) GetWorkspaceByID(ctx context.Context, id string) (*Workspace, error) {
	return ks.getWorkspaceBy(ctx, "id", id)
}

func (ks *KnowledgeStore) getWorkspaceBy(ctx context.Context, column, value string) (*Workspace, error) {
	w := &Workspace{}
	var tagsJSON string

	query := fmt.Sprintf(`
		SELECT id, name, description, status, tags, created_at, updated_at, COALESCE(last_commit_sha, '')
		FROM workspaces WHERE %s = ?`, column)

	err := ks.db.QueryRowContext(ctx, query, value).Scan(
		&w.ID, &w.Name, &w.Description, &w.Status, &tagsJSON, &w.CreatedAt, &w.UpdatedAt, &w.LastCommitSHA,
	)
	if err == sql.ErrNoRows {
		return nil, ErrWorkspaceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}

	// Parse tags JSON.
	if tagsJSON != "" && tagsJSON != "[]" {
		_ = json.Unmarshal([]byte(tagsJSON), &w.Tags)
	}

	return w, nil
}

// ListWorkspaces returns all workspaces, ordered by name.
func (ks *KnowledgeStore) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	rows, err := ks.db.QueryContext(ctx, `
		SELECT id, name, description, status, tags, created_at, updated_at, COALESCE(last_commit_sha, '')
		FROM workspaces
		ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()

	return scanWorkspaces(rows)
}

// UpdateWorkspace updates mutable fields of a workspace. Publishes WorkspaceUpdated.
func (ks *KnowledgeStore) UpdateWorkspace(ctx context.Context, name string, params UpdateWorkspaceParams) (*Workspace, error) {
	current, err := ks.GetWorkspace(ctx, name)
	if err != nil {
		return nil, err
	}

	now := nowMS()
	sets := []string{"updated_at = ?"}
	args := []any{now}

	if params.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *params.Name)
	}
	if params.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *params.Description)
	}
	if params.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *params.Status)
	}
	if params.Tags != nil {
		tagsJSON := "[]"
		if len(params.Tags) > 0 {
			tj, _ := json.Marshal(params.Tags)
			tagsJSON = string(tj)
		}
		sets = append(sets, "tags = ?")
		args = append(args, tagsJSON)
	}

	if len(sets) == 1 {
		return current, nil
	}

	args = append(args, current.ID)
	query := fmt.Sprintf(`UPDATE workspaces SET %s WHERE id = ?`, strings.Join(sets, ", "))

	if _, err := ks.db.ExecContext(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("update workspace: %w", err)
	}

	updated, err := ks.GetWorkspaceByID(ctx, current.ID)
	if err != nil {
		return nil, err
	}

	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventWorkspaceUpdated, events.WorkspaceUpdatedPayload{
			ID:          updated.ID,
			Name:        updated.Name,
			Description: updated.Description,
			Tags:        updated.Tags,
			UpdatedAt:   now,
		})
	}

	return updated, nil
}

// DeleteWorkspace hard-deletes a workspace and its cascade (files, branches,
// decisions, notes). Does NOT delete the decisions/notes themselves — they
// are handled via ON DELETE SET NULL on workspace_id in those tables, but
// since we use a string reference (not FK), we clear them explicitly.
func (ks *KnowledgeStore) DeleteWorkspace(ctx context.Context, name string) (*Workspace, error) {
	w, err := ks.GetWorkspace(ctx, name)
	if err != nil {
		return nil, err
	}

	now := nowMS()

	// Clear workspace_id on decisions and notes linked to this workspace.
	_, _ = ks.db.ExecContext(ctx, `UPDATE decisions SET workspace_id = NULL WHERE workspace_id = ?`, w.Name)
	_, _ = ks.db.ExecContext(ctx, `UPDATE notes SET workspace_id = NULL WHERE workspace_id = ?`, w.Name)

	// Delete workspace (cascades to workspace_files, workspace_branches).
	result, err := ks.db.ExecContext(ctx, `DELETE FROM workspaces WHERE id = ?`, w.ID)
	if err != nil {
		return nil, fmt.Errorf("delete workspace: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, ErrWorkspaceNotFound
	}

	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventWorkspaceDeleted, events.WorkspaceDeletedPayload{
			ID:        w.ID,
			Name:      w.Name,
			ItemCount: 0,
			DeletedAt: now,
		})
	}

	return w, nil
}

// ── Workspace files ─────────────────────────────────────────────────

// AddWorkspaceFile adds a file path to a workspace. Publishes WorkspaceItemAdded.
func (ks *KnowledgeStore) AddWorkspaceFile(ctx context.Context, workspaceName, filePath string) (*WorkspaceFile, error) {
	w, err := ks.GetWorkspace(ctx, workspaceName)
	if err != nil {
		return nil, err
	}

	now := nowMS()
	id := newULID()

	f := &WorkspaceFile{
		ID:          id,
		WorkspaceID: w.ID,
		Path:        filePath,
		CreatedAt:   now,
	}

	_, err = ks.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO workspace_files (id, workspace_id, path, created_at)
		VALUES (?, ?, ?, ?)`, f.ID, f.WorkspaceID, f.Path, f.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("add workspace file: %w", err)
	}

	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventWorkspaceItemAdded, events.WorkspaceItemAddedPayload{
			WorkspaceID: w.ID,
			ItemType:    "file",
			ItemTarget:  filePath,
			CreatedAt:   now,
		})
	}

	return f, nil
}

// RemoveWorkspaceFile removes a file path from a workspace. Publishes WorkspaceItemRemoved.
func (ks *KnowledgeStore) RemoveWorkspaceFile(ctx context.Context, workspaceName, filePath string) error {
	w, err := ks.GetWorkspace(ctx, workspaceName)
	if err != nil {
		return err
	}

	result, err := ks.db.ExecContext(ctx, `
		DELETE FROM workspace_files WHERE workspace_id = ? AND path = ?`, w.ID, filePath)
	if err != nil {
		return fmt.Errorf("remove workspace file: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("file %q not found in workspace %q", filePath, workspaceName)
	}

	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventWorkspaceItemRemoved, events.WorkspaceItemRemovedPayload{
			WorkspaceID: w.ID,
			ItemType:    "file",
			ItemTarget:  filePath,
			RemovedAt:   nowMS(),
		})
	}

	return nil
}

// ListWorkspaceFiles returns all files tracked in a workspace.
func (ks *KnowledgeStore) ListWorkspaceFiles(ctx context.Context, workspaceName string) ([]WorkspaceFile, error) {
	w, err := ks.GetWorkspace(ctx, workspaceName)
	if err != nil {
		return nil, err
	}

	rows, err := ks.db.QueryContext(ctx, `
		SELECT id, workspace_id, path, created_at
		FROM workspace_files WHERE workspace_id = ?
		ORDER BY path ASC`, w.ID)
	if err != nil {
		return nil, fmt.Errorf("list workspace files: %w", err)
	}
	defer rows.Close()

	return scanWorkspaceFiles(rows)
}

// ── Workspace branches ──────────────────────────────────────────────

// AddWorkspaceBranch adds a branch name to a workspace. Publishes WorkspaceItemAdded.
func (ks *KnowledgeStore) AddWorkspaceBranch(ctx context.Context, workspaceName, branchName string) (*WorkspaceBranch, error) {
	w, err := ks.GetWorkspace(ctx, workspaceName)
	if err != nil {
		return nil, err
	}

	now := nowMS()
	id := newULID()

	b := &WorkspaceBranch{
		ID:          id,
		WorkspaceID: w.ID,
		BranchName:  branchName,
		CreatedAt:   now,
	}

	_, err = ks.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO workspace_branches (id, workspace_id, branch_name, created_at)
		VALUES (?, ?, ?, ?)`, b.ID, b.WorkspaceID, b.BranchName, b.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("add workspace branch: %w", err)
	}

	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventWorkspaceItemAdded, events.WorkspaceItemAddedPayload{
			WorkspaceID: w.ID,
			ItemType:    "branch",
			ItemTarget:  branchName,
			CreatedAt:   now,
		})
	}

	return b, nil
}

// RemoveWorkspaceBranch removes a branch from a workspace. Publishes WorkspaceItemRemoved.
func (ks *KnowledgeStore) RemoveWorkspaceBranch(ctx context.Context, workspaceName, branchName string) error {
	w, err := ks.GetWorkspace(ctx, workspaceName)
	if err != nil {
		return err
	}

	result, err := ks.db.ExecContext(ctx, `
		DELETE FROM workspace_branches WHERE workspace_id = ? AND branch_name = ?`, w.ID, branchName)
	if err != nil {
		return fmt.Errorf("remove workspace branch: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("branch %q not found in workspace %q", branchName, workspaceName)
	}

	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventWorkspaceItemRemoved, events.WorkspaceItemRemovedPayload{
			WorkspaceID: w.ID,
			ItemType:    "branch",
			ItemTarget:  branchName,
			RemovedAt:   nowMS(),
		})
	}

	return nil
}

// ListWorkspaceBranches returns all branches tracked in a workspace.
func (ks *KnowledgeStore) ListWorkspaceBranches(ctx context.Context, workspaceName string) ([]WorkspaceBranch, error) {
	w, err := ks.GetWorkspace(ctx, workspaceName)
	if err != nil {
		return nil, err
	}

	rows, err := ks.db.QueryContext(ctx, `
		SELECT id, workspace_id, branch_name, created_at
		FROM workspace_branches WHERE workspace_id = ?
		ORDER BY branch_name ASC`, w.ID)
	if err != nil {
		return nil, fmt.Errorf("list workspace branches: %w", err)
	}
	defer rows.Close()

	return scanWorkspaceBranches(rows)
}

// ── Workspace commits ────────────────────────────────────────────

// AddWorkspaceCommit records a commit as activity in a workspace. Also
// updates the workspace's last_commit_sha for fast access. Idempotent
// (UNIQUE constraint on workspace_id + commit_sha).
func (ks *KnowledgeStore) AddWorkspaceCommit(ctx context.Context, params AddWorkspaceCommitParams) (*WorkspaceCommit, error) {
	w, err := ks.GetWorkspace(ctx, params.WorkspaceName)
	if err != nil {
		return nil, err
	}

	now := nowMS()
	id := newULID()

	c := &WorkspaceCommit{
		ID:          id,
		WorkspaceID: w.ID,
		CommitSHA:   params.CommitSHA,
		BranchName:  params.BranchName,
		Message:     params.Message,
		CreatedAt:   now,
	}

	_, err = ks.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO workspace_commits (id, workspace_id, commit_sha, branch_name, message, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.WorkspaceID, c.CommitSHA, c.BranchName, c.Message, c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("add workspace commit: %w", err)
	}

	// Update last_commit_sha on the workspace.
	_, _ = ks.db.ExecContext(ctx, `
		UPDATE workspaces SET last_commit_sha = ?, updated_at = ?
		WHERE id = ? AND (? != '' OR last_commit_sha = '')`,
		params.CommitSHA, now, w.ID, params.CommitSHA)

	return c, nil
}

// ListWorkspaceCommits returns commits recorded for a workspace, ordered
// by created_at (link time) descending.
func (ks *KnowledgeStore) ListWorkspaceCommits(ctx context.Context, workspaceName string, limit int) ([]WorkspaceCommit, error) {
	w, err := ks.GetWorkspace(ctx, workspaceName)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 20
	}

	rows, err := ks.db.QueryContext(ctx, `
		SELECT id, workspace_id, commit_sha, COALESCE(branch_name, ''), COALESCE(message, ''), created_at
		FROM workspace_commits WHERE workspace_id = ?
		ORDER BY created_at DESC
		LIMIT ?`, w.ID, limit)
	if err != nil {
		return nil, fmt.Errorf("list workspace commits: %w", err)
	}
	defer rows.Close()

	return scanWorkspaceCommits(rows)
}

// UpdateWorkspaceLastCommit updates just the last_commit_sha and updated_at
// on a workspace without recording a workspace_commit entry. Used during
// event-driven updates from the Git adapter.
func (ks *KnowledgeStore) UpdateWorkspaceLastCommit(ctx context.Context, workspaceName, commitSHA string) error {
	w, err := ks.GetWorkspace(ctx, workspaceName)
	if err != nil {
		return err
	}

	now := nowMS()
	_, err = ks.db.ExecContext(ctx, `
		UPDATE workspaces SET last_commit_sha = ?, updated_at = ? WHERE id = ?`,
		commitSHA, now, w.ID)
	if err != nil {
		return fmt.Errorf("update workspace last commit: %w", err)
	}

	return nil
}	// ── Workspace status ────────────────────────────────────────────────

// GetWorkspaceStatus returns a full summary of a workspace's contents —
// its metadata, tracked files, tracked branches, linked decisions, linked
// notes, linked pull requests, and linked issues. Also computes item
// count and last activity timestamp.
func (ks *KnowledgeStore) GetWorkspaceStatus(ctx context.Context, workspaceName string) (*WorkspaceStatus, error) {
	w, err := ks.GetWorkspace(ctx, workspaceName)
	if err != nil {
		return nil, err
	}

	files, _ := ks.ListWorkspaceFiles(ctx, workspaceName)
	branches, _ := ks.ListWorkspaceBranches(ctx, workspaceName)

	// Fetch decisions scoped to this workspace.
	decisions, _ := ks.ListDecisions(ctx, DecisionFilter{WorkspaceID: &w.Name, All: true})

	// Fetch notes scoped to this workspace.
	notes, _ := ks.ListNotes(ctx, NoteFilter{WorkspaceID: &w.Name, All: true})

	if files == nil {
		files = []WorkspaceFile{}
	}
	if branches == nil {
		branches = []WorkspaceBranch{}
	}
	if decisions == nil {
		decisions = []Decision{}
	}
	if notes == nil {
		notes = []Note{}
	}

	totalItems := len(files) + len(branches) + len(decisions) + len(notes)

	// Fetch recent commits linked to this workspace.
	commits, _ := ks.ListWorkspaceCommits(ctx, workspaceName, 10)
	if commits == nil {
		commits = []WorkspaceCommit{}
	}

	// Fetch pull requests linked to this workspace.
	prs, _ := ks.ListPullRequests(ctx, w.Name)
	if prs == nil {
		prs = []PullRequest{}
	}

	// Fetch issues linked to this workspace.
	issues, _ := ks.ListIssues(ctx, w.Name)
	if issues == nil {
		issues = []Issue{}
	}

	// Compute last activity — include workspace_commits timestamps.
	lastActivity := w.UpdatedAt
	if w.CreatedAt > lastActivity {
		lastActivity = w.CreatedAt
	}
	for _, c := range commits {
		if c.CreatedAt > lastActivity {
			lastActivity = c.CreatedAt
		}
	}
	for _, pr := range prs {
		if pr.CreatedAt > lastActivity {
			lastActivity = pr.CreatedAt
		}
	}
	for _, iss := range issues {
		if iss.CreatedAt > lastActivity {
			lastActivity = iss.CreatedAt
		}
	}

	return &WorkspaceStatus{
		Workspace:    *w,
		Files:        files,
		Branches:     branches,
		Decisions:    decisions,
		Notes:        notes,
		Commits:      commits,
		PullRequests: prs,
		Issues:       issues,
		ItemCount:    totalItems + len(commits) + len(prs) + len(issues),
		LastActivity: lastActivity,
	}, nil
}

// ── GitHub Config CRUD ──────────────────────────────────────────

// GetGitHubConfig retrieves the GitHub integration configuration.
// Returns nil if no config has been stored.
func (ks *KnowledgeStore) GetGitHubConfig(ctx context.Context) (*GitHubConfig, error) {
	cfg := &GitHubConfig{}
	err := ks.db.QueryRowContext(ctx, `
		SELECT token, COALESCE(owner, ''), COALESCE(repo, ''), COALESCE(base_branch, 'main'), updated_at
		FROM github_config WHERE id = 'default'`,
	).Scan(&cfg.Token, &cfg.Owner, &cfg.Repo, &cfg.BaseBranch, &cfg.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get github config: %w", err)
	}
	return cfg, nil
}

// SetGitHubConfig saves the GitHub integration configuration (upsert).
func (ks *KnowledgeStore) SetGitHubConfig(ctx context.Context, cfg GitHubConfig) error {
	now := nowMS()
	_, err := ks.db.ExecContext(ctx, `
		INSERT INTO github_config (id, token, owner, repo, base_branch, updated_at)
		VALUES ('default', ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			token = excluded.token,
			owner = excluded.owner,
			repo = excluded.repo,
			base_branch = excluded.base_branch,
			updated_at = excluded.updated_at`,
		cfg.Token, cfg.Owner, cfg.Repo, cfg.BaseBranch, now)
	if err != nil {
		return fmt.Errorf("set github config: %w", err)
	}
	return nil
}

// ── Pull Request CRUD ───────────────────────────────────────────────

// CreatePullRequest records a pull request in the store.
func (ks *KnowledgeStore) CreatePullRequest(ctx context.Context, params CreatePullRequestParams) (*PullRequest, error) {
	now := nowMS()
	id := newULID()

	pr := &PullRequest{
		ID:          id,
		Number:      params.Number,
		Title:       params.Title,
		State:       params.State,
		Branch:      params.Branch,
		Base:        params.Base,
		URL:         params.URL,
		WorkspaceID: params.WorkspaceID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err := ks.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO pull_requests (id, number, title, state, branch, base, url, workspace_id, merge_commit_sha, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
		pr.ID, pr.Number, pr.Title, pr.State, pr.Branch, pr.Base, pr.URL,
		nullableWorkspaceID(params.WorkspaceID), pr.CreatedAt, pr.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create pull request: %w", err)
	}

	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventPullRequestCreated, events.PullRequestCreatedPayload{
			Number:     pr.Number,
			Title:      pr.Title,
			Branch:     pr.Branch,
			Base:       pr.Base,
			URL:        pr.URL,
			CreatedAt:  now,
		})
	}

	return pr, nil
}

// ListPullRequests returns all tracked pull requests, optionally filtered by workspace.
func (ks *KnowledgeStore) ListPullRequests(ctx context.Context, workspaceID string) ([]PullRequest, error) {
	var rows *sql.Rows
	var err error
	if workspaceID != "" {
		rows, err = ks.db.QueryContext(ctx, `
			SELECT id, number, title, state, branch, base, COALESCE(url, ''), COALESCE(workspace_id, ''), COALESCE(merge_commit_sha, ''), COALESCE(merged_at, 0), created_at, updated_at
			FROM pull_requests WHERE workspace_id = ?
			ORDER BY created_at DESC`, workspaceID)
	} else {
		rows, err = ks.db.QueryContext(ctx, `
			SELECT id, number, title, state, branch, base, COALESCE(url, ''), COALESCE(workspace_id, ''), COALESCE(merge_commit_sha, ''), COALESCE(merged_at, 0), created_at, updated_at
			FROM pull_requests ORDER BY created_at DESC`)
	}
	if err != nil {
		return nil, fmt.Errorf("list pull requests: %w", err)
	}
	defer rows.Close()

	return scanPullRequests(rows)
}

// GetPullRequestByNumber retrieves a pull request by its GitHub number.
func (ks *KnowledgeStore) GetPullRequestByNumber(ctx context.Context, number int) (*PullRequest, error) {
	pr := &PullRequest{}
	err := ks.db.QueryRowContext(ctx, `
		SELECT id, number, title, state, branch, base, COALESCE(url, ''), COALESCE(workspace_id, ''), COALESCE(merge_commit_sha, ''), COALESCE(merged_at, 0), created_at, updated_at
		FROM pull_requests WHERE number = ?`, number,
	).Scan(&pr.ID, &pr.Number, &pr.Title, &pr.State, &pr.Branch, &pr.Base, &pr.URL, &pr.WorkspaceID, &pr.MergeCommitSHA, &pr.MergedAt, &pr.CreatedAt, &pr.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("pull request #%d not found", number)
	}
	if err != nil {
		return nil, fmt.Errorf("get pull request: %w", err)
	}
	return pr, nil
}	// ── PR Review CRUD ─────────────────────────────────────────────────

// CreateReview records a pull request review in the store.
func (ks *KnowledgeStore) CreateReview(ctx context.Context, params CreateReviewParams) (*PRReview, error) {
	now := nowMS()
	id := newULID()

	r := &PRReview{
		ID:          id,
		PRNumber:    params.PRNumber,
		Reviewer:    params.Reviewer,
		State:       params.State,
		Body:        params.Body,
		WorkspaceID: params.WorkspaceID,
		SubmittedAt: now,
	}

	_, err := ks.db.ExecContext(ctx, `
		INSERT INTO pr_reviews (id, pr_number, reviewer, state, body, workspace_id, submitted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.PRNumber, r.Reviewer, r.State, r.Body,
		nullableWorkspaceID(r.WorkspaceID), r.SubmittedAt)
	if err != nil {
		return nil, fmt.Errorf("create review: %w", err)
	}

	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventPullRequestReviewed, events.PullRequestReviewedPayload{
			PRNumber:    r.PRNumber,
			Reviewer:    r.Reviewer,
			State:       r.State,
			Body:        r.Body,
			SubmittedAt: now,
		})
	}

	return r, nil
}

// ListReviews returns all reviews for a given PR number.
func (ks *KnowledgeStore) ListReviews(ctx context.Context, prNumber int) ([]PRReview, error) {
	rows, err := ks.db.QueryContext(ctx, `
		SELECT id, pr_number, reviewer, state, COALESCE(body, ''), COALESCE(workspace_id, ''), submitted_at
		FROM pr_reviews WHERE pr_number = ?
		ORDER BY submitted_at DESC`, prNumber)
	if err != nil {
		return nil, fmt.Errorf("list reviews: %w", err)
	}
	defer rows.Close()

	return scanReviews(rows)
}

// UpdatePullRequestMerge sets merge fields on a pull request after a merge.
func (ks *KnowledgeStore) UpdatePullRequestMerge(ctx context.Context, params UpdatePullRequestMergeParams) error {
	now := nowMS()
	result, err := ks.db.ExecContext(ctx, `
		UPDATE pull_requests
		SET state = 'merged', merge_commit_sha = ?, merged_at = ?, updated_at = ?
		WHERE number = ?`,
		params.MergeCommitSHA, now, now, params.Number)
	if err != nil {
		return fmt.Errorf("merge pull request: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("pull request #%d not found", params.Number)
	}

	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventPullRequestMerged, events.PullRequestMergedPayload{
			PRNumber:       params.Number,
			MergeCommitSHA: params.MergeCommitSHA,
			MergedAt:       now,
		})
	}

	return nil
}

// scanReviews scans pr_review rows into a slice.
func scanReviews(rows *sql.Rows) ([]PRReview, error) {
	var reviews []PRReview
	for rows.Next() {
		var r PRReview
		if err := rows.Scan(&r.ID, &r.PRNumber, &r.Reviewer, &r.State, &r.Body, &r.WorkspaceID, &r.SubmittedAt); err != nil {
			return nil, fmt.Errorf("scan review: %w", err)
		}
		reviews = append(reviews, r)
	}
	return reviews, rows.Err()
}

	// ── Issue CRUD ───────────────────────────────────────────────────────

// CreateIssue records an issue in the store.
func (ks *KnowledgeStore) CreateIssue(ctx context.Context, params CreateIssueParams) (*Issue, error) {
	now := nowMS()
	id := newULID()

	labelsJSON := "[]"
	if len(params.Labels) > 0 {
		tj, _ := json.Marshal(params.Labels)
		labelsJSON = string(tj)
	}

	iss := &Issue{
		ID:          id,
		Number:      params.Number,
		Title:       params.Title,
		State:       params.State,
		Labels:      params.Labels,
		URL:         params.URL,
		WorkspaceID: params.WorkspaceID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err := ks.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO issues (id, number, title, state, labels, url, workspace_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		iss.ID, iss.Number, iss.Title, iss.State, labelsJSON, iss.URL,
		nullableWorkspaceID(params.WorkspaceID), iss.CreatedAt, iss.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create issue: %w", err)
	}

	if ks.bus != nil {
		_ = ks.bus.Publish(ctx, events.EventIssueCreated, events.IssueCreatedPayload{
			Number:    iss.Number,
			Title:     iss.Title,
			Labels:    iss.Labels,
			URL:       iss.URL,
			CreatedAt: now,
		})
	}

	return iss, nil
}

// ListIssues returns all tracked issues, optionally filtered by workspace.
func (ks *KnowledgeStore) ListIssues(ctx context.Context, workspaceID string) ([]Issue, error) {
	var rows *sql.Rows
	var err error
	if workspaceID != "" {
		rows, err = ks.db.QueryContext(ctx, `
			SELECT id, number, title, state, labels, COALESCE(url, ''), COALESCE(workspace_id, ''), created_at, updated_at
			FROM issues WHERE workspace_id = ?
			ORDER BY created_at DESC`, workspaceID)
	} else {
		rows, err = ks.db.QueryContext(ctx, `
			SELECT id, number, title, state, labels, COALESCE(url, ''), COALESCE(workspace_id, ''), created_at, updated_at
			FROM issues ORDER BY created_at DESC`)
	}
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	defer rows.Close()

	return scanIssues(rows)
}

// scanPullRequests scans pull_request rows into a slice.
func scanPullRequests(rows *sql.Rows) ([]PullRequest, error) {
	var prs []PullRequest
	for rows.Next() {
		var pr PullRequest
		if err := rows.Scan(&pr.ID, &pr.Number, &pr.Title, &pr.State, &pr.Branch, &pr.Base, &pr.URL, &pr.WorkspaceID, &pr.MergeCommitSHA, &pr.MergedAt, &pr.CreatedAt, &pr.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan pull request: %w", err)
		}
		prs = append(prs, pr)
	}
	return prs, rows.Err()
}

// scanIssues scans issue rows into a slice.
func scanIssues(rows *sql.Rows) ([]Issue, error) {
	var issues []Issue
	for rows.Next() {
		var iss Issue
		var labelsJSON string
		if err := rows.Scan(&iss.ID, &iss.Number, &iss.Title, &iss.State, &labelsJSON, &iss.URL, &iss.WorkspaceID, &iss.CreatedAt, &iss.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan issue: %w", err)
		}
		if labelsJSON != "" && labelsJSON != "[]" {
			_ = json.Unmarshal([]byte(labelsJSON), &iss.Labels)
		}
		issues = append(issues, iss)
	}
	return issues, rows.Err()
}

// nullableWorkspaceID returns the workspace ID string, or nil if empty.
func nullableWorkspaceID(id string) interface{} {
	if id == "" {
		return nil
	}
	return id
}

// ── Plugin CRUD ───────────────────────────────────────────────────

// InstallPlugin registers a plugin in the database. The manifest JSON is
// stored so future migrations or compatibility checks can inspect it.
func (ks *KnowledgeStore) InstallPlugin(ctx context.Context, name, version, description, path, manifestJSON string) (*Plugin, error) {
	now := nowMS()
	id := newULID()

	p := &Plugin{
		ID:           id,
		Name:         name,
		Version:      version,
		Description:  description,
		Path:         path,
		Enabled:      true,
		ManifestJSON: manifestJSON,
		InstalledAt:  now,
	}

	_, err := ks.db.ExecContext(ctx, `
		INSERT INTO plugins (id, name, version, description, path, enabled, manifest_json, installed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Version, p.Description, p.Path, boolToInt(p.Enabled), p.ManifestJSON, p.InstalledAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return nil, ErrDuplicatePlugin
		}
		return nil, fmt.Errorf("install plugin: %w", err)
	}

	return p, nil
}

// GetPlugin retrieves a plugin by name.
func (ks *KnowledgeStore) GetPlugin(ctx context.Context, name string) (*Plugin, error) {
	p := &Plugin{}
	var enabled int
	err := ks.db.QueryRowContext(ctx, `
		SELECT id, name, version, COALESCE(description, ''), path, enabled, manifest_json, installed_at
		FROM plugins WHERE name = ?`, name,
	).Scan(&p.ID, &p.Name, &p.Version, &p.Description, &p.Path, &enabled, &p.ManifestJSON, &p.InstalledAt)
	if err == sql.ErrNoRows {
		return nil, ErrPluginNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get plugin: %w", err)
	}
	p.Enabled = enabled == 1
	_ = json.Unmarshal([]byte(p.ManifestJSON), &p.Manifest)
	return p, nil
}

// ListPlugins returns all installed plugins, ordered by name.
func (ks *KnowledgeStore) ListPlugins(ctx context.Context) ([]Plugin, error) {
	rows, err := ks.db.QueryContext(ctx, `
		SELECT id, name, version, COALESCE(description, ''), path, enabled, manifest_json, installed_at
		FROM plugins ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list plugins: %w", err)
	}
	defer rows.Close()

	return scanPlugins(rows)
}

// RemovePlugin uninstalls a plugin from the database.
func (ks *KnowledgeStore) RemovePlugin(ctx context.Context, name string) error {
	result, err := ks.db.ExecContext(ctx, `DELETE FROM plugins WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("remove plugin: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrPluginNotFound
	}
	return nil
}

// EnablePlugin sets a plugin's enabled flag to 1.
func (ks *KnowledgeStore) EnablePlugin(ctx context.Context, name string) error {
	result, err := ks.db.ExecContext(ctx, `UPDATE plugins SET enabled = 1 WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("enable plugin: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrPluginNotFound
	}
	return nil
}

// DisablePlugin sets a plugin's enabled flag to 0.
func (ks *KnowledgeStore) DisablePlugin(ctx context.Context, name string) error {
	result, err := ks.db.ExecContext(ctx, `UPDATE plugins SET enabled = 0 WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("disable plugin: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrPluginNotFound
	}
	return nil
}

// scanPlugins scans plugin rows into a slice.
func scanPlugins(rows *sql.Rows) ([]Plugin, error) {
	var plugins []Plugin
	for rows.Next() {
		var p Plugin
		var enabled int
		if err := rows.Scan(&p.ID, &p.Name, &p.Version, &p.Description, &p.Path, &enabled, &p.ManifestJSON, &p.InstalledAt); err != nil {
			return nil, fmt.Errorf("scan plugin: %w", err)
		}
		p.Enabled = enabled == 1
		_ = json.Unmarshal([]byte(p.ManifestJSON), &p.Manifest)
		plugins = append(plugins, p)
	}
	return plugins, rows.Err()
}

// ── Helpers ─────────────────────────────────────────────────────────

// newULID generates a ULID-like identifier: 10 chars of base32-encoded
// timestamp (milliseconds) + 16 chars of base32-encoded random bytes.
func newULID() string {
	// Timestamp component: current time in ms, 10 base32 chars.
	ts := time.Now().UnixMilli()
	var buf [10]byte
	for i := 9; i >= 0; i-- {
		buf[i] = ulidEncoding[ts&0x1F]
		ts >>= 5
	}

	// Random component: 80 bits (16 base32 chars) from crypto/rand.
	randBytes := make([]byte, 10)
	if _, err := rand.Read(randBytes); err != nil {
		// Fallback: use nanosecond timestamp bits.
		fallback := time.Now().UnixNano()
		binary.LittleEndian.PutUint64(randBytes[:8], uint64(fallback))
	}

	// Encode 10 random bytes (80 bits) into 16 base32 chars.
	// We need 80 bits; 10 bytes = 80 bits → each group of 5 bits = 1 char (16 chars).
	var rbuf [16]byte
	for i := 15; i >= 0; i-- {
		// Take bottom 5 bits from the random source.
		bits := uint(0)
		if i*5/8 < len(randBytes) {
			byteIdx := i * 5 / 8
			bitOffset := (i * 5) % 8
			bits = uint(randBytes[byteIdx]>>bitOffset) & 0x1F
			// Try to grab an extra bit from the next byte if available.
			if bitOffset > 3 && byteIdx+1 < len(randBytes) {
				bits |= uint(randBytes[byteIdx+1]<<(8-bitOffset)) & 0x1F
			}
		}
		rbuf[15-i] = ulidEncoding[bits%32]
	}

	return string(buf[:]) + string(rbuf[:])
}

// ulidEncoding is Crockford's base32 encoding (excluding I, L, O, U for
// readability — though ULID spec uses all 32 chars).
var ulidEncoding = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// scanDecisions scans decision rows into a slice.
func scanDecisions(rows *sql.Rows) ([]Decision, error) {
	var decisions []Decision
	for rows.Next() {
		var d Decision
		var workspaceID, supersedesID sql.NullString
		if err := rows.Scan(
			&d.ID, &d.CreatedAt, &d.UpdatedAt, &d.Status, &d.Title,
			&d.Context, &d.Decision, &d.Alternatives, &d.Consequences,
			&d.BodyPath, &workspaceID, &supersedesID,
		); err != nil {
			return nil, fmt.Errorf("scan decision: %w", err)
		}
		if workspaceID.Valid {
			d.WorkspaceID = &workspaceID.String
		}
		if supersedesID.Valid {
			d.SupersedesID = &supersedesID.String
		}
		decisions = append(decisions, d)
	}
	return decisions, rows.Err()
}

// scanOnboardingItems scans onboarding_item rows into a slice.
func scanOnboardingItems(rows *sql.Rows) ([]OnboardingItem, error) {
	var items []OnboardingItem
	for rows.Next() {
		var item OnboardingItem
		if err := rows.Scan(&item.ID, &item.SessionID, &item.ItemType,
			&item.ItemTarget, &item.CoveredAt, &item.Skipped); err != nil {
			return nil, fmt.Errorf("scan onboarding item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// scanWorkspaces scans workspace rows into a slice.
func scanWorkspaces(rows *sql.Rows) ([]Workspace, error) {
	var workspaces []Workspace
	for rows.Next() {
		var w Workspace
		var tagsJSON string
		if err := rows.Scan(
			&w.ID, &w.Name, &w.Description, &w.Status, &tagsJSON, &w.CreatedAt, &w.UpdatedAt, &w.LastCommitSHA,
		); err != nil {
			return nil, fmt.Errorf("scan workspace: %w", err)
		}
		if tagsJSON != "" && tagsJSON != "[]" {
			_ = json.Unmarshal([]byte(tagsJSON), &w.Tags)
		}
		workspaces = append(workspaces, w)
	}
	return workspaces, rows.Err()
}

// scanWorkspaceFiles scans workspace_file rows into a slice.
func scanWorkspaceFiles(rows *sql.Rows) ([]WorkspaceFile, error) {
	var files []WorkspaceFile
	for rows.Next() {
		var f WorkspaceFile
		if err := rows.Scan(&f.ID, &f.WorkspaceID, &f.Path, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan workspace file: %w", err)
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// scanWorkspaceBranches scans workspace_branch rows into a slice.
func scanWorkspaceBranches(rows *sql.Rows) ([]WorkspaceBranch, error) {
	var branches []WorkspaceBranch
	for rows.Next() {
		var b WorkspaceBranch
		if err := rows.Scan(&b.ID, &b.WorkspaceID, &b.BranchName, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan workspace branch: %w", err)
		}
		branches = append(branches, b)
	}
	return branches, rows.Err()
}

// scanWorkspaceCommits scans workspace_commit rows into a slice.
func scanWorkspaceCommits(rows *sql.Rows) ([]WorkspaceCommit, error) {
	var commits []WorkspaceCommit
	for rows.Next() {
		var c WorkspaceCommit
		if err := rows.Scan(&c.ID, &c.WorkspaceID, &c.CommitSHA, &c.BranchName, &c.Message, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan workspace commit: %w", err)
		}
		commits = append(commits, c)
	}
	return commits, rows.Err()
}

// coalesceStr returns the dereferenced string or "" if the pointer is nil.
func coalesceStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// boolToInt converts a bool to an int (0/1) for SQLite storage.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
