//go:build darwin && !ios

package monitor

import "testing"

func TestParseLsof(t *testing.T) {
	out := "p123\ncSafari\nf12\ntIPv4\nPTCP\nn10.0.0.2:50000->17.1.2.3:443\nTST=ESTABLISHED\n"
	c := parseLsof([]byte(out))
	if len(c) != 1 || c[0].RemoteIP != "17.1.2.3" || c[0].RemotePort != 443 || c[0].Proto != "tcp" || c[0].State != "ESTABLISHED" {
		t.Fatalf("%+v", c)
	}
}
