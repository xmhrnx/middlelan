package vpn

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNormalizeDNS(t *testing.T) {
	got := normalizeDNS([]string{" 192.168.1.1 ", "", "10.111.0.2", "2001:db8::1", "[2001:db8::2]:5353",
		"127.0.0.1:5353", "192.168.1.1", "fe80::1%wlan0", "not/an/ip"})
	want := []string{"192.168.1.1:53", "[2001:db8::1]:53", "[2001:db8::2]:5353", "127.0.0.1:5353", "[fe80::1%wlan0]:53"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeDNS = %q, want %q", got, want)
	}
	if got := SplitDNS("1.1.1.1,8.8.8.8, 9.9.9.9;"); !reflect.DeepEqual(got, []string{"1.1.1.1", "8.8.8.8", "9.9.9.9"}) {
		t.Fatalf("SplitDNS = %q", got)
	}
}

func TestOwnerLookupCachesLabels(t *testing.T) {
	var labels atomic.Int32
	o := newOwnerLookup(func(proto int, src string, sport int, dst string, dport int) int {
		if dport == 1 {
			return -1
		}
		return 10123
	}, func(uid int) string {
		labels.Add(1)
		return "Chrome\tcom.android.chrome\n"
	})
	for range 3 {
		uid, app, pkg := o.Owner(6, "10.111.0.1", 40000, "1.2.3.4", 443)
		if uid != 10123 || app != "Chrome" || pkg != "com.android.chrome" {
			t.Fatalf("Owner = %d %q %q", uid, app, pkg)
		}
	}
	if labels.Load() != 1 {
		t.Fatalf("label looked up %d times, want 1", labels.Load())
	}
	if uid, app, _ := o.Owner(6, "10.111.0.1", 40000, "1.2.3.4", 1); uid != -1 || app != "" {
		t.Fatalf("unknown owner = %d %q", uid, app)
	}
	if app, pkg := parseLabel("", 42); app != "uid 42" || pkg != "" {
		t.Fatalf("parseLabel empty = %q %q", app, pkg)
	}
	if app, pkg := parseLabel("\tcom.x", 42); app != "com.x" || pkg != "com.x" {
		t.Fatalf("parseLabel no label = %q %q", app, pkg)
	}
}

// running returns an engine with a run in progress but no stack, for
// exercising the flow table and relays.
func running(h Hooks) *Engine {
	e := NewEngine(h)
	e.mu.Lock()
	e.beginLocked(newRun(nil))
	e.mu.Unlock()
	return e
}

var (
	appAddr = netip.MustParseAddrPort("10.111.0.1:40000")
	dstAddr = netip.MustParseAddrPort("93.184.216.34:443")
)

