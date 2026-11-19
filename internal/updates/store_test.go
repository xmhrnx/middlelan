package updates

import "testing"

func TestStoreManaged(t *testing.T) {
	cases := []struct {
		goos, exe string
		want      bool
	}{
		{"android", "/data/app/x/lib/libgio.so", true},
		{"windows", `C:\Program Files\WindowsApps\MehranZahir.MiddleMan_1.0.0.0_x64__abc\MiddleMan.exe`, true},
		{"windows", `C:\Users\sam\AppData\Local\Programs\MiddleMan\MiddleMan.exe`, false},
		{"darwin", "/Applications/MiddleMan.app/Contents/MacOS/MiddleMan", false},
		{"linux", "/usr/bin/middlelan", false},
	}
	for _, c := range cases {
		if got := storeManaged(c.goos, c.exe); got != c.want {
			t.Errorf("storeManaged(%q, %q) = %v, want %v", c.goos, c.exe, got, c.want)
		}
	}
}
