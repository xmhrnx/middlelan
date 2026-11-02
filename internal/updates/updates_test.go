package updates

import "testing"

func TestParseLatest(t *testing.T) {
	body := []byte(`{"tag_name":"v1.2.0.7","body":"Notes","html_url":"https://x/r","draft":false,"prerelease":false}`)
	r, err := parseLatest(body, "1.0.0")
	if err != nil || r == nil || r.Version != "1.2.0" || r.URL != "https://x/r" {
		t.Fatal(r, err)
	}
	if r, _ := parseLatest(body, "1.2.0"); r != nil {
		t.Fatal("same version reported as newer")
	}
	if r, _ := parseLatest([]byte(`{"tag_name":"v9.0.0","prerelease":true}`), "1.0.0"); r != nil {
		t.Fatal("prerelease offered")
	}
}
