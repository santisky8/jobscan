package ats

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// DefaultWorkers is how many boards are downloaded at the same time.
const DefaultWorkers = 32

// NewClient returns the HTTP client used for board requests. Python used a
// 25 s timeout per socket operation; here connecting and waiting for the
// response headers each get 25 s, and a whole request (large boards can be
// several MB) gets two minutes.
func NewClient(workers int) *http.Client {
	return &http.Client{
		Timeout: 2 * time.Minute,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 25 * time.Second}).DialContext,
			TLSHandshakeTimeout:   25 * time.Second,
			ResponseHeaderTimeout: 25 * time.Second,
			// Most boards live on three hosts, so allow every worker to keep
			// its connection open instead of the default two per host.
			MaxIdleConnsPerHost: workers,
		},
	}
}

// get downloads url and returns the body, or nil on any failure (network
// error, non-2xx status). A failed board is reported as empty, like Python.
func get(ctx context.Context, client *http.Client, url string) []byte {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "jobscan (+https://github.com/santisky8/jobscan)")
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	return body
}

// FetchAll downloads and parses every board using a fixed number of
// goroutines. The result has one entry per board, in the same order as
// boards, so the output does not depend on which request finished first.
func FetchAll(ctx context.Context, client *http.Client, boards []Board, workers int) [][]Job {
	results := make([][]Job, len(boards))
	next := make(chan int) // indexes into boards

	var wg sync.WaitGroup
	for range max(1, workers) {
		wg.Go(func() {
			for i := range next {
				// Each index is handled by exactly one worker, so writing
				// results[i] needs no lock.
				if body := get(ctx, client, boardURL(boards[i])); body != nil {
					results[i] = parse(boards[i], body)
				}
			}
		})
	}
	for i := range boards {
		next <- i
	}
	close(next)
	wg.Wait()
	return results
}
