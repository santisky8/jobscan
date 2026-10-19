// Package textutil holds small text helpers that reproduce the Python
// scanners' string handling: HTML to text, whitespace collapsing and
// rune-safe truncation.
//
// Python's \s, str.strip() and str.split() treat a wider set of characters
// as whitespace than Go's regexp \s (notably the non-breaking space U+00A0,
// which job descriptions are full of). Everything here uses Python's set so
// both versions see the same text.
package textutil

import (
	"html"
	"regexp"
	"strings"

	"github.com/santisky8/jobscan/internal/pyre"
)

var (
	spaceRun = regexp.MustCompile(pyre.SpaceClass + `+`)
	tag      = regexp.MustCompile(`<[^>]+>`)
)

// IsSpace reports whether r is whitespace for Python's str methods
// (the same set as pyre.SpaceClass).
func IsSpace(r rune) bool {
	switch {
	case r == ' ', r >= '\t' && r <= '\r', r >= 0x1c && r <= 0x1f:
		return true
	case r == 0x85, r == 0xa0, r == 0x1680, r >= 0x2000 && r <= 0x200a,
		r == 0x2028, r == 0x2029, r == 0x202f, r == 0x205f, r == 0x3000:
		return true
	}
	return false
}

// TrimSpace is Python's s.strip().
func TrimSpace(s string) string { return strings.TrimFunc(s, IsSpace) }

// Fields is Python's s.split() with no arguments.
func Fields(s string) []string { return strings.FieldsFunc(s, IsSpace) }

// CollapseSpace replaces every run of whitespace with one space
// (Python: re.sub(r"\s+", " ", s)). It does not trim the ends.
func CollapseSpace(s string) string { return spaceRun.ReplaceAllString(s, " ") }

// Strip turns an HTML-escaped job description into one line of plain text:
// unescape entities, replace tags with spaces, collapse whitespace.
// This is the prototype's strip(), used for ATS descriptions.
func Strip(s string) string {
	return CollapseSpace(tag.ReplaceAllString(html.UnescapeString(s), " "))
}

// Truncate returns at most n runes of s (Python: s[:n]).
func Truncate(s string, n int) string {
	i := 0
	for pos := range s {
		if i == n {
			return s[:pos]
		}
		i++
	}
	return s
}
