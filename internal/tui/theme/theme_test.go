package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestTabConstants(t *testing.T) {
	if TabStatus != 0 {
		t.Errorf("TabStatus = %d, want 0", TabStatus)
	}
	if TabBranches != 1 {
		t.Errorf("TabBranches = %d, want 1", TabBranches)
	}
	if TabRemotes != 2 {
		t.Errorf("TabRemotes = %d, want 2", TabRemotes)
	}
	if TabGraph != 3 {
		t.Errorf("TabGraph = %d, want 3", TabGraph)
	}
	if TabPlugins != 4 {
		t.Errorf("TabPlugins = %d, want 4", TabPlugins)
	}
	if TabCount != 5 {
		t.Errorf("TabCount = %d, want 5", TabCount)
	}
}

func TestTabNames(t *testing.T) {
	if len(TabNames) != TabCount {
		t.Fatalf("len(TabNames) = %d, want %d", len(TabNames), TabCount)
	}

	expected := []string{
		" Status ",
		" Branches ",
		" Remotes ",
		" Graph ",
		" Plugins ",
	}

	for i, want := range expected {
		if TabNames[i] != want {
			t.Errorf("TabNames[%d] = %q, want %q", i, TabNames[i], want)
		}
	}
}

func TestStylesRender(t *testing.T) {
	styles := []struct {
		name  string
		style lipgloss.Style
	}{
		{"Base", Base},
		{"Title", Title},
		{"TabActive", TabActive},
		{"TabInactive", TabInactive},
		{"TabBorder", TabBorder},
		{"StatusBar", StatusBar},
		{"Header", Header},
		{"Section", Section},
		{"Item", Item},
		{"Success", Success},
		{"Warning", Warning},
		{"Error", Error},
		{"Muted", Muted},
		{"Key", Key},
		{"Help", Help},
		{"Selected", Selected},
		{"BranchCurrent", BranchCurrent},
		{"BranchRemote", BranchRemote},
		{"CommitSHA", CommitSHA},
		{"GraphNode", GraphNode},
		{"GraphEdge", GraphEdge},
		{"Mag", Mag},
		{"Cya", Cya},
	}

	for _, s := range styles {
		t.Run(s.name, func(t *testing.T) {
			// Verify style renders without panic
			result := s.style.Render("test")
			if result == "" {
				t.Logf("Warning: style %s rendered empty string", s.name)
			}
		})
	}
}

func TestColorsDefined(t *testing.T) {
	// Verify all color variables are set (non-zero)
	colors := []struct {
		name  string
		color lipgloss.Color
	}{
		{"Bg", Bg},
		{"Fg", Fg},
		{"MutedC", MutedC},
		{"Accent", Accent},
		{"Green", Green},
		{"Yellow", Yellow},
		{"Red", Red},
		{"Cyan", Cyan},
		{"Magenta", Magenta},
	}

	for _, c := range colors {
		t.Run(c.name, func(t *testing.T) {
			// Color should be non-empty
			if string(c.color) == "" {
				t.Errorf("color %s is empty", c.name)
			}
		})
	}
}
