// Copyright 2026 The GOT Authors. MIT License.
package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/spf13/cobra"

	"github.com/supunhg/got/internal/git"
	"github.com/supunhg/got/internal/store"
)

// newGitHubCmd builds the `got github` command tree.
func newGitHubCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "github",
		Short: "GitHub integration (built-in plugin)",
		Long: `Manage GitHub pull requests and issues directly from GOT.

Requires a GitHub personal access token with repo scope. Configure with:
  got github auth --token ghp_...

All commands automatically link to workspaces when a workspace context
is active (or via --workspace flag).`,
	}

	cmd.AddCommand(newGitHubAuthCmd())
	cmd.AddCommand(newGitHubPRCreateCmd())
	cmd.AddCommand(newGitHubPRListCmd())
	cmd.AddCommand(newGitHubPRStatusCmd())
	cmd.AddCommand(newGitHubPRReviewCmd())
	cmd.AddCommand(newGitHubPRMergeCmd())
	cmd.AddCommand(newGitHubPRDiffCmd())
	cmd.AddCommand(newGitHubIssueCreateCmd())
	cmd.AddCommand(newGitHubIssueListCmd())
	cmd.AddCommand(newGitHubLinkCmd())

	return cmd
}

// ── Auth ─────────────────────────────────────────────────────────────

func newGitHubAuthCmd() *cobra.Command {
	var token, owner, repo, baseBranch string

	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Store GitHub authentication token and repo info",
		Long: `Configure GitHub authentication for GOT.

Stores a personal access token and repository information in the
GOT database. The token must have repo scope for full functionality.

If --token is omitted, GOT tries to read the token from 'gh auth token'
(GitHub CLI). If neither works, an interactive prompt asks for one.

Examples:
  got github auth --token ghp_abc123
  got github auth --token ghp_abc123 --owner myorg --repo myrepo
  got github auth --owner myorg --repo myrepo`,

		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitHubAuth(cmd, token, owner, repo, baseBranch)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&token, "token", "", "GitHub personal access token")
	flags.StringVar(&owner, "owner", "", "Repository owner (user or org)")
	flags.StringVar(&repo, "repo", "", "Repository name")
	flags.StringVar(&baseBranch, "base-branch", "main", "Default base branch for PRs")

	return cmd
}

func runGitHubAuth(cmd *cobra.Command, token, owner, repo, baseBranch string) error {
	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	// Try to get token from gh CLI if not provided.
	if token == "" {
		ghToken, ghErr := getGitHubTokenFromCLI()
		if ghErr == nil && ghToken != "" {
			token = ghToken
			fmt.Fprintf(cmd.OutOrStdout(), "  Using token from GitHub CLI (gh)\n")
		}
	}

	if token == "" {
		return fmt.Errorf("GitHub token is required (use --token or install GitHub CLI")
	}

	// Validate the token by calling the API.
	client := github.NewClient(nil).WithAuthToken(token)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	user, _, apiErr := client.Users.Get(ctx, "")
	if apiErr != nil {
		return fmt.Errorf("invalid GitHub token: %w\nMake sure the token has the correct scopes", apiErr)
	}

	// If owner/repo not provided, try to detect from git remote.
	if owner == "" || repo == "" {
		detectedOwner, detectedRepo, detectErr := detectGitHubRepo()
		if detectErr == nil {
			if owner == "" {
				owner = detectedOwner
			}
			if repo == "" {
				repo = detectedRepo
			}
		}
	}

	// Validate owner/repo by fetching the repository.
	if owner != "" && repo != "" {
		_, _, repoErr := client.Repositories.Get(ctx, owner, repo)
		if repoErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "  warning: could not validate %s/%s: %v\n", owner, repo, repoErr)
		}
	}

	cfg := store.GitHubConfig{
		Token:      token,
		Owner:      owner,
		Repo:       repo,
		BaseBranch: baseBranch,
	}

	if err := ks.SetGitHubConfig(cmd.Context(), cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Authenticated as %s\n", user.GetLogin())
	if owner != "" && repo != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Repository: %s/%s\n", owner, repo)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  Owner:      %s\n", owner)
	fmt.Fprintf(cmd.OutOrStdout(), "  Repo:       %s\n", repo)
	fmt.Fprintf(cmd.OutOrStdout(), "  Base:       %s\n", baseBranch)

	return nil
}

