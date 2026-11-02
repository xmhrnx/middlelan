//go:build linux

package vpn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/sys/unix"
)

func TestIsICMP(t *testing.T) {
	v4 := make([]byte, 20)
	v4[0] = 0x45
	v4[9] = 1
	v6 := make([]byte, 40)
	v6[0] = 0x60
	v6[6] = 58
	if !isICMP(v4) || !isICMP(v6) {
		t.Fatal("ICMP not recognized")
	}
	v4[9], v6[6] = 6, 17
	if isICMP(v4) || isICMP(v6) || isICMP(nil) {
		t.Fatal("TCP/UDP taken for ICMP")
	}
}

// The tunnel's addresses in the test: the host side is 10.111.0.1/24, so
// 10.111.0.0/24 (the virtual resolver 10.111.0.2 and the stand-in
// destination 10.111.0.10) is routed into the TUN device.
const (
	tunHost = "10.111.0.1"
	tunDest = "10.111.0.10"
)

// openTUN creates a TUN interface (no packet information header) with
// address tunHost/24 and brings it up. Closing the descriptor removes it.
func openTUN(t *testing.T) (fd int, name string) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("needs root to create a TUN device")
	}
	if _, err := os.Stat("/dev/net/tun"); err != nil {
		t.Skip("no /dev/net/tun")
	}
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Skipf("open /dev/net/tun: %v", err)
	}
	name = fmt.Sprintf("mltest%d", rand.IntN(10000))
	ifr, err := unix.NewIfreq(name)
	if err != nil {
		t.Fatal(err)
	}
	ifr.SetUint16(unix.IFF_TUN | unix.IFF_NO_PI)
	if err := unix.IoctlIfreq(fd, unix.TUNSETIFF, ifr); err != nil {
		unix.Close(fd)
		t.Skipf("TUNSETIFF: %v", err)
	}
	sock, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		unix.Close(fd)
		t.Fatal(err)
	}
	defer unix.Close(sock)
	set := func(req uint, ip net.IP) {
		r, _ := unix.NewIfreq(name)
		if err := r.SetInet4Addr(ip.To4()); err != nil {
			t.Fatal(err)
		}
		if err := unix.IoctlIfreq(sock, req, r); err != nil {
			unix.Close(fd)
			t.Skipf("configure %s: %v", name, err)
		}
	}
	set(unix.SIOCSIFADDR, net.ParseIP(tunHost))
	set(unix.SIOCSIFNETMASK, net.IPv4(255, 255, 255, 0))
	r, _ := unix.NewIfreq(name)
	if err := unix.IoctlIfreq(sock, unix.SIOCGIFFLAGS, r); err != nil {
		t.Fatal(err)
	}
	r.SetUint16(r.Uint16() | unix.IFF_UP | unix.IFF_RUNNING)
	if err := unix.IoctlIfreq(sock, unix.SIOCSIFFLAGS, r); err != nil {
		unix.Close(fd)
		t.Skipf("bring %s up: %v", name, err)
	}
	return fd, name
}

// echoServer echoes TCP on 127.0.0.1 and returns its port.
func echoServer(t *testing.T) int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				io.Copy(c, c)
				c.Close()
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// udpEcho echoes UDP on 127.0.0.1 and returns its port.
func udpEcho(t *testing.T) int {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			pc.WriteTo(buf[:n], from)
		}
	}()
	return pc.LocalAddr().(*net.UDPAddr).Port
}

