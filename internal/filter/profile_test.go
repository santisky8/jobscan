package filter

import (
	"os"
	"path/filepath"
	"testing"
)

func writeProfile(t *testing.T, json string) *Profile {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(path, []byte(json), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestEmptyProfileIsDefault(t *testing.T) {
	p := writeProfile(t, `{}`)
	if p.MaxYears != 5 {
		t.Errorf("MaxYears = %d, want 5", p.MaxYears)
	}
	for _, loc := range []string{"Remote - Spain", "Worldwide", "Toronto, Canada", "Berlin"} {
		if got, want := p.Tier(loc), Tier(loc); got != want {
			t.Errorf("Tier(%q) = %q, want %q", loc, got, want)
		}
	}
	desc := "React, TypeScript and SQL. 2 years. Spanish a plus."
	if got, want := p.Fit(desc, 2, ""), Fit(desc, 2, ""); got != want {
		t.Errorf("Fit = %d, want %d", got, want)
	}
}

func TestCustomProfile(t *testing.T) {
	p := writeProfile(t, `{
		"max_years": 3,
		"tiers": [
			{"name": "UK", "location": "united kingdom|\\buk\\b|london", "remote_required": true},
			{"name": "WORLD", "location": "worldwide|anywhere"}
		],
		"description_guesses": [{"name": "UK?desc", "description": "\\bgmt\\b|\\buk\\b"}],
		"skills": ["go", "c#", "postgres"],
		"missing_skills": ["java"],
		"languages": []
	}`)
	if p.MaxYears != 3 {
		t.Errorf("MaxYears = %d, want 3", p.MaxYears)
	}
	if got := p.Tier("London, UK"); got != "UK" {
		t.Errorf("Tier = %q, want UK", got)
	}
	if got := p.Tier("Remote - Spain"); got != "" {
		t.Errorf("Tier(Spain) = %q, want none", got)
	}
	if !(p.Rank("UK") < p.Rank("WORLD") && p.Rank("WORLD") < p.Rank("UK?desc") &&
		p.Rank("UK?desc") < p.Rank(RemoteGuess) && p.Rank(RemoteGuess) < p.Rank("SPAIN")) {
		t.Error("rank should follow tiers, then guesses, then REMOTE?, then unknown")
	}

	// 40 + 3 skills*4 - 1 missing*6 + 5 (no years); "go" must not match "google".
	if got := p.Fit("We use Go, C# and Postgres, some Java. Google Cloud.", 0, ""); got != 51 {
		t.Errorf("Fit = %d, want 51", got)
	}
	// An empty languages list gives no language bonus.
	if got := p.Fit("Spanish speaker", 0, ""); got != 45 {
		t.Errorf("Fit = %d, want 45", got)
	}

	r, ok := p.Classify(Posting{Title: "Software Engineer", Loc: "Remote", Desc: "Hours overlap with GMT.", Remote: true}, p.MaxYears)
	if !ok || r.Tier != "UK?desc" {
		t.Errorf("Classify = %+v, %v; want tier UK?desc", r, ok)
	}
	if _, ok := p.Classify(Posting{Title: "Software Engineer", Loc: "London, UK", Desc: "On our team."}, p.MaxYears); ok {
		t.Error("a non-remote role in a remote_required tier should be dropped")
	}
}

func TestBadProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	for _, body := range []string{
		`{"tiers": [{"name": "X", "location": "("}]}`,
		`{"tiers": [{"name": "X", "location": ""}]}`,
		`{"unknown_field": 1}`,
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadProfile(path); err == nil {
			t.Errorf("LoadProfile(%s) should fail", body)
		}
	}
}
