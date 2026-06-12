package commitwiz

import (
	"fmt"
	"regexp"
	"strings"
)

// ConventionalTypes is the canonical set of Conventional Commits
// types (https://www.conventionalcommits.org/en/v1.0.0/). Order
// matches the spec §8 radio (and is used by the wizard for the
// type-selection screen).
var ConventionalTypes = []string{
	"feat",
	"fix",
	"chore",
	"docs",
	"style",
	"refactor",
	"perf",
	"test",
	"build",
	"ci",
	"revert",
}

// isConventionalType reports whether t is in ConventionalTypes.
func isConventionalType(t string) bool {
	for _, ct := range ConventionalTypes {
		if ct == t {
			return true
		}
	}
	return false
}

// headerRe matches the conventional-commits header line:
//
//	<type>[(!)][(<scope>)]: <subject>
//
// Examples that match:
//
//	feat: add foo
//	fix(cli): handle nil
//	feat(api)!: drop v1 endpoints
//
// The (!) is the breaking-change marker. Scope is captured but
// otherwise unconstrained (alphanumerics + - and _).
var headerRe = regexp.MustCompile(`^(?P<type>[a-zA-Z]+)(?:\((?P<scope>[^)]*)\))?(?P<bang>!)?:\s*(?P<subject>.+?)\s*$`)

// Issue is one validation finding against a candidate commit
// message. Issues do not make a message invalid; the CLI shows them
// as a yellow warning banner and lets the user commit anyway
// (matching the spec's --no-verify path).
type Issue struct {
	// Code is a short, machine-readable tag (e.g. "missing_type",
	// "subject_too_long"). Suitable for matching in tests.
	Code string
	// Line is the 1-based line number the issue refers to, or 0
	// for whole-message issues.
	Line int
	// Message is the human-readable explanation.
	Message string
}

// Validated is the result of parsing a candidate commit message.
// Header is the parsed header; Body and Footers are the optional
// remainder. Issues lists every validation finding; an empty slice
// means the message is fully conventional.
type Validated struct {
	// Type is the conventional type, or empty if the header is
	// malformed.
	Type string
	// Scope is the parsed scope, or empty.
	Scope string
	// Breaking is true if the header carries a "!" marker or any
	// footer is a BREAKING CHANGE footer.
	Breaking bool
	// Subject is the parsed subject (the text after "type(scope): ").
	Subject string
	// Body is the message body (between the blank line and the
	// first footer). May be empty.
	Body string
	// Footers is the list of "Token: value" or "Token # value"
	// lines after the body. Each entry is a single line.
	Footers []string
	// Raw is the original input, preserved for re-rendering.
	Raw string
	// Issues lists every validation finding. An empty slice means
	// the message is fully conventional.
	Issues []Issue
}

// Validate parses and validates msg as a Conventional Commits
// message. It always returns a non-nil Validated; check Issues for
// warnings. The returned struct is also suitable for re-rendering
// via Render.
func Validate(msg string) Validated {
	v := Validated{Raw: msg}
	lines := splitLines(msg)
	if len(lines) == 0 {
		v.Issues = append(v.Issues, Issue{
			Code:    "empty",
			Message: "commit message is empty",
		})
		return v
	}
	typ, scope, bang, subject, parseIssues, ok := parseHeader(lines[0])
	if !ok {
		v.Issues = append(v.Issues, parseIssues...)
		return v
	}
	// parseIssues contains any non-fatal findings from the header
	// parse (e.g. unknown_type). They are surfaced as warnings, not
	// blockers, so we always include them.
	v.Issues = append(v.Issues, parseIssues...)
	v.Type = typ
	v.Scope = scope
	v.Breaking = bang
	v.Subject = subject

	// Subject rules: ≤72 chars, no trailing period, lowercase
	// leading char preferred (we warn but don't fail).
	if len(v.Subject) > 72 {
		v.Issues = append(v.Issues, Issue{
			Code:    "subject_too_long",
			Line:    1,
			Message: fmt.Sprintf("subject is %d chars; conventional commits recommends ≤72", len(v.Subject)),
		})
	}
	if strings.HasSuffix(v.Subject, ".") {
		v.Issues = append(v.Issues, Issue{
			Code:    "subject_trailing_period",
			Line:    1,
			Message: "subject should not end with a period",
		})
	}
	if v.Subject != "" && isUpper(v.Subject[0]) && !isAcronym(v.Subject) {
		// Uppercase first char is a style warning, not a hard
		// error. We surface it so the wizard can render a yellow
		// banner.
		v.Issues = append(v.Issues, Issue{
			Code:    "subject_capitalized",
			Line:    1,
			Message: "subject should start with a lowercase letter (imperative mood)",
		})
	}

	// Body + footers: everything after the first blank line.
	body, footers, _ := splitBodyAndFooters(lines[1:])
	v.Body = body
	v.Footers = footers

	// BREAKING CHANGE footer sets Breaking.
	for _, f := range footers {
		if isBreakingFooter(f) {
			v.Breaking = true
		}
	}

	// Body-wrapping check: a body line longer than 72 chars is a
	// warning, not an error.
	for i, line := range strings.Split(v.Body, "\n") {
		if len(line) > 72 {
			v.Issues = append(v.Issues, Issue{
				Code:    "body_line_too_long",
				Line:    i + 2, // +1 for header, +1 for the blank line
				Message: fmt.Sprintf("body line is %d chars; consider wrapping at 72", len(line)),
			})
		}
	}

	return v
}

