package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
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
	case "commit", "file", "workspace":
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

// coalesceStr returns the dereferenced string or "" if the pointer is nil.
func coalesceStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
