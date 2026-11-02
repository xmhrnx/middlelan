package wifi

import (
	"errors"
	"testing"
)

func TestParseAndroidCurrent(t *testing.T) {
	l, err := parseAndroidCurrent("1\tHomeNet\taa:bb:cc:dd:ee:02\t-58\t5180\t433\t390\t433\t2\t[RSN-PSK-CCMP][ESS]")
	if err != nil {
		t.Fatal(err)
	}
	if l.SSID != "HomeNet" || l.BSSID != "aa:bb:cc:dd:ee:02" || l.SignalDBm != -58 || l.Channel != 36 ||
		l.Band != "5 GHz" || l.TxRateMbps != 390 || l.RxRateMbps != 433 || l.Security != SecWPA2 || !l.Connected {
		t.Errorf("got %+v", l)
	}

	// Older Android: no tx/rx speeds or security type; security from scan caps.
	l, err = parseAndroidCurrent("1\t\"Cafe\"\taa:bb:cc:dd:ee:01\t-70\t2437\t72\t0\t0\t\t[RSN-PSK+SAE-CCMP][ESS]")
	if err != nil || l.SSID != "Cafe" || l.TxRateMbps != 72 || l.Security != SecWPA2WPA3 {
		t.Errorf("got %+v, %v", l, err)
	}

	l, err = parseAndroidCurrent("1\t<unknown ssid>\t02:00:00:00:00:00\t-60\t2412\t65\t65\t65\t-1\t")
	if !errors.Is(err, ErrNeedsLocation) || l.SSID != "" || l.BSSID != "" || l.Channel != 1 || l.SignalDBm != -60 || l.Security != SecUnknown {
		t.Errorf("unknown ssid: %+v, %v", l, err)
	}

	if _, err := parseAndroidCurrent("0\t<unknown ssid>\t\t-127\t-1\t-1\t-1\t-1\t-1\t"); !errors.Is(err, ErrNotWiFi) {
		t.Errorf("disconnected: %v", err)
	}
	if _, err := parseAndroidCurrent("ERR:off"); !errors.Is(err, ErrNotWiFi) {
		t.Errorf("off: %v", err)
	}
	if _, err := parseAndroidCurrent("garbage"); err == nil {
		t.Error("garbage accepted")
	}
}

func TestParseAndroidScan(t *testing.T) {
	nets, err := parseAndroidScan("HomeNet\taa:bb:cc:dd:ee:02\t-58\t5180\t[RSN-PSK-CCMP][ESS]\n" +
		"\taa:bb:cc:dd:ee:07\t-80\t2437\t[ESS]\n" +
		"Office\taa:bb:cc:dd:ee:04\t-75\t5955\t[RSN-SAE-CCMP][ESS]\n")
	if err != nil || len(nets) != 3 {
		t.Fatalf("got %+v, %v", nets, err)
	}
	if nets[1].Security != SecOpen || nets[1].Channel != 6 || nets[2].Band != "6 GHz" || nets[2].Security != SecWPA3 {
		t.Errorf("got %+v", nets)
	}
	if _, err := parseAndroidScan("ERR:location"); !errors.Is(err, ErrNeedsLocation) {
		t.Errorf("location: %v", err)
	}
	if nets, err := parseAndroidScan(""); err != nil || len(nets) != 0 {
		t.Errorf("empty: %v %v", nets, err)
	}
}
