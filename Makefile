# GOT — Git-native developer operating layer
#


SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

# ---- Version stamping ----------------------------------------------------
# These are injected into the binary via -ldflags at build time. The
# Makefile prefers values derived from git so a `git describe` build is
# reproducible, and falls back to "dev"/"none" for source tarball builds.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

GO      ?= go
BIN     := bin/got
LDFLAGS := -s -w \
           -X github.com/supunhg/got/internal/version.Version=$(VERSION) \
           -X github.com/supunhg/got/internal/version.Commit=$(COMMIT) \
           -X github.com/supunhg/got/internal/version.Date=$(DATE)

# ---- Targets ------------------------------------------------------------
.PHONY: help build run smoke test test-race lint fmt fmt-check vet tidy clean ci check-paths

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Compile the got binary to bin/got
	@mkdir -p bin
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/got

run: build ## Build and run got (with no args; prints help)
	./$(BIN)

smoke: build ## Build and run got --help as a smoke test
	./$(BIN) --help

test: ## Run unit tests
	$(GO) test ./...

test-race: ## Run unit tests with -race
	$(GO) test -race ./...

lint: ## Run golangci-lint
	golangci-lint run

fmt: ## Run gofumpt -w on all .go files
	gofumpt -w .

# CI-friendly format check: fails if gofumpt would rewrite any file.
fmt-check:
	@bad=$$(gofumpt -l -d . 2>/dev/null | grep -v '^$$' || true); \
	if [ -n "$$bad" ]; then \
		echo "files need gofumpt reformatting:"; \
		echo "$$bad"; \
		exit 1; \
	fi
	@echo "fmt-check: ok"

vet: ## Run go vet
	$(GO) vet ./...

tidy: ## Run go mod tidy
	$(GO) mod tidy

clean: ## Remove build artifacts
	rm -rf bin

# Local CI gate. Mirrors what .github/workflows/ci.yml will run on every PR.
# See got-spec.md §18 for the canonical ordering.
ci: fmt-check lint vet test check-paths

# Locks in the supunhg/got module path. Fails if any stray "org" placeholder
# (written with angle brackets, as in the spec) remains in code, YAML, or
# shell. Documentation (markdown) is excluded — it uses the same notation
# as a documented rename example. See got-spec.md §23.
check-paths:
	@bad=$$(grep -rnE '<'"'"'?org'"'"'?>' \
		--include='*.go' --include='*.yml' --include='*.yaml' \
		--include='*.sh' --include='Makefile' --include='*.mk' \
		. 2>/dev/null || true); \
	if [ -n "$$bad" ]; then \
		echo "stray org placeholders found:"; \
		echo "$$bad"; \
		exit 1; \
	fi
	@echo "check-paths: ok"
