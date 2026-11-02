package wifi

import (
	"reflect"
	"testing"
)

func TestSplitNmcli(t *testing.T) {
	got := splitNmcli(`yes:Cafe\: Guest:AA\:BB::x\\y`)
	want := []string{"yes", "Cafe: Guest", "AA:BB", "", `x\y`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestParseNmcli(t *testing.T) {
	rows := parseNmcli(fixture(t, "nmcli_list.txt"))
	if len(rows) != 7 {
		t.Fatalf("got %d rows", len(rows))
	}
	r := rows[0]
	if r.SSID != "Cafe: Guest" || r.BSSID != "aa:bb:cc:dd:ee:01" || r.Quality != 82 || r.SignalDBm != -33 ||
		r.Channel != 6 || r.FrequencyMHz != 2437 || r.Band != "2.4 GHz" || r.Security != SecOpen ||
		r.RateMbps != 54 || r.Device != "wlp2s0" || r.Connected {
		t.Errorf("row 0 = %+v", r)
	}
	if !rows[1].Connected || rows[1].Band != "5 GHz" || rows[1].Security != SecWPA2 {
		t.Errorf("row 1 = %+v", rows[1])
	}
	wantSec := []Security{SecOpen, SecWPA2, SecWPA2, SecWPA3, SecEnterprise, SecWEP, SecWPA2WPA3}
	for i, r := range rows {
		if r.Security != wantSec[i] {
			t.Errorf("row %d security %q, want %q", i, r.Security, wantSec[i])
		}
	}
	if rows[3].Band != "6 GHz" || rows[3].Channel != 1 {
		t.Errorf("6 GHz row = %+v", rows[3])
	}
	if rows[5].SSID != `Back\slash` {
		t.Errorf("escaped backslash: %q", rows[5].SSID)
	}
	if rows[6].SSID != "" {
		t.Errorf("hidden SSID: %q", rows[6].SSID)
	}
}