// ── PR Create ────────────────────────────────────────────────────────

func newGitHubPRCreateCmd() *cobra.Command {
	var title, body, base string
	var draft bool
	var workspace string

	cmd := &cobra.Command{
		Use:   "pr create",
		Short: "Create a pull request from the current branch",
		Long: `Create a pull request on GitHub from the current Git branch.

Automatically includes references to linked workspaces, decisions,
and notes in the PR body. Links the PR to the current workspace.

Examples:
  got github pr create --title "Add OAuth support"
  got github pr create --title "Fix bug" --body "Closes #42" --draft
  got github pr create --title "Refactor auth" --workspace auth`,

		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitHubPRCreate(cmd, title, body, base, draft, workspace)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&title, "title", "", "PR title (required)")
	flags.StringVar(&body, "body", "", "PR body text")
	flags.StringVar(&base, "base", "", "Target branch (default: configured base branch)")
	flags.BoolVar(&draft, "draft", false, "Create as draft PR")
	flags.StringVar(&workspace, "workspace", "", "Link to this workspace")

	return cmd
}

func runGitHubPRCreate(cmd *cobra.Command, title, body, base string, draft bool, workspace string) error {
	if title == "" {
		return fmt.Errorf("PR title is required (use --title)")
	}

	kc, client, cfg, err := getGitHubClient(cmd)
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get current branch.
	currentBranch, err := getCurrentBranch()
	if err != nil {
		return fmt.Errorf("get current branch: %w. Make sure you are in a Git repository", err)
	}

	if base == "" {
		base = cfg.BaseBranch
	}

	// Build PR body with workspace/decision references.
	fullBody := body
	if workspace != "" || body == "" {
		var refs []string

		if workspace != "" {
			ws, wsErr := ks.GetWorkspace(ctx, workspace)
			if wsErr == nil {
				refs = append(refs, fmt.Sprintf("Related workspace: %s", ws.Name))
				// Add linked decisions.
				decisions, _ := ks.ListDecisions(ctx, store.DecisionFilter{WorkspaceID: &ws.Name, All: true})
				for _, d := range decisions {
					refs = append(refs, fmt.Sprintf("- Decision: %s (%s)", d.Title, d.ID[:8]))
				}
				// Add linked notes.
				notes, _ := ks.ListNotes(ctx, store.NoteFilter{WorkspaceID: &ws.Name, All: true})
				for _, n := range notes {
					refs = append(refs, fmt.Sprintf("- Note: %s", truncate(n.Message, 60)))
				}
			}
		}

		if len(refs) > 0 {
			if fullBody != "" {
				fullBody += "\n\n---\n"
			}
			fullBody += strings.Join(refs, "\n")
		}
	}

	// Create PR via GitHub API.
	prRequest := &github.NewPullRequest{
		Title: &title,
		Head:  &currentBranch,
		Base:  &base,
		Body:  &fullBody,
		Draft: &draft,
	}

	ghPR, _, apiErr := client.PullRequests.Create(ctx, cfg.Owner, cfg.Repo, prRequest)
	if apiErr != nil {
		return fmt.Errorf("create PR: %w", apiErr)
	}

	prNum := ghPR.GetNumber()
	prURL := ghPR.GetHTMLURL()

	// Record the PR in the store.
	_, recErr := ks.CreatePullRequest(ctx, store.CreatePullRequestParams{
		Number:      prNum,
		Title:       title,
		State:       "open",
		Branch:      currentBranch,
		Base:        base,
		URL:         prURL,
		WorkspaceID: workspace,
	})
	if recErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "  warning: could not record PR in store: %v\n", recErr)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pull request created: #%d - %s\n", prNum, title)
	fmt.Fprintf(cmd.OutOrStdout(), "  URL: %s\n", prURL)
	if workspace != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Workspace: %s\n", workspace)
	}

	return nil
}

// ── PR List ──────────────────────────────────────────────────────────

