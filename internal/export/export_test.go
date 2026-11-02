package export

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/xmhrnx/middlelan/internal/monitor"
)

func TestExports(t *testing.T) {
	s := monitor.OpenStore("")
	now := time.Now()
	s.Seen("browser", now)
	s.Add("browser", now, 1000, 200)
	s.AddDestination("browser", "1.2.3.4", 443, now, 1000, 200, true)
	d := Data{Store: s, Describe: func(string) (string, string, string) { return "example.com", "Example", "US" }}
	var b bytes.Buffer
	if err := d.UsageCSV(&b); err != nil || !strings.Contains(b.String(), "browser,1000,200") {
		t.Fatalf("%v %q", err, b.String())
	}
	b.Reset()
	if err := d.DestinationsCSV(&b); err != nil || !strings.Contains(b.String(), "1.2.3.4,443,example.com,Example,US") {
		t.Fatalf("%v %q", err, b.String())
	}
	b.Reset()
	if err := d.JSON(&b); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b.Bytes(), &doc); err != nil || doc["usage"] == nil {
		t.Fatal(err, doc)
	}
}
