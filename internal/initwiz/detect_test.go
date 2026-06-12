package initwiz

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestDetectEmptyDir(t *testing.T) {
	d := Detect(t.TempDir())
	if d.Name == "" {
		t.Errorf("Name is empty")
	}
	if len(d.Languages) != 0 {
		t.Errorf("Languages = %v, want []", d.Languages)
	}
	if len(d.Frameworks) != 0 {
		t.Errorf("Frameworks = %v, want []", d.Frameworks)
	}
}

func TestDetectGoProject(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "go.mod", "module example.com/x\n\ngo 1.22\n")
	mustWrite(t, root, "go.sum", "")
	d := Detect(root)
	if !contains(d.Languages, "go") {
		t.Errorf("Languages = %v, want to contain go", d.Languages)
	}
}

func TestDetectNodeProject(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "package.json", `{"name":"x","version":"1.0.0"}`)
	mustWrite(t, root, "tsconfig.json", "{}")
	d := Detect(root)
	if !contains(d.Languages, "javascript") {
		t.Errorf("Languages = %v, want javascript", d.Languages)
	}
	if !contains(d.Frameworks, "typescript") {
		t.Errorf("Frameworks = %v, want typescript", d.Frameworks)
	}
}

func TestDetectPythonProject(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "pyproject.toml", "[project]\nname='x'\n")
	d := Detect(root)
	if !contains(d.Languages, "python") {
		t.Errorf("Languages = %v, want python", d.Languages)
	}
}

func TestDetectMultipleFrameworks(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "package.json", `{}`)
	mustWrite(t, root, "next.config.js", "")
	mustWrite(t, root, "tailwind.config.js", "")
	mustWrite(t, root, "Dockerfile", "")
	d := Detect(root)
	wantFws := []string{"docker", "next.js", "tailwindcss"}
	got := append([]string{}, d.Frameworks...)
	sort.Strings(got)
	if !reflect.DeepEqual(got, wantFws) {
		t.Errorf("Frameworks = %v, want %v", got, wantFws)
	}
}

func TestDetectCSharpGlob(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "Foo.csproj", "<Project></Project>")
	d := Detect(root)
	if !contains(d.Languages, "c#") {
		t.Errorf("Languages = %v, want c#", d.Languages)
	}
}

func TestDetectDetectsName(t *testing.T) {
	root := t.TempDir()
	d := Detect(root)
	if d.Name != filepath.Base(root) {
		t.Errorf("Name = %q, want %q", d.Name, filepath.Base(root))
	}
}

func mustWrite(t *testing.T, root, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func contains(xs []string, target string) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}
	return false
}
