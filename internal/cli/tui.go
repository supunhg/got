// Copyright 2026 Supun Hewagamage. MIT License.
package cli

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/spf13/cobra"
	"github.com/supunhg/got/internal/tui"
)

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Interactive terminal dashboard",
		Long: `Launch the interactive TUI dashboard for GOT.

Navigate with vim-style keybindings:
  h/l or Tab    Switch tabs
  j/k           Scroll content
  r             Refresh
  ?             Toggle help
  q             Quit

Tabs: Status, Branches, Remotes, Graph, Plugins`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m := tui.NewModel()
			p := tea.NewProgram(m, tea.WithAltScreen())
			_, err := p.Run()
			return err
		},
	}
}
