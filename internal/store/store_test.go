package store

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestWriteCSV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out", "x.csv")
	header := []string{"a", "b"}
	rows := [][]string{{" lead space", `say "hi", ok`}, {"line\nbreak", ""}}
	if err := WriteCSV(path, header, rows, true, true); err != nil {
		t.Fatal(err)
	}
	if err := WriteCSV(path, header, [][]string{{"1", "2"}}, true, false); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	// Python's csv module: CRLF endings, quotes only when needed.
	want := "a,b\r\n lead space,\"say \"\"hi\"\", ok\"\r\n\"line\nbreak\",\r\n1,2\r\n"
	if string(got) != want {
		t.Errorf("csv =\n%q\nwant\n%q", got, want)
	}
	if err := WriteCSV(path, header, nil, false, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "a,b\r\n" {
		t.Errorf("truncate mode left %q", got)
	}
}

func TestOrgsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orgs.json")
	// Key order must survive: lever before ashby here.
	if err := os.WriteFile(path, []byte(`{"lever": ["b"], "ashby": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	o, err := LoadOrgs(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := o.Add("ashby", "z", "a", "z"); n != 2 {
		t.Errorf("Add returned %d, want 2", n)
	}
	if n := o.Add("lever", "b"); n != 0 {
		t.Errorf("re-adding returned %d, want 0", n)
	}
	if n := o.Add("greenhouse"); n != 0 || slices.Contains(o.Keys(), "greenhouse") {
		t.Error("adding nothing must not create a key")
	}
	if err := o.Save(path); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := "{\n \"lever\": [\n  \"b\"\n ],\n \"ashby\": [\n  \"a\",\n  \"z\"\n ]\n}"
	if string(got) != want {
		t.Errorf("saved =\n%s\nwant\n%s", got, want)
	}
}

func TestSeen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seen.txt")
	if s, err := LoadSeen(path); err != nil || len(s) != 0 {
		t.Fatalf("missing file: %v %v", s, err)
	}
	if err := AppendLines(path, []string{"u1", "u2"}); err != nil {
		t.Fatal(err)
	}
	if err := AppendLines(path, []string{"u3"}); err != nil {
		t.Fatal(err)
	}
	s, _ := LoadSeen(path)
	if len(s) != 3 || !s["u1"] || !s["u3"] {
		t.Errorf("seen = %v", s)
	}
}
