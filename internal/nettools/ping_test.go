package nettools

import (
	"net"
	"testing"
	"time"
)

func TestParsePingOutputReplies(t *testing.T) {
	tests := []struct {
		name, out, from string
		ttl             int
		rtt             time.Duration
	}{
		{"linux", "PING 1.1.1.1 (1.1.1.1) 56(84) bytes of data.\n64 bytes from 1.1.1.1: icmp_seq=1 ttl=57 time=12.3 ms\n\n--- 1.1.1.1 ping statistics ---\n", "1.1.1.1", 57, 12300 * time.Microsecond},
		{"darwin", "PING 1.1.1.1 (1.1.1.1): 56 data bytes\n64 bytes from 1.1.1.1: icmp_seq=0 ttl=57 time=9.871 ms\n", "1.1.1.1", 57, 9871 * time.Microsecond},
		{"windows", "\r\nPinging 1.1.1.1 with 32 bytes of data:\r\nReply from 1.1.1.1: bytes=32 time=14ms TTL=57\r\n", "1.1.1.1", 57, 14 * time.Millisecond},
		{"windows-lt", "Reply from 192.168.1.1: bytes=32 time<1ms TTL=64\r\n", "192.168.1.1", 64, time.Millisecond},
		{"windows-de", "Antwort von 1.1.1.1: Bytes=32 Zeit=15ms TTL=57\r\n", "1.1.1.1", 57, 15 * time.Millisecond},
		{"linux-v6", "64 bytes from 2606:4700:4700::1111: icmp_seq=1 ttl=58 time=3.21 ms\n", "2606:4700:4700::1111", 58, 3210 * time.Microsecond},
	}
	for _, tc := range tests {
		r := parsePingOutput(tc.out)
		if !r.Reply || r.TTLExceeded || r.From != tc.from || r.TTL != tc.ttl || r.RTT != tc.rtt {
			t.Errorf("%s: got %+v", tc.name, r)
		}
	}
}

func TestParsePingOutputNoReply(t *testing.T) {
	for _, out := range []string{
		"Pinging 10.9.9.9 with 32 bytes of data:\r\nRequest timed out.\r\n",
		"Reply from 192.168.1.10: Destination host unreachable.\r\n",
		"PING 10.9.9.9 (10.9.9.9) 56(84) bytes of data.\n\n--- 10.9.9.9 ping statistics ---\n1 packets transmitted, 0 received, 100% packet loss, time 0ms\n",
	} {
		if r := parsePingOutput(out); r.Reply || r.TTLExceeded {
			t.Errorf("unexpected reply for %q: %+v", out, r)
		}
	}
}

func TestFinishStats(t *testing.T) {
	s := finishStats(PingStats{Sent: 4, Received: 3}, []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 15 * time.Millisecond})
	if s.Min != 10*time.Millisecond || s.Max != 20*time.Millisecond || s.Avg != 15*time.Millisecond {
		t.Errorf("min/avg/max: %+v", s)
	}
	if s.Jitter != 7500*time.Microsecond {
		t.Errorf("jitter %v", s.Jitter)
	}
	if s.Loss != 0.25 {
		t.Errorf("loss %v", s.Loss)
	}
}

func TestSystemPingCommand(t *testing.T) {
	ip := net.ParseIP("1.1.1.1")
	name, args := systemPingCommand("linux", ip, 5, time.Second)
	if name != "ping" || !contains(args, "-t") || args[len(args)-1] != "1.1.1.1" {
		t.Errorf("linux: %s %v", name, args)
	}
	name, args = systemPingCommand("darwin", ip, 5, time.Second)
	if name != "ping" || !contains(args, "-m") {
		t.Errorf("darwin: %s %v", name, args)
	}
	name, args = systemPingCommand("windows", ip, 5, time.Second)
	if name != "ping" || !contains(args, "-i") || !contains(args, "-n") {
		t.Errorf("windows: %s %v", name, args)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
