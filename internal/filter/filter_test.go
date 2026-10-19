package filter

import "testing"

func TestTitleOK(t *testing.T) {
	tests := []struct {
		title string
		want  bool
	}{
		{"Software Engineer", true},
		{"QA Analyst", true},
		{"Desarrollador Full Stack", true},
		{"Senior Software Engineer", false},
		{"Sr. Backend Developer", false},
		{"Staff Engineer", false},
		{"Software Engineer III", false},
		{"Go Developer", false},
		{"Engineering Manager", false},
		{"Software Engineer, German speaking", false},
		{"Account Executive", false}, // no role word at all
		{"Software Engineering Intern", false},
		{"Internal Tools Engineer", true}, // "intern\b" must not hit "Internal"
	}
	for _, tt := range tests {
		if got := TitleOK(tt.title); got != tt.want {
			t.Errorf("TitleOK(%q) = %v, want %v", tt.title, got, tt.want)
		}
	}
}

func TestTier(t *testing.T) {
	tests := []struct{ loc, want string }{
		{"Remote - Spain", "SPAIN"},
		{"Málaga", "SPAIN"},
		{"Anywhere in the World", "WORLD"},
		{"Remote - All", "WORLD"},
		{"Remote (EMEA)", "EUROPE"},
		{"Lisbon, Portugal", "EUROPE"},
		{"LATAM", "LATAM"},
		{"Bogotá, Colombia", "COLOMBIA"},
		{"Cali", "COLOMBIA"},
		{"California", ""}, // "cali\b" needs a word boundary
		{"Remote - Americas", "AMERICAS"},
		{"Toronto, Ontario", "CANADA"},
		{"Montréal", "CANADA"},
		{"New York", ""},
		// Order matters: Spain beats Europe.
		{"Madrid | Europe", "SPAIN"},
	}
	for _, tt := range tests {
		if got := Tier(tt.loc); got != tt.want {
			t.Errorf("Tier(%q) = %q, want %q", tt.loc, got, tt.want)
		}
	}
}

func TestMinYears(t *testing.T) {
	tests := []struct {
		desc string
		want int
	}{
		{"", 0},
		{"3+ years of experience", 3},
		{"2-4 years with React, ideally 5 years", 2},
		{"3 to 5 yrs", 3},
		{"at least 4 years", 4}, // non-breaking space, common in Ashby text
		{"2 – 3 años de experiencia", 2},
		{"25 years in business, 0 years required", 0}, // outside 1..19
		{"We were founded 10 years ago; you need 7+ years", 7},
	}
	for _, tt := range tests {
		if got := MinYears(tt.desc); got != tt.want {
			t.Errorf("MinYears(%q) = %d, want %d", tt.desc, got, tt.want)
		}
	}
}

