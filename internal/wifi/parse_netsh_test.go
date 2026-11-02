package wifi

import (
	"reflect"
	"testing"
)

func TestParseNetshInterfaces(t *testing.T) {
	l, ok := parseNetshInterfaces(fixture(t, "netsh_interfaces_en.txt"))
	want := Link{
		Interface: "Wi-Fi", SSID: "Home: Upstairs", BSSID: "a4:2b:b0:11:22:33",
		SignalDBm: -46, Quality: 92, Channel: 44, FrequencyMHz: 5220, Band: "5 GHz",
		TxRateMbps: 960.5, RxRateMbps: 1201, Security: SecWPA2, Connected: true,
	}
	if !ok || !reflect.DeepEqual(l, want) {
		t.Errorf("got %+v, %v\nwant %+v", l, ok, want)
	}
}

func TestParseNetshInterfacesDisconnected(t *testing.T) {
	l, ok := parseNetshInterfaces(fixture(t, "netsh_interfaces_disconnected.txt"))
	if ok || l.Interface != "WLAN" || l.Connected {
		t.Errorf("got %+v, %v", l, ok)
	}
}

func TestParseNetshNetworks(t *testing.T) {
	nets := parseNetshNetworks(fixture(t, "netsh_networks_en.txt"))
	finishNetworks(nets)
	want := []Network{
		{SSID: "Home: Upstairs", BSSID: "a4:2b:b0:11:22:33", SignalDBm: -54, Quality: 92, Channel: 44, FrequencyMHz: 5220, Band: "5 GHz", Security: SecWPA2},
		{SSID: "Home: Upstairs", BSSID: "a4:2b:b0:11:22:34", SignalDBm: -65, Quality: 70, Channel: 6, FrequencyMHz: 2437, Band: "2.4 GHz", Security: SecWPA2},
		{SSID: "", BSSID: "10:20:30:40:50:60", SignalDBm: -80, Quality: 40, Channel: 1, FrequencyMHz: 2412, Band: "2.4 GHz", Security: SecOpen},
		{SSID: "OldRouter", BSSID: "00:11:22:33:44:55", SignalDBm: -90, Quality: 20, Channel: 11, FrequencyMHz: 2462, Band: "2.4 GHz", Security: SecWEP},
	}
	if !reflect.DeepEqual(nets, want) {
		t.Errorf("got  %+v\nwant %+v", nets, want)
	}
	markConnected(nets, Link{BSSID: "a4:2b:b0:11:22:33"})
	if !nets[0].Connected || nets[1].Connected {
		t.Errorf("markConnected: %+v", nets)
	}
}

func TestNetshRefusals(t *testing.T) {
	if !netshNeedsLocation(fixture(t, "netsh_location.txt")) {
		t.Error("location refusal not detected")
	}
	if !netshNoWiFi("There is no wireless interface on the system.\r\n") {
		t.Error("no-interface message not detected")
	}
	if netshNoWiFi(fixture(t, "netsh_interfaces_en.txt")) || netshNeedsLocation(fixture(t, "netsh_interfaces_en.txt")) {
		t.Error("false positive on normal output")
	}
}
