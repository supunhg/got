# GOT — Specification (v0.1)

> Source-of-truth design document for the initial build of GOT. All decisions below
> have been confirmed with the user. Sections marked **[v0.2+]** are explicitly
> out of scope for this milestone but are documented so the architecture leaves
> room for them.

---

## 1. Vision (unchanged from `ARCHITECTURE.md`)

GOT is a Git-native developer operating layer. It does not replace Git; it
enhances Git with workflow abstraction, safety mechanisms, repository
intelligence, team knowledge management, and interactive developer
experiences. Git remains the source of truth and GOT metadata is isolated
under `.got/`. A repository must remain 100% usable without GOT.

### Core principles (binding)

1. Git is never modified in a way the user didn't ask for.
2. GOT metadata is isolated to `.got/`.
3. Repository remains usable without GOT.
4. Every operation must be recoverable.
5. Offline-first.
6. Plugin-first architecture (external binaries).

---

## 2. Scope of v0.1

Five features, scaffolded to a functional bar:

| # | Feature             | Status in v0.1                                          |
|---|---------------------|---------------------------------------------------------|
| 1 | Git Adapter         | Full: status, commit, branches, remotes, checkout, merge, reset, fetch, push |
| 2 | Init Wizard         | Interactive TUI with adaptive defaults; writes `.got/`, `got.yml`, `.gitignore` entries |
| 3 | Commit Wizard       | Interactive TUI; Conventional Commits enforced; heuristic type/scope suggestions |
| 4 | Branch Graph        | Terminal ASCII via `git log --graph` + Lip Gloss styling; also exports Graphviz DOT |
| 5 | Remote Manager      | Plain CLI (`got remote list/add/remove/fetch/push`)     |

Out of scope for v0.1 (deferred to later versions):

- Snapshot engine → v0.2 (DB schema is forward-compatible, but no UI)
- Repository analyzer, health engine → v0.3
- Workspaces, knowledge engine, ADRs → v0.4
- Plugin loader runtime checks (interface defined, no live loader) → v1.0

---

## 3. Tech stack

| Layer        | Choice                                                   |
|--------------|----------------------------------------------------------|
| Language     | Go 1.22+                                                 |
| Go module    | `github.com/supunhg/got` (replaceable via `find`/`sed` at scaffold time) |
| License      | MIT (see `LICENSE`)                                     |
| CLI router   | `github.com/spf13/cobra` + `pflag`                        |
| TUI          | `github.com/charmbracelet/bubbletea` + `lipgloss` + `bubbles` |
| Config       | `github.com/spf13/viper` (YAML), `got.yml` at repo root  |
| Storage      | `modernc.org/sqlite` (pure Go, no CGo) — single file `.got/got.db` |
| Migrations   | `github.com/golang-migrate/migrate/v4` (with `sqlite` driver) |
| Git plumbing | `os/exec` to `git` (parse porcelain/v2 + pretty formats); no `libgit2` |
| Logging      | `log/slog` (stdlib) with `slog.NewTextHandler(os.Stderr)` |
| Errors       | Wrapped errors via `fmt.Errorf("...: %w", err)` + a `gerr` package for typed user errors |
| Testing      | `testing` (stdlib) + `github.com/rogpeppe/go-internal/testscript` + `github.com/stretchr/testify` for unit assertions |
| Lint/format  | `golangci-lint` (revive, govet, staticcheck, gocritic, misspell) and `gofumpt` |
| CI           | GitHub Actions (test matrix: Go 1.22 on macOS-latest + ubuntu-latest) |
| Release      | GoReleaser producing macOS (arm64, amd64) + Linux (amd64, arm64) tarballs + Homebrew tap |
| Plugin model | External executables on `PATH` or `.got/plugins/`; invoked via `os/exec`; communicate over a documented NDJSON JSON-RPC-ish protocol (see §10) |

### Why these choices

- **Bubbletea** is the only mature, cross-platform Go TUI framework with Elm-style state and rich component library (`bubbles`).
- **`modernc.org/sqlite`** keeps the binary CGo-free, simplifying Homebrew + cross-compilation.
- **External plugin binaries** avoid Go's `plugin` package (Linux/macOS only, fragile reload semantics) and let plugin authors use any language.
- **MIT license** matches the de-facto Go-dev-tool standard (`gh`, `lazygit`, `fzf`, `ripgrep`, `delta`, `fd`). Apache-2.0 was considered for the explicit patent grant, but the shorter, more permissive MIT is preferred for a single-vendor project where the patent-grant advantage doesn't yet apply. If a corporate contributor joins before v1.0, this can be revisited.
- **Module path `github.com/supunhg/got`** is a working default. The `got-sh/` prefix is generic enough to remain valid if the project moves to its own org later, and the whole path is one `find ... | xargs sed` away from being swapped to `github.com/<real-org>/got` and `supunhg/homebrew-tap` → `<real-org>/homebrew-tap`. The CI workflows and `.goreleaser.yml` use the same path so a single substitution updates everything.

