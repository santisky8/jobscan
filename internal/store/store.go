// Package store reads and writes the scanner's files: orgs.json, the seen
// list and the CSV reports. The formats match the original Python prototype
// byte for byte.
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/santisky8/jobscan/internal/pyjson"
	"github.com/santisky8/jobscan/internal/textutil"
)

// Dir is the data folder (by default ./jobscan-data).
type Dir string

func (d Dir) OrgsPath() string { return filepath.Join(string(d), "orgs.json") }
func (d Dir) SeenPath() string { return filepath.Join(string(d), "seen.txt") }
func (d Dir) OutDir() string   { return filepath.Join(string(d), "out") }

// Today is the local date used in report file names, e.g. "2026-10-18".
func Today() string { return time.Now().Format(time.DateOnly) }

// Orgs is orgs.json: board slugs per ATS ({"greenhouse": [...], ...}).
// The order of the ATS keys is kept, because it decides the order boards
// are scanned in and therefore how ties sort in the report.
type Orgs struct {
	keys   []string
	boards map[string][]string
}

// LoadOrgs reads orgs.json.
func LoadOrgs(path string) (*Orgs, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Read the top-level object token by token to keep key order
	// (a plain map would lose it).
	dec := json.NewDecoder(bytes.NewReader(data))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, fmt.Errorf("%s: expected a JSON object", path)
	}
	o := &Orgs{boards: map[string][]string{}}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		key := t.(string) // object keys are always strings
		var list []string
		if err := dec.Decode(&list); err != nil {
			return nil, fmt.Errorf("%s: %q: %w", path, key, err)
		}
		if _, dup := o.boards[key]; !dup {
			o.keys = append(o.keys, key)
		}
		o.boards[key] = list
	}
	return o, nil
}

// Keys returns the ATS names in file order.
func (o *Orgs) Keys() []string { return o.keys }

// Boards returns the slugs listed for an ATS.
func (o *Orgs) Boards(ats string) []string { return o.boards[ats] }

// Add inserts slugs for an ATS, keeping each list sorted and unique.
// It returns how many were new.
func (o *Orgs) Add(ats string, slugs ...string) int {
	list, exists := o.boards[ats]
	added := 0
	for _, s := range slugs {
		if !slices.Contains(list, s) {
			list = append(list, s)
			added++
		}
	}
	if added == 0 {
		return 0
	}
	if !exists {
		o.keys = append(o.keys, ats)
	}
	slices.Sort(list)
	o.boards[ats] = slices.Compact(list)
	return added
}

// Save writes orgs.json the way Python's json.dump(orgs, f, indent=1) does.
func (o *Orgs) Save(path string) error {
	var b strings.Builder
	b.WriteString("{")
	for i, k := range o.keys {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "\n %s: ", pyjson.Quote(k))
		list := o.boards[k]
		if len(list) == 0 {
			b.WriteString("[]")
			continue
		}
		b.WriteString("[")
		for j, s := range list {
			if j > 0 {
				b.WriteString(",")
			}
			b.WriteString("\n  " + pyjson.Quote(s))
		}
		b.WriteString("\n ]")
	}
	b.WriteString("\n}")
	// Write a temporary file and rename it, so a crash never leaves a
	// half-written orgs.json.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadSeen reads a seen file (one key per line) into a set.
// A missing file is an empty set.
func LoadSeen(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, k := range textutil.Fields(string(data)) {
		seen[k] = true
	}
	return seen, nil
}

// AppendLines appends one line per key, creating the file if needed.
func AppendLines(path string, keys []string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if _, err := io.WriteString(f, k+"\n"); err != nil {
			f.Close()
			return err
		}
	}
	return f.Close()
}

// WriteCSV writes rows to path. With appendTo it appends (creating the file
// if needed), otherwise it truncates. The header is written when
// writeHeader is true.
func WriteCSV(path string, header []string, rows [][]string, appendTo, writeHeader bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	mode := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if appendTo {
		mode = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	f, err := os.OpenFile(path, mode, 0o644)
	if err != nil {
		return err
	}
	var b strings.Builder
	if writeHeader {
		writeRecord(&b, header)
	}
	for _, r := range rows {
		writeRecord(&b, r)
	}
	if _, err := io.WriteString(f, b.String()); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// writeRecord writes one CSV line in Python's csv "excel" dialect.
// encoding/csv is close but not identical: it quotes fields that start with
// a space and rewrites embedded newlines when using CRLF. Python quotes only
// fields containing a comma, quote, CR or LF, and ends lines with CRLF.
func writeRecord(b *strings.Builder, fields []string) {
	for i, f := range fields {
		if i > 0 {
			b.WriteByte(',')
		}
		if strings.ContainsAny(f, ",\"\r\n") {
			b.WriteString(`"` + strings.ReplaceAll(f, `"`, `""`) + `"`)
		} else {
			b.WriteString(f)
		}
	}
	b.WriteString("\r\n")
}

// Exists reports whether path exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
