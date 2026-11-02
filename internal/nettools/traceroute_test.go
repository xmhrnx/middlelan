package nettools

import "testing"

func TestParseTracerouteOutput(t *testing.T) {
	tests := []struct {
		name, out, from string
		exceeded, reply bool
	}{
		{"linux", "PING 8.8.8.8 (8.8.8.8) 56(84) bytes of data.\nFrom 192.168.1.1 icmp_seq=1 Time to live exceeded\n", "192.168.1.1", true, false},
		{"linux-names", "From router.lan (192.168.1.1) icmp_seq=1 Time to live exceeded\n", "192.168.1.1", true, false},
		{"darwin", "PING 8.8.8.8 (8.8.8.8): 56 data bytes\n92 bytes from 10.0.0.1: Time to live exceeded\nVr HL TOS  Len   ID Flg  off TTL Pro  cks      Src      Dst\n 4  5  00 5400 1c3d   0 0000  01  01 6b2a 192.168.1.20  8.8.8.8\n", "10.0.0.1", true, false},
		{"windows", "\r\nPinging 8.8.8.8 with 32 bytes of data:\r\nReply from 172.16.0.1: TTL expired in transit.\r\n", "172.16.0.1", true, false},
		{"windows-de", "Antwort von 172.16.0.1: TTL abgelaufen bei Übertragung.\r\n", "172.16.0.1", true, false},
		{"linux-v6", "From 2001:db8::1 icmp_seq=1 Time exceeded: Hop limit\n", "2001:db8::1", true, false},
		{"reached", "64 bytes from 8.8.8.8: icmp_seq=1 ttl=117 time=11.2 ms\n", "8.8.8.8", false, true},
		{"timeout", "Request timed out.\r\n", "", false, false},
	}
	for _, tc := range tests {
		r := parsePingOutput(tc.out)
		if r.From != tc.from || r.TTLExceeded != tc.exceeded || r.Reply != tc.reply {
			t.Errorf("%s: got %+v", tc.name, r)
		}
	}
}
