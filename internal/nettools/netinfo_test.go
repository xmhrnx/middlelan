package nettools

import (
	"reflect"
	"testing"
)

func TestNetinfoParsers(t *testing.T) {
	route := "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n" +
		"eth0\t0001A8C0\t00000000\t0001\t0\t0\t0\t00FFFFFF\t0\t0\t0\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t0\t00000000\t0\t0\t0\n"
	if got := parseProcNetRoute(route); got != "192.168.1.1" {
		t.Errorf("proc route: %q", got)
	}
	if got := parseIPRoute("8.8.8.8 via 10.0.0.1 dev wlan0 table 1021 src 10.0.0.5 uid 10123\n"); got != "10.0.0.1" {
		t.Errorf("ip route get: %q", got)
	}
	if got := parseKeyValueIP("   route to: default\ndestination: default\n    gateway: 192.168.0.1\n  interface: en0\n", "gateway"); got != "192.168.0.1" {
		t.Errorf("route get: %q", got)
	}
	if got := parseResolvConf("# c\nnameserver 1.1.1.1\nnameserver fe80::1%eth0\nsearch lan\nnameserver 1.1.1.1\n"); !reflect.DeepEqual(got, []string{"1.1.1.1", "fe80::1%eth0"}) {
		t.Errorf("resolv: %v", got)
	}
	if got := parseScutilDNS("resolver #1\n  nameserver[0] : 192.168.0.1\n  nameserver[1] : 8.8.8.8\nresolver #2\n  nameserver[0] : 192.168.0.1\n"); !reflect.DeepEqual(got, []string{"192.168.0.1", "8.8.8.8"}) {
		t.Errorf("scutil: %v", got)
	}
	rp := "IPv4 Route Table\r\nActive Routes:\r\nNetwork Destination        Netmask          Gateway       Interface  Metric\r\n          0.0.0.0          0.0.0.0      192.168.1.1     192.168.1.20     35\r\n          0.0.0.0          0.0.0.0      10.8.0.1     10.8.0.2     5\r\n"
	if got := parseRoutePrint(rp); got != "10.8.0.1" {
		t.Errorf("route print: %q", got)
	}
}

func TestParseNeighborTable(t *testing.T) {
	proc := "IP address       HW type     Flags       HW address            Mask     Device\n" +
		"192.168.1.1      0x1         0x2         aa:bb:cc:dd:ee:ff     *        eth0\n" +
		"192.168.1.50     0x1         0x0         00:00:00:00:00:00     *        eth0\n"
	win := "Interface: 192.168.1.20 --- 0xb\r\n  Internet Address      Physical Address      Type\r\n  192.168.1.1           AA-BB-CC-DD-EE-01     dynamic\r\n  192.168.1.255         ff-ff-ff-ff-ff-ff     static\r\n  224.0.0.22            01-00-5e-00-00-16     static\r\n"
	mac := "? (192.168.1.7) at a:b:c:d:e:f on en0 ifscope [ethernet]\n? (192.168.1.9) at (incomplete) on en0 ifscope [ethernet]\n"
	if got := parseNeighborTable(proc); !reflect.DeepEqual(got, map[string]string{"192.168.1.1": "aa:bb:cc:dd:ee:ff"}) {
		t.Errorf("proc: %v", got)
	}
	if got := parseNeighborTable(win); !reflect.DeepEqual(got, map[string]string{"192.168.1.1": "aa:bb:cc:dd:ee:01"}) {
		t.Errorf("windows: %v", got)
	}
	if got := parseNeighborTable(mac); !reflect.DeepEqual(got, map[string]string{"192.168.1.7": "0a:0b:0c:0d:0e:0f"}) {
		t.Errorf("darwin: %v", got)
	}
}

func TestNetbiosRoundTrip(t *testing.T) {
	req := netbiosStatusRequest(0x1234)
	if len(req) != 50 {
		t.Fatalf("request length %d", len(req))
	}
	// Build a minimal response: header, compressed name pointer, RR fields,
	// two names (a group name first, then the unique workstation name).
	resp := []byte{0x12, 0x34, 0x84, 0x00, 0, 0, 0, 1, 0, 0, 0, 0, 0xc0, 0x0c, 0, 0x21, 0, 1, 0, 0, 0, 0, 0, 0, 2}
	entry := func(name string, suffix byte, flags uint16) []byte {
		b := []byte(name)
		for len(b) < 15 {
			b = append(b, ' ')
		}
		return append(b, suffix, byte(flags>>8), byte(flags))
	}
	resp = append(resp, entry("WORKGROUP", 0, 0x8400)...)
	resp = append(resp, entry("DESKTOP-42", 0, 0x0400)...)
	got, err := parseNetbiosStatus(resp, 0x1234)
	if err != nil || got != "DESKTOP-42" {
		t.Errorf("got %q, %v", got, err)
	}
}