---

## 4. Repository layout

```
got/
├── ARCHITECTURE.md              # existing
├── README.md                    # quickstart
├── LICENSE                      # MIT
├── got-spec.md                  # this file
├── go.mod
├── go.sum
├── Makefile                     # build, test, lint, release
├── .golangci.yml
├── .goreleaser.yml
├── .github/
│   └── workflows/
│       ├── ci.yml
│       └── release.yml
├── cmd/
│   └── got/
│       └── main.go              # entrypoint
├── internal/
│   ├── cli/                     # cobra command tree
│   │   ├── root.go
│   │   ├── init.go
│   │   ├── commit.go
│   │   ├── status.go
│   │   ├── branch.go
│   │   ├── remote.go
│   │   ├── graph.go
│   │   ├── tui.go
│   │   └── plugin.go
│   ├── git/                     # Git adapter (exec git)
│   │   ├── adapter.go           # GitAdapter interface + impl
│   │   ├── status.go
│   │   ├── commit.go
│   │   ├── branch.go
│   │   ├── remote.go
│   │   ├── reflog.go
│   │   └── testdata/
│   ├── store/                   # SQLite layer
│   │   ├── store.go             # Open(), Close(), Migrate()
│   │   ├── migrations/          # *.sql files embedded via embed.FS
│   │   ├── snapshots.go         # schema present, no API in v0.1
│   │   ├── decisions.go         # stub
│   │   ├── workspaces.go        # stub
│   │   └── health.go            # stub
│   ├── config/                  # got.yml + .got/config.yaml
│   │   ├── config.go
│   │   └── defaults.go
│   ├── repo/                    # repository discovery + .got/ management
│   │   ├── discover.go          # walks up to find .git
│   │   ├── layout.go            # ensures .got/ exists, returns paths
│   │   └── exclude.go           # writes .gitignore entries
│   ├── initwiz/                 # init wizard TUI
│   │   ├── model.go
│   │   └── wizard.go
│   ├── commitwiz/               # commit wizard TUI
│   │   ├── model.go
│   │   ├── suggest.go           # heuristic type/scope suggestions
│   │   └── conventional.go      # Conventional Commits validation
│   ├── graph/                   # branch graph
│   │   ├── render.go            # ASCII via git log --graph + Lip Gloss
│   │   └── dot.go               # Graphviz DOT exporter
│   ├── plugin/                  # external plugin discovery
│   │   ├── discover.go
│   │   ├── protocol.go          # NDJSON message types
│   │   └── manager.go
│   ├── tui/                     # shared TUI framework
│   │   ├── theme.go             # Lip Gloss styles
│   │   └── components/          # reusable bubbles
│   └── version/
│       └── version.go           # injected via -ldflags
├── testdata/
│   └── testscript/              # *.txtar golden files
├── docs/
│   ├── commands.md              # auto-generated (cobra doc)
│   ├── plugin-author-guide.md
│   └── architecture-decisions/
│       ├── 0001-go-and-bubbletea.md
│       ├── 0002-sqlite-via-modernc.md
│       └── 0003-external-binary-plugins.md
└── scripts/
    └── install.sh
```

**Hidden directory at repo root** (`.got/`) is the in-repo storage location. A
GOT-managed repo's `.gitignore` will contain a single line `.got/` so Git status
stays clean. We will **not** write to `.git/info/exclude` — that file is local
and would not propagate to teammates.

---

## 5. The `.got/` directory (per-repo)

Created by `got init`. Layout matches `ARCHITECTURE.md` exactly:

```
.got/
├── config.yaml          # per-repo GOT settings (commits style, theme, plugins)
├── got.db               # SQLite (WAL mode)
├── snapshots/           # [v0.2] placeholder dir, created empty
├── workspaces/          # [v0.4] placeholder dir
├── decisions/           # [v0.4] ADRs (markdown) — empty in v0.1
├── health/              # [v0.3] derived health reports
├── cache/               # derived/indexed data (graph cache, etc.)
└── plugins/             # user-local plugin binaries
```

`.got/config.yaml` is the user-facing config (committed with the repo).
`got.yml` at the repo root is a thin pointer that names the project, declares
the commit style, and lists enabled plugins. Example:

