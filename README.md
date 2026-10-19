# jobscan

[![CI](https://github.com/santisky8/jobscan/actions/workflows/ci.yml/badge.svg)](https://github.com/santisky8/jobscan/actions/workflows/ci.yml)

Go command-line tool that finds fresh, fully remote junior and mid-level tech
roles and writes them to a CSV report, ranked by where you can work and how
well each role fits your skills.

It reads only the official public job-board APIs of **Greenhouse**, **Ashby**
and **Lever**, the systems most tech companies use to post jobs. No scraping,
no login, no LLM: plain HTTP, JSON and regular expressions.

## Why

Job sites show roles days after they are posted, mixed with hybrid roles,
senior roles and roles closed to applicants outside one country. Going
straight to the company boards finds roles within hours of posting, and a
fixed set of rules drops the ones that do not fit before you read them.

## Quick start

Needs Go 1.27 or newer. Standard library only, no other dependencies.

```sh
go build -o jobscan ./cmd/jobscan
mkdir -p jobscan-data && cp examples/orgs.json jobscan-data/
./jobscan                          # roles posted in the last 24 hours
```

Sample output (shortened):

```
boards 18 (empty 0), jobs 3458, matches 3, new 3
csv: jobscan-data/out/2026-10-19.csv
2026-10-16 11:41 WORLD fit=69 - acme | SDK Engineer - Python | Remote, Global |  | https://jobs.ashbyhq.com/acme/...
```

and `jobscan-data/out/2026-10-19.csv`:

```csv
posted,tier,fit,visa,org,title,loc,min_years,ats,url
2026-10-16 11:41,WORLD,69,,acme,SDK Engineer - Python,"Remote, Global",,ashby,https://jobs.ashbyhq.com/acme/...
2026-10-13 12:12,EUROPE,53,sponsor,example,Backend Engineer,"Remote - Europe",3,greenhouse,https://boards.greenhouse.io/example/jobs/...
```

## Usage

```sh
./jobscan --hours 72               # look back 3 days
./jobscan --all                    # include roles already reported (writes <date>-all.csv)
./jobscan --max-years 3            # stricter on required experience
./jobscan --profile examples/profile.json
./jobscan --add-org ashby:posthog  # add a company board to orgs.json
./jobscan --dir ~/jobs             # use another data folder
./jobscan --workers 16             # boards downloaded at the same time (default 32)
```

### Data folder

`--dir` (default `./jobscan-data`) holds:

- `orgs.json`: board names per system, e.g. `{"greenhouse": ["stripe"], "ashby": ["posthog"], "lever": ["spotify"]}`.
  The name is the part of the company's job-board URL after the system's domain.
- `seen.txt`: every role already reported, one URL per line. A role is reported once.
- `out/<date>.csv`: the report. Several runs on the same day append to it.

## How a role is chosen

All rules live in `internal/filter`. A posting is kept when every step passes:

1. **Title** looks like an engineering, QA, support or solutions role, and is
   not too senior (senior, staff, lead, III...), not a stack outside the
   profile, not sales, design or recruiting, and does not require a language
   the candidate does not speak.
2. **Not hybrid or on-site.**
3. **Location tier**: the first tier whose pattern matches the location. A
   tier can require that the role is also remote. A remote role with no place
   named gets a guess from its description, or `REMOTE?` for a bare "Remote".
4. **Not US-only** ("must be located in the US"...), unless the tier allows it.
5. **Experience**: the smallest "N years" in the description is at most
   `--max-years`.
6. **Visa**: `needs permit` when the text asks for existing EU/UK work rights
   or says there is no sponsorship; `sponsor` when it offers sponsorship or
   relocation.
7. **Fit score** (0-100): 40, +4 per known skill (up to 10), -6 per missing
   skill, +10 / +5 / 0 / -5 for 1-2 / 3 / 4 / 5 years required (+5 when none
   is stated), +8 when one of the candidate's languages is mentioned, +10 for
   sponsorship, -15 for "needs permit".

Rows are sorted by tier, then fit (high first), then posting time.

### Search profile

`--profile FILE` replaces parts of the built-in profile. Every field is
optional; see [`examples/profile.json`](examples/profile.json).

| Field | Meaning |
|---|---|
| `max_years` | default for `--max-years` |
| `tiers` | `name`, `location` (regex), `remote_required`, `allow_us_only`; checked in order |
| `description_guesses` | `name`, `description` (regex) for remote roles with no place named |
| `rank` | report order of tier names, best first |
| `skills`, `missing_skills` | plain keywords for the fit score, e.g. `"node.js"`, `"c#"` |
| `languages` | plain keywords; a match adds the language bonus |

Patterns are case-insensitive. Use `\b` only next to a letter or digit (see
below). The title rules (step 1) and the US-only and visa phrases are built
in and not part of the profile.

The built-in profile ranks Spain, then worldwide, Europe, Latin America,
Colombia, the Americas and Canada, for a full-stack and QA skill set.

## Design

```
cmd/jobscan        flags, profile loading, entry point
internal/ats       Greenhouse/Ashby/Lever clients and parsers, worker pool, the scan
internal/filter    rules, fit score, search profile
internal/store     orgs.json, seen.txt, CSV writing
internal/pyre      Python-compatible \b, \w, \s on top of Go's regexp
internal/pyjson    Python-style JSON string quoting for orgs.json
internal/textutil  HTML to text, Python whitespace rules
```

- **Concurrency**: boards download in parallel through a fixed pool of
  goroutines. Results are stored by board index, so the report does not
  depend on network timing. Ctrl-C cancels the requests and writes nothing
  partial.
- **Port from Python**: jobscan started as a Python script. The Go version
  reproduces its output byte for byte (CSV dialect, JSON formatting, regex
  results). A parity check over 57,662 real postings found no differences.
- **RE2 vs Python regex**: Go's regexp engine (RE2) treats `\b`, `\w` and
  `\s` as ASCII-only, while Python treats accented letters as word
  characters and a non-breaking space as whitespace. RE2 also has no
  lookaround. `internal/pyre` rewrites those escapes so results match
  Python's; its `\b` is exact when the pattern side of the boundary is a
  letter or digit.
- **Tests**: table-driven tests for every rule, parser tests on saved API
  responses in `testdata/`, and CI runs `go vet` and `go test -race`.

```sh
go vet ./...
go test -race ./...
```

## License

MIT. See [LICENSE](LICENSE).