func TestFit(t *testing.T) {
	tests := []struct {
		name  string
		desc  string
		years int
		visa  string
		want  int
	}{
		{"empty", "", 0, "", 45},                                   // 40 base + 5 for no years
		{"skills", "React, TypeScript, Python and SQL", 2, "", 66}, // 40+16+10
		{"repeats count once", "react react REACT", 0, "", 49},
		{"gaps", "Kubernetes and Terraform", 3, "", 33}, // 40-12+5
		{"language", "Spanish speaker", 4, "", 48},
		{"sponsor", "", 5, VisaSponsor, 45},
		{"needs permit", "", 1, VisaNeedsPermit, 35},
		{"floor at 0", "kubernetes k8s terraform golang kotlin rails scala rust elixir", 5, VisaNeedsPermit, 0},
		// Python prototype quirk: "c#" only counts when a letter follows it.
		{"c# then space", "C# developer", 0, "", 45},
		{"c# then letter", "C#dev", 0, "", 49},
		{"skills capped at 10", "react typescript javascript python postman pytest sql supabase qa sso saml azure", 1, VisaSponsor, 100},
	}
	for _, tt := range tests {
		if got := Fit(tt.desc, tt.years, tt.visa); got != tt.want {
			t.Errorf("%s: Fit = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		p        Posting
		maxYears int
		wantOK   bool
		want     Result
	}{
		{
			name:   "remote Spain role",
			p:      Posting{Title: "Software Engineer", Loc: "Remote Spain", Desc: "2+ years of Python", Remote: true},
			wantOK: true, want: Result{Tier: "SPAIN", MinYears: 2, Fit: 54},
		},
		{
			name: "city without remote is dropped",
			p:    Posting{Title: "Software Engineer", Loc: "Barcelona", Desc: ""},
		},
		{
			name:   "worldwide needs no remote flag",
			p:      Posting{Title: "Support Engineer", Loc: "Anywhere", Desc: ""},
			wantOK: true, want: Result{Tier: "WORLD", Fit: 45},
		},
		{
			name: "hybrid in location",
			p:    Posting{Title: "QA Engineer", Loc: "Madrid (Hybrid)", Remote: true},
		},
		{
			name: "senior title",
			p:    Posting{Title: "Senior QA Engineer", Loc: "Remote Spain", Remote: true},
		},
		{
			name:   "plain Remote, Europe from description",
			p:      Posting{Title: "QA Engineer", Loc: "Remote", Desc: "Work in CET hours", Remote: true},
			wantOK: true, want: Result{Tier: "EUROPE?desc", Fit: 45},
		},
		{
			name:   "plain Remote, LATAM from description",
			p:      Posting{Title: "QA Engineer", Loc: "", Desc: "Team across Latin America", Remote: true},
			wantOK: true, want: Result{Tier: "LATAM?desc", Fit: 45},
		},
		{
			name:   "plain Remote, unknown region",
			p:      Posting{Title: "QA Engineer", Loc: " Remote ", Desc: "", Remote: true},
			wantOK: true, want: Result{Tier: "REMOTE?", Fit: 45},
		},
		{
			name: "plain Remote but US only",
			p:    Posting{Title: "QA Engineer", Loc: "Remote", Desc: "You must be located in the US.", Remote: true},
		},
		{
			name:   "Canada ignores the US-only rule",
			p:      Posting{Title: "QA Engineer", Loc: "Remote Canada", Desc: "Must be authorized to work in the US", Remote: true},
			wantOK: true, want: Result{Tier: "CANADA", Fit: 45},
		},
		{
			name: "too many years",
			p:    Posting{Title: "QA Engineer", Loc: "Remote Spain", Desc: "6+ years", Remote: true},
		},
		{
			name:     "--max-years 8 lets it through, and 6+ years scores like none",
			p:        Posting{Title: "QA Engineer", Loc: "Remote Spain", Desc: "6+ years", Remote: true},
			maxYears: 8, wantOK: true, want: Result{Tier: "SPAIN", MinYears: 6, Fit: 45},
		},
		{
			name: "needs EU work permit",
			p: Posting{Title: "QA Engineer", Loc: "Remote - EU", Remote: true,
				Desc: "You need the right to work in the EU."},
			wantOK: true, want: Result{Tier: "EUROPE", Visa: VisaNeedsPermit, Fit: 30},
		},
		{
			name: "offers sponsorship",
			p: Posting{Title: "QA Engineer", Loc: "Remote - EU", Remote: true,
				Desc: "We offer visa sponsorship and relocation support."},
			wantOK: true, want: Result{Tier: "EUROPE", Visa: VisaSponsor, Fit: 55},
		},
	}
	for _, tt := range tests {
		max := tt.maxYears
		if max == 0 {
			max = 5
		}
		got, ok := Classify(tt.p, max)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("%s: Classify = %+v, %v; want %+v, %v", tt.name, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestRank(t *testing.T) {
	order := []string{"SPAIN", "WORLD", "EUROPE", "EUROPE?desc", "LATAM", "LATAM?desc", "COLOMBIA", "AMERICAS", "REMOTE?", "CANADA"}
	for i := 1; i < len(order); i++ {
		if Rank(order[i-1]) >= Rank(order[i]) {
			t.Errorf("Rank(%s) should be below Rank(%s)", order[i-1], order[i])
		}
	}
}
