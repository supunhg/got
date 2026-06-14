// Copyright 2026 Supun Hewagamage. MIT License.
package store

import (
	"context"
	"fmt"
)

// Snapshot represents a recovery point before a destructive Git operation.
type Snapshot struct {
	ID        string `json:"id"`
	CreatedAt int64  `json:"created_at"`
	Reason    string `json:"reason"`
	Ref       string `json:"ref"`
	ReflogSel string `json:"reflog_sel,omitempty"`
	StashRef  string `json:"stash_ref,omitempty"`
	Metadata  string `json:"metadata,omitempty"`
}

// CreateSnapshotParams holds fields for creating a snapshot.
type CreateSnapshotParams struct {
	Reason    string // e.g. "before-reset", "before-rebase", "before-force-push"
	Ref       string // e.g. "refs/heads/feature/x" or "detached@abc123"
	ReflogSel string // optional reflog selector
	StashRef  string // optional git stash ref
	Metadata  string // optional JSON metadata
}

// CreateSnapshot records a recovery point in the snapshots table.
func (ks *KnowledgeStore) CreateSnapshot(ctx context.Context, params CreateSnapshotParams) (*Snapshot, error) {
	now := nowMS()
	id := newULID()

	if params.Reason == "" {
		return nil, fmt.Errorf("snapshot reason is required")
	}
	if params.Ref == "" {
		return nil, fmt.Errorf("snapshot ref is required")
	}

	metadata := params.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	s := &Snapshot{
		ID:        id,
		CreatedAt: now,
		Reason:    params.Reason,
		Ref:       params.Ref,
		ReflogSel: params.ReflogSel,
		StashRef:  params.StashRef,
		Metadata:  metadata,
	}

	_, err := ks.db.ExecContext(
		ctx, `
		INSERT INTO snapshots (id, created_at, reason, ref, reflog_sel, stash_ref, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.CreatedAt, s.Reason, s.Ref, s.ReflogSel, s.StashRef, s.Metadata,
	)
	if err != nil {
		return nil, fmt.Errorf("insert snapshot: %w", err)
	}

	return s, nil
}

// GetSnapshot retrieves a snapshot by ID.
func (ks *KnowledgeStore) GetSnapshot(ctx context.Context, id string) (*Snapshot, error) {
	s := &Snapshot{}
	err := ks.db.QueryRowContext(
		ctx, `
		SELECT id, created_at, reason, ref, COALESCE(reflog_sel, ''),
		       COALESCE(stash_ref, ''), COALESCE(metadata, '{}')
		FROM snapshots WHERE id = ?`, id,
	).Scan(&s.ID, &s.CreatedAt, &s.Reason, &s.Ref, &s.ReflogSel, &s.StashRef, &s.Metadata)
	if err != nil {
		return nil, ErrSnapshotNotFound
	}
	return s, nil
}

// ListSnapshots returns all snapshots ordered by creation time (most recent first).
func (ks *KnowledgeStore) ListSnapshots(ctx context.Context, limit int) ([]Snapshot, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := ks.db.QueryContext(ctx, `
		SELECT id, created_at, reason, ref, COALESCE(reflog_sel, ''),
		       COALESCE(stash_ref, ''), COALESCE(metadata, '{}')
		FROM snapshots
		ORDER BY created_at DESC, id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []Snapshot
	for rows.Next() {
		var s Snapshot
		if err := rows.Scan(&s.ID, &s.CreatedAt, &s.Reason, &s.Ref, &s.ReflogSel, &s.StashRef, &s.Metadata); err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		snapshots = append(snapshots, s)
	}
	return snapshots, rows.Err()
}

// DeleteSnapshot removes a snapshot by ID.
func (ks *KnowledgeStore) DeleteSnapshot(ctx context.Context, id string) error {
	result, err := ks.db.ExecContext(ctx, `DELETE FROM snapshots WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete snapshot: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("snapshot not found: %s", id)
	}
	return nil
}
