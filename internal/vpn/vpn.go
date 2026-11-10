// Package vpn is MiddleMan's on-device VPN: a userspace TCP/IP stack
// (gVisor's netstack) on a TUN file descriptor that terminates every
// connection the device's apps make and re-makes it from MiddleMan's own
// sockets. Nothing leaves the device through a remote server; the tunnel
// only exists so MiddleMan can see, attribute and block each flow.
//
// Android has hidden other apps' sockets since Android 10. A local
// VpnService (the approach of NetGuard and PCAPdroid) gives per-app
// destinations, DNS names and blocking back. The Java side
// (android/java/com/middlelan/app/MiddleVpnService.java) establishes the
// tunnel and hands its descriptor to Default through nativeStart; the
// engine itself is plain Go and runs on any Linux TUN device, which is how
// it is tested.
package vpn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// VirtualDNS is the resolver address the VPN advertises to apps; queries
// to it are forwarded to the underlying network's DNS servers.
const VirtualDNS = "10.111.0.2"

var virtualDNS = netip.MustParseAddr(VirtualDNS)

const (
	maxFlows       = 2048             // open flows at once
	maxClosed      = 4096             // closed flows remembered
	closedKeep     = 2 * time.Minute  // how long closed flows stay in Flows
	dialTimeout    = 10 * time.Second // TCP connect to the real destination
	udpIdle        = 60 * time.Second // UDP flow idle timeout
	dnsIdle        = 10 * time.Second // idle timeout of port-53 flows
	halfCloseIdle  = 60 * time.Second // idle limit once one TCP direction ended
	dnsTimeout     = 10 * time.Second // one forwarded DNS query, all servers
	dnsPerServer   = 4 * time.Second  // one forwarded DNS query, one server
	maxDNSInFlight = 64               // forwarded DNS queries at once
	stopWait       = 3 * time.Second  // Stop waits this long for flow goroutines
	copyBufSize    = 32 << 10
)

var (
	// ErrUnsupported is returned where the VPN can't run (anything but
	// Android for the control functions, anything but Linux for Engine).
	ErrUnsupported = errors.New("vpn: not supported on this platform")
	// ErrRunning is returned by Start when the engine already runs.
	ErrRunning = errors.New("vpn: already running")

	errTooMany = errors.New("vpn: too many open flows")
	errStopped = errors.New("vpn: engine stopped")
)

// Flow is one TCP connection or UDP association made by an app through
// the tunnel. Local is the app's side, Remote the destination it asked
// for.
type Flow struct {
	Key        string // unique for the engine's lifetime
	Proto      string // "tcp", "tcp6", "udp" or "udp6"
	LocalIP    string
	LocalPort  int
	RemoteIP   string
	RemotePort int
	UID        int    // owning app's uid, -1 when unknown
	App        string // owning app's label
	Package    string // owning app's package name
	Sent, Recv uint64 // bytes from the app, bytes to the app
	Started    time.Time
	LastActive time.Time
	Closed     bool
	Blocked    bool
}

// Hooks connect the engine to the rest of MiddleMan. Every hook is
// optional and may be called from many goroutines at once.
type Hooks struct {
	// Owner names the app owning a new flow; proto is the IP protocol
	// number (6 for TCP, 17 for UDP), src the app's side.
	Owner func(proto int, srcIP string, srcPort int, dstIP string, dstPort int) (uid int, app, pkg string)
	// Block decides once per new flow whether to refuse it: blocked TCP
	// gets a reset, blocked UDP is dropped.
	Block func(app, pkg, ip string, port int) bool
	// DNS receives every DNS response passing the virtual resolver (and
	// any other port-53 flow). The slice is the hook's to keep.
	DNS func(response []byte)
	// Filter decides whether to refuse a DNS query for name (lowercase,
	// without the final dot) from an app: ad and tracker blocking. A
	// refused A or AAAA query is answered with the unspecified address,
	// any other type with NXDOMAIN. With Filter set, plain DNS to any
	// server goes through the filter and the upstream servers too.
	Filter func(app, pkg, name string) bool
}

// run is one Start..Stop cycle of an engine.
type run struct {
	ctx    context.Context // canceled by Stop; ends dials and DNS queries
	cancel context.CancelFunc
	wg     sync.WaitGroup // flow goroutines
	st     *stackState    // the platform part
}

