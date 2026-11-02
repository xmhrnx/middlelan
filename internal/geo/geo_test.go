package geo

import "testing"

func TestClassify(t *testing.T) {
	cases := map[string]Scope{
		"8.8.8.8": ScopePublic, "192.168.1.1": ScopePrivate, "127.0.0.1": ScopeLoopback,
		"fe80::1": ScopeLinkLocal, "224.0.0.251": ScopeMulticast, "100.64.1.1": ScopePrivate, "::": ScopeInvalid,
	}
	for ip, want := range cases {
		if got := Classify(ip); got != want {
			t.Errorf("%s: got %s want %s", ip, got, want)
		}
	}
}

func TestParseGeojs(t *testing.T) {
	body := `[{"organization_name":"GOOGLE","region":"California","asn":15169,"longitude":"-97.822","ip":"8.8.8.8","city":"Mountain View","country":"United States","country_code":"US","latitude":"37.751"}]`
	infos, err := parseGeojs([]byte(body))
	if err != nil || len(infos) != 1 {
		t.Fatal(err, infos)
	}
	in := infos[0]
	if in.Org != "Google" || in.Lat != 37.751 || in.ASN != 15169 || !in.Located || in.Place() != "Mountain View, United States" {
		t.Fatalf("%+v", in)
	}
}

func TestServiceName(t *testing.T) {
	if s := ServiceName("lhr25s34-in-f14.1e100.net", ""); s != "Google" {
		t.Fatal(s)
	}
}
