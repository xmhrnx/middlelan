package monitor

import "strings"

// Interface kinds the iOS source reports as the phone's "apps".
const (
	ifaceWiFi     = "Wi-Fi"
	ifaceCellular = "Cellular"
	ifaceWired    = "Ethernet"
)

// ifaceKind names the kind of network an Apple interface is on, or ""
// for the ones that carry no internet traffic of their own (loopback,
// tunnels, peer-to-peer Wi-Fi, bridges).
func ifaceKind(name string) string {
	switch {
	case name == "en0":
		return ifaceWiFi
	case strings.HasPrefix(name, "pdp_ip"):
		return ifaceCellular
	case strings.HasPrefix(name, "en"):
		return ifaceWired // USB and Thunderbolt adapters
	}
	return ""
}

// counter32 widens a 32-bit kernel byte counter, which wraps every 4 GB,
// into a 64-bit total.
type counter32 struct {
	last  uint32
	total uint64
	seen  bool
}

func (c *counter32) update(v uint32) uint64 {
	if c.seen {
		c.total += uint64(v - c.last) // wraps correctly in uint32 arithmetic
	} else {
		c.total = uint64(v)
		c.seen = true
	}
	c.last = v
	return c.total
}
