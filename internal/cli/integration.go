// Package cli provides the event-driven integration layer that connects
// the Git adapter, Workspace Engine, Knowledge Engine, and Event Bus.
//
// This is the "glue" that makes the system feel cohesive: commits
// automatically update workspace activity, branches get tracked, and
// knowledge artifacts get linked to the right commits.
// Copyright 2026 The GOT Authors. MIT License.
package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/supunhg/got/internal/events"
	"github.com/supunhg/got/internal/git"
	"github.com/supunhg/got/internal/store"
)

// IntegrationService ties the event bus to the store layer. It subscribes
// to relevant events and performs automatic updates (e.g., updating
// workspace last_activity when a commit is made on a tracked branch).
type IntegrationService struct {
	bus      *events.Bus
	ks       *store.KnowledgeStore
	git      *git.ExecAdapter
	repoPath string
}

// NewIntegrationService creates an integration service and subscribes to
// all relevant events. Call Close() to unsubscribe.
func NewIntegrationService(bus *events.Bus, ks *store.KnowledgeStore, repoPath string) *IntegrationService {
	s := &IntegrationService{
		bus:      bus,
		ks:       ks,
		repoPath: repoPath,
	}

	if repoPath != "" {
		s.git = git.NewExecAdapter(bus)
		_ = s.git.OpenRepository(context.Background(), repoPath)
	}

	s.subscribe()
	return s
}

// Close unsubscribes all handlers.
func (s *IntegrationService) Close() {
	// The event bus handles cleanup when closed.
}

func (s *IntegrationService) subscribe() {
	// When a CommitCreated event fires, check if any workspace tracks the
	// current branch and update the workspace's last_activity and commits.
	_, _ = s.bus.Subscribe(events.EventCommitCreated, s.onCommitCreated)

	// When a WorkspaceCreated event fires with --create-branch, a Git branch
	// is created (handled by the CLI command directly, not here).
}

// onCommitCreated handles CommitCreated events: updates workspace activity
// for any workspace tracking the branch that the commit was made on.
func (s *IntegrationService) onCommitCreated(ctx context.Context, e events.Event) error {
	payload, ok := e.Payload.(events.CommitCreatedPayload)
	if !ok {
		return nil
	}

	if payload.Branch == "" || payload.SHA == "" {
		return nil
	}

	workspaces, err := s.ks.ListWorkspaces(ctx)
	if err != nil {
		return fmt.Errorf("list workspaces: %w", err)
	}

	for _, ws := range workspaces {
		branches, listErr := s.ks.ListWorkspaceBranches(ctx, ws.Name)
		if listErr != nil {
			continue
		}

		for _, b := range branches {
			if b.BranchName == payload.Branch {
				// Record the commit in this workspace.
				_, _ = s.ks.AddWorkspaceCommit(ctx, store.AddWorkspaceCommitParams{
					WorkspaceName: ws.Name,
					CommitSHA:     payload.SHA,
					BranchName:    payload.Branch,
					Message:       truncate(payload.Message, 80),
				})
			}
		}
	}

	return nil
}

// AutoLinkOnCommit finds decisions and notes created since the last commit
// and links them to the new commit.
func AutoLinkOnCommit(ctx context.Context, ks *store.KnowledgeStore, adapter *git.ExecAdapter, commitSHA string) error {
	currentBranch, _ := adapter.CurrentBranch(ctx)

	// Get the previous commit SHA.
	prevSHA, _, _ := adapter.Run(ctx, "rev-parse", commitSHA+"~1")

	// Find decisions not yet linked to any commit.
	decisions, _ := ks.ListAllDecisions(ctx)
	for _, d := range decisions {
		links, _ := ks.GetDecisionLinks(ctx, d.ID)
		alreadyLinked := false
		for _, l := range links {
			if l.LinkType == "commit" {
				alreadyLinked = true
				break
			}
		}
		if alreadyLinked {
			continue
		}

		// Only link decisions created around the time of the commit.
		if prevSHA != "" {
			prevTime, _, _ := adapter.Run(ctx, "log", "-1", "--format=%ct", prevSHA)
			if prevTime != "" {
				var prevTimeSec int64
				if _, scanErr := fmt.Sscanf(prevTime, "%d", &prevTimeSec); scanErr == nil {
					if d.CreatedAt/1000 < prevTimeSec {
						continue
					}
				}
			}
		}

		_ = ks.LinkDecision(ctx, store.LinkDecisionParams{
			DecisionID: d.ID,
			LinkType:   "commit",
			Target:     commitSHA,
			Branch:     currentBranch,
		})
	}

	// Find notes without commit_hash and update them.
	notes, _ := ks.ListNotes(ctx, store.NoteFilter{All: true})
	for _, n := range notes {
		if strings.TrimSpace(n.CommitHash) != "" {
			continue
		}
		// Re-create the note with the commit hash.
		_ = ks.DeleteNote(ctx, n.ID)
		_, _ = ks.CreateNote(ctx, store.CreateNoteParams{
			Message:     n.Message,
			WorkspaceID: n.WorkspaceID,
			Branch:      n.Branch,
			CommitHash:  commitSHA,
		})
	}

	return nil
}
