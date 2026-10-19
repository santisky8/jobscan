package filter

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/santisky8/jobscan/internal/pyre"
)

// Profile is a compiled search profile: where the candidate can work, in
// which order those places are preferred, and which skills raise or lower
// the fit score. Build one with LoadProfile, or use Default.
type Profile struct {
	MaxYears int // default for the --max-years flag

	tiers   []tierRule
	guesses []guess
	rank    map[string]int
	have    *pyre.Regexp
	lack    *pyre.Regexp
	lang    *pyre.Regexp
}

type tierRule struct {
	name           string
	re             *pyre.Regexp
	remoteRequired bool // the place counts only for a remote role
	allowUSOnly    bool // keep the role even if the description says US only
}

type guess struct {
	name string
	re   *pyre.Regexp
}

// ProfileFile is the JSON form of a profile (see examples/profile.json).
// Every field is optional; a missing field keeps the built-in value.
type ProfileFile struct {
	MaxYears int `json:"max_years,omitempty"`

	// Tiers are checked in order against the location; the first match wins.
	Tiers []TierFile `json:"tiers,omitempty"`

	// DescriptionGuesses name the tier of a remote role with no place in its
	// location, from words in its description. The first match wins.
	DescriptionGuesses []GuessFile `json:"description_guesses,omitempty"`

	// Rank is the report order of tier names, best first. It defaults to the
	// tiers, then the guesses, then REMOTE?.
	Rank []string `json:"rank,omitempty"`

	Skills        []string `json:"skills,omitempty"`         // +4 each, up to 10
	MissingSkills []string `json:"missing_skills,omitempty"` // -6 each
	Languages     []string `json:"languages,omitempty"`      // +8 if any is mentioned
}

// TierFile is one location tier. Location is a case-insensitive regular
// expression.
type TierFile struct {
	Name           string `json:"name"`
	Location       string `json:"location"`
	RemoteRequired bool   `json:"remote_required,omitempty"`
	AllowUSOnly    bool   `json:"allow_us_only,omitempty"`
}

// GuessFile is one description guess. Description is a case-insensitive
// regular expression.
type GuessFile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Default is the built-in profile.
var Default = mustBuild(defaultFile, defaultHave, defaultLack, defaultLang)

var defaultFile = ProfileFile{
	MaxYears: 5,
	Tiers: []TierFile{
		{Name: "SPAIN", Location: `spain|españa|espana|madrid|barcelona|valencia|m[aá]laga|sevill`, RemoteRequired: true},
		{Name: "WORLD", Location: `worldwide|anywhere|global|international|\bworld\b|remote.?-?\s*all`},
		{Name: "EUROPE", Location: `europe|emea|\beu\b|\bcet\b|portugal|lisbon`, RemoteRequired: true},
		{Name: "LATAM", Location: `latam|latin america|south america|lat\.? am`},
		{Name: "COLOMBIA", Location: `colombia|bogot|medell|cali\b|barranquilla`, RemoteRequired: true},
		{Name: "AMERICAS", Location: `americas|\bamer\b`},
		{Name: "CANADA", Location: `canada|toronto|ontario|vancouver|montr[eé]al|ottawa|calgary|waterloo`, RemoteRequired: true, AllowUSOnly: true},
	},
	DescriptionGuesses: []GuessFile{
		{Name: "EUROPE?desc", Description: `spain|europe|emea|\bcet\b|european time`},
		{Name: "LATAM?desc", Description: `colombia|latam|latin america|south america`},
	},
	Rank: []string{"SPAIN", "WORLD", "EUROPE", "EUROPE?desc", "LATAM", "LATAM?desc",
		"COLOMBIA", "AMERICAS", RemoteGuess, "CANADA"},
}

// The default skill patterns are kept as regexes, exactly as in the Python
// prototype. In it "c#" sits inside \b(...)\b. Because "#" is not a word
// character, the closing \b there means "followed by a letter or digit", so
// "C# developer" does not count but "C#dev" would (likely a bug in the
// prototype, kept for parity). It gets its own branch to say that explicitly.
var (
	defaultHave = rx(`\b(react|typescript|javascript|python|node\.?js|postman|pytest|sql|postgres\w*|supabase|qa|` +
		`quality assurance|test cases?|regression|sso|saml|entra|azure|llms?|openai|claude|rag|` +
		`rest(?:ful)? apis?|troubleshoot\w*|customer[- ]facing|flutter|next\.js|astro|salesforce|` +
		`servicenow|outsystems|agile|scrum|ci/cd|docker|github|prompt\w*|automation|n8n|html|css|` +
		`debugging|root cause)\b` +
		`|\b(c#)\w`)
	defaultLack = rx(`\b(kubernetes|k8s|terraform|golang|kotlin|rails|scala|rust|elixir|haskell|swift|graphql|` +
		`playwright|cypress|selenium|appium|jest|langchain|langgraph)\b`)
	defaultLang = rx(`spanish|español|castellano|french|français|bilingual`)
)

