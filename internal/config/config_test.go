package config

import (
	"path/filepath"
	"testing"
)

func TestDefaultProjectConfig(t *testing.T) {
	c := DefaultProjectConfig()
	if c.Version != 1 {
		t.Errorf("Version = %d, want 1", c.Version)
	}
	if c.Project.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, want main", c.Project.DefaultBranch)
	}
	if c.Commits.Style != "conventional" {
		t.Errorf("Commits.Style = %q, want conventional", c.Commits.Style)
	}
	if !c.Commits.AllowBreaking {
		t.Errorf("Commits.AllowBreaking = false, want true")
	}
	if len(c.Commits.Scopes) != 0 {
		t.Errorf("Commits.Scopes = %v, want []", c.Commits.Scopes)
	}
	if len(c.Plugins.Enabled) != 0 {
		t.Errorf("Plugins.Enabled = %v, want []", c.Plugins.Enabled)
	}
	if c.AI.Provider != "heuristic" {
		t.Errorf("AI.Provider = %q, want heuristic", c.AI.Provider)
	}
}

func TestProjectConfigRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "got.yml")
	in := DefaultProjectConfig()
	in.Project.Name = "myproject"
	in.Project.DefaultBranch = "trunk"
	in.Commits.Style = "freeform"
	in.Commits.Scopes = []string{"api", "cli"}
	in.Plugins.Enabled = []string{"github"}

	if err := WriteProjectConfig(path, in); err != nil {
		t.Fatalf("WriteProjectConfig: %v", err)
	}
	out, err := ReadProjectConfig(path)
	if err != nil {
		t.Fatalf("ReadProjectConfig: %v", err)
	}
	if out.Project.Name != "myproject" {
		t.Errorf("Project.Name = %q, want myproject", out.Project.Name)
	}
	if out.Project.DefaultBranch != "trunk" {
		t.Errorf("Project.DefaultBranch = %q, want trunk", out.Project.DefaultBranch)
	}
	if out.Commits.Style != "freeform" {
		t.Errorf("Commits.Style = %q, want freeform", out.Commits.Style)
	}
	if len(out.Commits.Scopes) != 2 || out.Commits.Scopes[0] != "api" {
		t.Errorf("Commits.Scopes = %v", out.Commits.Scopes)
	}
	if len(out.Plugins.Enabled) != 1 || out.Plugins.Enabled[0] != "github" {
		t.Errorf("Plugins.Enabled = %v", out.Plugins.Enabled)
	}
}

func TestInternalConfigRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	in := DefaultInternalConfig("0.1.0")
	if err := WriteInternalConfig(path, in); err != nil {
		t.Fatalf("WriteInternalConfig: %v", err)
	}
	out, err := ReadInternalConfig(path)
	if err != nil {
		t.Fatalf("ReadInternalConfig: %v", err)
	}
	if out.Version != 1 {
		t.Errorf("Version = %d, want 1", out.Version)
	}
	if out.CreatedFrom != "0.1.0" {
		t.Errorf("CreatedFrom = %q, want 0.1.0", out.CreatedFrom)
	}
}

func TestValidateCommitStyle(t *testing.T) {
	good := []string{"conventional", "freeform", "custom", "Conventional", "FREEFORM"}
	for _, s := range good {
		if err := ValidateCommitStyle(s); err != nil {
			t.Errorf("ValidateCommitStyle(%q): %v", s, err)
		}
	}
	bad := []string{"", "weird", "conv"}
	for _, s := range bad {
		if err := ValidateCommitStyle(s); err == nil {
			t.Errorf("ValidateCommitStyle(%q) = nil, want error", s)
		}
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	exists, err := FileExists(filepath.Join(dir, "nope"))
	if err != nil {
		t.Fatalf("FileExists missing: %v", err)
	}
	if exists {
		t.Errorf("FileExists(missing) = true, want false")
	}

	path := filepath.Join(dir, "yes.txt")
	if err := writeFile(path, []byte("x")); err != nil {
		t.Fatal(err)
	}
	exists, err = FileExists(path)
	if err != nil {
		t.Fatalf("FileExists present: %v", err)
	}
	if !exists {
		t.Errorf("FileExists(present) = false, want true")
	}
}
