package ats

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/santisky8/jobscan/internal/filter"
	"github.com/santisky8/jobscan/internal/store"
	"github.com/santisky8/jobscan/internal/textutil"
)

// Columns of out/<date>.csv.
var Columns = []string{"posted", "tier", "fit", "visa", "org", "title", "loc", "min_years", "ats", "url"}

// Options are the flags of jobscan.
type Options struct {
	Dir      store.Dir
	Hours    int             // keep roles posted within this many hours
	MaxYears int             // drop roles asking for more years than this
	Profile  *filter.Profile // nil means filter.Default
	All      bool            // include roles already in seen.txt; write <date>-all.csv
	AddOrg   string          // "ats:slug": add a board to orgs.json and exit
	Workers  int
}

func (o Options) profile() *filter.Profile {
	if o.Profile == nil {
		return filter.Default
	}
	return o.Profile
}

// Row is one matched role in the report.
type Row struct {
	Posted string // local time, "2006-01-02 15:04"
	filter.Result
	Org, Title, Loc, ATS, URL string
}

func (r Row) record() []string {
	return []string{r.Posted, r.Tier, strconv.Itoa(r.Fit), r.Visa, r.Org, r.Title, r.Loc,
		yearsField(r.MinYears), r.ATS, r.URL}
}

// yearsField prints "no years stated" as an empty cell, like Python.
func yearsField(y int) string {
	if y == 0 {
		return ""
	}
	return strconv.Itoa(y)
}

// Run scans every board in orgs.json and writes the report.
func Run(ctx context.Context, opt Options, stdout, stderr io.Writer) error {
	orgs, err := store.LoadOrgs(opt.Dir.OrgsPath())
	if err != nil {
		return err
	}
	if opt.AddOrg != "" {
		return addOrg(orgs, opt, stdout)
	}

	cutoff := time.Now().Add(-time.Duration(opt.Hours) * time.Hour)
	seen, err := store.LoadSeen(opt.Dir.SeenPath())
	if err != nil {
		return err
	}
	var boards []Board
	for _, name := range orgs.Keys() {
		if !Supported(name) {
			continue
		}
		for _, org := range orgs.Boards(name) {
			boards = append(boards, Board{ATS: name, Org: org})
		}
	}

	results := FetchAll(ctx, NewClient(opt.Workers), boards, opt.Workers)
	if err := ctx.Err(); err != nil {
		return err // interrupted: write nothing rather than a partial report
	}
	var jobs []Job
	var dead []string
	for i, res := range results {
		jobs = append(jobs, res...)
		if len(res) == 0 {
			dead = append(dead, boards[i].String())
		}
	}

	rows, matched := match(jobs, seen, cutoff, opt)
	slices.SortStableFunc(rows, func(a, b Row) int {
		if d := opt.profile().Rank(a.Tier) - opt.profile().Rank(b.Tier); d != 0 {
			return d
		}
		if d := b.Fit - a.Fit; d != 0 {
			return d
		}
		return strings.Compare(a.Posted, b.Posted)
	})

	// Several runs a day append to the same file; header only once.
	// --all rewrites its own file instead.
	suffix := ""
	if opt.All {
		suffix = "-all"
	}
	path := filepath.Join(opt.Dir.OutDir(), store.Today()+suffix+".csv")
	fresh := opt.All || !store.Exists(path)
	records := make([][]string, len(rows))
	for i, r := range rows {
		records[i] = r.record()
	}
	if err := store.WriteCSV(path, Columns, records, !opt.All, fresh); err != nil {
		return err
	}
	var newURLs []string
	for _, u := range matched {
		if !seen[u] {
			newURLs = append(newURLs, u)
		}
	}
	slices.Sort(newURLs)
	if err := store.AppendLines(opt.Dir.SeenPath(), newURLs); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "boards %d (empty %d), jobs %d, matches %d, new %d\n",
		len(boards), len(dead), len(jobs), len(matched), len(rows))
	fmt.Fprintln(stdout, "csv:", path)
	for _, r := range rows {
		fmt.Fprintln(stdout, strings.Join([]string{r.Posted, r.Tier, "fit=" + strconv.Itoa(r.Fit), orDash(r.Visa),
			r.Org, "|", r.Title, "|", textutil.Truncate(r.Loc, 50), "|", yearsField(r.MinYears), "|", r.URL}, " "))
	}
	if len(dead) > 0 {
		fmt.Fprintln(stderr, "empty boards (renamed or closed?):", strings.Join(dead, " "))
	}
	return nil
}

// match keeps fresh, unique postings that pass the filters. It returns the
// report rows (skipping seen URLs unless opt.All) and every matched URL.
func match(jobs []Job, seen map[string]bool, cutoff time.Time, opt Options) (rows []Row, matched []string) {
	isMatched := map[string]bool{}
	for _, j := range jobs {
		if j.URL == "" || isMatched[j.URL] || j.Posted.IsZero() || j.Posted.Before(cutoff) {
			continue
		}
		res, ok := opt.profile().Classify(filter.Posting{Title: j.Title, Loc: j.Loc, Desc: j.Desc, Remote: j.Remote}, opt.MaxYears)
		if !ok {
			continue
		}
		isMatched[j.URL] = true
		matched = append(matched, j.URL)
		if seen[j.URL] && !opt.All {
			continue
		}
		rows = append(rows, Row{
			Posted: j.Posted.Local().Format("2006-01-02 15:04"),
			Result: res,
			Org:    j.Org, Title: j.Title, Loc: textutil.Truncate(j.Loc, 120), ATS: j.ATS, URL: j.URL,
		})
	}
	return rows, matched
}

// addOrg handles --add-org ats:slug.
func addOrg(orgs *store.Orgs, opt Options, stdout io.Writer) error {
	name, slug, ok := strings.Cut(opt.AddOrg, ":")
	if !ok {
		return fmt.Errorf("--add-org wants ats:slug, e.g. ashby:posthog")
	}
	if orgs.Add(name, slug) > 0 {
		if err := orgs.Save(opt.Dir.OrgsPath()); err != nil {
			return err
		}
	}
	fmt.Fprintln(stdout, "added", opt.AddOrg)
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
