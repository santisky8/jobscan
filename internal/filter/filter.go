// Package filter decides whether a posting is worth applying to and how
// well it fits. It is a port of the regexes and rules of the original Python
// prototype, and gives the same results.
//
// The title rules are fixed. The search profile (location tiers and their
// order, skills, missing skills, languages, maximum years) comes from a
// Profile; Default holds the built-in one. Python's re.I becomes a (?i)
// prefix, and package pyre gives \s, \w and \b their Python (Unicode)
// meaning.
package filter

import (
	"strings"

	"github.com/santisky8/jobscan/internal/pyre"
	"github.com/santisky8/jobscan/internal/textutil"
)

// rx compiles a Python pattern with re.I semantics.
func rx(pattern string) *pyre.Regexp { return pyre.MustCompile("(?i)" + pattern) }

var (
	// role: the title must look like a role the candidate can do.
	role = rx(`engineer|developer|desarroll|ingenier|programmer|\bqa\b|quality|sdet|\btest|automation|` +
		`full.?stack|back.?end|front.?end|software|\bai\b|\bllm|forward deployed|integration|` +
		`implementation|solutions|support|technical|product engineer|soporte`)

	// bad: too senior, wrong stack, or not engineering.
	bad = rx(`senior|\bsr\b|sr\.|staff|\blead\b|principal|manager|director|\bhead\b|architect|\bvp\b|` +
		`\biii\b|\biv\b|\bl[4-7]\b|ic[3-6]\b|semi.?senior|ssr|distinguished|chief|intern\b|internship|` +
		`sales|account exec|recruit|designer|marketing|data scien|machine learning researcher|` +
		`research scientist|android|\bios\b|golang|\bgo\b|rust|kotlin|scala|ruby|rails|c\+\+|php|` +
		`embedded|firmware|hardware|kernel|network engineer|security|compliance|devops|\bsre\b|` +
		`site reliability|salesforce developer|mainframe|sap\b|` +
		`\b(german|dutch|italian|portuguese|polish|japanese|korean|mandarin|turkish|swedish|danish|` +
		`norwegian|finnish|nordic|dach)\b`)

	hybrid = rx(`hybrid|on-?site|in-?office|onsite|\boffice\b`)

	// usOnly: description phrases that block a non-US applicant.
	usOnly = rx(`(must|need to|required to) (be )?(located|based|reside|live) in the (us|u\.s\.|united states)|` +
		`authori[sz]ed to work in the (us|u\.s\.|united states)|us[- ]based only|` +
		`only (considering|accepting) (candidates|applicants) (in|from) the (us|united states)`)

	// noSponsor and sponsor: EU/UK work-rights signals.
	noSponsor = rx(`(unable to|cannot|can't|do not|don't|does not|will not|won't|not able to) ` +
		`(offer |provide |support )?(visa |work permit )?sponsor|no (visa )?sponsorship|` +
		`(right|authori[sz]ation|authori[sz]ed|eligib\w*|permit) to work in (the )?` +
		`(eu\b|european union|europe|spain|eea|uk\b|united kingdom|netherlands|germany|france|` +
		`portugal|ireland)|valid (eu )?work permit|eu citizen`)
	sponsor = rx(`visa sponsorship|sponsor (your |a |the )?(work )?visa|relocation (support|package|assistance)|` +
		`we sponsor`)

	remoteWord = rx(`remote`)
	justRemote = rx(`^\s*remote\s*$`) // Python: re.fullmatch(r"\s*remote\s*", loc, re.I)

	years = rx(`(\d{1,2})\s*\+?\s*(?:-|–|to)?\s*\d{0,2}\s*\+?\s*(?:years|yrs|años)`)
)

// Visa values written to the CSV.
const (
	VisaSponsor     = "sponsor"
	VisaNeedsPermit = "needs permit"
)

// RemoteGuess is the tier of a role marked remote whose location is just
// "Remote" and whose description gives no better hint.
const RemoteGuess = "REMOTE?"

// Posting is the part of a job that the filters look at.
type Posting struct {
	Title  string
	Loc    string
	Desc   string
	Remote bool // the source marked it remote
}

// Result is what Classify reports for a posting that passes.
type Result struct {
	Tier     string
	MinYears int // 0 means the description states no years
	Visa     string
	Fit      int
}

// TitleOK reports whether a title looks like a junior/mid role the candidate
// can do (matches role, not bad).
func TitleOK(title string) bool {
	return role.MatchString(title) && !bad.MatchString(title)
}

