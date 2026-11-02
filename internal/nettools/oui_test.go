package nettools

import "testing"

func TestVendor(t *testing.T) {
	tests := map[string]string{
		"00:00:00:12:34:56": "XEROX CORPORATION",
		"00-00-00-12-34-56": "XEROX CORPORATION",
		"0000.0012.3456":    "XEROX CORPORATION",
	}
	for mac, want := range tests {
		if got := Vendor(mac); got != want {
			t.Errorf("Vendor(%q) = %q, want %q", mac, got, want)
		}
	}
	if got := Vendor("3c:22:fb:00:00:00"); got == "" {
		t.Error("expected a vendor for 3c:22:fb (Apple)")
	}
	for _, mac := range []string{"", "zz:zz:zz:zz:zz:zz", "02:00:00:00:00:01", "12"} {
		if got := Vendor(mac); got != "" {
			t.Errorf("Vendor(%q) = %q, want empty", mac, got)
		}
	}
}

func TestIsRandomMAC(t *testing.T) {
	if !IsRandomMAC("da:a1:19:00:00:01") || IsRandomMAC("00:1a:2b:3c:4d:5e") {
		t.Error("IsRandomMAC wrong")
	}
}
