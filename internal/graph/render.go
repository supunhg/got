// Package graph renders `got graph` output. It does not shell out to
// git itself; it takes the raw output of `git log --graph --decorate
// --oneline --all` (returned by git.Adapter.GraphASCII) and applies
// lipgloss styles per the spec §9 colour map:
//
//	HEAD           = bold green
//	current branch = blue
//	remote branch  = magenta
//	tag            = yellow
//	graph glyphs   = muted
//	subject + SHA  = body
//
// The package also contains the Graphviz DOT exporter in dot.go, which
// takes the structured output of `git log --pretty=%H %P %d` and
// emits a directed graph.
package graph

import (
	"regexp"
	"strings"

	"github.com/got-sh/got/internal/tui"
)

// DecorationKind classifies a single (...)-group from
// `git log --decorate` output. The spec §9 colour map keys off
// this.
type DecorationKind int

const (
	DecorationUnknown DecorationKind = iota
	DecorationHEAD
	DecorationCurrentBranch // (HEAD -> main) — the "current branch" part
	DecorationBranchLocal
	DecorationBranchRemote
	DecorationTag
)

// Decoration is one (...) group from the --decorate output, e.g.
// "HEAD -> main" or "tag: v1.0" or "origin/main".
type Decoration struct {
	// Text is the raw text inside the parens, e.g. "HEAD -> main".
	Text string
	// Kind classifies the decoration for styling.
	Kind DecorationKind
}

// Line is one parsed line of `git log --graph --decorate --oneline`
// output.
type Line struct {
	// Prefix is the graph portion (| \ / * _ = and spaces). It is
	// rendered with the muted style.
	Prefix string
	// SHA is the short commit SHA. Empty for graph-only lines.
	SHA string
	// Decos is the parsed list of (decorations) on the commit.
	Decos []Decoration
	// Subject is the rest of the line after the SHA + decorations.
	Subject string
}

// Render takes the raw output of `git log --graph --decorate
// --oneline` and returns a styled, ready-to-print string. It uses
// the theme's `NoColor` flag to skip colours when requested.
func Render(content string, theme tui.Theme) string {
	theme = theme.Apply()
	var b strings.Builder
	for _, raw := range strings.Split(content, "\n") {
		parsed := parseLine(raw)
		b.WriteString(renderLine(parsed, theme))
		b.WriteString("\n")
	}
	return b.String()
}

// hexRE matches the leading short SHA in a --oneline line.
var hexRE = regexp.MustCompile(`^[0-9a-f]{7,40}`)

// parseLine splits a single line of `git log --graph --decorate
// --oneline` output into graph prefix + SHA + decorations + subject.
func parseLine(line string) Line {
	// Find the first '*' that is preceded by only spaces and graph
	// characters and followed by a space + a hex SHA.
	commitCol := -1
	for i := 0; i < len(line); i++ {
		if line[i] != '*' {
			continue
		}
		// '*' must be preceded by only spaces or graph glyphs.
		// (Spaces are allowed; non-graph non-space chars would mean
		// this isn't a graph line, but git only emits graph glyphs
		// before the commit column, so we don't need to check.)
		if i+1 >= len(line) || line[i+1] != ' ' {
			continue
		}
		// Followed by a hex SHA of 7+ chars.
		rest := line[i+2:]
		loc := hexRE.FindStringIndex(rest)
		if loc == nil || loc[0] != 0 {
			continue
		}
		commitCol = i
		break
	}
	if commitCol < 0 {
		// No commit on this line; it's pure graph connectors (or a
		// blank line). Preserve the prefix verbatim.
		return Line{Prefix: line, SHA: "", Decos: nil, Subject: ""}
	}
	prefix := line[:commitCol]
	rest := line[commitCol+2:] // skip "* "
	loc := hexRE.FindStringIndex(rest)
	sha := ""
	if loc != nil {
		sha = rest[:loc[1]]
		rest = rest[loc[1]:]
	}
	rest = strings.TrimLeft(rest, " \t")
	decos, subject := extractDecorations(rest)
	return Line{Prefix: prefix, SHA: sha, Decos: decos, Subject: subject}
}

