// Package ats scans public job boards on Greenhouse, Ashby and Lever.
//
// Each board is one JSON request. The parsers here turn a board's JSON into
// Jobs; fetch.go downloads many boards at once with a worker pool; scan.go
// filters, ranks and writes the report.
package ats

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/santisky8/jobscan/internal/textutil"
)

// Job is one posting from any ATS, normalized.
type Job struct {
	ATS    string
	Org    string
	Title  string
	Loc    string
	Remote bool
	Posted time.Time // zero when the board gave no usable timestamp
	URL    string
	Desc   string
}

// Board identifies one company's job board.
type Board struct {
	ATS string // "greenhouse", "ashby" or "lever"
	Org string // the board slug
}

func (b Board) String() string { return b.ATS + ":" + b.Org }

// boardURL returns the public API endpoint for a board.
func boardURL(b Board) string {
	switch b.ATS {
	case "greenhouse":
		return "https://boards-api.greenhouse.io/v1/boards/" + b.Org + "/jobs?content=true"
	case "ashby":
		return "https://api.ashbyhq.com/posting-api/job-board/" + b.Org
	case "lever":
		return "https://api.lever.co/v0/postings/" + b.Org + "?mode=json"
	}
	return ""
}

// Supported reports whether jobscan knows how to fetch this ATS.
func Supported(ats string) bool { return boardURL(Board{ATS: ats}) != "" }

// parse turns a board's JSON body into jobs.
func parse(b Board, body []byte) []Job {
	switch b.ATS {
	case "greenhouse":
		return parseGreenhouse(b.Org, body)
	case "ashby":
		return parseAshby(b.Org, body)
	case "lever":
		return parseLever(b.Org, body)
	}
	return nil
}

// unmarshal decodes JSON but tolerates fields of an unexpected type: those
// are left empty and the rest of the document is still used, which is how
// the Python version (reading fields with .get) behaves in practice.
func unmarshal(body []byte, v any) bool {
	err := json.Unmarshal(body, v)
	var typeErr *json.UnmarshalTypeError
	return err == nil || errors.As(err, &typeErr)
}

func parseGreenhouse(org string, body []byte) []Job {
	var d struct {
		Jobs []struct {
			Title    string `json:"title"`
			Location struct {
				Name string `json:"name"`
			} `json:"location"`
			FirstPublished any    `json:"first_published"`
			UpdatedAt      any    `json:"updated_at"`
			AbsoluteURL    string `json:"absolute_url"`
			Content        string `json:"content"` // HTML, entity-escaped
		} `json:"jobs"`
	}
	if !unmarshal(body, &d) {
		return nil
	}
	var jobs []Job
	for _, j := range d.Jobs {
		loc := j.Location.Name
		ts := j.FirstPublished
		if !truthy(ts) {
			ts = j.UpdatedAt
		}
		jobs = append(jobs, Job{
			ATS: "greenhouse", Org: org, Title: j.Title, Loc: loc,
			Remote: strings.Contains(strings.ToLower(loc), "remote"),
			Posted: ParseTime(ts), URL: j.AbsoluteURL, Desc: textutil.Strip(j.Content),
		})
	}
	return jobs
}

func parseAshby(org string, body []byte) []Job {
	var d struct {
		Jobs []struct {
			Title              string `json:"title"`
			Location           string `json:"location"`
			SecondaryLocations []struct {
				Location string `json:"location"`
			} `json:"secondaryLocations"`
			IsRemote         bool   `json:"isRemote"`
			WorkplaceType    string `json:"workplaceType"`
			PublishedAt      any    `json:"publishedAt"`
			JobURL           string `json:"jobUrl"`
			DescriptionPlain string `json:"descriptionPlain"`
		} `json:"jobs"`
	}
	if !unmarshal(body, &d) {
		return nil
	}
	var jobs []Job
	for _, j := range d.Jobs {
		locs := []string{j.Location}
		for _, s := range j.SecondaryLocations {
			locs = append(locs, s.Location)
		}
		jobs = append(jobs, Job{
			ATS: "ashby", Org: org, Title: j.Title, Loc: joinNonEmpty(locs),
			Remote: j.IsRemote || strings.ToLower(j.WorkplaceType) == "remote",
			Posted: ParseTime(j.PublishedAt), URL: j.JobURL, Desc: j.DescriptionPlain,
		})
	}
	return jobs
}