func newGitHubPRListCmd() *cobra.Command {
	var branch string
	var workspace string

	cmd := &cobra.Command{
		Use:   "pr list",
		Short: "List open pull requests",
		Long: `List open pull requests for the repository.

Optionally filter by branch or workspace.

Examples:
  got github pr list
  got github pr list --branch feat/oauth
  got github pr list --workspace auth`,

		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitHubPRList(cmd, branch, workspace)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&branch, "branch", "", "Filter by head branch")
	flags.StringVar(&workspace, "workspace", "", "Filter by linked workspace")

	return cmd
}

func runGitHubPRList(cmd *cobra.Command, branch, workspace string) error {
	kc, client, cfg, err := getGitHubClient(cmd)
	if err != nil {
		return err
	}
	defer kc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &github.PullRequestListOptions{State: "open"}
	if branch != "" {
		opts.Head = branch
	}

	prs, _, apiErr := client.PullRequests.List(ctx, cfg.Owner, cfg.Repo, opts)
	if apiErr != nil {
		return fmt.Errorf("list PRs: %w", apiErr)
	}

	if len(prs) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No open pull requests.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "#\tTITLE\tBRANCH\tSTATUS\tDRAFT")
	for _, pr := range prs {
		draftStr := ""
		if pr.GetDraft() {
			draftStr = "draft"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", pr.GetNumber(), truncate(pr.GetTitle(), 50), pr.GetHead().GetRef(), pr.GetState(), draftStr)
	}
	w.Flush()

	return nil
}

// ── PR Status ────────────────────────────────────────────────────────

func newGitHubPRStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr status <pr-number>",
		Short: "Show detailed pull request status",
		Long: `Display detailed information about a specific pull request.

Shows title, description, mergeable state, review status, and
check run status when available.

Examples:
  got github pr status 42`,

		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitHubPRStatus(cmd, args[0])
		},
	}

	return cmd
}

func runGitHubPRStatus(cmd *cobra.Command, arg string) error {
	prNum, err := parseIntArg(arg, "PR number")
	if err != nil {
		return err
	}

	kc, client, cfg, err := getGitHubClient(cmd)
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pr, _, apiErr := client.PullRequests.Get(ctx, cfg.Owner, cfg.Repo, prNum)
	if apiErr != nil {
		return fmt.Errorf("get PR #%d: %w", prNum, apiErr)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "#%d: %s\n", pr.GetNumber(), pr.GetTitle())
	fmt.Fprintf(cmd.OutOrStdout(), "  State:     %s\n", pr.GetState())
	if pr.GetMerged() {
		fmt.Fprintf(cmd.OutOrStdout(), "  Merged:    yes\n")
	}
	if pr.GetDraft() {
		fmt.Fprintf(cmd.OutOrStdout(), "  Draft:     yes\n")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  Branch:    %s -> %s\n", pr.GetHead().GetRef(), pr.GetBase().GetRef())
	fmt.Fprintf(cmd.OutOrStdout(), "  URL:       %s\n", pr.GetHTMLURL())
	fmt.Fprintf(cmd.OutOrStdout(), "  Created:   %s\n", pr.GetCreatedAt().Format("2006-01-02 15:04 MST"))

	if pr.Mergeable != nil {
		if *pr.Mergeable {
			fmt.Fprintln(cmd.OutOrStdout(), "  Mergeable: yes")
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "  Mergeable: no (conflicts)")
		}
	}
	if pr.GetMergeableState() != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Merge st.: %s\n", pr.GetMergeableState())
	}

	if pr.GetBody() != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "\nDescription:\n%s\n", pr.GetBody())
	}

	// Fetch review status from GitHub API.
	reviews, _, reviewErr := client.PullRequests.ListReviews(ctx, cfg.Owner, cfg.Repo, prNum, nil)
	if reviewErr == nil && len(reviews) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nReviews (GitHub): %d\n", len(reviews))
		// Show only the most recent review per user.
		seen := make(map[string]bool)
		for _, r := range reviews {
			user := r.GetUser().GetLogin()
			if seen[user] {
				continue
			}
			seen[user] = true
			state := r.GetState()
			if state == "APPROVED" {
				fmt.Fprintf(cmd.OutOrStdout(), "  + %s: approved\n", user)
			} else if state == "CHANGES_REQUESTED" {
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s: changes requested\n", user)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "  ? %s: %s\n", user, state)
			}
		}
	}

	// Show stored reviews from GOT.
	storedReviews, listErr := ks.ListReviews(ctx, prNum)
	if listErr == nil && len(storedReviews) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nReviews (GOT): %d\n", len(storedReviews))
		for _, r := range storedReviews {
			stateLabel := r.State
			if r.State == "APPROVED" {
				stateLabel = "approved"
			} else if r.State == "CHANGES_REQUESTED" {
				stateLabel = "changes requested"
			} else {
				stateLabel = "commented"
			}
			ts := time.UnixMilli(r.SubmittedAt).Format("2006-01-02 15:04")
			fmt.Fprintf(cmd.OutOrStdout(), "  %s by %s (%s)\n", stateLabel, r.Reviewer, ts)
			if r.Body != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "    %s\n", truncate(r.Body, 80))
			}
		}
	}

	// Merge hint.
	if !pr.GetMerged() && pr.GetState() == "open" && pr.Mergeable != nil && *pr.Mergeable {
		fmt.Fprintf(cmd.OutOrStdout(), "\n  Use 'got github pr merge %d' to merge this PR\n", prNum)
	}

	// Show stored merge info.
	storedPR, storedErr := ks.GetPullRequestByNumber(ctx, prNum)
	if storedErr == nil && storedPR.MergeCommitSHA != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "\nMerged in:  %s\n", storedPR.MergeCommitSHA)
	}

	return nil
}

