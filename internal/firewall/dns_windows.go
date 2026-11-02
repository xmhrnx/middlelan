//go:build windows

package firewall

import (
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

// systemDNSServers returns the DNS servers of every network adapter. On
// failure it returns none: loopback and DHCP stay protected regardless.
func systemDNSServers() []netip.Addr {
	size := uint32(15000)
	for range 3 {
		buf := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, windows.GAA_FLAG_SKIP_ANYCAST|windows.GAA_FLAG_SKIP_MULTICAST, 0, first, &size)
		if err == windows.ERROR_BUFFER_OVERFLOW {
			continue
		}
		if err != nil {
			return nil
		}
		var out []netip.Addr
		for aa := first; aa != nil; aa = aa.Next {
			for d := aa.FirstDnsServerAddress; d != nil; d = d.Next {
				if a, ok := netip.AddrFromSlice(d.Address.IP()); ok {
					out = append(out, a.Unmap().WithZone(""))
				}
			}
		}
		return dedupeAddrs(out)
	}
	return nil
}
