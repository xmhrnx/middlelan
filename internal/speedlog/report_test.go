package speedlog

import (
	"testing"
	"time"
)

func TestReport(t *testing.T) {
	base := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	var rs []Result
	for i := 0; i < 10; i++ {
		d := 95.0
		if i >= 8 {
			d = 40 // two slow tests
		}
		rs = append(rs, Result{When: base.Add(time.Duration(i) * time.Hour), DownloadMbps: d, UploadMbps: 20, LatencyMs: 12})
	}
	r := BuildReport(rs, Plan{DownloadMbps: 100, UploadMbps: 20})
	if r.Grade != "B" || r.MedianDown != 95 || r.UpPct != 100 || r.Tests != 10 {
		t.Fatalf("%+v", r)
	}
	if e := BuildReport(nil, Plan{}); e.Grade != "–" {
		t.Fatal(e)
	}
	if !Due(time.Time{}, time.Hour, base) || Due(base, time.Hour, base.Add(time.Minute)) {
		t.Fatal("due")
	}
}

func TestLogKeepsOrder(t *testing.T) {
	l := Open(t.TempDir())
	now := time.Now()
	if err := l.Add(Result{When: now}); err != nil {
		t.Fatal(err)
	}
	if err := l.Add(Result{When: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	rs := l.Results(time.Time{})
	if len(rs) != 2 || !rs[0].When.Before(rs[1].When) {
		t.Fatal(rs)
	}
	if last, _ := l.Last(); !last.When.Equal(now) {
		t.Fatal(last)
	}
}
