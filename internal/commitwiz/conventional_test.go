package commitwiz

import (
	"strings"
	"testing"
)

func TestValidate_Empty(t *testing.T) {
	v := Validate("")
	if len(v.Issues) == 0 {
		t.Errorf("Issues is empty; want at least one (empty)")
	}
	if v.Issues[0].Code != "empty" {
		t.Errorf("first Issue code = %q, want empty", v.Issues[0].Code)
	}
}

func TestValidate_ValidSimple(t *testing.T) {
	v := Validate("feat: add foo")
	if v.Type != "feat" {
		t.Errorf("Type = %q, want feat", v.Type)
	}
	if v.Scope != "" {
		t.Errorf("Scope = %q, want empty", v.Scope)
	}
	if v.Breaking {
		t.Errorf("Breaking = true, want false")
	}
	if v.Subject != "add foo" {
		t.Errorf("Subject = %q, want %q", v.Subject, "add foo")
	}
	if len(v.Issues) != 0 {
		t.Errorf("Issues = %+v, want []", v.Issues)
	}
}

func TestValidate_ValidWithScope(t *testing.T) {
	v := Validate("fix(cli): handle nil")
	if v.Type != "fix" || v.Scope != "cli" || v.Subject != "handle nil" {
		t.Errorf("got %+v", v)
	}
}

func TestValidate_ValidWithBang(t *testing.T) {
	v := Validate("feat(api)!: drop v1 endpoints")
	if !v.Breaking {
		t.Errorf("Breaking = false, want true")
	}
	if v.Scope != "api" {
		t.Errorf("Scope = %q, want api", v.Scope)
	}
}

func TestValidate_BadHeader(t *testing.T) {
	v := Validate("this is not conventional")
	if len(v.Issues) == 0 || v.Issues[0].Code != "bad_header" {
		t.Errorf("Issues = %+v, want bad_header", v.Issues)
	}
}

func TestValidate_UnknownType(t *testing.T) {
	v := Validate("wibble: stuff")
	if v.Type != "wibble" {
		t.Errorf("Type = %q, want wibble (preserved on unknown type)", v.Type)
	}
	found := false
	for _, i := range v.Issues {
		if i.Code == "unknown_type" {
			found = true
		}
	}
	if !found {
		t.Errorf("Issues = %+v, want unknown_type", v.Issues)
	}
}

func TestValidate_SubjectTooLong(t *testing.T) {
	long := "feat: " + strings.Repeat("a", 100)
	v := Validate(long)
	found := false
	for _, i := range v.Issues {
		if i.Code == "subject_too_long" {
			found = true
		}
	}
	if !found {
		t.Errorf("Issues = %+v, want subject_too_long", v.Issues)
	}
}

func TestValidate_SubjectTrailingPeriod(t *testing.T) {
	v := Validate("feat: add foo.")
	found := false
	for _, i := range v.Issues {
		if i.Code == "subject_trailing_period" {
			found = true
		}
	}
	if !found {
		t.Errorf("Issues = %+v, want subject_trailing_period", v.Issues)
	}
}

func TestValidate_SubjectCapitalized(t *testing.T) {
	v := Validate("feat: Add foo")
	found := false
	for _, i := range v.Issues {
		if i.Code == "subject_capitalized" {
			found = true
		}
	}
	if !found {
		t.Errorf("Issues = %+v, want subject_capitalized", v.Issues)
	}
}

func TestValidate_AcronymAllowed(t *testing.T) {
	// "HTTP: ..." should not trigger subject_capitalized.
	v := Validate("feat: HTTP support")
	for _, i := range v.Issues {
		if i.Code == "subject_capitalized" {
			t.Errorf("HTTP subject should not trigger subject_capitalized: %+v", i)
		}
	}
}

func TestValidate_BodyAndFooters(t *testing.T) {
	msg := `feat(cli): add foo

This is a longer body line that explains
the change in more detail.

BREAKING CHANGE: requires git 2.30+
Refs: #123`
	v := Validate(msg)
	if v.Body == "" {
		t.Errorf("Body is empty; want non-empty")
	}
	if len(v.Footers) != 2 {
		t.Errorf("Footers = %v, want 2 entries", v.Footers)
	}
	if !v.Breaking {
		t.Errorf("Breaking = false; BREAKING CHANGE footer should set it")
	}
}

func TestValidate_BodyLineTooLong(t *testing.T) {
	msg := "feat: x\n\n" + strings.Repeat("a", 80)
	v := Validate(msg)
	found := false
	for _, i := range v.Issues {
		if i.Code == "body_line_too_long" {
			found = true
		}
	}
	if !found {
		t.Errorf("Issues = %+v, want body_line_too_long", v.Issues)
	}
}

func TestRender_RoundTrip(t *testing.T) {
	in := "feat(cli): add foo\n\nLong body line.\n\nBREAKING CHANGE: requires git 2.30+"
	v := Validate(in)
	out := v.Render()
	// Re-validate the rendered message; it should parse identically
	// (modulo whitespace normalization).
	v2 := Validate(out)
	if v2.Type != v.Type || v2.Scope != v.Scope || v2.Subject != v.Subject || v2.Breaking != v.Breaking {
		t.Errorf("round-trip mismatch:\nbefore: %+v\nafter:  %+v\nrendered: %q", v, v2, out)
	}
}

func TestRender_OmitsBangWhenFooterPresent(t *testing.T) {
	v := Validated{
		Type:     "feat",
		Scope:    "api",
		Breaking: true,
		Subject:  "drop v1",
		Footers:  []string{"BREAKING CHANGE: requires v2 client"},
	}
	out := v.Render()
	if strings.Contains(out, "!:") {
		t.Errorf("Render with breaking footer should not also emit '!':\n%s", out)
	}
	if !strings.Contains(out, "BREAKING CHANGE:") {
		t.Errorf("Render should keep the BREAKING CHANGE footer:\n%s", out)
	}
}

func TestRender_EmitsBangWithoutFooter(t *testing.T) {
	v := Validated{
		Type:     "feat",
		Scope:    "api",
		Breaking: true,
		Subject:  "drop v1",
	}
	out := v.Render()
	if !strings.Contains(out, "feat(api)!: drop v1") {
		t.Errorf("Render without breaking footer should emit '!':\n%s", out)
	}
}

func TestRender_PlainHeader(t *testing.T) {
	v := Validated{Type: "fix", Subject: "handle nil"}
	out := v.Render()
	if !strings.HasSuffix(out, "fix: handle nil\n") {
		t.Errorf("Render = %q, want trailing 'fix: handle nil\\n'", out)
	}
}
