package monitor

import (
	"testing"
	"time"
)

type fakeSource struct{ n int }

func (f *fakeSource) Limits() string { return "" }
func (f *fakeSource) Sample() (Sample, error) {
	f.n++
	b := uint64(f.n) * 1000
	return Sample{
		PerApp: true,
		Iface:  ByteCount{Rx: b * 3, Tx: b},
		Conns: []Conn{
			{Key: "a", Proto: "tcp", Process: "browser", RemoteIP: "1.2.3.4", RemotePort: 443, State: "ESTABLISHED", Recv: b * 2, Sent: b / 2, HasBytes: true},
			{Key: "b", Proto: "tcp", Process: "server", LocalPort: 80, State: "LISTEN", HasBytes: true},
		},
	}, nil
}

func TestCollectorAttributes(t *testing.T) {
	c := NewCollector(&fakeSource{}, Options{Interval: time.Second})
	c.sample()
	c.prevTime = time.Now().Add(-time.Second)
	c.sample()
	s := c.Snapshot()
	if len(s.Apps) != 2 || s.Apps[0].Name != "browser" {
		t.Fatalf("apps: %+v", s.Apps)
	}
	b := s.Apps[0]
	if b.SessionRx != 2000 || b.SessionTx != 500 || b.Active != 1 || len(b.Endpoints) != 1 {
		t.Fatalf("browser: %+v", b)
	}
	if s.TotalRx != 3000 || s.Apps[1].Listening != 1 {
		t.Fatalf("snapshot: %+v", s)
	}
	if d := c.Store().Destinations("browser"); len(d) != 1 || d[0].Rx != 2000 {
		t.Fatalf("dest: %+v", d)
	}
}

func TestStoreHourly(t *testing.T) {
	s := OpenStore("")
	now := time.Date(2026, 11, 2, 15, 30, 0, 0, time.Local)
	s.Add("a", now, 100, 10)
	s.Add("b", now.Add(-time.Hour), 50, 5)
	h := s.Hourly("", 3, now)
	if len(h) != 3 || h[2].Rx != 100 || h[1].Rx != 50 || h[0].Rx != 0 {
		t.Fatalf("%+v", h)
	}
	if a := s.Hourly("a", 2, now); a[1].Tx != 10 || a[0].Tx != 0 {
		t.Fatalf("%+v", a)
	}
	s.SetToday("c", now, ByteCount{Rx: 70})
	s.SetToday("c", now, ByteCount{Rx: 100})
	if got := s.HourApps(now)["c"].Rx; got != 100 {
		t.Fatalf("SetToday hourly share %d", got)
	}
}
