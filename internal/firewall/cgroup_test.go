package firewall

import "testing"

func TestParseProcCgroup(t *testing.T) {
	p, ok := parseProcCgroup("12:pids:/user.slice\n0::/user.slice/user-1000.slice/app.slice/app-firefox-12.scope\n")
	if !ok || p != "/user.slice/user-1000.slice/app.slice/app-firefox-12.scope" {
		t.Fatalf("got %q %v", p, ok)
	}
	if _, ok := parseProcCgroup("1:name=systemd:/\n"); ok {
		t.Fatal("v1-only file parsed")
	}
	if r := cgroupRefFor("/a/b/c.scope"); r.Path != "a/b/c.scope" || r.Level != 3 {
		t.Fatalf("ref = %+v", r)
	}
	if r := cgroupRefFor("/"); r.Path != "" || r.Level != 0 {
		t.Fatalf("root ref = %+v", r)
	}
}

func TestDedicatedScope(t *testing.T) {
	const ff = "/usr/lib/firefox/firefox"
	cases := []struct {
		path    string
		exe     string
		members []string
		want    bool
	}{
		{"/user.slice/user-1000.slice/user@1000.service/app.slice/app-gnome-firefox-123.scope", ff, nil, true},
		{"/user.slice/user-1000.slice/user@1000.service/app.slice/app-flatpak-org.telegram.desktop-9.scope", "/app/bin/telegram-desktop", nil, false},
		{"/user.slice/user-1000.slice/user@1000.service/app.slice/app-flatpak-org.telegram.desktop-9.scope", "/app/bin/telegram-desktop", []string{"/app/bin/telegram-desktop"}, true},
		// A terminal's scope holds everything started from it.
		{"/user.slice/user-1000.slice/user@1000.service/app.slice/app-org.gnome.Terminal.slice/vte-spawn-1.scope", "/usr/bin/curl", []string{"/usr/bin/bash", "/usr/bin/curl"}, false},
		{"/user.slice/user-1000.slice/user@1000.service/app.slice/app-gnome-terminal-1.scope", "/usr/bin/curl", []string{"/usr/bin/bash", "/usr/bin/curl"}, false},
		{"/system.slice/nginx.service", "/usr/sbin/nginx", nil, true},
		{"/user.slice/user-1000.slice/session-2.scope", "/usr/bin/session", nil, false},
		{"/user.slice/user-1000.slice", "/usr/bin/user", nil, false},
		{"/init.scope", "/sbin/init", nil, false},
		{"/", ff, []string{ff}, false},
		{"/app.slice/app-x.scope", "/bin/ab", []string{"/bin/ab"}, true},
	}
	for _, c := range cases {
		if got := dedicatedScope(c.path, c.exe, c.members); got != c.want {
			t.Errorf("dedicatedScope(%q, %q, %v) = %v, want %v", c.path, c.exe, c.members, got, c.want)
		}
	}
	if validCgroupPath(`a"b`) || validCgroupPath("a\nb") || !validCgroupPath("/user.slice/app-x.scope") {
		t.Fatal("validCgroupPath")
	}
}