// parseHeader pulls the type/scope/bang/subject out of one header
// line. Returns (typ, scope, bang, subject, issues, ok) where ok is
// false on a parse error and the issue slice carries the reason.
func parseHeader(line string) (typ, scope string, bang bool, subject string, issues []Issue, ok bool) {
	m := headerRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", false, "", []Issue{{
			Code:    "bad_header",
			Line:    1,
			Message: fmt.Sprintf("header %q does not match Conventional Commits format (<type>[!][(scope)]: <subject>)", line),
		}}, false
	}
	typ = m[1]
	scope = m[2]
	bang = m[3] == "!"
	subject = m[4]
	if !isConventionalType(typ) {
		issues = append(issues, Issue{
			Code:    "unknown_type",
			Line:    1,
			Message: fmt.Sprintf("type %q is not a recognized Conventional Commits type; want one of: %s", typ, strings.Join(ConventionalTypes, ", ")),
		})
	}
	return typ, scope, bang, subject, issues, true
}

// isBreakingFooter reports whether a footer line is a BREAKING
// CHANGE annotation. The spec accepts both `BREAKING CHANGE: <reason>`
// and `BREAKING-CHANGE: <reason>`.
func isBreakingFooter(line string) bool {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return false
	}
	token := strings.TrimSpace(line[:colon])
	return strings.EqualFold(token, "BREAKING CHANGE") || strings.EqualFold(token, "BREAKING-CHANGE")
}

// isUpper reports whether b is an uppercase ASCII letter.
func isUpper(b byte) bool { return b >= 'A' && b <= 'Z' }

// isAcronym is a tiny check used to suppress the "subject should
// start with lowercase" warning for short all-caps tokens at the
// start of a subject (e.g. "HTTP: ..." or "API: ..."). v0.1 only
// checks the first 2-3 characters; this is best-effort.
func isAcronym(s string) bool {
	if len(s) < 2 {
		return false
	}
	uppers := 0
	for i := 0; i < len(s) && i < 4; i++ {
		c := s[i]
		if c == ':' || c == ' ' {
			break
		}
		if !isUpper(c) {
			return false
		}
		uppers++
	}
	return uppers >= 2
}

// splitLines splits a message into physical lines, preserving order
// and trimming only the final trailing empty line (so a message that
// ends with "\n" doesn't produce a phantom empty line at the end).
func splitLines(msg string) []string {
	if msg == "" {
		return nil
	}
	lines := strings.Split(msg, "\n")
	// Drop a single trailing empty line, if any.
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// splitBodyAndFooters takes the lines after the header and splits
// them into body + footers. The body is everything up to the first
// line that looks like a footer ("Token: value" or "Token # value")
// at column 0. Footers continue until end-of-message. Returns ok=false
// if the slice is empty (no body, no footers).
func splitBodyAndFooters(rest []string) (body string, footers []string, ok bool) {
	if len(rest) == 0 {
		return "", nil, false
	}
	bodyLines := []string{}
	footerLines := []string{}
	inFooters := false
	first := true
	for _, line := range rest {
		if first {
			first = false
			if line == "" {
				// Blank line between header and body/footers.
				continue
			}
		}
		if !inFooters && looksLikeFooter(line) {
			inFooters = true
		}
		if inFooters {
			footerLines = append(footerLines, line)
		} else {
			bodyLines = append(bodyLines, line)
		}
	}
	body = strings.TrimRight(strings.Join(bodyLines, "\n"), "\n")
	return body, footerLines, true
}

// footerRe matches a single footer line: "Token: value" or
// "Token # value" with the token in kebab-case or PascalCase.
// "BREAKING CHANGE" and "BREAKING-CHANGE" are special-cased
// because the canonical token contains a space.
var footerRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9-]*|BREAKING[ -]CHANGE)(\s+#|:\s)`)

// looksLikeFooter reports whether line looks like a Conventional
// Commits footer (Token: value or Token # value). It is a heuristic;
// the spec says "footers ... should be denoted with a token followed
// by `:<space>` or ` #`".
func looksLikeFooter(line string) bool {
	if line == "" {
		return false
	}
	// Indented lines are body continuations, not footers.
	if line[0] == ' ' || line[0] == '\t' {
		return false
	}
	return footerRe.MatchString(line)
}

// Render re-formats v as a clean Conventional Commits message
// suitable for `git commit -F -`. The order is header / blank /
// body / blank / footers; empty sections are omitted. The trailing
// newline is included.
func (v Validated) Render() string {
	var b strings.Builder
	// Header
	b.WriteString(v.Type)
	if v.Scope != "" {
		b.WriteByte('(')
		b.WriteString(v.Scope)
		b.WriteByte(')')
	}
	if v.Breaking && !v.hasBangFooter() {
		// The BREAKING CHANGE footer is the more expressive form
		// (it carries a reason). We only emit `!` if there is no
		// footer to attach the reason to.
		b.WriteByte('!')
	}
	b.WriteString(": ")
	b.WriteString(strings.TrimRight(v.Subject, " "))
	b.WriteByte('\n')
	if v.Body != "" {
		b.WriteByte('\n')
		b.WriteString(strings.TrimRight(v.Body, "\n"))
		b.WriteByte('\n')
	}
	for _, f := range v.Footers {
		if f == "" {
			continue
		}
		b.WriteByte('\n')
		b.WriteString(f)
	}
	if !strings.HasSuffix(b.String(), "\n") {
		b.WriteByte('\n')
	}
	return b.String()
}

// hasBangFooter reports whether v already carries a BREAKING CHANGE
// footer (in which case Render will not also emit the "!" marker).
func (v Validated) hasBangFooter() bool {
	for _, f := range v.Footers {
		if isBreakingFooter(f) {
			return true
		}
	}
	return false
}
