# GitHub Integration (Built-in Plugin)

## Overview

GOT's GitHub integration connects local development to GitHub — managing pull
requests, issues, and their relationships to workspaces, decisions, and notes
directly from the CLI. It is built as a "built-in plugin" (distributed with GOT
itself rather than installed via `got plugin install`) using the same event-driven
patterns established by the Plugin Runtime v2.

The integration is designed as a reference for future platform plugins (GitLab,
Bitbucket, etc.) and focuses on pragmatic PR/issue CRUD + linking to GOT's
knowledge model.

---

## CLI Commands

All commands live under `got github`:

| Command | Description |
|---------|-------------|
| `got github auth` | Store GitHub token (PAT) and repo info. Tries `gh auth token` if --token omitted. Validates via API. |
| `got github pr create --title` | Create PR from current branch. Auto-includes workspace/decision/note references in body. Links to workspace. |
| `got github pr list` | List open PRs. Filter by `--branch`, `--workspace`. |
| `got github pr status <number>` | Detailed PR status: title, mergeable, reviews (per-user), checks. |
| `got github issue create --title` | Create issue. Supports `--labels`, `--assignee`, `--workspace`. |
| `got github issue list` | List open issues. Filter by `--workspace`. |
| `got github link pr|issue <number>` | Manually link a workspace to a PR or issue. |

### Command patterns

- All commands check for authentication before any API call (`getGitHubClient`).
- Network calls have 30-second timeouts (configurable via `--plugin-timeout`).
- Network errors are reported but never crash GOT.
- Workspace linking is first-class: PR bodies include references to linked
  decisions and notes automatically.

---

## Data Model

### `github_config` table (singleton row)

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT | `'default'` singleton |
| `token` | TEXT | GitHub personal access token (plaintext, stored in .got/) |
| `owner` | TEXT | Repository owner (user or org) |
| `repo` | TEXT | Repository name |
| `base_branch` | TEXT | Default target branch for PRs (`main`) |
| `updated_at` | INTEGER | Unix ms |

### `pull_requests` table

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT (ULID) | Primary key |
| `number` | INTEGER (UNIQUE) | GitHub PR number |
| `title` | TEXT | PR title |
| `state` | TEXT | `open`, `closed`, `merged` |
| `branch` | TEXT | Head branch name |
| `base` | TEXT | Target branch name |
| `url` | TEXT | GitHub HTML URL |
| `workspace_id` | TEXT | Optional link to workspaces.name |
| `created_at` / `updated_at` | INTEGER | Unix ms |

### `issues` table

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT (ULID) | Primary key |
| `number` | INTEGER (UNIQUE) | GitHub issue number |
| `title` | TEXT | Issue title |
| `state` | TEXT | `open`, `closed` |
| `labels` | TEXT | JSON array of label names |
| `url` | TEXT | GitHub HTML URL |
| `workspace_id` | TEXT | Optional link to workspaces.name |
| `created_at` / `updated_at` | INTEGER | Unix ms |

---

## Event Integration

Two new event types are published via the shared event bus:

| Event | Constant | Payload | When |
|-------|----------|---------|------|
| `PullRequestCreated` | `events.EventPullRequestCreated` | `PullRequestCreatedPayload` | After a PR is recorded in the store |
| `IssueCreated` | `events.EventIssueCreated` | `IssueCreatedPayload` | After an issue is recorded |

These events are published by `KnowledgeStore.CreatePullRequest()` and
`KnowledgeStore.CreateIssue()` respectively. Plugins and other in-process
consumers can subscribe to them.

---

## Authentication Flow

1. User runs `got github auth --token <pat>` (or without `--token` to attempt
   `gh auth token` from GitHub CLI)
2. Token is validated by calling `GET /user` on the GitHub API
3. Repository (owner/repo) is validated by calling `GET /repos/:owner/:repo`
4. If owner/repo not provided, GOT detects from the Git remote URL
5. Config is stored in the `github_config` table

---

## Code Structure

| File | Purpose |
|------|---------|
| `internal/cli/github.go` | All CLI command implementations (auth, PR, issue, link) |
| `internal/cli/github_test.go` | Tests with store-level mock server (6 tests) |
| `internal/store/knowledge.go` | PullRequest, Issue, GitHubConfig types + CRUD methods |
| `internal/store/migrations/0008_github.sql` | Schema for github_config, pull_requests, issues |
| `internal/events/event.go` | PullRequestCreatedPayload, IssueCreatedPayload |

---

## Testing

The test file covers:

- **`TestGitHubAuthConfig`**: Save/load GitHub configuration round-trip
- **`TestGitHubPullRequestCRUD`**: Create PR, list by workspace, get by number
- **`TestGitHubIssueCRUD`**: Create issue, list by workspace, verify labels
- **`TestGitHubEvents`**: Verify `PullRequestCreated` and `IssueCreated` events fire
- **`TestGitHubLink`**: Create PR linked to workspace, verify workspace filter
- **`TestGitHubAuthFlow`**: Verify nil config before any auth

Tests use `httptest` for mock HTTP setup and test directly through the store
layer, avoiding real network calls.

---

## Future Plans

- CI/CD status checks in `got github pr status`
- PR review submission (approve/request changes)
- Webhook server for real-time event updates from GitHub
- GitHub Issues project/timeline integration
- GitLab/Bitbucket platform plugins following the same pattern
