// Package theme provides colors and styles for the GOT TUI.
// Separated from the tui package to avoid import cycles with tabs.
package theme

import "github.com/charmbracelet/lipgloss"

// Colors
var (
	Bg      = lipgloss.Color("#1a1b26")
	Fg      = lipgloss.Color("#c0caf5")
	MutedC  = lipgloss.Color("#565f89")
	Accent  = lipgloss.Color("#7aa2f7")
	Green   = lipgloss.Color("#9ece6a")
	Yellow  = lipgloss.Color("#e0af68")
	Red     = lipgloss.Color("#f7768e")
	Cyan    = lipgloss.Color("#7dcfff")
	Magenta = lipgloss.Color("#bb9af7")
)

// Styles
var (
	Base = lipgloss.NewStyle().
		Background(Bg).Foreground(Fg)

	Title = lipgloss.NewStyle().
		Bold(true).Foreground(Accent).Background(Bg).Padding(0, 1)

	TabActive = lipgloss.NewStyle().
			Bold(true).Foreground(Bg).Background(Accent).Padding(0, 1)

	TabInactive = lipgloss.NewStyle().
			Foreground(MutedC).Background(Bg).Padding(0, 1)

	TabBorder = lipgloss.NewStyle().
			Foreground(MutedC).Background(Bg)

	StatusBar = lipgloss.NewStyle().
			Foreground(MutedC).Background(Bg).Padding(0, 1)

	Header = lipgloss.NewStyle().
		Bold(true).Foreground(Accent).Background(Bg).Padding(0, 1)

	Section = lipgloss.NewStyle().
		Foreground(Cyan).Background(Bg).MarginTop(1).MarginLeft(2)

	Item = lipgloss.NewStyle().
		Foreground(Fg).Background(Bg).MarginLeft(4)

	Success = lipgloss.NewStyle().
		Foreground(Green).Background(Bg)

	Warning = lipgloss.NewStyle().
		Foreground(Yellow).Background(Bg)

	Error = lipgloss.NewStyle().
		Foreground(Red).Background(Bg)

	Muted = lipgloss.NewStyle().
		Foreground(MutedC).Background(Bg)

	Key = lipgloss.NewStyle().
		Foreground(Magenta).Background(Bg).Bold(true)

	Help = lipgloss.NewStyle().
		Foreground(MutedC).Background(Bg).PaddingTop(1).PaddingLeft(2)

	Selected = lipgloss.NewStyle().
			Foreground(Bg).Background(Accent).Padding(0, 1)

	BranchCurrent = lipgloss.NewStyle().
			Foreground(Green).Background(Bg).Bold(true)

	BranchRemote = lipgloss.NewStyle().
			Foreground(Cyan).Background(Bg)

	CommitSHA = lipgloss.NewStyle().
			Foreground(Magenta).Background(Bg).Bold(true)

	GraphNode = lipgloss.NewStyle().
			Foreground(Fg).Background(Bg)

	GraphEdge = lipgloss.NewStyle().
			Foreground(MutedC).Background(Bg)

	Mag = lipgloss.NewStyle().
		Foreground(Magenta).Background(Bg)

	Cya = lipgloss.NewStyle().
		Foreground(Cyan).Background(Bg)
)

// Tab indices
const (
	TabStatus   = 0
	TabBranches = 1
	TabRemotes  = 2
	TabGraph    = 3
	TabPlugins  = 4
	TabCount    = 5
)

var TabNames = [TabCount]string{
	TabStatus:   " Status ",
	TabBranches: " Branches ",
	TabRemotes:  " Remotes ",
	TabGraph:    " Graph ",
	TabPlugins:  " Plugins ",
}
