package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/store"
)

// newInitCmd returns the `got init` subcommand.
func newInitCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Initialize GOT in a Git repository",
		Long: `Initialize GOT metadata storage in a Git repository.

Creates a .got/ directory at the repository root with SQLite database
and supporting subdirectories, writes a minimal got.yml configuration,
and adds .got/ to .gitignore.

Run inside a Git repository, or pass a path to one.

Examples:
  got init                       # init in current directory (must be in a Git repo)
  got init /path/to/repo         # init in a specific repo
  got init --force               # re-init (preserves DB, overwrites config)`,

		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, args, force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Re-initialize (preserves DB, overwrites config)")

	return cmd
}

func runInit(cmd *cobra.Command, args []string, force bool) error {
	// ── Determine target directory ───────────────────────────────
	target := "."
	if len(args) > 0 {
		target = args[0]
	}

	target, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("init: resolve path: %w", err)
	}

	// ── Find repo root (walk up for .git/) ───────────────────────
	repoRoot, err := findGitDir(target)
	if err != nil {
		return fmt.Errorf("init: not a Git repository: %w\n  Use 'git init' first, then run 'got init'", err)
	}

	gotDir := filepath.Join(repoRoot, ".got")

	// ── Check if already initialized ─────────────────────────────
	if !force {
		if info, err := os.Stat(gotDir); err == nil && info.IsDir() {
			return fmt.Errorf("init: GOT already initialized in %s (use --force to re-initialize)", repoRoot)
		}
	}

	// ── Create .got/ directory structure ─────────────────────────
	subdirs := []string{
		gotDir,
		filepath.Join(gotDir, "decisions"),
		filepath.Join(gotDir, "snapshots"),
		filepath.Join(gotDir, "workspaces"),
		filepath.Join(gotDir, "health"),
		filepath.Join(gotDir, "cache"),
		filepath.Join(gotDir, "plugins"),
	}

	for _, d := range subdirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("init: create %s: %w", d, err)
		}
	}

	// ── Open store (creates DB + runs migrations) ────────────────
	dbPath := filepath.Join(gotDir, "got.db")
	s, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("init: database: %w", err)
	}
	s.Close()

	// ── Write got.yml ────────────────────────────────────────────
	projectName := filepath.Base(repoRoot)
	gotYmlPath := filepath.Join(repoRoot, "got.yml")
	gotYml := fmt.Sprintf(`# GOT project configuration
# See https://got.sh/docs for more information.
version: 1
project:
  name: %s
  default_branch: main
commits:
  style: conventional
  scopes: []
  allow_breaking: true
plugins:
  enabled: []
ai:
  provider: heuristic
`, projectName)

	if err := os.WriteFile(gotYmlPath, []byte(gotYml), 0644); err != nil {
		return fmt.Errorf("init: write got.yml: %w", err)
	}

	// ── Append .got/ to .gitignore if not present ────────────────
	gitignorePath := filepath.Join(repoRoot, ".gitignore")
	gitignoreEntry := "\n# GOT metadata directory\n.got/\n"

	gitignoreContent, err := os.ReadFile(gitignorePath)
	if err != nil {
		// File doesn't exist — start with header.
		gitignoreContent = []byte("# .gitignore\n")
	}

	if !strings.Contains(string(gitignoreContent), ".got/") {
		gitignoreContent = append(gitignoreContent, []byte(gitignoreEntry)...)
		if err := os.WriteFile(gitignorePath, gitignoreContent, 0644); err != nil {
			return fmt.Errorf("init: update .gitignore: %w", err)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Initialized GOT in %s\n", repoRoot)
	fmt.Fprintf(cmd.OutOrStdout(), "  .got/got.db        SQLite database (migrations applied)\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  got.yml            Project configuration\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  .gitignore         .got/ added to ignore file\n")
	fmt.Fprintf(cmd.OutOrStdout(), "\nNext: try 'got decision create' or 'got note add'\n")

	return nil
}

// findGitDir walks up from start looking for a .git/ directory (file or dir).
// Returns the repo root (parent of .git/).
func findGitDir(start string) (string, error) {
	start, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}

	dir := start
	for {
		gitPath := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			// .git can be a directory (regular repo) or a file (worktree).
			if info.IsDir() || info.Mode().IsRegular() {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .git found in %s or any parent", start)
		}
		dir = parent
	}
}
