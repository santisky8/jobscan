// Package pyjson writes JSON strings exactly like Python's json.dumps with
// default settings (non-ASCII characters escaped as \uXXXX), so orgs.json
// keeps the format of the original Python prototype.
package pyjson

import (
	"fmt"
	"strings"
)

// Quote returns s as a JSON string literal with non-ASCII characters escaped
// (Python's ensure_ascii=True).
func Quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r < 0x20 || (r > 0x7e && r <= 0xffff):
			fmt.Fprintf(&b, `\u%04x`, r)
		case r > 0xffff: // outside the BMP: a UTF-16 surrogate pair
			r -= 0x10000
			fmt.Fprintf(&b, `\u%04x\u%04x`, 0xd800+(r>>10), 0xdc00+(r&0x3ff))
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