func TestEngineOnTUN(t *testing.T) {
	fd, name := openTUN(t)
	echoPort := echoServer(t)
	udpPort := udpEcho(t)
	dnsAddr, dnsQueries := fakeDNS(t)
	blockedLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer blockedLn.Close()
	blockedPort := blockedLn.Addr().(*net.TCPAddr).Port

	var mu sync.Mutex
	var dnsSeen [][]byte
	e := NewEngine(Hooks{
		Owner: func(proto int, srcIP string, srcPort int, dstIP string, dstPort int) (int, string, string) {
			if srcIP != tunHost {
				t.Errorf("owner asked for source %s", srcIP)
			}
			return 10042, "Test App", "com.example.test"
		},
		Block: func(app, pkg, ip string, port int) bool { return port == blockedPort },
		DNS: func(b []byte) {
			mu.Lock()
			dnsSeen = append(dnsSeen, b)
			mu.Unlock()
		},
	})
	// The stand-in destination is the host's loopback servers.
	var dials sync.Map
	e.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		dials.Store(network+" "+address, true)
		if host, port, _ := net.SplitHostPort(address); host == tunDest {
			address = net.JoinHostPort("127.0.0.1", port)
		}
		return netDial(ctx, network, address)
	}
	if err := e.Start(fd, 1500, []string{dnsAddr}); err != nil {
		unix.Close(fd)
		t.Fatal(err)
	}
	stopped := false
	defer func() {
		if !stopped {
			e.Stop()
		}
	}()
	if !e.Running() {
		t.Fatal("not running after Start")
	}
	if err := e.Start(fd, 1500, nil); !errors.Is(err, ErrRunning) {
		t.Fatalf("second Start: %v", err)
	}
	t.Logf("engine running on %s", name)

	// TCP: bytes flow both ways and are counted.
	c, err := net.DialTimeout("tcp", net.JoinHostPort(tunDest, fmt.Sprint(echoPort)), 5*time.Second)
	if err != nil {
		t.Fatalf("dial through the tunnel: %v", err)
	}
	payload := bytes.Repeat([]byte("middlelan "), 50_000) // 500 kB, many segments
	go func() {
		c.Write(payload)
		c.(*net.TCPConn).CloseWrite()
	}()
	c.SetReadDeadline(time.Now().Add(15 * time.Second))
	echoed, err := io.ReadAll(c)
	c.Close()
	if err != nil || !bytes.Equal(echoed, payload) {
		t.Fatalf("echo: got %d bytes (%v), want %d", len(echoed), err, len(payload))
	}
	tcpFlow := waitFlow(t, e, func(f Flow) bool {
		return f.Proto == "tcp" && f.RemotePort == echoPort && f.Closed
	})
	if tcpFlow.Sent != uint64(len(payload)) || tcpFlow.Recv != uint64(len(payload)) ||
		tcpFlow.UID != 10042 || tcpFlow.App != "Test App" || tcpFlow.Package != "com.example.test" ||
		tcpFlow.LocalIP != tunHost || tcpFlow.RemoteIP != tunDest || tcpFlow.Blocked {
		t.Fatalf("TCP flow %+v", tcpFlow)
	}

	// Blocked TCP: the app sees a reset, nothing is dialed.
	_, err = net.DialTimeout("tcp", net.JoinHostPort(tunDest, fmt.Sprint(blockedPort)), 5*time.Second)
	if !errors.Is(err, syscall.ECONNREFUSED) {
		t.Fatalf("blocked dial: %v, want connection refused", err)
	}
	if _, ok := dials.Load(fmt.Sprintf("tcp 10.111.0.10:%d", blockedPort)); ok {
		t.Fatal("blocked flow was dialed")
	}
	if f := waitFlow(t, e, func(f Flow) bool { return f.RemotePort == blockedPort }); !f.Blocked || !f.Closed {
		t.Fatalf("blocked flow %+v", f)
	}

	// UDP: datagrams are relayed both ways.
	u, err := net.Dial("udp", net.JoinHostPort(tunDest, fmt.Sprint(udpPort)))
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	buf := make([]byte, 2048)
	for i := range 3 {
		msg := fmt.Sprintf("datagram %d", i)
		u.Write([]byte(msg))
		u.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := u.Read(buf)
		if err != nil || string(buf[:n]) != msg {
			t.Fatalf("UDP echo %d: %q, %v", i, buf[:n], err)
		}
	}
	if f := waitFlow(t, e, func(f Flow) bool { return f.Proto == "udp" && f.RemotePort == udpPort }); f.Sent != 30 || f.Recv != 30 || f.Closed {
		t.Fatalf("UDP flow %+v", f)
	}

	// DNS: a query to the virtual resolver goes to the upstream server and
	// the hook sees the answer.
	d, err := net.Dial("udp", VirtualDNS+":53")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.Write(dnsQuery(0xcafe))
	d.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := d.Read(buf)
	if err != nil || n < 12 || buf[0] != 0xca || buf[1] != 0xfe || buf[2]&0x80 == 0 {
		t.Fatalf("DNS answer %x, %v", buf[:n], err)
	}
	if dnsQueries.Load() != 1 {
		t.Fatalf("upstream saw %d queries", dnsQueries.Load())
	}
	mu.Lock()
	if len(dnsSeen) != 1 || !bytes.Equal(dnsSeen[0], buf[:n]) {
		t.Fatalf("DNS hook saw %d responses", len(dnsSeen))
	}
	mu.Unlock()

	// ICMP: pings get no answer from the stack.
	if pc, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0"); err == nil {
		defer pc.Close()
		echo := icmp.Message{Type: ipv4.ICMPTypeEcho, Body: &icmp.Echo{ID: 0x4d4c, Seq: 1, Data: []byte("ping")}}
		b, _ := echo.Marshal(nil)
		pc.WriteTo(b, &net.IPAddr{IP: net.ParseIP(tunDest)})
		pc.SetReadDeadline(time.Now().Add(time.Second))
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				break // timeout: dropped, as intended
			}
			m, err := icmp.ParseMessage(1, buf[:n])
			if err == nil && m.Type == ipv4.ICMPTypeEchoReply && strings.HasPrefix(from.String(), tunDest) {
				t.Fatal("the stack answered a ping")
			}
		}
	} else {
		t.Logf("ICMP check skipped: %v", err)
	}

	if rx, tx := e.Totals(); rx < uint64(len(payload)) || tx < uint64(len(payload)) {
		t.Fatalf("Totals = %d, %d", rx, tx)
	}

	// Stop: everything closes, the device goes away.
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	stopped = true
	if e.Running() {
		t.Fatal("running after Stop")
	}
	for _, f := range e.Flows() {
		if !f.Closed {
			t.Fatalf("flow still open after Stop: %+v", f)
		}
	}
	if _, err := net.InterfaceByName(name); err == nil {
		t.Fatalf("%s still exists after Stop", name)
	}
	if err := e.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

