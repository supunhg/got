// Copyright 2026 The GOT Authors. MIT License.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/supunhg/got/internal/git"
	"github.com/supunhg/got/internal/store"
)

// healthResult holds the outcome of a single health check.
type healthResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "ok", "warn", "fail"
	Message string `json:"message,omitempty"`
}

// newHealthCmd returns the `got health` subcommand.
func newHealthCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Run pre-flight health checks",
		Long: `Run quick health checks on the GOT environment.

Checks that:
  - .got/ directory exists and has the expected structure
  - SQLite database is present and readable
  - All migrations have been applied
  - Git repository is accessible
  - Workspace references are consistent with the repository

This is useful for diagnosing issues and preparing for safe operations.

Examples:
  got health
  got health --json`,

		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHealth(cmd, jsonOut)
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func runHealth(cmd *cobra.Command, jsonOut bool) error {
	var results []healthResult

	// 1. Check .got/ directory.
	gotDir, err := findGotDir()
	if err != nil {
		results = append(results, healthResult{
			Name:    "got-directory",
			Status:  "fail",
			Message: ".got/ directory not found — run 'got init' first",
		})
		return outputHealthResults(cmd, results, jsonOut)
	}
	results = append(results, healthResult{
		Name:    "got-directory",
		Status:  "ok",
		Message: fmt.Sprintf("found at %s", gotDir),
	})

	// 2. Check expected subdirectories.
	expectedDirs := []string{"decisions", "snapshots", "workspaces", "health", "cache", "plugins"}
	for _, d := range expectedDirs {
		dirPath := filepath.Join(gotDir, d)
		if info, statErr := os.Stat(dirPath); statErr != nil || !info.IsDir() {
			results = append(results, healthResult{
				Name:    fmt.Sprintf("directory/%s", d),
				Status:  "warn",
				Message: fmt.Sprintf("missing subdirectory: %s/", d),
			})
		}
	}

	// 3. Check SQLite database.
	dbPath := filepath.Join(gotDir, "got.db")
	if _, statErr := os.Stat(dbPath); statErr != nil {
		results = append(results, healthResult{
			Name:    "database",
			Status:  "fail",
			Message: "got.db not found",
		})
		return outputHealthResults(cmd, results, jsonOut)
	}

	s, err := store.Open(dbPath)
	if err != nil {
		results = append(results, healthResult{
			Name:    "database",
			Status:  "fail",
			Message: fmt.Sprintf("cannot open database: %v", err),
		})
		return outputHealthResults(cmd, results, jsonOut)
	}
	defer s.Close()

	// Verify we can query the database.
	var tableCount int
	if err := s.DB().QueryRow(
		"SELECT COUNT(*) FROM schema_migrations",
	).Scan(&tableCount); err != nil {
		results = append(results, healthResult{
			Name:    "database",
			Status:  "warn",
			Message: fmt.Sprintf("schema_migrations table not readable: %v", err),
		})
	} else {
		results = append(results, healthResult{
			Name:    "database",
			Status:  "ok",
			Message: fmt.Sprintf("readable, %d migrations applied", tableCount),
		})
	}

	// 4. Check Git repository.
	repoPath, err := findRepoRoot()
	if err != nil {
		results = append(results, healthResult{
			Name:    "git-repository",
			Status:  "fail",
			Message: "not inside a Git repository",
		})
		return outputHealthResults(cmd, results, jsonOut)
	}
	results = append(results, healthResult{
		Name:    "git-repository",
		Status:  "ok",
		Message: fmt.Sprintf("root at %s", repoPath),
	})

	// 5. Check Git adapter works.
	ctx := context.Background()
	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		results = append(results, healthResult{
			Name:    "git-adapter",
			Status:  "fail",
			Message: fmt.Sprintf("cannot open repository: %v", err),
		})
		return outputHealthResults(cmd, results, jsonOut)
	}

	branch, err := adapter.CurrentBranch(ctx)
	if err != nil {
		results = append(results, healthResult{
			Name:    "git-adapter",
			Status:  "warn",
			Message: fmt.Sprintf("cannot read current branch: %v", err),
		})
	} else {
		results = append(results, healthResult{
			Name:    "git-adapter",
			Status:  "ok",
			Message: fmt.Sprintf("on branch %s", branch),
		})
	}

	// 6. Check workspace consistency.
	ks := store.NewKnowledgeStore(s.DB(), nil)
	workspaces, wsErr := ks.ListWorkspaces(ctx)
	if wsErr != nil {
		results = append(results, healthResult{
			Name:    "workspaces",
			Status:  "warn",
			Message: fmt.Sprintf("cannot list workspaces: %v", wsErr),
		})
	} else if len(workspaces) == 0 {
		results = append(results, healthResult{
			Name:    "workspaces",
			Status:  "ok",
			Message: "no workspaces defined",
		})
	} else {
		// Check for stale workspace references.
		realBranches, brErr := adapter.ListBranches(ctx)
		branchSet := make(map[string]bool)
		if brErr == nil {
			for _, b := range realBranches {
				branchSet[b.Name] = true
			}
		}

		staleCount := 0
		for _, ws := range workspaces {
			wsStatus, statusErr := ks.GetWorkspaceStatus(ctx, ws.Name)
			if statusErr != nil {
				continue
			}
			for _, b := range wsStatus.Branches {
				if !branchSet[b.BranchName] {
					staleCount++
				}
			}
		}

		msg := fmt.Sprintf("%d workspace(s) found", len(workspaces))
		if staleCount > 0 {
			msg += fmt.Sprintf(", %d stale branch reference(s)", staleCount)
			results = append(results, healthResult{
				Name:    "workspaces",
				Status:  "warn",
				Message: msg + " — run 'got workspace sync' to clean up",
			})
		} else {
			results = append(results, healthResult{
				Name:    "workspaces",
				Status:  "ok",
				Message: msg,
			})
		}
	}

	return outputHealthResults(cmd, results, jsonOut)
}

func outputHealthResults(cmd *cobra.Command, results []healthResult, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	allOK := true
	for _, r := range results {
		var icon string
		switch r.Status {
		case "ok":
			icon = "✓"
		case "warn":
			icon = "⚠"
			allOK = false
		case "fail":
			icon = "✗"
			allOK = false
		}
		msg := r.Message
		if msg != "" {
			msg = " — " + msg
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  %s %s%s\n", icon, r.Name, msg)
	}

	if allOK {
		fmt.Fprintf(cmd.OutOrStdout(), "\nAll checks passed.\n")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "\nSome checks have warnings or failures.\n")
	}

	return nil
}