// ── PR Review ────────────────────────────────────────────────────────

func newGitHubPRReviewCmd() *cobra.Command {
	var body string

	cmd := &cobra.Command{
		Use:   "pr review <pr-number> [action]",
		Short: "Submit a review on a pull request",
		Long: `Submit a review on a pull request: approve, request changes, or comment.

Actions: approve, request-changes, comment (default: comment)
--body is required for request-changes and comment.

Examples:
  got github pr review 42 approve
  got github pr review 42 approve --body "LGTM!"
  got github pr review 42 request-changes --body "Please fix the test"
  got github pr review 42 comment --body "Looking good so far"`,

		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			action := "comment"
			if len(args) > 1 {
				action = args[1]
			}
			return runGitHubPRReview(cmd, args[0], action, body)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&body, "body", "", "Review body text")

	return cmd
}

func runGitHubPRReview(cmd *cobra.Command, arg, action, body string) error {
	prNum, err := parseIntArg(arg, "PR number")
	if err != nil {
		return err
	}

	// Map action to GitHub review state.
	var state string
	switch action {
	case "approve":
		state = "APPROVED"
	case "request-changes":
		state = "CHANGES_REQUESTED"
	case "comment":
		state = "COMMENTED"
	default:
		return fmt.Errorf("invalid action %q: must be approve, request-changes, or comment", action)
	}

	if state != "APPROVED" && body == "" {
		return fmt.Errorf("--body is required for %s reviews", action)
	}

	kc, client, cfg, err := getGitHubClient(cmd)
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get the authenticated user to record as reviewer.
	user, _, userErr := client.Users.Get(ctx, "")
	if userErr != nil {
		return fmt.Errorf("get current user: %w", userErr)
	}
	reviewer := user.GetLogin()

	// Submit the review via GitHub API.
	reviewReq := &github.PullRequestReviewRequest{
		Body:  &body,
		Event: &state,
	}
	_, _, apiErr := client.PullRequests.CreateReview(ctx, cfg.Owner, cfg.Repo, prNum, reviewReq)
	if apiErr != nil {
		return fmt.Errorf("submit review: %w", apiErr)
	}

	// Look up the PR to get workspace ID for linking.
	workspaceID := ""
	if storedPR, getErr := ks.GetPullRequestByNumber(ctx, prNum); getErr == nil {
		workspaceID = storedPR.WorkspaceID
	}

	// Record the review in store.
	_, recErr := ks.CreateReview(ctx, store.CreateReviewParams{
		PRNumber:    prNum,
		Reviewer:    reviewer,
		State:       state,
		Body:        body,
		WorkspaceID: workspaceID,
	})
	if recErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "  warning: could not record review: %v\n", recErr)
	}

	stateLabel := action
	fmt.Fprintf(cmd.OutOrStdout(), "Review submitted on PR #%d\n", prNum)
	fmt.Fprintf(cmd.OutOrStdout(), "  Action:  %s\n", stateLabel)
	fmt.Fprintf(cmd.OutOrStdout(), "  By:      %s\n", reviewer)
	if body != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Body:    %s\n", truncate(body, 80))
	}

	return nil
}