func TestFlowTable(t *testing.T) {
	stopped := NewEngine(Hooks{})
	if _, err := stopped.newFlow(false, appAddr, dstAddr); !errors.Is(err, errStopped) {
		t.Fatalf("newFlow on a stopped engine: %v", err)
	}

	e := running(Hooks{
		Owner: func(proto int, srcIP string, srcPort int, dstIP string, dstPort int) (int, string, string) {
			if proto != 17 || srcIP != "10.111.0.1" || srcPort != 40000 || dstIP != "2001:db8::1" || dstPort != 53 {
				t.Errorf("Owner(%d, %s, %d, %s, %d)", proto, srcIP, srcPort, dstIP, dstPort)
			}
			return 10001, "Maps", "com.maps"
		},
		Block: func(app, pkg, ip string, port int) bool { return pkg == "com.maps" },
	})
	f, err := e.newFlow(true, appAddr, netip.MustParseAddrPort("[2001:db8::1]:53"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Proto != "udp6" || f.UID != -1 {
		t.Fatalf("new flow: %+v", f.Flow)
	}
	if !e.decide(f) {
		t.Fatal("decide: not blocked")
	}
	e.addSent(f, 100)
	e.addRecv(f, 250)
	fl := e.Flows()
	if len(fl) != 1 || fl[0].UID != 10001 || fl[0].App != "Maps" || !fl[0].Blocked || fl[0].Closed || fl[0].Sent != 100 || fl[0].Recv != 250 {
		t.Fatalf("Flows = %+v", fl)
	}
	var closed atomic.Bool
	e.addCloser(f, closerFunc(func() { closed.Store(true) }))
	e.endFlow(f)
	e.endFlow(f) // twice is harmless
	if !closed.Load() {
		t.Fatal("endFlow didn't close the flow's connections")
	}
	if fl := e.Flows(); len(fl) != 1 || !fl[0].Closed {
		t.Fatalf("Flows after end = %+v", fl)
	}
	if rx, tx := e.Totals(); rx != 250 || tx != 100 {
		t.Fatalf("Totals = %d, %d", rx, tx)
	}
	// A closer added after the end is closed at once.
	var late atomic.Bool
	if e.addCloser(f, closerFunc(func() { late.Store(true) })) || !late.Load() {
		t.Fatal("late closer not closed")
	}

	// Closed flows age out after two minutes.
	e.mu.Lock()
	f.endedAt = time.Now().Add(-closedKeep - time.Second)
	e.mu.Unlock()
	if fl := e.Flows(); len(fl) != 0 {
		t.Fatalf("expired flow still listed: %+v", fl)
	}

	// The open flow limit.
	var flows []*flow
	for i := range maxFlows {
		f, err := e.newFlow(false, appAddr, netip.AddrPortFrom(dstAddr.Addr(), uint16(i+1)))
		if err != nil {
			t.Fatalf("flow %d: %v", i, err)
		}
		flows = append(flows, f)
	}
	if _, err := e.newFlow(false, appAddr, dstAddr); !errors.Is(err, errTooMany) {
		t.Fatalf("flow over the limit: %v", err)
	}
	for _, f := range flows {
		e.endFlow(f)
	}
	if n := len(e.Flows()); n != maxFlows {
		t.Fatalf("%d flows listed, want %d", n, maxFlows)
	}
	if g, err := e.newFlow(false, appAddr, dstAddr); err != nil {
		t.Fatalf("flow after closing: %v", err)
	} else {
		e.endFlow(g)
	}

	// Ending the run closes every open flow's connections.
	f, _ = e.newFlow(false, appAddr, dstAddr)
	var aborted atomic.Bool
	e.addCloser(f, closerFunc(func() { aborted.Store(true) }))
	e.mu.Lock()
	r, cs := e.endLocked()
	e.mu.Unlock()
	for _, c := range cs {
		c.Close()
	}
	if !aborted.Load() || e.Running() {
		t.Fatal("endLocked didn't abort the open flows")
	}
	e.endFlow(f)
	if !r.waitFlows() {
		t.Fatal("flows of the run still counted as running")
	}
}

type closerFunc func()

func (c closerFunc) Close() error { c(); return nil }

// tcpPair returns both ends of a loopback TCP connection.
func tcpPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ch := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		ch <- c
	}()
	a, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	b := <-ch
	t.Cleanup(func() { a.Close(); b.Close() })
	return a, b
}

func TestSpliceTCP(t *testing.T) {
	e := running(Hooks{})
	f, _ := e.newFlow(false, appAddr, dstAddr)
	app, appInner := tcpPair(t)       // app writes to app; the engine holds appInner
	remoteInner, server := tcpPair(t) // the engine holds remoteInner; server is the destination
	done := make(chan struct{})
	go func() {
		e.spliceTCP(f, appInner, remoteInner)
		e.endFlow(f)
		close(done)
	}()

	req := bytes.Repeat([]byte("ping"), 10000)
	go func() {
		app.Write(req)
		app.(*net.TCPConn).CloseWrite()
	}()
	got, err := io.ReadAll(server)
	if err != nil || !bytes.Equal(got, req) {
		t.Fatalf("server got %d bytes (%v), want %d", len(got), err, len(req))
	}
	resp := bytes.Repeat([]byte("pong!"), 7000)
	server.Write(resp)
	server.Close()
	got, err = io.ReadAll(app)
	if err != nil || !bytes.Equal(got, resp) {
		t.Fatalf("app got %d bytes (%v), want %d", len(got), err, len(resp))
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("splice didn't end")
	}
	fl := e.Flows()
	if len(fl) != 1 || fl[0].Sent != uint64(len(req)) || fl[0].Recv != uint64(len(resp)) || !fl[0].Closed {
		t.Fatalf("Flows = %+v", fl)
	}
}

func TestDNSStream(t *testing.T) {
	msg := func(id byte, n int) []byte {
		m := make([]byte, n)
		m[1] = id
		b := binary.BigEndian.AppendUint16(nil, uint16(n))
		return append(b, m...)
	}
	stream := append(msg(1, 40), msg(2, 300)...)
	var got []byte
	var s dnsStream
	for i := 0; i < len(stream); i += 7 { // arrives in small pieces
		s.feed(stream[i:min(i+7, len(stream))], func(m []byte) { got = append(got, m[1]) })
	}
	if string(got) != "\x01\x02" || len(s.buf) != 0 {
		t.Fatalf("messages %v, left %d", got, len(s.buf))
	}
	var bad dnsStream
	bad.feed([]byte("GET / HTTP/1.1\r\n"), func([]byte) { t.Fatal("not DNS") })
	bad.feed(msg(3, 40), func([]byte) { t.Fatal("after giving up") })
}

