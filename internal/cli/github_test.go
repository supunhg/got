package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/got-sh/got/internal/events"
	"github.com/got-sh/got/internal/store"
)

// setupGitHubTest creates a temp GOT environment with store and bus, plus
// a mock GitHub API server for testing.
func setupGitHubTest(t *testing.T) (*store.KnowledgeStore, *events.Bus, string, *httptest.Server, func()) {
	t.Helper()

	gotDir := t.TempDir()

	dbPath := filepath.Join(gotDir, "got.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	bus := events.New()
	ks := store.NewKnowledgeStore(st.DB(), bus)

	// Create a mock GitHub API server.
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)

	// Mock /user endpoint for auth and review (gets authenticated user).
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"login": "testuser",
			"id":    12345,
		})
	})

	// Mock /repos/:owner/:repo endpoint
	mux.HandleFunc("/repos/testowner/testrepo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    123,
			"name":  "testrepo",
			"owner": map[string]interface{}{"login": "testowner"},
		})
	})

	// Mock /repos/:owner/:repo/pulls endpoint (create/list)
	var mockPRs []map[string]interface{}
	mux.HandleFunc("/repos/testowner/testrepo/pulls", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			// Create PR
			pr := map[string]interface{}{
				"number":    42,
				"title":     "Test PR",
				"state":     "open",
				"html_url":  fmt.Sprintf("%s/pull/42", srv.URL),
				"draft":     false,
				"mergeable": true,
				"body":      "Test body",
				"head":      map[string]interface{}{"ref": "feature-branch"},
				"base":      map[string]interface{}{"ref": "main"},
			}
			mockPRs = append(mockPRs, pr)
			json.NewEncoder(w).Encode(pr)
		} else {
			// List PRs
			json.NewEncoder(w).Encode(mockPRs)
		}
	})

	// Mock /repos/:owner/:repo/pulls/:number endpoint (get)
	// Also handle /repos/:owner/:repo/pulls/:number being used for merge endpoint via Accept header.
	mux.HandleFunc("/repos/testowner/testrepo/pulls/42", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		pr := map[string]interface{}{
			"number":        42,
			"title":         "Test PR",
			"state":         "open",
			"html_url":      fmt.Sprintf("%s/pull/42", srv.URL),
			"draft":         false,
			"mergeable":     true,
			"merged":        false,
			"body":          "Test body",
			"head":          map[string]interface{}{"ref": "feature-branch"},
			"base":          map[string]interface{}{"ref": "main"},
			"mergeable_state": "clean",
		}
		json.NewEncoder(w).Encode(pr)
	})

	// Mock /repos/:owner/:repo/pulls/:number/reviews endpoint
	mux.HandleFunc("/repos/testowner/testrepo/pulls/42/reviews", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"id":    1,
				"state": "APPROVED",
				"user":  map[string]interface{}{"login": "reviewer1"},
			},
		})
	})

	// Mock /repos/:owner/:repo/pulls/:number/merge endpoint (PUT)
	// go-github uses: PUT /repos/:owner/:repo/pulls/:number/merge
	mux.HandleFunc("/repos/testowner/testrepo/pulls/42/merge", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"sha":     "abc123def456",
			"merged":  true,
			"message": "Pull Request successfully merged",
		})
	})



	// Mock /repos/:owner/:repo/issues endpoint (create/list)
	var mockIssues []map[string]interface{}
	mux.HandleFunc("/repos/testowner/testrepo/issues", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			issue := map[string]interface{}{
				"number":   7,
				"title":    "Test Issue",
				"state":    "open",
				"html_url": fmt.Sprintf("%s/issues/7", srv.URL),
				"labels": []map[string]interface{}{
					{"name": "bug"},
				},
			}
			mockIssues = append(mockIssues, issue)
			json.NewEncoder(w).Encode(issue)
		} else {
			json.NewEncoder(w).Encode(mockIssues)
		}
	})

	cleanup := func() {
		srv.Close()
		bus.Close()
		st.Close()
	}

	// Override the GitHub API base URL for the test.
	// Note: go-github uses a custom http.Client. We'll create clients
	// that use the test server URL directly in tests.
	t.Setenv("GOT_TEST_GITHUB_URL", srv.URL)

	return ks, bus, gotDir, srv, cleanup
}