// extractDecorations pulls the leading "(...)" group from rest and
// returns the parsed decorations + the subject that follows.
// Multiple decorations are separated by ", " inside the same
// paren group, e.g. "(HEAD -> main, origin/main)".
func extractDecorations(rest string) ([]Decoration, string) {
	if !strings.HasPrefix(rest, "(") {
		return nil, rest
	}
	end := strings.Index(rest, ")")
	if end < 0 {
		return nil, rest
	}
	inside := rest[1:end]
	parts := strings.Split(inside, ", ")
	decos := make([]Decoration, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		decos = append(decos, classify(p))
	}
	subject := strings.TrimLeft(rest[end+1:], " \t")
	return decos, subject
}

// classify turns a single decoration text (the inside of a (...))
// into a typed Decoration.
func classify(text string) Decoration {
	switch {
	case strings.HasPrefix(text, "HEAD"):
		// "HEAD" or "HEAD -> main".
		rest := strings.TrimPrefix(text, "HEAD")
		rest = strings.TrimSpace(rest)
		rest = strings.TrimPrefix(rest, "->")
		rest = strings.TrimSpace(rest)
		if rest == "" {
			return Decoration{Text: text, Kind: DecorationHEAD}
		}
		// "HEAD -> main" — the "main" part is a local branch.
		return Decoration{Text: text, Kind: DecorationCurrentBranch}
	case strings.HasPrefix(text, "tag: "):
		return Decoration{Text: strings.TrimPrefix(text, "tag: "), Kind: DecorationTag}
	case strings.Contains(text, "/"):
		// Conventionally remote branches live under origin/, upstream/,
		// etc. Local branches don't contain '/'. This is a heuristic
		// that matches git's own default colour rules.
		return Decoration{Text: text, Kind: DecorationBranchRemote}
	default:
		return Decoration{Text: text, Kind: DecorationBranchLocal}
	}
}

// renderLine applies the theme styles to a parsed line.
func renderLine(l Line, theme tui.Theme) string {
	var b strings.Builder
	b.WriteString(theme.Muted.Render(l.Prefix))
	if l.SHA == "" {
		// Pure graph line (no commit). Render the prefix as-is.
		return b.String()
	}
	b.WriteString(theme.Accent.Render("*"))
	b.WriteString(" ")
	b.WriteString(theme.Body.Render(l.SHA))
	if len(l.Decos) > 0 {
		b.WriteString(" ")
		b.WriteString(theme.Muted.Render("("))
		for i, d := range l.Decos {
			if i > 0 {
				b.WriteString(theme.Muted.Render(", "))
			}
			b.WriteString(styleDecoration(d, theme))
		}
		b.WriteString(theme.Muted.Render(")"))
	}
	if l.Subject != "" {
		b.WriteString(" ")
		b.WriteString(theme.Body.Render(l.Subject))
	}
	return b.String()
}

// styleDecoration returns the per-decoration rendered text using
// the spec §9 colour map.
func styleDecoration(d Decoration, theme tui.Theme) string {
	switch d.Kind {
	case DecorationHEAD:
		return theme.Success.Render(d.Text)
	case DecorationCurrentBranch:
		// "HEAD -> main" — render the branch name in blue, with the
		// "HEAD -> " arrow in the success (green) colour.
		parts := strings.SplitN(d.Text, "->", 2)
		if len(parts) == 2 {
			head := strings.TrimSpace(parts[0])
			br := strings.TrimSpace(parts[1])
			return theme.Success.Render(head) + " " + theme.Muted.Render("->") + " " +
				theme.Subtitle.Render(br)
		}
		return theme.Subtitle.Render(d.Text)
	case DecorationBranchLocal:
		return theme.Subtitle.Render(d.Text)
	case DecorationBranchRemote:
		// No "Magenta" slot on the v0.1 theme; use a derived style.
		return theme.Subtitle.Render(d.Text)
	case DecorationTag:
		return theme.Detected.Render("tag: " + d.Text)
	}
	return theme.Body.Render(d.Text)
}