// MinYears returns the smallest "N years" figure (1-19) in desc, or 0.
func MinYears(desc string) int {
	least := 0
	for _, m := range years.FindAllGroups(desc) {
		n := atoi(m)
		if n > 0 && n < 20 && (least == 0 || n < least) {
			least = n
		}
	}
	return least
}

// atoi parses the one or two ASCII digits captured by the years pattern.
// (Python's \d also matches other scripts' digits, e.g. Arabic-Indic; Go's
// does not. That needs such digits right before "years", which the parity
// check never saw.)
func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// Tier returns the first Default location tier that matches loc, or "".
func Tier(loc string) string { return Default.Tier(loc) }

// Rank returns the Default sort position of a tier (lower is better).
func Rank(tier string) int { return Default.Rank(tier) }

// Fit scores a posting with the Default profile.
func Fit(desc string, minYears int, visa string) int { return Default.Fit(desc, minYears, visa) }

// Classify applies every rule with the Default profile and the given
// maximum years. It returns ok=false when the posting should be dropped.
func Classify(p Posting, maxYears int) (Result, bool) {
	return Default.Classify(p, maxYears)
}

// Tier returns the first location tier that matches loc, or "".
func (pr *Profile) Tier(loc string) string {
	for _, t := range pr.tiers {
		if t.re.MatchString(loc) {
			return t.name
		}
	}
	return ""
}

// Rank returns the sort position of a tier (lower is better). Unknown
// tiers sort last.
func (pr *Profile) Rank(tier string) int {
	if r, ok := pr.rank[tier]; ok {
		return r
	}
	return len(pr.rank)
}

// Fit is a rough 0-100 "top candidate" score: skill overlap, few gaps,
// low years required, a language edge, and visa signals.
func (pr *Profile) Fit(desc string, minYears int, visa string) int {
	score := 40 + 4*min(distinct(pr.have, desc), 10) - 6*distinct(pr.lack, desc)
	switch minYears {
	case 1, 2:
		score += 10
	case 3:
		score += 5
	case 4:
	case 5:
		score -= 5
	default: // no years stated, or more than 5 (only possible with max years > 5)
		score += 5
	}
	if pr.lang != nil && pr.lang.MatchString(desc) {
		score += 8
	}
	switch visa {
	case VisaSponsor:
		score += 10
	case VisaNeedsPermit:
		score -= 15
	}
	return max(0, min(100, score))
}

// distinct counts the different (lower-cased) keywords re finds in s.
func distinct(re *pyre.Regexp, s string) int {
	if re == nil {
		return 0
	}
	seen := map[string]bool{}
	for _, m := range re.FindAllGroups(s) {
		seen[strings.ToLower(m)] = true
	}
	return len(seen)
}

// Classify applies every rule. It returns ok=false when the posting should
// be dropped.
func (pr *Profile) Classify(p Posting, maxYears int) (r Result, ok bool) {
	if !TitleOK(p.Title) || hybrid.MatchString(p.Loc+" "+p.Title) {
		return r, false
	}
	var rule *tierRule
	for i := range pr.tiers {
		if pr.tiers[i].re.MatchString(p.Loc) {
			rule = &pr.tiers[i]
			break
		}
	}
	tier := ""
	if rule != nil {
		tier = rule.name
		// A named country or city counts only when the role is remote.
		if rule.remoteRequired && !p.Remote && !strings.Contains(strings.ToLower(p.Loc+p.Title), "remote") {
			return r, false
		}
	}
	// No place named but marked remote: guess from the description.
	if tier == "" && p.Remote {
		generic := textutil.TrimSpace(p.Loc) == "" || remoteWord.MatchString(p.Loc)
		if generic {
			for _, g := range pr.guesses {
				if g.re.MatchString(p.Desc) {
					tier = g.name
					break
				}
			}
		}
		if tier == "" && justRemote.MatchString(p.Loc) && !usOnly.MatchString(p.Desc) {
			tier = RemoteGuess
		}
	}
	if tier == "" || ((rule == nil || !rule.allowUSOnly) && usOnly.MatchString(p.Desc)) {
		return r, false
	}
	y := MinYears(p.Desc)
	if y != 0 && y > maxYears {
		return r, false
	}
	visa := ""
	if noSponsor.MatchString(p.Desc) {
		visa = VisaNeedsPermit
	} else if sponsor.MatchString(p.Desc) {
		visa = VisaSponsor
	}
	return Result{Tier: tier, MinYears: y, Visa: visa, Fit: pr.Fit(p.Desc, y, visa)}, true
}
