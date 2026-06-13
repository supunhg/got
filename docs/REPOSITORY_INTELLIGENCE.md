# Repository Intelligence Subsystem

This document describes GOT's **Repository Intelligence** subsystem, the
implementation of the **Repository Analyzer** and **Health Engine** modules
from `ARCHITECTURE.md`. It is the binding reference for the two new
subcommands, the packages that power them, and the plugin extension
contract that lets third-party detectors contribute to the model.

The subsystem is **read-only and offline-only**. It never modifies the
working tree, `.git/`, or `.got/`, and it never makes a network call. All
data is derived from local files and the local Git history.

---

## Module Map

```
internal/analyzer/        Repository Analyzer
  analyzer.go             RepositoryModel + Analyzer struct
  walk.go                 filesystem walker, path resolution, helpers
  analyze.go              Analyze() entry point + Detector merging
  detector.go             Detector interface + DetectionContext
  language.go             language detection (extension map + counting)
  framework.go            framework detection (manifest-based + config fallback)
  packagemanager.go       package manager detection
  cicd.go                 CI/CD system detection
  container.go            Dockerfile / Compose / K8s / Helm
  monorepo.go             monorepo detection
  repotype.go             repository type classification
  stats.go                git-derived statistics
  json_helpers.go         shared json wrapper
  analyzer_test.go        unit tests
  json_helpers_test.go    shared json test wrapper

internal/health/          Health Engine
  health.go               HealthReport + Checker
  branches.go             stale / merged / excessive branch checks
  remotes.go              unused / malformed remote checks
  files.go                large binary detection
  docs.go                 missing documentation
  cleanliness.go          working-tree state
  health_test.go          unit tests

internal/cli/
  inspect.go              `got inspect` command
  health.go               `got health` command
  format_bytes.go         shared byte-formatting helper

cmd/got/main.go           (no change — uses cli.Execute)
```

---

## `RepositoryModel` (analyzer)

`RepositoryModel` is the single, JSON-serializable struct that
`analyzer.Analyzer.Analyze(ctx)` returns. Every field is computed in a
single call; nothing is cached or persisted.

| Field             | Type                | Source                                |
|-------------------|---------------------|---------------------------------------|
| `Path`            | `string`            | work tree passed to `New`             |
| `Name`            | `string`            | `got.yml` project.name, else dir name |
| `Description`     | `string`            | `got.yml` project.description         |
| `DefaultBranch`   | `string`            | `got.yml` project.default_branch      |
| `Languages`       | `[]LanguageStat`    | extension map + line/byte counts      |
| `Frameworks`      | `[]Framework`       | manifest deps + config fallback       |
| `PackageManagers` | `[]PackageManager`  | manifest + lockfile presence          |
| `CICDSystems`     | `[]CICDSystem`      | config dir + filename rules           |
| `Containerization`| `Containerization`  | Dockerfile/Compose/K8s/Helm detection|
| `Monorepo`        | `MonorepoInfo`      | workspaces / pnpm-workspace / etc.    |
| `Type`            | `RepositoryType`    | rule-based classifier                 |
| `TypeReason`      | `string`            | one-line rationale for `Type`         |
| `Stats`           | `RepositoryStats`   | git adapter + filesystem walk         |
| `DetectedAt`      | `time.Time`         | UTC timestamp of the analysis         |

### Detection Pipeline

`Analyze` runs the detectors in this fixed order, so users can predict
where a custom detector's output lands:

1. **Walk** the work tree once (skips `.git`, `node_modules`, `vendor`,
   `target`, `dist`, `build`, `out`, `__pycache__`, `venv`, `.venv`,
   `coverage`, `.idea`, `.vscode`, `Godeps`, `_obj`, `_test`, `bin`,
   `pkg`).
2. **Identity**: read `got.yml` (when present) for project name and
   default branch; fall back to the work tree's directory name and
   `"main"`.
3. **Languages** (extension map + per-file line/byte counting; binary
   files contribute bytes but not lines).
4. **Frameworks** (high-confidence: package.json / pyproject.toml /
   go.mod / Cargo.toml / Gemfile / composer.json / pom.xml /
   build.gradle / pubspec.yaml; low-confidence: config-file presence
   like `next.config.js`).
5. **Package managers** (manifest + matching lockfile).
6. **CI/CD** (filename + directory patterns: `.github/workflows/*`,
   `.gitlab-ci.yml`, `.circleci/config.yml`, `Jenkinsfile`, etc.).
7. **Containerization** (Dockerfile variants, docker-compose files,
   K8s manifests in `k8s/`, `kubernetes/`, `manifests/`, `kube/`;
   Helm charts via `Chart.yaml`).
8. **Monorepo** (explicit tools: pnpm workspaces, lerna, nx,
   turborepo, go.work, Bazel; implicit: 2+ top-level subdirs with
   their own manifest).