// waitFlow waits for a flow matching ok to appear.
func waitFlow(t *testing.T, e *Engine, ok func(Flow) bool) Flow {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, f := range e.Flows() {
			if ok(f) {
				return f
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no matching flow in %+v", e.Flows())
	return Flow{}
}

func TestEngineStopWithOpenFlows(t *testing.T) {
	fd, _ := openTUN(t)
	echoPort := echoServer(t)
	udpPort := udpEcho(t)
	before := runtime.NumGoroutine()
	e := NewEngine(Hooks{})
	e.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		_, port, _ := net.SplitHostPort(address)
		return netDial(ctx, network, net.JoinHostPort("127.0.0.1", port))
	}
	if err := e.Start(fd, 0, nil); err != nil {
		unix.Close(fd)
		t.Fatal(err)
	}
	var conns []net.Conn
	for range 20 {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(tunDest, fmt.Sprint(echoPort)), 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	u, err := net.Dial("udp", net.JoinHostPort(tunDest, fmt.Sprint(udpPort)))
	if err != nil {
		t.Fatal(err)
	}
	conns = append(conns, u)
	u.Write([]byte("hello"))
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	waitFlow(t, e, func(f Flow) bool { return f.Proto == "udp" })
	if n := len(e.Flows()); n != 21 {
		t.Fatalf("%d flows, want 21", n)
	}
	start := time.Now()
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("Stop took %v", d)
	}
	for _, f := range e.Flows() {
		if !f.Closed {
			t.Fatalf("open after Stop: %+v", f)
		}
	}
	// The engine's goroutines are gone (the test's own servers remain).
	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before+2 {
		buf := make([]byte, 1<<20)
		t.Fatalf("%d goroutines after Stop, %d before\n%s", n, before, buf[:runtime.Stack(buf, true)])
	}
}

func TestEngineStopsWhenTUNCloses(t *testing.T) {
	fd, _ := openTUN(t)
	e := NewEngine(Hooks{})
	stoppedCh := make(chan struct{}, 4)
	e.onStop = func() { stoppedCh <- struct{}{} }
	if err := e.Start(fd, 0, nil); err != nil {
		unix.Close(fd)
		t.Fatal(err)
	}
	// The descriptor fails under the engine (as when the VPN is revoked).
	e.mu.Lock()
	st := e.cur.st
	e.mu.Unlock()
	unix.Close(fd)
	select {
	case <-stoppedCh:
	case <-time.After(5 * time.Second):
		t.Fatal("engine didn't stop")
	}
	if e.Running() {
		t.Fatal("still running")
	}
	// A stale stop for the old run does nothing to a new one.
	fd2, _ := openTUN(t)
	if err := e.Start(fd2, 0, nil); err != nil {
		unix.Close(fd2)
		t.Fatal(err)
	}
	if err := e.stopRun(st); err != nil || !e.Running() {
		t.Fatalf("stale stop affected the new run: %v", err)
	}
	e.Stop()
}
