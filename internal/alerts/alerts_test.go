package alerts

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xmhrnx/middlelan/internal/monitor"
	"github.com/xmhrnx/middlelan/internal/threat"
)

func TestCheck(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "threat"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A cached Feodo list with one C2 address.
	if err := os.WriteFile(filepath.Join(dir, "threat", "feodo.txt"), []byte("# feodo\n203.0.113.9\n198.51.100.0\n45.9.148.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db := threat.Open(dir)
	w := New(db, nil, dir)
	conns := []monitor.Conn{{Process: "evil", RemoteIP: "45.9.148.3", RemotePort: 443, State: "ESTABLISHED"}, {Process: "ok", RemoteIP: "1.1.1.1", RemotePort: 443, State: "ESTABLISHED"}}
	fresh := w.Check(conns, time.Now())
	if len(fresh) != 1 || fresh[0].App != "evil" {
		t.Fatalf("%+v", fresh)
	}
	if again := w.Check(conns, time.Now()); len(again) != 0 {
		t.Fatal("reported twice")
	}
	if w.Active() != 1 {
		t.Fatal(w.Active())
	}
	w.Dismiss(fresh[0].Key())
	if w.Active() != 0 {
		t.Fatal("dismiss")
	}
	if len(New(db, nil, dir).List()) != 1 {
		t.Fatal("not saved")
	}
}