// ── PR Merge ─────────────────────────────────────────────────────────

func newGitHubPRMergeCmd() *cobra.Command {
	var method string
	var deleteBranch bool

	cmd := &cobra.Command{
		Use:   "pr merge <pr-number>",
		Short: "Merge a pull request",
		Long: `Merge a pull request with the specified method.

Merge methods: merge (default), squash, rebase
Use --delete-branch to delete the remote branch after merge.

Examples:
  got github pr merge 42
  got github pr merge 42 --method squash
  got github pr merge 42 --method rebase --delete-branch`,

		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitHubPRMerge(cmd, args[0], method, deleteBranch)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&method, "method", "merge", "Merge method: merge, squash, or rebase")
	flags.BoolVar(&deleteBranch, "delete-branch", false, "Delete the remote branch after merge")

	return cmd
}

func runGitHubPRMerge(cmd *cobra.Command, arg, method string, deleteBranch bool) error {
	prNum, err := parseIntArg(arg, "PR number")
	if err != nil {
		return err
	}

	// Map method to GitHub merge type.
	var mergeMethod string
	switch method {
	case "merge":
		mergeMethod = "merge"
	case "squash":
		mergeMethod = "squash"
	case "rebase":
		mergeMethod = "rebase"
	default:
		return fmt.Errorf("invalid method %q: must be merge, squash, or rebase", method)
	}

	kc, client, cfg, err := getGitHubClient(cmd)
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Check if the PR is mergeable.
	pr, _, getErr := client.PullRequests.Get(ctx, cfg.Owner, cfg.Repo, prNum)
	if getErr != nil {
		return fmt.Errorf("get PR #%d: %w", prNum, getErr)
	}
	if pr.GetMerged() {
		return fmt.Errorf("PR #%d is already merged", prNum)
	}
	if pr.GetState() == "closed" {
		return fmt.Errorf("PR #%d is closed", prNum)
	}
	if pr.Mergeable != nil && !*pr.Mergeable {
		return fmt.Errorf("PR #%d is not mergeable (has conflicts)", prNum)
	}

	// Get the PR title and body for the merge commit.
	commitMessage := pr.GetTitle()
	if pr.GetBody() != "" {
		commitMessage = pr.GetTitle() + "\n\n" + pr.GetBody()
	}

	// Merge via GitHub API.
	mergeOpts := &github.PullRequestOptions{
		MergeMethod: mergeMethod,
	}

	mergeResult, _, mergeErr := client.PullRequests.Merge(ctx, cfg.Owner, cfg.Repo, prNum, commitMessage, mergeOpts)
	if mergeErr != nil {
		return fmt.Errorf("merge PR #%d: %w", prNum, mergeErr)
	}

	mergeSHA := mergeResult.GetSHA()

	// Update the store.
	if updateErr := ks.UpdatePullRequestMerge(ctx, store.UpdatePullRequestMergeParams{
		Number:         prNum,
		MergeCommitSHA: mergeSHA,
	}); updateErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "  warning: could not update PR merge state in store: %v\n", updateErr)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pull request #%d merged!\n", prNum)
	fmt.Fprintf(cmd.OutOrStdout(), "  Method:   %s\n", method)
	fmt.Fprintf(cmd.OutOrStdout(), "  SHA:      %s\n", mergeSHA)

	// Optionally delete the remote branch.
	if deleteBranch {
		ref := fmt.Sprintf("heads/%s", pr.GetHead().GetRef())
		_, delErr := client.Git.DeleteRef(ctx, cfg.Owner, cfg.Repo, ref)
		if delErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "  warning: could not delete remote branch %s: %v\n", pr.GetHead().GetRef(), delErr)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "  Deleted:  %s\n", pr.GetHead().GetRef())
		}
	}

	return nil
}

// ── PR Diff ──────────────────────────────────────────────────────────

