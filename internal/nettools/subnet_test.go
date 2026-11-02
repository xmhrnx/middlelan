package nettools

import "testing"

func TestCalcSubnetIPv4(t *testing.T) {
	s, err := CalcSubnet("192.168.1.10/24")
	if err != nil {
		t.Fatal(err)
	}
	want := SubnetInfo{CIDR: "192.168.1.0/24", Network: "192.168.1.0", Broadcast: "192.168.1.255",
		Netmask: "255.255.255.0", Wildcard: "0.0.0.255", FirstHost: "192.168.1.1", LastHost: "192.168.1.254",
		Class: "C", Hosts: 254, Prefix: 24, Private: true}
	if s != want {
		t.Errorf("got %+v\nwant %+v", s, want)
	}

	s, err = CalcSubnet("10.0.0.0 255.0.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if s.Prefix != 8 || s.Hosts != 16777214 || s.Class != "A" || s.Broadcast != "10.255.255.255" || !s.Private {
		t.Errorf("10/8: %+v", s)
	}

	s, err = CalcSubnet("8.8.8.8/31")
	if err != nil {
		t.Fatal(err)
	}
	if s.Hosts != 2 || s.FirstHost != "8.8.8.8" || s.LastHost != "8.8.8.9" || s.Private {
		t.Errorf("/31: %+v", s)
	}

	s, err = CalcSubnet("1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if s.Prefix != 32 || s.Hosts != 1 {
		t.Errorf("/32: %+v", s)
	}
}

func TestCalcSubnetIPv6(t *testing.T) {
	s, err := CalcSubnet("2001:db8::1/64")
	if err != nil {
		t.Fatal(err)
	}
	if !s.IPv6 || s.CIDR != "2001:db8::/64" || s.Network != "2001:db8::" || s.LastHost != "2001:db8::ffff:ffff:ffff:ffff" || s.Hosts != ^uint64(0) || s.Broadcast != "" {
		t.Errorf("v6 /64: %+v", s)
	}
	s, err = CalcSubnet("fd00::/120")
	if err != nil {
		t.Fatal(err)
	}
	if s.Hosts != 255 || !s.Private || s.FirstHost != "fd00::1" {
		t.Errorf("v6 /120: %+v", s)
	}
}

func TestCalcSubnetErrors(t *testing.T) {
	for _, in := range []string{"", "300.1.1.1/24", "10.0.0.0/33", "10.0.0.0 255.0.255.0", "fe80::/129", "a b c"} {
		if _, err := CalcSubnet(in); err == nil {
			t.Errorf("CalcSubnet(%q) expected error", in)
		}
	}
}