// flow is the engine's record of a Flow.
type flow struct {
	Flow // Key..Started are fixed; the rest is guarded by Engine.mu

	run        *run
	src, dst   netip.AddrPort
	sent, recv atomic.Uint64
	last       atomic.Int64 // unix nanoseconds
	closers    []io.Closer  // guarded by Engine.mu
	ended      bool         // guarded by Engine.mu
	endedAt    time.Time    // guarded by Engine.mu
}

func (f *flow) touch() { f.last.Store(time.Now().UnixNano()) }

func (f *flow) idleFor() time.Duration {
	return time.Since(time.Unix(0, f.last.Load()))
}

// Engine runs the userspace stack on one TUN descriptor at a time.
type Engine struct {
	hooks atomic.Pointer[Hooks]
	dns   atomic.Pointer[[]string]

	// dial makes the real connections; tests replace it to point
	// tunnel addresses at local servers.
	dial func(ctx context.Context, network, address string) (net.Conn, error)
	// onStop runs (in its own goroutine) after the engine stopped.
	onStop func()

	mu      sync.Mutex
	running bool
	cur     *run // the current run, nil when stopped
	open    map[*flow]struct{}
	closed  []*flow // oldest first
	seq     uint64
	dnsSem  chan struct{}

	rx, tx atomic.Uint64
}

// NewEngine returns a stopped engine using h.
func NewEngine(h Hooks) *Engine {
	e := &Engine{
		open:   map[*flow]struct{}{},
		dnsSem: make(chan struct{}, maxDNSInFlight),
	}
	e.dial = netDial
	e.hooks.Store(&h)
	e.dns.Store(&[]string{})
	return e
}

// SetHooks replaces the engine's hooks; flows started afterwards use them.
func (e *Engine) SetHooks(h Hooks) { e.hooks.Store(&h) }

// SetDNS replaces the upstream DNS servers ("ip" or "ip:port") the
// virtual resolver forwards to, for when the underlying network changes.
func (e *Engine) SetDNS(servers []string) {
	s := normalizeDNS(servers)
	e.dns.Store(&s)
}

// DNSServers returns the upstream DNS servers in use.
func (e *Engine) DNSServers() []string { return append([]string(nil), *e.dns.Load()...) }

// Running reports whether the engine is running.
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// Totals returns the bytes received by and sent from apps through the
// tunnel since the engine was created.
func (e *Engine) Totals() (rx, tx uint64) { return e.rx.Load(), e.tx.Load() }

// Flows returns the open flows and those closed in the last two minutes,
// newest first.
func (e *Engine) Flows() []Flow {
	e.mu.Lock()
	e.pruneLocked(time.Now())
	out := make([]Flow, 0, len(e.open)+len(e.closed))
	for f := range e.open {
		out = append(out, e.snapshotLocked(f))
	}
	for _, f := range e.closed {
		out = append(out, e.snapshotLocked(f))
	}
	e.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Started.Equal(out[j].Started) {
			return out[i].Started.After(out[j].Started)
		}
		return out[i].Key > out[j].Key
	})
	return out
}

func (e *Engine) snapshotLocked(f *flow) Flow {
	s := f.Flow
	s.Sent = f.sent.Load()
	s.Recv = f.recv.Load()
	s.LastActive = time.Unix(0, f.last.Load())
	return s
}

func (e *Engine) pruneLocked(now time.Time) {
	i := 0
	for i < len(e.closed) && (len(e.closed)-i > maxClosed || now.Sub(e.closed[i].endedAt) > closedKeep) {
		e.closed[i] = nil
		i++
	}
	if i > 0 {
		e.closed = append(e.closed[:0], e.closed[i:]...)
	}
}

func newRun(st *stackState) *run {
	ctx, cancel := context.WithCancel(context.Background())
	return &run{ctx: ctx, cancel: cancel, st: st}
}

// beginLocked makes r the current run; the platform code calls it once
// its stack is up.
func (e *Engine) beginLocked(r *run) {
	e.running = true
	e.cur = r
}

// endLocked marks the current run as stopped and returns it with the
// connections of its open flows, for the caller to close outside the lock.
func (e *Engine) endLocked() (*run, []io.Closer) {
	r := e.cur
	if r == nil {
		return nil, nil
	}
	e.running = false
	e.cur = nil
	r.cancel()
	var cs []io.Closer
	for f := range e.open {
		cs = append(cs, f.closers...)
		f.closers = nil
	}
	return r, cs
}

// waitFlows waits, up to stopWait, for r's flow goroutines.
func (r *run) waitFlows() bool {
	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(stopWait):
		return false
	}
}