func parseLever(org string, body []byte) []Job {
	// Lever returns a bare list; anything else (an error object) means no jobs.
	var d []struct {
		Text       string `json:"text"`
		Categories struct {
			Location     string   `json:"location"`
			AllLocations []string `json:"allLocations"`
		} `json:"categories"`
		DescriptionPlain string `json:"descriptionPlain"`
		Lists            []struct {
			Text    string `json:"text"`
			Content string `json:"content"`
		} `json:"lists"`
		AdditionalPlain string `json:"additionalPlain"`
		WorkplaceType   string `json:"workplaceType"`
		CreatedAt       any    `json:"createdAt"` // epoch milliseconds
		HostedURL       string `json:"hostedUrl"`
	}
	if !unmarshal(body, &d) {
		return nil
	}
	var jobs []Job
	for _, j := range d {
		// Main location first, then the rest, without repeats.
		var locs []string
		for _, l := range append([]string{j.Categories.Location}, j.Categories.AllLocations...) {
			if !slices.Contains(locs, l) {
				locs = append(locs, l)
			}
		}
		parts := []string{j.DescriptionPlain}
		var lists []string
		for _, l := range j.Lists {
			lists = append(lists, l.Text+": "+textutil.Strip(l.Content))
		}
		parts = append(parts, strings.Join(lists, " "), j.AdditionalPlain)
		jobs = append(jobs, Job{
			ATS: "lever", Org: org, Title: j.Text, Loc: joinNonEmpty(locs),
			Remote: strings.ToLower(j.WorkplaceType) == "remote" ||
				strings.Contains(strings.ToLower(strings.Join(locs, " ")), "remote"),
			Posted: ParseTime(j.CreatedAt), URL: j.HostedURL, Desc: strings.Join(parts, " "),
		})
	}
	return jobs
}

func joinNonEmpty(parts []string) string {
	var keep []string
	for _, p := range parts {
		if p != "" {
			keep = append(keep, p)
		}
	}
	return strings.Join(keep, " | ")
}

// truthy mirrors Python truthiness for a decoded JSON value.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case float64:
		return x != 0
	case bool:
		return x
	}
	return true
}

// ParseTime reads an ATS timestamp: an ISO 8601 string (Greenhouse, Ashby)
// or epoch milliseconds (Lever). It returns the zero time if v is neither.
// Like Python's datetime.fromisoformat, a missing zone means UTC and
// fractions are kept to the microsecond.
func ParseTime(v any) time.Time {
	switch x := v.(type) {
	case float64:
		return time.UnixMicro(int64(x * 1000)).UTC()
	case string:
		return parseISO(strings.ReplaceAll(x, "Z", "+00:00"))
	}
	return time.Time{}
}

// isoLayouts are the ISO 8601 shapes the job boards use. When parsing, Go
// accepts a fraction of any length after the seconds even though the
// layouts do not show one.
var isoLayouts = []string{
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04Z07:00",
	"2006-01-02T15:04",
	"2006-01-02T15Z07:00",
	"2006-01-02T15",
	"2006-01-02",
}

func parseISO(s string) time.Time {
	// fromisoformat accepts any single separator between date and time;
	// the boards use "T" or a space.
	if len(s) > 10 && s[10] == ' ' {
		s = s[:10] + "T" + s[11:]
	}
	for _, layout := range isoLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Truncate(time.Microsecond)
		}
	}
	return time.Time{}
}
