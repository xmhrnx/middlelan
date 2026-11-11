//go:build ios

package nettools

import (
	"net"
	"os"
	"syscall"

	"golang.org/x/net/route"
)

// systemGatewayDNS returns the default gateway on iOS, read from the
// routing table (apps can't run commands there). DNS servers come from
// /etc/resolv.conf where the system provides one.
func systemGatewayDNS(string) (string, []string) {
	var dns []string
	if b, err := os.ReadFile("/etc/resolv.conf"); err == nil {
		dns = parseResolvConf(string(b))
	}
	return defaultGatewayRIB(), dns
}

// defaultGatewayRIB finds the IPv4 default route's gateway.
func defaultGatewayRIB() string {
	rib, err := route.FetchRIB(syscall.AF_INET, route.RIBTypeRoute, 0)
	if err != nil {
		return ""
	}
	msgs, err := route.ParseRIB(route.RIBTypeRoute, rib)
	if err != nil {
		return ""
	}
	for _, m := range msgs {
		rm, ok := m.(*route.RouteMessage)
		if !ok || rm.Flags&syscall.RTF_GATEWAY == 0 || len(rm.Addrs) <= syscall.RTAX_GATEWAY {
			continue
		}
		dst, ok := rm.Addrs[syscall.RTAX_DST].(*route.Inet4Addr)
		if !ok || dst.IP != [4]byte{} {
			continue
		}
		if gw, ok := rm.Addrs[syscall.RTAX_GATEWAY].(*route.Inet4Addr); ok {
			return net.IP(gw.IP[:]).String()
		}
	}
	return ""
}