```yaml
# got.yml
version: 1
project:
  name: got
  default_branch: main
commits:
  style: conventional
  scopes: [cli, git, store, tui, graph, plugin, docs, build]
  allow_breaking: true
plugins:
  enabled: []           # populated as users opt-in
ai:
  provider: heuristic   # only option in v0.1
```

---

## 6. Git adapter

A single `git.GitAdapter` interface in `internal/git/adapter.go`. All other
modules depend on this interface, never on `os/exec` directly. This makes the
adapter mockable and lets us add a `libgit2` impl later without touching
callers.

```go
type GitAdapter interface {
    Status(ctx context.Context) (Status, error)
    Commit(ctx context.Context, msg string, opts CommitOpts) (SHA, error)
    Branches(ctx context.Context) ([]Branch, error)
    Remotes(ctx context.Context) ([]Remote, error)
    Checkout(ctx context.Context, ref string, opts CheckoutOpts) error
    Merge(ctx context.Context, ref string, opts MergeOpts) error
    Reset(ctx context.Context, target string, mode ResetMode) error
    Fetch(ctx context.Context, remote string) error
    Push(ctx context.Context, remote, branch string, opts PushOpts) error
    Log(ctx context.Context, rangeStr string, format LogFormat) (io.Reader, error)
    CurrentRef(ctx context.Context) (string, error)
}
```

Implementation: `ExecAdapter` shells out to `git`, parses porcelain v2 for
status and pretty format for log. Captures both stdout and stderr; returns
typed errors (`*gerr.GitError`) for known non-zero exit codes. Cancellation
respects `ctx.Done()` by killing the child process group.

**Non-Git directory behavior:** All `got` commands except `got init` and
`got version` refuse to run with a clear error if `.git/` is not found in the
current or any parent directory. Error reads:

```
got: not inside a Git repository (no .git found in this directory or any parent).
  To start a new repository:  git init && got init
  To navigate to one:          cd <path>
```

---

## 7. Init wizard

**Trigger:** `got init` (or `got init --here` from inside a Git repo, or `got init --path <dir>`).

**Flow (TUI, Bubbletea):**

