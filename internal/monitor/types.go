// Package monitor watches the network connections of every process on the
// computer and attributes traffic to the apps that make it. It has no UI
// dependencies; the UI reads snapshots from a Collector.
package monitor

import "time"

// Conn is one socket as reported by the operating system.
type Conn struct {
	// Key identifies the socket across samples (an inode, cookie or
	// endpoint tuple), so byte counters can be turned into rates.
	Key        string
	Proto      string // "tcp", "tcp6", "udp" or "udp6"
	LocalIP    string
	LocalPort  int
	RemoteIP   string
	RemotePort int
	State      string // "ESTABLISHED", "LISTEN", ... ("" for UDP)
	PID        int    // 0 when the owner can't be seen
	UID        int    // -1 when unknown
	Process    string // short process name
	Exe        string // full executable path when known
	// Sent and Recv are cumulative byte counters of the socket, when the
	// platform reports them (HasBytes).
	Sent, Recv uint64
	HasBytes   bool
	RTT        time.Duration
}

// Listening reports whether the socket waits for connections rather than
// talking to a remote host.
func (c Conn) Listening() bool {
	return c.State == "LISTEN" || c.RemoteIP == "" || c.RemoteIP == "0.0.0.0" || c.RemoteIP == "::" || c.RemotePort == 0
}

// ByteCount is a pair of cumulative byte counters.
type ByteCount struct{ Rx, Tx uint64 }

// Sample is one reading from a Source.
type Sample struct {
	Conns []Conn
	// Proc holds cumulative per-process counters (by PID) on platforms
	// that count per process rather than per socket.
	Proc map[int]ProcBytes
	// Iface is the sum of all non-loopback interface counters.
	Iface ByteCount
	// PerApp reports whether Conns or Proc carry byte counters, so rates
	// per app are real rather than unknown.
	PerApp bool
	// AppTotals holds per-app cumulative counters reported directly by the
	// platform (Android's NetworkStatsManager), keyed by app name.
	AppTotals map[string]ByteCount
}

// ProcBytes is a process's cumulative traffic.
type ProcBytes struct {
	Name string
	Exe  string
	ByteCount
}

// Source reads connections and counters from the operating system.
type Source interface {
	Sample() (Sample, error)
	// Limits explains what the platform can't see, for the UI ("" when
	// everything is visible).
	Limits() string
}

// Endpoint is a remote host an app talked to.
type Endpoint struct {
	IP        string
	Port      int
	Proto     string
	Host      string // reverse DNS name, when resolved
	Rx, Tx    uint64 // bytes attributed to this endpoint
	FirstSeen time.Time
	LastSeen  time.Time
	Active    bool // has an open connection right now
}

// App is the aggregated network activity of one program.
type App struct {
	Name       string
	Exe        string
	PIDs       []int
	Conns      []Conn // open sockets right now
	Active     int    // connected (non-listening) sockets
	Listening  int
	RateRx     float64 // bytes per second over the last interval
	RateTx     float64
	SessionRx  uint64 // since the collector started
	SessionTx  uint64
	TodayRx    uint64 // today, including earlier runs
	TodayTx    uint64
	Endpoints  []Endpoint // most recent first
	FirstSeen  time.Time
	LastSeen   time.Time
	History    []float64 // recent combined rate samples, oldest first
	KnownBytes bool      // whether byte counts are measured for this app
}

// Snapshot is the collector's view at one moment.
type Snapshot struct {
	Time     time.Time
	Apps     []App // busiest first
	Conns    []Conn
	RateRx   float64 // whole-computer throughput, bytes per second
	RateTx   float64
	TotalRx  uint64 // since the collector started
	TotalTx  uint64
	TodayRx  uint64
	TodayTx  uint64
	History  []RatePoint // whole-computer rate history, oldest first
	PerApp   bool
	Limits   string
	Err      string
	Interval time.Duration
}

// RatePoint is one throughput reading.
type RatePoint struct {
	Time   time.Time
	Rx, Tx float64
}
