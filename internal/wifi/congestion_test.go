package wifi

import (
	"reflect"
	"testing"
)

func TestCongestion(t *testing.T) {
	nets := []Network{
		{SSID: "a", Channel: 1, Band: "2.4 GHz", SignalDBm: -40},
		{SSID: "b", Channel: 1, Band: "2.4 GHz", SignalDBm: -70},
		{SSID: "c", Channel: 3, Band: "2.4 GHz", SignalDBm: -80}, // overlaps 1 and 6
		{SSID: "d", Channel: 11, Band: "2.4 GHz", SignalDBm: -60},
		{SSID: "e", FrequencyMHz: 5180, SignalDBm: -50},      // ch 36, derived
		{SSID: "f", Channel: 36, Band: "5 GHz", Quality: 50}, // -75 dBm from quality
		{SSID: "g", Channel: 0, Band: ""},                    // ignored
		{SSID: "h", FrequencyMHz: 6135, SignalDBm: -66},      // 6 GHz ch 37
	}
	uses, best := Congestion(nets)
	want := []ChannelUse{
		{Band: "2.4 GHz", Channel: 1, Networks: 2, StrongestDBm: -40},
		{Band: "2.4 GHz", Channel: 3, Networks: 1, StrongestDBm: -80},
		{Band: "2.4 GHz", Channel: 11, Networks: 1, StrongestDBm: -60},
		{Band: "5 GHz", Channel: 36, Networks: 2, StrongestDBm: -50},
		{Band: "6 GHz", Channel: 37, Networks: 1, StrongestDBm: -66},
	}
	if !reflect.DeepEqual(uses, want) {
		t.Errorf("uses = %+v\nwant %+v", uses, want)
	}
	// 2.4: ch1 sees 3 nets, ch6 sees 1 (ch3, -80), ch11 sees 1 (-60) -> 6.
	wantBest := map[string]int{"2.4 GHz": 6, "5 GHz": 40, "6 GHz": 5}
	if !reflect.DeepEqual(best, wantBest) {
		t.Errorf("best = %v, want %v", best, wantBest)
	}
}

func TestCongestionEmpty(t *testing.T) {
	uses, best := Congestion(nil)
	if len(uses) != 0 || len(best) != 0 {
		t.Errorf("got %v %v", uses, best)
	}
}
