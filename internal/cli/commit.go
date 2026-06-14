// Copyright 2026 Supun Hewagamage. MIT License.
package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/supunhg/got/internal/git"
	"github.com/supunhg/got/internal/store"
)

func newCommitCmd() *cobra.Command {
	var message string
	var allowEmpty bool
	var all bool
	var autoLink bool

	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Create a commit with staged changes",
		Long: `Create a commit with the given message.

Stages all tracked files (git add -A) before committing unless
changes are already staged. Use -m to supply a message directly.

With --auto-link, automatically links any unlinked decisions and notes
created since the last commit to the new commit.

Examples:
  got commit -m "feat: add user authentication"
  got commit -m "fix: resolve null pointer in login" --all
  got commit -m "feat: add OAuth" --auto-link`,

		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCommit(cmd, message, allowEmpty, all, autoLink)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&message, "message", "m", "", "Commit message")
	flags.BoolVar(&allowEmpty, "allow-empty", false, "Allow empty commit")
	flags.BoolVarP(&all, "all", "a", false, "Stage all tracked files first")
	flags.BoolVar(&autoLink, "auto-link", false, "Auto-link unlinked decisions/notes since last commit")

	return cmd
}

func runCommit(cmd *cobra.Command, message string, allowEmpty, all, autoLink bool) error {
	ctx := context.Background()

	message = strings.TrimSpace(message)
	if message == "" {
		return fmt.Errorf("commit message is required (use -m \"<message>\")")
	}

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Stage all if --all is set.
	if all {
		if _, _, err := adapter.Run(ctx, "add", "-A"); err != nil {
			return fmt.Errorf("commit: stage all: %w", err)
		}
	}

	// Use CreateCommit which handles events and SHA retrieval.
	// For --allow-empty, use raw Run since CreateCommit doesn't support that flag.
	var sha string

	if allowEmpty {
		_, stderr, err := adapter.Run(ctx, "commit", "-m", message, "--allow-empty")
		if err != nil {
			return fmt.Errorf("commit: %w\n%s", err, stderr)
		}
		sha, _, _ = adapter.Run(ctx, "rev-parse", "HEAD")
	} else {
		var err error
		sha, err = adapter.CreateCommit(ctx, message, "")
		if err != nil {
			return fmt.Errorf("commit: %w", err)
		}
	}

	// Auto-link unlinked decisions and notes since last commit.
	if autoLink {
		if err := autoLinkDecisionsAndNotes(ctx, sha, repoPath); err != nil {
			// Warn but don't fail — the commit already succeeded.
			fmt.Fprintf(cmd.ErrOrStderr(), "  warning: auto-link: %v\n", err)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "  Auto-linked: decisions and notes linked to %s\n", sha[:8])
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Committed: %s\n", truncate(message, 60))
	fmt.Fprintf(cmd.OutOrStdout(), "  SHA: %s\n", sha)

	return nil
}

// autoLinkDecisionsAndNotes finds decisions and notes created since the last
// commit that have no links/commit references, and links them to the given
// commit SHA. Also updates workspaces tracking the current branch.
func autoLinkDecisionsAndNotes(ctx context.Context, commitSHA, repoPath string) error {
	kc, err := openKnowledgeStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer kc.Close()
	ks := kc.ks

	adapter := git.NewExecAdapter(kc.bus)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("open repo: %w", err)
	}

	currentBranch, _ := adapter.CurrentBranch(ctx)

	// Get the previous commit SHA (the one before this one).
	prevSHA, _, _ := adapter.Run(ctx, "rev-parse", commitSHA+"~1")

	// Find all decisions created since the last commit.
	decisions, _ := ks.ListAllDecisions(ctx)
	linkCount := 0

	for _, d := range decisions {
		// Skip if already linked to any commit.
		links, _ := ks.GetDecisionLinks(ctx, d.ID)
		hasCommitLink := false
		for _, l := range links {
			if l.LinkType == "commit" {
				hasCommitLink = true
				break
			}
		}
		if hasCommitLink {
			continue
		}

		// Link if created after previous commit (or if no previous commit).
		if prevSHA != "" {
			prevTime, _, _ := adapter.Run(ctx, "log", "-1", "--format=%ct", prevSHA)
			if prevTime != "" && fmt.Sprintf("%d", d.CreatedAt/1000) < prevTime {
				continue
			}
		}

		if err := ks.LinkDecision(ctx, store.LinkDecisionParams{
			DecisionID: d.ID,
			LinkType:   "commit",
			Target:     commitSHA,
			Branch:     currentBranch,
		}); err == nil {
			linkCount++
		}
	}

	// Find notes with no commit_hash and update them in place.
	notes, _ := ks.ListNotes(ctx, store.NoteFilter{All: true})
	for _, n := range notes {
		if n.CommitHash != "" {
			continue
		}
		if err := ks.UpdateNoteCommitHash(ctx, n.ID, commitSHA); err == nil {
			linkCount++
		}
	}

	// Update workspaces tracking the current branch.
	workspaces, _ := ks.ListWorkspaces(ctx)
	for _, ws := range workspaces {
		branches, _ := ks.ListWorkspaceBranches(ctx, ws.Name)
		for _, b := range branches {
			if b.BranchName == currentBranch {
				msg, _, _ := adapter.Run(ctx, "log", "-1", "--format=%s", commitSHA)
				ks.AddWorkspaceCommit(ctx, store.AddWorkspaceCommitParams{
					WorkspaceName: ws.Name,
					CommitSHA:     commitSHA,
					BranchName:    currentBranch,
					Message:       msg,
				})
			}
		}
	}

	return nil
}
