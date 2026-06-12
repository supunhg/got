// Package tui holds the shared Lip Gloss theme for all GOT TUIs. The
// init wizard (internal/initwiz) and the commit wizard
// (internal/commitwiz, future) both pull colors from here so the look
// and feel stays consistent across surfaces.
//
// The theme is intentionally tiny: a handful of named styles, no
// component definitions (those live in charmbracelet/bubbles). When
// --no-color is set, every style returns the plain text so the
// terminal stays readable in a log or pipe.
package tui

import "github.com/charmbracelet/lipgloss"

// Theme bundles the Lip Gloss styles used across GOT's TUIs. It is
// constructed once at startup and passed to every model that wants
// styled output. NoColor is wired into every style so callers can
// build a Theme{NoColor: true} for CI / log-only contexts.
type Theme struct {
	NoColor bool

	Title      lipgloss.Style
	Subtitle   lipgloss.Style
	Body       lipgloss.Style
	Muted      lipgloss.Style
	Accent     lipgloss.Style
	Selected   lipgloss.Style
	Unselected lipgloss.Style
	Hint       lipgloss.Style
	Success    lipgloss.Style
	Error      lipgloss.Style
	Box        lipgloss.Style
	Detected   lipgloss.Style
}

// NewTheme returns a Theme with the v0.1 color palette. Palette is
// chosen to read well on both light and dark terminals; colors are
// the same set charmbracelet/lipgloss examples use.
func NewTheme() Theme {
	if lipgloss.HasDarkBackground() {
		return darkTheme()
	}
	return lightTheme()
}

// NoColorTheme returns a Theme with all colors stripped. Used when
// --no-color is set or stdout is not a TTY.
func NoColorTheme() Theme {
	t := NewTheme()
	t.NoColor = true
	return t
}

// apply returns the given style with Foreground/Background stripped
// when NoColor is set. Called by every Render helper to keep callers
// from having to remember the no-color dance.
func (t Theme) apply(s lipgloss.Style) lipgloss.Style {
	if t.NoColor {
		return s.Foreground(lipgloss.NoColor{}).Background(lipgloss.NoColor{})
	}
	return s
}

func darkTheme() Theme {
	return Theme{
		Title:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1),
		Subtitle:   lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true),
		Body:       lipgloss.NewStyle().Foreground(lipgloss.Color("#EEEEEE")),
		Muted:      lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")),
		Accent:     lipgloss.NewStyle().Foreground(lipgloss.Color("#43BF6D")).Bold(true),
		Selected:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Padding(0, 1),
		Unselected: lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA")).Padding(0, 1),
		Hint:       lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Italic(true),
		Success:    lipgloss.NewStyle().Foreground(lipgloss.Color("#43BF6D")).Bold(true),
		Error:      lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F5F")).Bold(true),
		Box:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#7D56F4")).Padding(0, 1),
		Detected:   lipgloss.NewStyle().Foreground(lipgloss.Color("#00BFFF")),
	}
}

func lightTheme() Theme {
	return Theme{
		Title:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#000000")).Padding(0, 1),
		Subtitle:   lipgloss.NewStyle().Foreground(lipgloss.Color("#5A2CA0")).Bold(true),
		Body:       lipgloss.NewStyle().Foreground(lipgloss.Color("#1A1A1A")),
		Muted:      lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")),
		Accent:     lipgloss.NewStyle().Foreground(lipgloss.Color("#118C4F")).Bold(true),
		Selected:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Background(lipgloss.Color("#5A2CA0")).Padding(0, 1),
		Unselected: lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")).Padding(0, 1),
		Hint:       lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Italic(true),
		Success:    lipgloss.NewStyle().Foreground(lipgloss.Color("#118C4F")).Bold(true),
		Error:      lipgloss.NewStyle().Foreground(lipgloss.Color("#B00020")).Bold(true),
		Box:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#5A2CA0")).Padding(0, 1),
		Detected:   lipgloss.NewStyle().Foreground(lipgloss.Color("#0066AA")),
	}
}

// Apply rebuilds all of t's styles so the NoColor override takes
// effect. Returns a new Theme value (themes are small; copy is fine).
func (t Theme) Apply() Theme {
	return Theme{
		NoColor:    t.NoColor,
		Title:      t.apply(t.Title),
		Subtitle:   t.apply(t.Subtitle),
		Body:       t.apply(t.Body),
		Muted:      t.apply(t.Muted),
		Accent:     t.apply(t.Accent),
		Selected:   t.apply(t.Selected),
		Unselected: t.apply(t.Unselected),
		Hint:       t.apply(t.Hint),
		Success:    t.apply(t.Success),
		Error:      t.apply(t.Error),
		Box:        t.apply(t.Box),
		Detected:   t.apply(t.Detected),
	}
}
