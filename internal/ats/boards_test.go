package ats

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/santisky8/jobscan/internal/store"
)

// The expected values below were produced by running the Python prototype's fetchers on
// the same fixture files.

func load(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestParseBoards(t *testing.T) {
	tests := []struct {
		board Board
		file  string
		want  []Job
	}{
		{Board{"greenhouse", "acme"}, "greenhouse.json", []Job{
			{ATS: "greenhouse", Org: "acme", Title: "QA Engineer", Loc: "Remote - Spain", Remote: true,
				Posted: mustTime(t, "2026-10-15T09:30:00-04:00"), URL: "https://boards.greenhouse.io/acme/jobs/1",
				Desc: " 3+ years&nbsp;of Python "},
			// null location and first_published fall back to "" and updated_at.
			{ATS: "greenhouse", Org: "acme", Title: "Office Manager",
				Posted: mustTime(t, "2026-10-16T10:00:00Z"), URL: "https://boards.greenhouse.io/acme/jobs/2"},
		}},
		{Board{"ashby", "acme"}, "ashby.json", []Job{
			{ATS: "ashby", Org: "acme", Title: "Support Engineer", Loc: "Madrid | Remote - EU", Remote: true,
				Posted: mustTime(t, "2026-09-18T08:41:19.683Z"), URL: "https://jobs.ashbyhq.com/acme/abc", Desc: "Plain text"},
			{ATS: "ashby", Org: "acme", Title: "Designer",
				Posted: time.UnixMilli(12345), URL: "https://jobs.ashbyhq.com/acme/def"},
		}},
		{Board{"lever", "acme"}, "lever.json", []Job{
			{ATS: "lever", Org: "acme", Title: "Backend Developer", Loc: "Bogotá | Remote - LATAM", Remote: true,
				Posted: time.UnixMilli(1760000000000), URL: "https://jobs.lever.co/acme/1",
				Desc: "Intro Requirements:  2 years SQL  Benefits"},
		}},
		{Board{"lever", "acme"}, "lever-error.json", nil},
	}
	for _, tt := range tests {
		got := parse(tt.board, load(t, tt.file))
		if len(got) != len(tt.want) {
			t.Fatalf("%s: got %d jobs, want %d: %+v", tt.file, len(got), len(tt.want), got)
		}
		for i := range got {
			g, w := got[i], tt.want[i]
			if !g.Posted.Equal(w.Posted) {
				t.Errorf("%s[%d]: Posted = %v, want %v", tt.file, i, g.Posted, w.Posted)
			}
			g.Posted, w.Posted = time.Time{}, time.Time{}
			if g != w {
				t.Errorf("%s[%d]:\n got %+v\nwant %+v", tt.file, i, g, w)
			}
		}
	}
}

func TestParseTime(t *testing.T) {
	tests := []struct {
		in   any
		want string // RFC 3339, "" for zero
	}{
		{"2026-10-05T08:34:12.123+00:00", "2026-10-05T08:34:12.123Z"},
		{"2026-10-05T08:34:12.1234567Z", "2026-10-05T08:34:12.123456Z"}, // Python keeps microseconds
		{"2026-10-05T08:34:12-04:00", "2026-10-05T12:34:12Z"},
		{"2026-10-05T08:34:12", "2026-10-05T08:34:12Z"}, // no zone means UTC
		{"2026-10-05 08:34", "2026-10-05T08:34:00Z"},
		{"2026-10-05", "2026-10-05T00:00:00Z"},
		{float64(1760000000000), "2025-10-09T08:53:20Z"},
		{"not a date", ""},
		{nil, ""},
	}
	for _, tt := range tests {
		got := ParseTime(tt.in)
		s := ""
		if !got.IsZero() {
			s = got.UTC().Format(time.RFC3339Nano)
		}
		if s != tt.want {
			t.Errorf("ParseTime(%v) = %q, want %q", tt.in, s, tt.want)
		}
	}
}

func TestMatch(t *testing.T) {
	now := time.Now()
	jobs := []Job{
		{ATS: "ashby", Org: "a", Title: "QA Engineer", Loc: "Remote Spain", Remote: true, Posted: now, URL: "u1"},
		{ATS: "ashby", Org: "a", Title: "QA Engineer", Loc: "Remote Spain", Remote: true, Posted: now, URL: "u1"}, // duplicate
		{ATS: "ashby", Org: "a", Title: "QA Engineer", Loc: "Remote Spain", Remote: true, Posted: now.Add(-48 * time.Hour), URL: "old"},
		{ATS: "ashby", Org: "a", Title: "QA Engineer", Loc: "Remote Spain", Remote: true, URL: "no-date"},
		{ATS: "ashby", Org: "a", Title: "QA Engineer", Loc: "Remote Spain", Remote: true, Posted: now, URL: "seen"},
		{ATS: "ashby", Org: "a", Title: "Senior QA", Loc: "Remote Spain", Remote: true, Posted: now, URL: "senior"},
	}
	seen := map[string]bool{"seen": true}
	cutoff := now.Add(-24 * time.Hour)

	rows, matched := match(jobs, seen, cutoff, Options{MaxYears: 5})
	if len(rows) != 1 || rows[0].URL != "u1" || len(matched) != 2 {
		t.Errorf("new only: rows %+v, matched %v", rows, matched)
	}
	rows, _ = match(jobs, seen, cutoff, Options{MaxYears: 5, All: true})
	if len(rows) != 2 {
		t.Errorf("--all: got %d rows, want 2 (u1 and seen)", len(rows))
	}
}

func TestAddOrg(t *testing.T) {
	dir := t.TempDir()
	d := store.Dir(dir)
	if err := os.WriteFile(d.OrgsPath(), []byte(`{"ashby": ["b", "d"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	orgs, err := store.LoadOrgs(d.OrgsPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := addOrg(orgs, Options{Dir: d, AddOrg: "ashby:c"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(d.OrgsPath())
	if want := "{\n \"ashby\": [\n  \"b\",\n  \"c\",\n  \"d\"\n ]\n}"; string(got) != want {
		t.Errorf("orgs.json =\n%s\nwant\n%s", got, want)
	}
	if err := addOrg(orgs, Options{Dir: d, AddOrg: "nocolon"}, io.Discard); err == nil {
		t.Error("want an error for a value without ats:")
	}
}
