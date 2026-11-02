package wifi

import (
	"reflect"
	"testing"
)

func TestParseIWDev(t *testing.T) {
	if got := parseIWDev(fixture(t, "iw_dev.txt")); !reflect.DeepEqual(got, []string{"wlp2s0"}) {
		t.Errorf("got %q", got)
	}
}

func TestParseIWLink(t *testing.T) {
	l, ok := parseIWLink(fixture(t, "iw_link.txt"))
	want := Link{
		Interface: "wlp2s0", SSID: "Home’s Net", BSSID: "aa:bb:cc:dd:ee:02",
		SignalDBm: -52, Quality: 96, Channel: 36, FrequencyMHz: 5180, Band: "5 GHz",
		TxRateMbps: 650, RxRateMbps: 866.7, Security: SecUnknown, Connected: true,
	}
	if !ok || !reflect.DeepEqual(l, want) {
		t.Errorf("got %+v\nwant %+v", l, want)
	}
	if _, ok := parseIWLink(fixture(t, "iw_link_notconnected.txt")); ok {
		t.Error("Not connected parsed as connected")
	}
}

func TestParseIWScan(t *testing.T) {
	nets := parseIWScan(fixture(t, "iw_scan_dump.txt"))
	type row struct {
		bssid, ssid string
		sec         Security
		ch, dbm     int
		conn        bool
	}
	want := []row{
		{"aa:bb:cc:dd:ee:02", "HomeNet", SecWPA2WPA3, 36, -52, true},
		{"aa:bb:cc:dd:ee:03", "Neighbour", SecWPA, 1, -71, false},
		{"aa:bb:cc:dd:ee:05", "Corp", SecEnterprise, 149, -64, false},
		{"aa:bb:cc:dd:ee:06", "OldRouter", SecWEP, 11, -80, false},
		{"aa:bb:cc:dd:ee:01", "Cafe Guest", SecOpen, 6, -60, false},
		{"aa:bb:cc:dd:ee:04", "Office", SecWPA3, 1, -75, false},
	}
	if len(nets) != len(want) {
		t.Fatalf("got %d networks: %+v", len(nets), nets)
	}
	finishNetworks(nets) // sorts by signal; compare by BSSID instead
	byBSSID := map[string]Network{}
	for _, n := range nets {
		byBSSID[n.BSSID] = n
	}
	for _, w := range want {
		n := byBSSID[w.bssid]
		if n.SSID != w.ssid || n.Security != w.sec || n.Channel != w.ch || n.SignalDBm != w.dbm || n.Connected != w.conn {
			t.Errorf("%s: got %+v, want %+v", w.bssid, n, w)
		}
	}
	if byBSSID["aa:bb:cc:dd:ee:04"].Band != "6 GHz" {
		t.Errorf("6 GHz band: %+v", byBSSID["aa:bb:cc:dd:ee:04"])
	}
}

func TestParseProcWireless(t *testing.T) {
	out := fixture(t, "proc_net_wireless.txt")
	if got := parseProcWireless(out, "wlp2s0"); got != -52 {
		t.Errorf("wlp2s0 = %d", got)
	}
	if got := parseProcWireless(out, "wlan1"); got != -70 {
		t.Errorf("wlan1 (unsigned) = %d", got)
	}
	if got := parseProcWireless(out, "eth0"); got != 0 {
		t.Errorf("eth0 = %d", got)
	}
}
