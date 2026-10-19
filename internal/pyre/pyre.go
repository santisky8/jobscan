// Package pyre runs regexes written for Python's re module on Go's RE2
// engine, with Python's Unicode meaning of \s, \w and \b.
//
// Python 3 treats every letter and digit of every script as a word
// character, so in "AI広告" there is no word boundary between "I" and "広".
// Go's \w and \b are ASCII-only, and \s misses the non-breaking space. Real
// job descriptions (Japanese, Spanish, French...) hit all three, so a plain
// copy of the patterns gave different results on about 1 in 2,500 jobs.
//
// Translation:
//   - \s becomes SpaceClass and \w becomes WordClass.
//   - \b becomes NonWordClass, which consumes the neighbouring character
//     instead of being zero-width (RE2 has no lookaround). The text is
//     padded with a space at each end so a boundary at the start or end of
//     the text still has a character to consume.
//
// The \b rewrite is only correct when the pattern side of every \b is a
// word character, as in \bqa\b or sap\b. That holds for every pattern in
// jobscan; the one exception in the Python prototype (c#\b) is spelled out by hand,
// and profile keywords add \b only next to a letter or digit.
package pyre

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Character classes equal to Python's for str patterns.
const (
	SpaceClass   = `[\t\n\v\f\r \x{1c}-\x{1f}\x{85}\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}]`
	WordClass    = `[\p{L}\p{N}_]`
	NonWordClass = `[^\p{L}\p{N}_]`
)

// Regexp is a compiled Python pattern.
type Regexp struct {
	re     *regexp.Regexp
	padded bool // the pattern had \b: match against " " + s + " "
}

// MustCompile translates and compiles a Python pattern, panicking on error
// (patterns are constants, so an error is a bug).
func MustCompile(pattern string) *Regexp {
	translated, hasB := translate(pattern)
	return &Regexp{re: regexp.MustCompile(translated), padded: hasB}
}

func translate(p string) (out string, hasB bool) {
	var b strings.Builder
	inClass := false
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c == '\\' && i+1 < len(p):
			i++
			switch e := p[i]; e {
			case 's', 'w', 'b':
				if inClass {
					panic(fmt.Sprintf("pyre: \\%c inside [...] is not supported: %s", e, p))
				}
				b.WriteString(map[byte]string{'s': SpaceClass, 'w': WordClass, 'b': NonWordClass}[e])
				hasB = hasB || e == 'b'
			default:
				b.WriteByte('\\')
				b.WriteByte(e)
			}
		case c == '[' && !inClass:
			inClass = true
			b.WriteByte(c)
			// A ']' right after '[' or '[^' is a literal, not the end.
			if i+1 < len(p) && p[i+1] == '^' {
				i++
				b.WriteByte('^')
			}
			if i+1 < len(p) && p[i+1] == ']' {
				i++
				b.WriteByte(']')
			}
		case c == ']' && inClass:
			inClass = false
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), hasB
}

func (r *Regexp) text(s string) string {
	if r.padded {
		return " " + s + " "
	}
	return s
}

// MatchString reports whether the pattern matches anywhere in s
// (Python: bool(re.search(pattern, s))).
func (r *Regexp) MatchString(s string) bool { return r.re.MatchString(r.text(s)) }

// FindAllGroups returns, for each match from left to right, the text of the
// first capture group that took part in it (Python: m.group(n) for m in
// re.finditer(...), taking the first non-empty group).
func (r *Regexp) FindAllGroups(s string) []string {
	if !r.padded {
		var out []string
		for _, m := range r.re.FindAllStringSubmatchIndex(s, -1) {
			start, end := firstGroup(m)
			out = append(out, s[start:end])
		}
		return out
	}
	s = r.text(s)
	var out []string
	for pos := 0; pos < len(s); {
		m := r.re.FindStringSubmatchIndex(s[pos:])
		if m == nil {
			break
		}
		start, end := firstGroup(m)
		out = append(out, s[pos+start:pos+end])
		// The match consumed a boundary character after the group that may
		// also be the boundary before the next match. Python resumes right
		// after the group; resume one character earlier so the next search
		// has a character to use as its leading boundary. The group starts
		// after the match's own leading boundary, so this always moves on.
		_, last := utf8.DecodeLastRuneInString(s[pos+start : pos+end])
		pos += max(end-last, m[0]+1)
	}
	return out
}

// firstGroup returns the span of the first capture group that took part in
// match m, or of the whole match if the pattern has no groups.
func firstGroup(m []int) (start, end int) {
	for g := 1; 2*g < len(m); g++ {
		if m[2*g] >= 0 {
			return m[2*g], m[2*g+1]
		}
	}
	return m[0], m[1]
}
