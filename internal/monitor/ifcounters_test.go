package monitor

import "testing"

func TestIfaceKind(t *testing.T) {
	for name, want := range map[string]string{
		"en0": ifaceWiFi, "pdp_ip0": ifaceCellular, "pdp_ip3": ifaceCellular, "en2": ifaceWired,
		"lo0": "", "utun3": "", "awdl0": "", "llw0": "", "bridge100": "", "ap1": "",
	} {
		if got := ifaceKind(name); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
}

func TestCounter32(t *testing.T) {
	var c counter32
	if got := c.update(4_000_000_000); got != 4_000_000_000 {
		t.Fatal(got)
	}
	if got := c.update(4_294_000_000); got != 4_294_000_000 {
		t.Fatal(got)
	}
	// Wrapped past 2^32: 967,296 + 1,000,000 more bytes.
	if got := c.update(1_000_000); got != 4_294_000_000+967_296+1_000_000 {
		t.Fatalf("after wrap: %d", got)
	}
}
