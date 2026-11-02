package wifi

import (
	"reflect"
	"testing"
)

func TestParseSystemProfiler(t *testing.T) {
	l, ok, nets, redacted, err := parseSystemProfiler([]byte(fixture(t, "system_profiler.json")))
	if err != nil || !ok || redacted {
		t.Fatalf("err %v ok %v redacted %v", err, ok, redacted)
	}
	want := Link{
		Interface: "en0", SSID: "HomeNet", SignalDBm: -52, Quality: 96, Channel: 149,
		FrequencyMHz: 5745, Band: "5 GHz", TxRateMbps: 867, Security: SecWPA2WPA3, Connected: true,
	}
	if !reflect.DeepEqual(l, want) {
		t.Errorf("link %+v\nwant %+v", l, want)
	}
	if len(nets) != 4 {
		t.Fatalf("got %d networks", len(nets))
	}
	if !nets[0].Connected || nets[0].SSID != "HomeNet" {
		t.Errorf("current network = %+v", nets[0])
	}
	if n := nets[2]; n.SSID != "Office6E" || n.Band != "6 GHz" || n.Channel != 37 || n.FrequencyMHz != 6135 || n.Security != SecWPA3 {
		t.Errorf("6 GHz network = %+v", n)
	}
	if n := nets[3]; n.Security != SecOpen || n.SignalDBm != 0 || n.Channel != 11 {
		t.Errorf("open network = %+v", n)
	}
}

func TestParseSystemProfilerRedacted(t *testing.T) {
	l, ok, nets, redacted, err := parseSystemProfiler([]byte(fixture(t, "system_profiler_redacted.json")))
	if err != nil || !ok || !redacted {
		t.Fatalf("err %v ok %v redacted %v", err, ok, redacted)
	}
	if l.SSID != "" || l.Channel != 1 || l.Band != "2.4 GHz" || l.SignalDBm != -60 || l.Security != SecWPA2 {
		t.Errorf("link %+v", l)
	}
	if len(nets) != 2 || nets[1].SSID != "" || nets[1].Security != SecEnterprise {
		t.Errorf("nets %+v", nets)
	}
}

func TestParseSystemProfilerOff(t *testing.T) {
	l, ok, nets, _, err := parseSystemProfiler([]byte(fixture(t, "system_profiler_off.json")))
	if err != nil || ok || len(nets) != 0 || l.Connected {
		t.Errorf("got %+v %v %v %v", l, ok, nets, err)
	}
	if _, _, _, _, err := parseSystemProfiler([]byte("not json")); err == nil {
		t.Error("bad JSON accepted")
	}
}

func TestParseAirport(t *testing.T) {
	l, ok := parseAirportInfo(fixture(t, "airport_info.txt"))
	want := Link{
		SSID: "HomeNet", BSSID: "04:d9:f5:0a:0b:0c", SignalDBm: -55, Quality: 90, Channel: 149,
		FrequencyMHz: 5745, Band: "5 GHz", TxRateMbps: 702, Security: SecWPA2, Connected: true,
	}
	if !ok || !reflect.DeepEqual(l, want) {
		t.Errorf("got %+v\nwant %+v", l, want)
	}
	if _, ok := parseAirportInfo("WARNING: The airport command line tool is deprecated and will be removed in a future release.\n"); ok {
		t.Error("deprecation notice parsed as a link")
	}

	nets := parseAirportScan(fixture(t, "airport_scan.txt"))
	if len(nets) != 4 {
		t.Fatalf("got %d networks: %+v", len(nets), nets)
	}
	wantSec := []Security{SecWPA2, SecWPA2, SecOpen, SecWEP}
	wantSSID := []string{"HomeNet", "My Neighbour", "FreeWiFi", ""}
	for i, n := range nets {
		if n.Security != wantSec[i] || n.SSID != wantSSID[i] {
			t.Errorf("row %d = %+v", i, n)
		}
	}
	if nets[0].Channel != 149 || nets[0].SignalDBm != -55 || nets[0].BSSID != "04:d9:f5:0a:0b:0c" {
		t.Errorf("row 0 = %+v", nets[0])
	}
}