9. **Type classification** (rules-based: monorepo orchestrator → docs
   → config-only → tool → library → application → unknown).
10. **Statistics** (git adapter: commit count, branch count,
    contributor count, first/last commit times; filesystem walk: file
    count, line count, size).
11. **User detectors** (see `Detector` below).

### Plugin Extension: `Detector`

```go
type Detector interface {
    Name() string
    Detect(ctx context.Context, dc DetectionContext) ([]DetectedItem, error)
}

type DetectedItem struct {
    Kind      DetectionKind  // language, framework, package-manager, cicd,
                             // monorepo, repository-type, custom
    Name      string
    Category  string
    Language  string
    Version   string
    Evidence  []string  // populated into ConfigFiles on the model
    Confidence string
}
```

`DetectionContext` gives the detector the work tree path, the
pre-walked file list, the list of skipped directories, and a logger
surface. Detectors can read files via `dc.ReadFile(path)` rather than
walking the work tree a second time.

Detectors are added via `analyzer.NewWithDetectors(workTree, adapter, []Detector{...})`.
The CLI does not yet plumb detectors from `internal/plugin`; the
extension point is in place for a follow-up milestone.

`KindCustom` items are intentionally dropped at the model layer
(`applyDetectedItems`). The intent is for plugins to refine
core signals (overriding `Type`, adding a missing framework, etc.)
rather than to surface arbitrary new categories. Custom signals
are easy to plumb through if/when the v0.2+ plugin surface needs
them.

### Repository Type Classification

The classifier walks the rules in priority order:

| Priority | Match                                                | `Type`            |
|----------|------------------------------------------------------|-------------------|
| 1        | Monorepo orchestrator (`Bazel`, `Nx`, `Turborepo`, `Lerna`, `pnpm/npm workspaces`) | `monorepo`        |
| 2        | Doc site config (mkdocs, docusaurus, hugo, jekyll, sphinx, mdbook, vitepress) | `documentation`   |
| 2        | Only Markdown / reST / AsciiDoc, no source code      | `documentation`   |
| 3        | Only Terraform / Ansible / K8s / Helm, no source      | `config`          |
| 4        | CLI entry point: Go `cmd/<x>/main.go` or `main.go`, package.json `bin`, setup.py `console_scripts`, `[[bin]]` in Cargo.toml | `tool` |
| 5        | Library shape: `go.mod` without main, package.json `main`/`module`/`exports`, Cargo.toml `[lib]`, `.gemspec` | `library` |
| 6        | Has source code, none of the above                    | `application`     |
| 7        | No source code detected                              | `unknown`         |

---

## `HealthReport` (health)

`HealthReport` is the JSON-serializable output of `health.Checker.Check(ctx)`.
The checker runs every check independently; a single check failure does
not abort the others. Errors are converted to `Severity=Info` findings
so the report is still usable when a check cannot run.

| Field             | Type            | Description                              |
|-------------------|-----------------|------------------------------------------|
| `RepositoryPath`  | `string`        | work tree path                           |
| `GeneratedAt`     | `time.Time`     | UTC timestamp                            |
| `Score`           | `int` (0-100)   | overall health score                     |
| `Grade`           | `string`        | `A+` (perfect), `A`/`B`/`C`/`D`/`F`      |
| `Findings`        | `[]HealthFinding` | sorted by severity desc, then ID asc   |
| `Recommendations` | `[]Recommendation` | sorted by Priority asc, then Title asc |
| `Counts`          | `HealthCounts`  | per-severity tally                       |
| `ChecksRun`       | `[]string`      | IDs of the checks that ran               |

### Health Checks

| ID                       | Category       | Severity (default) | Trigger                                     |
|--------------------------|----------------|--------------------|---------------------------------------------|
| `stale-branches`         | `branches`     | low/medium/high    | branch not committed to in `StaleDays` (default 180) |
| `merged-branches`        | `branches`     | low/medium/high    | branch tip reachable from default branch    |
| `excessive-branches`     | `branches`     | low/medium/high    | more than `MaxBranches` (default 50) local branches |
| `unreachable-remotes`    | `remotes`      | low/medium/high    | remote has no tracking branches             |
| `malformed-remote-urls`  | `remotes`      | high               | URL fails `looksLikeURL`                    |
| `large-binaries`         | `files`        | low/medium/high    | file with binary ext + size > 1 MiB         |
| `missing-readme`         | `documentation`| critical           | no `README*` at the work tree root          |
| `missing-license`        | `documentation`| high               | no `LICENSE*` at the root                   |
| `missing-changelog`      | `documentation`| medium             | no `CHANGELOG*` / `HISTORY*` / `NEWS*`      |
| `missing-contributing`   | `documentation`| low                | no `CONTRIBUTING*` at the root              |
| `working-tree-dirty`     | `cleanliness`  | low                | uncommitted staged or unstaged changes     |
| `untracked-files`        | `cleanliness`  | low/medium/high    | 1+ / 20+ / 50+ untracked files             |

