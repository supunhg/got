# Contributing to GOT

Thank you for your interest in contributing to GOT! This document explains how to get started.

## Development Setup

### Prerequisites

- **Go 1.25+** — [Download](https://go.dev/dl/)
- **Git** — any recent version
- **[golangci-lint](https://golangci-lint.run/)** — for linting (optional but recommended)

### Getting Started

```bash
# Clone the repo
git clone https://github.com/supunhg/got.git
cd got

# Build
make build

# Run tests
make test

# Run the full CI gate locally
make ci
```

### Project Structure

```
cmd/got/           Entrypoint
internal/
├── cli/           Cobra command tree
├── git/           Git adapter (os/exec)
├── store/         SQLite storage
├── events/        Event bus
└── version/       Build version info
testdata/          Test fixtures and sample plugins
docs/              Architecture and design docs
```

## How to Contribute

### Reporting Bugs

Open an issue with:
- A clear description of the problem
- Steps to reproduce
- Expected vs. actual behavior
- `got version` output and OS/Go version

### Suggesting Features

Open an issue with:
- The problem you're trying to solve
- Your proposed solution
- Any alternatives you considered

### Submitting Changes

1. **Fork** the repository
2. **Create a branch** from `main`:
   ```bash
   git checkout -b feat/my-feature
   ```
3. **Make your changes** following the code conventions below
4. **Add tests** for new functionality
5. **Run the CI gate**:
   ```bash
   make ci
   ```
6. **Commit** with a descriptive message:
   ```bash
   git commit -m "feat: add my new feature"
   ```
7. **Push** and open a Pull Request

### Commit Messages

GOT uses [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add new feature
fix: resolve bug in workspace sync
docs: update architecture diagram
test: add tests for health command
refactor: simplify event bus subscription
chore: update dependencies
```

## Code Conventions

- **Formatting**: `gofumpt` (not `gofmt`). Run `make fmt`.
- **Linting**: `golangci-lint` with the config in `.golangci.yml`. Run `make lint`.
- **Testing**: Standard `testing` package. Run `make test-race` to check for race conditions.
- **Error handling**: Wrap errors with `fmt.Errorf("context: %w", err)`.
- **No CGo**: The project uses `modernc.org/sqlite` (pure Go). Don't introduce CGo dependencies.
- **Cobra-only in `internal/cli/`**: Domain packages (`git/`, `store/`, `events/`) should not depend on Cobra.
- **Event-driven**: New features should publish events on the event bus so plugins can react.

### Adding a New Command

1. Create `internal/cli/mycommand.go`
2. Define `newMyCmd() *cobra.Command`
3. Register it in `internal/cli/root.go` via `cmd.AddCommand(newMyCmd())`
4. Add tests in `internal/cli/mycommand_test.go`

### Adding a Migration

1. Create `internal/store/migrations/NNNN_description.sql`
2. The migration is auto-discovered and applied in filename order at startup
3. Run `make test` to verify

## Testing

```bash
make test           # unit tests
make test-race      # with race detector
go test -run TestName ./internal/cli/...  # run a specific test
```

Tests use real Git repositories created in temp directories. No mocking framework is needed for most tests.

## Questions?

Open a discussion on [GitHub Discussions](https://github.com/supunhg/got/discussions) or comment on an existing issue.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
