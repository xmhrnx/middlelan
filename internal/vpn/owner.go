package vpn

import (
	"strconv"
	"strings"
	"sync"
)

// ownerLookup names flow owners through the platform's two lookups, uid
// by connection and label by uid, caching labels per uid.
type ownerLookup struct {
	uidOf   func(proto int, srcIP string, srcPort int, dstIP string, dstPort int) int
	labelOf func(uid int) string // "label\tpackage"

	mu     sync.Mutex
	labels map[int][2]string
}

func newOwnerLookup(uidOf func(int, string, int, string, int) int, labelOf func(int) string) *ownerLookup {
	return &ownerLookup{uidOf: uidOf, labelOf: labelOf, labels: map[int][2]string{}}
}

// Owner implements Hooks.Owner.
func (o *ownerLookup) Owner(proto int, srcIP string, srcPort int, dstIP string, dstPort int) (int, string, string) {
	uid := o.uidOf(proto, srcIP, srcPort, dstIP, dstPort)
	if uid < 0 {
		return -1, "", ""
	}
	o.mu.Lock()
	l, ok := o.labels[uid]
	o.mu.Unlock()
	if !ok {
		l[0], l[1] = parseLabel(o.labelOf(uid), uid)
		o.mu.Lock()
		if len(o.labels) > 4096 { // uids are few; bound it anyway
			clear(o.labels)
		}
		o.labels[uid] = l
		o.mu.Unlock()
	}
	return uid, l[0], l[1]
}

// parseLabel splits "label\tpackage", falling back to "uid N".
func parseLabel(s string, uid int) (app, pkg string) {
	app, pkg, _ = strings.Cut(strings.TrimRight(s, "\r\n"), "\t")
	app, pkg = strings.TrimSpace(app), strings.TrimSpace(pkg)
	if app == "" {
		app = pkg
	}
	if app == "" {
		app = "uid " + strconv.Itoa(uid)
	}
	return app, pkg
}
