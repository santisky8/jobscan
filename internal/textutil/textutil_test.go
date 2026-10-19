package textutil

import (
	"slices"
	"testing"
)

func TestStrip(t *testing.T) {
	in := "&lt;p&gt;Hello&lt;/p&gt;\n\n&lt;ul&gt;&lt;li&gt;A&amp;B&lt;/li&gt;&lt;/ul&gt;"
	if got, want := Strip(in), " Hello A&B "; got != want {
		t.Errorf("Strip = %q, want %q", got, want)
	}
}

func TestPythonWhitespace(t *testing.T) {
	if got := TrimSpace("  x 　"); got != "x" {
		t.Errorf("TrimSpace = %q", got)
	}
	if got := Fields("a b  c"); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("Fields = %q", got)
	}
	if got := CollapseSpace("a  \n b"); got != "a b" {
		t.Errorf("CollapseSpace = %q", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("Bogotá, Colombia", 6); got != "Bogotá" {
		t.Errorf("Truncate = %q (must count runes, not bytes)", got)
	}
	if got := Truncate("short", 50); got != "short" {
		t.Errorf("Truncate = %q", got)
	}
}
