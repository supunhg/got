// Copyright 2026 Supun Hewagamage. MIT License.
package store

import (
	"context"
	"testing"
)

func TestCreateSnapshot(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	s, err := ks.CreateSnapshot(ctx, CreateSnapshotParams{
		Reason: "before-reset",
		Ref:    "refs/heads/main@abc123def456",
	})
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	if s.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if s.Reason != "before-reset" {
		t.Fatalf("expected reason 'before-reset', got %q", s.Reason)
	}
	if s.Ref != "refs/heads/main@abc123def456" {
		t.Fatalf("expected ref 'refs/heads/main@abc123def456', got %q", s.Ref)
	}
	if s.CreatedAt == 0 {
		t.Fatal("expected non-zero created_at")
	}
}

func TestCreateSnapshotEmptyReason(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := ks.CreateSnapshot(ctx, CreateSnapshotParams{
		Reason: "",
		Ref:    "refs/heads/main",
	})
	if err == nil {
		t.Fatal("expected error for empty reason")
	}
}

func TestCreateSnapshotEmptyRef(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := ks.CreateSnapshot(ctx, CreateSnapshotParams{
		Reason: "test",
		Ref:    "",
	})
	if err == nil {
		t.Fatal("expected error for empty ref")
	}
}

func TestGetSnapshot(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	created, err := ks.CreateSnapshot(ctx, CreateSnapshotParams{
		Reason: "before-rebase",
		Ref:    "refs/heads/feature@xyz",
	})
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	fetched, err := ks.GetSnapshot(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("ID mismatch: %q vs %q", fetched.ID, created.ID)
	}
	if fetched.Reason != "before-rebase" {
		t.Fatalf("reason mismatch: %q", fetched.Reason)
	}
}

func TestGetSnapshotNotFound(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := ks.GetSnapshot(ctx, "nonexistent")
	if err != ErrSnapshotNotFound {
		t.Fatalf("expected ErrSnapshotNotFound, got %v", err)
	}
}

func TestListSnapshots(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	ks.CreateSnapshot(ctx, CreateSnapshotParams{Reason: "first", Ref: "a"})
	ks.CreateSnapshot(ctx, CreateSnapshotParams{Reason: "second", Ref: "b"})

	snapshots, err := ks.ListSnapshots(ctx, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}
	// Most recent first.
	if snapshots[0].Reason != "second" {
		t.Fatalf("expected first snapshot 'second', got %q", snapshots[0].Reason)
	}
}

func TestDeleteSnapshot(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	s, _ := ks.CreateSnapshot(ctx, CreateSnapshotParams{Reason: "delete me", Ref: "x"})

	err := ks.DeleteSnapshot(ctx, s.ID)
	if err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}

	_, err = ks.GetSnapshot(ctx, s.ID)
	if err != ErrSnapshotNotFound {
		t.Fatalf("expected ErrSnapshotNotFound after delete, got %v", err)
	}
}

func TestDeleteSnapshotNotFound(t *testing.T) {
	ks, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	err := ks.DeleteSnapshot(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent snapshot")
	}
}
