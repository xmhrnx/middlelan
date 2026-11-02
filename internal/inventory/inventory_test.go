package inventory

import (
	"testing"
	"time"
)

func TestMergeReportsNewDevices(t *testing.T) {
	inv := Open("")
	now := time.Now()
	if n := inv.Merge([]Device{{IP: "192.168.1.1", MAC: "AA:BB:CC:00:00:01"}}, now); len(n) != 0 {
		t.Fatalf("first scan reported %d new", len(n))
	}
	n := inv.Merge([]Device{{IP: "192.168.1.1", MAC: "aa:bb:cc:00:00:01"}, {IP: "192.168.1.20"}}, now)
	if len(n) != 1 || n[0].IP != "192.168.1.20" {
		t.Fatalf("new: %+v", n)
	}
	list := inv.List()
	if len(list) != 2 || list[0].IP != "192.168.1.1" {
		t.Fatalf("list: %+v", list)
	}
	if n := inv.Merge([]Device{{IP: "192.168.1.20", MAC: "11:22:33:44:55:66"}}, now); len(n) != 0 {
		t.Fatalf("IP-only device re-keyed by MAC reported as new: %+v", n)
	}
}
