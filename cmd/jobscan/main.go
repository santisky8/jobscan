// Command jobscan finds fresh junior/mid remote tech roles on Greenhouse,
// Ashby and Lever job boards and writes them to a CSV report.
//
//	jobscan [--dir DIR] [--profile FILE] [--hours N] [--max-years N] [--all] [--workers N]
//	jobscan [--dir DIR] --add-org ats:slug
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/santisky8/jobscan/internal/ats"
	"github.com/santisky8/jobscan/internal/filter"
	"github.com/santisky8/jobscan/internal/store"
)

const usage = `Usage:
  jobscan [flags]                  scan the boards in DIR/orgs.json -> DIR/out/<date>.csv
  jobscan [--dir DIR] --add-org ats:slug   add a board to DIR/orgs.json

Flags:
`

func main() {
	// Ctrl-C stops the scan cleanly instead of leaving half-written files.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp): // -h already printed the usage
	default:
		fmt.Fprintln(os.Stderr, "jobscan:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("jobscan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}
	opt := ats.Options{Workers: ats.DefaultWorkers}
	dir := fs.String("dir", "jobscan-data", "folder with orgs.json, seen.txt and out/")
	profile := fs.String("profile", "", "search profile JSON (default: built-in profile)")
	fs.IntVar(&opt.Hours, "hours", 24, "posted within this many hours")
	fs.IntVar(&opt.MaxYears, "max-years", 0, "drop roles whose minimum required years is above this (default: the profile's, 5 built in)")
	fs.BoolVar(&opt.All, "all", false, "include roles already in seen.txt (writes out/<date>-all.csv)")
	fs.StringVar(&opt.AddOrg, "add-org", "", "add a board to orgs.json and exit, e.g. ashby:posthog")
	fs.IntVar(&opt.Workers, "workers", opt.Workers, "boards downloaded at the same time")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		fs.Usage()
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	opt.Profile = filter.Default
	if *profile != "" {
		p, err := filter.LoadProfile(*profile)
		if err != nil {
			return err
		}
		opt.Profile = p
	}
	if opt.MaxYears == 0 {
		opt.MaxYears = opt.Profile.MaxYears
	}
	opt.Dir = store.Dir(*dir)
	if _, err := os.Stat(opt.Dir.OrgsPath()); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s not found; start from the example: mkdir -p %s && cp examples/orgs.json %s",
			opt.Dir.OrgsPath(), *dir, opt.Dir.OrgsPath())
	}
	return ats.Run(ctx, opt, stdout, stderr)
}
