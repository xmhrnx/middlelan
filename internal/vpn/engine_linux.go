//go:build linux

package vpn

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"

	"golang.org/x/sys/unix"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/fdbased"
	"gvisor.dev/gvisor/pkg/tcpip/link/nested"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"gvisor.dev/gvisor/pkg/waiter"
)

const nicID tcpip.NICID = 1

// stackState is the netstack of one run.
type stackState struct {
	fd    int
	stack *stack.Stack
}

// Start runs the engine on the TUN descriptor fd (raw IP packets, no
// packet-information header) until Stop. It returns once the stack is up.
// The engine owns fd from then on and closes it in Stop; when Start fails
// the caller still owns it. dns are the upstream servers for the virtual
// resolver ("ip" or "ip:port"); mtu 0 means 1500.
func (e *Engine) Start(fd int, mtu int, dns []string) error {
	if fd < 0 {
		return fmt.Errorf("vpn: bad descriptor %d", fd)
	}
	if mtu <= 0 {
		mtu = 1500
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		return ErrRunning
	}
	e.SetDNS(dns)

	st := &stackState{fd: fd}
	ep, err := fdbased.New(&fdbased.Options{
		FDs:               []int{fd},
		MTU:               uint32(mtu),
		RXChecksumOffload: true, // the kernel built these packets
		ClosedFunc: func(tcpip.Error) {
			// The descriptor failed or was closed (the VPN was revoked):
			// stop, unless this run is already stopping.
			go e.stopRun(st)
		},
	})
	if err != nil {
		return fmt.Errorf("vpn: %v", err)
	}
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
	})
	st.stack = s
	nicUp := false
	fail := func(what string, terr tcpip.Error) error {
		if !nicUp {
			ep.Close()
		}
		s.Destroy()
		return fmt.Errorf("vpn: %s: %s", what, terr)
	}
	sack := tcpip.TCPSACKEnabled(true)
	if terr := s.SetTransportProtocolOption(tcp.ProtocolNumber, &sack); terr != nil {
		return fail("tcp sack", terr)
	}
	rcv := tcpip.TCPReceiveBufferSizeRangeOption{Min: tcp.MinBufferSize, Default: 128 << 10, Max: 2 << 20}
	if terr := s.SetTransportProtocolOption(tcp.ProtocolNumber, &rcv); terr != nil {
		return fail("tcp receive buffer", terr)
	}
	snd := tcpip.TCPSendBufferSizeRangeOption{Min: tcp.MinBufferSize, Default: 128 << 10, Max: 2 << 20}
	if terr := s.SetTransportProtocolOption(tcp.ProtocolNumber, &snd); terr != nil {
		return fail("tcp send buffer", terr)
	}
	// The handlers are in place before packets flow; they wait on e.mu
	// until Start returns.
	r := newRun(st)
	tcpFwd := tcp.NewForwarder(s, 0, 512, func(req *tcp.ForwarderRequest) { e.onTCP(r, req) })
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, tcpFwd.HandlePacket)
	udpFwd := udp.NewForwarder(s, func(req *udp.ForwarderRequest) bool { return e.onUDP(r, req) })
	s.SetTransportProtocolHandler(udp.ProtocolNumber, udpFwd.HandlePacket)

	if terr := s.CreateNIC(nicID, newICMPDrop(ep)); terr != nil {
		return fail("create NIC", terr)
	}
	nicUp = true
	// Accept packets to any address and answer from any address: the
	// stack stands in for every destination.
	if terr := s.SetPromiscuousMode(nicID, true); terr != nil {
		return fail("promiscuous mode", terr)
	}
	if terr := s.SetSpoofing(nicID, true); terr != nil {
		return fail("spoofing", terr)
	}
	s.SetRouteTable([]tcpip.Route{
		{Destination: header.IPv4EmptySubnet, NIC: nicID},
		{Destination: header.IPv6EmptySubnet, NIC: nicID},
	})
	e.beginLocked(r)
	return nil
}

// Stop ends the current run: it closes every flow, the stack and the TUN
// descriptor. Stopping a stopped engine does nothing.
func (e *Engine) Stop() error {
	e.mu.Lock()
	var st *stackState
	if e.cur != nil {
		st = e.cur.st
	}
	e.mu.Unlock()
	if st == nil {
		return nil
	}
	return e.stopRun(st)
}

// stopRun stops the run using st, if it is still the current one.
func (e *Engine) stopRun(st *stackState) error {
	e.mu.Lock()
	if e.cur == nil || e.cur.st != st {
		e.mu.Unlock()
		return nil
	}
	r, closers := e.endLocked()
	e.mu.Unlock()

	for _, c := range closers {
		c.Close()
	}
	// Abort the stack's endpoints, then remove the NIC, which closes the
	// link endpoint and waits for its dispatcher.
	st.stack.Destroy()
	err := unix.Close(st.fd)
	r.waitFlows()
	if e.onStop != nil {
		go e.onStop()
	}
	if err != nil {
		return fmt.Errorf("vpn: close tun: %w", err)
	}
	return nil
}

