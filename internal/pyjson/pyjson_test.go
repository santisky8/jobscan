package pyjson

import "testing"

func TestQuote(t *testing.T) {
	// Expected strings are Python's json.dumps(s).
	tests := []struct{ in, want string }{
		{"posthog", `"posthog"`},
		{"España", `"Espa\u00f1a"`},
		{"tab\tquote\" 😀", `"tab\tquote\" \ud83d\ude00"`},
		{"", `""`},
	}
	for _, tt := range tests {
		if got := Quote(tt.in); got != tt.want {
			t.Errorf("Quote(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}