1. **Detected values screen** — show auto-detected project name (dir basename),
   primary branch (`git symbolic-ref --short HEAD` or default to `main`), and
   languages/frameworks (best-effort by file presence; v0.1 only shows them in
   a "Detected" panel, doesn't gate anything).
2. **Commit style** — radio: Conventional Commits (default) / Free-form /
   Custom template (path input).
3. **Plugin prompts** — v0.1 ships with no bundled plugins; the wizard shows
   an empty list with "Skip" as the only default action. (Seeded for v1.0.)
4. **Confirm & write** — preview of files to be created. On confirm:
   - Create `.got/` with all subdirectories.
   - Write `.got/config.yaml` from the wizard's answers.
   - Write `got.yml` at the repo root.
   - Append `.got/` to `.gitignore` if not already present (idempotent).
   - Run SQLite migrations → empty `.got/got.db`.
5. **Success screen** — "Initialized GOT in <path>. Next: try `got status`."

**Adaptive defaults** (when the user just hits Enter repeatedly):

| Setting         | Default                                              |
|-----------------|------------------------------------------------------|
| project.name    | directory basename                                   |
| default_branch  | `main` (or detected)                                 |
| commits.style   | `conventional`                                       |
| commits.scopes  | `[]` (user adds)                                     |
| plugins.enabled | `[]`                                                 |
| ai.provider     | `heuristic`                                          |

**Re-running `got init`**: refuses with a clear error and a `--force` flag
that re-runs the wizard and overwrites `.got/config.yaml` (preserving the
SQLite DB). `got.yml` is always overwritten after confirm.

---

## 8. Commit wizard

**Trigger:** `got commit` (alias `got ci`).

**Flow (TUI, Bubbletea):**

1. **Stage review** — show `git status --porcelain` grouped (Staged / Unstaged
   / Untracked). Multi-select to toggle staging (`git add` / `git reset`).
   `got commit -a` to skip the review and stage all tracked.
2. **Type** — radio: `feat`, `fix`, `chore`, `docs`, `style`, `refactor`,
   `perf`, `test`, `build`, `ci`, `revert`. Default pre-selected by
   heuristic.
3. **Scope** — optional. Free text or pick from `got.yml`'s `commits.scopes`.
4. **Subject** — single line, ≤72 chars, imperative mood, no trailing period.
5. **Body** — optional, wrapped at 72.
6. **Breaking change footer** — checkbox; appends `BREAKING CHANGE: <reason>`.
7. **Confirm** — preview the rendered message. `Enter` commits (`git commit
   -m <rendered>` or `-F -` for multi-line).

**Heuristic type/scope suggestion** (`internal/commitwiz/suggest.go`):

- File path patterns → scope. `internal/cli/*.go` → `cli`, `docs/**` →
  `docs`, `*.test.go` → `test`, `*.md` → `docs`, `Makefile` / `*.yml` /
  `*.yaml` / `go.mod` / `go.sum` → `build`.
- Diff size patterns → type. Tests added only → `test`. Docs only → `docs`.
  Build files only → `build`. A rename/rewrite that touches many files → `refactor`.
- Confidence score 0..1; the suggestion is shown as "Suggested: feat (cli)
  [84%]" with a one-key accept.

**No LLM in v0.1.** A `SuggestionProvider` interface allows an LLM-backed
implementation in a later version without touching the wizard.

**Validation** (`internal/commitwiz/conventional.go`): reuses
`github.com/go-playground/validator` rules. Failure shows a yellow warning
banner but allows commit with `--no-verify`.

---

## 9. Branch graph

**Trigger:** `got graph` (no args) or `got graph --dot > out.dot` then `dot -Tsvg out.dot`.

**Terminal renderer (`internal/graph/render.go`):**

- Runs `git log --graph --oneline --decorate --all -n 200` (configurable via
  `got graph -n 500`).
- Parses the `git log --graph` output (the leading `|`, `\`, `/`, `*`,
  `_`, `=` glyphs) into a sequence of cells.
- Applies Lip Gloss styles: HEAD = bold green, current branch = blue, remote
  branches = magenta, tags = yellow.
- Pager: `bubbles/viewport` with `/` to search by message, `n/N` to jump
  between matches, `q` to quit, `j/k` or arrows to scroll.
- `--since`, `--until`, `--author`, `--grep` filter flags pass through to
  `git log`.

**DOT exporter (`internal/graph/dot.go`):**

- Walks `git log --pretty='%H %P'` and builds a directed graph (commits as
  nodes, parents as edges). Decorations (branches, tags, HEAD) become DOT
  attributes (color, label).
- Output to stdout or file. The user runs `dot` themselves; we don't ship it.

**In-repo cache:** Graph layout for terminal view is **not** cached in v0.1;
it's cheap to re-render. Graph cache file
(`.got/cache/graph.json`) is reserved for v0.3 health engine.

---

## 10. Remote manager

**Trigger:** `got remote` (no args → `list`).

| Command                        | Description                                                                 |
|--------------------------------|-----------------------------------------------------------------------------|
| `got remote list`              | Table: name, URL, fetch/push spec, last fetch time (from reflog).           |
| `got remote add <name> <url>`  | Wraps `git remote add`. Validates URL is parseable.                         |
| `got remote remove <name>`     | Refuses if remote has any tracking branch unless `--force`.                 |
| `got remote rename <old> <new>`| Wraps `git remote rename`.                                                  |
| `got remote set-url <name> <u>`| Wraps `git remote set-url`. Validates reachability with a dry-run fetch if `--check`. |
| `got remote fetch [name]`      | Wraps `git fetch`. `--prune` and `--all` pass through.                      |
| `got remote push <name> <br>`  | Wraps `git push`. **Refuses non-fast-forward without `--force-with-lease`** and emits a snapshot-prompt in v0.2. |
| `got remote prune <name>`      | Wraps `git remote prune`.                                                  |

Output: plain CLI, styled with `lipgloss` (cyan name, dim URL, yellow
warning for stale remotes detected via reflog). All commands support
`--json` for machine-readable output.

---

## 11. Plugin system (v0.1: interface + discovery only)

**Discovery** (`internal/plugin/discover.go`):

1. Walk `PATH` for executables prefixed `got-` (e.g. `got-github`).
2. Walk `.got/plugins/` (per-repo, not committed by default).
3. Run each with `--got-plugin-manifest` and parse a JSON manifest (full
   schema below; `manifest_version` is required):
   ```json
   {
     "manifest_version": 1,
     "name": "github",
     "version": "1.2.0",
     "min_got": "0.1.0",
     "commands": [
       { "name": "pr", "description": "Open a GitHub PR", "args": [...] }
     ]
   }
   ```
4. Register discovered commands into the Cobra tree as `got <plugin-name>
   <command>` (e.g. `got github pr`).

**Manifest versioning (locked in for v0.1):** The manifest is shipped with
`manifest_version: 1` from day one, and GOT's plugin loader refuses to load
any manifest with a `manifest_version` it doesn't recognize. This makes the
contract between GOT and plugin authors stable: plugins targeting v0.1.x
will continue to load through the v0.1 minor series. Breaking manifest
changes will require bumping the integer. The manifest schema:

```json
{
  "manifest_version": 1,
  "name": "github",
  "version": "1.2.0",
  "min_got": "0.1.0",
  "commands": [
    { "name": "pr", "description": "Open a GitHub PR", "args": [...] }
  ]
}
```

`min_got` is a semver string; GOT refuses to load a plugin that requires a
newer `min_got` than the running binary.

**Invocation** (`internal/plugin/manager.go`):

- Plugin is launched as a subprocess with `os/exec`.
- GOT writes the command args as NDJSON to the plugin's stdin, one JSON
  object per line; the plugin writes NDJSON responses to stdout.
- A simple message protocol:
  - Request: `{"type":"call","id":"...","command":"pr","args":{"title":"..."}}`
  - Response: `{"type":"result","id":"...","ok":true,"data":{...}}` or
    `{"type":"error","id":"...","code":"...","message":"..."}`.
  - Streaming (TUI updates) uses `{"type":"event","id":"...","event":"...","payload":{...}}`.
- 30-second default timeout, configurable via `--plugin-timeout`.
- Plugin stdout/stderr are surfaced in the parent GOT process; `--quiet`
  suppresses non-error plugin output.

**v0.1 ships zero plugins.** The interface, discovery, and protocol are
fully implemented so plugin authors can start building immediately.

---

## 12. Data model (SQLite, `modernc.org/sqlite`)

**File:** `.got/got.db` (WAL mode, `synchronous=NORMAL`).

**Migrations** live in `internal/store/migrations/` as `0001_init.sql`,
`0002_snapshots.sql`, etc., embedded via `//go:embed`. Migrations are
applied in order at `store.Open()`.

**v0.1 tables (forward-looking, most rows stay empty until later versions):**

```sql
-- 0001_init.sql

CREATE TABLE meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
-- rows: schema_version, got_version, init_at, init_user

CREATE TABLE snapshots (        -- USED IN v0.2, schema here for forward compat
  id          TEXT PRIMARY KEY,        -- ULID
  created_at  INTEGER NOT NULL,        -- unix ms
  reason      TEXT NOT NULL,           -- e.g. "before-reset", "before-rebase"
  ref         TEXT NOT NULL,           -- "refs/heads/feature/x" or "detached@abc123"
  reflog_sel  TEXT,                    -- optional reflog selector
  stash_ref   TEXT,                    -- optional git stash ref
  metadata    TEXT NOT NULL DEFAULT '{}'  -- JSON
);
CREATE INDEX snapshots_created_at_idx ON snapshots(created_at DESC);

CREATE TABLE decisions (        -- USED IN v0.4
  id         TEXT PRIMARY KEY,
  created_at INTEGER NOT NULL,
  status     TEXT NOT NULL,           -- proposed|accepted|rejected|superseded
  title      TEXT NOT NULL,
  body_path  TEXT NOT NULL            -- relative to .got/decisions/
);

CREATE TABLE workspaces (       -- USED IN v0.4
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  created_at  INTEGER NOT NULL,
  state       TEXT NOT NULL DEFAULT 'open'  -- open|closed
);
CREATE TABLE workspace_files (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  path         TEXT NOT NULL,
  PRIMARY KEY (workspace_id, path)
);

CREATE TABLE health_runs (      -- USED IN v0.3
  id         TEXT PRIMARY KEY,
  run_at     INTEGER NOT NULL,
  report     TEXT NOT NULL           -- JSON
);

CREATE TABLE cache_kv (         -- generic key/value cache
  key        TEXT PRIMARY KEY,
  value      BLOB NOT NULL,
  updated_at INTEGER NOT NULL
);
```

**Why this shape:** every table that v0.1 doesn't fully use is created in
v0.1 with its final schema, so no migration churn in v0.2/v0.3/v0.4. Code
that doesn't have an interface yet simply has no callers.

---

## 13. CLI surface (Cobra)

Top-level:

```
got --version
got --help
got completion {bash|zsh|fish|powershell}
```

Subcommands:

```
got init [path] [flags]
  --here                 use current directory
  --name <string>        override project name
  --branch <string>      override default branch
  --style <conventional|freeform|custom>
  --custom-template <path>
  --force                overwrite existing .got/config.yaml
  --no-interactive       use defaults, do not prompt

got status [path] [flags]
  --short                porcelain output
  --json                 machine-readable

got commit [flags]
  -a, --all              stage all tracked files first
  -m, --message <string> skip wizard, use message (still validated)
  --no-verify            skip Conventional Commits validation
  --amend                amend previous commit

got branch [flags]
  (no args)              list local branches
  -a, --all              include remote
  -r, --remotes          remote only
  -d, --delete <name>    delete branch
  -m, --move <old> <new> rename branch
  --json

got remote <subcommand> [args]   # see §10

got graph [flags]
  -n, --max-count <int>  default 200
  --since <date>
  --until <date>
  --author <pattern>
  --grep <pattern>
  --dot                  emit Graphviz DOT instead of TUI

got plugin <subcommand>
  list                   list discovered plugins
  info <name>            show manifest
  install <source>       install from local path or git URL  [v0.1: stubbed]

got tui                   open the dashboard TUI (placeholder in v0.1)
```

**Global flags:**

```
--cwd <path>       operate on a different directory
--no-color         disable Lip Gloss styles
--no-tui           force plain CLI output even in wizards (CI-friendly)
--log-level <lvl>  debug|info|warn|error (default: warn)
--plugin-timeout  default 30s
```

---

## 14. TUI framework

- Single shared `internal/tui/theme.go` exporting Lip Gloss styles.
- Wizard TUIs use the `bubbles` library (textinput, textarea, list, spinner,
  help).
- Keybindings follow Bubbletea conventions (`ctrl+c` to quit, `?` for help,
  `tab`/`shift+tab` for navigation, `enter` to confirm, `esc` to cancel).

### `got tui` dashboard scope in v0.1 (locked in)

The dashboard is a real v0.1 deliverable, **not** a stripped placeholder. It
ships with these tabs, top to bottom in the tab bar:

| Tab        | Behavior in v0.1 | Backed by                                            |
|------------|------------------|------------------------------------------------------|
| Status     | **Real**         | `git.GitAdapter.Status` + `bubbles/list`             |
| Branches   | **Real**         | `git.GitAdapter.Branches` + `bubbles/table`         |
| Remotes    | Read-only        | `git.GitAdapter.Remotes` + a "Coming in v0.2" banner; mutations are CLI-only |
| Graph      | Read-only preview| A 20-line preview of `got graph`'s ASCII output + a "Coming in v0.2" banner; the real interactive renderer ships in v0.2 |
| Plugins    | Read-only        | `internal/plugin/discover` showing whatever is on `PATH`, plus a "Coming in v0.2" banner |

The two **real** tabs (Status, Branches) use the same `tea.Model` plumbing
as the wizards, prove the Bubbletea integration, and give the README a
screenshot-worthy artifact on day one. The three **read-only** tabs
(Remotes, Graph, Plugins) render a visible "Coming in v0.2" banner plus a
small live panel backed by the real `git.GitAdapter` /
`internal/plugin/discover` calls so the dashboard doesn't feel empty and
the adapter integration is exercised end-to-end on day one. This is a
deliberate choice: a fully-stripped dashboard would force users to
evaluate the TUI only through the wizards, and the read-only remotes/
graph/plugins panels lay the groundwork for v0.2 mutations without
throwing away code.

---

## 15. Error handling

- A small `internal/gerr` package defines typed user-facing errors:
  `NotInGitRepo`, `NotInitialized`, `PermissionDenied`, `GitError`,
  `ValidationError`, `PluginError`. Each implements `Error() string` and
  `UserMessage() string` (the latter is friendly and free of jargon).
- All CLI commands map errors to a single exit-code scheme:
  - `0` success
  - `1` generic failure
  - `2` usage error
  - `3` not in a Git repo
  - `4` GOT not initialized (ran a command requiring `.got/` without running `got init`)
  - `5` validation error (e.g. bad commit message)
  - `64` plugin error (subtract command-specific codes later)
- `--log-level debug` shows full stack + raw `git` stderr.

---

## 16. Logging

- `log/slog` writing JSON or text to stderr based on `--log-level`.
- `debug`: full command list, raw git invocations and exit codes.
- `info`: high-level operation started/finished.
- `warn`: recoverable issues (e.g. remote not reachable on fetch).
- `error`: failures.
- Default in interactive mode: `warn`. Default in non-interactive mode
  (`--no-tui` or non-TTY): `info`. The TUI never logs to stderr; it uses an
  in-app log view reachable via `ctrl+l`.

---

## 17. Testing strategy

| Test type        | Tool                                             | Where                                  |
|------------------|--------------------------------------------------|----------------------------------------|
| Unit             | `testing` + `testify/assert`                     | `*_test.go` next to code               |
| End-to-end CLI   | `testscript`                                     | `testdata/testscript/*.txtar`          |
| Golden files     | `testscript` + `go-internal/txtar-arena`         | `testdata/golden/*.txtar`              |
| Adapter mocks    | in-package `fakeAdapter` for unit tests          | `internal/git/adapter_test.go`         |
| TUI snapshot     | `github.com/charmbracelet/x/exp/teatest`         | `internal/{initwiz,commitwiz}/*_test.go` |

**Coverage targets:** `go test -cover` should report ≥ 80% for `internal/git`,
`internal/store`, `internal/repo`, `internal/config`. ≥ 60% for the rest.

**testscript scenarios committed in v0.1:**

1. `got init` in a fresh repo, then `got status` works.
2. `got init` outside a Git repo fails clearly.
3. `got commit -m "feat: add foo"` succeeds; bad subject fails with
   validation banner.
4. `got branch -d` of the current branch refuses.
5. `got remote add` + `got remote list --json` roundtrip.
6. `got graph --dot` produces valid DOT.
7. Re-running `got init` without `--force` refuses.
8. `got commit` with no staged files prompts to stage.

Each scenario uses a real `git` binary and a fresh tempdir created via
`mktemp` inside the txtar setup.

---

## 18. CI/CD

**`.github/workflows/ci.yml`** runs on every PR and push to `main`:

- `setup-go@v5` with Go 1.22.
- `make ci` which runs: `go mod download`, `gofumpt -l -d`, `golangci-lint run`,
  `go test ./...`, `go test -tags=e2e ./testdata/testscript/...`.
- Matrix: `[ubuntu-latest, macos-latest]`. Windows intentionally absent in
  v0.1.

**`.github/workflows/release.yml`** triggered by `v*` tags:

- Runs GoReleaser with `.goreleaser.yml`.
- Produces: tar.gz + zip for darwin/{amd64,arm64}, linux/{amd64,arm64}.
- Signs with cosign (keyless via OIDC) and generates SBOMs (syft).
- Pushes a Homebrew formula to `got/homebrew-tap` (a separate repo the
  user must create, or we publish to `homebrew-core` in a later milestone).

**`.goreleaser.yml`** snapshot (with the locked-in module path):

```yaml
project_name: got
builds:
  - main: ./cmd/got
    binary: got
    env: [CGO_ENABLED=0]
    goos: [darwin, linux]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w
      - -X github.com/supunhg/got/internal/version.Version={{.Version}}
      - -X github.com/supunhg/got/internal/version.Commit={{.ShortCommit}}
brews:
  - repository:
      owner: got-sh
      name: homebrew-tap
    homepage: https://github.com/supunhg/got
    description: "Git-native developer operating layer"
    install: |
      bin.install "got"
```

If the module path is ever renamed (e.g. to `github.com/<real-org>/got`),
update three places in one PR: `go.mod`, `.goreleaser.yml`, and the
`README.md` install snippet. A `make check-paths` target greps for
`supunhg/got` in the repo and fails CI if any stray reference remains.

---

## 19. Performance targets

- Cold start (`got --version`): < 50 ms on M1, < 100 ms on a CI Linux box.
- `got status`: < 200 ms on a 10k-file repo.
- `got graph -n 200`: < 500 ms including `git log` call.
- `got commit` (no edits): < 600 ms from Enter on confirm to Git SHA printed.
- Binary size (stripped): < 25 MB.

These are measured in `make bench` (a tiny bench binary) and reported on
PRs touching performance-sensitive paths.

---

## 20. Documentation

- `LICENSE`: full MIT license text at the repo root.
- `README.md`: install (`brew install supunhg/homebrew-tap/got`),
  quickstart (3 commands), links to docs.
- `docs/commands.md`: auto-generated by `cobra/doc`.
- `docs/plugin-author-guide.md`: how to write a plugin, the NDJSON
  protocol, the manifest format, an example plugin in Go.
- `docs/architecture-decisions/000{1,2,3}.md`: the three locked-in
  decisions (Go + Bubbletea, modernc.org/sqlite, external binary
  plugins) with rationale and tradeoffs.
- `CONTRIBUTING.md`: dev setup (`go version`, `task install-deps`),
  branching model, PR rules.
- All examples run in CI as doctests via `testscript` (the
  `docs/examples/*.txtar` files).

---

## 21. Out of scope (locked out for v0.1)

- Snapshot creation UI, safe reset, safe rebase, recovery manager. (DB
  schema present, no API.)
- Health engine, repository analyzer, repository dashboard (real).
- Workspaces, knowledge engine, ADRs.
- Any LLM call.
- Real plugin loader (interface defined, discovery stubbed, runtime
  calls return "plugin not found" until v0.5 / v1.0).
- Windows support.
- Worktree sub-commands (v0.5).
- Submodule sub-commands (v0.5).

---

## 22. Resolved decisions and remaining open questions

### Resolved in v0.1 (locked in)

The following four open questions from earlier drafts have been resolved
and are now binding:

1. **Module path / Homebrew tap** — *Resolved.* Go module is
   `github.com/supunhg/got`; Homebrew tap is `supunhg/homebrew-tap`; the
   GitHub repo URL is `https://github.com/supunhg/got`. See §3 and §18.
   Trivially renamed via `find ... | xargs sed` if the project moves
   orgs.
2. **License** — *Resolved.* MIT, matching the de-facto Go-dev-tool
   standard (`gh`, `lazygit`, `fzf`, `ripgrep`). See §3 and §20.
3. **Plugin manifest versioning** — *Resolved.* Manifests ship with
   `manifest_version: 1` from v0.1.0. GOT refuses manifests with any
   `manifest_version` it does not recognize, and plugins declare a
   `min_got` semver that GOT enforces. Bumping the integer is required
   for any breaking manifest change. See §11.
4. **TUI dashboard scope** — *Resolved.* The `got tui` dashboard ships in
   v0.1 with **Status** and **Branches** as real tabs and
   **Remotes / Graph / Plugins** as visible placeholders backed by
   real read-only data (so the layout, Bubbletea plumbing, and
   `git.GitAdapter` integration are all exercised on day one). See §14.

### Still open

These do not block v0.1 but should be revisited in a future milestone:

5. **Conventional Commits strictness** — should we accept merge commits,
   revert commits, and empty commits as exceptions to validation?
6. **`got graph` for huge repos** — should the v0.1 terminal renderer
   virtualize the list (only render visible rows) to handle 10k+ commits
   smoothly?
7. **Telemetry / crash reports** — none in v0.1, but worth a deliberate
   "no, never" decision to keep the offline-first principle binding.
8. **Repository templates** — does `got init` need a `--template <url>`
   flag to bootstrap from a known good `got.yml`? (Defer to v0.5.)

---

## 23. Definition of done for v0.1

The release is shippable when **all** of the following are true:

- [ ] All five v0.1 features (§2) work end-to-end on macOS and Linux.
- [ ] `make ci` is green on both platforms.
- [ ] `golangci-lint run` is clean with the configured linters.
- [ ] `go test -race ./...` passes.
- [ ] Test coverage targets in §17 are met or exceeded.
- [ ] `got init`, `got status`, `got commit`, `got branch`, `got remote`,
       `got graph`, `got plugin list` all have help text and examples.
- [ ] `README.md`, `LICENSE`, and `docs/plugin-author-guide.md` are
       written and reviewed.
- [ ] GoReleaser produces a working binary for all four targets
       (darwin/{amd64,arm64}, linux/{amd64,arm64}).
- [ ] The Homebrew tap installs the binary and `got --version` works.
- [ ] No LLM, no telemetry, no network calls except user-initiated
       `git fetch`/`git push` and Go module downloads at build time.
- [ ] Three ADRs (0001, 0002, 0003) are merged.
- [ ] `make check-paths` confirms no stray `<org>` placeholders remain
       in code, YAML, or shell scripts (locks in the `supunhg/got` module
       path). The check explicitly excludes markdown documentation where
       `<org>` appears as an intentional rename example.

---

## 24. Top-level file summary (what to scaffold first)

When implementation begins, the recommended order is:

1. `go.mod` (module `github.com/supunhg/got`), `LICENSE` (MIT),
   `cmd/got/main.go`, `internal/cli/root.go`, `internal/version` →
   `make build` produces a runnable `got` with `--version` and `--help`.
2. `internal/repo` + `internal/git` (adapter + tests) + `internal/config` →
   `got status` and `got branch` work in a real Git repo.
3. `internal/store` (migrations + open) →
   `got init` writes `.got/got.db`.
4. `internal/initwiz` + commit `init` command →
   full init flow.
5. `internal/commitwiz` + heuristic suggest + `got commit`.
6. `internal/graph/render.go` + `internal/graph/dot.go` + `got graph`.
7. `internal/cli/remote.go` + table output + JSON flag.
8. `internal/plugin` (interface, discovery, manifest, NDJSON protocol).
9. TUI dashboard placeholder.
10. CI, GoReleaser, Homebrew tap.

---

*End of spec. See `docs/architecture-decisions/` (to be created) for the
ADRs that lock in the major technical choices.*
