package nettools

import (
	"reflect"
	"testing"
)

func TestParsePortSpec(t *testing.T) {
	tests := []struct {
		in      string
		want    []int
		wantErr bool
	}{
		{"22,80,8000-8003", []int{22, 80, 8000, 8001, 8002, 8003}, false},
		{" 443 , 80 ,80", []int{80, 443}, false},
		{"1-3;5", []int{1, 2, 3, 5}, false},
		{"65535", []int{65535}, false},
		{"", nil, true},
		{"0", nil, true},
		{"65536", nil, true},
		{"abc", nil, true},
		{"10-5", nil, true},
		{"1-", nil, true},
	}
	for _, tc := range tests {
		got, err := ParsePortSpec(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParsePortSpec(%q) err=%v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParsePortSpec(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	all, err := ParsePortSpec("1-65535")
	if err != nil || len(all) != 65535 {
		t.Fatalf("full range: len=%d err=%v", len(all), err)
	}
}

func TestServiceNameAndCommonPorts(t *testing.T) {
	if ServiceName(22) != "ssh" || ServiceName(443) != "https" || ServiceName(1) != "" {
		t.Error("unexpected service names")
	}
	if len(CommonPorts) < 90 {
		t.Errorf("CommonPorts has %d entries", len(CommonPorts))
	}
	seen := map[int]bool{}
	for _, p := range CommonPorts {
		if seen[p] {
			t.Errorf("duplicate port %d", p)
		}
		seen[p] = true
	}
}

func TestSanitizeBanner(t *testing.T) {
	if got := sanitizeBanner("SSH-2.0-OpenSSH_9.6\r\nmore"); got != "SSH-2.0-OpenSSH_9.6" {
		t.Errorf("got %q", got)
	}
}