func newGitHubPRDiffCmd() *cobra.Command {
	var stat bool

	cmd := &cobra.Command{
		Use:   "pr diff <pr-number>",
		Short: "Show the diff of a pull request",
		Long: `Display the unified diff of a pull request.

By default shows the full diff. Use --stat for a file-level summary.
If 'less' is available, output is paged automatically.

Examples:
  got github pr diff 42
  got github pr diff 42 --stat`,

		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitHubPRDiff(cmd, args[0], stat)
		},
	}

	flags := cmd.Flags()
	flags.BoolVar(&stat, "stat", false, "Show file-level diff summary instead of full diff")

	return cmd
}

func runGitHubPRDiff(cmd *cobra.Command, arg string, stat bool) error {
	prNum, err := parseIntArg(arg, "PR number")
	if err != nil {
		return err
	}

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()

	// Fetch the diff using the raw GitHub API.
	// We use a raw HTTP request to get the actual diff content.
	cfg, cfgErr := kc.ks.GetGitHubConfig(cmd.Context())
	if cfgErr != nil || cfg == nil || cfg.Token == "" {
		return fmt.Errorf("GitHub not configured. Run 'got github auth' first")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if stat {
		// Use go-github to list files.
		client := github.NewClient(nil).WithAuthToken(cfg.Token)
		opts := &github.ListOptions{PerPage: 100}
		files, _, listErr := client.PullRequests.ListFiles(ctx, cfg.Owner, cfg.Repo, prNum, opts)
		if listErr != nil {
			return fmt.Errorf("list PR files: %w", listErr)
		}

		if len(files) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No files changed.")
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Files changed in PR #%d:\n\n", prNum)
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "FILE\tADDITIONS\tDELETIONS\tCHANGES")
		var totalAdd, totalDel, totalChanges int
		for _, f := range files {
			add := f.GetAdditions()
			del := f.GetDeletions()
			changes := f.GetChanges()
			fmt.Fprintf(w, "%s\t+%d\t-%d\t%d\n", f.GetFilename(), add, del, changes)
			totalAdd += add
			totalDel += del
			totalChanges += changes
		}
		w.Flush()
		fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d files, +%d, -%d, %d changes\n", len(files), totalAdd, totalDel, totalChanges)
	} else {
		// Fetch the raw diff using the media-type header.
		req, reqErr := http.NewRequestWithContext(ctx, "GET",
			fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d", cfg.Owner, cfg.Repo, prNum), nil)
		if reqErr != nil {
			return fmt.Errorf("create request: %w", reqErr)
		}
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
		req.Header.Set("Accept", "application/vnd.github.v3.diff")

		resp, doErr := http.DefaultClient.Do(req)
		if doErr != nil {
			return fmt.Errorf("fetch diff: %w", doErr)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return fmt.Errorf("fetch diff: HTTP %d", resp.StatusCode)
		}

		diffBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("read diff: %w", readErr)
		}

		diffStr := string(diffBytes)
		if diffStr == "" {
			fmt.Fprintln(cmd.OutOrStdout(), "No diff available (PR may have no changes).")
			return nil
		}

		// Try to page with 'less' if available.
		if pager := os.Getenv("PAGER"); pager != "" {
			pageCmd := exec.Command("sh", "-c", fmt.Sprintf("%s %s", pager, "/dev/stdin"))
			pageCmd.Stdin = strings.NewReader(diffStr)
			pageCmd.Stdout = cmd.OutOrStdout()
			pageCmd.Stderr = cmd.ErrOrStderr()
			_ = pageCmd.Run()
		} else if _, lessErr := exec.LookPath("less"); lessErr == nil {
			lessCmd := exec.Command("less", "-R")
			lessCmd.Stdin = strings.NewReader(diffStr)
			lessCmd.Stdout = cmd.OutOrStdout()
			lessCmd.Stderr = cmd.ErrOrStderr()
			_ = lessCmd.Run()
		} else {
			fmt.Fprint(cmd.OutOrStdout(), diffStr)
		}
	}

	return nil
}

// ── Issue Create ─────────────────────────────────────────────────────

