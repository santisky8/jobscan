package pyre

import (
	"slices"
	"testing"
)

func TestUnicodeWordBoundary(t *testing.T) {
	ai := MustCompile(`(?i)\bai\b`)
	tests := []struct {
		s    string
		want bool
	}{
		{"AI engineer", true},
		{"engineer, AI", true}, // boundary at the end of the text
		{"AI", true},
		{"AI広告", false}, // 広 is a word character in Python
		{"maid", false},
		{"(AI)", true},
	}
	for _, tt := range tests {
		if got := ai.MatchString(tt.s); got != tt.want {
			t.Errorf("\\bai\\b on %q = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestSpaceAndWord(t *testing.T) {
	if !MustCompile(`3\s+years`).MatchString("3 years") {
		t.Error(`\s should match a non-breaking space`)
	}
	got := MustCompile(`\b(prompt\w*)\b`).FindAllGroups("promptné x")
	if !slices.Equal(got, []string{"promptné"}) {
		t.Errorf(`\w should include accented letters, got %q`, got)
	}
}

func TestFindAllGroupsAdjacent(t *testing.T) {
	// Matches separated by a single character share a boundary character;
	// both must be found, like Python's finditer.
	re := MustCompile(`(?i)\b(react|sql|qa)\b`)
	got := re.FindAllGroups("React,SQL QA")
	if want := []string{"React", "SQL", "QA"}; !slices.Equal(got, want) {
		t.Errorf("FindAllGroups = %q, want %q", got, want)
	}
	// Patterns without \b behave like ordinary FindAll.
	years := MustCompile(`(\d{1,2})\s*years`)
	if got := years.FindAllGroups("2 years, 10 years"); !slices.Equal(got, []string{"2", "10"}) {
		t.Errorf("years = %q", got)
	}
}

func TestClassPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error(`\s inside [...] should panic`)
		}
	}()
	MustCompile(`[\s-]`)
}
