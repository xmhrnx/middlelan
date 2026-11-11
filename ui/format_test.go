package ui

import "testing"

func TestFormat(t *testing.T) {
	cases := map[string]string{
		formatBytes(0): "0 B", formatBytes(999): "999 B", formatBytes(1500): "1.5 KB",
		formatBytes(2_500_000_000): "2.5 GB", formatRate(125_000): "1.0 Mbps", formatRate(0): "0 bps",
		endpointLabel("::1", 80): "[::1]:80", endpointLabel("1.2.3.4", 443): "1.2.3.4:443",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	}
}

func TestFormatCount(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 1234567: "1,234,567", -4500: "-4,500"} {
		if got := formatCount(n); got != want {
			t.Errorf("%d: %q, want %q", n, got, want)
		}
	}
}