### Score / Grade Algorithm

`Score` starts at 100 and deducts per finding:

| Severity  | Deduction |
|-----------|-----------|
| critical  | 25        |
| high      | 10        |
| medium    | 4         |
| low       | 1         |
| info      | 0         |

The grade is then:

| Score     | Grade |
|-----------|-------|
| 100 (no findings) | A+ |
| ≥ 90      | A     |
| ≥ 80      | B     |
| ≥ 70      | C     |
| ≥ 60      | D     |
| < 60      | F     |

`--min-severity info|low|medium|high|critical` filters the findings
*after* they are produced, so a `medium`-filtered report still has the
correctly-scored baseline (it just doesn't show low/info findings).

### Thresholds

All knobs are in `health.Thresholds`:

| Field                       | Default   | Meaning                                |
|-----------------------------|-----------|----------------------------------------|
| `StaleDays`                 | 180       | days without a commit before "stale"   |
| `MaxBranches`               | 50        | local branch count to trigger "excessive" |
| `LargeBinaryBytes`          | 1 MiB     | size threshold for "large binary"      |
| `MaxLargeBinaries.Low`      | 1         | count threshold for low severity       |
| `MaxLargeBinaries.Medium`   | 3         | count threshold for medium severity    |
| `MaxLargeBinaries.High`     | 10        | count threshold for high severity      |

The health engine does **not** currently read `got.yml`. The
"default branch" lookup walks the local branch list and picks
`main` / `master` / `trunk` / `develop` / `default`, falling back to
`"main"`. A future v0.2+ change can plumb the project config through
`health.New(...)` for projects whose default branch is non-canonical.

---

## CLI Surface

### `got inspect [--json] [--no-header]`

```
got inspect
```

Runs `analyzer.Analyzer.Analyze` and prints the model as a multi-section
human-readable report. With `--json`, emits a `inspectReport` wrapper
containing the GOT version, generation time, and the full model as
indented JSON suitable for piping to `jq`.

Section order: header → languages → frameworks → package managers →
CI/CD → containerization → monorepo → type → statistics.

### `got health [--json] [--no-header] [--min-severity S]`

```
got health
got health --min-severity medium
got health --json | jq '.report.score'
```

Runs `health.Checker.Check` and prints a header (path, score, grade,
severity counts), a findings table grouped by severity, and a
prioritized recommendation list. `--json` emits a `healthReportJSON`
wrapper; `--min-severity` filters findings below the given severity
without re-running the checks.

### Why Two Commands?

`got inspect` answers "what is this repo?" (the static facts).
`got health` answers "is this repo in good shape?" (the dynamic
smells). The two use different caches, different scoring, and
different update cadences — bundling them would make the output
harder to read in CI logs and harder to consume from scripts.

---

## Testing

The analyzer and health packages are tested with **in-memory fakes**
of the `git.Adapter` interface. The fakes return canned branches,
remotes, status, and NDJSON-encoded log streams; tests build the
work tree on disk with `t.TempDir()` and feed both the fake
adapter and the analyzer's walker the same paths. No real `git`
invocation is needed.

Coverage (as of this writing):

- `internal/analyzer`: empty repo, Go library, JS monorepo, Python
  project, Docker/Compose/K8s/Helm, CI/CD, type classification
  (tool / library / docs / config), git-derived stats, custom
  `Detector` extension, skipped directories.
- `internal/health`: stale branches, excessive branches, missing
  docs (per-file), large binaries, large binaries skip
  `node_modules`, unreachable remotes, malformed remote URLs,
  working-tree dirty / untracked, recommendations for findings,
  perfect score (no findings), score-from-findings math.

The CLI commands have their own test coverage in `internal/cli/`;
`got status`-style snapshot tests for the human-readable output
land in a follow-up.

---

## Out of Scope (for v0.1)

This subsystem does **not** implement, and the user spec explicitly
excludes:

- **Snapshots** (recovery points) — handled by the snapshot engine in v0.2.
- **GitHub / GitLab / Bitbucket integration** — the plugin API exists
  in `internal/plugin`; live invocation lands in v0.5.
- **Pull-request workflow** (`got pr create`, `got pr review`) —
  plugin-driven, lands in v0.5+.
- **AI suggestions** — the user spec explicitly says "no AI".
  Existing heuristics in `commitwiz` and elsewhere stay.

These omissions are intentional. The repository intelligence
subsystem's value is in giving the user **information Git itself
cannot provide** — language breakdowns, framework detection, branch
hygiene, missing documentation — without any external dependency.
