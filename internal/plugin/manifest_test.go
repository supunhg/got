package plugin

import (
	"strings"
	"testing"
)

const sampleManifest = `{
  "manifest_version": 1,
  "name": "github",
  "version": "1.2.0",
  "min_got": "0.1.0",
  "commands": [
    {"name": "pr", "description": "Open a GitHub PR"}
  ]
}`

func TestParseManifest_Valid(t *testing.T) {
	m, err := ParseManifest([]byte(sampleManifest))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Name != "github" {
		t.Errorf("Name = %q, want github", m.Name)
	}
	if m.Version != "1.2.0" {
		t.Errorf("Version = %q, want 1.2.0", m.Version)
	}
	if m.MinGOT != "0.1.0" {
		t.Errorf("MinGOT = %q, want 0.1.0", m.MinGOT)
	}
	if len(m.Commands) != 1 || m.Commands[0].Name != "pr" {
		t.Errorf("Commands = %+v, want one command named pr", m.Commands)
	}
}

func TestParseManifest_RejectsUnknownVersion(t *testing.T) {
	raw := strings.Replace(sampleManifest, `"manifest_version": 1`, `"manifest_version": 2`, 1)
	_, err := ParseManifest([]byte(raw))
	if err == nil {
		t.Fatalf("expected error for manifest_version=2, got nil")
	}
	if !strings.Contains(err.Error(), "manifest_version 2 is not supported") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseManifest_RejectsMissingVersion(t *testing.T) {
	raw := strings.Replace(sampleManifest, `"manifest_version": 1,`, ``, 1)
	_, err := ParseManifest([]byte(raw))
	if err == nil {
		t.Fatalf("expected error for missing manifest_version, got nil")
	}
	if !strings.Contains(err.Error(), "missing manifest_version") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseManifest_RejectsMissingName(t *testing.T) {
	raw := strings.Replace(sampleManifest, `"name": "github",`, `"name": "",`, 1)
	_, err := ParseManifest([]byte(raw))
	if err == nil {
		t.Fatalf("expected error for empty name, got nil")
	}
	if !strings.Contains(err.Error(), "missing name") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseManifest_RejectsMissingMinGOT(t *testing.T) {
	raw := strings.Replace(sampleManifest, `"min_got": "0.1.0",`, `"min_got": "",`, 1)
	_, err := ParseManifest([]byte(raw))
	if err == nil {
		t.Fatalf("expected error for empty min_got, got nil")
	}
	if !strings.Contains(err.Error(), "missing min_got") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseManifest_RejectsNoCommands(t *testing.T) {
	// Use a fully separate manifest so the replacement isn't
	// whitespace-sensitive.
	raw := `{"manifest_version":1,"name":"x","version":"1.0.0","min_got":"0.1.0","commands":[]}`
	_, err := ParseManifest([]byte(raw))
	if err == nil {
		t.Fatalf("expected error for no commands, got nil")
	}
	if !strings.Contains(err.Error(), "no commands") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseManifest_RejectsCommandWithoutName(t *testing.T) {
	raw := strings.Replace(sampleManifest, `"name": "pr"`, `"name": ""`, 1)
	_, err := ParseManifest([]byte(raw))
	if err == nil {
		t.Fatalf("expected error for command without name, got nil")
	}
	if !strings.Contains(err.Error(), "missing name") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseManifest_RejectsInvalidJSON(t *testing.T) {
	_, err := ParseManifest([]byte("not json"))
	if err == nil {
		t.Fatalf("expected error for invalid JSON, got nil")
	}
}

func TestParseManifest_RejectsUnknownFields(t *testing.T) {
	raw := strings.Replace(sampleManifest, `"min_got": "0.1.0",`, `"min_got": "0.1.0", "extra": "nope",`, 1)
	_, err := ParseManifest([]byte(raw))
	if err == nil {
		t.Fatalf("expected error for unknown field, got nil")
	}
}

func TestMeetsMinGOT(t *testing.T) {
	cases := []struct {
		manifest string
		running  string
		want     bool
		wantErr  bool
	}{
		{"0.1.0", "0.1.0", true, false},
		{"0.1.0", "0.2.0", true, false},
		{"0.1.0", "1.0.0", true, false},
		{"0.2.0", "0.1.0", false, false},
		{"1.0.0", "0.1.0", false, false},
		{"", "0.1.0", true, false},    // empty min_got = no constraint
		{"0.1.0", "dev", true, false}, // dev builds satisfy everything
		{"not-semver", "0.1.0", false, true},
	}
	for _, c := range cases {
		got, err := MeetsMinGOT(c.manifest, c.running)
		if (err != nil) != c.wantErr {
			t.Errorf("MeetsMinGOT(%q, %q) err = %v, wantErr = %v", c.manifest, c.running, err, c.wantErr)
		}
		if got != c.want {
			t.Errorf("MeetsMinGOT(%q, %q) = %v, want %v", c.manifest, c.running, got, c.want)
		}
	}
}

func TestParseSemver(t *testing.T) {
	cases := []struct {
		in    string
		want  [3]int
		valid bool
	}{
		{"0.0.0", [3]int{0, 0, 0}, true},
		{"0.1.0", [3]int{0, 1, 0}, true},
		{"10.20.30", [3]int{10, 20, 30}, true},
		{"1.2", [3]int{}, false},
		{"1.2.3.4", [3]int{}, false},
		{"1.2.x", [3]int{}, false},
		{"", [3]int{}, false},
	}
	for _, c := range cases {
		got, err := parseSemver(c.in)
		if (err == nil) != c.valid {
			t.Errorf("parseSemver(%q) valid = %v, want %v (err=%v)", c.in, err == nil, c.valid, err)
		}
		if c.valid && got != c.want {
			t.Errorf("parseSemver(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
