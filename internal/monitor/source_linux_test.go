//go:build linux && !android

package monitor

import "testing"

func TestLinuxSample(t *testing.T) {
	s := NewSource()
	smp, err := s.Sample()
	if err != nil {
		t.Fatal(err)
	}
	if !smp.PerApp {
		t.Logf("limits: %s", s.Limits())
	}
	for _, c := range smp.Conns {
		t.Logf("%s %s:%d -> %s:%d %s pid=%d %s sent=%d recv=%d", c.Proto, c.LocalIP, c.LocalPort, c.RemoteIP, c.RemotePort, c.State, c.PID, c.Process, c.Sent, c.Recv)
	}
}

func TestParseHexAddr(t *testing.T) {
	ip, port, err := parseHexAddr("0100007F:0035")
	if err != nil || ip != "127.0.0.1" || port != 53 {
		t.Fatalf("got %s %d %v", ip, port, err)
	}
}