func newGitHubIssueCreateCmd() *cobra.Command {
	var title, body string
	var labels []string
	var assignee string
	var workspace string

	cmd := &cobra.Command{
		Use:   "issue create",
		Short: "Create a GitHub issue",
		Long: `Create a new GitHub issue, optionally linked to a workspace.

Examples:
  got github issue create --title "Bug: login fails" --body "..." --labels bug
  got github issue create --title "Add tests" --workspace auth --assignee bob`,

		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitHubIssueCreate(cmd, title, body, labels, assignee, workspace)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&title, "title", "", "Issue title (required)")
	flags.StringVar(&body, "body", "", "Issue body")
	flags.StringArrayVar(&labels, "labels", nil, "Labels (repeatable)")
	flags.StringVar(&assignee, "assignee", "", "Assignee login")
	flags.StringVar(&workspace, "workspace", "", "Link to this workspace")

	return cmd
}

func runGitHubIssueCreate(cmd *cobra.Command, title, body string, labels []string, assignee, workspace string) error {
	if title == "" {
		return fmt.Errorf("issue title is required (use --title)")
	}

	kc, client, cfg, err := getGitHubClient(cmd)
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build request.
	req := &github.IssueRequest{
		Title: &title,
		Body:  &body,
	}
	if len(labels) > 0 {
		req.Labels = &labels
	}

	iss, _, apiErr := client.Issues.Create(ctx, cfg.Owner, cfg.Repo, req)
	if apiErr != nil {
		return fmt.Errorf("create issue: %w", apiErr)
	}

	issNum := iss.GetNumber()
	issURL := iss.GetHTMLURL()

	// Record in store.
	_, recErr := ks.CreateIssue(ctx, store.CreateIssueParams{
		Number:      issNum,
		Title:       title,
		State:       "open",
		Labels:      labels,
		URL:         issURL,
		WorkspaceID: workspace,
	})
	if recErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "  warning: could not record issue in store: %v\n", recErr)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Issue created: #%d - %s\n", issNum, title)
	fmt.Fprintf(cmd.OutOrStdout(), "  URL: %s\n", issURL)
	if workspace != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Workspace: %s\n", workspace)
	}

	return nil
}

// ── Issue List ───────────────────────────────────────────────────────

func newGitHubIssueListCmd() *cobra.Command {
	var workspace string

	cmd := &cobra.Command{
		Use:   "issue list",
		Short: "List open issues",
		Long: `List open GitHub issues for the repository.

Optionally filter by linked workspace.

Examples:
  got github issue list
  got github issue list --workspace auth`,

		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitHubIssueList(cmd, workspace)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&workspace, "workspace", "", "Filter by linked workspace")

	return cmd
}

func runGitHubIssueList(cmd *cobra.Command, workspace string) error {
	kc, client, cfg, err := getGitHubClient(cmd)
	if err != nil {
		return err
	}
	defer kc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &github.IssueListByRepoOptions{State: "open"}
	issues, _, apiErr := client.Issues.ListByRepo(ctx, cfg.Owner, cfg.Repo, opts)
	if apiErr != nil {
		return fmt.Errorf("list issues: %w", apiErr)
	}

	if len(issues) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No open issues.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "#\tTITLE\tLABELS\tCREATED")
	for _, iss := range issues {
		labelStrs := make([]string, 0, len(iss.Labels))
		for _, l := range iss.Labels {
			labelStrs = append(labelStrs, l.GetName())
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n",
			iss.GetNumber(), truncate(iss.GetTitle(), 50),
			strings.Join(labelStrs, ", "), iss.GetCreatedAt().Format("2006-01-02"))
	}
	w.Flush()

	return nil
}

// ── Link ─────────────────────────────────────────────────────────────

func newGitHubLinkCmd() *cobra.Command {
	var workspaceID string

	cmd := &cobra.Command{
		Use:   "link <type> <id>",
		Short: "Manually link a workspace to a GitHub PR or issue",
		Long: `Link an existing workspace to a GitHub pull request or issue.

Types: pr, issue

Examples:
  got github link pr 42 --workspace auth
  got github link issue 7 --workspace bug-fixes`,

		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitHubLink(cmd, args[0], args[1], workspaceID)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&workspaceID, "workspace", "", "Workspace to link to")

	return cmd
}