// newFlow registers a flow from src (the app) to dst. The caller must
// hand it to endFlow exactly once.
func (e *Engine) newFlow(udp bool, src, dst netip.AddrPort) (*flow, error) {
	src = netip.AddrPortFrom(src.Addr().Unmap(), src.Port())
	dst = netip.AddrPortFrom(dst.Addr().Unmap(), dst.Port())
	proto := "tcp"
	if udp {
		proto = "udp"
	}
	if dst.Addr().Is6() {
		proto += "6"
	}
	now := time.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running {
		return nil, errStopped
	}
	if len(e.open) >= maxFlows {
		return nil, errTooMany
	}
	e.seq++
	f := &flow{
		Flow: Flow{
			Key:        fmt.Sprintf("vpn-%d", e.seq),
			Proto:      proto,
			LocalIP:    src.Addr().String(),
			LocalPort:  int(src.Port()),
			RemoteIP:   dst.Addr().String(),
			RemotePort: int(dst.Port()),
			UID:        -1,
			Started:    now,
		},
		run: e.cur,
		src: src,
		dst: dst,
	}
	f.last.Store(now.UnixNano())
	e.open[f] = struct{}{}
	f.run.wg.Add(1)
	return f, nil
}

// decide looks up the flow's owner and asks the Block hook; it reports
// whether the flow is blocked.
func (e *Engine) decide(f *flow) bool {
	h := e.hooks.Load()
	uid, app, pkg := -1, "", ""
	if h.Owner != nil {
		proto := 6
		if strings.HasPrefix(f.Proto, "udp") {
			proto = 17
		}
		uid, app, pkg = h.Owner(proto, f.LocalIP, f.LocalPort, f.RemoteIP, f.RemotePort)
	}
	blocked := h.Block != nil && h.Block(app, pkg, f.RemoteIP, f.RemotePort)
	e.mu.Lock()
	f.UID, f.App, f.Package, f.Blocked = uid, app, pkg, blocked
	e.mu.Unlock()
	return blocked
}

// addCloser ties c to f: it is closed when the flow ends or the engine
// stops. If either already happened, c is closed at once and false is
// returned.
func (e *Engine) addCloser(f *flow, c io.Closer) bool {
	e.mu.Lock()
	if f.ended || !e.running {
		e.mu.Unlock()
		c.Close()
		return false
	}
	f.closers = append(f.closers, c)
	e.mu.Unlock()
	return true
}

// endFlow closes everything tied to f and moves it to the closed list.
func (e *Engine) endFlow(f *flow) {
	e.mu.Lock()
	if f.ended {
		e.mu.Unlock()
		return
	}
	f.ended = true
	f.Closed = true
	f.endedAt = time.Now()
	cs := f.closers
	f.closers = nil
	delete(e.open, f)
	e.closed = append(e.closed, f)
	e.pruneLocked(f.endedAt)
	e.mu.Unlock()
	for _, c := range cs {
		c.Close()
	}
	f.run.wg.Done()
}

func (e *Engine) addSent(f *flow, n int) {
	f.sent.Add(uint64(n))
	e.tx.Add(uint64(n))
	f.touch()
}

func (e *Engine) addRecv(f *flow, n int) {
	f.recv.Add(uint64(n))
	e.rx.Add(uint64(n))
	f.touch()
}

// netDial makes a real connection from MiddleMan's own sockets (which
// bypass the tunnel).
func netDial(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}

// dialWithTimeout dials with dialTimeout on top of ctx.
func (e *Engine) dialWithTimeout(ctx context.Context, network, address string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	return e.dial(ctx, network, address)
}

// normalizeDNS turns "ip" and "ip:port" entries into dialable addresses,
// dropping empty ones and the virtual resolver itself.
func normalizeDNS(servers []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range servers {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		host, port := s, "53"
		if h, p, err := net.SplitHostPort(s); err == nil {
			host, port = h, p
		} else {
			host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
		}
		if a, err := netip.ParseAddr(host); err == nil {
			if a.Unmap() == virtualDNS && port == "53" {
				continue
			}
		} else if strings.Contains(host, "/") {
			continue
		}
		addr := net.JoinHostPort(host, port)
		if !seen[addr] {
			seen[addr] = true
			out = append(out, addr)
		}
	}
	return out
}

// SplitDNS splits a comma (or space) separated server list, as the Java
// side passes it.
func SplitDNS(list string) []string {
	return strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == ' ' || r == ';' || r == '\n' })
}