// fakeDNS answers every query by echoing it with the response bit set.
func fakeDNS(t *testing.T) (addr string, queries *atomic.Int32) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	queries = new(atomic.Int32)
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			queries.Add(1)
			resp := append([]byte(nil), buf[:n]...)
			resp[2] |= 0x80
			pc.WriteTo(resp, from)
		}
	}()
	return pc.LocalAddr().String(), queries
}

func dnsQuery(id uint16) []byte {
	q := []byte{byte(id >> 8), byte(id), 1, 0, 0, 1, 0, 0, 0, 0, 0, 0}
	q = append(q, 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0, 0, 1, 0, 1)
	return q
}

func TestExchange(t *testing.T) {
	addr, queries := fakeDNS(t)
	// A dead server first: the query moves on to the next one.
	dead, _ := net.ListenPacket("udp", "127.0.0.1:0")
	deadAddr := dead.LocalAddr().String()
	dead.Close()
	e := NewEngine(Hooks{})
	e.SetDNS([]string{deadAddr, addr})
	resp, err := e.exchange(context.Background(), dnsQuery(0x1234))
	if err != nil {
		t.Fatal(err)
	}
	if resp[0] != 0x12 || resp[1] != 0x34 || resp[2]&0x80 == 0 || queries.Load() != 1 {
		t.Fatalf("response %x after %d queries", resp, queries.Load())
	}
	e.SetDNS(nil)
	if _, err := e.exchange(context.Background(), dnsQuery(1)); err == nil {
		t.Fatal("exchange without servers succeeded")
	}
}

func TestServeDNSAndRelayUDP(t *testing.T) {
	addr, _ := fakeDNS(t)
	var mu sync.Mutex
	var seen [][]byte
	e := running(Hooks{DNS: func(b []byte) { mu.Lock(); seen = append(seen, b); mu.Unlock() }})
	e.SetDNS([]string{addr})

	// The virtual resolver: the app's side is a connected UDP pair.
	appSide, engineSide := udpPair(t)
	f, _ := e.newFlow(true, appAddr, netip.MustParseAddrPort(VirtualDNS+":53"))
	go func() { e.serveDNS(f, engineSide); e.endFlow(f) }()
	appSide.Write(dnsQuery(0xbeef))
	appSide.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 1500)
	n, err := appSide.Read(buf)
	if err != nil || n < 12 || buf[0] != 0xbe || buf[1] != 0xef {
		t.Fatalf("answer %x, %v", buf[:n], err)
	}
	mu.Lock()
	if len(seen) != 1 || seen[0][0] != 0xbe {
		t.Fatalf("DNS hook saw %d responses", len(seen))
	}
	mu.Unlock()

	// A plain UDP flow to port 53 is relayed and tapped too.
	appSide2, engineSide2 := udpPair(t)
	remote, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	f2, _ := e.newFlow(true, appAddr, netip.MustParseAddrPort("192.0.2.1:53"))
	done := make(chan struct{})
	go func() { e.relayUDP(f2, engineSide2, remote, 300*time.Millisecond); e.endFlow(f2); close(done) }()
	appSide2.Write(dnsQuery(0x0102))
	appSide2.SetReadDeadline(time.Now().Add(5 * time.Second))
	if n, err := appSide2.Read(buf); err != nil || buf[0] != 1 || buf[1] != 2 {
		t.Fatalf("relayed answer %x, %v", buf[:n], err)
	}
	select {
	case <-done: // idle timeout
	case <-time.After(5 * time.Second):
		t.Fatal("UDP relay didn't time out")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Fatalf("DNS hook saw %d responses, want 2", len(seen))
	}
	for _, fl := range e.Flows() {
		if fl.Key == f2.Key && (fl.Sent != uint64(len(dnsQuery(0))) || fl.Recv != fl.Sent || !fl.Closed) {
			t.Fatalf("relayed flow %+v", fl)
		}
	}
}

// udpPair returns two UDP sockets that talk to each other, both usable
// with Read and Write.
func udpPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	a, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := net.DialUDP("udp", nil, a.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close(); b.Close() })
	return b, &peerUDP{a, b.LocalAddr()}
}

type peerUDP struct {
	*net.UDPConn
	peer net.Addr
}

func (p *peerUDP) Write(b []byte) (int, error) { return p.WriteTo(b, p.peer) }

func TestControlStubs(t *testing.T) {
	if Supported() {
		t.Skip("platform has the VPN")
	}
	if _, err := Prepare(); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	if err := Start(); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	SetHooks(Hooks{})
	if Default.Running() {
		t.Fatal("Default running")
	}
}
