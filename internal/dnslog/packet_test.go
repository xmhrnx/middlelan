package dnslog

import (
	"encoding/binary"
	"testing"
	"time"

	"golang.org/x/net/bpf"
	"golang.org/x/net/dns/dnsmessage"
)

// ipv4UDP wraps payload in IPv4 and UDP headers.
func ipv4UDP(src, dst uint16, payload []byte) []byte {
	p := make([]byte, 28+len(payload))
	p[0] = 0x45
	binary.BigEndian.PutUint16(p[2:], uint16(len(p)))
	p[8] = 64
	p[9] = protoUDP
	copy(p[12:], []byte{192, 0, 2, 53})
	copy(p[16:], []byte{192, 0, 2, 10})
	binary.BigEndian.PutUint16(p[20:], src)
	binary.BigEndian.PutUint16(p[22:], dst)
	binary.BigEndian.PutUint16(p[24:], uint16(8+len(payload)))
	copy(p[28:], payload)
	return p
}

// ipv6UDP wraps payload in IPv6 and UDP headers.
func ipv6UDP(src, dst uint16, payload []byte) []byte {
	p := make([]byte, 48+len(payload))
	p[0] = 0x60
	binary.BigEndian.PutUint16(p[4:], uint16(8+len(payload)))
	p[6] = protoUDP
	p[7] = 64
	binary.BigEndian.PutUint16(p[40:], src)
	binary.BigEndian.PutUint16(p[42:], dst)
	binary.BigEndian.PutUint16(p[44:], uint16(8+len(payload)))
	copy(p[48:], payload)
	return p
}

func TestDNSPayload(t *testing.T) {
	body := []byte("dns!")
	padded := append(ipv4UDP(53, 4000, body), 0, 0, 0, 0) // Ethernet padding
	for name, pkt := range map[string][]byte{"v4": ipv4UDP(53, 4000, body), "v4 padded": padded, "v6": ipv6UDP(53, 4000, body)} {
		got, ok := dnsPayload(pkt)
		if !ok || string(got) != "dns!" {
			t.Errorf("%s: payload %q %v", name, got, ok)
		}
	}
	frag := ipv4UDP(53, 4000, body)
	frag[7] = 1 // fragment offset 8
	tcp := ipv4UDP(53, 4000, body)
	tcp[9] = 6
	for name, pkt := range map[string][]byte{
		"query":    ipv4UDP(4000, 53, body),
		"fragment": frag,
		"tcp":      tcp,
		"v6 query": ipv6UDP(4000, 53, body),
		"empty":    nil,
		"short":    {0x45, 0, 0},
		"bad ihl":  {0x41, 0, 0, 28, 0, 0, 0, 0, 64, 17, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 53, 0, 1, 0, 8, 0, 0},
	} {
		if got, ok := dnsPayload(pkt); ok {
			t.Errorf("%s: accepted, payload %q", name, got)
		}
	}
	for n := 0; n < len(padded); n++ {
		dnsPayload(padded[:n])
	}
}

func TestFilter(t *testing.T) {
	ether := func(etherType uint16, ip []byte) []byte {
		f := make([]byte, 14, 14+len(ip))
		binary.BigEndian.PutUint16(f[12:], etherType)
		return append(f, ip...)
	}
	null := func(ip []byte) []byte { return append([]byte{2, 0, 0, 0}, ip...) }
	body := []byte("response")
	frag := ipv4UDP(53, 9, body)
	frag[7] = 1
	opts := ipv4UDP(53, 9, body)
	opts = append(opts[:20], append(make([]byte, 4), opts[20:]...)...)
	opts[0] = 0x46 // IPv4 options shift the UDP header
	cases := []struct {
		name string
		o    filterOptions
		pkt  []byte
		want bool
	}{
		{"raw v4 response", filterOptions{}, ipv4UDP(53, 9, body), true},
		{"raw v4 with options", filterOptions{}, opts, true},
		{"raw v4 query", filterOptions{}, ipv4UDP(9, 53, body), false},
		{"raw v4 fragment", filterOptions{}, frag, false},
		{"raw v6 response", filterOptions{}, ipv6UDP(53, 9, body), true},
		{"raw v6 query", filterOptions{}, ipv6UDP(9, 53, body), false},
		{"raw garbage", filterOptions{}, []byte{0x12, 0x34}, false},
		{"ether v4", filterOptions{offset: 14, ethernet: true}, ether(0x0800, ipv4UDP(53, 9, body)), true},
		{"ether v6", filterOptions{offset: 14, ethernet: true}, ether(0x86dd, ipv6UDP(53, 9, body)), true},
		{"ether arp", filterOptions{offset: 14, ethernet: true}, ether(0x0806, ipv4UDP(53, 9, body)), false},
		{"null v4", filterOptions{offset: 4}, null(ipv4UDP(53, 9, body)), true},
		{"null v4 query", filterOptions{offset: 4}, null(ipv4UDP(9, 53, body)), false},
	}
	for _, c := range cases {
		raw, err := dnsFilter(c.o)
		if err != nil {
			t.Fatal(err)
		}
		prog, ok := bpf.Disassemble(raw)
		if !ok {
			t.Fatalf("%s: filter does not disassemble", c.name)
		}
		vm, err := bpf.NewVM(prog)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		n, err := vm.Run(c.pkt)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := n > 0; got != c.want {
			t.Errorf("%s: accepted = %v, want %v", c.name, got, c.want)
		}
	}
	// The Linux variant only adds an ancillary load the VM can't run.
	if _, err := dnsFilter(filterOptions{skipOutgoing: true}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleBPF(t *testing.T) {
	msg := buildResponse(t, "www.example.", dnsmessage.TypeA, [2]any{"www.example.", a4("192.0.2.80")})
	msg6 := buildResponse(t, "six.example.", dnsmessage.TypeAAAA, [2]any{"six.example.", a6("2001:db8::80")})
	frame := func(ip []byte) []byte { return append(make([]byte, 14), ip...) }
	var buf []byte
	for _, pkt := range [][]byte{frame(ipv4UDP(53, 9, msg)), frame(ipv6UDP(53, 9, msg6))} {
		hdr := make([]byte, 18)
		binary.NativeEndian.PutUint32(hdr[8:], uint32(len(pkt)))
		binary.NativeEndian.PutUint32(hdr[12:], uint32(len(pkt)))
		binary.NativeEndian.PutUint16(hdr[16:], 18)
		buf = append(buf, hdr...)
		buf = append(buf, pkt...)
		for len(buf)%4 != 0 {
			buf = append(buf, 0)
		}
	}
	l := New()
	l.handleBPF(buf, 14, time.Now())
	if name, ok := l.Lookup("192.0.2.80"); !ok || name != "www.example" {
		t.Errorf("Lookup v4 = %q %v", name, ok)
	}
	if name, ok := l.Lookup("2001:db8::80"); !ok || name != "six.example" {
		t.Errorf("Lookup v6 = %q %v", name, ok)
	}
	for n := 0; n < len(buf); n++ {
		l.handleBPF(buf[:n], 14, time.Now())
	}
}