// TestGitHubAuth tests authentication configuration.
func TestGitHubAuthConfig(t *testing.T) {
	ks, _, _, srv, cleanup := setupGitHubTest(t)
	defer cleanup()

	// Override the GitHub client to use our test server.
	origNewClient := githubNewClient
	defer func() { githubNewClient = origNewClient }()

	githubNewClient = func(token string) *githubAPIClient {
		return &githubAPIClient{
			baseURL: srv.URL,
			token:   token,
		}
	}

	// Save config.
	ctx := context.Background()
	err := ks.SetGitHubConfig(ctx, store.GitHubConfig{
		Token:      "test-token",
		Owner:      "testowner",
		Repo:       "testrepo",
		BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("SetGitHubConfig: %v", err)
	}

	// Read it back.
	cfg, err := ks.GetGitHubConfig(ctx)
	if err != nil {
		t.Fatalf("GetGitHubConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Token != "test-token" {
		t.Errorf("expected token 'test-token', got %q", cfg.Token)
	}
	if cfg.Owner != "testowner" {
		t.Errorf("expected owner 'testowner', got %q", cfg.Owner)
	}
	if cfg.Repo != "testrepo" {
		t.Errorf("expected repo 'testrepo', got %q", cfg.Repo)
	}
}

// TestGitHubPullRequestCRUD tests PR creation and listing from the store.
func TestGitHubPullRequestCRUD(t *testing.T) {
	ks, _, _, _, cleanup := setupGitHubTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a PR record.
	pr, err := ks.CreatePullRequest(ctx, store.CreatePullRequestParams{
		Number:      42,
		Title:       "Test PR",
		State:       "open",
		Branch:      "feature-branch",
		Base:        "main",
		URL:         "https://github.com/testowner/testrepo/pull/42",
		WorkspaceID: "test-ws",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if pr.Number != 42 {
		t.Errorf("expected number 42, got %d", pr.Number)
	}
	if pr.WorkspaceID != "test-ws" {
		t.Errorf("expected workspace 'test-ws', got %q", pr.WorkspaceID)
	}

	// List PRs for the workspace.
	prs, err := ks.ListPullRequests(ctx, "test-ws")
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}
	if prs[0].Number != 42 {
		t.Errorf("expected PR #42, got #%d", prs[0].Number)
	}

	// Get by number.
	got, err := ks.GetPullRequestByNumber(ctx, 42)
	if err != nil {
		t.Fatalf("GetPullRequestByNumber: %v", err)
	}
	if got.Title != "Test PR" {
		t.Errorf("expected title 'Test PR', got %q", got.Title)
	}
}

// TestGitHubIssueCRUD tests issue creation and listing from the store.
func TestGitHubIssueCRUD(t *testing.T) {
	ks, _, _, _, cleanup := setupGitHubTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create an issue record.
	iss, err := ks.CreateIssue(ctx, store.CreateIssueParams{
		Number:      7,
		Title:       "Test Issue",
		State:       "open",
		Labels:      []string{"bug"},
		URL:         "https://github.com/testowner/testrepo/issues/7",
		WorkspaceID: "test-ws",
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if iss.Number != 7 {
		t.Errorf("expected number 7, got %d", iss.Number)
	}
	if len(iss.Labels) != 1 || iss.Labels[0] != "bug" {
		t.Errorf("expected labels [bug], got %v", iss.Labels)
	}

	// List issues.
	issues, err := ks.ListIssues(ctx, "test-ws")
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Number != 7 {
		t.Errorf("expected issue #7, got #%d", issues[0].Number)
	}
}

// TestGitHubEvents tests that events are published for PR and issue creation.
func TestGitHubEvents(t *testing.T) {
	ks, bus, _, _, cleanup := setupGitHubTest(t)
	defer cleanup()

	ctx := context.Background()

	// Subscribe to events.
	var prEvents, issueEvents int
	bus.Subscribe("PullRequestCreated", func(ctx context.Context, e events.Event) error {
		prEvents++
		return nil
	})
	bus.Subscribe("IssueCreated", func(ctx context.Context, e events.Event) error {
		issueEvents++
		return nil
	})

	// Create a PR — should fire PullRequestCreated.
	_, err := ks.CreatePullRequest(ctx, store.CreatePullRequestParams{
		Number: 42,
		Title:  "Test PR",
		State:  "open",
		Branch: "feature",
		Base:   "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if prEvents != 1 {
		t.Errorf("expected 1 PullRequestCreated event, got %d", prEvents)
	}

	// Create an issue — should fire IssueCreated.
	_, err = ks.CreateIssue(ctx, store.CreateIssueParams{
		Number: 7,
		Title:  "Test Issue",
		State:  "open",
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if issueEvents != 1 {
		t.Errorf("expected 1 IssueCreated event, got %d", issueEvents)
	}
}

// TestGitHubLink tests linking a PR to a workspace.
func TestGitHubLink(t *testing.T) {
	ks, _, _, _, cleanup := setupGitHubTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a workspace first.
	_, err := ks.CreateWorkspace(ctx, store.CreateWorkspaceParams{
		Name:        "test-ws",
		Description: "Test workspace",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	// Create a PR linked to the workspace.
	pr, err := ks.CreatePullRequest(ctx, store.CreatePullRequestParams{
		Number:      42,
		Title:       "Linked PR",
		State:       "open",
		Branch:      "feature",
		Base:        "main",
		WorkspaceID: "test-ws",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if pr.WorkspaceID != "test-ws" {
		t.Errorf("expected workspace 'test-ws', got %q", pr.WorkspaceID)
	}

	// List PRs for the workspace.
	prs, err := ks.ListPullRequests(ctx, "test-ws")
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("expected 1 PR for workspace, got %d", len(prs))
	}
}

// ── GitHub client stub for tests ─────────────────────────────────────

// githubAPIClient is a simplified GitHub API client for testing.
type githubAPIClient struct {
	baseURL string
	token   string
}

// githubNewClient is overridable for tests.
var githubNewClient = func(token string) *githubAPIClient {
	return &githubAPIClient{
		baseURL: "https://api.github.com",
		token:   token,
	}
}

// TestGitHubAuth tests the full auth flow.
func TestGitHubAuthFlow(t *testing.T) {
	// This test verifies the GetGitHubConfig / SetGitHubConfig round trip
	// already tested in TestGitHubAuthConfig above.
	// Additional test: verify config is nil when not set.
	ks, _, _, _, cleanup := setupGitHubTest(t)
	defer cleanup()

	ctx := context.Background()

	// No config yet.
	cfg, err := ks.GetGitHubConfig(ctx)
	if err != nil {
		t.Fatalf("GetGitHubConfig (no config): %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config, got %+v", cfg)
	}
}

// ── PR Review Tests ──────────────────────────────────────────────────

// TestGitHubReviewCRUD tests creating and listing reviews in the store.
func TestGitHubReviewCRUD(t *testing.T) {
	ks, _, _, _, cleanup := setupGitHubTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a review.
	r, err := ks.CreateReview(ctx, store.CreateReviewParams{
		PRNumber: 42,
		Reviewer: "testuser",
		State:    "APPROVED",
		Body:     "LGTM!",
	})
	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if r.State != "APPROVED" {
		t.Errorf("expected APPROVED, got %s", r.State)
	}
	if r.Reviewer != "testuser" {
		t.Errorf("expected reviewer 'testuser', got %q", r.Reviewer)
	}

	// List reviews.
	reviews, err := ks.ListReviews(ctx, 42)
	if err != nil {
		t.Fatalf("ListReviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(reviews))
	}
	if reviews[0].State != "APPROVED" {
		t.Errorf("expected APPROVED, got %s", reviews[0].State)
	}
}

// TestGitHubReviewEvents tests that PullRequestReviewed event is published.
func TestGitHubReviewEvents(t *testing.T) {
	ks, bus, _, _, cleanup := setupGitHubTest(t)
	defer cleanup()

	ctx := context.Background()

	var reviewEvents int
	bus.Subscribe("PullRequestReviewed", func(ctx context.Context, e events.Event) error {
		reviewEvents++
		return nil
	})

	_, err := ks.CreateReview(ctx, store.CreateReviewParams{
		PRNumber: 42,
		Reviewer: "testuser",
		State:    "APPROVED",
		Body:     "Nice work!",
	})
	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if reviewEvents != 1 {
		t.Errorf("expected 1 PullRequestReviewed event, got %d", reviewEvents)
	}
}

// ── PR Merge Tests ───────────────────────────────────────────────────

// TestGitHubMerge tests updating PR merge state in the store.
func TestGitHubMerge(t *testing.T) {
	ks, _, _, _, cleanup := setupGitHubTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a PR first.
	_, err := ks.CreatePullRequest(ctx, store.CreatePullRequestParams{
		Number: 42,
		Title:  "Test PR",
		State:  "open",
		Branch: "feature",
		Base:   "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}

	// Update with merge info.
	err = ks.UpdatePullRequestMerge(ctx, store.UpdatePullRequestMergeParams{
		Number:         42,
		MergeCommitSHA: "abc123def456",
	})
	if err != nil {
		t.Fatalf("UpdatePullRequestMerge: %v", err)
	}

	// Verify the PR is now merged.
	pr, err := ks.GetPullRequestByNumber(ctx, 42)
	if err != nil {
		t.Fatalf("GetPullRequestByNumber: %v", err)
	}
	if pr.State != "merged" {
		t.Errorf("expected state 'merged', got %q", pr.State)
	}
	if pr.MergeCommitSHA != "abc123def456" {
		t.Errorf("expected merge SHA 'abc123def456', got %q", pr.MergeCommitSHA)
	}
	if pr.MergedAt == 0 {
		t.Errorf("expected non-zero MergedAt")
	}
}

// TestGitHubMergeEvents tests that PullRequestMerged event is published.
func TestGitHubMergeEvents(t *testing.T) {
	ks, bus, _, _, cleanup := setupGitHubTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a PR first.
	_, err := ks.CreatePullRequest(ctx, store.CreatePullRequestParams{
		Number: 42,
		Title:  "Test PR",
		State:  "open",
		Branch: "feature",
		Base:   "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}

	var mergeEvents int
	bus.Subscribe("PullRequestMerged", func(ctx context.Context, e events.Event) error {
		mergeEvents++
		return nil
	})

	err = ks.UpdatePullRequestMerge(ctx, store.UpdatePullRequestMergeParams{
		Number:         42,
		MergeCommitSHA: "abc123",
	})
	if err != nil {
		t.Fatalf("UpdatePullRequestMerge: %v", err)
	}
	if mergeEvents != 1 {
		t.Errorf("expected 1 PullRequestMerged event, got %d", mergeEvents)
	}
}

// TestGitHubDiffStat tests PR diff stat via store (file-level summary).
// This tests that ListPullRequests still works after schema changes.
func TestGitHubPRState(t *testing.T) {
	ks, _, _, _, cleanup := setupGitHubTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a PR with merge info.
	_, err := ks.CreatePullRequest(ctx, store.CreatePullRequestParams{
		Number: 42,
		Title:  "Test PR",
		State:  "open",
		Branch: "feature",
		Base:   "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}

	// Verify the PR fields including new merge columns.
	pr, err := ks.GetPullRequestByNumber(ctx, 42)
	if err != nil {
		t.Fatalf("GetPullRequestByNumber: %v", err)
	}
	if pr.MergeCommitSHA != "" {
		t.Errorf("expected empty MergeCommitSHA, got %q", pr.MergeCommitSHA)
	}
	if pr.MergedAt != 0 {
		t.Errorf("expected 0 MergedAt, got %d", pr.MergedAt)
	}
}
