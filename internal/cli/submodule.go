// Copyright 2026 Supun Hewagamage. MIT License.
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/supunhg/got/internal/git"
)

func newSubmoduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submodule",
		Short: "Manage Git submodules",
		Long:  `Manage Git submodules in the current repository.`,
	}

	cmd.AddCommand(newSubmoduleListCmd())
	cmd.AddCommand(newSubmoduleInitCmd())
	cmd.AddCommand(newSubmoduleUpdateCmd())

	return cmd
}

func newSubmoduleListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all submodules",
		Long:  `List all submodules in the current repository.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSubmoduleList(cmd)
		},
	}
}

func newSubmoduleInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init [name]",
		Short: "Initialize a submodule",
		Long:  `Initialize a submodule. If no name is provided, initializes all submodules.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return runSubmoduleInit(cmd, name)
		},
	}
}

func newSubmoduleUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update [name]",
		Short: "Update a submodule",
		Long:  `Update a submodule to the latest commit. If no name is provided, updates all submodules.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return runSubmoduleUpdate(cmd, name)
		},
	}
}

func runSubmoduleList(cmd *cobra.Command) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("submodule list: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("submodule list: %w", err)
	}

	submodules, err := adapter.ListSubmodules(ctx)
	if err != nil {
		return fmt.Errorf("submodule list: %w", err)
	}

	if len(submodules) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No submodules found.")
		return nil
	}

	for _, s := range submodules {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", s.Name, s.URL)
	}

	return nil
}

func runSubmoduleInit(cmd *cobra.Command, name string) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("submodule init: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("submodule init: %w", err)
	}

	if name != "" {
		if err := adapter.InitSubmodule(ctx, name); err != nil {
			return fmt.Errorf("submodule init: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Initialized submodule %s\n", name)
	} else {
		// Init all submodules
		submodules, err := adapter.ListSubmodules(ctx)
		if err != nil {
			return fmt.Errorf("submodule init: %w", err)
		}
		for _, s := range submodules {
			if err := adapter.InitSubmodule(ctx, s.Name); err != nil {
				return fmt.Errorf("submodule init: %w", err)
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Initialized %d submodules\n", len(submodules))
	}

	return nil
}

func runSubmoduleUpdate(cmd *cobra.Command, name string) error {
	ctx := context.Background()

	repoPath, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("submodule update: %w", err)
	}

	adapter := git.NewExecAdapter(nil)
	if err := adapter.OpenRepository(ctx, repoPath); err != nil {
		return fmt.Errorf("submodule update: %w", err)
	}

	if name != "" {
		if err := adapter.UpdateSubmodule(ctx, name); err != nil {
			return fmt.Errorf("submodule update: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated submodule %s\n", name)
	} else {
		// Update all submodules
		submodules, err := adapter.ListSubmodules(ctx)
		if err != nil {
			return fmt.Errorf("submodule update: %w", err)
		}
		for _, s := range submodules {
			if err := adapter.UpdateSubmodule(ctx, s.Name); err != nil {
				return fmt.Errorf("submodule update: %w", err)
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated %d submodules\n", len(submodules))
	}

	return nil
}