func endpointAddrs(id stack.TransportEndpointID) (src, dst netip.AddrPort) {
	sa, _ := netip.AddrFromSlice(id.RemoteAddress.AsSlice())
	da, _ := netip.AddrFromSlice(id.LocalAddress.AsSlice())
	return netip.AddrPortFrom(sa, id.RemotePort), netip.AddrPortFrom(da, id.LocalPort)
}

// target returns the address to dial for an app's destination: the real
// upstream resolver for the virtual one.
func (e *Engine) target(dst netip.AddrPort) (string, bool) {
	if dst.Addr() == virtualDNS {
		if dst.Port() != 53 {
			return "", false
		}
		servers := *e.dns.Load()
		if len(servers) == 0 {
			return "", false
		}
		return servers[0], true
	}
	return net.JoinHostPort(dst.Addr().String(), strconv.Itoa(int(dst.Port()))), true
}

// onTCP handles a new TCP connection from an app (in its own goroutine):
// it connects to the real destination first and only then completes the
// app's handshake, so failures and blocks reach the app as resets.
func (e *Engine) onTCP(r *run, req *tcp.ForwarderRequest) {
	src, dst := endpointAddrs(req.ID())
	f, err := e.newFlow(false, src, dst)
	if err != nil {
		req.Complete(true)
		return
	}
	defer e.endFlow(f)
	if e.decide(f) {
		req.Complete(true)
		return
	}
	addr, ok := e.target(f.dst)
	if !ok {
		req.Complete(true)
		return
	}
	remote, err := e.dialWithTimeout(r.ctx, "tcp", addr)
	if err != nil {
		req.Complete(true)
		return
	}
	if !e.addCloser(f, remote) {
		req.Complete(true)
		return
	}
	var wq waiter.Queue
	ep, terr := req.CreateEndpoint(&wq)
	if terr != nil {
		req.Complete(true)
		return
	}
	req.Complete(false)
	app := gonet.NewTCPConn(&wq, ep)
	if !e.addCloser(f, app) {
		return
	}
	e.spliceTCP(f, app, remote)
}

// onUDP handles the first datagram of a new UDP association. It runs on
// the stack's packet path, so it only creates the endpoint and leaves
// the owner lookup, the block decision and dialing to a goroutine.
func (e *Engine) onUDP(r *run, req *udp.ForwarderRequest) bool {
	src, dst := endpointAddrs(req.ID())
	f, err := e.newFlow(true, src, dst)
	if err != nil {
		return false
	}
	var wq waiter.Queue
	ep, terr := req.CreateEndpoint(&wq)
	if terr != nil {
		e.endFlow(f)
		return false
	}
	app := gonet.NewUDPConn(&wq, ep)
	if !e.addCloser(f, app) {
		e.endFlow(f)
		return true
	}
	go e.handleUDP(r, f, app)
	return true
}

func (e *Engine) handleUDP(r *run, f *flow, app net.Conn) {
	defer e.endFlow(f)
	idle := udpIdle
	if f.RemotePort == 53 {
		idle = dnsIdle
	}
	if e.decide(f) {
		e.drainUDP(f, app, idle)
		return
	}
	if f.dst.Addr() == virtualDNS {
		if f.dst.Port() == 53 {
			e.serveDNS(f, app)
		} else {
			e.drainUDP(f, app, idle)
		}
		return
	}
	if f.dst.Port() == 53 && e.hooks.Load().Filter != nil {
		// Plain DNS to a server of the app's choosing goes through the
		// filter too, answered by the upstream servers.
		e.serveDNS(f, app)
		return
	}
	addr, _ := e.target(f.dst)
	remote, err := e.dialWithTimeout(r.ctx, "udp", addr)
	if err != nil {
		e.drainUDP(f, app, idle)
		return
	}
	if !e.addCloser(f, remote) {
		return
	}
	e.relayUDP(f, app, remote, idle)
}

// icmpDrop is a link endpoint that drops ICMP from apps: the stack would
// otherwise answer every ping itself, making any host look reachable.
type icmpDrop struct {
	nested.Endpoint
}

func newICMPDrop(child stack.LinkEndpoint) *icmpDrop {
	e := &icmpDrop{}
	e.Endpoint.Init(child, e)
	return e
}

// DeliverNetworkPacket implements stack.NetworkDispatcher.
func (e *icmpDrop) DeliverNetworkPacket(proto tcpip.NetworkProtocolNumber, pkt *stack.PacketBuffer) {
	n := header.IPv4MinimumSize
	if proto == header.IPv6ProtocolNumber {
		n = header.IPv6MinimumSize
	}
	if hdr, ok := pkt.Data().PullUp(n); ok && isICMP(hdr) {
		return
	}
	e.Endpoint.DeliverNetworkPacket(proto, pkt)
}

// isICMP reports whether the IP packet starting with hdr carries ICMP or
// ICMPv6 (directly after the fixed IPv6 header).
func isICMP(hdr []byte) bool {
	if len(hdr) < 1 {
		return false
	}
	switch hdr[0] >> 4 {
	case 4:
		return len(hdr) >= header.IPv4MinimumSize && hdr[9] == uint8(header.ICMPv4ProtocolNumber)
	case 6:
		return len(hdr) >= header.IPv6MinimumSize && hdr[6] == uint8(header.ICMPv6ProtocolNumber)
	}
	return false
}