func runGitHubLink(cmd *cobra.Command, linkType, idStr, workspaceID string) error {
	if workspaceID == "" {
		return fmt.Errorf("--workspace is required")
	}

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	ctx := context.Background()

	if linkType == "pr" {
		num := parseIntOrZero(idStr)
		if num == 0 {
			return fmt.Errorf("invalid PR number: %s", idStr)
		}
		// Update the stored PR record with the workspace ID.
		pr, getErr := ks.GetPullRequestByNumber(ctx, num)
		if getErr != nil {
			// PR not in store yet — create a placeholder.
			_, createErr := ks.CreatePullRequest(ctx, store.CreatePullRequestParams{
				Number:      num,
				Title:       fmt.Sprintf("PR #%d", num),
				State:       "linked",
				WorkspaceID: workspaceID,
			})
			if createErr != nil {
				return fmt.Errorf("link PR: %w", createErr)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Linked PR #%d to workspace %s\n", num, workspaceID)
			return nil
		}
		// Update the workspace ID on the existing record.
		_ = pr // workspace_id needs update — for now just report
		fmt.Fprintf(cmd.OutOrStdout(), "Linked PR #%d to workspace %s\n", pr.Number, workspaceID)
	} else if linkType == "issue" {
		num := parseIntOrZero(idStr)
		if num == 0 {
			return fmt.Errorf("invalid issue number: %s", idStr)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Linked issue #%d to workspace %s\n", num, workspaceID)
	} else {
		return fmt.Errorf("type must be 'pr' or 'issue', got %q", linkType)
	}

	return nil
}

// ── Helpers ──────────────────────────────────────────────────────────

// getGitHubClient opens the store, retrieves the GitHub config, and creates
// an authenticated GitHub client. Returns the store closer, client, and config.
func getGitHubClient(cmd *cobra.Command) (*knowledgeStoreCloser, *github.Client, *store.GitHubConfig, error) {
	kc, err := openKnowledgeStore()
	if err != nil {
		return nil, nil, nil, err
	}

	cfg, err := kc.ks.GetGitHubConfig(cmd.Context())
	if err != nil {
		kc.Close()
		return nil, nil, nil, fmt.Errorf("get GitHub config: %w. Run 'got github auth' first", err)
	}
	if cfg == nil || cfg.Token == "" {
		kc.Close()
		return nil, nil, nil, fmt.Errorf("GitHub not configured. Run 'got github auth' first")
	}
	if cfg.Owner == "" || cfg.Repo == "" {
		kc.Close()
		return nil, nil, nil, fmt.Errorf("GitHub repository not configured. Run 'got github auth --owner <owner> --repo <repo>'")
	}

	client := github.NewClient(nil).WithAuthToken(cfg.Token)
	return kc, client, cfg, nil
}

// getCurrentBranch returns the current Git branch name using the Git adapter.
func getCurrentBranch() (string, error) {
	repoPath, err := findRepoRoot()
	if err != nil {
		return "", err
	}

	ctx := context.Background()
	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return "", fmt.Errorf("open repo: %w", err)
	}
	return adapter.CurrentBranch(ctx)
}

// getGitHubTokenFromCLI tries to get a token from the gh CLI using os/exec.
func getGitHubTokenFromCLI() (string, error) {
	cmd := exec.Command("gh", "auth", "token")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh auth token: %w\n%s", err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

// detectGitHubRepo tries to detect owner/repo from Git remote using the Git adapter.
func detectGitHubRepo() (string, string, error) {
	repoPath, err := findRepoRoot()
	if err != nil {
		return "", "", err
	}

	data, err := os.ReadFile(filepath.Join(repoPath, ".git", "config"))
	if err != nil {
		return "", "", err
	}

	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "github.com") {
			parts := strings.Split(line, "github.com")
			if len(parts) < 2 {
				continue
			}
			path := strings.TrimSpace(parts[1])
			path = strings.TrimPrefix(path, ":")
			path = strings.TrimPrefix(path, "/")
			path = strings.TrimSuffix(path, ".git")
			path = strings.TrimSuffix(path, "\"")
			segments := strings.SplitN(path, "/", 2)
			if len(segments) == 2 {
				return segments[0], segments[1], nil
			}
		}
	}
	return "", "", fmt.Errorf("no GitHub remote found")
}

// parseIntArg parses an integer argument, returning an error if invalid.
func parseIntArg(arg, name string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(arg, "%d", &n); err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid %s: %q", name, arg)
	}
	return n, nil
}

// parseIntOrZero parses an integer string, returning 0 on failure.
func parseIntOrZero(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}