// LoadProfile reads a profile JSON file. Fields it leaves out keep the
// built-in values.
func LoadProfile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f ProfileFile
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	p, err := build(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// build compiles f over the defaults.
func build(f ProfileFile) (*Profile, error) {
	have, lack, lang := defaultHave, defaultLack, defaultLang
	var err error
	if f.Skills != nil {
		if have, err = keywords(f.Skills); err != nil {
			return nil, fmt.Errorf("skills: %w", err)
		}
	}
	if f.MissingSkills != nil {
		if lack, err = keywords(f.MissingSkills); err != nil {
			return nil, fmt.Errorf("missing_skills: %w", err)
		}
	}
	if f.Languages != nil {
		if lang, err = keywords(f.Languages); err != nil {
			return nil, fmt.Errorf("languages: %w", err)
		}
	}
	customOrder := f.Tiers != nil || f.DescriptionGuesses != nil
	if f.MaxYears == 0 {
		f.MaxYears = defaultFile.MaxYears
	}
	if f.Tiers == nil {
		f.Tiers = defaultFile.Tiers
	}
	if f.DescriptionGuesses == nil {
		f.DescriptionGuesses = defaultFile.DescriptionGuesses
	}
	if f.Rank == nil {
		if !customOrder {
			f.Rank = defaultFile.Rank
		} else {
			for _, t := range f.Tiers {
				f.Rank = append(f.Rank, t.Name)
			}
			for _, g := range f.DescriptionGuesses {
				f.Rank = append(f.Rank, g.Name)
			}
			f.Rank = append(f.Rank, RemoteGuess)
		}
	}
	return compile(f, have, lack, lang)
}

func compile(f ProfileFile, have, lack, lang *pyre.Regexp) (*Profile, error) {
	p := &Profile{MaxYears: f.MaxYears, have: have, lack: lack, lang: lang, rank: map[string]int{}}
	for _, t := range f.Tiers {
		re, err := compileRx(t.Location)
		if err != nil {
			return nil, fmt.Errorf("tier %q: %w", t.Name, err)
		}
		p.tiers = append(p.tiers, tierRule{name: t.Name, re: re, remoteRequired: t.RemoteRequired, allowUSOnly: t.AllowUSOnly})
	}
	for _, g := range f.DescriptionGuesses {
		re, err := compileRx(g.Description)
		if err != nil {
			return nil, fmt.Errorf("description guess %q: %w", g.Name, err)
		}
		p.guesses = append(p.guesses, guess{name: g.Name, re: re})
	}
	for i, name := range f.Rank {
		if _, dup := p.rank[name]; !dup {
			p.rank[name] = i
		}
	}
	return p, nil
}

func mustBuild(f ProfileFile, have, lack, lang *pyre.Regexp) *Profile {
	p, err := compile(f, have, lack, lang)
	if err != nil {
		panic(err)
	}
	return p
}

// compileRx checks a user pattern with Go's parser before pyre.MustCompile,
// so a bad pattern is an error rather than a panic.
func compileRx(pattern string) (*pyre.Regexp, error) {
	if pattern == "" {
		return nil, fmt.Errorf("empty pattern")
	}
	if _, err := regexp.Compile("(?i)" + pattern); err != nil {
		return nil, err
	}
	return rx(pattern), nil
}

// keywords turns plain words ("react", "c#", "node.js") into one pattern
// that captures whichever word matched. A word boundary is required only on
// a side that starts or ends with a letter or digit, so "c#" and "c++" work.
// An empty list matches nothing.
func keywords(words []string) (*pyre.Regexp, error) {
	var alts []string
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		alt := regexp.QuoteMeta(w)
		if isWord(w[0]) {
			alt = `\b` + alt
		}
		if isWord(w[len(w)-1]) {
			alt += `\b`
		}
		alts = append(alts, alt)
	}
	if len(alts) == 0 {
		return nil, nil
	}
	return compileRx("(" + strings.Join(alts, "|") + ")")
}

func isWord(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}
