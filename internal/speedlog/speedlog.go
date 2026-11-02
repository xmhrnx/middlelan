// Package speedlog keeps speed test results and grades the connection
// against what the user's plan promises: an ISP report card.
package speedlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Result is one speed test.
type Result struct {
	When         time.Time `json:"when"`
	DownloadMbps float64   `json:"down"`
	UploadMbps   float64   `json:"up"`
	LatencyMs    float64   `json:"ping"`
	JitterMs     float64   `json:"jitter"`
	Scheduled    bool      `json:"scheduled,omitempty"`
	ISP          string    `json:"isp,omitempty"`
}

// maxResults bounds the history (a year of tests every 6 hours).
const maxResults = 1500

// Log is the saved history. Safe from any goroutine.
type Log struct {
	mu      sync.Mutex
	path    string
	results []Result
}

// Open loads the history from dir ("" keeps it in memory).
func Open(dir string) *Log {
	l := &Log{}
	if dir == "" {
		return l
	}
	l.path = filepath.Join(dir, "speedtests.json")
	if b, err := os.ReadFile(l.path); err == nil {
		_ = json.Unmarshal(b, &l.results) // an unreadable history starts fresh
	}
	return l
}

// Add records a result and saves.
func (l *Log) Add(r Result) error {
	l.mu.Lock()
	l.results = append(l.results, r)
	sort.Slice(l.results, func(i, j int) bool { return l.results[i].When.Before(l.results[j].When) })
	if len(l.results) > maxResults {
		l.results = l.results[len(l.results)-maxResults:]
	}
	data, err := json.Marshal(l.results)
	l.mu.Unlock()
	if err != nil || l.path == "" {
		return err
	}
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, l.path)
}

// Results returns the results since t, oldest first.
func (l *Log) Results(since time.Time) []Result {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []Result{}
	for _, r := range l.results {
		if !r.When.Before(since) {
			out = append(out, r)
		}
	}
	return out
}

// Last returns the newest result.
func (l *Log) Last() (Result, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.results) == 0 {
		return Result{}, false
	}
	return l.results[len(l.results)-1], true
}
